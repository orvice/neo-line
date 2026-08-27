# 证书管理 — Server 证书分发

本文档说明 Server 使用 `CertificateAccessToken` 调用 `ServerCertificateService` 发现获授权证书并下载 active bundle（#20）。

## 鉴权

- **仅** 接受 `Authorization: Bearer nlct_<secret>`；Admin session、`mcp_` token 及其他凭据一律返回 `unauthenticated`。
- Token 通过 MongoDB **SHA-256 hash** 查表并推导 **Server ID**；无效、过期或已删除 token 返回 `unauthenticated`（不区分原因，防枚举）。
- 鉴权与 Server 分配 **不缓存**；吊销 token 或取消分配后立即生效。
- `last_used_at` 在 token 校验成功时 **best-effort** 更新。

## Connect API

挂载于 `/api/grpc`，服务名 `ServerCertificateService`：

| RPC | 说明 |
|-----|------|
| `ListCertificates` | 列出当前 token 所属 Server 获授权的全部 ManagedCertificate |
| `GetCertificateBundle` | 按 `managed_certificate_id` 原子返回 active 的 fullchain + private key 与 metadata |

### ListCertificates 语义

- 仅返回 `server_ids` 包含本 Server 的证书。
- **有 active version**：返回 active 的版本 ID、`available`、validity、域名（active 快照）、key type、leaf SHA-256 fingerprint、not-before/not-after、staging 标识；**不暴露** 尚未发布的 desired config。
- **无 active**：以配置 **名称** 与 **domains** 标识，`available=false`，validity 为 `Missing`；若最近 operation 失败则附带 **安全 error_summary**（不含 Cloudflare/EAB/order URL 等内部细节）。

### GetCertificateBundle 语义

- 请求体字段：`managed_certificate_id`。
- 同一响应包含 `fullchain_pem`、`private_key_pem`（bytes）及 certificate/version ID、active 域名、key type、fingerprint、not-before/not-after、validity、staging 标识。
- **过期但未吊销** 的 active 仍可下载，validity 为 `Expired`。
- **已吊销** 或 **revoke_pending** 的 active 不可下载。
- 证书不存在 **与** 未授权：统一 `not_found`。
- 已授权但无可分发 active（Missing / 吊销阻止）：`failed_precondition`。
- 响应 Header：`Cache-Control: no-store`；正文不得写入运行日志或 audit PEM。

### 限流

- Redis 键：`neo-line:cert-dist:{token_id}`，滑动窗口 **1 分钟**、上限 **120 次**（List + Get 合计）。
- 超限：`resource_exhausted`（HTTP 429）。
- Redis 故障：**fail-open**（记录错误并放行）；MongoDB token 与授权校验仍 **fail-closed**。

### 审计与 metrics

| 事件 | audit_logs | metrics / 运行日志 |
|------|------------|-------------------|
| 成功 `ListCertificates` | 否 | `neoline_server_cert_list_total` + 结构化 info |
| 成功 `GetCertificateBundle` | 是（`source=server_cert`, `action=download`） | info（不含 PEM） |
| 鉴权失败（无效 token 等） | 是（`action=auth`） | info |

审计字段：`token_prefix`、`server_id`、`managed_certificate_id`、`version_id`（下载时）、结果；**绝不** 记录 token 明文、PEM 或 hash。

## 调用示例

### Protobuf 客户端（Go 概念示例）

```go
req := connect.NewRequest(&neolinev1.ListCertificatesRequest{})
req.Header().Set("Authorization", "Bearer "+os.Getenv("NLCT_TOKEN"))
resp, err := client.ListCertificates(ctx, req)
```

```go
req := connect.NewRequest(&neolinev1.ServerCertificateServiceGetCertificateBundleRequest{
    ManagedCertificateId: "mcert_abc123",
})
req.Header().Set("Authorization", "Bearer "+os.Getenv("NLCT_TOKEN"))
resp, err := client.GetCertificateBundle(ctx, req)
// resp.Msg.Bundle.FullchainPem / PrivateKeyPem 为 []byte
```

### Connect JSON（curl）

List：

```bash
curl -sS -X POST "https://neo-line.example/api/grpc/neoline.v1.ServerCertificateService/ListCertificates" \
  -H "Authorization: Bearer nlct_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" \
  -H "Content-Type: application/json" \
  -d '{}'
```

Get bundle（Connect JSON 对 `bytes` 使用 **标准 Base64**）：

```bash
curl -sS -X POST "https://neo-line.example/api/grpc/neoline.v1.ServerCertificateService/GetCertificateBundle" \
  -H "Authorization: Bearer nlct_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" \
  -H "Content-Type: application/json" \
  -d '{"managed_certificate_id":"mcert_abc123"}'
```

示例响应片段：

```json
{
  "bundle": {
    "managedCertificateId": "mcert_abc123",
    "versionId": "cver_xyz",
    "domains": ["example.com", "www.example.com"],
    "validity": "CERTIFICATE_VALIDITY_VALID",
    "stagingUntrusted": false,
    "fullchainPem": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t...",
    "privateKeyPem": "LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0t..."
  }
}
```

客户端应对 Base64 解码得到 PEM 文本。首版 **不提供** REST 文件下载或官方 CLI；Server 自行落盘与 reload。

## 相关文档

- [访问 Token 与 Server 分配](./certificate-management-access-tokens.md)
- [托管证书](./certificate-management-managed-certificates.md)
- [功能说明 — 证书管理](./features.md)
