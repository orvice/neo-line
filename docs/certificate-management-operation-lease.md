# 证书 Operation 多副本 Lease 与恢复

本文档说明 neo-line 多副本部署下 `CertificateOperation` 的 MongoDB lease 竞争、自动退避重试与进程中断恢复语义（#23）。

## 设计原则

- **MongoDB 是权威状态**：operation lease、attempt 计数与退避时间均持久化在 `certificate_operations` collection；**Redis 不参与** operation/lease。
- **不要求 MongoDB replica set transaction**：依赖单文档 compare-and-swap（CAS）与 partial unique index。
- **首版无 Cancel**：运行中的 operation 不可取消；Admin 只能等待完成或终态失败后再手动重试。
- **不恢复未知远端半完成 order**：副本接管后 best-effort 清理已记录 TXT，再在同一 operation 上开启新的 ACME order attempt。

## 互斥规则

- 同一 `ManagedCertificate` 在任意时刻最多 **一条** `Pending` 或 `Running` operation（Issue / Renew / Revoke 共享互斥，partial unique index：`uniq_inflight_per_cert`）。
- 相同类型的并发 Admin 请求（如重复点击「签发新版本」）返回 **现有** Pending/Running operation，不创建重复 CA order。
- 终态 `Failed` 后的 Admin 手动重试会 **创建新 operation**；自动重试仍属于 **同一 operation** 并递增 `attempt_count`。

## Operation 持久化字段

| 字段 | 说明 |
|------|------|
| `lease_owner` | 当前持有 lease 的副本 ID（hostname + 随机后缀） |
| `lease_expires_at` | lease 过期时间；过期后其他副本可接管 |
| `attempt_count` | 已执行的 attempt 总数（含接管后的新 attempt） |
| `consecutive_failures` | 连续失败次数，用于退避；成功后清零 |
| `next_attempt_at` | 自动重试最早执行时间（Pending 且等待退避时） |
| `pending_txt_records` | 本次 attempt 已创建的 DNS-01 TXT，供接管副本清理 |
| `started_at` / `finished_at` | operation 首次开始与终态/成功结束时间 |
| `config_snapshot` | 创建时冻结的签发参数 |
| `error_summary` / `warning` | 脱敏错误与告警（不含 token、PEM、ACME order URL） |

## Lease 生命周期

1. **领取**：operation runner 轮询可 claim 的 operation（Pending 且 `next_attempt_at` 已到，或 Running 且 lease 已过期），通过 CAS 设为 `Running`、写入 `lease_owner` / `lease_expires_at`、递增 `attempt_count`。
2. **续租**：执行 attempt 期间每 **30 秒**（lease 时长的 1/3）续租一次；默认 lease **90 秒**。
3. **成功提交**：仅当仍持有 lease 时方可激活 active/previous 并将 operation 标为 `Succeeded`。
4. **失去 lease**：若续租失败或 lease 已被接管，**不得**提交 active/previous 或终态结果。
5. **接管**：lease 过期后，其他副本 claim 同一 operation，先 **best-effort** 清理 `pending_txt_records`，再发起新的 ACME order（不尝试恢复旧 order 状态）。

## 自动退避

- 首次失败后 **15 分钟** 起算，指数翻倍，封顶 **12 小时**，并加入最多约 **10%** 的 jitter。
- 失败后 operation 回到 `Pending`，保留 `error_summary`，设置 `next_attempt_at`。
- 永久性错误（如 Issuer 非 Ready）直接标为终态 `Failed`，不自动退避。
- 成功后清空 `error_summary`、`next_attempt_at`、`consecutive_failures` 与 lease 字段。

## 优雅关闭

- 收到停止信号后，runner **停止领取新 lease**（`claimingLeases=false`）。
- 已在执行的 attempt 享有 **2 分钟** 有界宽限期完成清理与状态提交。
- certificate reconciler 与 Monitor scheduler **独立** 关闭；关闭顺序见进程 `TeardownFunc`。

## 与其他约束的交互

- **签发字段锁定**：Pending/Running operation 期间禁止修改 domains、Issuer、DNS 账户、key type。
- **删除 ManagedCertificate**：存在 Pending/Running operation 时禁止删除（`HasRunningCertificateOperation`）。
- **Admin UI**：展示 operation 类型、状态、attempt 次数、起止时间、下次重试时间与脱敏错误；不展示 lease owner、ACME order URL 或 secret。

## 默认值摘要

| 项 | 默认 |
|----|------|
| operation lease 时长 | 90 秒 |
| lease 续租间隔 | 30 秒 |
| operation 轮询 | 5 秒 |
| 退避起始 | 15 分钟 |
| 退避封顶 | 12 小时 |
| 关闭宽限期 | 2 分钟 |

## 相关文档

- [托管证书](./certificate-management-managed-certificates.md)
- [功能说明 — 证书管理](./features.md)
