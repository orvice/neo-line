# 证书管理 — 运维、默认值与安全

本文档汇总 ACME 证书管理的 **运行默认值**、Secret 存储假设、外部依赖与 Prometheus 指标（#27）。

## 运行默认值

| 项 | 默认值 | 说明 |
| --- | --- | --- |
| 密钥类型 | **EC P-256**（`ec_p256`） | 可选 RSA-2048 |
| `renew_before_days` | **30** 天 | 每张 ManagedCertificate |
| 有效续期窗口 | `min(renew_before_days, 证书总有效期 / 3)` | 短有效期证书避免签发后立即续期循环 |
| Certificate reconciler 扫描 | **每小时** | 与 Monitor scheduler（5s reconcile）独立 |
| Operation 失败退避 | **15 分钟** 起，指数至 **12 小时** 封顶 + jitter | 成功清零连续失败 |
| Operation 总超时 | **72 小时** | 自创建起算；持久化 `deadline_at`；超时后终态 Failed，不再自动重试 |
| DNS 传播超时 | **120** 秒 | `propagation_timeout_seconds`，范围 30–900 |
| Server 分发限流 | **120 次 / 分钟 / token** | List + Get 合计；Redis 键 `neo-line:cert-dist:{token_id}` |
| 失败通知提醒 | 首次立即；持续失败每 **24 小时** | 恢复通知不受失败节流影响 |
| 临期提醒 | active 剩余 **7 天** 且仍未续期 | 与 Expired 通知各一次（持久化节流） |

## MongoDB Collections 概览

| Collection | 主要 Secret 字段 | API 是否返回明文 |
| --- | --- | --- |
| `dns_provider_accounts` | `api_token` | 否（`token_configured`） |
| `certificate_issuers` | `account_key_pem`, `eab_hmac_key` | 否 |
| `managed_certificates` | `active_version` / `previous_version` 内 PEM | 否（Admin bundle 为一次性下载） |
| `certificate_operations` | 无 PEM | 否（不含 lease/TXT 细节） |
| `certificate_access_tokens` | `token_hash` 仅 hash | 否（创建时一次性 secret） |

**MongoDB 与备份包含上述全部 Secret 明文。** 部署方必须限制数据库与备份访问；**这些 Secret 不进入 S3 archive**（S3 仍仅归档 `monitor_results`）。

## 外部依赖

| 依赖 | 用途 | 故障行为 |
| --- | --- | --- |
| MongoDB | 全部证书配置、operation、lease、token | fail-closed |
| Redis | Server 分发限流 | **fail-open**（记录错误）；token/授权仍 fail-closed |
| ACME CA | 签发/续期/吊销 | operation 重试/终态失败 + 通知 |
| Cloudflare API | DNS-01 | Token 验证失败不保存；签发失败脱敏错误 |

**HTTPS：** 面向 Server 与 Admin 的外部入口必须使用 HTTPS。neo-line 可运行在 TLS 终止反向代理之后（应用内 HTTP 可接受）。

**Staging：** Let's Encrypt staging 等 Issuer 标记 `staging_untrusted`；bundle 可下载但不受公共信任。

**Cache-Control：** Admin 与 Server 的 bundle 响应均为 `no-store`；正文不得写入 audit 或运行日志。

## CA Preset 与 EAB

| Preset | Directory | EAB | 备注 |
| --- | --- | --- | --- |
| Let's Encrypt 生产 | 内置 | 不需要 | 默认生产 CA |
| Let's Encrypt Staging | 内置 | 不需要 | 集成测试；证书不受信任 |
| ZeroSSL | 内置 | **需要** Admin 提供 | 创建 Issuer 时填写 |
| Google Public CA | 内置 | **需要** Admin 提供 | 创建 Issuer 时填写 |
| 自定义 HTTPS Directory | Admin 填写 URL | 视 CA 而定 | 系统根信任；**不支持** 私有 Root CA |

创建 Issuer 前必须显式同意 Directory 元数据中的 **Terms of Service**；持久化 ToS URL 与 agreed-at。

## Prometheus 指标（证书）

低基数 label；**不使用** certificate ID 作为无界 label。

| 指标 | 类型 | Labels | 说明 |
| --- | --- | --- | --- |
| `neoline_managed_cert_validity` | Gauge | `validity` | 各 validity 状态的证书数量 |
| `neoline_cert_operation_total` | Counter | `op_type`, `result` | issue/renew/revoke × succeeded/failed/retry_scheduled |
| `neoline_cert_renew_failures_total` | Counter | — | Renew 失败尝试累计 |
| `neoline_server_cert_list_total` | Counter | — | 成功 ListCertificates |
| `neoline_server_cert_bundle_download_total` | Counter | — | 成功 GetCertificateBundle |

Monitor 探测证书指标 `neoline_certificate_days_remaining` 仍按 `monitor_id` / `server_id` 标注，与 ManagedCertificate 无关。

## 相关文档

- [功能说明 — 证书管理](./features.md)
- [Out of Scope](./certificate-management-out-of-scope.md)
- [CONTEXT.md](../CONTEXT.md) 领域 glossary
- [docs/adr/](./adr/)
