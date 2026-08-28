# 证书管理 — DNS 账户

本文档说明 neo-line 首版 ACME 证书管理中的 **DNSProviderAccount**（Cloudflare DNS 账户）配置与行为。

## 概述

DNSProviderAccount 存储可复用的 Cloudflare API Token，供后续 ManagedCertificate 通过 DNS-01 完成域名验证。Admin 通过 Connect API `DNSProviderAccountService` 或 Web 控制台「证书 → DNS 账户」管理。

首版范围：

- 仅支持 Cloudflare（`provider: cloudflare`）。
- 提供创建、列表、查看、更新（含 Token 轮换）、删除。
- **所有 RPC 均要求 admin 角色**。

## Cloudflare API Token 权限

保存 Token 前，系统会调用 Cloudflare `GET /user/tokens/verify` 验证有效性。Token 必须对目标 Zone 具备以下权限：

| 权限 | 用途 |
| --- | --- |
| **Zone:Read** | 解析 challenge 所在 Zone |
| **DNS:Edit** | 创建与清理 `_acme-challenge` TXT 记录 |

建议使用「编辑区域 DNS」模板或按 Zone 限定权限的自定义 Token。验证失败时 API 返回错误摘要（不含 Token 明文），且不会写入 MongoDB。

## CNAME 与通配符解析

neo-line 默认设置 `LEGO_DISABLE_CNAME_SUPPORT=true`，在原始 `_acme-challenge.<domain>` 名称创建精确 TXT 记录，不跟随查询结果中的 CNAME。这样可以正确处理以下常见业务解析：

```dns
*.example.com. 300 IN CNAME app.example.net.
```

在尚无精确 challenge 记录时，DNS 通配符也会为 `_acme-challenge.host.example.com` 合成 CNAME。若错误地把它当成 ACME 委派，provider 会尝试修改 `example.net` 的 Zone。精确 TXT 创建后会覆盖该名称上的通配符匹配，ACME 校验仍可正常读取 TXT。

如果部署明确使用 `_acme-challenge...` 的**显式 CNAME 委派**，可在启动 neo-line 前设置：

```bash
LEGO_DISABLE_CNAME_SUPPORT=false
```

该开关对整个进程生效，修改后须重启服务。启用 CNAME 跟随后，Cloudflare Token 必须覆盖委派目标所在 Zone，而不只是证书域名的源 Zone。

## DNS 传播超时

字段 `propagation_timeout_seconds` 控制 ACME DNS-01 挑战等待 DNS 传播的最长时间（由 certmanager provider adapter 在后续签发流程中使用）。

| 项 | 值 |
| --- | --- |
| 默认值 | **120** 秒 |
| 最小值 | 30 秒 |
| 最大值 | 900 秒 |

创建时若未指定或传入 `0`，服务端使用默认值 120。更新时若传入 `0` 且账户已有配置，则保留现有超时。

## MongoDB 字段

Collection：`dns_provider_accounts`

```yaml
id: dns_<uuid>           # 资源 ID，唯一索引
name: prod-cloudflare    # 显示名称，全局唯一
provider: cloudflare     # 首版固定 cloudflare
propagation_timeout_seconds: 120
api_token: "<secret>"    # Cloudflare API Token 明文（首版）；永不出现在 API 响应或审计日志
token_last_verified_at: 2026-05-30T08:00:00Z
created_at: ...
updated_at: ...
```

索引：

- `id`：唯一
- `name`：唯一
- `created_at`：列表排序（降序）

## API 行为

Connect 服务：`DNSProviderAccountService`（挂载于 `/api/grpc`）

| RPC | 说明 |
| --- | --- |
| `ListDNSProviderAccounts` | 分页列表；响应含 `token_configured`，不含 Token |
| `CreateDNSProviderAccount` | 请求体 `api_token` 必填；验证通过后保存 |
| `GetDNSProviderAccount` | 按 ID 查询；不含 Token |
| `UpdateDNSProviderAccount` | 可改名称与传播超时；`api_token` 非空时视为轮换，须验证成功后替换 |
| `DeleteDNSProviderAccount` | 删除账户记录 |

Secret 处理：

- 创建：无 Token 或验证失败 → 不创建记录。
- 轮换：新 Token 验证失败 → 旧 Token 与其它字段均不变。
- 更新：空 `api_token` → 保留已存储 Token。
- 审计日志与结构化日志不记录 Token 明文。

## Web 控制台

导航「证书」指向 `/certificates/dns-accounts`。支持：

- 列出账户、传播超时、Token 配置状态与最近验证时间。
- 创建账户（输入 Token，保存前验证）。
- 编辑名称/超时，或通过非空 Token 字段轮换凭据。
- 删除账户。
- 验证失败时在表单提交后展示 API 返回的错误信息。

## 相关术语

详见根目录 [CONTEXT.md](../CONTEXT.md) 中的 **DNSProviderAccount** 词条。
