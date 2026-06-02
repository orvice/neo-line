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

**状态：** 已实现（初始 API 读写能力）

neo-line 的监控业务配置统一从 MongoDB 读取。Server、monitor、探测参数、阈值、启用状态和告警策略都以 MongoDB 中的数据为准。

当前实现：

- 服务启动时由 Butterfly 读取 `store.mongo.<key>.uri` 并初始化 MongoDB client；应用默认使用 `store.mongo.primary`。
- 通过应用配置 `mongo.database` 指定数据库，默认 `neo_line`。
- Server API 会读写 `servers` collection，并支持 `sort_order` 排序字段。
- Monitor API 会读写 `monitors` collection。
- Monitor group API 会读写 `monitor_groups` collection。
- Check result 查询会读取 `monitor_results` collection。
- Server event 查询和状态聚合事件会读写 `server_events` collection。
- Connect API 与 MCP tool 调用会写入 `audit_logs` collection，用于操作审计。

原则：

- MongoDB 是监控业务配置的唯一权威来源。
- 不使用本地 YAML、JSON 或静态文件作为 server / monitor 配置来源。
- API 创建、更新或删除配置时，应写入 MongoDB。
- 调度器和探测 worker 应从 MongoDB 读取已启用的 server 和 monitor。
- 配置变更后，运行中的探测任务需要能够刷新配置。
- 只有连接 MongoDB 所需的最小 bootstrap 信息可以来自运行环境或 Butterfly 应用配置。

### 操作审计日志

**状态：** 已实现

neo-line 会为 Connect API 和 MCP tool 调用记录操作日志：

- 运行日志：通过 `slog` 输出，包含来源、动作、资源类型、资源 ID、状态、耗时等字段。
- 持久化日志：写入 MongoDB `audit_logs` collection。审计写入失败只记录错误日志，不阻塞原 API 或 MCP 操作。
- Connect API：统一挂载在 `/api/grpc`，覆盖公开读接口与 Admin 写接口。已登录 Admin 请求会记录 `actor_id` 与 `actor_email`。
- MCP：覆盖所有 MCP tools，记录 tool 名、token 前缀、token 来源、资源类型、资源 ID、成功/失败与耗时。
- 敏感信息：不会记录 Authorization header、MCP token 明文、登录密码、MCP token 创建接口返回的一次性 secret、通知通道 secret 等完整请求/响应体。

`audit_logs` 字段：

- `id`：审计记录 ID，格式 `aud_<uuid>`。
- `source`：来源，`api` 或 `mcp`。
- `actor_id` / `actor_email`：API 登录用户信息；公开接口和 MCP 可能为空。
- `token_prefix`：MCP token 前缀，仅用于识别调用方，不含完整 token。
- `action`：API 为 `read` / `create` / `update` / `delete`，MCP 为 tool 名。
- `resource_type` / `resource_id`：被操作资源类型与 ID。
- `method` / `path` / `status_code`：Connect API 请求信息。
- `success` / `error`：操作是否成功及错误摘要。
- `duration_ms`：操作耗时。
- `remote_ip` / `user_agent`：Connect API 请求来源信息。
- `metadata`：扩展字段，例如 MCP token 来源。
- `occurred_at`：操作发生时间。

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

**状态：** 已实现（调度器周期探测并聚合健康状态）

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

**状态：** 已实现（基础 CRUD 与健康查询 API）

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

**状态：** 已实现（基础 CRUD 与结果查询 API）

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

TCP 和 TLS Port monitor 的 `host` 可以留空。运行时探测会使用所属 server 的 `host` 作为目标地址；如果 monitor 自己配置了 `host`，则以 monitor 的值为准。

每次探测都应该生成一条检查结果。结果字段建议包括：

- Server ID
- Monitor ID
- 状态：`Healthy`、`Warning`、`Critical`、`Down`、`Unknown`
- 开始时间
- 结束时间
- 探测耗时
- 错误阶段：`none`、`dns`、`tcp`、`tls`、`http`、`timeout`（`none` 表示探测成功）
- 错误信息
- 远端地址
- 端口

### TCP 端口探测

**状态：** 已实现

TCP 端口探测用于判断 server 上的某个端口是否可以建立 TCP 连接。

预期行为：

