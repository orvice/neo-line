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
