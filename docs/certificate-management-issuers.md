# ACME Certificate Issuer 管理

本文档说明 neo-line 首版 **CertificateIssuer**（ACME 签发账户）的配置字段、内置 CA preset、EAB 要求、Terms of Service 同意语义，以及自定义 ACME Directory 的信任限制。

CertificateIssuer 数据保存在 MongoDB `certificate_issuers` collection。ACME account private key、EAB KID/HMAC 以明文字段存储，但**永远不会**通过 Connect 读取接口或审计日志返回。

## 生命周期

| 状态 | 含义 |
|------|------|
| `Pending` | 已持久化配置，后台正在向 ACME CA 注册账户 |
| `Ready` | 注册成功，可被 ManagedCertificate 引用并用于签发 |
| `Failed` | 注册失败，`registration_error` 提供脱敏摘要；可修正身份字段后重试 |

创建 Issuer 后 Connect 请求立即返回 `Pending`，注册在 goroutine 中异步完成。只有 `Ready` Issuer 可用于后续证书签发（#17 及以后）。

删除 Issuer **仅删除本地 MongoDB 文档**，不会调用 ACME 远端 account deactivation。

## 内置 CA Preset

Admin UI 与 Connect API 通过 `ca_type` 选择 preset；系统会解析为固定 Directory URL（`custom` 除外）。

| `ca_type` | Directory URL | Staging / 不受公共信任 | 需要 EAB |
|-----------|---------------|------------------------|----------|
| `lets_encrypt_production` | `https://acme-v02.api.letsencrypt.org/directory` | 否 | 否 |
| `lets_encrypt_staging` | `https://acme-staging-v02.api.letsencrypt.org/directory` | **是** | 否 |
| `zerossl` | `https://acme.zerossl.com/v2/DV90` | 否 | **是** |
| `google_public_ca` | `https://dv.acme-v02.api.pki.goog/directory` | 否 | **是** |
| `custom` | Admin 提供的 HTTPS URL | 否（TLS 由系统根信任） | 否 |

### Let's Encrypt Staging

Let's Encrypt **staging** 环境签发的证书**不受公共客户端信任**。UI 与 API 响应通过 `staging_untrusted=true` 明确标识，适用于集成测试，不应部署到生产 TLS 终端。

### ZeroSSL 与 Google Public CA

这两家免费 ACME CA 要求 **External Account Binding（EAB）**。创建 Issuer 时必须同时提供：

- `eab_kid`：EAB Key ID
- `eab_hmac`：Base64url 编码的 HMAC 密钥

EAB 凭据保存在 MongoDB，读取接口仅返回 `eab_configured=true`。

## Terms of Service 同意

注册 ACME 账户前，Admin 必须：

1. 通过 `GetCertificateIssuerDirectoryPreview`（或 UI 自动加载）获取 Directory 元数据中的 `terms_of_service_url`
2. 在创建请求中设置 `terms_of_service_agreed=true`

未同意时创建会被拒绝。同意时系统持久化：

- `terms_of_service_url`：同意时的 ToS 链接
- `terms_of_service_agreed_at`：同意时间戳

## 自定义 ACME Directory

当 `ca_type=custom` 时，Admin 提供 `custom_directory_url`。约束：

- **必须使用 HTTPS**
- TLS 证书链必须由**操作系统根证书库**验证
- **不支持**上传私有 Root CA 或接入使用私有 TLS 根的 step-ca 等部署

首版不提供“自定义信任锚”配置；若 Directory 使用自签名或私有 CA 签发 HTTPS，Directory 预览与注册均会失败。

## 身份字段与更新规则

创建时可提供 `account_key_pem`（EC/RSA PEM）；留空则自动生成 EC P-256 私钥。

| 状态 | 可修改字段 |
|------|------------|
| `Pending` | 不可 Update（等待注册完成） |
| `Failed` | 显示名称、邮箱、`ca_type`、Directory、account key、EAB；Update 后自动重新进入 `Pending` 并注册 |
| `Ready` | **仅**显示名称；CA、Directory、邮箱、EAB、account key 不可变 |

失败后可调用 `RetryCertificateIssuerRegistration` 在不改字段的情况下重新发起注册（前提是 Directory 等配置仍然有效）。

## Connect 服务

`CertificateIssuerService` 全部 RPC 要求 **admin role**。Secret 字段仅出现在 Create/Update 请求体中，响应使用 `account_key_configured` / `eab_configured` 布尔值。

主要 RPC：

- `ListCertificateIssuers` / `GetCertificateIssuer`
- `GetCertificateIssuerDirectoryPreview` — 创建前加载 ToS 与 `requires_eab`
- `CreateCertificateIssuer` — 需 `terms_of_service_agreed=true`
- `UpdateCertificateIssuer`
- `DeleteCertificateIssuer`
- `RetryCertificateIssuerRegistration`

## MongoDB 文档示例

```yaml
id: iss_0195a1b2-c3d4-7890-abcd-ef1234567890
name: le-prod
ca_type: lets_encrypt_production
directory_url: https://acme-v02.api.letsencrypt.org/directory
email: admin@example.com
registration_status: Ready
staging_untrusted: false
terms_of_service_url: https://letsencrypt.org/documents/LE-SA-v1.4-April-3-2024.pdf
terms_of_service_agreed_at: 2026-03-01T12:00:00Z
account_key_pem: "<secret — 不在 API 中返回>"
created_at: 2026-03-01T12:00:00Z
updated_at: 2026-03-01T12:00:05Z
```

## 相关文档

- [Cloudflare DNS 账户](./certificate-management-dns-accounts.md)
- [功能说明](./features.md)
