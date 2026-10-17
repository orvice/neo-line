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
