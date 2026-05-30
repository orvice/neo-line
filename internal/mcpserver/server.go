// Package mcpserver exposes neo-line monitoring data and configuration over
// the Model Context Protocol using the official Go SDK. It serves read and
// write tools backed by the MongoDB store over the streamable HTTP transport.
package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/orvice/neo-line/internal/store"
)

// NewServer builds an MCP server with read and write monitoring tools.
func NewServer(st store.Store) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "neo-line", Version: "v1.0.0"}, nil)
	t := &tools{store: st}

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_servers",
		Description: "List monitored servers, optionally filtered by environment and tags.",
	}, t.listServers)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_server",
		Description: "Get a single monitored server by id.",
	}, t.getServer)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_server_health",
		Description: "Get aggregated health status for a server, including monitor counts per state.",
	}, t.getServerHealth)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_server_events",
		Description: "List health status change events for a server, most recent first.",
	}, t.listServerEvents)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_monitors",
		Description: "List monitors (checks) attached to a server.",
	}, t.listMonitors)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_monitor",
		Description: "Get a single monitor by server id and monitor id.",
	}, t.getMonitor)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_check_results",
		Description: "List recent check results for a monitor, optionally filtered by an RFC3339 time range.",
	}, t.listCheckResults)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_monitor_uptime",
		Description: "Get Kuma-style rolling uptime windows for a monitor by server id and monitor id.",
	}, t.getMonitorUptime)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_monitor_groups",
		Description: "List monitor groups (flat) with their alert policies.",
	}, t.listMonitorGroups)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_monitor_group",
		Description: "Get a single monitor group by id, including its alert policy.",
	}, t.getMonitorGroup)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_monitors_by_group",
		Description: "List monitors that belong to the given monitor group (across servers).",
	}, t.listMonitorsByGroup)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_server",
		Description: "Create a new monitored server. Returns the persisted server with its generated id.",
	}, t.createServer)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "update_server",
		Description: "Update an existing server by id. Replaces mutable fields; preserves health and timestamps.",
	}, t.updateServer)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_server",
		Description: "Delete a server by id. Also deletes monitors attached to that server.",
	}, t.deleteServer)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_monitor",
		Description: "Create a new monitor attached to a server. Returns the persisted monitor.",
	}, t.createMonitor)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "update_monitor",
		Description: "Update an existing monitor by server id and monitor id.",
	}, t.updateMonitor)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_monitor",
		Description: "Delete a monitor by server id and monitor id.",
	}, t.deleteMonitor)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_monitor_group",
		Description: "Create a new monitor group with its alert policy.",
	}, t.createMonitorGroup)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "update_monitor_group",
		Description: "Update an existing monitor group by id, including its alert policy.",
	}, t.updateMonitorGroup)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_monitor_group",
		Description: "Delete a monitor group by id.",
	}, t.deleteMonitorGroup)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_notify_groups",
		Description: "List notify groups (reusable buckets of alert delivery channels).",
	}, t.listNotifyGroups)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_notify_group",
		Description: "Get a single notify group by id, including its channels.",
	}, t.getNotifyGroup)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_notify_group",
		Description: "Create a new notify group with its delivery channels (webhook, telegram, discord, mastodon).",
	}, t.createNotifyGroup)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "update_notify_group",
		Description: "Update an existing notify group by id, including its channels.",
	}, t.updateNotifyGroup)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_notify_group",
		Description: "Delete a notify group by id. Also removes it from monitor groups that reference it.",
	}, t.deleteNotifyGroup)

	return srv
}

type tools struct {
	store store.Store
}

type pageInput struct {
	PageSize  int64  `json:"page_size,omitempty" jsonschema:"max results to return, 1-200, defaults to 50"`
	PageToken string `json:"page_token,omitempty" jsonschema:"opaque token from a previous response to fetch the next page"`
}

type listServersInput struct {
	Environment string   `json:"environment,omitempty" jsonschema:"filter servers by environment"`
	Tags        []string `json:"tags,omitempty" jsonschema:"filter servers that have all of these tags"`
	pageInput
}