- 对配置的 host 和 port 发起 TCP 连接。
- 如果 monitor 未配置 host，则使用所属 server 的 host。
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

**状态：** 已实现

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
- 期望状态码表达式，字符串类型，支持逗号分隔的单个状态码与闭区间范围，例如 `"200-299,301,302"`，默认 `200`
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

**状态：** 已实现

TLS Port 探测用于检查 server 的某个 TLS 端口是否可以完成 TLS 握手，并记录证书状态。

该探测不发送 HTTP 请求，也不判断 HTTP 状态码。它适用于只需要验证 TLS 层是否正常的服务，例如 HTTPS 端口、TLS 代理、LDAPS、SMTPS 或其他基于 TLS 的自定义服务。

预期行为：

- 对配置的 host 和 port 发起 TCP 连接。
- 如果 monitor 未配置 host，则使用所属 server 的 host。
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

**状态：** 已实现

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

**状态：** 已实现

TLS Port 探测需要记录证书状态。HTTPS URL 探测也可以记录证书状态，但证书状态的主模型应归属于 TLS Port 探测。

预期行为：

- 在 TLS 握手阶段获取 peer certificate。
- 记录证书 subject、issuer、DNS names、serial number、`not_before`、`not_after`。
- 计算证书剩余有效天数。
- 将最近一次证书信息写回 monitor 的 `certificate` 字段作为当前快照，前端在服务器的监控项列表和监控详情页展示证书到期时间与剩余天数。
- 证书临近过期时进入 `Warning` 或 `Critical` 状态。
- 证书已过期、尚未生效或 TLS 握手失败时，monitor 标记为 `Down`。

初始阈值建议：

- Warning：证书将在 30 天内过期。
- Critical：证书将在 7 天内过期。
- Down：证书已经过期、尚未生效，或 TLS 握手失败。

证书检查必须遵循配置的自定义 SNI name，因为同一个 IP 和端口可能会根据 SNI 返回不同证书。

## 探测调度

**状态：** 已实现

调度器负责周期性地执行已启用的 monitor，并将结果写回 MongoDB。

运行行为：

- 调度器每 `5s` 进行一次 reconcile，从 MongoDB 重新读取 `enabled = true` 的 server 及其 `enabled = true` 的 monitor，因此配置变更无需重启即可生效。
- 每个 monitor 按各自的 `interval_seconds` 判断是否到期，到期才会触发探测。
- 同一时刻最多并发执行 `32` 个探测，且同一个 monitor 不会重叠执行。
- 单次探测在 `timeout_seconds` 内完成；失败（`Down`）时会在一次探测调用内最多重试 `retries` 次，任意一次成功或得到 `Warning` / `Critical` 这类确定结果即提前结束。
- 探测完成后写入 `monitor_results`，更新 monitor 的当前状态与 `last_check_at`，并重新聚合所属 server 的健康状态，状态变化时写入 `server_events`。

调度器在应用启动时通过 Butterfly `InitFunc` 拉起，在退出时通过 `TeardownFunc` 停止。

## Prometheus 监控指标

**状态：** 已实现

neo-line 在 Butterfly 框架自动暴露的 Prometheus registry 上注册自身业务指标。框架默认在专用端口 `2223` 上暴露 `/metrics`，可通过 `curl http://localhost:2223/metrics` 抓取。

调度器在每次 reconcile 和每次探测完成时更新以下指标：

- `neoline_probe_total{kind,status}`：探测执行总数，按 monitor 类型和结果状态计数。
- `neoline_probe_duration_seconds{kind}`：探测耗时直方图（秒），按 monitor 类型区分。
- `neoline_probe_errors_total{kind,stage}`：探测失败计数，按类型和失败阶段（`dns` / `tcp` / `tls` / `http` / `timeout`）区分。
- `neoline_monitor_status_changes_total{kind,previous_status,status}`：monitor 状态切换计数。
- `neoline_monitor_status{monitor_id,server_id,kind}`：每个 monitor 的当前健康状态（数值编码：`0=Unknown`、`1=Healthy`、`2=Warning`、`3=Critical`、`4=Down`）。
- `neoline_certificate_days_remaining{monitor_id,server_id}`：观测到 TLS 证书时，距离过期的剩余天数。
- `neoline_enabled_monitors`：当前已启用的 monitor 数量。
- `neoline_scheduler_reconcile_total`：调度器 reconcile tick 总数。

