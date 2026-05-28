# neo-line 文档

neo-line 是一个基于 Go 和 Butterfly 应用框架构建的服务器监控服务。

项目的主要目标是：为服务器添加和管理监控配置，并对服务器暴露的网络服务进行 TCP、URL 和 TLS Port 探测。

所有监控业务配置都从 MongoDB 读取，包括 server、monitor、启停状态、探测参数和阈值。

## 文档目录

- [功能说明](./features.md) — 当前已实现能力、规划功能和功能边界
- [监控配置](./monitoring-configuration.md) — MongoDB 中 Server、TCP、URL、TLS Port、SNI 和证书状态的配置模型

## 当前应用基础

当前项目已经初始化了 Butterfly 服务，并提供了基础 MongoDB API：

- 服务名称：`neo-line`
- HTTP 框架：Gin，由 Butterfly `app` 管理
- 测试端点：`GET /ping`
- Server API：`/v1/servers`
- Monitor API：`/v1/servers/:id/monitors`
- 默认 HTTP 服务端口：`8080`，由 Butterfly 提供
- Metrics 端口：`2223`，由 Butterfly 初始化流程提供
- MongoDB 连接：`MONGODB_URI`，默认 `mongodb://localhost:27017`
- MongoDB 数据库：`MONGODB_DATABASE`，默认 `neo_line`

## 产品范围

neo-line 的核心监控能力包括：

- 添加和管理被监控的服务器
- 为每台服务器添加多个监控配置，配置统一存储在 MongoDB
- 监控普通 TCP 端口是否可连接
- 通过 URL 探测统一监控 HTTP 和 HTTPS 服务是否可访问
- 通过 TLS Port 探测监控 TLS 握手和证书状态
- URL 和 TLS Port 探测支持自定义 SNI name
- 记录每个监控项的健康状态和检查结果

## 文档维护

本文档是持续演进的项目文档。新增、修改或移除功能时，需要同步更新相关文档。
