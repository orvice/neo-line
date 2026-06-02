# 监控配置

本文档描述 neo-line 规划中的监控配置模型。

目标是允许用户为一台 server 绑定一个或多个 monitor。每个 monitor 描述一个具体的网络检查，例如 TCP 端口探测、URL 探测或 TLS Port 探测。

所有监控业务配置都从 MongoDB 读取。本文档中的 YAML 片段用于表达字段结构，不代表本地配置文件格式。

## 配置来源

MongoDB 是 neo-line 监控业务配置的唯一权威来源。

当前服务启动配置通过 Butterfly 统一加载。MongoDB 连接放在 `store.mongo` 下，登录 token 会话使用 `store.redis` 下的 Redis client：

```yaml
store:
  mongo:
    primary:
      uri: "mongodb://localhost:27017"
  redis:
    session:
      addr: "localhost:6379"
      password: ""
      db: 0

mongo:
  client_key: "primary"
  database: "neo_line"

redis:
  session_client_key: "session"
```

- `store.mongo.<key>.uri`：Butterfly MongoDB store 配置，框架会按 key 初始化 client。
- `mongo.client_key`：neo-line 使用的 Butterfly Mongo client key，默认 `primary`。
- `mongo.database`：MongoDB 数据库名，默认 `neo_line`。
- `store.redis.<key>`：Butterfly Redis store 配置，框架会按 key 初始化 client。
- `redis.session_client_key`：neo-line 用于存储 Bearer token 的 Redis client key，默认 `session`。

### 全局 SSH 配置（可选）

SSH 远程执行能力的全局配置放在运行时配置的 `ssh` 下。这是一个 bootstrap/运维级配置，与监控业务配置不同：私钥是绑定到运行 neo-line 的主机文件系统的本地资源，因此**不存入 MongoDB**，只在此处指定路径。

```yaml
ssh:
  key_path: "/etc/neo-line/id_ed25519"
  user: "root"
  port: 22
  known_hosts_path: "/etc/neo-line/known_hosts"
```

- `ssh.key_path`：本地私钥路径，是所有 SSH 连接的唯一私钥来源。为空时不启用 SSH 相关 MCP 工具。私钥不存在或解析失败会导致服务启动报错。
- `ssh.user`：默认 SSH 用户，server 未覆盖时使用，默认 `root`。
- `ssh.port`：默认 SSH 端口，server 未覆盖时使用，默认 `22`。
- `ssh.known_hosts_path`：known_hosts 文件路径，配置后按其校验主机密钥；为空时不校验主机密钥（仅适用于受信任内网）。

每台 server 通过 `servers` 文档中的 `ssh` 子文档启用并覆盖连接目标，详见下文「Server」。

配置读写规则：

- Server、monitor、阈值、启停状态和探测参数都存储在 MongoDB。
- API 创建、更新或删除配置时，写入 MongoDB。
- 调度器从 MongoDB 读取启用状态为 `true` 的 server 和 monitor。
- 探测 worker 执行时使用 MongoDB 中当前有效的 monitor 配置。
- TCP 和 TLS Port monitor 的 `host` 为空时，运行时探测目标会回落到所属 server 的 `host`。该回落只影响运行时探测配置，不会把回落值写回 monitor 文档。
- 探测结果、当前状态、状态变化事件和证书信息写回 MongoDB。
- 本地文件不作为 server / monitor 配置来源。

初始 collection 建议：

- `servers`
- `monitors`
- `monitor_groups`
- `monitor_results`
- `server_events`
- `audit_logs`
- `tls_certificates`
- `alert_events`

## Server

Server 是主要的被监控资源。Server 列表按 `sort_order ASC, created_at DESC` 返回，`sort_order` 越小越靠前。

MongoDB document 字段示例：

```yaml
id: srv_01
name: production-api-01
host: 10.0.0.10
environment: production
region: ap-east-1
tags:
  - api
  - production
sort_order: 10
enabled: true
ssh:
  enabled: true
  host: 10.0.0.12
  port: 22
  user: ops
```