type listServersOutput struct {
	Servers       []store.Server `json:"servers"`
	NextPageToken string         `json:"next_page_token,omitempty"`
}

func (t *tools) listServers(ctx context.Context, _ *mcp.CallToolRequest, in listServersInput) (*mcp.CallToolResult, listServersOutput, error) {
	servers, next, err := t.store.ListServers(ctx, in.Environment, in.Tags, in.PageSize, in.PageToken)
	if err != nil {
		return nil, listServersOutput{}, err
	}
	return nil, listServersOutput{Servers: servers, NextPageToken: next}, nil
}

type serverIDInput struct {
	ID string `json:"id" jsonschema:"the server id"`
}

type serverOutput struct {
	Server store.Server `json:"server"`
}

func (t *tools) getServer(ctx context.Context, _ *mcp.CallToolRequest, in serverIDInput) (*mcp.CallToolResult, serverOutput, error) {
	server, err := t.store.GetServer(ctx, in.ID)
	if err != nil {
		return nil, serverOutput{}, mapErr(err)
	}
	return nil, serverOutput{Server: server}, nil
}

type healthOutput struct {
	Health store.ServerHealth `json:"health"`
}

func (t *tools) getServerHealth(ctx context.Context, _ *mcp.CallToolRequest, in serverIDInput) (*mcp.CallToolResult, healthOutput, error) {
	health, err := t.store.GetServerHealth(ctx, in.ID)
	if err != nil {
		return nil, healthOutput{}, mapErr(err)
	}
	return nil, healthOutput{Health: health}, nil
}

type listServerEventsInput struct {
	ServerID string `json:"server_id" jsonschema:"the server id"`
	pageInput
}

type listServerEventsOutput struct {
	Events        []store.ServerEvent `json:"events"`
	NextPageToken string              `json:"next_page_token,omitempty"`
}

func (t *tools) listServerEvents(ctx context.Context, _ *mcp.CallToolRequest, in listServerEventsInput) (*mcp.CallToolResult, listServerEventsOutput, error) {
	events, next, err := t.store.ListServerEvents(ctx, in.ServerID, in.PageSize, in.PageToken)
	if err != nil {
		return nil, listServerEventsOutput{}, err
	}
	return nil, listServerEventsOutput{Events: events, NextPageToken: next}, nil
}

type listMonitorsInput struct {
	ServerID string `json:"server_id" jsonschema:"the server id"`
	pageInput
}

type listMonitorsOutput struct {
	Monitors      []store.Monitor `json:"monitors"`
	NextPageToken string          `json:"next_page_token,omitempty"`
}

func (t *tools) listMonitors(ctx context.Context, _ *mcp.CallToolRequest, in listMonitorsInput) (*mcp.CallToolResult, listMonitorsOutput, error) {
	monitors, next, err := t.store.ListMonitors(ctx, in.ServerID, in.PageSize, in.PageToken)
	if err != nil {
		return nil, listMonitorsOutput{}, err
	}
	return nil, listMonitorsOutput{Monitors: monitors, NextPageToken: next}, nil
}

type getMonitorInput struct {
	ServerID  string `json:"server_id" jsonschema:"the server id"`
	MonitorID string `json:"monitor_id" jsonschema:"the monitor id"`
}

type monitorOutput struct {
	Monitor store.Monitor `json:"monitor"`
}

func (t *tools) getMonitor(ctx context.Context, _ *mcp.CallToolRequest, in getMonitorInput) (*mcp.CallToolResult, monitorOutput, error) {
	monitor, err := t.store.GetMonitor(ctx, in.ServerID, in.MonitorID)
	if err != nil {
		return nil, monitorOutput{}, mapErr(err)
	}
	return nil, monitorOutput{Monitor: monitor}, nil
}

type monitorUptimeOutput struct {
	Uptime store.MonitorUptime `json:"uptime"`
}

