# 配置导入导出 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 提供 Admin 专用的 Connect API `DataService.ExportConfig` / `ImportConfig`,以单个 JSON 文件导出和导入监控配置(settings、notify_groups、monitor_groups、servers 及其 monitors)。

**Architecture:** 新增 `internal/configio` 包,只依赖一个窄的 `Store` 接口(由 `store.Store` 满足),负责文件模型、导出、预检与导入、报告。`internal/connectapi/data_service.go` 是薄层,负责 proto 转换与错误码。导入不是原子的,用逐项报告和 `dry_run` 兜底。

**Tech Stack:** Go、Connect(`connectrpc.com/connect`)、Buf v2(远程插件,需要联网)、MongoDB(经 `store.Store`)、标准库 `encoding/json`。

设计文档:`docs/superpowers/specs/2026-10-18-config-import-export-design.md`

## Global Constraints

- 文件为单个 JSON,顶层 `version` 固定为 `1`;`version` 不等于 `1` 一律拒绝。
- 范围只含 `settings`、`notify_groups`、`monitor_groups`、`servers`、`monitors`;不含 `monitor_results`、`server_events`、`users`、`mcp_tokens`、`audit_logs`。
- monitor 嵌套在所属 server 下;文件中不出现 `server_id`。
- 导入保留文件中的原始 ID;`notify_groups`、`monitor_groups`、`servers`、`monitors` 的 `id` 和 `name` 必填。
- 计算与运行时托管字段不导出、导入时忽略:server 的 `health_status`、`last_status_change_at`、`last_check_at`、`created_at`、`updated_at`;monitor 的 `status`、`last_check_at`、`last_status_change_at`、`certificate`、`created_at`、`updated_at`。
- 通知组 `channels[].extra` 始终导出明文。
- 导入模式:`MERGE`(已存在则跳过)、`UPSERT`(已存在则覆盖);`UNSPECIFIED` 非法。`settings` 是单例,`MERGE` 视为已存在并跳过,`UPSERT` 覆盖。
- 同名但 ID 不同的 group / 通知组:`MERGE` 跳过并报告,`UPSERT` 报冲突(计入 failed)。
- 导入从不删除库内数据。
- 导入请求体解码后上限 `10 MiB`(`10 << 20` 字节)。
- `DataService` 整体要求 `admin` 角色;审计 action 为 `export` / `import`,`resource_type` 为 `config`,不记录文件内容。
- 文档使用中文;每次改动功能同步更新 `docs/features.md`、`docs/monitoring-configuration.md`、`docs/README.md`。
- 完成前运行 `gofmt`、`go test ./...`、`go build ./...`;不要手改 `pkg/proto` 下的生成文件。

## File Structure

| 文件 | 职责 |
|---|---|
| `proto/neoline/v1/data.proto`(新) | `DataService` 的消息与服务定义 |
| `pkg/proto/neoline/v1/data.pb.go`、`neolinev1connect/data.connect.go`(生成) | `buf generate` 产物 |
| `internal/configio/file.go`(新) | 文件 DTO、与 `store` 类型互转、`decode`、常量与 `ErrInvalidInput` |
| `internal/configio/backend.go`(新) | 窄 `Store` 接口与 `listAll` 分页辅助 |
| `internal/configio/export.go`(新) | `Export` |
| `internal/configio/import.go`(新) | `Import`、预检、写入、报告类型 |
| `internal/configio/fake_test.go`(新) | 测试用内存 fake |
| `internal/configio/export_test.go`、`import_test.go`(新) | `configio` 测试 |
| `internal/connectapi/data_service.go`(新) | `ExportConfig` / `ImportConfig` handler 与转换 |
| `internal/connectapi/data_service_test.go`(新) | 鉴权、参数错误与转换测试 |
| `internal/connectapi/interceptors.go`(改) | admin 判定、审计 action 与 resource type |
| `internal/connectapi/interceptors_test.go`(新) | 拦截器映射测试 |
| `internal/connectapi/service.go`(改) | 在 `Register` 挂载 `DataService` |
| `docs/features.md`、`docs/monitoring-configuration.md`、`docs/README.md`(改) | 文档 |

执行前先建分支:`git switch -c feat/config-import-export`。

---

### Task 1: 定义 proto 并生成代码

**Files:**
- Create: `proto/neoline/v1/data.proto`
- Generated: `pkg/proto/neoline/v1/data.pb.go`、`pkg/proto/neoline/v1/neolinev1connect/data.connect.go`

**Interfaces:**
- Produces(Go,package `neolinev1`):`pb.ExportConfigRequest`、`pb.ExportConfigResponse{Data []byte}`、`pb.ImportMode`(`pb.ImportMode_IMPORT_MODE_UNSPECIFIED/MERGE/UPSERT`)、`pb.ImportConfigRequest{Data []byte; Mode ImportMode; DryRun bool}`、`pb.ImportCounts{Created, Updated, Skipped, Failed uint32}`、`pb.ImportSummary{Settings, NotifyGroups, MonitorGroups, Servers, Monitors *ImportCounts}`、`pb.ImportError{ResourceType, Id, Name, Message string}`、`pb.ImportConfigResponse{DryRun bool; Summary *ImportSummary; Errors []*ImportError}`
- Produces(connect):`neolinev1connect.DataServiceHandler`、`NewDataServiceHandler`、`NewDataServiceClient`、`DataServiceExportConfigProcedure`、`DataServiceImportConfigProcedure`

- [ ] **Step 1: 新建 `proto/neoline/v1/data.proto`**

```proto
syntax = "proto3";

package neoline.v1;

option go_package = "github.com/orvice/neo-line/pkg/proto/neoline/v1;neolinev1";

// ImportMode controls how ImportConfig treats resources that already exist.
enum ImportMode {
  IMPORT_MODE_UNSPECIFIED = 0;
  // IMPORT_MODE_MERGE only creates missing resources; existing ones are skipped.
  IMPORT_MODE_MERGE = 1;
  // IMPORT_MODE_UPSERT creates missing resources and overwrites existing ones.
  IMPORT_MODE_UPSERT = 2;
}

message ExportConfigRequest {}

message ExportConfigResponse {
  // data is a JSON document (version 1) containing all monitoring configuration.
  bytes data = 1;
}

message ImportConfigRequest {
  // data is a JSON document previously produced by ExportConfig.
  bytes data = 1;
  ImportMode mode = 2;
  // dry_run validates and counts what would change without writing anything.
  bool dry_run = 3;
}

message ImportCounts {
  uint32 created = 1;
  uint32 updated = 2;
  uint32 skipped = 3;
  uint32 failed = 4;
}

message ImportSummary {
  ImportCounts settings = 1;
  ImportCounts notify_groups = 2;
  ImportCounts monitor_groups = 3;
  ImportCounts servers = 4;
  ImportCounts monitors = 5;
}

// ImportError describes one failed item, or one item skipped because of a
// name conflict.
message ImportError {
  string resource_type = 1;
  string id = 2;
  string name = 3;
  string message = 4;
}

message ImportConfigResponse {
  bool dry_run = 1;
  ImportSummary summary = 2;
  repeated ImportError errors = 3;
}

// DataService exports and imports monitoring configuration. Admin only.
service DataService {
  rpc ExportConfig(ExportConfigRequest) returns (ExportConfigResponse) {}
  rpc ImportConfig(ImportConfigRequest) returns (ImportConfigResponse) {}
}
```

- [ ] **Step 2: Lint 与生成**

Run: `buf lint && buf generate`
Expected: 无输出(lint 通过);`pkg/proto/neoline/v1/data.pb.go` 与 `pkg/proto/neoline/v1/neolinev1connect/data.connect.go` 出现。`buf.gen.yaml` 使用远程插件,需要联网。若失败,不要手写生成文件,先排查网络或 `buf` 登录状态。

- [ ] **Step 3: 确认只新增了预期文件**

Run: `git status --short`
Expected: 新增 `proto/neoline/v1/data.proto`、两个 `data` 生成文件,其他已跟踪的生成文件不应有变化(`clean: true` 会重建目录,若其他文件出现 diff,检查 `buf` 版本与远程插件版本是否与此前一致,有差异则 `git checkout -- <文件>` 还原无关改动)。

- [ ] **Step 4: 编译确认**

Run: `go build ./...`
Expected: 成功,无输出。

- [ ] **Step 5: Commit**

```bash
git add proto/neoline/v1/data.proto pkg/proto/neoline/v1
git commit -m "feat: add DataService proto for config import/export"
```

---

### Task 2: configio 文件模型与导出

**Files:**
- Create: `internal/configio/file.go`
- Create: `internal/configio/backend.go`
- Create: `internal/configio/export.go`
- Create: `internal/configio/fake_test.go`
- Test: `internal/configio/export_test.go`

**Interfaces:**
- Consumes: `store` 包的 `Server`、`Monitor`、`NotifyGroup`、`MonitorGroup`、`AlertPolicy`、`AlertChannel`、`ServerSSH`、`Settings`。
- Produces:
  - `const Version = 1`、`const MaxImportBytes = 10 << 20`、`var ErrInvalidInput error`
  - 资源常量 `ResourceSettings = "settings"`、`ResourceNotifyGroup = "notify_group"`、`ResourceMonitorGroup = "monitor_group"`、`ResourceServer = "server"`、`ResourceMonitor = "monitor"`
  - 类型 `File`、`Settings`、`NotifyGroup`、`Channel`、`MonitorGroup`、`AlertPolicy`、`Server`、`SSH`、`Monitor`
  - `func decode(data []byte) (File, error)`
  - 转换:`func (s Settings) toStore() store.Settings`、`func (g NotifyGroup) toStore() store.NotifyGroup`、`func (g MonitorGroup) toStore() store.MonitorGroup`、`func (s Server) toStore() store.Server`、`func (m Monitor) toStore(serverID string) store.Monitor`
  - `type Store interface{...}`(见下)、`func listAll[T any](fetch func(limit int64, token string) ([]T, string, error)) ([]T, error)`
  - `func Export(ctx context.Context, st Store, now time.Time) ([]byte, error)`
  - 测试辅助:`*fakeStore`(实现 `Store`,每页 2 条,`writes` 计数,`failIDs` 注入写入错误)

- [ ] **Step 1: 写 `internal/configio/file.go`**

