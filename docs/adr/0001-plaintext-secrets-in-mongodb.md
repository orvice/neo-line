# ADR-0001：MongoDB 明文存储证书与 DNS Secret

## 状态

已接受（2026-05）

## 背景

neo-line 首版 ACME 证书管理需要在 MongoDB 中持久化 fullchain、证书私钥、ACME account key、EAB 与 Cloudflare API Token，以支撑签发、续期、吊销与 Server 分发。KMS、Vault 或字段级加密尚未纳入产品范围。

## 决策

- 上述 Secret **以明文字段** 存入 MongoDB 相应 collection（`managed_certificates` 嵌套版本、`certificate_issuers`、`dns_provider_accounts`）。
- 所有 Admin / Server **读取接口**、审计日志与结构化运行日志 **永不返回** 这些 Secret 明文。
- 现有 S3 archiver **继续只归档** `monitor_results`；**绝不** 接收证书 PEM、account key、EAB 或 DNS 凭据。
- MongoDB 备份与副本因此 **同样包含** 这些 Secret；部署方必须按此假设保护数据库与备份访问路径。

## 后果

- 运维必须将 MongoDB 与备份视为 **高敏感** 存储，与对象存储归档区分权限。
- 未来若引入 envelope encryption 或外部 Secret 存储，需迁移现有字段并更新读取路径。
- 文档与 onboarding 必须明确该取舍，避免误以为 S3 或日志中存在私钥副本。

## 参考

- GitHub #13 Further Notes
- [证书管理 — 运维与安全](./../certificate-management-operations.md)