指标定义集中在 `internal/metric`，由该包 `init()` 注册到 `otel.PrometheusRegistry()`。

## 可用率与心跳

**状态：** 已实现

每个 monitor 会基于 `monitor_results` 中已持久化的探测历史，实时计算最近一段时间的可用率（uptime）和心跳序列，呈现方式与 Uptime Kuma 类似。

运行行为：

- 可用率按滚动时间窗计算，当前提供 `1h`（最近 1 小时）和 `24h`（最近 24 小时）两个窗口。
- 可用判定：状态为 `Down` 记为不可用，其余状态（`Healthy`、`Warning`、`Critical`）均视为目标有响应、记为可用。`Warning` / `Critical` 通常表示 TLS 证书临近过期，不影响可用率。
- 每个窗口返回总检查次数、可用次数、不可用次数、可用率（0–1）以及可用检查的平均耗时（平均耗时只统计可用检查，避免超时拉高数值）。
- 心跳序列返回最近最多 `50` 条检查（状态、开始时间、耗时），按时间从旧到新排序，便于从左到右渲染心跳条。
- 数据完全来自 `monitor_results`，不在 monitor 文档上做反规范化冗余；聚合读取最多回溯 `24h`。

前端在 monitor 详情页展示：最近 1 小时 / 24 小时可用率，以及 Kuma 风格的心跳条（悬停显示该次检查的状态、时间与耗时）。

## 状态页（首页）

**状态：** 已实现

前端首页（`/`）是一个 Uptime Kuma 风格的公开状态页，无需登录即可查看，数据全部来自公开读接口。

运行行为：

- 顶部展示整体状态横幅，按所有展示中 monitor 的当前状态聚合：存在 `Down` / `Critical` 时显示「部分系统发生中断」，存在 `Warning` 时显示「部分系统出现异常」，否则显示「所有系统正常运行」，并展示最近一次检查的相对时间。
- 内容按 monitor 分组（`MonitorGroup`）组织：每个分组渲染为一张卡片，列出该分组下已启用的 monitor。未加入任何分组的 monitor 不会出现在状态页。
- 仅展示所属 server 处于启用（`enabled = true`）状态的 monitor。当 server 被暂停 / 未启用时，其下所有 monitor 都不会出现在状态页（与调度器只探测已启用 server 的行为保持一致）。
- 每个 monitor 行展示状态圆点、名称、最近 30 个心跳条以及最近 24 小时可用率，点击可进入 monitor 详情页。
- 数据每 60 秒自动刷新一次，也可点击横幅上的刷新按钮手动刷新。
- 支持「标准 / 紧凑」两种布局密度切换：标准布局每个 monitor 展示完整明细卡片（探测参数、证书环、心跳条等）；紧凑布局将服务器卡片排布为更多列（最多 3 列），并把每个 monitor 折叠为单行（状态圆点、名称、目标、迷你心跳条、24h 可用率与响应时间），从而在一屏内显示更多服务器。该偏好保存在浏览器 `localStorage`（键 `status-page-density`），按访问者本地记忆，不写入服务端。
- 原服务器管理列表移动到 `/servers`，顶部导航新增「状态」入口指向首页。

## 站点设置

**状态：** 已实现

支持配置站点的展示信息，用于状态页和管理面板的标题呈现。配置以单例文档形式存储在 MongoDB 中。

字段：

- `site_name`：网站名称，显示在顶部导航品牌位以及浏览器标签页标题（`document.title`）。默认 `neo-line`。
- `status_page_title`：状态页（首页）顶部的大标题。默认 `服务状态`。

运行行为：

- 设置存储在 `settings` collection 的单例文档（`_id: "site"`）中，写入时缺省字段会回落到默认值，保证状态页始终有可展示的名称。
- `SettingsService.GetSettings` 公开，无需登录，未配置时返回默认值。
- `SettingsService.UpdateSettings` 需鉴权，请求体包含 `settings.site_name` 与 `settings.status_page_title`。
- 前端在登录后通过顶部导航「设置」入口（`/settings`）编辑，保存后状态页标题与站点名称即时刷新。

## 状态计算

### Monitor 状态计算

