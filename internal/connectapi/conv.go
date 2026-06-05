package connectapi

import (
	"time"

	"github.com/orvice/neo-line/internal/store"
	pb "github.com/orvice/neo-line/pkg/proto/neoline/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func timeToTS(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

func tsToTime(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

func tsToTimePtr(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}

// --- Server ---

func serverToProto(s store.Server) *pb.Server {
	out := &pb.Server{
		Id:                 s.ID,
		Name:               s.Name,
		Host:               s.Host,
		Environment:        s.Environment,
		Region:             s.Region,
		Tags:               s.Tags,
		SortOrder:          s.SortOrder,
		Enabled:            s.Enabled,
		HealthStatus:       s.HealthStatus,
		LastStatusChangeAt: timeToTS(s.LastStatusChangeAt),
		LastCheckAt:        timeToTS(s.LastCheckAt),
		CreatedAt:          timeToTS(s.CreatedAt),
		UpdatedAt:          timeToTS(s.UpdatedAt),
	}
	if s.SSH != nil {
		out.Ssh = &pb.ServerSSH{
			Enabled: s.SSH.Enabled,
			Host:    s.SSH.Host,
			Port:    s.SSH.Port,
			User:    s.SSH.User,
		}
	}
	return out
}

func serverFromProto(p *pb.Server) store.Server {
	if p == nil {
		return store.Server{}
	}
	out := store.Server{
		ID:                 p.GetId(),
		Name:               p.GetName(),
		Host:               p.GetHost(),
		Environment:        p.GetEnvironment(),
		Region:             p.GetRegion(),
		Tags:               p.GetTags(),
		SortOrder:          p.GetSortOrder(),
		Enabled:            p.GetEnabled(),
		HealthStatus:       p.GetHealthStatus(),
		LastStatusChangeAt: tsToTime(p.GetLastStatusChangeAt()),
		LastCheckAt:        tsToTime(p.GetLastCheckAt()),
		CreatedAt:          tsToTime(p.GetCreatedAt()),
		UpdatedAt:          tsToTime(p.GetUpdatedAt()),
	}
	if ssh := p.GetSsh(); ssh != nil {
		out.SSH = &store.ServerSSH{
			Enabled: ssh.GetEnabled(),
			Host:    ssh.GetHost(),
			Port:    ssh.GetPort(),
			User:    ssh.GetUser(),
		}
	}
	return out
}

func serverEventToProto(e store.ServerEvent) *pb.ServerEvent {
	return &pb.ServerEvent{
		Id:             e.ID,
		ServerId:       e.ServerID,
		PreviousStatus: e.PreviousStatus,
		CurrentStatus:  e.CurrentStatus,
		Reason:         e.Reason,
		OccurredAt:     timeToTS(e.OccurredAt),
	}
}

func serverHealthToProto(h store.ServerHealth) *pb.ServerHealth {
	return &pb.ServerHealth{
		ServerId:           h.ServerID,
		Status:             h.Status,
		LastStatusChangeAt: timeToTS(h.LastStatusChangeAt),
		LastCheckAt:        timeToTS(h.LastCheckAt),
		TotalMonitors:      h.TotalMonitors,
		HealthyMonitors:    h.HealthyMonitors,
		WarningMonitors:    h.WarningMonitors,
		CriticalMonitors:   h.CriticalMonitors,
		DownMonitors:       h.DownMonitors,
		UnknownMonitors:    h.UnknownMonitors,
	}
}

// --- Certificate ---

func certToProto(c *store.CertificateInfo) *pb.CertificateInfo {
	if c == nil {
		return nil
	}
	return &pb.CertificateInfo{
		Subject:       c.Subject,
		Issuer:        c.Issuer,
		DnsNames:      c.DNSNames,
		SerialNumber:  c.SerialNumber,
		NotBefore:     timeToTS(c.NotBefore),
		NotAfter:      timeToTS(c.NotAfter),
		DaysRemaining: c.DaysRemaining,
	}
}

func certFromProto(c *pb.CertificateInfo) *store.CertificateInfo {
	if c == nil {
		return nil
	}
	return &store.CertificateInfo{
		Subject:       c.GetSubject(),
		Issuer:        c.GetIssuer(),
		DNSNames:      c.GetDnsNames(),
		SerialNumber:  c.GetSerialNumber(),
		NotBefore:     tsToTime(c.GetNotBefore()),
		NotAfter:      tsToTime(c.GetNotAfter()),
		DaysRemaining: c.GetDaysRemaining(),
	}
}

// --- Monitor ---

func monitorToProto(m store.Monitor) *pb.Monitor {
	return &pb.Monitor{
		Id:                  m.ID,
		ServerId:            m.ServerID,
		GroupIds:            m.GroupIDs,
		Name:                m.Name,
		Kind:                m.Kind,
		Enabled:             m.Enabled,
		Host:                m.Host,
		Port:                m.Port,
		Url:                 m.URL,
		Method:              m.Method,
		Path:                m.Path,
		Headers:             m.Headers,
		ExpectedStatusCodes: m.ExpectedStatusCodes,
		TlsVerify:           m.TLSVerify,
		SniName:             m.SNIName,
		WarningDays:         m.WarningDays,
		CriticalDays:        m.CriticalDays,
		IntervalSeconds:     m.IntervalSeconds,
		TimeoutSeconds:      m.TimeoutSeconds,
		Retries:             m.Retries,
		Status:              m.Status,
		LastCheckAt:         timeToTS(m.LastCheckAt),
		LastStatusChangeAt:  timeToTS(m.LastStatusChangeAt),
		Certificate:         certToProto(m.Certificate),
		CreatedAt:           timeToTS(m.CreatedAt),
		UpdatedAt:           timeToTS(m.UpdatedAt),
	}
}

func monitorFromProto(p *pb.Monitor) store.Monitor {
	if p == nil {
		return store.Monitor{}
	}
	return store.Monitor{
		ID:                  p.GetId(),
		ServerID:            p.GetServerId(),
		GroupIDs:            p.GetGroupIds(),
		Name:                p.GetName(),
		Kind:                p.GetKind(),
		Enabled:             p.GetEnabled(),
		Host:                p.GetHost(),
		Port:                p.GetPort(),
		URL:                 p.GetUrl(),
		Method:              p.GetMethod(),
		Path:                p.GetPath(),
		Headers:             p.GetHeaders(),
		ExpectedStatusCodes: p.GetExpectedStatusCodes(),
		TLSVerify:           p.GetTlsVerify(),
		SNIName:             p.GetSniName(),
		WarningDays:         p.GetWarningDays(),
		CriticalDays:        p.GetCriticalDays(),
		IntervalSeconds:     p.GetIntervalSeconds(),
		TimeoutSeconds:      p.GetTimeoutSeconds(),
		Retries:             p.GetRetries(),
		Status:              p.GetStatus(),
		LastCheckAt:         tsToTime(p.GetLastCheckAt()),
		LastStatusChangeAt:  tsToTime(p.GetLastStatusChangeAt()),
		Certificate:         certFromProto(p.GetCertificate()),
		CreatedAt:           tsToTime(p.GetCreatedAt()),
		UpdatedAt:           tsToTime(p.GetUpdatedAt()),
	}
}

func checkResultToProto(r store.CheckResult) *pb.CheckResult {
	return &pb.CheckResult{
		Id:             r.ID,
		ServerId:       r.ServerID,
		MonitorId:      r.MonitorID,
		Status:         r.Status,
		StartedAt:      timeToTS(r.StartedAt),
		EndedAt:        timeToTS(r.EndedAt),
		DurationMs:     r.DurationMS,
		ErrorStage:     r.ErrorStage,
		ErrorMessage:   r.ErrorMessage,
		RemoteAddress:  r.RemoteAddress,
		Port:           r.Port,
		HttpStatusCode: r.HTTPStatusCode,
		Certificate:    certToProto(r.Certificate),
	}
}

func uptimeToProto(u store.MonitorUptime) *pb.MonitorUptime {
	out := &pb.MonitorUptime{
		Windows:    make(map[string]*pb.UptimeWindow, len(u.Windows)),
		Heartbeats: make([]*pb.Heartbeat, 0, len(u.Heartbeats)),
	}
	for k, w := range u.Windows {
		out.Windows[k] = &pb.UptimeWindow{
			WindowSeconds: w.WindowSeconds,
			Total:         int32(w.Total),
			Up:            int32(w.Up),
			Down:          int32(w.Down),
			Uptime:        w.Uptime,
			AvgLatencyMs:  w.AvgLatencyMS,
		}
	}
	for _, h := range u.Heartbeats {
		out.Heartbeats = append(out.Heartbeats, &pb.Heartbeat{
			Status:     h.Status,
			StartedAt:  timeToTS(h.StartedAt),
			DurationMs: h.DurationMS,
		})
	}
	return out
}

// --- Monitor group ---

func alertPolicyToProto(p store.AlertPolicy) *pb.AlertPolicy {
	return &pb.AlertPolicy{
		Enabled:            p.Enabled,
		NotifyGroupIds:     p.NotifyGroupIDs,
		OnDown:             p.OnDown,
		OnRecover:          p.OnRecover,
		OnWarning:          p.OnWarning,
		OnCritical:         p.OnCritical,
		MinIntervalSeconds: p.MinIntervalSeconds,
	}
}

func alertPolicyFromProto(p *pb.AlertPolicy) store.AlertPolicy {
	if p == nil {
		return store.AlertPolicy{}
	}
	return store.AlertPolicy{
		Enabled:            p.GetEnabled(),
		NotifyGroupIDs:     p.GetNotifyGroupIds(),
		OnDown:             p.GetOnDown(),
		OnRecover:          p.GetOnRecover(),
		OnWarning:          p.GetOnWarning(),
		OnCritical:         p.GetOnCritical(),
		MinIntervalSeconds: p.GetMinIntervalSeconds(),
	}
}

func monitorGroupToProto(g store.MonitorGroup) *pb.MonitorGroup {
	return &pb.MonitorGroup{
		Id:          g.ID,
		Name:        g.Name,
		Description: g.Description,
		SortOrder:   g.SortOrder,
		AlertPolicy: alertPolicyToProto(g.AlertPolicy),
		CreatedAt:   timeToTS(g.CreatedAt),
		UpdatedAt:   timeToTS(g.UpdatedAt),
	}
}

func monitorGroupFromProto(p *pb.MonitorGroup) store.MonitorGroup {
	if p == nil {
		return store.MonitorGroup{}
	}
	return store.MonitorGroup{
		ID:          p.GetId(),
		Name:        p.GetName(),
		Description: p.GetDescription(),
		SortOrder:   p.GetSortOrder(),
		AlertPolicy: alertPolicyFromProto(p.GetAlertPolicy()),
		CreatedAt:   tsToTime(p.GetCreatedAt()),
		UpdatedAt:   tsToTime(p.GetUpdatedAt()),
	}
}

// --- Notify group ---

func alertChannelToProto(c store.AlertChannel) *pb.AlertChannel {
	return &pb.AlertChannel{
		Type:   c.Type,
		Target: c.Target,
		Extra:  c.Extra,
	}
}

func alertChannelFromProto(c *pb.AlertChannel) store.AlertChannel {
	return store.AlertChannel{
		Type:   c.GetType(),
		Target: c.GetTarget(),
		Extra:  c.GetExtra(),
	}
}

func notifyGroupToProto(g store.NotifyGroup) *pb.NotifyGroup {
	out := &pb.NotifyGroup{
		Id:          g.ID,
		Name:        g.Name,
		Description: g.Description,
		Channels:    make([]*pb.AlertChannel, 0, len(g.Channels)),
		CreatedAt:   timeToTS(g.CreatedAt),
		UpdatedAt:   timeToTS(g.UpdatedAt),
	}
	for _, c := range g.Channels {
		out.Channels = append(out.Channels, alertChannelToProto(c))
	}
	return out
}

func notifyGroupFromProto(p *pb.NotifyGroup) store.NotifyGroup {
	if p == nil {
		return store.NotifyGroup{}
	}
	out := store.NotifyGroup{
		ID:          p.GetId(),
		Name:        p.GetName(),
		Description: p.GetDescription(),
		CreatedAt:   tsToTime(p.GetCreatedAt()),
		UpdatedAt:   tsToTime(p.GetUpdatedAt()),
	}
	for _, c := range p.GetChannels() {
		out.Channels = append(out.Channels, alertChannelFromProto(c))
	}
	return out
}

// --- Settings / MCP / User ---

func settingsToProto(s store.Settings) *pb.Settings {
	return &pb.Settings{
		SiteName:        s.SiteName,
		StatusPageTitle: s.StatusPageTitle,
		UpdatedAt:       timeToTS(s.UpdatedAt),
	}
}

func settingsFromProto(p *pb.Settings) store.Settings {
	if p == nil {
		return store.Settings{}
	}
	return store.Settings{
		SiteName:        p.GetSiteName(),
		StatusPageTitle: p.GetStatusPageTitle(),
	}
}

func mcpTokenToProto(t store.McpToken) *pb.McpToken {
	return &pb.McpToken{
		Id:         t.ID,
		Name:       t.Name,
		Prefix:     t.Prefix,
		CreatedAt:  timeToTS(t.CreatedAt),
		LastUsedAt: timeToTS(t.LastUsedAt),
	}
}

func userToProto(id, email, role string) *pb.User {
	return &pb.User{Id: id, Email: email, Role: role}
}

func auditLogToProto(log store.AuditLog) *pb.AuditLog {
	return &pb.AuditLog{
		Id:           log.ID,
		Source:       log.Source,
		ActorId:      log.ActorID,
		ActorEmail:   log.ActorEmail,
		TokenPrefix:  log.TokenPrefix,
		Action:       log.Action,
		ResourceType: log.ResourceType,
		ResourceId:   log.ResourceID,
		Method:       log.Method,
		Path:         log.Path,
		StatusCode:   int32(log.StatusCode),
		Success:      log.Success,
		Error:        log.Error,
		DurationMs:   log.DurationMS,
		RemoteIp:     log.RemoteIP,
		UserAgent:    log.UserAgent,
		Metadata:     log.Metadata,
		OccurredAt:   timeToTS(log.OccurredAt),
	}
}
