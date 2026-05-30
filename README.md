# neo-line

neo-line 是一个基于 Go 和 Butterfly 应用框架构建的服务器监控服务，用于管理服务器监控配置，并持续检查服务器暴露的网络服务状态。

项目当前以 MongoDB 作为监控业务配置和运行状态的唯一数据源，已覆盖 TCP 端口探测、HTTP/HTTPS URL 探测、TLS Port 握手与证书状态探测，以及服务器健康状态聚合。

## 当前开发进度

### 后端服务

- 已接入 Butterfly 应用框架和 Gin HTTP Router。
- 已接入 MongoDB，当前代码读写 `servers`、`monitors`、`monitor_results`、`server_events`、`users`、`sessions` collections。
- 已提供 `/ping` 存活检查接口。
- 已实现管理员账号初始化、登录、Bearer Token 会话和登出。
- 已实现 Server CRUD、Monitor CRUD、健康状态查询、状态事件查询、检查结果查询和 uptime 汇总查询。
- 已实现后台调度器，每 `5s` 从 MongoDB 重新读取已启用的 server / monitor，并按 monitor 的 `interval_seconds` 调度探测。
- 已实现探测并持久化结果：`tcp`、`url`、`tls_port`。
- 已实现 MCP Streamable HTTP 只读工具端点 `/mcp`，可查询 server、monitor、健康状态、事件和检查结果。

### 探测能力

- `tcp`：检查目标 `host:port` 是否可以建立 TCP 连接。
- `url`：统一支持 HTTP 和 HTTPS URL，支持 method、headers、期望状态码、超时、重试、HTTPS TLS 校验和自定义 `sni_name`。
- `tls_port`：连接目标 TLS 端口并执行握手，不发送 HTTP 请求；支持证书校验、自定义 SNI、证书元数据记录，以及证书过期 Warning / Critical 阈值。

当前健康状态聚合顺序为：

```text
Down > Critical > Warning > Unknown > Healthy
```

### 前端管理台

`front/` 目录已包含 React + Vite + Tailwind v4 + shadcn/ui 管理界面：

- 邮箱 / 密码登录。
- 服务器列表、搜索、增删改。
- 服务器详情、健康概览、状态变更事件。
- 监控项管理。
- 监控项详情、TLS 证书信息、检查历史和 uptime heartbeat。
- 支持 `tcp`、`url`、`tls_port` 三种监控类型的动态表单。

### Protobuf

- `proto/` 下已有 `neoline.v1` protobuf 定义。
- `pkg/proto` 下已有 Buf 生成的 Go、gRPC 和 grpc-gateway 代码。
- `Makefile` 提供 `proto`、`proto-lint`、`proto-format`、`proto-breaking`、`proto-deps` 命令。

## 运行要求

- Go `1.26.2`
- MongoDB
- 前端开发需要 Node.js 和 pnpm，当前前端声明 `pnpm@10.15.1`
- 如需重新生成 protobuf，需要安装 Buf

## 启动后端

先准备 MongoDB，然后设置最小运行环境：

```bash
export MONGODB_URI=mongodb://localhost:27017
export MONGODB_DATABASE=neo_line
export ADMIN_EMAIL=admin@neo-line.local
export ADMIN_PASSWORD=change-me
go run ./cmd/server
```

默认 HTTP 服务端口由 Butterfly 提供，当前文档约定为 `8080`。启动后可以验证：

```bash
curl http://localhost:8080/ping
```

期望响应：

```json
{"message":"pong"}
```

如果未设置 `ADMIN_PASSWORD`，服务会跳过管理员账号初始化；这适合只读调试，但无法使用需要登录的写接口。

## 启动前端

```bash
cd front
pnpm install
VITE_API_TARGET=http://localhost:8080 pnpm dev
```

前端开发服务器默认运行在 `http://localhost:5173`，并把 `/v1` 与 `/ping` 代理到后端。

生产构建：

```bash
cd front
pnpm build
```

## 主要环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `MONGODB_URI` | `mongodb://localhost:27017` | MongoDB 连接字符串 |
| `MONGODB_DATABASE` | `neo_line` | MongoDB 数据库名 |
| `ADMIN_EMAIL` | `admin@neo-line.local` | 管理员账号邮箱 |
| `ADMIN_PASSWORD` | 无 | 管理员密码；设置后启动时会创建或轮换管理员密码 |
| `MCP_AUTH_TOKEN` | 无 | `/mcp` 静态鉴权 token；为空时 MCP 端点不额外校验 |

