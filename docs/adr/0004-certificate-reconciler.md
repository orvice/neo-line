# ADR-0004：独立 Certificate Reconciler

## 状态

已接受（2026-05）

## 背景

ManagedCertificate 需要自动续期、失败退避、多副本 lease 互斥与 validity 通知扫描。Monitor 调度器已有独立生命周期与 probe 语义，不应与 ACME operation 共享 goroutine 或重试状态。

## 决策

- 新增 **certificate reconciler**（`internal/certmanager/reconciler.go`），与 Monitor **scheduler 分开** 启动、运行与关闭。
- 默认 **每小时** 扫描 `auto_renew_enabled=true` 且 `active_validity=RenewalDue` 的证书并 enqueue Renew（使用 **active 快照**，非未发布 desired）。
- Renew/Issue/Revoke 执行由 **operation runner** _poll loop 领取；MongoDB lease + CAS 保证同证书互斥。
- 失败退避：自 **15 分钟** 指数增长至最长 **12 小时** 并加 jitter；成功清零连续失败计数。
- 同一 reconcile tick 刷新 **低基数** validity metrics 并触发 7 天/Expired 通知扫描（与 NotifyGroup 通道复用，语义独立）。

## 后果

- `cmd/server/main.go` 需分别 teardown scheduler 与 certificate reconciler/operation runner。
- 优雅关闭：停止领取新 lease，允许 in-flight attempt 在总超时内结束。
- 运维可独立观察 `neoline_cert_operation_total` 与 `neoline_cert_renew_failures_total`。

## 参考

- [Operation Lease 与多副本](./../certificate-management-operation-lease.md)
- [托管证书 — 自动续期](./../certificate-management-managed-certificates.md)
