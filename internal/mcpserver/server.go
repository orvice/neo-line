// Package mcpserver exposes neo-line monitoring data over the Model Context
// Protocol using the official Go SDK. It serves read-only tools backed by the
// MongoDB store over the streamable HTTP transport.
package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/orvice/neo-line/internal/store"
)

// NewServer builds an MCP server with read-only monitoring tools.
func NewServer(st *store.Store) *mcp.Server {
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

	return srv
}

type tools struct {
	store *store.Store
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