func (t *tools) getMonitorUptime(ctx context.Context, _ *mcp.CallToolRequest, in getMonitorInput) (*mcp.CallToolResult, monitorUptimeOutput, error) {
	uptime, err := t.store.GetMonitorUptime(ctx, in.ServerID, in.MonitorID)
	if err != nil {
		return nil, monitorUptimeOutput{}, mapErr(err)
	}
	return nil, monitorUptimeOutput{Uptime: uptime}, nil
}

type listCheckResultsInput struct {
	ServerID  string `json:"server_id" jsonschema:"the server id"`
	MonitorID string `json:"monitor_id" jsonschema:"the monitor id"`
	StartTime string `json:"start_time,omitempty" jsonschema:"RFC3339 lower bound on started_at, for example 2026-05-29T01:22:00+08:00"`
	EndTime   string `json:"end_time,omitempty" jsonschema:"RFC3339 upper bound on started_at"`
	pageInput
}

type listCheckResultsOutput struct {
	Results       []store.CheckResult `json:"results"`
	NextPageToken string              `json:"next_page_token,omitempty"`
}

func (t *tools) listCheckResults(ctx context.Context, _ *mcp.CallToolRequest, in listCheckResultsInput) (*mcp.CallToolResult, listCheckResultsOutput, error) {
	start, err := parseTime(in.StartTime)
	if err != nil {
		return nil, listCheckResultsOutput{}, fmt.Errorf("invalid start_time: %w", err)
	}
	end, err := parseTime(in.EndTime)
	if err != nil {
		return nil, listCheckResultsOutput{}, fmt.Errorf("invalid end_time: %w", err)
	}
	results, next, err := t.store.ListCheckResults(ctx, in.ServerID, in.MonitorID, in.PageSize, in.PageToken, start, end)
	if err != nil {
		return nil, listCheckResultsOutput{}, err
	}
	return nil, listCheckResultsOutput{Results: results, NextPageToken: next}, nil
}

type listMonitorGroupsOutput struct {
	Groups        []store.MonitorGroup `json:"groups"`
	NextPageToken string               `json:"next_page_token,omitempty"`
}

func (t *tools) listMonitorGroups(ctx context.Context, _ *mcp.CallToolRequest, in pageInput) (*mcp.CallToolResult, listMonitorGroupsOutput, error) {
	groups, next, err := t.store.ListMonitorGroups(ctx, in.PageSize, in.PageToken)
	if err != nil {
		return nil, listMonitorGroupsOutput{}, err
	}
	return nil, listMonitorGroupsOutput{Groups: groups, NextPageToken: next}, nil
}

type monitorGroupIDInput struct {
	GroupID string `json:"group_id" jsonschema:"the monitor group id"`
}

type monitorGroupOutput struct {
	Group store.MonitorGroup `json:"group"`
}

func (t *tools) getMonitorGroup(ctx context.Context, _ *mcp.CallToolRequest, in monitorGroupIDInput) (*mcp.CallToolResult, monitorGroupOutput, error) {
	group, err := t.store.GetMonitorGroup(ctx, in.GroupID)
	if err != nil {
		return nil, monitorGroupOutput{}, mapErr(err)
	}
	return nil, monitorGroupOutput{Group: group}, nil
}

type listMonitorsByGroupInput struct {
	GroupID string `json:"group_id" jsonschema:"the monitor group id"`
	pageInput
}

func (t *tools) listMonitorsByGroup(ctx context.Context, _ *mcp.CallToolRequest, in listMonitorsByGroupInput) (*mcp.CallToolResult, listMonitorsOutput, error) {
	monitors, next, err := t.store.ListMonitorsByGroup(ctx, in.GroupID, in.PageSize, in.PageToken)
	if err != nil {
		return nil, listMonitorsOutput{}, err
	}
	return nil, listMonitorsOutput{Monitors: monitors, NextPageToken: next}, nil
}

type createServerInput struct {
	Server store.Server `json:"server" jsonschema:"server fields; id is optional and will be generated when empty"`
}

