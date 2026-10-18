# 证书管理 — 停用、吊销、回滚与删除

本文档说明 ManagedCertificate 四类破坏性操作的语义差异与约束（#25）。它们彼此独立，Admin 应理解每种操作的精确后果。

## 操作对比

| 操作 | API / UI | 影响 desired config | 影响 active/previous PEM | 调用 CA | Server 分发 |
|------|----------|---------------------|--------------------------|---------|-------------|
| **停用自动续期** | 更新 `auto_renew_enabled=false` | 否 | 否；active 仍可下载 | 否 | 不变 |
| **吊销版本** | `SubmitRevokeVersion` | 否 | 目标版本立即 `revoke_pending`；成功后 `revoked_at` | 是（ACME Revoke） | 目标版本立即停止 |
| **回滚 previous** | `ActivatePreviousVersion` | 否 | 交换 active/previous | 否 | 跟随新 active |
| **删除证书** | `DeleteManagedCertificate` | 删除本地记录 | 删除本地 PEM | **否**（不隐式吊销） | 分配解除后自然失效 |

要点：

- **停用**只关闭 reconciler 自动 Renew；Admin 仍可 Issue desired 或 Renew active。
- **吊销**针对指定 CertificateVersion（active 或 previous），接受请求后 **立即** 阻止分发，不等待 CA；CA 失败保持阻止并指数退避重试。
- **回滚**只交换本地 active/previous；已吊销 previous 禁止激活；过期未吊销 previous 可在显著警告后激活。
- **删除**只清理 neo-line 本地状态；**不会**向 CA 提交吊销；`audit_logs` 保留；CertificateAccessToken 因绑定 Server 而不被误删。

## 吊销（RevokeVersion）

### Connect

- `ManagedCertificateService.SubmitRevokeVersion`
- 参数：`managed_certificate_id`、`version_id`、可选 `revocation_reason`（RFC 5280，默认 **unspecified / 0**）

### 行为

1. 校验版本存在于 active 或 previous，且未吊销、未处于 `revoke_pending`。
2. **立即** 在 MongoDB 将目标版本设为 `revoke_pending=true`（停止 Admin 下载与 Server 分发）。
3. 创建 `Revoke` + `Pending` 的 `CertificateOperation`（含 `target_version_id`、`revoke_reason`、config 快照）。
4. Operation runner 通过 Mongo lease 执行 ACME Revoke；失败时 operation 回到 Pending 并按 15 分钟～12 小时退避重试。
5. 成功后设置 `revoked_at`，清除 `revoke_pending`；`active_validity` 可为 **Revoked**，`bundle_available=false`（若 active 被吊销）。
6. 吊销 active **不会**自动激活 previous、**不会**自动 Issue 新版本。

### RFC 5280 原因码

| Proto / UI 值 | 码 | 含义 |
|---------------|-----|------|
| unspecified | 0 | 未指定（默认） |
| key_compromise | 1 | 密钥泄露 |
| ca_compromise | 2 | CA 泄露 |
| affiliation_changed | 3 | 隶属关系变更 |
| superseded | 4 | 已被替代 |
| cessation_of_operation | 5 | 停止运营 |
| certificate_hold | 6 | 证书挂起 |
| privilege_withdrawn | 9 | 权限撤销 |
| aa_compromise | 10 | AA 泄露 |

## 停用自动续期

- 字段：`auto_renew_enabled=false`（UpdateManagedCertificate）。
- reconciler **不再**为该证书创建 Renew operation。
- **active bundle 仍可下载**；Admin 手动 Issue / Renew **不受影响**。
- 与吊销、删除无耦合。

## 回滚（ActivatePreviousVersion）

- 将 **未吊销** 的 previous 与 active 交换；desired config **不变**。
- 已吊销 previous（`revoked_at` 已设置）→ `failed_precondition`。
- 过期但未吊销 previous **允许**激活；UI 须显著警告（灾难恢复场景）。
- 不回滚 CA 侧状态，仅切换本地分发版本。

## 删除 ManagedCertificate

### 前置条件

- `server_ids` 为空（零 Server 分配）。
- 无 Pending / Running operation（Issue / Renew / Revoke 均算）。

### 级联清理（仅本地）

- 删除 `managed_certificates` 文档（含 desired、active、previous PEM）。
- 删除该证书全部 `certificate_operations`。
- 删除证书通知节流状态（若已实现）。
- **保留** `audit_logs`。
- **不删除** 任何 `certificate_access_tokens`（token 绑定 Server，与证书生命周期独立）。
- **不调用** ACME Revoke。

### Issuer / DNS 账户删除

- 若 desired、active 或 previous 仍引用 `CertificateIssuer` 或 `DNSProviderAccount` → `failed_precondition`。
- 无引用时仅删除本地账户/key 记录；**不**停用远端 ACME account。

## Server 分发

`ServerCertificateService` 对以下 active 版本拒绝 bundle（`failed_precondition`，不泄露 CA 细节）：

- `revoke_pending`
- 已吊销（`revoked_at` 已设置）
- Revoke operation 失败但版本仍 `revoke_pending`

过期但未吊销 active 仍可下载并标记 `Expired`。

## 管理 UI

- **吊销**：独立确认，可选 RFC 5280 reason，说明立即停分发与不自动回滚。
- **激活过期 previous**：独立确认，说明灾难恢复语义。
- **删除证书**：独立确认，说明仅本地删除、不隐式吊销、audit 保留。

## 相关文档

- [托管证书](./certificate-management-managed-certificates.md)
- [Operation Lease 与恢复](./certificate-management-operation-lease.md)
- [Server 分发](./certificate-management-server-distribution.md)