```go
// Package configio exports and imports neo-line monitoring configuration as a
// single versioned JSON document. It depends only on the narrow Store
// interface so it can be tested without MongoDB.
package configio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

const (
	// Version is the only file format version this package reads and writes.
	Version = 1
	// MaxImportBytes bounds the size of an import document.
	MaxImportBytes = 10 << 20
)

// Resource types used in import reports; they match the audit resource types.
const (
	ResourceSettings     = "settings"
	ResourceNotifyGroup  = "notify_group"
	ResourceMonitorGroup = "monitor_group"
	ResourceServer       = "server"
	ResourceMonitor      = "monitor"
)

// ErrInvalidInput marks problems with the caller's request or file (bad JSON,
// unsupported version, missing mode, oversize file). Item-level problems are
// reported in Report.Errors instead.
var ErrInvalidInput = errors.New("invalid import input")

// File is the on-disk document. Monitors are nested under their server.
type File struct {
	Version       int            `json:"version"`
	ExportedAt    time.Time      `json:"exported_at"`
	Settings      *Settings      `json:"settings,omitempty"`
	NotifyGroups  []NotifyGroup  `json:"notify_groups,omitempty"`
	MonitorGroups []MonitorGroup `json:"monitor_groups,omitempty"`
	Servers       []Server       `json:"servers,omitempty"`
}

type Settings struct {
	SiteName        string `json:"site_name"`
	StatusPageTitle string `json:"status_page_title"`
}

type Channel struct {
	Type   string            `json:"type"`
	Target string            `json:"target"`
	Extra  map[string]string `json:"extra,omitempty"`
}

type NotifyGroup struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Channels    []Channel `json:"channels,omitempty"`
}

type AlertPolicy struct {
	Enabled            bool     `json:"enabled"`
	NotifyGroupIDs     []string `json:"notify_group_ids,omitempty"`
	OnDown             bool     `json:"on_down"`
	OnRecover          bool     `json:"on_recover"`
	OnWarning          bool     `json:"on_warning"`
	OnCritical         bool     `json:"on_critical"`
	MinIntervalSeconds uint32   `json:"min_interval_seconds,omitempty"`
}

type MonitorGroup struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	SortOrder   uint32      `json:"sort_order"`
	AlertPolicy AlertPolicy `json:"alert_policy"`
}

type SSH struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host,omitempty"`
	Port    uint32 `json:"port,omitempty"`
	User    string `json:"user,omitempty"`
}

type Server struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Host        string    `json:"host"`
	Environment string    `json:"environment,omitempty"`
	Region      string    `json:"region,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	SortOrder   uint32    `json:"sort_order"`
	Enabled     bool      `json:"enabled"`
	SSH         *SSH      `json:"ssh,omitempty"`
	Monitors    []Monitor `json:"monitors,omitempty"`
}

type Monitor struct {
	ID                  string            `json:"id"`
	GroupIDs            []string          `json:"group_ids,omitempty"`
	Name                string            `json:"name"`
	Kind                string            `json:"kind"`
	Enabled             bool              `json:"enabled"`
	Host                string            `json:"host,omitempty"`
	Port                uint32            `json:"port,omitempty"`
	URL                 string            `json:"url,omitempty"`
	Method              string            `json:"method,omitempty"`
	Path                string            `json:"path,omitempty"`
	Headers             map[string]string `json:"headers,omitempty"`
	ExpectedStatusCodes string            `json:"expected_status_codes,omitempty"`
	TLSVerify           bool              `json:"tls_verify"`
	SNIName             string            `json:"sni_name,omitempty"`
	WarningDays         uint32            `json:"warning_days,omitempty"`
	CriticalDays        uint32            `json:"critical_days,omitempty"`
	IntervalSeconds     uint32            `json:"interval_seconds"`
	TimeoutSeconds      uint32            `json:"timeout_seconds"`
	Retries             uint32            `json:"retries"`
}

// decode parses an import document. The version is checked first so a newer
// file produces a version error rather than an unknown-field error; the full
// decode is strict so typos in field names are rejected instead of silently
// ignored.
func decode(data []byte) (File, error) {
	if len(data) > MaxImportBytes {
		return File{}, fmt.Errorf("%w: file exceeds %d bytes", ErrInvalidInput, MaxImportBytes)
	}
	var head struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return File{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if head.Version != Version {
		return File{}, fmt.Errorf("%w: unsupported version %d, expected %d", ErrInvalidInput, head.Version, Version)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var f File
	if err := dec.Decode(&f); err != nil {
		return File{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return File{}, fmt.Errorf("%w: unexpected data after JSON document", ErrInvalidInput)
	}
	return f, nil
}

func settingsFromStore(s store.Settings) *Settings {
	return &Settings{SiteName: s.SiteName, StatusPageTitle: s.StatusPageTitle}
}

func (s Settings) toStore() store.Settings {
	return store.Settings{SiteName: s.SiteName, StatusPageTitle: s.StatusPageTitle}
}

func notifyGroupFromStore(g store.NotifyGroup) NotifyGroup {
	out := NotifyGroup{ID: g.ID, Name: g.Name, Description: g.Description}
	for _, c := range g.Channels {
		out.Channels = append(out.Channels, Channel{Type: c.Type, Target: c.Target, Extra: c.Extra})
	}
	return out
}

func (g NotifyGroup) toStore() store.NotifyGroup {
	out := store.NotifyGroup{ID: g.ID, Name: g.Name, Description: g.Description}
	for _, c := range g.Channels {
		out.Channels = append(out.Channels, store.AlertChannel{Type: c.Type, Target: c.Target, Extra: c.Extra})
	}
	return out
}

func monitorGroupFromStore(g store.MonitorGroup) MonitorGroup {
	p := g.AlertPolicy
	return MonitorGroup{
		ID: g.ID, Name: g.Name, Description: g.Description, SortOrder: g.SortOrder,
		AlertPolicy: AlertPolicy{
			Enabled: p.Enabled, NotifyGroupIDs: p.NotifyGroupIDs,
			OnDown: p.OnDown, OnRecover: p.OnRecover, OnWarning: p.OnWarning, OnCritical: p.OnCritical,
			MinIntervalSeconds: p.MinIntervalSeconds,
		},
	}
}

func (g MonitorGroup) toStore() store.MonitorGroup {
	p := g.AlertPolicy
	return store.MonitorGroup{
		ID: g.ID, Name: g.Name, Description: g.Description, SortOrder: g.SortOrder,
		AlertPolicy: store.AlertPolicy{
			Enabled: p.Enabled, NotifyGroupIDs: p.NotifyGroupIDs,
			OnDown: p.OnDown, OnRecover: p.OnRecover, OnWarning: p.OnWarning, OnCritical: p.OnCritical,
			MinIntervalSeconds: p.MinIntervalSeconds,
		},
	}
}

func serverFromStore(s store.Server, monitors []store.Monitor) Server {
	out := Server{
		ID: s.ID, Name: s.Name, Host: s.Host, Environment: s.Environment, Region: s.Region,
		Tags: s.Tags, SortOrder: s.SortOrder, Enabled: s.Enabled,
	}
	if s.SSH != nil {
		out.SSH = &SSH{Enabled: s.SSH.Enabled, Host: s.SSH.Host, Port: s.SSH.Port, User: s.SSH.User}
	}
	for _, m := range monitors {
		out.Monitors = append(out.Monitors, monitorFromStore(m))
	}
	return out
}

// toStore converts the server config only; nested monitors are handled
// separately because each needs the (possibly newly created) server ID.
func (s Server) toStore() store.Server {
	out := store.Server{
		ID: s.ID, Name: s.Name, Host: s.Host, Environment: s.Environment, Region: s.Region,
		Tags: s.Tags, SortOrder: s.SortOrder, Enabled: s.Enabled,
	}
	if s.SSH != nil {
		out.SSH = &store.ServerSSH{Enabled: s.SSH.Enabled, Host: s.SSH.Host, Port: s.SSH.Port, User: s.SSH.User}
	}
	return out
}

func monitorFromStore(m store.Monitor) Monitor {
	return Monitor{
		ID: m.ID, GroupIDs: m.GroupIDs, Name: m.Name, Kind: m.Kind, Enabled: m.Enabled,
		Host: m.Host, Port: m.Port, URL: m.URL, Method: m.Method, Path: m.Path, Headers: m.Headers,
		ExpectedStatusCodes: m.ExpectedStatusCodes, TLSVerify: m.TLSVerify, SNIName: m.SNIName,
		WarningDays: m.WarningDays, CriticalDays: m.CriticalDays,
		IntervalSeconds: m.IntervalSeconds, TimeoutSeconds: m.TimeoutSeconds, Retries: m.Retries,
	}
}

func (m Monitor) toStore(serverID string) store.Monitor {
	return store.Monitor{
		ID: m.ID, ServerID: serverID, GroupIDs: m.GroupIDs, Name: m.Name, Kind: m.Kind, Enabled: m.Enabled,
		Host: m.Host, Port: m.Port, URL: m.URL, Method: m.Method, Path: m.Path, Headers: m.Headers,
		ExpectedStatusCodes: m.ExpectedStatusCodes, TLSVerify: m.TLSVerify, SNIName: m.SNIName,
		WarningDays: m.WarningDays, CriticalDays: m.CriticalDays,
		IntervalSeconds: m.IntervalSeconds, TimeoutSeconds: m.TimeoutSeconds, Retries: m.Retries,
	}
}
```

- [ ] **Step 2: 写 `internal/configio/backend.go`**

```go
package configio

import (
	"context"

	"github.com/orvice/neo-line/internal/store"
)

// Store is the subset of store.Store that configio needs. store.Store (and so
// MongoStore) satisfies it; tests use a small in-memory fake.
type Store interface {
	GetSettings(ctx context.Context) (store.Settings, error)
	UpdateSettings(ctx context.Context, settings store.Settings) (store.Settings, error)

	ListNotifyGroups(ctx context.Context, limit int64, pageToken string) ([]store.NotifyGroup, string, error)
	CreateNotifyGroup(ctx context.Context, group store.NotifyGroup) (store.NotifyGroup, error)
	UpdateNotifyGroup(ctx context.Context, id string, group store.NotifyGroup) (store.NotifyGroup, error)

	ListMonitorGroups(ctx context.Context, limit int64, pageToken string) ([]store.MonitorGroup, string, error)
	CreateMonitorGroup(ctx context.Context, group store.MonitorGroup) (store.MonitorGroup, error)
	UpdateMonitorGroup(ctx context.Context, id string, group store.MonitorGroup) (store.MonitorGroup, error)

	ListServers(ctx context.Context, environment string, tags []string, limit int64, pageToken string) ([]store.Server, string, error)
	CreateServer(ctx context.Context, server store.Server) (store.Server, error)
	UpdateServer(ctx context.Context, id string, server store.Server) (store.Server, error)

	ListMonitors(ctx context.Context, serverID string, limit int64, pageToken string) ([]store.Monitor, string, error)
	CreateMonitor(ctx context.Context, serverID string, monitor store.Monitor) (store.Monitor, error)
	UpdateMonitor(ctx context.Context, serverID, monitorID string, monitor store.Monitor) (store.Monitor, error)
}

// Compile-time check that the full store satisfies the narrow interface.
var _ Store = store.Store(nil)

// listAll drains a paginated list endpoint using the maximum page size.
func listAll[T any](fetch func(limit int64, token string) ([]T, string, error)) ([]T, error) {
	var out []T
	token := ""
	for {
		items, next, err := fetch(200, token)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if next == "" {
			return out, nil
		}
		token = next
	}
}
```

- [ ] **Step 3: 写测试 fake `internal/configio/fake_test.go`**

