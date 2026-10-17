# 监控配置导入导出设计

日期:2026-10-18
状态:待评审

## 目标

为 neo-line 提供监控配置的导出与导入能力,用于备份、环境迁移和批量编辑。MongoDB 仍是配置的唯一权威来源,导入导出只是读写 MongoDB 的一种方式,不引入任何本地文件作为配置源。

## 范围

导出和导入的数据:

- `settings`(站点设置单例)
- `notify_groups`
- `monitor_groups`
- `servers`
- `monitors`(嵌套在所属 server 下)

不在范围内:

- `monitor_results`、`server_events` 等运行数据
- `users`、`mcp_tokens`、`audit_logs`
- Web 控制台入口和 MCP 工具(首期只做 Connect API)
- "全量替换"式导入,导入从不删除库内已有数据

## 文件格式

单个 JSON 文件,顶层带 `version`。首期 `version` 固定为 `1`。

```json
{
  "version": 1,
  "exported_at": "2026-10-18T00:00:00Z",
  "settings": { "site_name": "neo-line", "status_page_title": "服务状态" },
  "notify_groups": [
    { "id": "ntf_...", "name": "ops", "description": "", "channels": [ { "type": "telegram", "target": "123", "extra": { "bot_token": "..." } } ] }
  ],
  "monitor_groups": [
    { "id": "grp_...", "name": "prod-edge", "sort_order": 0, "alert_policy": { "enabled": true, "notify_group_ids": ["ntf_..."] } }
  ],
  "servers": [
    {
      "id": "srv_...",
      "name": "web-1",
      "host": "10.0.0.1",
      "monitors": [
        { "id": "mon_...", "name": "api", "kind": "url", "group_ids": ["grp_..."] }
      ]
    }
  ]
}
```

规则:

- 导入时保留文件中的原始 ID,因此 monitor 到 group、group 到通知组的引用在迁移后保持有效。
- monitor 嵌套在 server 下,`server_id` 由嵌套关系决定,文件中不单独出现。
- 计算与运行时托管字段不导出,导入时即使出现也忽略,与现有 API 规则一致:
  - server:`health_status`、`last_status_change_at`、`last_check_at`、`created_at`、`updated_at`
  - monitor:`status`、`last_check_at`、`last_status_change_at`、`certificate`、`created_at`、`updated_at`
- 通知组 `channels[].extra` 始终导出明文(含 `bot_token`、`access_token`、webhook header)。导出文件属于敏感文件,需要妥善保管。
- 导入请求体大小上限 10 MiB。

## API

新增 `proto/neoline/v1/data.proto`,package `neoline.v1`,生成代码按 Buf 配置输出到 `pkg/proto`。

```text
DataService.ExportConfig(ExportConfigRequest) -> ExportConfigResponse{ bytes data }
DataService.ImportConfig(ImportConfigRequest{ bytes data, ImportMode mode, bool dry_run })
    -> ImportConfigResponse{ dry_run, per-resource-type summary, repeated ImportError errors }
```

- `ImportMode`:`MERGE`(已存在则跳过)、`UPSERT`(已存在则覆盖);`UNSPECIFIED` 视为非法请求。
- summary 按资源类型(settings / notify_groups / monitor_groups / servers / monitors)给出 `created`、`updated`、`skipped`、`failed` 计数。
- `ImportError`:`resource_type`、`id`、`name`、`message`。

### 鉴权与审计

- `DataService` 整体要求 `admin` 角色。`internal/connectapi/interceptors.go` 当前仅按方法名前缀 `Create/Update/Delete` 判定写操作,`ExportConfig` / `ImportConfig` 不命中,因此需要像 `SshService`、`McpTokenService` 一样按服务名加入 admin 判定。Export 会返回通知渠道明文密钥,同样必须是 admin。
- 在审计拦截器的动作映射中为 `ExportConfig`、`ImportConfig` 补充 action,`resource_type` 为 `config`。审计日志只记录元信息(模式、dry_run、各类计数),不记录文件内容和密钥。

## 组件

新增包 `internal/configio`,只依赖 `store.Store` 接口:

- `Export(ctx, store) ([]byte, error)`:读取全部配置,装配成文件结构并序列化。需要分页遍历列表接口,直到取完。
- `Import(ctx, store, data, opts) (Report, error)`:解析、校验、写入并汇总报告。
- `Report`:各资源类型的计数和错误列表,由 `connectapi` 层转换为 proto 响应。

`internal/connectapi/data_service.go` 负责鉴权后调用 `configio`、转换响应、处理错误码,保持薄层。`Register` 中挂载 `DataService`。

## 导入流程

1. 解析 JSON 并校验 `version`。`version` 大于当前支持版本时返回 `invalid_argument`;JSON 非法或超过大小上限同样返回 `invalid_argument`。
2. 离线预检(不写库):
   - 文件内 ID 重复、名称重复(group、通知组)
   - monitor 的 `kind` 及必需参数不合法
   - 引用的 `group_ids` / `notify_group_ids` 既不在文件内也不在库内
   预检发现的问题计入 `failed`,对应条目不会被写入。
3. 对照库内现状,为每个条目确定动作:新增、更新、跳过、冲突。
4. `dry_run=true` 时到此为止,返回将要发生的各类计数,不写库。
5. 否则按依赖顺序写入:`settings` → `notify_groups` → `monitor_groups` → `servers` → `monitors`。
6. 单项写入失败时记录错误并继续,最后汇总返回。导入不是原子的,不整体回滚,调用方应先用 `dry_run` 预检。

### 冲突与已存在处理

| 情况 | MERGE | UPSERT |
|---|---|---|
| ID 不存在、名称不冲突 | 新增 | 新增 |
| ID 已存在 | 跳过 | 覆盖(保留库内计算字段) |
| 名称与库内其他 ID 的 group / 通知组重复 | 跳过并报告 | 报冲突,不写入 |

不会自动改名或自动合并;文件中没有而库内有的数据一律保留。

写入复用 `Store` 现有的 Create / Update 方法,因此 `group_ids` 与 `notify_group_ids` 的存在性校验、名称唯一约束和计算字段剥离沿用既有行为。由于需要保留原始 ID,`Store` 的创建路径需要支持调用方指定 ID。具体做法在实现计划中确认,优先选择对现有 Create 方法影响最小的方式。

## 错误码

- `invalid_argument`:JSON 非法、版本不支持、超过大小上限、`mode` 未指定。
- `permission_denied`:非 admin。
- `unauthenticated`:未登录。
- 条目级问题不转化为 RPC 错误,而是出现在响应的 `errors` 中。

## 测试

- `internal/configio`:基于内存假 Store 的表驱动测试,覆盖导出再导入的往返一致、MERGE 与 UPSERT 差异、`dry_run` 不写库、引用校验、名称冲突、计算字段被忽略、版本不支持。
- `internal/connectapi`:补充 admin 判定与审计动作映射的用例。

## 文档更新

- `docs/features.md`:新增「配置导入导出」章节并更新 API 列表。
- `docs/monitoring-configuration.md`:补充导入导出文件格式、字段说明和示例。
- `docs/README.md`:如文档索引或产品范围有变化则同步。

## 实施步骤概览

1. 新增 `data.proto`,执行 `buf lint` 与 `buf generate`。
2. 实现 `internal/configio`(导出、预检、导入、报告)及测试。
3. 确认并实现 `Store` 保留 ID 的创建路径。
4. 实现 `data_service.go`,在 `Register` 中挂载。
5. 更新 `interceptors.go` 的 admin 判定与审计映射,补充测试。
6. 更新文档,运行 `go fmt ./...`、`go test ./...`、`go build ./...`。