func (t *tools) createServer(ctx context.Context, _ *mcp.CallToolRequest, in createServerInput) (*mcp.CallToolResult, serverOutput, error) {
	created, err := t.store.CreateServer(ctx, in.Server)
	if err != nil {
		return nil, serverOutput{}, err
	}
	return nil, serverOutput{Server: created}, nil
}

type updateServerInput struct {
	ID     string       `json:"id" jsonschema:"the server id"`
	Server store.Server `json:"server" jsonschema:"updated server fields; replaces mutable fields"`
}

func (t *tools) updateServer(ctx context.Context, _ *mcp.CallToolRequest, in updateServerInput) (*mcp.CallToolResult, serverOutput, error) {
	updated, err := t.store.UpdateServer(ctx, in.ID, in.Server)
	if err != nil {
		return nil, serverOutput{}, mapErr(err)
	}
	return nil, serverOutput{Server: updated}, nil
}

type deleteOutput struct {
	Deleted bool `json:"deleted"`
}

func (t *tools) deleteServer(ctx context.Context, _ *mcp.CallToolRequest, in serverIDInput) (*mcp.CallToolResult, deleteOutput, error) {
	if err := t.store.DeleteServer(ctx, in.ID); err != nil {
		return nil, deleteOutput{}, mapErr(err)
	}
	return nil, deleteOutput{Deleted: true}, nil
}

type createMonitorInput struct {
	ServerID string        `json:"server_id" jsonschema:"the server id this monitor belongs to"`
	Monitor  store.Monitor `json:"monitor" jsonschema:"monitor fields; id is optional and will be generated when empty"`
}

func (t *tools) createMonitor(ctx context.Context, _ *mcp.CallToolRequest, in createMonitorInput) (*mcp.CallToolResult, monitorOutput, error) {
	created, err := t.store.CreateMonitor(ctx, in.ServerID, in.Monitor)
	if err != nil {
		return nil, monitorOutput{}, mapErr(err)
	}
	return nil, monitorOutput{Monitor: created}, nil
}

type updateMonitorInput struct {
	ServerID  string        `json:"server_id" jsonschema:"the server id"`
	MonitorID string        `json:"monitor_id" jsonschema:"the monitor id"`
	Monitor   store.Monitor `json:"monitor" jsonschema:"updated monitor fields"`
}

func (t *tools) updateMonitor(ctx context.Context, _ *mcp.CallToolRequest, in updateMonitorInput) (*mcp.CallToolResult, monitorOutput, error) {
	updated, err := t.store.UpdateMonitor(ctx, in.ServerID, in.MonitorID, in.Monitor)
	if err != nil {
		return nil, monitorOutput{}, mapErr(err)
	}
	return nil, monitorOutput{Monitor: updated}, nil
}

func (t *tools) deleteMonitor(ctx context.Context, _ *mcp.CallToolRequest, in getMonitorInput) (*mcp.CallToolResult, deleteOutput, error) {
	if err := t.store.DeleteMonitor(ctx, in.ServerID, in.MonitorID); err != nil {
		return nil, deleteOutput{}, mapErr(err)
	}
	return nil, deleteOutput{Deleted: true}, nil
}

type createMonitorGroupInput struct {
	Group store.MonitorGroup `json:"group" jsonschema:"monitor group fields; id is optional and will be generated when empty"`
}

func (t *tools) createMonitorGroup(ctx context.Context, _ *mcp.CallToolRequest, in createMonitorGroupInput) (*mcp.CallToolResult, monitorGroupOutput, error) {
	created, err := t.store.CreateMonitorGroup(ctx, in.Group)
	if err != nil {
		return nil, monitorGroupOutput{}, err
	}
	return nil, monitorGroupOutput{Group: created}, nil
}

type updateMonitorGroupInput struct {
	GroupID string             `json:"group_id" jsonschema:"the monitor group id"`
	Group   store.MonitorGroup `json:"group" jsonschema:"updated monitor group fields, including alert policy"`
}

