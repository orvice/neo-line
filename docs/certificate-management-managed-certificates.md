# 证书管理 — 托管证书（ManagedCertificate）

本文档说明 Admin 管理的 `ManagedCertificate` desired config、与 Monitor 探测快照及签发版本的区别，以及 #17 实现的创建/更新与 Pending Issue operation 行为。

## 领域概念对比

| 概念 | 含义 | 存储位置 | 是否含私钥 |
|------|------|----------|------------|
| **ManagedCertificate** | Admin 配置的期望证书：域名、Issuer、DNS 账户、密钥类型、续期策略、Server 分配与通知组；并作为 active/previous **两个** CertificateVersion 的容器 | `managed_certificates` collection | 否（desired config 本身不含 PEM） |
| **CertificateVersion** | 一次 **成功签发** 得到的不可变 bundle（fullchain + 私钥）及签发参数快照；同一 ManagedCertificate 最多保留 active 与 previous 两个完整版本 | 嵌套在 `managed_certificates` 文档的 `active_version` / `previous_version` | 是（#18 起写入） |
| **CertificateInfo** | **Monitor TLS 探测** 读到的对端证书公开元数据快照（subject、issuer、到期日等） | Monitor 文档的 `certificate` 字段 | 否 |

要点：

- Monitor 上的 `CertificateInfo` 只反映 **线上已部署** 证书的观测结果，与 neo-line 是否托管该证书无关。
- ManagedCertificate 的 desired config 变更 **不会** 立即向 CA 下单；需显式 Issue（或创建时自动首次 Issue）才会产生 `CertificateOperation`。
- Server 分发接口（#19 起）只暴露 **active** CertificateVersion，不返回尚未发布的 desired config。

## API 与 UI

- Connect：`ManagedCertificateService`（List / Create / Get / Update），挂载于 `/api/grpc`。
- Web：`证书 → 托管证书`（列表、创建表单、详情：desired config + Missing 有效性 + Pending operation）。
- 所有读写接口 **不返回** DNS Token、EAB、ACME account key 或证书私钥。

## MongoDB：`managed_certificates`

| 字段 | 说明 |
|------|------|
| `id` | 自动生成，前缀 `mcert_` |
| `name` | 全局唯一显示名 |
| `domains` | 有序域名列表，**第一个为主域名**，其余为 SAN；最多 **100** 个 |
| `certificate_issuer_id` | 须引用 **Ready** 的 CertificateIssuer |
| `dns_provider_account_id` | 须存在的 DNSProviderAccount |
| `key_type` | `ec_p256`（默认）或 `rsa_2048` |
| `auto_renew_enabled` | 默认 **true** |
| `renew_before_days` | 默认 **30** |
| `notify_group_ids` | 可选，引用 NotifyGroup |
| `server_ids` | 可选，可为 **空**（允许先签发再分配） |
| `active_version` / `previous_version` | #18 起填充 |

域名规范化（创建/更新时）：

1. trim 空白  
2. 转小写  
3. IDNA 转 ASCII（如 `München.de` → `xn--mnchen-3ya.de`）  
4. 移除尾部 `.`  
5. DNS / 泛域名语法校验（仅最左标签允许 `*.example.com`）  
6. 去重并保持顺序  

## MongoDB：`certificate_operations`

| 字段 | 说明 |
|------|------|
| `id` | 前缀 `cop_` |
| `managed_certificate_id` | 所属证书 |
| `type` | `Issue` / `Renew` / `Revoke` |
| `status` | `Pending` / `Running` / `Succeeded` / `Failed` |
| `attempt_count` | 尝试次数（含多副本接管后的 attempt） |
| `consecutive_failures` | 连续失败次数（退避计算；成功后清零） |
| `lease_owner` / `lease_expires_at` | 多副本 lease（#23；Admin API 不返回） |
| `pending_txt_records` | 已创建 TXT 记录（接管清理用；Admin API 不返回） |
| `config_snapshot` | 创建时冻结的 domains、issuer、dns、key_type |
| `error_summary` / `warning` | 脱敏错误与告警摘要 |
| `started_at` / `finished_at` / `next_attempt_at` | 时间戳 |

## 行为（#17 / #21）

### Desired 与 active 分离（#21）

- 修改 **domains、issuer、DNS 账户、key_type** 只更新 desired config，**不会**自动创建 ACME order。
- 管理 UI 展示 desired 与 active 签发快照的差异，并提供 **「签发新版本」** 按钮（`SubmitIssueOperation`）。
- 显式 Issue 使用提交时的 desired 快照；Pending/Running Issue 期间继续禁止修改签发字段。
- 新版本 **失败** 时 active 与 previous 完全不变。
- 新版本 **成功** 后，在同一 `managed_certificates` 文档内原子将原 active 移为 previous、新版本设为 active；产生第三个成功版本时丢弃更老 previous 的本地 PEM（不向 CA 隐式吊销）。
- Admin 可下载 **active** 或 **previous** bundle（`GetCertificateBundle.version_slot`），响应含准确 version ID 且 `Cache-Control: no-store`。
- Admin 可将任何 **未吊销** previous 重新激活（允许过期版本；UI 需显著警告并确认）；回滚后 desired config **不变**。
- 已吊销 previous（`revoked_at` 已设置）永远不能重新激活。
- Admin 可 **`SubmitRevokeVersion`** 吊销 active 或 previous（详见 [停用/吊销/回滚/删除](./certificate-management-destructive-operations.md)）。
- Admin 可 **`DeleteManagedCertificate`** 删除无 Server 分配且无运行 operation 的证书（本地级联，不隐式吊销）。
- `active_validity` 根据当前时间计算 Missing / Valid / RenewalDue / Expired / Revoked，与 operation 状态、auto-renew 正交。

