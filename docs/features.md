# 功能说明

neo-line 是一个面向 server 状态监控的项目。核心关注点是：

- 记录 server 是否可用、是否异常、异常原因是什么。
- 对 server 暴露的端口进行探测。
- 支持基础 TCP 端口探测、URL 探测，以及 TLS Port 探测。
- 根据探测结果聚合出 server 的整体健康状态。

本文档用于记录当前已实现功能、规划功能以及后续开发方向。

## 状态说明

- **已实现** — 当前代码中已经可用
- **规划中** — 已明确为目标功能，但尚未实现
- **未来增强** — 可能在后续版本中支持

## 已实现功能

### 应用骨架

**状态：** 已实现

项目已经初始化为一个基于 Butterfly 应用框架的 Go 服务。

当前信息：

- Module：`github.com/orvice/neo-line`
- 应用框架：`butterfly.orx.me/core/app`
- HTTP Router：`github.com/gin-gonic/gin`
- 入口文件：`cmd/server/main.go`

### Ping 测试接口

**状态：** 已实现

项目提供了一个基础接口，用于验证 HTTP 服务是否正常运行。

```http
GET /ping
```

示例响应：

```json
{
  "message": "pong"
}
```

该接口可用于本地开发、部署后 smoke test 或基础存活检查。

### Butterfly Runtime 基础能力

**状态：** 通过框架基础能力已具备

当调用 `application.Run()` 时，Butterfly runtime 会提供以下基础能力：

- 应用配置加载
- 日志初始化
- Prometheus metrics 初始化
- OpenTelemetry tracing 初始化
- HTTP 服务生命周期管理
- 初始化和退出清理 hooks

这些能力由框架提供。neo-line 自身的 server 状态监控和端口探测逻辑后续将在此基础上实现。

## 配置来源

### MongoDB 配置中心

**状态：** 规划中

neo-line 的监控业务配置统一从 MongoDB 读取。Server、monitor、探测参数、阈值、启用状态和告警策略都以 MongoDB 中的数据为准。

原则：

- MongoDB 是监控业务配置的唯一权威来源。
- 不使用本地 YAML、JSON 或静态文件作为 server / monitor 配置来源。
- API 创建、更新或删除配置时，应写入 MongoDB。
- 调度器和探测 worker 应从 MongoDB 读取已启用的 server 和 monitor。
- 配置变更后，运行中的探测任务需要能够刷新配置。
- 只有连接 MongoDB 所需的最小 bootstrap 信息可以来自运行环境或 Butterfly 应用配置。

初始需要从 MongoDB 读取的配置：

- Server metadata
- Monitor 配置
- Monitor 启用 / 禁用状态
- 探测间隔
- 超时时间
- 重试次数
- URL 探测参数
- TLS Port 探测参数
- TLS 校验模式
- SNI name
- Warning / Critical 阈值

## 核心目标

### Server 状态监控

**状态：** 规划中

Server 是 neo-line 的主要被监控对象。系统需要持续回答以下问题：

- 这台 server 当前是否可用？
- 哪些端口或 endpoint 正常？
- 哪些端口或 endpoint 异常？
- 异常发生在哪一层：DNS、TCP、TLS 还是 HTTP？
- 最近一次探测是什么时间，结果是什么，耗时是多少？

Server 可能状态：

- `Healthy`：所有启用的探测项均正常。
- `Warning`：存在需要关注的问题，例如 TLS 证书即将过期。
- `Critical`：存在严重风险，例如证书临近过期或探测连续失败但尚未确认 down。
- `Down`：关键端口不可达、URL 不可用，或 TLS Port 握手失败。
- `Unknown`：尚无有效探测结果，或 server 未启用任何探测项。

Server 状态应由其启用的 monitors 聚合得出。默认聚合规则建议使用最严重状态作为 server 状态：

```text
Down > Critical > Warning > Unknown > Healthy
```

### Server 管理

**状态：** 规划中

支持添加和管理需要被监控的 server。Server 配置存储在 MongoDB 中，运行时从 MongoDB 读取。

规划字段：

- Server ID
- 显示名称
- Hostname 或 IP 地址
- 环境，例如 production、staging
- 区域或数据中心
- Tags
- 启用 / 禁用状态
- 当前健康状态
- 最近一次状态变化时间
- 最近一次探测时间

每台 server 可以关联多个 monitor。一个 monitor 对应一个具体端口或 endpoint 探测。

## 端口探测

### Monitor 通用模型

**状态：** 规划中

Monitor 用于描述 neo-line 需要探测的目标，以及如何判断探测结果。Monitor 配置存储在 MongoDB 中，运行时从 MongoDB 读取。

通用字段：

- Monitor ID
- Server ID
- 监控名称
- 监控类型：`tcp`、`url`、`tls_port`
- 目标地址：host / IP / URL
- 目标端口，URL 探测可从 URL scheme 推导默认端口
- 检查间隔
- 超时时间
- 重试次数
- 启用 / 禁用状态
- Warning / Critical 阈值

每次探测都应该生成一条检查结果。结果字段建议包括：

