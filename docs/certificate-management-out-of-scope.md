# 证书管理 — 首版范围外（Out of Scope）

本文档明确 **不在** neo-line ACME 证书管理首版（#13）范围内的能力，避免与已实现行为混淆。

## Challenge 与 DNS

- HTTP-01 与 TLS-ALPN-01 challenge。
- Cloudflare **之外** 的 DNS Provider（阿里云 DNS、腾讯云 DNSPod、Route 53 等）。
- 同一 ManagedCertificate 按域名选择多个 DNSProviderAccount。
- IP 地址证书。

## 证书导入与 CA 策略

- 上传或导入已有 `fullchain.pem` / `private_key.pem`。
- 上传自定义 ACME directory 的 **私有 Root CA**（step-ca 等私有 TLS 根）。
- 多 CA 自动 failover。
- ACME preferred chain 选择；fullchain 附带 root。

## Secret 存储与归档

- KMS、Vault、envelope encryption、MongoDB 字段级加密或加密私钥 PEM（首版见 [ADR-0001](./adr/0001-plaintext-secrets-in-mongodb.md)）。
- 将证书、私钥、ACME account key、EAB 或 DNS secret **归档到 S3**（S3 仅 `monitor_results`）。

## Server 侧自动化

- Server 常驻 **agent**、参考 **CLI**、自动写文件、原子替换部署文件。
- 自动 reload nginx / HAProxy 或执行任意远程命令。
- Server push、streaming watch、webhook 通知更新、部署确认或部署状态回执。
- 原始 **REST 文件下载**；首版仅 Connect RPC。
- Server 自主提交域名或触发任意证书签发。

## Token 与 MCP

- CertificateAccessToken 细粒度 scopes 或非证书能力。
- 证书管理 **MCP tools**（含元数据管理与私钥下载）。

## 与 Monitor / 状态页的耦合

- ManagedCertificate 与现有 **TLS Monitor 自动创建、关联或部署验证**。
- 将证书 validity 映射为 `Healthy` / `Warning` / `Critical` / `Down` / `Unknown`。
- 在 **公开状态页** 展示 ManagedCertificate 配置、域名或 CA 关系。

## 版本与 Operation 管理

- 永久保留所有 CertificateVersion 或全部历史私钥。
- **取消** 运行中的 ACME operation。
- 删除 Issuer 时远端 ACME account deactivation。

## 已实现但易误解的边界

以下能力 **已实现**，不属于上列 out-of-scope：

| 能力 | 说明 |
| --- | --- |
| Connect Server 分发 | `ServerCertificateService` + `nlct_` token |
| Admin Web UI | 「证书」工作区与 Dashboard 摘要 |
| 双版本回滚 | active/previous 单文档模型 |
| NotifyGroup 证书事件 | 与 Monitor 告警通道复用、payload 独立 |
| 过期 active 仍可 Server 下载 | 标记 `Expired`，灾难恢复场景 |

## 参考

- GitHub #13 Out of Scope
- [功能说明 — 证书管理](./features.md)