### 自动续期（#22）

- **certificate reconciler** 与 Monitor scheduler **独立** 构造、启动与停止；默认 **每小时** 扫描 `auto_renew_enabled = true` 且存在 `active_version` 的证书。
- 当 `active_validity = RenewalDue` 时，reconciler 自动创建 **Renew** operation；关闭自动续期时不创建 operation，但 active bundle 仍可下载。
- **Renew** 严格使用 **active version 签发快照**（domains、Issuer、DNS 账户、key type），不使用尚未发布的 desired config。
- **Issue**（「签发新版本」）使用当前 **desired config** 快照；两者语义在 UI 中分别展示。
- `renew_before_days` 默认 **30**；**有效续期窗口** = `min(renew_before_days, 证书总有效期 / 3)`。
- 每次 Renew 生成 **新私钥**，成功经双版本原子切换写入 active；失败时 active/previous 不变。
- 相同 **Renew** 正在 Pending/Running 时，reconcile 或重复手动请求 **复用同一条** operation。
- 自动 attempt 失败后 operation 回到 **Pending** 并按 15 分钟起、12 小时封顶的指数退避调度 `next_attempt_at`；终态 **Failed** 后 Admin 手动重试创建 **新** operation（#23）。
- Admin 可在自动续期关闭时手动 **续期 active** 或 **签发 desired**；详情展示配置/有效窗口、下次自动续期与手动 Renew。

### 多副本 lease 与恢复（#23）

- 详见 [Operation Lease 与恢复](./certificate-management-operation-lease.md)。
- MongoDB partial unique index 保证每张证书最多一条 Pending/Running operation。
- 进程中断后 lease 过期，其他副本接管同一 operation：清理已知 TXT → 新 ACME order attempt。
- 首版 **不提供 Cancel**；graceful shutdown 停止新 lease，当前 attempt 有界完成。

### 创建

1. 校验 Ready Issuer、存在的 DNS 账户、NotifyGroup / Server ID（Server 可为空）。  
2. 写入 desired config。  
3. **自动** 创建一条 `Issue` + `Pending` 的 CertificateOperation（含 config 快照）。  

### 有效性（尚无 active version）

- `active_validity` = **Missing**  
- `bundle_available` = **false**  

### Operation 进行中（Pending 或 Running）

- 修改 **domains、issuer、DNS 账户、key_type** → Connect `failed_precondition`。  
- 仍可修改 **name、server_ids、notify_group_ids**（详情页可独立保存 Server 分配，见 [访问 Token 与 Server 分配](./certificate-management-access-tokens.md)）。  

### 幂等 Issue 提交

- 若同一 ManagedCertificate 已有 **Pending 或 Running** 的 Issue operation，再次提交返回 **同一条** operation，不创建第二条（`SubmitIssueOperation` 内部逻辑；显式 Issue RPC 在后续 ticket 暴露）。  

## 证书生命周期通知

ManagedCertificate 通过 `notify_group_ids` 直接引用 NotifyGroup，**不经过** MonitorGroup `AlertPolicy`。Monitor 探测告警与证书事件复用同一组 webhook/Telegram/Discord/Mastodon 传输 adapter，但 webhook JSON 与人类可读文本各自渲染，证书 payload **不包含** `monitor_id`、`group_id` 或 HealthStatus。

### 事件与节流

- **首次 Issue/Renew 失败**：立即发送 `certificate_operation_failed`。
- **持续失败**：同一失败 episode 内最多每 **24 小时** 发送 `certificate_operation_failed_reminder`。
- **恢复**：此前存在失败时，首次成功 Issue/Renew 发送一次 `certificate_operation_recovered`。
- **7 天提醒**：active 剩余 ≤7 天且尚未被新版本替换时，每个 active version 提醒一次。
- **过期**：active 进入 `Expired` 时，每个 active version 通知一次。

### `notification_state` 字段

| 字段 | 含义 |
|------|------|
| `had_operation_failure` | 当前失败 episode |
| `last_fail_notified_at` | 最近失败/提醒时间（24h 节流） |
| `seven_day_reminder_version_id` | 已发送 7 天提醒的 version |
| `expired_notified_version_id` | 已发送过期通知的 version |
| `last_notification_warning` | 最近通道投递失败摘要 |

删除 NotifyGroup 时从证书 `notify_group_ids` 中移除引用，不删除证书。通道失败不改变 operation 或 active/previous 版本。

## 默认值摘要

| 项 | 默认 |
|----|------|
| 密钥类型 | EC P-256 |
| 自动续期 | 开启 |
| renew_before_days | 30 |
| 有效续期窗口 | min(renew_before_days, 证书有效期 / 3) |
| 自动续期扫描周期 | 每小时 |
| Issue operation 执行轮询 | 5 秒 |
| Renew operation 执行轮询 | 5 秒（与 Issue 共用 operation runner） |
| Server 分配 | 空（可选） |

## 相关文档

- [DNS 账户](./certificate-management-dns-accounts.md)  
- [ACME Issuer](./certificate-management-issuers.md)  
- [功能说明 — 证书管理](./features.md)  