字段说明：

- `id`：server 唯一标识；创建时如果未提供，会自动生成 `srv_<uuid>`。
- `name`：显示名称
- `host`：默认 host，可以是 hostname 或 IP
- `environment`：环境，例如 production、staging
- `region`：区域或数据中心
- `tags`：标签
- `sort_order`：展示排序值，默认 `0`；启动时会为历史 server 补齐缺失的 `sort_order: 0`
- `enabled`：是否启用该 server 的监控
- `ssh`：可选的 SSH 连接子文档，用于 SSH 远程执行（私钥来自全局 `ssh.key_path`，此处不保存密钥）：
  - `ssh.enabled`：是否允许对该 server 执行 SSH 工具，默认 `false`
  - `ssh.host`：SSH 连接地址，为空时回落到 server 的 `host`
  - `ssh.port`：SSH 端口，为空时回落到全局 `ssh.port`（默认 `22`）
  - `ssh.user`：SSH 用户，为空时回落到全局 `ssh.user`（默认 `root`）
- `health_status`：当前聚合健康状态，创建时默认 `Unknown`
- `last_status_change_at`：最近一次状态变化时间
- `last_check_at`：最近一次关联 monitor 检查时间
- `created_at` / `updated_at`：创建和更新时间

## Monitor 通用字段

所有 monitor 类型应共享一组基础配置字段。

Monitor 存储在 MongoDB `monitors` collection 中，通过 `server_id` 关联到 `servers` collection。

```yaml
id: mon_01
server_id: srv_01
name: api-url
kind: url
enabled: true
interval_seconds: 60
timeout_seconds: 5
retries: 3
```

字段说明：

- `id`：monitor 唯一标识；创建时如果未提供，会自动生成 `mon_<uuid>`。
- `server_id`：关联的 server ID，由 URL 路径中的 server ID 写入
- `group_ids`：所属分组 ID 列表，可选；每个 ID 必须在 `monitor_groups` 中存在，否则写入返回 `400`
- `name`：monitor 显示名称
- `kind`：monitor 类型，可选值为 `tcp`、`url`、`tls_port`。`tls_port` 是 TLS 证书端口探测的规范值；运行时会兼容历史数据中的 `tls` 和 `tls_certificate`，按 `tls_port` 同等处理，但新建配置应继续写入 `tls_port`
- `enabled`：是否启用该 monitor
- `interval_seconds`：检查间隔，默认 `60`
- `timeout_seconds`：单次检查超时时间，默认 `5`
- `retries`：标记为异常前允许的重试次数，默认 `3`
- `status`：monitor 当前状态，创建时默认 `Unknown`
- `certificate`：最近一次探测读取到的证书信息，仅 TLS 类 monitor 写入；探测到证书时随 monitor 状态一并更新，用于在列表和详情页展示证书到期时间，无需扫描历史 `monitor_results`
- `created_at` / `updated_at`：创建和更新时间

## TCP 端口探测

TCP 端口探测用于检查目标端口是否接受 TCP 连接。

MongoDB document 字段示例：

```yaml
kind: tcp
host: 10.0.0.10
port: 22
timeout_seconds: 3
```

如果 `host` 留空，运行时使用该 monitor 所属 server 的 `host`：

```yaml
kind: tcp
port: 22
timeout_seconds: 3
```

字段说明：

- `host`：目标 hostname 或 IP；为空时回落到所属 server 的 `host`
- `port`：目标 TCP 端口
- `timeout_seconds`：连接超时时间

健康条件：

- 在超时时间内成功建立 TCP 连接。

异常条件：

- 连接被拒绝
- 连接超时
- DNS 解析失败
- 网络不可达

检查结果需要记录：

- 成功或失败
- 连接延迟
- 错误信息
- 检查时间