**状态：** 已实现（基于最近一次探测结果，单次探测内按 `retries` 重试后判定 Down）

每个 monitor 需要根据最近一次或最近多次探测结果计算当前状态。

建议规则：

- 单次探测成功时，状态为 `Healthy`。
- 探测失败但未达到连续失败阈值时，状态可保持上一状态或进入 `Critical`。
- 连续失败达到阈值后，状态为 `Down`。
- TLS 证书进入过期阈值时，状态为 `Warning` 或 `Critical`。
- 没有探测结果时，状态为 `Unknown`。

### Server 状态计算

**状态：** 已实现（基于已启用 monitor 当前状态聚合）

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

**状态：** 已实现

提供 API 用于管理 server 和查询 server 健康状态。Server 配置的写入、更新和删除都会落到 MongoDB。Server 列表按 `sort_order` 升序返回，值越小越靠前；排序值相同时按 `created_at` 倒序返回。管理面板的 server 列表在「排序」列提供上移 / 下移按钮，点击后会把整张列表重排为连续的 `sort_order`（从 `0` 起），并仅对发生变化的 server 调用更新接口持久化。搜索过滤生效时禁用该重排操作。

```text
ServerService.ListServers
ServerService.CreateServer
ServerService.GetServer
ServerService.UpdateServer
ServerService.DeleteServer
ServerService.GetServerHealth
ServerService.ListServerEvents
```

其中创建、更新、删除为 Admin 写操作，需携带 Bearer token；查询接口保持公开。

支持的查询参数：

- `ListServersRequest`：`page_size`、`page_token`、`environment`、`tags`
- `ListServerEventsRequest`：`page_size`、`page_token`

### Monitor API

**状态：** 已实现

提供 API 用于管理 server 下的端口探测配置和查询探测结果。Monitor 配置的写入、更新和删除都会落到 MongoDB。

```text
MonitorService.ListMonitors
MonitorService.CreateMonitor
MonitorService.GetMonitor
MonitorService.UpdateMonitor
MonitorService.DeleteMonitor
MonitorService.ListCheckResults
MonitorService.GetMonitorUptime
```

### Monitor Group API

**状态：** 已实现

提供 API 用于管理分组及查询分组下的 monitor。Monitor group 配置的写入、更新和删除都会落到 MongoDB。分组列表按 `sort_order` 升序返回，值越小越靠前；排序值相同时按 `created_at` 倒序返回。管理面板的分组列表在「排序」列提供上移 / 下移按钮，点击后会把整张列表重排为连续的 `sort_order`（从 `0` 起），并仅对发生变化的分组调用更新接口持久化。搜索过滤生效时禁用该重排操作。

```text
MonitorGroupService.ListMonitorGroups
MonitorGroupService.CreateMonitorGroup
MonitorGroupService.GetMonitorGroup
MonitorGroupService.UpdateMonitorGroup
MonitorGroupService.DeleteMonitorGroup
MonitorGroupService.ListMonitorsByGroup
```

其中创建、更新、删除为 Admin 写操作，需携带 Bearer token；查询接口保持公开。

支持的查询参数：

- 分组列表：`page_size`、`page_token`
- 分组下 monitor 列表：`page_size`、`page_token`

错误码：

- `409 Conflict`：分组名称已存在。
- `400 Bad Request`：写入 monitor 时 `group_ids` 中存在不可识别的 ID。

其中 `POST` / `PUT` / `DELETE` 为 Admin 写操作，需携带 Bearer token；`GET` 查询接口保持公开。

支持的查询参数：

- monitor 列表：`page_size`、`page_token`
- result 列表：`page_size`、`page_token`、`start_time`、`end_time`，时间格式为 RFC3339。
- uptime：无查询参数，返回 `1h` / `24h` 可用率窗口和最近最多 50 条心跳。

`GET .../uptime` 响应示例：

```json
{
  "uptime": {
    "windows": {
      "1h": { "window_seconds": 3600, "total": 60, "up": 59, "down": 1, "uptime": 0.9833, "avg_latency_ms": 42.5 },
      "24h": { "window_seconds": 86400, "total": 1440, "up": 1438, "down": 2, "uptime": 0.9986, "avg_latency_ms": 41.2 }
    },
    "heartbeats": [
      { "status": "Healthy", "started_at": "2026-05-29T01:00:00Z", "duration_ms": 40 }
    ]
  }
}
```

