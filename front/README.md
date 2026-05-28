# neo-line 前端

neo-line 监控服务的管理面板，基于 React + Vite + Tailwind v4 + shadcn/ui 构建，消费后端 `/v1` REST API。

## 开发

```bash
pnpm install
pnpm dev
```

开发服务器默认运行在 `http://localhost:5173`，并将 `/v1` 与 `/ping` 代理到后端。后端地址可通过环境变量覆盖：

```bash
VITE_API_TARGET=http://localhost:8080 pnpm dev
```

## 构建

```bash
pnpm build      # 类型检查 + 生产构建到 dist/
pnpm preview    # 本地预览构建产物
```

## 功能

- 邮箱 / 密码登录，Bearer Token 存于 localStorage
- 服务器列表：健康状态汇总、搜索、增删改
- 服务器详情：健康概览、监控项管理、状态变更事件
- 监控项详情：配置展示、TLS 证书信息、检查历史（每 30 秒自动刷新）
- 支持 `tcp` / `url` / `tls_port` 三种监控类型，表单按类型动态展示字段

读取接口为公开访问，写操作（增删改、登录态相关）需要管理员登录。