fake 每页只返回 2 条,用来覆盖 `listAll` 的翻页;`failIDs` 按资源 ID 注入写入错误;`writes` 统计成功写入次数,用于断言 dry-run 不写库。

```go
package configio

import (
	"context"
	"fmt"
	"strconv"

	"github.com/orvice/neo-line/internal/store"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const fakePageSize = 2

type fakeStore struct {
	settings store.Settings
	notify   []store.NotifyGroup
	groups   []store.MonitorGroup
	servers  []store.Server
	monitors []store.Monitor

	writes  int
	failIDs map[string]error
}

var _ Store = (*fakeStore)(nil)

func (f *fakeStore) fail(id string) error { return f.failIDs[id] }

func pageOf[T any](items []T, token string) ([]T, string, error) {
	start := 0
	if token != "" {
		n, err := strconv.Atoi(token)
		if err != nil {
			return nil, "", err
		}
		start = n
	}
	if start >= len(items) {
		return nil, "", nil
	}
	end := start + fakePageSize
	if end >= len(items) {
		return items[start:], "", nil
	}
	return items[start:end], strconv.Itoa(end), nil
}

func (f *fakeStore) GetSettings(context.Context) (store.Settings, error) {
	if f.settings.SiteName == "" {
		return store.Settings{SiteName: store.DefaultSiteName, StatusPageTitle: store.DefaultStatusPageTitle}, nil
	}
	return f.settings, nil
}

func (f *fakeStore) UpdateSettings(_ context.Context, s store.Settings) (store.Settings, error) {
	if err := f.fail("settings"); err != nil {
		return store.Settings{}, err
	}
	f.writes++
	f.settings = s
	return s, nil
}

func (f *fakeStore) ListNotifyGroups(_ context.Context, _ int64, token string) ([]store.NotifyGroup, string, error) {
	return pageOf(f.notify, token)
}

func (f *fakeStore) CreateNotifyGroup(_ context.Context, g store.NotifyGroup) (store.NotifyGroup, error) {
	if err := f.fail(g.ID); err != nil {
		return store.NotifyGroup{}, err
	}
	for _, e := range f.notify {
		if e.Name == g.Name {
			return store.NotifyGroup{}, store.ErrNotifyGroupNameTaken
		}
	}
	f.writes++
	f.notify = append(f.notify, g)
	return g, nil
}

func (f *fakeStore) UpdateNotifyGroup(_ context.Context, id string, g store.NotifyGroup) (store.NotifyGroup, error) {
	if err := f.fail(id); err != nil {
		return store.NotifyGroup{}, err
	}
	idx := -1
	for i, e := range f.notify {
		if e.ID == id {
			idx = i
		} else if e.Name == g.Name {
			return store.NotifyGroup{}, store.ErrNotifyGroupNameTaken
		}
	}
	if idx < 0 {
		return store.NotifyGroup{}, mongo.ErrNoDocuments
	}
	g.ID = id
	f.writes++
	f.notify[idx] = g
	return g, nil
}

func (f *fakeStore) validateNotifyIDs(ids []string) error {
	for _, id := range ids {
		found := false
		for _, n := range f.notify {
			if n.ID == id {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%w: %s", store.ErrInvalidNotifyGroupIDs, id)
		}
	}
	return nil
}

func (f *fakeStore) validateGroupIDs(ids []string) error {
	for _, id := range ids {
		found := false
		for _, g := range f.groups {
			if g.ID == id {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%w: %s", store.ErrInvalidGroupIDs, id)
		}
	}
	return nil
}

func (f *fakeStore) ListMonitorGroups(_ context.Context, _ int64, token string) ([]store.MonitorGroup, string, error) {
	return pageOf(f.groups, token)
}

func (f *fakeStore) CreateMonitorGroup(_ context.Context, g store.MonitorGroup) (store.MonitorGroup, error) {
	if err := f.fail(g.ID); err != nil {
		return store.MonitorGroup{}, err
	}
	if err := f.validateNotifyIDs(g.AlertPolicy.NotifyGroupIDs); err != nil {
		return store.MonitorGroup{}, err
	}
	for _, e := range f.groups {
		if e.Name == g.Name {
			return store.MonitorGroup{}, store.ErrGroupNameTaken
		}
	}
	f.writes++
	f.groups = append(f.groups, g)
	return g, nil
}

func (f *fakeStore) UpdateMonitorGroup(_ context.Context, id string, g store.MonitorGroup) (store.MonitorGroup, error) {
	if err := f.fail(id); err != nil {
		return store.MonitorGroup{}, err
	}
	if err := f.validateNotifyIDs(g.AlertPolicy.NotifyGroupIDs); err != nil {
		return store.MonitorGroup{}, err
	}
	idx := -1
	for i, e := range f.groups {
		if e.ID == id {
			idx = i
		} else if e.Name == g.Name {
			return store.MonitorGroup{}, store.ErrGroupNameTaken
		}
	}
	if idx < 0 {
		return store.MonitorGroup{}, mongo.ErrNoDocuments
	}
	g.ID = id
	f.writes++
	f.groups[idx] = g
	return g, nil
}

func (f *fakeStore) ListServers(_ context.Context, _ string, _ []string, _ int64, token string) ([]store.Server, string, error) {
	return pageOf(f.servers, token)
}

func (f *fakeStore) CreateServer(_ context.Context, s store.Server) (store.Server, error) {
	if err := f.fail(s.ID); err != nil {
		return store.Server{}, err
	}
	if s.HealthStatus == "" {
		s.HealthStatus = store.StatusUnknown
	}
	f.writes++
	f.servers = append(f.servers, s)
	return s, nil
}

func (f *fakeStore) UpdateServer(_ context.Context, id string, s store.Server) (store.Server, error) {
	if err := f.fail(id); err != nil {
		return store.Server{}, err
	}
	for i, e := range f.servers {
		if e.ID == id {
			s.ID = id
			s.HealthStatus = e.HealthStatus
			s.LastCheckAt = e.LastCheckAt
			s.CreatedAt = e.CreatedAt
			f.writes++
			f.servers[i] = s
			return s, nil
		}
	}
	return store.Server{}, mongo.ErrNoDocuments
}

func (f *fakeStore) ListMonitors(_ context.Context, serverID string, _ int64, token string) ([]store.Monitor, string, error) {
	var own []store.Monitor
	for _, m := range f.monitors {
		if m.ServerID == serverID {
			own = append(own, m)
		}
	}
	return pageOf(own, token)
}

func (f *fakeStore) CreateMonitor(_ context.Context, serverID string, m store.Monitor) (store.Monitor, error) {
	if err := f.fail(m.ID); err != nil {
		return store.Monitor{}, err
	}
	found := false
	for _, s := range f.servers {
		if s.ID == serverID {
			found = true
		}
	}
	if !found {
		return store.Monitor{}, mongo.ErrNoDocuments
	}
	if err := f.validateGroupIDs(m.GroupIDs); err != nil {
		return store.Monitor{}, err
	}
	m.ServerID = serverID
	if m.Status == "" {
		m.Status = store.StatusUnknown
	}
	f.writes++
	f.monitors = append(f.monitors, m)
	return m, nil
}

func (f *fakeStore) UpdateMonitor(_ context.Context, serverID, monitorID string, m store.Monitor) (store.Monitor, error) {
	if err := f.fail(monitorID); err != nil {
		return store.Monitor{}, err
	}
	if err := f.validateGroupIDs(m.GroupIDs); err != nil {
		return store.Monitor{}, err
	}
	for i, e := range f.monitors {
		if e.ServerID == serverID && e.ID == monitorID {
			m.ID = monitorID
			m.ServerID = serverID
			m.Status = e.Status
			m.Certificate = e.Certificate
			m.LastCheckAt = e.LastCheckAt
			f.writes++
			f.monitors[i] = m
			return m, nil
		}
	}
	return store.Monitor{}, mongo.ErrNoDocuments
}
```

- [ ] **Step 4: 写失败的导出测试 `internal/configio/export_test.go`**

```go
package configio

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

func TestExportOmitsComputedFieldsAndKeepsSecrets(t *testing.T) {
	now := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	fs := &fakeStore{
		notify: []store.NotifyGroup{{
			ID: "ntf_1", Name: "ops",
			Channels: []store.AlertChannel{{Type: "telegram", Target: "123", Extra: map[string]string{"bot_token": "s3cret"}}},
		}},
		servers: []store.Server{{
			ID: "srv_1", Name: "web-1", Host: "10.0.0.1", Enabled: true,
			HealthStatus: store.StatusDown, LastCheckAt: now, LastStatusChangeAt: now, CreatedAt: now, UpdatedAt: now,
		}},
		monitors: []store.Monitor{{
			ID: "mon_1", ServerID: "srv_1", Name: "api", Kind: "url", Enabled: true, URL: "https://example.com",
			Status: store.StatusDown, LastCheckAt: now, LastStatusChangeAt: now, CreatedAt: now, UpdatedAt: now,
			Certificate: &store.CertificateInfo{Subject: "CN=example.com"},
		}},
	}

	data, err := Export(context.Background(), fs, now)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	out := string(data)

	for _, banned := range []string{"health_status", `"status"`, "certificate", "created_at", "updated_at", "last_check_at", "last_status_change_at", "server_id"} {
		if strings.Contains(out, banned) {
			t.Errorf("export must not contain %q:\n%s", banned, out)
		}
	}
	if !strings.Contains(out, "s3cret") {
		t.Errorf("export must include channel secrets in plaintext:\n%s", out)
	}

	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("exported JSON invalid: %v", err)
	}
	if f.Version != Version || !f.ExportedAt.Equal(now) {
		t.Fatalf("header = version %d exported_at %v", f.Version, f.ExportedAt)
	}
	if len(f.Servers) != 1 || len(f.Servers[0].Monitors) != 1 || f.Servers[0].Monitors[0].ID != "mon_1" {
		t.Fatalf("monitors must be nested under their server: %+v", f.Servers)
	}
}

func TestExportPaginatesAllResources(t *testing.T) {
	fs := &fakeStore{}
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("srv_%d", i)
		fs.servers = append(fs.servers, store.Server{ID: id, Name: id, Host: "h"})
		for j := 0; j < 3; j++ {
			fs.monitors = append(fs.monitors, store.Monitor{ID: fmt.Sprintf("mon_%d_%d", i, j), ServerID: id, Name: "m", Kind: "tcp"})
		}
		fs.notify = append(fs.notify, store.NotifyGroup{ID: fmt.Sprintf("ntf_%d", i), Name: fmt.Sprintf("n%d", i)})
		fs.groups = append(fs.groups, store.MonitorGroup{ID: fmt.Sprintf("grp_%d", i), Name: fmt.Sprintf("g%d", i)})
	}

	data, err := Export(context.Background(), fs, time.Now())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Servers) != 5 || len(f.NotifyGroups) != 5 || len(f.MonitorGroups) != 5 {
		t.Fatalf("got %d servers, %d notify groups, %d groups; want 5 each", len(f.Servers), len(f.NotifyGroups), len(f.MonitorGroups))
	}
	for _, s := range f.Servers {
		if len(s.Monitors) != 3 {
			t.Fatalf("server %s has %d monitors, want 3", s.ID, len(s.Monitors))
		}
	}
}
```

