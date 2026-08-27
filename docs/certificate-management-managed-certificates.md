# 证书管理 — 托管证书（ManagedCertificate）

本文档说明 Admin 管理的 `ManagedCertificate` desired config、与 Monitor 探测快照及签发版本的区别，以及 #17 实现的创建/更新与 Pending Issue operation 行为。

## 领域概念对比

| 概念 | 含义 | 存储位置 | 是否含私钥 |
|------|------|----------|------------|
| **ManagedCertificate** | Admin 配置的期望证书：域名、Issuer、DNS 账户、密钥类型、续期策略、Server 分配与通知组；并作为 active/previous **两个** CertificateVersion 的容器 | `managed_certificates` collection | 否（desired config 本身不含 PEM） |
| **CertificateVersion** | 一次 **成功签发** 得到的不可变 bundle（fullchain + 私钥）及签发参数快照；同一 ManagedCertificate 最多保留 active 与 previous 两个完整版本 | 嵌套在 `managed_certificates` 文档的 `active_version` / `previous_version` | 是（#18 起写入） |
| **CertificateInfo** | **Monitor TLS 探测** 读到的对端证书公开元数据快照（subject、issuer、到期日等） | Monitor 文档的 `certificate` 字段 | 否 |

要点：

- Monitor 上的 `CertificateInfo` 只反映 **线上已部署** 证书的观测结果，与 neo-line 是否托管该证书无关。
- ManagedCertificate 的 desired config 变更 **不会** 立即向 CA 下单；需显式 Issue（或创建时自动首次 Issue）才会产生 `CertificateOperation`。
- Server 分发接口（#19 起）只暴露 **active** CertificateVersion，不返回尚未发布的 desired config。

## API 与 UI

- Connect：`ManagedCertificateService`（List / Create / Get / Update），挂载于 `/api/grpc`。
- Web：`证书 → 托管证书`（列表、创建表单、详情：desired config + Missing 有效性 + Pending operation）。
- 所有读写接口 **不返回** DNS Token、EAB、ACME account key 或证书私钥。

## MongoDB：`managed_certificates`

| 字段 | 说明 |
|------|------|
| `id` | 自动生成，前缀 `mcert_` |
| `name` | 全局唯一显示名 |
| `domains` | 有序域名列表，**第一个为主域名**，其余为 SAN；最多 **100** 个 |
| `certificate_issuer_id` | 须引用 **Ready** 的 CertificateIssuer |
| `dns_provider_account_id` | 须存在的 DNSProviderAccount |
| `key_type` | `ec_p256`（默认）或 `rsa_2048` |
| `auto_renew_enabled` | 默认 **true** |
| `renew_before_days` | 默认 **30** |
| `notify_group_ids` | 可选，引用 NotifyGroup |
| `server_ids` | 可选，可为 **空**（允许先签发再分配） |
| `active_version` / `previous_version` | #18 起填充 |

域名规范化（创建/更新时）：

1. trim 空白  
2. 转小写  
3. IDNA 转 ASCII（如 `München.de` → `xn--mnchen-3ya.de`）  
4. 移除尾部 `.`  
5. DNS / 泛域名语法校验（仅最左标签允许 `*.example.com`）  
6. 去重并保持顺序  

## MongoDB：`certificate_operations`

| 字段 | 说明 |
|------|------|
| `id` | 前缀 `cop_` |
| `managed_certificate_id` | 所属证书 |
| `type` | `Issue` / `Renew` / `Revoke` |
| `status` | `Pending` / `Running` / `Succeeded` / `Failed` |
| `attempt_count` | 尝试次数（#18 reconciler 递增） |
| `config_snapshot` | 创建时冻结的 domains、issuer、dns、key_type |
| `error_summary` / `warning` | 脱敏错误与告警摘要 |
| `started_at` / `finished_at` / `next_attempt_at` | 时间戳 |

## 行为（#17）

### 创建

1. 校验 Ready Issuer、存在的 DNS 账户、NotifyGroup / Server ID（Server 可为空）。  
2. 写入 desired config。  
3. **自动** 创建一条 `Issue` + `Pending` 的 CertificateOperation（含 config 快照）。  

### 有效性（尚无 active version）

- `active_validity` = **Missing**  
- `bundle_available` = **false**  

### Operation 进行中（Pending 或 Running）

- 修改 **domains、issuer、DNS 账户、key_type** → Connect `failed_precondition`。  
- 仍可修改 **name、server_ids、notify_group_ids**（详情页可独立保存 Server 分配，见 [访问 Token 与 Server 分配](./certificate-management-access-tokens.md)）。  

### 幂等 Issue 提交

- 若同一 ManagedCertificate 已有 **Pending 或 Running** 的 Issue operation，再次提交返回 **同一条** operation，不创建第二条（`SubmitIssueOperation` 内部逻辑；显式 Issue RPC 在后续 ticket 暴露）。  

## 默认值摘要

| 项 | 默认 |
|----|------|
| 密钥类型 | EC P-256 |
| 自动续期 | 开启 |
| renew_before_days | 30 |
| Server 分配 | 空（可选） |

## 相关文档

- [DNS 账户](./certificate-management-dns-accounts.md)  
- [ACME Issuer](./certificate-management-issuers.md)  
- [功能说明 — 证书管理](./features.md)  