func (t *tools) updateMonitorGroup(ctx context.Context, _ *mcp.CallToolRequest, in updateMonitorGroupInput) (*mcp.CallToolResult, monitorGroupOutput, error) {
	updated, err := t.store.UpdateMonitorGroup(ctx, in.GroupID, in.Group)
	if err != nil {
		return nil, monitorGroupOutput{}, mapErr(err)
	}
	return nil, monitorGroupOutput{Group: updated}, nil
}

func (t *tools) deleteMonitorGroup(ctx context.Context, _ *mcp.CallToolRequest, in monitorGroupIDInput) (*mcp.CallToolResult, deleteOutput, error) {
	if err := t.store.DeleteMonitorGroup(ctx, in.GroupID); err != nil {
		return nil, deleteOutput{}, mapErr(err)
	}
	return nil, deleteOutput{Deleted: true}, nil
}

type listNotifyGroupsOutput struct {
	Groups        []store.NotifyGroup `json:"groups"`
	NextPageToken string              `json:"next_page_token,omitempty"`
}

func (t *tools) listNotifyGroups(ctx context.Context, _ *mcp.CallToolRequest, in pageInput) (*mcp.CallToolResult, listNotifyGroupsOutput, error) {
	groups, next, err := t.store.ListNotifyGroups(ctx, in.PageSize, in.PageToken)
	if err != nil {
		return nil, listNotifyGroupsOutput{}, err
	}
	return nil, listNotifyGroupsOutput{Groups: groups, NextPageToken: next}, nil
}

type notifyGroupIDInput struct {
	NotifyGroupID string `json:"notify_group_id" jsonschema:"the notify group id"`
}

type notifyGroupOutput struct {
	Group store.NotifyGroup `json:"group"`
}

func (t *tools) getNotifyGroup(ctx context.Context, _ *mcp.CallToolRequest, in notifyGroupIDInput) (*mcp.CallToolResult, notifyGroupOutput, error) {
	group, err := t.store.GetNotifyGroup(ctx, in.NotifyGroupID)
	if err != nil {
		return nil, notifyGroupOutput{}, mapErr(err)
	}
	return nil, notifyGroupOutput{Group: group}, nil
}

type createNotifyGroupInput struct {
	Group store.NotifyGroup `json:"group" jsonschema:"notify group fields; id is optional and will be generated when empty"`
}

func (t *tools) createNotifyGroup(ctx context.Context, _ *mcp.CallToolRequest, in createNotifyGroupInput) (*mcp.CallToolResult, notifyGroupOutput, error) {
	created, err := t.store.CreateNotifyGroup(ctx, in.Group)
	if err != nil {
		return nil, notifyGroupOutput{}, err
	}
	return nil, notifyGroupOutput{Group: created}, nil
}

type updateNotifyGroupInput struct {
	NotifyGroupID string            `json:"notify_group_id" jsonschema:"the notify group id"`
	Group         store.NotifyGroup `json:"group" jsonschema:"updated notify group fields, including channels"`
}

func (t *tools) updateNotifyGroup(ctx context.Context, _ *mcp.CallToolRequest, in updateNotifyGroupInput) (*mcp.CallToolResult, notifyGroupOutput, error) {
	updated, err := t.store.UpdateNotifyGroup(ctx, in.NotifyGroupID, in.Group)
	if err != nil {
		return nil, notifyGroupOutput{}, mapErr(err)
	}
	return nil, notifyGroupOutput{Group: updated}, nil
}

func (t *tools) deleteNotifyGroup(ctx context.Context, _ *mcp.CallToolRequest, in notifyGroupIDInput) (*mcp.CallToolResult, deleteOutput, error) {
	if err := t.store.DeleteNotifyGroup(ctx, in.NotifyGroupID); err != nil {
		return nil, deleteOutput{}, mapErr(err)
	}
	return nil, deleteOutput{Deleted: true}, nil
}

func parseTime(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func mapErr(err error) error {
	if store.IsNotFound(err) {
		return fmt.Errorf("not found")
	}
	return err
}