- [ ] **Step 5: 运行测试确认失败**

Run: `go test ./internal/configio/ -run TestExport -v`
Expected: 编译失败,`undefined: Export`(`export.go` 尚未创建)。

- [ ] **Step 6: 写 `internal/configio/export.go`**

```go
package configio

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

// Export serialises all monitoring configuration into a version-1 JSON
// document. Computed and runtime-managed fields are never included; channel
// secrets are included in plaintext, so the result must be handled as a
// sensitive file.
func Export(ctx context.Context, st Store, now time.Time) ([]byte, error) {
	settings, err := st.GetSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("get settings: %w", err)
	}
	f := File{Version: Version, ExportedAt: now.UTC(), Settings: settingsFromStore(settings)}

	notify, err := listAll(func(limit int64, token string) ([]store.NotifyGroup, string, error) {
		return st.ListNotifyGroups(ctx, limit, token)
	})
	if err != nil {
		return nil, fmt.Errorf("list notify groups: %w", err)
	}
	for _, g := range notify {
		f.NotifyGroups = append(f.NotifyGroups, notifyGroupFromStore(g))
	}

	groups, err := listAll(func(limit int64, token string) ([]store.MonitorGroup, string, error) {
		return st.ListMonitorGroups(ctx, limit, token)
	})
	if err != nil {
		return nil, fmt.Errorf("list monitor groups: %w", err)
	}
	for _, g := range groups {
		f.MonitorGroups = append(f.MonitorGroups, monitorGroupFromStore(g))
	}

	servers, err := listAll(func(limit int64, token string) ([]store.Server, string, error) {
		return st.ListServers(ctx, "", nil, limit, token)
	})
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}
	for _, srv := range servers {
		monitors, err := listAll(func(limit int64, token string) ([]store.Monitor, string, error) {
			return st.ListMonitors(ctx, srv.ID, limit, token)
		})
		if err != nil {
			return nil, fmt.Errorf("list monitors of server %s: %w", srv.ID, err)
		}
		f.Servers = append(f.Servers, serverFromStore(srv, monitors))
	}

	return json.MarshalIndent(f, "", "  ")
}
```

- [ ] **Step 7: 运行测试确认通过**

Run: `gofmt -l internal/configio; go vet ./internal/configio/ && go test ./internal/configio/ -run TestExport -v`
Expected: `gofmt -l` 无输出;两个 `TestExport*` 均 `PASS`。

- [ ] **Step 8: Commit**

```bash
git add internal/configio
git commit -m "feat: add configio file model and export"
```

---

### Task 3: configio 导入(预检、写入、报告)

**Files:**
- Create: `internal/configio/import.go`
- Test: `internal/configio/import_test.go`

**Interfaces:**
- Consumes: Task 2 的 `File` 及其 DTO、`decode`、`Store`、`listAll`、`ErrInvalidInput`、`Resource*` 常量、各 `toStore` 转换、`*fakeStore`。
- Produces:
  - `type Mode int`,`const ModeMerge Mode = 1`、`ModeUpsert Mode = 2`
  - `type Options struct{ Mode Mode; DryRun bool }`
  - `type Counts struct{ Created, Updated, Skipped, Failed uint32 }`
  - `type Summary struct{ Settings, NotifyGroups, MonitorGroups, Servers, Monitors Counts }`
  - `type Issue struct{ ResourceType, ID, Name, Message string }`
  - `type Report struct{ DryRun bool; Summary Summary; Errors []Issue }`
  - `func Import(ctx context.Context, st Store, data []byte, opts Options) (Report, error)`
  - 语义:`Report.Errors` 同时包含失败项和因名称冲突被跳过的项;只有读取库内现状失败才返回 `error`,写入失败记入报告。

- [ ] **Step 1: 写失败的导入测试 `internal/configio/import_test.go`**

```go
package configio

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

func sampleFile() File {
	return File{
		Version:  Version,
		Settings: &Settings{SiteName: "Prod", StatusPageTitle: "Status"},
		NotifyGroups: []NotifyGroup{{
			ID: "ntf_1", Name: "ops",
			Channels: []Channel{{Type: "telegram", Target: "123", Extra: map[string]string{"bot_token": "s3cret"}}},
		}},
		MonitorGroups: []MonitorGroup{{
			ID: "grp_1", Name: "edge", SortOrder: 1,
			AlertPolicy: AlertPolicy{Enabled: true, NotifyGroupIDs: []string{"ntf_1"}, OnDown: true},
		}},
		Servers: []Server{
			{
				ID: "srv_1", Name: "web-1", Host: "10.0.0.1", Enabled: true, Tags: []string{"prod"},
				SSH: &SSH{Enabled: true, User: "ops"},
				Monitors: []Monitor{
					{ID: "mon_1", Name: "api", Kind: "url", Enabled: true, URL: "https://example.com/health", GroupIDs: []string{"grp_1"}, TLSVerify: true, IntervalSeconds: 60, TimeoutSeconds: 5, Retries: 3},
					{ID: "mon_2", Name: "ssh", Kind: "tcp", Enabled: true, Port: 22, IntervalSeconds: 60, TimeoutSeconds: 5, Retries: 3},
				},
			},
			{ID: "srv_2", Name: "web-2", Host: "10.0.0.2", Enabled: true},
		},
	}
}

func mustJSON(t *testing.T, f File) []byte {
	t.Helper()
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func runImport(t *testing.T, fs *fakeStore, f File, opts Options) Report {
	t.Helper()
	report, err := Import(context.Background(), fs, mustJSON(t, f), opts)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	return report
}

func TestImportThenExportRoundTrip(t *testing.T) {
	fs := &fakeStore{}
	want := sampleFile()

	report := runImport(t, fs, want, Options{Mode: ModeUpsert})
	wantSummary := Summary{
		Settings:      Counts{Updated: 1},
		NotifyGroups:  Counts{Created: 1},
		MonitorGroups: Counts{Created: 1},
		Servers:       Counts{Created: 2},
		Monitors:      Counts{Created: 2},
	}
	if report.Summary != wantSummary || len(report.Errors) != 0 {
		t.Fatalf("summary = %+v errors = %+v, want %+v", report.Summary, report.Errors, wantSummary)
	}

	data, err := Export(context.Background(), fs, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	got, err := decode(data)
	if err != nil {
		t.Fatalf("exported file must be importable: %v", err)
	}
	got.ExportedAt = time.Time{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch:\n got  %+v\n want %+v", got, want)
	}
}

func TestImportMergeSkipsExistingAndCreatesMissing(t *testing.T) {
	fs := &fakeStore{
		notify:  []store.NotifyGroup{{ID: "ntf_1", Name: "ops-old"}},
		servers: []store.Server{{ID: "srv_1", Name: "old-name", Host: "old", HealthStatus: store.StatusHealthy}},
	}

	report := runImport(t, fs, sampleFile(), Options{Mode: ModeMerge})

	want := Summary{
		Settings:      Counts{Skipped: 1},
		NotifyGroups:  Counts{Skipped: 1},
		MonitorGroups: Counts{Created: 1},
		Servers:       Counts{Created: 1, Skipped: 1},
		Monitors:      Counts{Created: 2},
	}
	if report.Summary != want || len(report.Errors) != 0 {
		t.Fatalf("summary = %+v errors = %+v, want %+v", report.Summary, report.Errors, want)
	}
	if fs.servers[0].Name != "old-name" || fs.notify[0].Name != "ops-old" {
		t.Fatalf("MERGE must not overwrite existing resources: %+v %+v", fs.servers[0], fs.notify[0])
	}
	if fs.settings.SiteName != "" {
		t.Fatalf("MERGE must not overwrite settings: %+v", fs.settings)
	}
}

func TestImportUpsertOverwritesButKeepsComputedFields(t *testing.T) {
	fs := &fakeStore{
		servers:  []store.Server{{ID: "srv_1", Name: "old-name", Host: "old", HealthStatus: store.StatusDown}},
		monitors: []store.Monitor{{ID: "mon_1", ServerID: "srv_1", Name: "old", Kind: "url", Status: store.StatusDown}},
		groups:   []store.MonitorGroup{{ID: "grp_1", Name: "edge"}},
		notify:   []store.NotifyGroup{{ID: "ntf_1", Name: "ops"}},
	}

	report := runImport(t, fs, sampleFile(), Options{Mode: ModeUpsert})

	if report.Summary.Servers.Updated != 1 || report.Summary.Monitors.Updated != 1 || report.Summary.Monitors.Created != 1 {
		t.Fatalf("summary = %+v", report.Summary)
	}
	if fs.servers[0].Name != "web-1" || fs.servers[0].HealthStatus != store.StatusDown {
		t.Fatalf("server must be overwritten but keep health status: %+v", fs.servers[0])
	}
	if fs.monitors[0].Name != "api" || fs.monitors[0].Status != store.StatusDown {
		t.Fatalf("monitor must be overwritten but keep status: %+v", fs.monitors[0])
	}
}

func TestImportDryRunWritesNothing(t *testing.T) {
	fs := &fakeStore{}

	report := runImport(t, fs, sampleFile(), Options{Mode: ModeUpsert, DryRun: true})

	if fs.writes != 0 {
		t.Fatalf("dry run performed %d writes", fs.writes)
	}
	if !report.DryRun {
		t.Fatal("report.DryRun must be true")
	}
	want := Summary{
		Settings:      Counts{Updated: 1},
		NotifyGroups:  Counts{Created: 1},
		MonitorGroups: Counts{Created: 1},
		Servers:       Counts{Created: 2},
		Monitors:      Counts{Created: 2},
	}
	if report.Summary != want {
		t.Fatalf("summary = %+v, want %+v", report.Summary, want)
	}
}

func TestImportNameConflict(t *testing.T) {
	for _, tc := range []struct {
		name        string
		mode        Mode
		wantSkipped uint32
		wantFailed  uint32
	}{
		{"merge skips and reports", ModeMerge, 1, 0},
		{"upsert fails", ModeUpsert, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := &fakeStore{notify: []store.NotifyGroup{{ID: "ntf_other", Name: "ops"}}}

			report := runImport(t, fs, sampleFile(), Options{Mode: tc.mode})

			nc := report.Summary.NotifyGroups
			if nc.Skipped != tc.wantSkipped || nc.Failed != tc.wantFailed || nc.Created != 0 {
				t.Fatalf("notify groups = %+v", nc)
			}
			if len(fs.notify) != 1 {
				t.Fatalf("conflicting notify group must not be written: %+v", fs.notify)
			}
			found := false
			for _, e := range report.Errors {
				if e.ResourceType == ResourceNotifyGroup && e.ID == "ntf_1" && strings.Contains(e.Message, "ntf_other") {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected a conflict issue naming the owner, got %+v", report.Errors)
			}
			// The group referencing the unavailable notify group, and the
			// monitor referencing that group, cascade into failures.
			if report.Summary.MonitorGroups.Failed != 1 {
				t.Fatalf("monitor groups = %+v", report.Summary.MonitorGroups)
			}
			if report.Summary.Monitors.Failed != 1 || report.Summary.Monitors.Created != 1 {
				t.Fatalf("monitors = %+v", report.Summary.Monitors)
			}
		})
	}
}

func TestImportItemValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*File)
		check  func(*testing.T, Summary)
	}{
		{
			name: "duplicate server id fails the server and its monitors",
			mutate: func(f *File) {
				f.Servers = append(f.Servers, Server{ID: "srv_1", Name: "dup", Host: "h", Monitors: []Monitor{{ID: "mon_9", Name: "x", Kind: "tcp"}}})
			},
			check: func(t *testing.T, s Summary) {
				if s.Servers.Failed != 1 || s.Servers.Created != 2 || s.Monitors.Failed != 1 || s.Monitors.Created != 2 {
					t.Fatalf("summary = %+v", s)
				}
			},
		},
		{
			name:   "missing monitor id",
			mutate: func(f *File) { f.Servers[0].Monitors[0].ID = "" },
			check: func(t *testing.T, s Summary) {
				if s.Monitors.Failed != 1 || s.Monitors.Created != 1 {
					t.Fatalf("summary = %+v", s)
				}
			},
		},
		{
			name:   "unsupported monitor kind",
			mutate: func(f *File) { f.Servers[0].Monitors[1].Kind = "icmp" },
			check: func(t *testing.T, s Summary) {
				if s.Monitors.Failed != 1 || s.Monitors.Created != 1 {
					t.Fatalf("summary = %+v", s)
				}
			},
		},
		{
			name:   "monitor references unknown group",
			mutate: func(f *File) { f.Servers[0].Monitors[1].GroupIDs = []string{"grp_missing"} },
			check: func(t *testing.T, s Summary) {
				if s.Monitors.Failed != 1 || s.Monitors.Created != 1 {
					t.Fatalf("summary = %+v", s)
				}
			},
		},
		{
			name: "monitor group references unknown notify group",
			mutate: func(f *File) {
				f.MonitorGroups[0].AlertPolicy.NotifyGroupIDs = []string{"ntf_missing"}
			},
			check: func(t *testing.T, s Summary) {
				if s.MonitorGroups.Failed != 1 || s.Monitors.Failed != 1 {
					t.Fatalf("summary = %+v", s)
				}
			},
		},
		{
			name: "duplicate group name in file",
			mutate: func(f *File) {
				f.MonitorGroups = append(f.MonitorGroups, MonitorGroup{ID: "grp_2", Name: "edge"})
			},
			check: func(t *testing.T, s Summary) {
				if s.MonitorGroups.Created != 1 || s.MonitorGroups.Failed != 1 {
					t.Fatalf("summary = %+v", s)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := sampleFile()
			tc.mutate(&f)
			report := runImport(t, &fakeStore{}, f, Options{Mode: ModeUpsert})
			tc.check(t, report.Summary)
			if len(report.Errors) == 0 {
				t.Fatal("expected at least one issue in the report")
			}
		})
	}
}

func TestImportWriteFailureIsReportedAndContinues(t *testing.T) {
	fs := &fakeStore{failIDs: map[string]error{"mon_1": errors.New("boom")}}

	report := runImport(t, fs, sampleFile(), Options{Mode: ModeUpsert})

	if report.Summary.Monitors.Failed != 1 || report.Summary.Monitors.Created != 1 {
		t.Fatalf("monitors = %+v", report.Summary.Monitors)
	}
	for _, e := range report.Errors {
		if strings.Contains(e.Message, "boom") {
			t.Fatalf("raw store errors must not leak into the report: %+v", e)
		}
	}
}

func TestImportRejectsInvalidInput(t *testing.T) {
	valid := mustJSON(t, File{Version: Version})
	for _, tc := range []struct {
		name string
		data []byte
		mode Mode
	}{
		{"mode unspecified", valid, 0},
		{"malformed json", []byte("{"), ModeMerge},
		{"missing version", []byte(`{}`), ModeMerge},
		{"future version", []byte(`{"version":2}`), ModeMerge},
		{"unknown field", []byte(`{"version":1,"bogus":true}`), ModeMerge},
		{"trailing data", append(append([]byte{}, valid...), []byte(`{}`)...), ModeMerge},
		{"oversize", make([]byte, MaxImportBytes+1), ModeMerge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Import(context.Background(), &fakeStore{}, tc.data, Options{Mode: tc.mode})
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/configio/ -run TestImport -v`
Expected: 编译失败,`undefined: Import`、`undefined: Options`、`undefined: ModeUpsert` 等。