## URL 探测

URL 探测用于向指定 HTTP 或 HTTPS URL 发送请求，并判断响应是否符合预期。

HTTP 和 HTTPS 不拆分为两个 monitor 类型，统一使用 `kind: url`，由 `url` 字段中的 scheme 决定协议。

MongoDB document 字段示例：

```yaml
kind: url
url: https://api.example.com/health
method: GET
headers:
  User-Agent: neo-line-monitor
expected_status_codes: "200-299,301,302"
timeout_seconds: 5
tls_verify: true
sni_name: api.example.com
```

字段说明：

- `url`：目标 URL，scheme 支持 `http` 和 `https`
- `method`：HTTP method，默认 `GET`
- `headers`：请求 headers
- `expected_status_codes`：期望状态码表达式，字符串类型。使用逗号分隔多个条目，每个条目可以是单个状态码（如 `200`）或闭区间范围（如 `200-299`）。例如 `"200-299,301,302"`。留空时默认只接受 `200`
- `timeout_seconds`：请求超时时间
- `tls_verify`：是否校验证书，仅适用于 `https` URL
- `sni_name`：自定义 TLS server name，仅适用于 `https` URL

健康条件：

- DNS 解析成功。
- TCP 连接成功。
- 如果 URL scheme 是 `https`，TLS 握手成功。
- 如果 URL scheme 是 `https` 且 `tls_verify` 开启，证书校验成功。
- 请求在超时时间内完成。
- 响应状态码匹配 `expected_status_codes`。

异常条件：

- DNS 解析失败
- TCP 连接失败
- TLS 握手失败
- 证书校验失败
- 请求超时
- HTTP 状态码不符合预期
- 响应无效

检查结果需要记录：

- 成功或失败
- HTTP 状态码
- DNS / TCP / TLS / 总请求延迟
- 错误阶段
- 错误信息
- 检查时间

## TLS Port 探测

TLS Port 探测用于检查目标端口是否可以完成 TLS 握手，并记录证书状态。

该探测不发送 HTTP 请求，也不判断 HTTP 状态码。它适用于 HTTPS 端口、TLS 代理、LDAPS、SMTPS 或其他基于 TLS 的自定义服务。

规范配置使用 `kind: tls_port`。历史 MongoDB 文档中如果存在 `kind: tls` 或 `kind: tls_certificate`，运行时会作为 TLS Port 探测兼容处理，并在探测成功后写回 `certificate` 当前快照。

MongoDB document 字段示例：

```yaml
kind: tls_port
host: 203.0.113.10
port: 443
sni_name: api.example.com
tls_verify: true
warning_days: 30
critical_days: 7
timeout_seconds: 5
```

如果 `host` 留空，运行时使用该 monitor 所属 server 的 `host`：

```yaml
kind: tls_port
port: 443
sni_name: api.example.com
tls_verify: true
```

字段说明：

- `host`：目标 hostname 或 IP；为空时回落到所属 server 的 `host`
- `port`：目标 TLS 端口，默认 `443`
- `sni_name`：自定义 TLS server name
- `tls_verify`：是否校验证书
- `warning_days`：证书剩余有效期低于该天数时进入 Warning
- `critical_days`：证书剩余有效期低于该天数时进入 Critical
- `timeout_seconds`：连接和 TLS 握手超时时间

健康条件：

- DNS 解析成功。
- TCP 连接成功。
- TLS 握手成功。
- 当 `tls_verify` 开启时，证书校验成功。
- 证书有效，且过期时间大于 `warning_days`。

异常条件：

- DNS 解析失败
- TCP 连接失败
- TLS 握手失败
- 证书校验失败
- 证书已过期
- 证书尚未生效
- 证书将在阈值时间内过期

检查结果需要记录：

- 成功或失败
- DNS / TCP / TLS 延迟
- TLS 版本
- Cipher suite
- 证书元数据
- 证书剩余有效天数
- 错误阶段
- 错误信息
- 检查时间

