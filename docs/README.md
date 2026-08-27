# neo-line 文档

neo-line 是一个基于 Go 和 Butterfly 应用框架构建的服务器监控服务。

项目的主要目标是：为服务器添加和管理监控配置，并对服务器暴露的网络服务进行 TCP、URL 和 TLS Port 探测。

所有监控业务配置都从 MongoDB 读取，包括 server、monitor、启停状态、探测参数和阈值。

## 文档目录

- [功能说明](./features.md) — 当前已实现能力、规划功能和功能边界
- [监控配置](./monitoring-configuration.md) — MongoDB 中 Server、TCP、URL、TLS Port、SNI 和证书状态的配置模型
- [证书管理 — DNS 账户](./certificate-management-dns-accounts.md) — Cloudflare DNSProviderAccount 配置、权限与传播超时
- [证书管理 — ACME Issuer](./certificate-management-issuers.md) — CertificateIssuer preset、EAB、ToS 与注册状态
- [证书管理 — 托管证书](./certificate-management-managed-certificates.md) — ManagedCertificate desired config、领域区别与 Pending Issue
- [证书管理 — 访问 Token 与 Server 分配](./certificate-management-access-tokens.md) — CertificateAccessToken 与 Server 授权
- [证书管理 — Server 分发](./certificate-management-server-distribution.md) — ServerCertificateService、限流与调用示例
- [证书管理 — Operation Lease 与多副本](./certificate-management-operation-lease.md) — Mongo lease、接管与 TXT 清理
- [证书管理 — 运维与默认值](./certificate-management-operations.md) — 默认值、Secret 假设、metrics 与 CA preset
- [证书管理 — 停用/吊销/回滚/删除](./certificate-management-destructive-operations.md) — 四类破坏性操作语义与约束
- [证书管理 — 首版范围外](./certificate-management-out-of-scope.md) — 明确不在首版内的能力边界
- [领域术语（CONTEXT.md）](../CONTEXT.md) — ManagedCertificate、CertificateVersion 等 glossary
- [架构决策记录（ADR）](./adr/) — 明文 Secret、双版本模型、Server token、reconciler

## 当前应用基础

当前项目已经初始化了 Butterfly 服务，提供 MongoDB API，并运行后台探测调度器：

- 服务名称：`neo-line`
- HTTP 框架：Gin，由 Butterfly `app` 管理
- 测试端点：`GET /ping`
- Connect gRPC-Web API：统一挂载在 `/api/grpc`
- AuthService：登录、登出和当前用户查询；除公开接口外，Connect API 通过 Bearer token 鉴权
- Web Dashboard：登录后通过 `/dashboard` 查看内部运维总览、异常监控、证书关注、最近审计和快捷入口
- ServerService：Server 管理与健康聚合查询
- MonitorService：Monitor 管理、检查结果和 uptime 查询
- MonitorGroupService：Monitor Group 管理（含分组级告警策略，引用通知组派发）
- NotifyGroupService：可复用的 webhook / Telegram / Discord / Mastodon 通知组
- DNSProviderAccountService：Cloudflare DNS-01 账户管理（admin 专用；Token 验证后保存，读取接口脱敏）
- CertificateIssuerService：ACME Issuer 管理（admin 专用；异步注册、ToS 同意、Secret 脱敏）
- ManagedCertificateService：托管证书 desired config CRUD（admin 专用；创建后自动 Pending Issue；读取脱敏）
- AuditLogService：按来源、动作、资源、调用方、结果与时间范围查询操作审计日志
- 探测调度器：随应用启动，每 `5s` 从 MongoDB 读取已启用的 monitor 并按各自间隔执行探测
- 默认 HTTP 服务端口：`8080`，由 Butterfly 提供
- Metrics 端口：`2223`，由 Butterfly 初始化流程提供
- MongoDB 连接：通过 Butterfly 配置 `store.mongo.<key>.uri` 初始化，默认应用侧使用 `primary` key
- MongoDB 数据库：通过应用配置 `mongo.database` 指定，默认 `neo_line`
- Redis 会话：通过 Butterfly 配置 `store.redis.<key>` 初始化，默认应用侧使用 `session` key 存储 Bearer token
- Admin 账户初始化：`ADMIN_PASSWORD`（必填，用于设置/轮换密码），`ADMIN_EMAIL`（可选，默认 `admin@neo-line.local`）

## 产品范围

neo-line 的核心监控能力包括：

- 添加和管理被监控的服务器
- 为每台服务器添加多个监控配置，配置统一存储在 MongoDB
- 监控普通 TCP 端口是否可连接
- 通过 URL 探测统一监控 HTTP 和 HTTPS 服务是否可访问
- 通过 TLS Port 探测监控 TLS 握手和证书状态
- 登录态 Web Dashboard 聚合展示服务器、监控项、异常状态、证书关注、审计日志与运维入口
- URL 和 TLS Port 探测支持自定义 SNI name
- 记录每个监控项的健康状态和检查结果
- 通过 monitor 分组聚合监控项并配置分组级告警策略，引用可复用通知组派发告警
- 通过 Connect API、MCP 工具和 Web 控制台服务器详情页使用本地 SSH 私钥连接服务器并执行远程命令（`SshService.Exec` / `ssh_exec`）
- 记录并查询 Connect API 和 MCP 操作的审计日志（`audit_logs` collection / `AuditLogService.ListAuditLogs` / Web 控制台「审计」页面）

## 文档维护

本文档是持续演进的项目文档。新增、修改或移除功能时，需要同步更新相关文档。