- [ ] **Step 3: 写 `internal/configio/import.go`**

```go
package configio

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/orvice/neo-line/internal/store"
)

// Mode controls how Import treats resources that already exist.
type Mode int

const (
	// ModeMerge only creates missing resources; existing ones are skipped.
	ModeMerge Mode = iota + 1
	// ModeUpsert creates missing resources and overwrites existing ones.
	ModeUpsert
)

type Options struct {
	Mode   Mode
	DryRun bool
}

type Counts struct {
	Created, Updated, Skipped, Failed uint32
}

type Summary struct {
	Settings, NotifyGroups, MonitorGroups, Servers, Monitors Counts
}

// Issue describes one failed item, or one item skipped because of a name
// conflict.
type Issue struct {
	ResourceType string
	ID           string
	Name         string
	Message      string
}

type Report struct {
	DryRun  bool
	Summary Summary
	Errors  []Issue
}

// validKinds lists the monitor kinds the store understands. Legacy kinds are
// accepted so that an export of an older database can be re-imported.
var validKinds = map[string]bool{
	"": true, "tcp": true, "url": true, "http": true, "https": true,
	"tls_port": true, "tls": true, "tls_certificate": true,
}

type action int

const (
	actCreate action = iota
	actUpdate
	actSkip
	actSkipConflict
	actConflict
)

// decide picks what to do with one entry. exists means the ID is already in
// the database; conflict means the name is owned by a different ID.
func decide(mode Mode, exists, conflict bool) action {
	switch {
	case exists && mode == ModeMerge:
		return actSkip
	case conflict && mode == ModeMerge:
		return actSkipConflict
	case conflict:
		return actConflict
	case exists:
		return actUpdate
	default:
		return actCreate
	}
}

type seenSet struct {
	ids, names map[string]bool
}

func newSeenSet() *seenSet {
	return &seenSet{ids: map[string]bool{}, names: map[string]bool{}}
}

// check validates the required fields and rejects duplicates within the file.
// Names are only required to be unique for groups (uniqueName).
func (s *seenSet) check(id, name string, uniqueName bool) string {
	switch {
	case strings.TrimSpace(id) == "":
		return "id is required"
	case strings.TrimSpace(name) == "":
		return "name is required"
	case s.ids[id]:
		return "duplicate id in file"
	case uniqueName && s.names[name]:
		return "duplicate name in file"
	}
	s.ids[id] = true
	if uniqueName {
		s.names[name] = true
	}
	return ""
}

func missingRefs(kind string, ids []string, available map[string]bool) string {
	for _, id := range ids {
		if !available[id] {
			return fmt.Sprintf("%s %q does not exist", kind, id)
		}
	}
	return ""
}

func conflictMessage(owner string) string {
	return fmt.Sprintf("name already used by %s", owner)
}

// writeMessage turns a store error into a report message. Known sentinel
// errors are safe to show; anything else is logged and replaced so driver
// details never leave the server.
func writeMessage(err error) string {
	switch {
	case errors.Is(err, store.ErrGroupNameTaken),
		errors.Is(err, store.ErrNotifyGroupNameTaken),
		errors.Is(err, store.ErrInvalidGroupIDs),
		errors.Is(err, store.ErrInvalidNotifyGroupIDs):
		return err.Error()
	case store.IsNotFound(err):
		return "parent resource not found"
	default:
		slog.Error("config import write failed", "error", err)
		return "write failed"
	}
}

type importer struct {
	st     Store
	opts   Options
	report Report
}

// Import reads a version-1 document and applies it to the store. Item-level
// problems are recorded in the report and never abort the run; an error is
// returned only for invalid input or when current state cannot be read.
func Import(ctx context.Context, st Store, data []byte, opts Options) (Report, error) {
	if opts.Mode != ModeMerge && opts.Mode != ModeUpsert {
		return Report{}, fmt.Errorf("%w: mode is required", ErrInvalidInput)
	}
	f, err := decode(data)
	if err != nil {
		return Report{}, err
	}
	im := &importer{st: st, opts: opts, report: Report{DryRun: opts.DryRun}}
	im.importSettings(ctx, f.Settings)
	notify, err := im.importNotifyGroups(ctx, f.NotifyGroups)
	if err != nil {
		return Report{}, err
	}
	groups, err := im.importMonitorGroups(ctx, f.MonitorGroups, notify)
	if err != nil {
		return Report{}, err
	}
	if err := im.importServers(ctx, f.Servers, groups); err != nil {
		return Report{}, err
	}
	return im.report, nil
}

func (im *importer) failed(c *Counts, resource, id, name, msg string) {
	c.Failed++
	im.report.Errors = append(im.report.Errors, Issue{ResourceType: resource, ID: id, Name: name, Message: msg})
}

func (im *importer) skipped(c *Counts, resource, id, name, msg string) {
	c.Skipped++
	if msg != "" {
		im.report.Errors = append(im.report.Errors, Issue{ResourceType: resource, ID: id, Name: name, Message: msg})
	}
}

func done(c *Counts, act action) {
	if act == actUpdate {
		c.Updated++
		return
	}
	c.Created++
}

func (im *importer) importSettings(ctx context.Context, s *Settings) {
	if s == nil {
		return
	}
	c := &im.report.Summary.Settings
	if im.opts.Mode == ModeMerge {
		// The settings document is a singleton that always exists.
		im.skipped(c, ResourceSettings, "site", "", "")
		return
	}
	if !im.opts.DryRun {
		if _, err := im.st.UpdateSettings(ctx, s.toStore()); err != nil {
			im.failed(c, ResourceSettings, "site", "", writeMessage(err))
			return
		}
	}
	c.Updated++
}

// importNotifyGroups returns the set of notify group IDs a monitor group may
// reference: everything already stored plus everything imported successfully.
func (im *importer) importNotifyGroups(ctx context.Context, groups []NotifyGroup) (map[string]bool, error) {
	existing, err := listAll(func(limit int64, token string) ([]store.NotifyGroup, string, error) {
		return im.st.ListNotifyGroups(ctx, limit, token)
	})
	if err != nil {
		return nil, fmt.Errorf("list notify groups: %w", err)
	}
	byID := map[string]bool{}
	nameOwner := map[string]string{}
	available := map[string]bool{}
	for _, g := range existing {
		byID[g.ID] = true
		nameOwner[g.Name] = g.ID
		available[g.ID] = true
	}

	seen := newSeenSet()
	c := &im.report.Summary.NotifyGroups
	for _, g := range groups {
		if msg := seen.check(g.ID, g.Name, true); msg != "" {
			im.failed(c, ResourceNotifyGroup, g.ID, g.Name, msg)
			continue
		}
		owner, taken := nameOwner[g.Name]
		act := decide(im.opts.Mode, byID[g.ID], taken && owner != g.ID)
		switch act {
		case actSkip:
			im.skipped(c, ResourceNotifyGroup, g.ID, g.Name, "")
		case actSkipConflict:
			im.skipped(c, ResourceNotifyGroup, g.ID, g.Name, conflictMessage(owner))
		case actConflict:
			im.failed(c, ResourceNotifyGroup, g.ID, g.Name, conflictMessage(owner))
		default:
			if !im.opts.DryRun {
				var err error
				if act == actUpdate {
					_, err = im.st.UpdateNotifyGroup(ctx, g.ID, g.toStore())
				} else {
					_, err = im.st.CreateNotifyGroup(ctx, g.toStore())
				}
				if err != nil {
					im.failed(c, ResourceNotifyGroup, g.ID, g.Name, writeMessage(err))
					continue
				}
			}
			done(c, act)
			available[g.ID] = true
		}
	}
	return available, nil
}

// importMonitorGroups returns the set of monitor group IDs a monitor may
// reference.
func (im *importer) importMonitorGroups(ctx context.Context, groups []MonitorGroup, notify map[string]bool) (map[string]bool, error) {
	existing, err := listAll(func(limit int64, token string) ([]store.MonitorGroup, string, error) {
		return im.st.ListMonitorGroups(ctx, limit, token)
	})
	if err != nil {
		return nil, fmt.Errorf("list monitor groups: %w", err)
	}
	byID := map[string]bool{}
	nameOwner := map[string]string{}
	available := map[string]bool{}
	for _, g := range existing {
		byID[g.ID] = true
		nameOwner[g.Name] = g.ID
		available[g.ID] = true
	}

	seen := newSeenSet()
	c := &im.report.Summary.MonitorGroups
	for _, g := range groups {
		if msg := seen.check(g.ID, g.Name, true); msg != "" {
			im.failed(c, ResourceMonitorGroup, g.ID, g.Name, msg)
			continue
		}
		if msg := missingRefs("notify group", g.AlertPolicy.NotifyGroupIDs, notify); msg != "" {
			im.failed(c, ResourceMonitorGroup, g.ID, g.Name, msg)
			continue
		}
		owner, taken := nameOwner[g.Name]
		act := decide(im.opts.Mode, byID[g.ID], taken && owner != g.ID)
		switch act {
		case actSkip:
			im.skipped(c, ResourceMonitorGroup, g.ID, g.Name, "")
		case actSkipConflict:
			im.skipped(c, ResourceMonitorGroup, g.ID, g.Name, conflictMessage(owner))
		case actConflict:
			im.failed(c, ResourceMonitorGroup, g.ID, g.Name, conflictMessage(owner))
		default:
			if !im.opts.DryRun {
				var err error
				if act == actUpdate {
					_, err = im.st.UpdateMonitorGroup(ctx, g.ID, g.toStore())
				} else {
					_, err = im.st.CreateMonitorGroup(ctx, g.toStore())
				}
				if err != nil {
					im.failed(c, ResourceMonitorGroup, g.ID, g.Name, writeMessage(err))
					continue
				}
			}
			done(c, act)
			available[g.ID] = true
		}
	}
	return available, nil
}

func (im *importer) failMonitors(srv Server, msg string) {
	for _, m := range srv.Monitors {
		im.failed(&im.report.Summary.Monitors, ResourceMonitor, m.ID, m.Name, msg)
	}
}

func (im *importer) importServers(ctx context.Context, servers []Server, groups map[string]bool) error {
	existing, err := listAll(func(limit int64, token string) ([]store.Server, string, error) {
		return im.st.ListServers(ctx, "", nil, limit, token)
	})
	if err != nil {
		return fmt.Errorf("list servers: %w", err)
	}
	byID := map[string]bool{}
	for _, s := range existing {
		byID[s.ID] = true
	}

	seenServers := newSeenSet()
	seenMonitors := newSeenSet() // monitor IDs must be unique across the whole file
	c := &im.report.Summary.Servers
	for _, srv := range servers {
		if msg := seenServers.check(srv.ID, srv.Name, false); msg != "" {
			im.failed(c, ResourceServer, srv.ID, srv.Name, msg)
			im.failMonitors(srv, "parent server was not imported")
			continue
		}
		exists := byID[srv.ID]
		act := decide(im.opts.Mode, exists, false)
		if act == actSkip {
			im.skipped(c, ResourceServer, srv.ID, srv.Name, "")
		} else {
			if !im.opts.DryRun {
				var err error
				if act == actUpdate {
					_, err = im.st.UpdateServer(ctx, srv.ID, srv.toStore())
				} else {
					_, err = im.st.CreateServer(ctx, srv.toStore())
				}
				if err != nil {
					im.failed(c, ResourceServer, srv.ID, srv.Name, writeMessage(err))
					im.failMonitors(srv, "parent server was not imported")
					continue
				}
			}
			done(c, act)
		}
		if err := im.importMonitors(ctx, srv, exists, groups, seenMonitors); err != nil {
			return err
		}
	}
	return nil
}

func (im *importer) importMonitors(ctx context.Context, srv Server, serverExists bool, groups map[string]bool, seen *seenSet) error {
	existing := map[string]bool{}
	if serverExists {
		monitors, err := listAll(func(limit int64, token string) ([]store.Monitor, string, error) {
			return im.st.ListMonitors(ctx, srv.ID, limit, token)
		})
		if err != nil {
			return fmt.Errorf("list monitors of server %s: %w", srv.ID, err)
		}
		for _, m := range monitors {
			existing[m.ID] = true
		}
	}

	c := &im.report.Summary.Monitors
	for _, m := range srv.Monitors {
		if msg := seen.check(m.ID, m.Name, false); msg != "" {
			im.failed(c, ResourceMonitor, m.ID, m.Name, msg)
			continue
		}
		if !validKinds[m.Kind] {
			im.failed(c, ResourceMonitor, m.ID, m.Name, fmt.Sprintf("unsupported kind %q", m.Kind))
			continue
		}
		if msg := missingRefs("group", m.GroupIDs, groups); msg != "" {
			im.failed(c, ResourceMonitor, m.ID, m.Name, msg)
			continue
		}
		act := decide(im.opts.Mode, existing[m.ID], false)
		if act == actSkip {
			im.skipped(c, ResourceMonitor, m.ID, m.Name, "")
			continue
		}
		if !im.opts.DryRun {
			var err error
			if act == actUpdate {
				_, err = im.st.UpdateMonitor(ctx, srv.ID, m.ID, m.toStore(srv.ID))
			} else {
				_, err = im.st.CreateMonitor(ctx, srv.ID, m.toStore(srv.ID))
			}
			if err != nil {
				im.failed(c, ResourceMonitor, m.ID, m.Name, writeMessage(err))
				continue
			}
		}
		done(c, act)
	}
	return nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `gofmt -l internal/configio; go vet ./internal/configio/ && go test ./internal/configio/ -v`
Expected: `gofmt -l` 无输出;全部测试 `PASS`(含 Task 2 的导出测试)。

若 `TestImportThenExportRoundTrip` 因 `reflect.DeepEqual` 失败,先看差异是不是 `nil` 与空切片(DTO 全部使用 `omitempty`,正常情况下不会出现);不要通过放宽比较来掩盖真实的字段丢失。

- [ ] **Step 5: Commit**

```bash
git add internal/configio
git commit -m "feat: add configio import with dry-run, merge and upsert modes"
```

---

### Task 4: DataService、鉴权与审计

**Files:**
- Create: `internal/connectapi/data_service.go`
- Modify: `internal/connectapi/interceptors.go`(`requiresAdmin`、`auditAction`、`auditResourceType`)
- Modify: `internal/connectapi/service.go`(`Register`)
- Test: `internal/connectapi/interceptors_test.go`
- Test: `internal/connectapi/data_service_test.go`

**Interfaces:**
- Consumes: Task 1 的 `pb.*` 与 `neolinev1connect.DataService*`;Task 2/3 的 `configio.Export`、`configio.Import`、`configio.Options`、`configio.Mode`、`configio.Report`、`configio.ErrInvalidInput`。`store.Store` 满足 `configio.Store`。
- Produces:
  - `func (s *Service) ExportConfig(ctx, *connect.Request[pb.ExportConfigRequest]) (*connect.Response[pb.ExportConfigResponse], error)`
  - `func (s *Service) ImportConfig(ctx, *connect.Request[pb.ImportConfigRequest]) (*connect.Response[pb.ImportConfigResponse], error)`
  - `func importModeFromProto(m pb.ImportMode) configio.Mode`(未指定返回 `0`)
  - `func reportToProto(r configio.Report) *pb.ImportConfigResponse`

- [ ] **Step 1: 写拦截器失败测试 `internal/connectapi/interceptors_test.go`**

```go
package connectapi