## 自定义 SNI Name

URL 探测和 TLS Port 探测都可以配置 `sni_name`。

SNI，全称 Server Name Indication，会在 TLS 握手阶段发送给服务端。部分服务会根据 SNI 返回不同证书，或者将请求路由到不同后端。

行为规则：

- 如果设置了 `sni_name`，neo-line 应使用该值作为 TLS server name。
- 如果未设置 `sni_name`，且目标 host 是域名，neo-line 应默认使用 host 作为 TLS server name。
- 如果未设置 `sni_name`，且目标 host 是 IP 地址，TLS hostname 校验可能失败，除非证书包含该 IP 或关闭 TLS 校验。

典型 MongoDB document 字段：

```yaml
kind: tls_port
host: 203.0.113.10
port: 443
sni_name: api.example.com
tls_verify: true
```

该配置表示：连接目标是 `203.0.113.10:443`，但 TLS 握手时发送的 SNI name 是 `api.example.com`。

## TLS 证书状态

TLS 证书状态主要由 `tls_port` monitor 负责记录。HTTPS URL 探测也可以记录证书状态，但不应替代 TLS Port 探测的证书模型。

需要记录的证书字段：

- Subject
- Issuer
- DNS names
- Serial number
- Not before
- Not after
- Days remaining

证书信息的存储方式：

- 每次探测都会把读取到的证书信息写入对应的 `monitor_results` document（`certificate` 字段），作为历史记录。
- 同时把最近一次证书信息写回 `monitors` document 的 `certificate` 字段，作为当前快照。前端列表和详情页优先读取该快照展示证书到期时间（Not after）和剩余天数，避免扫描历史结果。

默认阈值建议（当前实现作为创建 monitor 时的默认值）：

```yaml
warning_days: 30
critical_days: 7
```

状态规则：

- **Healthy** — 证书有效，且过期时间大于 `warning_days`。
- **Warning** — 证书将在 `warning_days` 天内过期。
- **Critical** — 证书将在 `critical_days` 天内过期。
- **Down** — 证书已过期、尚未生效，或 TLS 握手失败。

## Monitor 分组

Monitor 可以归入零个或多个分组（`MonitorGroup`）。分组是扁平结构，不支持嵌套；同一个 monitor 可以同时属于多个分组。分组用于：

- 在 UI 上跨 server 聚合展示监控项
- 配置分组级别的告警策略，所属任意分组中的策略都会评估

MongoDB collection：`monitor_groups`。`name` 上建立唯一索引；`sort_order` 与 `created_at` 上建立组合索引以支持分组展示排序；`monitors.group_ids` 上建立多键索引以支持按分组列表 monitor。

MongoDB document 字段示例：

```yaml
id: grp_01
name: prod-edge
description: 生产环境边缘节点
sort_order: 10
alert_policy:
  enabled: true
  on_down: true
  on_recover: true
  on_warning: false
  on_critical: true
  min_interval_seconds: 300
  notify_group_ids:
    - ntf_01
    - ntf_02
```

字段说明：

- `id`：分组唯一标识；创建时如果未提供，会自动生成 `grp_<uuid>`。
- `name`：分组名称，全局唯一（重复会返回 `409 Conflict`）
- `description`：可选描述
- `sort_order`：展示排序值，默认 `0`；启动时会为历史分组补齐缺失的 `sort_order: 0`。分组列表按 `sort_order ASC, created_at DESC` 返回，值越小越靠前
- `alert_policy`：分组级告警策略，见下文
- `created_at` / `updated_at`：创建和更新时间

### AlertPolicy 字段

