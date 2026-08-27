# ADR-0003：独立 Server 分发 Token（CertificateAccessToken）

## 状态

已接受（2026-05）

## 背景

Server 需要稳定接口发现获授权证书并下载 active bundle，且凭据须可吊销、可轮换、与 Admin 会话分离。复用 Admin Bearer session 或 MCP token 会扩大 blast radius 并混淆审计语义。

## 决策

- 引入 **CertificateAccessToken**：前缀 `nlct_`，绑定单台 Server，MongoDB 存 hash + prefix。
- 独立 Connect 服务 **ServerCertificateService**；仅接受 `Authorization: Bearer nlct_*`。
- Admin session、MCP token 在该服务上一律 **unauthenticated**。
- Secret 明文 **仅创建时返回一次**；后续只能轮换不能读取。
- 分配/取消分配 ManagedCertificate 与 token 吊销 **立即** 生效；鉴权不缓存。
- Redis 按 token 限流（120 次/分钟）；Redis 故障 **fail-open**，MongoDB 鉴权仍 fail-closed。
- 成功 bundle 下载与鉴权失败写 audit；成功 List 仅 metrics/日志。

## 后果

- Server 集成使用标准 Bearer 与 Connect（protobuf 或 JSON/Base64）；无官方 agent/CLI。
- 一台 Server 可持多个 token 以支持无停机轮换。
- 首版无细粒度 scope；token 仅用于证书分发。

## 参考

- [Server 分发](./../certificate-management-server-distribution.md)
- [访问 Token 与 Server 分配](./../certificate-management-access-tokens.md)