import (
	"testing"

	"github.com/orvice/neo-line/pkg/proto/neoline/v1/neolinev1connect"
)

func TestDataServiceRequiresAdmin(t *testing.T) {
	for _, procedure := range []string{
		neolinev1connect.DataServiceExportConfigProcedure,
		neolinev1connect.DataServiceImportConfigProcedure,
	} {
		if !requiresAdmin(procedure) {
			t.Errorf("%s must require the admin role", procedure)
		}
	}
}

func TestDataServiceAuditMapping(t *testing.T) {
	if got := auditAction(neolinev1connect.DataServiceExportConfigProcedure); got != "export" {
		t.Errorf("export action = %q, want export", got)
	}
	if got := auditAction(neolinev1connect.DataServiceImportConfigProcedure); got != "import" {
		t.Errorf("import action = %q, want import", got)
	}
	for _, procedure := range []string{
		neolinev1connect.DataServiceExportConfigProcedure,
		neolinev1connect.DataServiceImportConfigProcedure,
	} {
		if got := auditResourceType(procedure); got != "config" {
			t.Errorf("%s resource type = %q, want config", procedure, got)
		}
	}
}

func TestExistingAdminAndAuditRulesUnchanged(t *testing.T) {
	if requiresAdmin(neolinev1connect.ServerServiceListServersProcedure) {
		t.Error("read procedures must stay available to non-admin sessions")
	}
	if !requiresAdmin(neolinev1connect.ServerServiceCreateServerProcedure) {
		t.Error("Create* procedures must keep requiring admin")
	}
	if got := auditAction(neolinev1connect.ServerServiceCreateServerProcedure); got != "create" {
		t.Errorf("create action = %q", got)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/connectapi/ -run 'TestDataService|TestExistingAdmin' -v`
Expected: `TestDataServiceRequiresAdmin` 与 `TestDataServiceAuditMapping` FAIL(admin 判定为 false、action 为 `read`、resource type 为空);`TestExistingAdminAndAuditRulesUnchanged` PASS。

- [ ] **Step 3: 修改 `internal/connectapi/interceptors.go`**

`requiresAdmin` 的 `switch` 加入 `DataService`,并更新函数注释:

```go
// requiresAdmin reports whether a procedure is restricted to admin sessions:
// remote command execution, MCP token management, config import/export (the
// export contains channel secrets), and every mutating (Create/Update/Delete)
// procedure.
func requiresAdmin(procedure string) bool {
	service, method := splitProcedure(procedure)
	if adminProcedureExempt[service] {
		return false
	}
	switch service {
	case "SshService", "McpTokenService", "DataService":
		return true
	}
	return strings.HasPrefix(method, "Create") ||
		strings.HasPrefix(method, "Update") ||
		strings.HasPrefix(method, "Delete")
}
```

`auditAction` 的 `switch` 在 `Delete` 分支后、`default` 前加入两个分支:

```go
	case strings.HasPrefix(method, "Delete"), method == "Logout":
		return "delete"
	case method == "ExportConfig":
		return "export"
	case method == "ImportConfig":
		return "import"
	default:
		return "read"
```

`auditResourceType` 的 `switch` 在 `SshService` 分支后加入:

```go
	case "SshService":
		return "ssh"
	case "DataService":
		return "config"
	default:
		return ""
```

- [ ] **Step 4: 写 `internal/connectapi/data_service.go`**

```go
package connectapi

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/orvice/neo-line/internal/configio"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
)

// ExportConfig returns all monitoring configuration as a JSON document. The
// document includes notify channel secrets in plaintext; the procedure is
// admin only.
func (s *Service) ExportConfig(ctx context.Context, _ *connect.Request[pb.ExportConfigRequest]) (*connect.Response[pb.ExportConfigResponse], error) {
	data, err := configio.Export(ctx, s.store, time.Now().UTC())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&pb.ExportConfigResponse{Data: data}), nil
}

