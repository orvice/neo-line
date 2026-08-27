# neo-line 领域术语

本文档记录 neo-line 核心领域词汇，供代码、文档与 API 命名保持一致。

## DNSProviderAccount

**DNSProviderAccount** 是可复用的 DNS-01 提供商凭据配置。Admin 在证书管理区域创建并维护 DNSProviderAccount，供后续 ManagedCertificate 引用同一组 Cloudflare API Token，以完成 ACME DNS-01 挑战。

要点：

- 首版仅支持 `provider: cloudflare`。
- 凭据保存在 MongoDB `dns_provider_accounts` collection；MongoDB 是权威来源。
- Cloudflare API Token 在创建或轮换前必须通过 Cloudflare verify API 验证；验证失败时不保存新 Token，轮换失败时旧 Token 继续有效。
- 读取接口只返回 `token_configured` 布尔值，不返回 Token 明文；更新时空 `api_token` 表示保留现有 Secret。
- `propagation_timeout_seconds` 控制 DNS 传播等待时间，默认 **120** 秒，有效范围 **30–900** 秒。

与 Monitor 探测得到的 **CertificateInfo**（证书快照）不同：DNSProviderAccount 管理的是签发所需的 DNS 凭据，而不是观测到的线上证书元数据。