- `enabled`：是否启用告警；为 `false` 时该分组永不派发
- `on_down`：monitor 状态变为 `Down` 时派发
- `on_recover`：非健康状态恢复为 `Healthy` 时派发（首次探测得到 `Healthy` 不算恢复）
- `on_warning`：monitor 状态变为 `Warning` 时派发
- `on_critical`：monitor 状态变为 `Critical` 时派发
- `min_interval_seconds`：同 `(group, monitor)` 维度的派发节流窗口；`0` 或未填表示不节流
- `notify_group_ids`：引用的通知组 ID 列表；派发时解析这些通知组并汇总它们的全部通道。为空时不派发

### 通知组（NotifyGroup）

通知组是一组可复用的通知通道，独立于 monitor 分组存在，多个分组可以共享同一个通知组。

MongoDB collection：`notify_groups`。`name` 上建立唯一索引。删除通知组会从所有分组的 `alert_policy.notify_group_ids` 中 `$pull` 掉该 ID。创建或更新 monitor 分组时校验 `notify_group_ids` 中的每个 ID 是否存在，不存在返回 `400`。

MongoDB document 字段示例：

```yaml
id: ntf_01
name: prod-oncall
description: 生产值班通道
channels:
  - type: webhook
    target: https://hooks.example.com/neo-line
    extra:
      X-Source: neo-line
  - type: telegram
    target: "-1001234567890" # Chat ID
    extra:
      bot_token: "123456:ABC-DEF..."
  - type: discord
    target: https://discord.com/api/webhooks/xxx/yyy
  - type: mastodon
    target: https://mastodon.social
    extra:
      access_token: "应用访问令牌"
      visibility: unlisted # 可选，默认 unlisted
```

字段说明：

- `id`：通知组唯一标识；创建时如果未提供，会自动生成 `ntf_<uuid>`
- `name`：通知组名称，全局唯一（重复会返回 `409 Conflict`）
- `description`：可选描述
- `channels[].type`：通道类型，支持 `webhook`、`telegram`、`discord`、`mastodon`
- `channels[].target`：通道目标，含义随类型而定（见下表）
- `channels[].extra`：附加参数，含义随类型而定（见下表）

各通道类型的字段约定：

| type | target | extra |
| --- | --- | --- |
| `webhook` | 接收 POST 的 URL | 作为附加 HTTP header 写入请求头 |
| `telegram` | Chat ID | `bot_token`（必填）Bot 令牌 |
| `discord` | Discord Webhook URL | 无 |
| `mastodon` | 实例地址，如 `https://mastodon.social` | `access_token`（必填）；`visibility`（可选，默认 `unlisted`） |

`webhook` 通道发送完整 JSON Payload；`telegram`、`discord`、`mastodon` 发送一条人类可读的文本消息（含 monitor 名称、状态变化、分组、server 和时间）。

派发为 best-effort：通道调用失败仅记日志，不阻塞调度器和探测主流程。`webhook` Payload 字段：

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

## 完整配置示例

以下示例展示 MongoDB 中 server document 与 monitor documents 的逻辑关系。

```yaml
servers:
  - id: srv_01
    name: production-api-01
    host: 203.0.113.10
    environment: production
    enabled: true

monitors:
  - id: mon_ssh
    server_id: srv_01
    name: ssh-port
    kind: tcp
    host: 203.0.113.10
    port: 22
    interval_seconds: 60
    timeout_seconds: 3
    retries: 3
    enabled: true

  - id: mon_url
    server_id: srv_01
    group_ids:
      - grp_01
    name: api-url-health
    kind: url
    url: https://api.example.com/health
    method: GET
    headers:
      User-Agent: neo-line-monitor
    tls_verify: true
    expected_status_codes: "200-299"
    interval_seconds: 60
    timeout_seconds: 5
    retries: 3
    enabled: true

  - id: mon_tls
    server_id: srv_01
    name: api-tls-port
    kind: tls_port
    host: 203.0.113.10
    port: 443
    sni_name: api.example.com
    tls_verify: true
    warning_days: 30
    critical_days: 7
    interval_seconds: 3600
    timeout_seconds: 5
    enabled: true
```