## MCP Server

**状态：** 已实现

neo-line 基于官方 `github.com/modelcontextprotocol/go-sdk` 暴露一个 MCP（Model Context Protocol）server，方便 AI 客户端以工具调用方式读写监控配置和监控数据。

运行行为：

- 使用 streamable HTTP transport，挂载在 gin router 的 `/api/mcp` 路径上。
- 提供读和写工具，底层复用 MongoDB store，与现有 Connect API 行为一致。
- 工具的输入 / 输出 schema 由 Go struct 通过 `jsonschema` tag 自动推导。

只读工具：

- `list_servers`：列出 server，支持 `environment`、`tags`、`page_size`、`page_token`。
- `get_server`：按 `id` 查询单个 server。
- `get_server_health`：查询 server 聚合健康状态及各状态 monitor 数量。
- `list_server_events`：查询 server 状态变化事件。
- `list_monitors`：列出 server 下的 monitor。
- `get_monitor`：按 `server_id` + `monitor_id` 查询单个 monitor。
- `list_check_results`：查询 monitor 探测结果，支持 RFC3339 的 `start_time` / `end_time` 范围。
- `get_monitor_uptime`：按 `server_id` + `monitor_id` 查询 Kuma 风格的滚动可用率窗口。
- `list_monitor_groups`：列出所有 monitor 分组及其 alert policy。
- `get_monitor_group`：按 `group_id` 查询单个分组。
- `list_monitors_by_group`：列出指定分组下的 monitor（跨 server）。
- `list_notify_groups`：列出所有通知组及其通道。
- `get_notify_group`：按 `notify_group_id` 查询单个通知组。

写入工具：

- `create_server` / `update_server` / `delete_server`：创建、更新、删除被监控的 server（删除 server 同时清除其下 monitor）。
- `create_monitor` / `update_monitor` / `delete_monitor`：在指定 server 下创建、更新、删除 monitor。
- `create_monitor_group` / `update_monitor_group` / `delete_monitor_group`：管理 monitor 分组及其 `alert_policy`（含 `notify_group_ids`）。
- `create_notify_group` / `update_notify_group` / `delete_notify_group`：管理可复用的通知组及其通道。

SSH 工具（仅当配置了全局 `ssh.key_path` 时才注册）：

- `ssh_exec`：在指定 server 上通过 SSH 执行 shell 命令，入参 `server_id`、`command`、可选 `timeout_seconds`（默认 30），返回 `stdout`、`stderr`、`exit_code`。命令的非零退出码通过 `exit_code` 返回，不视为错误；连接或握手失败才返回错误。
- `ssh_test_connection`：对指定 server 建立 SSH 连接并执行一条 `echo` 命令，用于验证连通性与密钥认证。

详见下文「SSH 远程执行」。

### MCP 鉴权

**状态：** 已实现

MCP 端点使用 header token 鉴权，支持多个持久化 token：

- 请求需在 `Authorization: Bearer <token>` 或 `X-MCP-Token: <token>` 头中携带 token，否则返回 `401`。
- token 来源有两类，二者均有效：
  - MongoDB `mcp_tokens` 集合中存储的 token，可通过 Admin API 或 Web 控制台「MCP 接入 → 访问 Token」生成与吊销。
  - 环境变量 `MCP_AUTH_TOKEN` 配置的静态 token（向后兼容，可选）。
- 当环境变量为空且 `mcp_tokens` 集合中没有任何 token 时，`/api/mcp` 不做鉴权（适用于受信任内网或本地开发）。
- 写入工具与读取工具共用同一组 token，没有更细粒度的权限区分；在生产环境务必配置 token。

`mcp_tokens` 集合的字段：

- `id`：token 记录 ID。
- `name`：便于识别的名称（如使用方或用途）。
- `token_hash`：token 明文的 SHA-256 哈希，建立唯一索引；明文不会落库。
- `prefix`：token 明文前缀，仅用于列表展示。
- `created_at` / `last_used_at`：创建时间与最近一次校验通过的时间。

Token 明文仅在创建时返回一次，之后无法再次获取，只能吊销后重新生成。

Admin API：

