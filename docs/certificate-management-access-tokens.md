# 证书管理 — Server 分配与 CertificateAccessToken

本文档说明 ManagedCertificate 与 Server 的分配关系，以及绑定单台 Server 的证书分发访问 token 生命周期（#19）。

## Server 分配

- Admin 在 **托管证书详情** 页将零到多台 **现有** Server 加入或移出 `server_ids`。
- 变更通过 `ManagedCertificateService.UpdateManagedCertificate` 写入 MongoDB，**立即生效**，不缓存分配关系。
- **不会** 因分配或取消分配触发新的 Issue / Renew operation；多台 Server 共享同一 active bundle。
- 允许在尚未分配任何 Server 时先完成首次签发（#17 行为不变）。

### 删除 Server 的级联

删除 Server（`ServerService.DeleteServer`）时：

1. 级联删除该 Server 下的全部 **CertificateAccessToken**；
2. 从所有 ManagedCertificate 的 `server_ids` 中 **移除** 该 Server ID；
3. **不删除** ManagedCertificate 本身及其证书版本。

## CertificateAccessToken

每台 Server 可持有 **多个** token，用于 `ServerCertificateService` 的 Bearer 鉴权（见 [Server 分发](./certificate-management-server-distribution.md)）。

### Connect API

`CertificateAccessTokenService`（挂载于 `/api/grpc`），**全部方法要求 admin role**：

| RPC | 说明 |
|-----|------|
| `ListCertificateAccessTokens` | 按 `server_id` 列出 token（含已过期记录） |
| `CreateCertificateAccessToken` | 创建 token；响应含 **一次性** `secret` |
| `DeleteCertificateAccessToken` | 按 `server_id` + `token_id` 吊销 |

### Secret 格式与存储

- 明文格式：`nlct_` + 64 位十六进制随机串（32 字节熵）。
- 明文 **仅在** `CreateCertificateAccessToken` 响应与管理 UI 创建流程中展示 **一次**。
- MongoDB `certificate_access_tokens` 只保存：
  - `token_hash`：SHA-256(明文)，十六进制编码
  - `prefix`：明文前 13 字符（`nlct_` + 8 位），用于列表与审计
  - `server_id`、`name`、可选 `expires_at`、`created_at`、可选 `last_used_at`
- **读取、审计与运行日志 never 包含 token 明文或 hash。**

### 名称与过期

| 规则 | 说明 |
|------|------|
| 名称唯一性 | 同一 `server_id` 内 `name` 唯一；不同 Server 可使用相同显示名 |
| 过期 | `expires_at` 可选，默认 **不过期** |
| 过期后 | 记录仍保留在列表并标记 `expired=true`，直到 Admin 删除 |
| 鉴权 | 过期 token 不可通过校验（#20 分发时 fail-closed） |
| 吊销 | 删除后立即失权；**不缓存** token 或分配，保证即时生效 |

### 索引

| 索引 | 字段 |
|------|------|
| `uniq_token_hash` | `token_hash` 唯一 |
| `uniq_server_name` | `(server_id, name)` 唯一 |
| `server_created_at_desc` | 列表排序 |

## 管理 UI

- **托管证书详情**：独立「Server 分配」卡片，多选保存，不打开完整编辑表单。
- **Server 详情 → 证书授权**：
  - 展示已分配的 ManagedCertificate 列表（链至证书详情）；
  - CertificateAccessToken 管理：创建、一次性 secret 对话框、过期状态、吊销确认。

## 相关文档

- [托管证书](./certificate-management-managed-certificates.md)
- [功能说明 — 证书管理](./features.md)
