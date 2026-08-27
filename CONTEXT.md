# neo-line 领域术语

本文档记录 neo-line 核心领域词汇，供代码、文档与 API 命名保持一致。

## CertificateInfo

**CertificateInfo** 是 **Monitor TLS 探测** 读到的对端证书公开元数据快照（subject、issuer、DNS names、serial、`not_before`、`not_after`、剩余天数等）。它嵌在 `monitors` 文档的 `certificate` 字段中，由调度器在 `url` / `tls_port` 探测成功后更新。

要点：

- 只反映 **线上已部署** 证书的观测结果，不含私钥或 desired config。
- 与 **ManagedCertificate** 生命周期完全独立；签发成功不等于 Monitor 已观测到部署成功。
- 健康语义使用 Monitor 的 `Healthy` / `Warning` / `Critical` / `Down` / `Unknown`，**不是** ManagedCertificate 的 `Missing` / `Valid` / `RenewalDue` / `Expired` / `Revoked`。

## CertificateIssuer

**CertificateIssuer** 是命名 ACME 签发账户配置，包含 CA preset 或自定义 Directory URL、联系邮箱、Terms of Service 同意记录、注册状态（Pending / Ready / Failed）、ACME account key 与可选 EAB。Admin 通过 `CertificateIssuerService` 管理；仅 **Ready** Issuer 可用于 ManagedCertificate 签发。

MongoDB collection：`certificate_issuers`。Secret（account key、EAB）以明文存储但读取接口与审计日志永不返回。

## CertificateOperation

**CertificateOperation** 是一次 ACME 后台任务（Issue / Renew / Revoke）的持久化记录，包含类型、状态、attempt 次数、配置快照、lease 信息、错误摘要与时间戳。独立存储于 `certificate_operations` collection；同一 ManagedCertificate 同时最多一个运行中 Issue/Renew/Revoke operation。

要点：

- 运行中 operation 冻结签发字段快照；终态失败可手动重试创建 **新** operation。
- 首版不提供 Cancel；进程中断后 lease 到期由其他副本接管并增加 attempt。
- Admin API 不返回 lease 字段与 `pending_txt_records`。

## CertificateVersion

**CertificateVersion** 是一次 **成功 ACME 签发** 得到的不可变 bundle：fullchain PEM、PKCS#8 私钥、签发参数快照、指纹、有效期与 staging 标识。同一 ManagedCertificate 在 MongoDB **单文档** 内最多保留 **active** 与 **previous** 两个完整版本；第三个成功版本激活时丢弃更老版本的本地 PEM（不向 CA 隐式吊销）。

Server 分发与 Admin bundle 下载均针对 **active**（或 Admin 指定的 previous slot），不暴露尚未发布的 desired config。

## CertificateAccessToken

**CertificateAccessToken** 是绑定单台 **Server** 的长期凭据，供 `ServerCertificateService` 列出获授权证书并下载 active bundle。Secret 格式为 `nlct_` 前缀加高熵随机值；MongoDB 仅存 SHA-256 hash、显示用 prefix 与元数据；明文 **仅在创建响应中返回一次**。

要点：

- 与 Admin session、MCP token **完全隔离**；鉴权路径独立。
- 名称在同一 Server 内唯一；可选 `expires_at`；吊销/删除后立即生效（不缓存）。
- Redis 按 token 限流 Server 分发（120 次/分钟）；Redis 故障 fail-open。

MongoDB collection：`certificate_access_tokens`。

## DNSProviderAccount

**DNSProviderAccount** 是可复用的 DNS-01 提供商凭据配置。Admin 在证书管理区域创建并维护 DNSProviderAccount，供后续 ManagedCertificate 引用同一组 Cloudflare API Token，以完成 ACME DNS-01 挑战。

要点：

- 首版仅支持 `provider: cloudflare`。
- 凭据保存在 MongoDB `dns_provider_accounts` collection；MongoDB 是权威来源。
- Cloudflare API Token 在创建或轮换前必须通过 Cloudflare verify API 验证；验证失败时不保存新 Token，轮换失败时旧 Token 继续有效。
- 读取接口只返回 `token_configured` 布尔值，不返回 Token 明文；更新时空 `api_token` 表示保留现有 Secret。
- `propagation_timeout_seconds` 控制 DNS 传播等待时间，默认 **120** 秒，有效范围 **30–900** 秒。

与 Monitor 探测得到的 **CertificateInfo**（证书快照）不同：DNSProviderAccount 管理的是签发所需的 DNS 凭据，而不是观测到的线上证书元数据。

## ManagedCertificate

**ManagedCertificate** 是 Admin 配置的 **期望证书**（desired config）：有序域名、CertificateIssuer、DNSProviderAccount、密钥类型、自动续期策略、NotifyGroup 与 Server 分配。它在同一 MongoDB 文档中原子维护 **active** 与 **previous** 两个 **CertificateVersion**，并关联 CertificateOperation 与通知节流状态。

要点：

- `name` 全局唯一；`domains` 第一个为主域名，其余为 SAN（≤100）。
- 修改签发字段只更新 desired config；必须显式 Issue（或创建时自动首次 Issue）才向 CA 下单。
- `active_validity`（Missing / Valid / RenewalDue / Expired / Revoked）与 operation 状态、`auto_renew_enabled` 正交。
- 私钥与 PEM 存在嵌套版本字段中；API 响应脱敏。

MongoDB collection：`managed_certificates`（active/previous **不** 拆为独立 collection）。

## 相关架构决策

见 [docs/adr/](./docs/adr/)：

- [ADR-0001：MongoDB 明文存储证书与 DNS Secret](./docs/adr/0001-plaintext-secrets-in-mongodb.md)
- [ADR-0002：active/previous 单文档双版本模型](./docs/adr/0002-active-previous-single-document.md)
- [ADR-0003：独立 Server 分发 Token](./docs/adr/0003-server-certificate-distribution-token.md)
- [ADR-0004：独立 Certificate Reconciler](./docs/adr/0004-certificate-reconciler.md)