- `McpTokenService.ListMcpTokens`：列出所有 token（不含明文）。
- `McpTokenService.CreateMcpToken`：按 `name` 生成新 token，响应中包含一次性明文 `secret`。
- `McpTokenService.DeleteMcpToken`：吊销 token。

## SSH 远程执行

**状态：** 已实现（通过 MCP 工具，无限制命令执行）

neo-line 支持通过本地 SSH 私钥连接被监控的 server，并以 MCP 工具的形式提供远程命令执行能力。私钥本身**不入库**：全局只在运行时配置中指定一个本地私钥路径，MongoDB 中的 server 文档只保存连接目标（host / port / user）等可继承、可覆盖的字段。

运行行为：

- 全局 SSH 配置来自运行时配置 `ssh`（见 `docs/monitoring-configuration.md`），其中 `ssh.key_path` 是唯一的私钥来源。`key_path` 为空时，SSH 相关 MCP 工具不会注册。
- 启动时一次性读取并解析私钥；私钥不存在或解析失败会导致启动报错（fail fast）。
- 每台 server 通过 `ssh` 子文档启用并按需覆盖连接目标，缺省字段继承全局默认：
  - `ssh.enabled`：是否允许对该 server 执行 SSH 工具，默认 `false`。
  - `ssh.host`：SSH 连接地址，为空时回落到 server 的 `host`。
  - `ssh.port`：SSH 端口，为空时回落到全局 `ssh.port`（默认 `22`）。
  - `ssh.user`：SSH 用户，为空时回落到全局 `ssh.user`（默认 `root`）。
- 主机密钥校验：配置了全局 `ssh.known_hosts_path` 时按 known_hosts 校验主机密钥；未配置时**不校验主机密钥**（无限制模式，仅适用于受信任内网）。
- 命令执行带超时（`ssh_exec` 默认 30s，`ssh_test_connection` 固定 15s）。命令非零退出码通过 `exit_code` 返回，不视为错误；仅连接或握手失败才返回错误。

安全说明：

- 当前为**无限制**版本：`ssh_exec` 可执行任意命令，没有命令白名单或只读限制。访问控制完全依赖 MCP token 鉴权，生产环境务必配置 MCP token 并谨慎分发。
- SSH 工具调用会与其它 MCP 工具一样写入 `audit_logs` 审计记录。

MongoDB `servers` 文档中的 SSH 子文档示例：

```yaml
ssh:
  enabled: true
  host: "10.0.0.12"   # 可选，缺省用 server.host
  port: 22            # 可选，缺省用全局默认
  user: "ops"         # 可选，缺省用全局默认
```

## 用户与鉴权

**状态：** 已实现

neo-line 提供基于 email + password 的用户系统，用于保护 Admin 相关接口。

用户信息存储在 MongoDB，登录会话 token 存储在 Redis：

- MongoDB `users`：账户信息，包含 `id`、`email`、`password_hash`、`role`、`created_at`、`updated_at`。密码使用 bcrypt 哈希存储，明文密码不会落库。
- Redis `neo-line:session:<token>`：登录后签发的 Bearer token 会话 JSON，包含 `token`、`user_id`、`email`、`role`、`created_at`、`expires_at`。Redis key 使用 `24h` TTL 自动过期。

运行行为：

- `email` 建立唯一索引，账户创建保持幂等。
- 登录成功后签发一个随机 token，默认有效期 `24h`。
- 请求 Admin 接口时在 `Authorization: Bearer <token>` 头中携带 token。
- token 缺失、无效或过期时返回 `401`。

### Admin 账户初始化

**状态：** 已实现

Admin 账户从运行环境初始化，环境是 admin 密码的权威来源：

- `ADMIN_PASSWORD`：必填。设置后服务启动时会创建或更新 admin 账户的密码哈希，因此修改该值即可轮换密码。未设置时跳过 admin 初始化，不改动已有账户。
- `ADMIN_EMAIL`：可选，默认 `admin@neo-line.local`。

该初始化逻辑在服务启动连接 MongoDB 和 Redis 后执行，同时确保 `users.email` 唯一索引存在。

### 认证 API

**状态：** 已实现

```text
AuthService.Login           # 公开，email + password 登录，返回 token
AuthService.GetCurrentUser  # 需鉴权，返回当前用户信息
AuthService.Logout          # 需鉴权，吊销当前 token
```

