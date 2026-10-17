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