- Server ID
- Monitor ID
- 状态：`Healthy`、`Warning`、`Critical`、`Down`、`Unknown`
- 开始时间
- 结束时间
- 探测耗时
- 错误阶段：`dns`、`tcp`、`tls`、`http`、`timeout`
- 错误信息
- 远端地址
- 端口

### TCP 端口探测

**状态：** 规划中

TCP 端口探测用于判断 server 上的某个端口是否可以建立 TCP 连接。

预期行为：

- 对配置的 host 和 port 发起 TCP 连接。
- 在超时时间内连接成功，则 monitor 为 `Healthy`。
- 连接失败、连接超时、连接被拒绝或网络不可达，则 monitor 为 `Down`。
- 记录连接耗时和错误原因。

典型目标端口：

- SSH：`22`
- HTTP：`80`
- HTTPS：`443`
- 自定义业务服务端口

TCP 探测只判断端口连通性，不判断应用协议响应是否正确。HTTP 和 HTTPS endpoint 应统一使用 URL 探测；只需要验证 TLS 握手和证书状态时使用 TLS Port 探测。

### URL 探测

**状态：** 规划中

URL 探测用于检查 HTTP 或 HTTPS endpoint 是否可访问，并判断响应是否符合预期。

预期行为：

- 向配置的 URL 发起请求，URL scheme 可以是 `http` 或 `https`。
- 支持配置 method、path、headers、期望状态码。
- 对 `https` URL 执行 TLS 握手和证书校验。
- 记录 DNS 解析耗时、TCP 连接耗时、TLS 握手耗时、总请求耗时、响应状态码和错误信息。
- 当 endpoint 不可访问、请求超时或返回非预期状态码时，monitor 标记为 `Down`。

MongoDB 配置字段：

- URL，例如 `http://example.com/health` 或 `https://example.com/health`
- Method，默认 `GET`
- Headers
- 期望状态码，默认 `200`
- Timeout
- TLS 校验模式，仅适用于 `https` URL
- 自定义 SNI name，仅适用于 `https` URL

健康条件：

- DNS 解析成功。
- TCP 连接成功。
- 如果 URL scheme 是 `https`，TLS 握手成功。
- 如果 URL scheme 是 `https` 且 TLS 校验开启，证书校验成功。
- 请求在超时时间内完成。
- HTTP 状态码匹配期望状态码。

异常条件：

- DNS 解析失败。
- TCP 连接失败。
- TLS 握手失败。
- 证书校验失败。
- 请求超时。
- HTTP 状态码不符合预期。
- 响应无效。

### TLS Port 探测

**状态：** 规划中

TLS Port 探测用于检查 server 的某个 TLS 端口是否可以完成 TLS 握手，并记录证书状态。

该探测不发送 HTTP 请求，也不判断 HTTP 状态码。它适用于只需要验证 TLS 层是否正常的服务，例如 HTTPS 端口、TLS 代理、LDAPS、SMTPS 或其他基于 TLS 的自定义服务。

预期行为：

- 对配置的 host 和 port 发起 TCP 连接。
- 建立 TCP 连接后执行 TLS 握手。
- 支持证书校验。
- 支持自定义 SNI name。
- 读取 peer certificate。
- 记录 DNS、TCP、TLS 各阶段结果和耗时。
- 当 DNS、TCP、TLS 或证书校验失败时，monitor 标记为 `Down`。

MongoDB 配置字段：

- Host 或 IP
- Port，默认 `443`
- Timeout
- TLS 校验模式
- 自定义 SNI name
- 证书过期 Warning 阈值
- 证书过期 Critical 阈值

健康条件：

- DNS 解析成功。
- TCP 连接成功。
- TLS 握手成功。
- 当 TLS 校验开启时，证书校验成功。
- 证书未过期，且不在 Warning / Critical 阈值内。

异常条件：

- DNS 解析失败。
- TCP 连接失败。
- TLS 握手失败。
- 证书校验失败。
- 证书已经过期或尚未生效。
- 证书临近过期。

### 自定义 SNI Name

**状态：** 规划中

URL 探测和 TLS Port 探测都支持设置自定义 SNI name，并在 TLS 握手阶段使用该名称。

该能力适用于以下场景：

- 通过 IP 地址探测 TLS 服务，但证书签发给域名。
- 探测 ingress 或 load balancer，并且后端路由依赖 SNI。
- 探测内部 TLS 服务，该服务要求指定 TLS server name。

预期行为：

- 如果配置了 `sni_name`，则使用它作为 TLS `ServerName`。
- 如果没有配置 `sni_name`，且 host 是域名，则默认使用 host 作为 TLS `ServerName`。
- 如果没有配置 `sni_name`，且 host 是 IP，则证书 hostname 校验可能失败，除非证书包含该 IP 或关闭 TLS 校验。
- 检查结果中需要记录 SNI 相关 TLS 错误。

### TLS 证书状态

**状态：** 规划中

TLS Port 探测需要记录证书状态。HTTPS URL 探测也可以记录证书状态，但证书状态的主模型应归属于 TLS Port 探测。