登录请求体：

```json
{
  "email": "admin@neo-line.local",
  "password": "your-password"
}
```

登录成功响应：

```json
{
  "token": "<bearer-token>",
  "expires_at": "2026-05-30T01:00:00Z",
  "user": { "id": "usr_...", "email": "admin@neo-line.local", "role": "admin" }
}
```

### 接口鉴权范围

**状态：** 已实现

- 公开接口：`GET /ping`、`AuthService.Login`、`SettingsService.GetSettings`、`StatusService.GetStatusOverview`，以及 server / monitor 的只读查询，方便 dashboard 读取。
- 需鉴权的 Admin 接口：server 和 monitor 的写操作、`SettingsService.UpdateSettings`，以及 `AuthService.GetCurrentUser`、`AuthService.Logout`。

## Monitor 分组

**状态：** 已实现

Monitor 可以归入零个或多个分组（`MonitorGroup`），分组扁平、不支持嵌套，同一个 monitor 可以同时属于多个分组。

运行行为：

- 分组存储在 MongoDB `monitor_groups` collection 中，`name` 字段全局唯一。
- 分组支持 `sort_order` 排序字段，默认值为 `0`；启动时会为历史分组补齐缺失的 `sort_order: 0`。列表、状态页和分组选择 UI 按 `sort_order ASC, created_at DESC` 展示。
- 监控项的 `group_ids` 字段在 `monitors` 文档上以数组形式持久化，并建有多键索引以便按分组列表 monitor。
- 创建或更新 monitor 时，会校验 `group_ids` 中的每个 ID 是否存在；不存在时返回 `400`，错误信息包含 `ErrInvalidGroupIDs`。
- 删除分组会从所有 monitors 的 `group_ids` 中 `$pull` 掉该 ID，monitor 自身不会被删除。

分组用于：

- 在 UI 上跨 server 聚合展示监控项。
- 配置分组级别的告警策略（见下文）。

## 告警

**状态：** 已实现（基于分组 alert policy 的多通道派发）

告警按"分组级 AlertPolicy"驱动：调度器在每次探测完成后，如果 monitor 状态发生变化，会查询其所属的每个分组的 `AlertPolicy`，对启用且匹配条件的分组逐个派发通知。

派发为 best-effort：通道调用失败仅记日志，不阻塞调度器主流程和 `monitor_results` 写入。

AlertPolicy 字段：

- `enabled`：是否启用告警；为 `false` 时该分组永不派发。
- `on_down`：monitor 状态变为 `Down` 时派发。
- `on_recover`：非健康状态恢复为 `Healthy` 时派发；首次探测得到 `Healthy` 不算恢复。
- `on_warning`：monitor 状态变为 `Warning` 时派发。
- `on_critical`：monitor 状态变为 `Critical` 时派发。
- `min_interval_seconds`：同 `(group, monitor)` 维度的派发节流窗口；`0` 或未填表示不节流。
- `notify_group_ids`：引用的通知组 ID 列表（见下文「通知组」）。派发时解析这些通知组并汇总它们的全部通道。为空时该分组不向任何通道派发。

## 通知组

**状态：** 已实现

通知组（`NotifyGroup`）是一组可复用的通知通道，独立于 monitor 分组存在。分组的 `AlertPolicy` 通过 `notify_group_ids` 引用一个或多个通知组来派发告警，多个分组可以共享同一个通知组。

运行行为：

- 通知组存储在 MongoDB `notify_groups` collection 中，`name` 字段全局唯一。
- 创建或更新 monitor 分组时，会校验 `alert_policy.notify_group_ids` 中的每个 ID 是否存在；不存在时返回 `400`，错误信息包含 `ErrInvalidNotifyGroupIDs`。
- 删除通知组会从所有 monitor 分组的 `alert_policy.notify_group_ids` 中 `$pull` 掉该 ID，分组自身不会被删除。
- 派发时，调度器解析分组引用的每个通知组，汇总它们的 `channels` 后逐个发送；无法解析的通知组仅记日志并跳过。

NotifyGroup 字段：

- `id`：通知组 ID（`ntf_` 前缀）。
- `name`：全局唯一名称。
- `description`：可选描述。
- `channels[].type`：通道类型，支持 `webhook`、`telegram`、`discord`、`mastodon`。
- `channels[].target`：通道目标，含义随类型而定。
- `channels[].extra`：附加参数，含义随类型而定。