// ImportConfig applies a JSON document produced by ExportConfig. Item-level
// failures are reported in the response; only invalid requests return an RPC
// error.
func (s *Service) ImportConfig(ctx context.Context, req *connect.Request[pb.ImportConfigRequest]) (*connect.Response[pb.ImportConfigResponse], error) {
	report, err := configio.Import(ctx, s.store, req.Msg.GetData(), configio.Options{
		Mode:   importModeFromProto(req.Msg.GetMode()),
		DryRun: req.Msg.GetDryRun(),
	})
	if err != nil {
		if errors.Is(err, configio.ErrInvalidInput) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, toConnectError(err)
	}
	return connect.NewResponse(reportToProto(report)), nil
}

func importModeFromProto(m pb.ImportMode) configio.Mode {
	switch m {
	case pb.ImportMode_IMPORT_MODE_MERGE:
		return configio.ModeMerge
	case pb.ImportMode_IMPORT_MODE_UPSERT:
		return configio.ModeUpsert
	default:
		return 0
	}
}

func countsToProto(c configio.Counts) *pb.ImportCounts {
	return &pb.ImportCounts{Created: c.Created, Updated: c.Updated, Skipped: c.Skipped, Failed: c.Failed}
}

func reportToProto(r configio.Report) *pb.ImportConfigResponse {
	out := &pb.ImportConfigResponse{
		DryRun: r.DryRun,
		Summary: &pb.ImportSummary{
			Settings:      countsToProto(r.Summary.Settings),
			NotifyGroups:  countsToProto(r.Summary.NotifyGroups),
			MonitorGroups: countsToProto(r.Summary.MonitorGroups),
			Servers:       countsToProto(r.Summary.Servers),
			Monitors:      countsToProto(r.Summary.Monitors),
		},
	}
	for _, e := range r.Errors {
		out.Errors = append(out.Errors, &pb.ImportError{ResourceType: e.ResourceType, Id: e.ID, Name: e.Name, Message: e.Message})
	}
	return out
}
```

- [ ] **Step 5: 修改 `internal/connectapi/service.go` 的 `Register`**

在 `NewSshServiceHandler` 之后加一行。请求体用 `WithReadMaxBytes` 限制为 16 MiB(`bytes` 字段在 Connect JSON 编码中会做 base64,约膨胀 4/3,16 MiB 足以容纳解码后 10 MiB 的文件;精确的 10 MiB 上限由 `configio` 校验):

```go
	mux.Handle(neolinev1connect.NewSshServiceHandler(svc, opts))
	mux.Handle(neolinev1connect.NewDataServiceHandler(svc, opts, connect.WithReadMaxBytes(16<<20)))
```

- [ ] **Step 6: 写 `internal/connectapi/data_service_test.go`**

测试通过真实 Connect 链路(`Register` + `httptest`)验证鉴权顺序与参数错误。fake 嵌入 `store.Store` 接口,只实现 `GetSession`;这些用例都不会触达其他 store 方法(`ImportConfig` 在解码阶段就返回)。

```go
package connectapi

import (
	"context"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/gin-gonic/gin"
	"github.com/orvice/neo-line/internal/configio"
	"github.com/orvice/neo-line/internal/store"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
	"github.com/orvice/neo-line/pkg/proto/neoline/v1/neolinev1connect"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// sessionStore embeds the Store interface so only GetSession needs to be
// implemented; any other call would panic, which proves the request was
// rejected before reaching the store.
type sessionStore struct {
	store.Store
	sessions map[string]store.Session
}

func (s sessionStore) GetSession(_ context.Context, token string) (store.Session, error) {
	if sess, ok := s.sessions[token]; ok {
		return sess, nil
	}
	return store.Session{}, mongo.ErrNoDocuments
}

func newDataClient(t *testing.T) neolinev1connect.DataServiceClient {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r, sessionStore{sessions: map[string]store.Session{
		"admin-token":  {Token: "admin-token", Role: store.RoleAdmin, Email: "a@example.com"},
		"viewer-token": {Token: "viewer-token", Role: "viewer", Email: "v@example.com"},
	}}, nil)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return neolinev1connect.NewDataServiceClient(srv.Client(), srv.URL+BasePath)
}

func withToken[T any](msg *T, token string) *connect.Request[T] {
	req := connect.NewRequest(msg)
	if token != "" {
		req.Header().Set("Authorization", "Bearer "+token)
	}
	return req
}

func TestDataServiceAuth(t *testing.T) {
	client := newDataClient(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name  string
		token string
		want  connect.Code
	}{
		{"no token", "", connect.CodeUnauthenticated},
		{"unknown token", "nope", connect.CodeUnauthenticated},
		{"non-admin", "viewer-token", connect.CodePermissionDenied},
	} {
		t.Run("export "+tc.name, func(t *testing.T) {
			_, err := client.ExportConfig(ctx, withToken(&pb.ExportConfigRequest{}, tc.token))
			if got := connect.CodeOf(err); got != tc.want {
				t.Fatalf("code = %v, want %v (err %v)", got, tc.want, err)
			}
		})
		t.Run("import "+tc.name, func(t *testing.T) {
			_, err := client.ImportConfig(ctx, withToken(&pb.ImportConfigRequest{Data: []byte(`{"version":1}`), Mode: pb.ImportMode_IMPORT_MODE_MERGE}, tc.token))
			if got := connect.CodeOf(err); got != tc.want {
				t.Fatalf("code = %v, want %v (err %v)", got, tc.want, err)
			}
		})
	}
}

func TestImportConfigRejectsInvalidRequests(t *testing.T) {
	client := newDataClient(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		req  *pb.ImportConfigRequest
	}{
		{"mode unspecified", &pb.ImportConfigRequest{Data: []byte(`{"version":1}`)}},
		{"malformed json", &pb.ImportConfigRequest{Data: []byte(`{`), Mode: pb.ImportMode_IMPORT_MODE_MERGE}},
		{"future version", &pb.ImportConfigRequest{Data: []byte(`{"version":2}`), Mode: pb.ImportMode_IMPORT_MODE_UPSERT}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.ImportConfig(ctx, withToken(tc.req, "admin-token"))
			if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
				t.Fatalf("code = %v, want invalid_argument (err %v)", got, err)
			}
		})
	}
}