预期行为：

- 在 TLS 握手阶段获取 peer certificate。
- 记录证书 subject、issuer、DNS names、serial number、`not_before`、`not_after`。
- 计算证书剩余有效天数。
- 证书临近过期时进入 `Warning` 或 `Critical` 状态。
- 证书已过期、尚未生效或 TLS 握手失败时，monitor 标记为 `Down`。

初始阈值建议：

- Warning：证书将在 30 天内过期。
- Critical：证书将在 7 天内过期。
- Down：证书已经过期、尚未生效，或 TLS 握手失败。

证书检查必须遵循配置的自定义 SNI name，因为同一个 IP 和端口可能会根据 SNI 返回不同证书。

## 状态计算

### Monitor 状态计算

**状态：** 规划中

每个 monitor 需要根据最近一次或最近多次探测结果计算当前状态。

建议规则：

- 单次探测成功时，状态为 `Healthy`。
- 探测失败但未达到连续失败阈值时，状态可保持上一状态或进入 `Critical`。
- 连续失败达到阈值后，状态为 `Down`。
- TLS 证书进入过期阈值时，状态为 `Warning` 或 `Critical`。
- 没有探测结果时，状态为 `Unknown`。

### Server 状态计算

**状态：** 规划中

Server 状态由其启用的 monitors 聚合得出。

建议规则：

- 没有启用 monitor 时，server 状态为 `Unknown`。
- 任一关键 monitor 为 `Down` 时，server 状态为 `Down`。
- 没有 `Down`，但存在 `Critical` 时，server 状态为 `Critical`。
- 没有 `Down` / `Critical`，但存在 `Warning` 时，server 状态为 `Warning`。
- 所有启用 monitor 均为 `Healthy` 时，server 状态为 `Healthy`。

Server 状态变化时需要记录状态事件，用于后续告警和历史查询。

## API 规划

### Server API

**状态：** 规划中

提供 API 用于管理 server 和查询 server 健康状态。Server 配置的写入、更新和删除都应落到 MongoDB。

```http
GET /servers
POST /servers
GET /servers/:id
PUT /servers/:id
DELETE /servers/:id
GET /servers/:id/health
GET /servers/:id/events
```

### Monitor API

**状态：** 规划中

提供 API 用于管理 server 下的端口探测配置和查询探测结果。Monitor 配置的写入、更新和删除都应落到 MongoDB。

```http
GET /servers/:id/monitors
POST /servers/:id/monitors
GET /servers/:id/monitors/:monitor_id
PUT /servers/:id/monitors/:monitor_id
DELETE /servers/:id/monitors/:monitor_id
GET /servers/:id/monitors/:monitor_id/results
```

## 告警

**状态：** 规划中

当 server 状态或 monitor 状态发生异常变化时，可以发送告警通知。

初始告警条件：

- TCP 端口不可连接。
- URL endpoint 不可用。
- HTTP 状态码不符合预期。
- TLS 握手失败。
- TLS 证书即将过期。
- TLS 证书已经过期。
- Server 状态从正常变为异常。
- Server 状态从异常恢复。

## MongoDB 存储

**状态：** 规划中

MongoDB 是 neo-line 监控业务配置和运行结果的主要存储。

需要存储以下数据：

- Server metadata
- Monitor 配置
- Monitor 当前状态
- Server 当前状态
- 探测结果历史
- Server 状态变化事件
- TLS 证书信息
- 告警事件

初始 collection 建议：

- `servers`：server metadata、启用状态和当前健康状态
- `monitors`：monitor 配置、启用状态、探测参数和阈值
- `monitor_results`：每次探测的结果历史
- `server_events`：server 状态变化事件
- `tls_certificates`：TLS 证书快照和有效期信息
- `alert_events`：告警触发和恢复事件

读取规则：

- 调度器只调度 MongoDB 中 `enabled = true` 的 server 和 monitor。
- 探测 worker 执行前应使用当前有效的 monitor 配置。
- API 查询当前状态时应读取 MongoDB 中的最新状态字段或最近结果。
- 探测完成后应将结果、状态变化和证书信息写回 MongoDB。

## 未来增强

### Dashboard 支持

**状态：** 未来增强

为 Web dashboard 或外部 UI 提供所需数据。

可能的页面：

- Server 列表
- Server 详情
- Server 当前健康状态概览
- 端口探测配置列表
- 探测结果历史
- 延迟趋势图
- TLS 证书过期概览
- 告警历史

### 更多探测类型

**状态：** 未来增强

后续可以扩展更多协议或场景：

- ICMP ping
- UDP 端口探测
- gRPC health check
- DNS query check
- 自定义脚本探测

## 开发说明

新增功能时需要同步更新文档：

1. 更新本文档中的功能状态。
2. 如果新增 API，需要补充 API 文档。
3. 如果新增配置项，需要补充 MongoDB 字段说明和默认值。
4. 记录运行行为，例如端口、超时时间、重试次数、SNI 行为、证书阈值和默认值。