各通道类型的字段约定：

| type | target | extra |
| --- | --- | --- |
| `webhook` | 接收 POST 的 URL | 作为附加 HTTP header 写入请求头 |
| `telegram` | Chat ID | `bot_token`（必填）Bot 令牌 |
| `discord` | Discord Webhook URL | 无 |
| `mastodon` | 实例地址 | `access_token`（必填）；`visibility`（可选，默认 `unlisted`） |

`webhook` 通道发送完整 JSON payload；`telegram`、`discord`、`mastodon` 发送一条人类可读的文本消息。

通知组 API：

```text
NotifyGroupService.ListNotifyGroups    # 列出通知组
NotifyGroupService.CreateNotifyGroup   # 创建通知组
NotifyGroupService.GetNotifyGroup      # 查询单个通知组
NotifyGroupService.UpdateNotifyGroup   # 更新通知组
NotifyGroupService.DeleteNotifyGroup   # 删除通知组
```

Webhook payload 示例：

```json
{
  "monitor_id": "mon_...",
  "monitor_name": "api-url-health",
  "server_id": "srv_...",
  "previous_status": "Healthy",
  "current_status": "Down",
  "occurred_at": "2026-05-30T08:12:34Z",
  "group_id": "grp_...",
  "group_name": "prod-edge"
}
```

未来增强：

- 更多 channel 类型（Slack、Email、PagerDuty）。
- 持久化告警历史到 `alert_events` collection。
- 分组级权限控制。
- 分组级 HealthStatus 聚合。

## MongoDB 存储

**状态：** 已实现（基础配置与查询 collection）

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

## S3 历史归档

**状态：** 已实现（可选）

除了写入 MongoDB，探测结果还可以**额外**归档到 S3 或任意 S3 兼容对象存储（如 MinIO），用于长期历史记录。MongoDB 仍是唯一权威来源，S3 归档是尽力而为的旁路副本。

运行行为：

- 仅当 `archive.client_key` 指向 Butterfly 配置中的 `store.s3.<key>` 时启用；未配置时为 no-op。
- 调度器在结果成功写入 MongoDB 后，再把该结果交给归档器，**不会**因为 S3 慢或不可用而阻塞、失败主写入路径。
- 结果在内存中缓冲并按批刷写为 NDJSON（每行一条结果）对象，避免每次探测都产生一个小对象。
- 刷写触发：攒够 `archive.batch_size` 条，或经过 `archive.flush_interval_seconds` 秒，二者先到先刷；进程退出时排空缓冲并执行最后一次刷写。
- 内存缓冲有上限（10000 条）。当 S3 持续不可用导致缓冲写满时，丢弃最旧的结果并记录告警，保证内存不会无限增长。
- 刷写失败只记录日志，下一周期继续尝试。

对象布局（按 UTC 时间分区）：

```text
<prefix>/YYYY/MM/DD/HH/<unix_millis>-<count>-<rand>.jsonl
```

配置示例（Butterfly 配置文件）：

```yaml
store:
  s3:
    archive:
      endpoint: "s3.amazonaws.com"
      region: "us-east-1"
      bucket: "neo-line-archive"
      access_key_id: "AKIAIOSFODNN7EXAMPLE"
      secret_access_key: "wJalrXUtnFEMI/K7MDENG"
      use_ssl: true
      use_path_style: false

archive:
  client_key: "archive"
  prefix: "monitor_results"
  batch_size: 100
  flush_interval_seconds: 60
```

配置项说明：

- `store.s3.<key>`：Butterfly 的 S3 客户端配置，详见 [Butterfly S3 文档](https://butterfly.orz.ee/stores/s3.html)。支持 AWS S3 和任意 S3 兼容存储（MinIO 等，需 `use_path_style: true`）。
- `archive.client_key`：选择上面 `store.s3.<key>` 中的客户端；为空时不启用归档。
- `archive.prefix`：对象 key 前缀，默认 `monitor_results`。
- `archive.batch_size`：单批最大结果数，默认 `100`。
- `archive.flush_interval_seconds`：刷写间隔秒数，默认 `60`。

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