func TestImportModeFromProto(t *testing.T) {
	for in, want := range map[pb.ImportMode]configio.Mode{
		pb.ImportMode_IMPORT_MODE_UNSPECIFIED: 0,
		pb.ImportMode_IMPORT_MODE_MERGE:       configio.ModeMerge,
		pb.ImportMode_IMPORT_MODE_UPSERT:      configio.ModeUpsert,
	} {
		if got := importModeFromProto(in); got != want {
			t.Errorf("importModeFromProto(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestReportToProto(t *testing.T) {
	got := reportToProto(configio.Report{
		DryRun: true,
		Summary: configio.Summary{
			Servers:  configio.Counts{Created: 1, Updated: 2, Skipped: 3, Failed: 4},
			Monitors: configio.Counts{Created: 5},
		},
		Errors: []configio.Issue{{ResourceType: configio.ResourceServer, ID: "srv_1", Name: "web", Message: "boom"}},
	})
	if !got.GetDryRun() {
		t.Error("dry_run not propagated")
	}
	s := got.GetSummary().GetServers()
	if s.GetCreated() != 1 || s.GetUpdated() != 2 || s.GetSkipped() != 3 || s.GetFailed() != 4 {
		t.Errorf("servers counts = %v", s)
	}
	if got.GetSummary().GetMonitors().GetCreated() != 5 || got.GetSummary().GetSettings() == nil {
		t.Errorf("summary = %v", got.GetSummary())
	}
	if len(got.GetErrors()) != 1 || got.GetErrors()[0].GetId() != "srv_1" || got.GetErrors()[0].GetMessage() != "boom" {
		t.Errorf("errors = %v", got.GetErrors())
	}
}
```

- [ ] **Step 7: 运行测试确认通过**

Run: `gofmt -l internal; go vet ./... && go test ./internal/connectapi/ ./internal/configio/ -v`
Expected: `gofmt -l` 无输出;所有测试 `PASS`,包括已有的 `TestRegisterMountsConnectHandlersUnderAPIPrefix`。

- [ ] **Step 8: Commit**

```bash
git add internal/connectapi
git commit -m "feat: add admin-only DataService for config import/export"
```

---

### Task 5: 文档与整体验证

**Files:**
- Modify: `docs/features.md`
- Modify: `docs/monitoring-configuration.md`
- Modify: `docs/README.md`

**Interfaces:**
- Consumes: Task 1–4 的行为与 Global Constraints。
- Produces: 与实现一致的中文文档。

- [ ] **Step 1: 更新 `docs/features.md`**

在「## MCP Server」章节之前(「### Monitor Group API」章节末尾的字段说明之后)新增一节:

````markdown
### 配置导入导出

**状态：** 已实现（Connect API，仅 Admin）

提供 API 用于把监控配置导出为单个 JSON 文件，或把该文件导入到另一个 neo-line 实例（备份、环境迁移、批量编辑）。配置仍以 MongoDB 为唯一权威来源，导入导出只是读写 MongoDB 的一种方式。

```text
DataService.ExportConfig
DataService.ImportConfig
```

两个接口都要求 `admin` 角色：导出文件包含通知渠道的明文密钥（`channels[].extra`，如 `bot_token`、`access_token`、webhook header），请当作敏感文件保管。审计日志记录 action `export` / `import`、`resource_type` 为 `config`，不记录文件内容。

范围：

- 包含：站点设置、通知组、monitor 分组、server 及其下的 monitor。
- 不包含：`monitor_results`、`server_events`、`users`、`mcp_tokens`、`audit_logs`。

`ImportConfig` 请求字段：

- `data`：由 `ExportConfig` 生成的 JSON 文件内容，解码后上限 `10 MiB`。
- `mode`：必填。`MERGE` 只新增，已存在（按 ID 匹配）的跳过；`UPSERT` 新增并覆盖已存在的。
- `dry_run`：为 `true` 时只校验并统计将发生的变化，不写库。

运行行为：

- 导入保留文件中的原始 ID，因此 monitor 对分组、分组对通知组的引用在迁移后保持有效。
- 按依赖顺序写入：settings → 通知组 → monitor 分组 → server → monitor。
- 计算与运行时托管字段（健康状态、monitor 状态、证书快照、各类时间戳）不导出，导入时忽略；`UPSERT` 覆盖已有 server / monitor 时保留库内已有的这些字段。
- `settings` 是单例：`MERGE` 视为已存在并跳过，`UPSERT` 覆盖。
- 同名但 ID 不同的通知组 / 分组：`MERGE` 跳过并在 `errors` 中报告，`UPSERT` 计入 `failed` 并报告；不会自动改名或合并。
- 引用了不存在的通知组 / 分组、缺少 `id` / `name`、文件内 ID 重复、monitor `kind` 不支持的条目计入 `failed`，其下依赖它的条目随之失败。
- 导入不是原子的：单项写入失败只记录并继续，不整体回滚。建议先用 `dry_run` 预检。
- 导入从不删除库内数据。
- 响应包含 `dry_run`、按资源类型的 `created` / `updated` / `skipped` / `failed` 计数，以及 `errors`（失败项和因名称冲突被跳过的项，含 `resource_type`、`id`、`name`、`message`）。底层存储错误细节只写服务端日志，不返回给客户端。

错误码：

- `invalid_argument`：JSON 非法、`version` 不是 `1`、存在未知字段、超过大小上限、`mode` 未指定。
- `unauthenticated` / `permission_denied`：未登录 / 非 admin。
````

同时在「## 用户与鉴权」→「### 接口鉴权范围」的「需 `admin` 角色的接口」一条中,把 `SshService` 全部接口和 `McpTokenService` 全部接口改为 `SshService`、`McpTokenService`、`DataService` 全部接口。

- [ ] **Step 2: 更新 `docs/monitoring-configuration.md`**

在文末追加一节,说明文件格式:

````markdown
## 配置导入导出文件格式

`DataService.ExportConfig` 生成、`DataService.ImportConfig` 读取的是单个 JSON 文件，顶层 `version` 固定为 `1`。下面是字段结构示例（不是本地配置文件，neo-line 不会从磁盘读取它）：

```json
{
  "version": 1,
  "exported_at": "2026-10-18T00:00:00Z",
  "settings": { "site_name": "neo-line", "status_page_title": "服务状态" },
  "notify_groups": [
    {
      "id": "ntf_ops",
      "name": "ops",
      "channels": [ { "type": "telegram", "target": "123456", "extra": { "bot_token": "..." } } ]
    }
  ],
  "monitor_groups": [
    {
      "id": "grp_edge",
      "name": "prod-edge",
      "sort_order": 0,
      "alert_policy": { "enabled": true, "notify_group_ids": ["ntf_ops"], "on_down": true, "on_recover": true }
    }
  ],
  "servers": [
    {
      "id": "srv_web1",
      "name": "web-1",
      "host": "10.0.0.1",
      "enabled": true,
      "monitors": [
        {
          "id": "mon_api",
          "name": "api-health",
          "kind": "url",
          "enabled": true,
          "url": "https://example.com/health",
          "group_ids": ["grp_edge"],
          "tls_verify": true,
          "interval_seconds": 60,
          "timeout_seconds": 5,
          "retries": 3
        }
      ]
    }
  ]
}
```

规则：

- `notify_groups`、`monitor_groups`、`servers`、`monitors` 的 `id` 与 `name` 必填；`id` 在文件内必须唯一，分组与通知组的 `name` 在文件内也必须唯一。手工新增条目时需要自行指定 `id`，格式不限，建议沿用 `ntf_` / `grp_` / `srv_` / `mon_` 前缀。
- monitor 嵌套在所属 server 下，没有 `server_id` 字段；`group_ids` 引用 `monitor_groups[].id`，`alert_policy.notify_group_ids` 引用 `notify_groups[].id`，引用目标可以在同一文件中，也可以已存在于库内。
- 布尔字段（如 `enabled`、`tls_verify`）缺省时按 `false` 处理，导入时请显式写出。
- 字段名严格校验，出现未知字段会整体拒绝导入，用于及早发现拼写错误。
- monitor `kind` 支持 `tcp`、`url`、`tls_port`，以及为兼容历史数据保留的 `http`、`https`、`tls`、`tls_certificate`；缺省按 `tcp` 处理。
- 以下字段不出现在文件中，导入时即使携带也会被忽略：server 的 `health_status`、`last_status_change_at`、`last_check_at`、`created_at`、`updated_at`，monitor 的 `status`、`last_check_at`、`last_status_change_at`、`certificate`、`created_at`、`updated_at`。
- `channels[].extra` 以明文导出，包含密钥，文件需要按敏感文件保管。
````

- [ ] **Step 3: 更新 `docs/README.md`**

在「当前应用基础」列表的 `AuditLogService` 一行之后加入:

```markdown
- DataService：Admin 专用的监控配置导入导出（单个 JSON 文件，支持 `MERGE` / `UPSERT` 与 `dry_run` 预检）
```

在「产品范围」列表末尾加入:

```markdown
- 导出和导入监控配置（站点设置、通知组、monitor 分组、server 与 monitor），用于备份、环境迁移和批量编辑（`DataService.ExportConfig` / `ImportConfig`，仅 Admin）
```

- [ ] **Step 4: 回写 spec 中已确认的细节**

在 `docs/superpowers/specs/2026-10-18-config-import-export-design.md` 做以下修改:

1. 「导入流程」小节末尾「具体做法在实现计划中确认……」那一段改为:`Store` 的 `Create*` 方法已经在 ID 非空时沿用调用方指定的 ID,无需改动 store 层。
2. 「冲突与已存在处理」表格之后补充:`settings` 是单例,MERGE 视为已存在并跳过,UPSERT 覆盖。
3. 「文件格式」规则中补充:`notify_groups`、`monitor_groups`、`servers`、`monitors` 的 `id`、`name` 必填;导入时对字段名严格校验,出现未知字段整体拒绝。
4. 「API」小节补充:`errors` 包含失败项和因名称冲突被跳过的项。

- [ ] **Step 5: 整体验证**

Run: `gofmt -l . | grep -v '^pkg/proto' ; go vet ./... && go test ./... && go build ./...`
Expected: `gofmt -l` 无输出(已排除生成目录);`go vet`、`go test ./...`、`go build ./...` 全部成功。

Run: `buf lint`
Expected: 无输出。

- [ ] **Step 6: 检查仓库状态并提交**

Run: `git status --short`
Expected: 只包含 Task 1–5 涉及的文件。

```bash
git add docs
git commit -m "docs: document config import/export"
```

---

## Self-Review

**Spec coverage**

| Spec 要求 | 任务 |
|---|---|
| 范围:settings / notify_groups / monitor_groups / servers / monitors | Task 2(导出)、Task 3(导入) |
| 单个 JSON、`version`、嵌套 monitor、保留 ID | Task 2 `file.go`、Task 3 |
| 计算字段不导出、导入忽略 | Task 2 DTO 无这些字段;`TestExportOmitsComputedFieldsAndKeepsSecrets`;`TestImportUpsertOverwritesButKeepsComputedFields` |
| 通知渠道明文导出 | `TestExportOmitsComputedFieldsAndKeepsSecrets` |
| 10 MiB 上限 | Task 2 `decode`、Task 4 `WithReadMaxBytes`;`TestImportRejectsInvalidInput` 的 oversize 用例 |
| `ExportConfig` / `ImportConfig` proto | Task 1 |
| MERGE / UPSERT / dry_run | Task 3 `decide`、`TestImportMerge…`、`TestImportUpsert…`、`TestImportDryRunWritesNothing` |
| 名称冲突:MERGE 跳过报告、UPSERT 报冲突 | `TestImportNameConflict` |
| 引用校验、缺字段、重复 ID、kind 非法 | `TestImportItemValidation` |
| 单项失败继续、非原子 | `TestImportWriteFailureIsReportedAndContinues` |
| 从不删除库内数据 | 实现只含 Create / Update,无 Delete 调用 |
| 错误码 `invalid_argument` 等 | Task 4 `TestImportConfigRejectsInvalidRequests`、`TestDataServiceAuth` |
| `DataService` 整体 admin | Task 4 `requiresAdmin`、`TestDataServiceRequiresAdmin`、`TestDataServiceAuth` |
| 审计 action `export` / `import`、resource_type `config` | Task 4 `TestDataServiceAuditMapping` |
| 文档三处更新 | Task 5 |
| Spec 的待确认点(Store 保留 ID) | 已确认 `Create*` 在 ID 非空时沿用,Task 5 Step 4 回写 spec |

**Placeholder scan:** 没有 TBD、TODO 或"类似 Task N"的引用,每个代码步骤都给出完整代码。

**Type consistency:** `configio.Mode` / `ModeMerge` / `ModeUpsert`、`Options`、`Counts`、`Summary`、`Issue`、`Report`、`ErrInvalidInput` 在 Task 3 定义,Task 4 按同名使用;`pb.ImportCounts` 等字段名与 Task 1 的 proto 字段一一对应(`notify_groups` → `NotifyGroups`,`resource_type` → `ResourceType`,`id` → `Id`);`fakeStore` 实现的方法签名与 `configio.Store` 一致,并有 `var _ Store = (*fakeStore)(nil)` 编译期检查。

**已知取舍:** 导入时只在同一 server 内用 `ListMonitors` 判断 monitor 是否已存在;若某个 monitor ID 在库内属于另一台 server,会按新建处理(数据库唯一索引是 `(server_id, id)`,允许这种情况)。这与现有 `CreateMonitor` 行为一致,不额外处理。