## 主要 HTTP 接口

公开读接口：

- `GET /ping`
- `POST /v1/auth/login`
- `GET /v1/servers`
- `GET /v1/servers/:id`
- `GET /v1/servers/:id/health`
- `GET /v1/servers/:id/events`
- `GET /v1/servers/:id/monitors`
- `GET /v1/servers/:id/monitors/:monitor_id`
- `GET /v1/servers/:id/monitors/:monitor_id/results`
- `GET /v1/servers/:id/monitors/:monitor_id/uptime`

需要 Bearer Token 的管理接口：

- `GET /v1/auth/me`
- `POST /v1/auth/logout`
- `POST /v1/servers`
- `PUT /v1/servers/:id`
- `DELETE /v1/servers/:id`
- `POST /v1/servers/:id/monitors`
- `PUT /v1/servers/:id/monitors/:monitor_id`
- `DELETE /v1/servers/:id/monitors/:monitor_id`

登录示例：

```bash
curl -s http://localhost:8080/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@neo-line.local","password":"change-me"}'
```

## 配置模型示例

创建 server：

```json
{
  "name": "production-api-01",
  "host": "10.0.0.10",
  "environment": "production",
  "region": "ap-east-1",
  "tags": ["api", "production"],
  "enabled": true
}
```

创建 URL monitor：

```json
{
  "name": "api-health",
  "kind": "url",
  "enabled": true,
  "url": "https://api.example.com/health",
  "method": "GET",
  "expected_status_codes": "200-299,301,302",
  "timeout_seconds": 5,
  "interval_seconds": 60,
  "retries": 3,
  "tls_verify": true,
  "sni_name": "api.example.com"
}
```

创建 TLS Port monitor：

```json
{
  "name": "api-cert",
  "kind": "tls_port",
  "enabled": true,
  "host": "203.0.113.10",
  "port": 443,
  "sni_name": "api.example.com",
  "tls_verify": true,
  "warning_days": 30,
  "critical_days": 7,
  "timeout_seconds": 5,
  "interval_seconds": 60,
  "retries": 3
}
```

## 验证命令

后端：

```bash
go fmt ./...
go test ./...
go build ./...
```

前端：

```bash
cd front
pnpm build
```

Protobuf：

```bash
make proto-lint
make proto
```

## Docker

后端镜像：

```bash
docker build -t neo-line .
```

前端镜像：

```bash
docker build -t neo-line-front ./front
```

前端镜像通过 Nginx 提供静态资源，并把 `/v1`、`/ping` 反向代理到后端。运行时可用 `BACKEND_URL` 覆盖默认后端地址：

```bash
docker run -e BACKEND_URL=http://neo-line:8080 -p 8081:80 neo-line-front
```

最小 Docker Compose 示例：

```yaml
services:
  mongodb:
    image: mongo:8
    restart: unless-stopped
    volumes:
      - neo-line-mongodb:/data/db

  neo-line:
    build: .
    restart: unless-stopped
    depends_on:
      - mongodb
    environment:
      MONGODB_URI: mongodb://mongodb:27017
      MONGODB_DATABASE: neo_line
      ADMIN_EMAIL: admin@neo-line.local
      ADMIN_PASSWORD: change-me
    ports:
      - "8080:8080"

  neo-line-front:
    build: ./front
    restart: unless-stopped
    depends_on:
      - neo-line
    environment:
      BACKEND_URL: http://neo-line:8080
    ports:
      - "8081:80"

volumes:
  neo-line-mongodb:
```

启动：

```bash
docker compose up --build
```

启动后：

- 后端 API：`http://localhost:8080`
- 前端管理台：`http://localhost:8081`
- 默认管理员：`admin@neo-line.local` / `change-me`

部署时应把 `ADMIN_PASSWORD` 改为实际密码；服务每次启动都会按该环境变量创建或轮换管理员密码。

## 文档

更多项目说明见：

- [docs/README.md](./docs/README.md)
- [docs/features.md](./docs/features.md)
- [docs/monitoring-configuration.md](./docs/monitoring-configuration.md)
