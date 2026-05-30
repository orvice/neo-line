# neo-line

neo-line 是一个基于 Go 和 Butterfly 应用框架构建的服务器监控服务，用于管理服务器监控配置，并持续检查服务器暴露的网络服务状态。

MongoDB 是监控业务配置和运行状态的主要数据源；Redis 用于登录后签发的 Bearer token 会话存储。

## 主要能力

- Server / Monitor / Monitor Group 管理
- TCP 端口探测、HTTP/HTTPS URL 探测、TLS Port 握手和证书状态探测
- URL 和 TLS Port 探测支持 TLS 校验配置与自定义 SNI
- Monitor 与 Server 健康状态聚合：`Down > Critical > Warning > Unknown > Healthy`
- 公开状态页、管理后台、站点展示设置
- 可复用通知组（webhook / Telegram / Discord / Mastodon）与分组级告警策略引用
- 可选 S3 / S3 兼容对象存储归档检查结果
- MCP Streamable HTTP 端点：`/api/mcp`

## 项目结构

- `cmd/server/main.go`：后端入口
- `internal/`：HTTP API、探测器、调度器、存储、告警、归档、MCP server
- `front/`：React + Vite + Tailwind v4 + shadcn/ui 前端
- `proto/`：protobuf 定义
- `pkg/proto/`：Buf 生成代码
- `docs/`：中文项目文档

## 运行要求

- Go `1.26.2`
- MongoDB
- Redis
- Node.js + pnpm（前端开发需要，当前前端声明 `pnpm@10.15.1`）
- Buf（仅修改 protobuf 后重新生成时需要）

## 本地启动

准备 MongoDB 和 Redis，然后创建 Butterfly 配置文件，例如 `config.yaml`：

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

log:
  level: "info"
  format: "text"
```

启动后端：

```bash
export BUTTERFLY_CONFIG_TYPE=file
export BUTTERFLY_CONFIG_FILE_PATH=$PWD/config.yaml
export ADMIN_EMAIL=admin@neo-line.local
export ADMIN_PASSWORD=change-me
go run ./cmd/server
```

验证：

```bash
curl http://localhost:8080/ping
```

启动前端：

```bash
cd front
pnpm install
VITE_API_TARGET=http://localhost:8080 pnpm dev
```

前端开发服务器默认运行在 `http://localhost:5173`。

## 常用配置

| 变量 | 说明 |
| --- | --- |
| `BUTTERFLY_CONFIG_TYPE` | 配置来源；本地通常设为 `file` |
| `BUTTERFLY_CONFIG_FILE_PATH` | `BUTTERFLY_CONFIG_TYPE=file` 时的 YAML 配置文件路径 |
| `ADMIN_EMAIL` | 管理员账号邮箱，默认 `admin@neo-line.local` |
| `ADMIN_PASSWORD` | 管理员密码；设置后启动时会创建或轮换管理员密码 |
| `MCP_AUTH_TOKEN` | `/api/mcp` 静态鉴权 token；为空时 MCP 端点不额外校验 |

更多配置项和监控字段见 [监控配置文档](./docs/monitoring-configuration.md)。

## 常用命令

```bash
go fmt ./...
go test ./...
go build ./...
```

```bash
cd front
pnpm build
```

```bash
make proto-lint
make proto
```

## Docker

```bash
docker build -t neo-line .
docker build -t neo-line-front ./front
```

前端镜像通过 Nginx 提供静态资源，并把 `/api/v1`、`/ping` 反向代理到后端。运行时可用 `BACKEND_URL` 覆盖默认后端地址。

## 文档

- [文档首页](./docs/README.md)
- [功能说明](./docs/features.md)
- [监控配置](./docs/monitoring-configuration.md)
