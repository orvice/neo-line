package connectapi

import (
	"time"

	"github.com/orvice/neo-line/internal/certmanager"
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

// serverFromProto maps client-supplied fields onto a store.Server. Computed and
// server-managed fields (health status, status-change/check timestamps,
// created/updated timestamps) are intentionally omitted: the store derives them
// so a client cannot forge a monitor's health or rewrite its history.
func serverFromProto(p *pb.Server) store.Server {
	if p == nil {
		return store.Server{}
	}
	out := store.Server{
		ID:          p.GetId(),
		Name:        p.GetName(),
		Host:        p.GetHost(),
		Environment: p.GetEnvironment(),
		Region:      p.GetRegion(),
		Tags:        p.GetTags(),
		SortOrder:   p.GetSortOrder(),
		Enabled:     p.GetEnabled(),
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

// monitorFromProto maps client-supplied fields onto a store.Monitor. Computed
// and probe-managed fields (status, last-check/status-change timestamps, the
// observed certificate, created/updated timestamps) are intentionally omitted so
// a client cannot forge a monitor's status or certificate; the store and probe
// own those values.
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

// --- DNS provider account ---

func dnsProviderAccountToProto(a certmanager.PublicAccount) *pb.DNSProviderAccount {
	return &pb.DNSProviderAccount{
		Id:                        a.ID,
		Name:                      a.Name,
		Provider:                  a.Provider,
		PropagationTimeoutSeconds: a.PropagationTimeoutSeconds,
		TokenConfigured:           a.TokenConfigured,
		TokenLastVerifiedAt:       timeToTSPtr(a.TokenLastVerifiedAt),
		CreatedAt:                 timeToTS(a.CreatedAt),
		UpdatedAt:                 timeToTS(a.UpdatedAt),
	}
}

func certificateIssuerRegistrationStatusToProto(status string) pb.CertificateIssuerRegistrationStatus {
	switch status {
	case store.IssuerRegistrationPending:
		return pb.CertificateIssuerRegistrationStatus_CERTIFICATE_ISSUER_REGISTRATION_STATUS_PENDING
	case store.IssuerRegistrationReady:
		return pb.CertificateIssuerRegistrationStatus_CERTIFICATE_ISSUER_REGISTRATION_STATUS_READY
	case store.IssuerRegistrationFailed:
		return pb.CertificateIssuerRegistrationStatus_CERTIFICATE_ISSUER_REGISTRATION_STATUS_FAILED
	default:
		return pb.CertificateIssuerRegistrationStatus_CERTIFICATE_ISSUER_REGISTRATION_STATUS_UNSPECIFIED
	}
}

func certificateIssuerToProto(i certmanager.PublicIssuer) *pb.CertificateIssuer {
	return &pb.CertificateIssuer{
		Id:                     i.ID,
		Name:                   i.Name,
		CaType:                 i.CAType,
		DirectoryUrl:           i.DirectoryURL,
		Email:                  i.Email,
		RegistrationStatus:     certificateIssuerRegistrationStatusToProto(i.RegistrationStatus),
		RegistrationError:      i.RegistrationError,
		StagingUntrusted:       i.StagingUntrusted,
		TermsOfServiceUrl:      i.TermsOfServiceURL,
		TermsOfServiceAgreedAt: timeToTSPtr(i.TermsOfServiceAgreedAt),
		AccountKeyConfigured:   i.AccountKeyConfigured,
		EabConfigured:          i.EABConfigured,
		CreatedAt:              timeToTS(i.CreatedAt),
		UpdatedAt:              timeToTS(i.UpdatedAt),
	}
}

func certificateIssuerDirectoryPreviewToProto(p certmanager.DirectoryPreview) *pb.CertificateIssuerDirectoryPreview {
	return &pb.CertificateIssuerDirectoryPreview{
		CaType:            p.CAType,
		DirectoryUrl:      p.DirectoryURL,
		TermsOfServiceUrl: p.TermsOfServiceURL,
		StagingUntrusted:  p.StagingUntrusted,
		RequiresEab:       p.RequiresEAB,
	}
}

func certificateKeyTypeToProto(keyType string) pb.CertificateKeyType {
	switch keyType {
	case store.CertKeyTypeRSA2048:
		return pb.CertificateKeyType_CERTIFICATE_KEY_TYPE_RSA_2048
	case store.CertKeyTypeECP256:
		return pb.CertificateKeyType_CERTIFICATE_KEY_TYPE_EC_P256
	default:
		return pb.CertificateKeyType_CERTIFICATE_KEY_TYPE_UNSPECIFIED
	}
}

func certificateValidityToProto(validity string) pb.CertificateValidity {
	switch validity {
	case store.CertValidityMissing:
		return pb.CertificateValidity_CERTIFICATE_VALIDITY_MISSING
	case store.CertValidityValid:
		return pb.CertificateValidity_CERTIFICATE_VALIDITY_VALID
	case store.CertValidityRenewalDue:
		return pb.CertificateValidity_CERTIFICATE_VALIDITY_RENEWAL_DUE
	case store.CertValidityExpired:
		return pb.CertificateValidity_CERTIFICATE_VALIDITY_EXPIRED
	case store.CertValidityRevoked:
		return pb.CertificateValidity_CERTIFICATE_VALIDITY_REVOKED
	default:
		return pb.CertificateValidity_CERTIFICATE_VALIDITY_UNSPECIFIED
	}
}

func certificateOperationTypeToProto(opType string) pb.CertificateOperationType {
	switch opType {
	case store.CertOpTypeIssue:
		return pb.CertificateOperationType_CERTIFICATE_OPERATION_TYPE_ISSUE
	case store.CertOpTypeRenew:
		return pb.CertificateOperationType_CERTIFICATE_OPERATION_TYPE_RENEW
	case store.CertOpTypeRevoke:
		return pb.CertificateOperationType_CERTIFICATE_OPERATION_TYPE_REVOKE
	default:
		return pb.CertificateOperationType_CERTIFICATE_OPERATION_TYPE_UNSPECIFIED
	}
}

func certificateOperationStatusToProto(status string) pb.CertificateOperationStatus {
	switch status {
	case store.CertOpStatusPending:
		return pb.CertificateOperationStatus_CERTIFICATE_OPERATION_STATUS_PENDING
	case store.CertOpStatusRunning:
		return pb.CertificateOperationStatus_CERTIFICATE_OPERATION_STATUS_RUNNING
	case store.CertOpStatusSucceeded:
		return pb.CertificateOperationStatus_CERTIFICATE_OPERATION_STATUS_SUCCEEDED
	case store.CertOpStatusFailed:
		return pb.CertificateOperationStatus_CERTIFICATE_OPERATION_STATUS_FAILED
	default:
		return pb.CertificateOperationStatus_CERTIFICATE_OPERATION_STATUS_UNSPECIFIED
	}
}

func issueConfigSnapshotToProto(s store.IssueConfigSnapshot) *pb.IssueConfigSnapshot {
	return &pb.IssueConfigSnapshot{
		Domains:              append([]string(nil), s.Domains...),
		CertificateIssuerId:  s.CertificateIssuerID,
		DnsProviderAccountId: s.DNSProviderAccountID,
		KeyType:              certificateKeyTypeToProto(s.KeyType),
	}
}

func certificateOperationToProto(op certmanager.PublicOperation) *pb.CertificateOperation {
	return &pb.CertificateOperation{
		Id:                   op.ID,
		ManagedCertificateId: op.ManagedCertificateID,
		Type:                 certificateOperationTypeToProto(op.Type),
		Status:               certificateOperationStatusToProto(op.Status),
		AttemptCount:         op.AttemptCount,
		ConfigSnapshot:       issueConfigSnapshotToProto(op.ConfigSnapshot),
		ErrorSummary:         op.ErrorSummary,
		Warning:              op.Warning,
		StartedAt:            timeToTSPtr(op.StartedAt),
		FinishedAt:           timeToTSPtr(op.FinishedAt),
		NextAttemptAt:        timeToTSPtr(op.NextAttemptAt),
		CreatedAt:            timeToTS(op.CreatedAt),
		UpdatedAt:            timeToTS(op.UpdatedAt),
	}
}

func certificateVersionToProto(v *certmanager.PublicCertificateVersion) *pb.CertificateVersionMetadata {
	if v == nil {
		return nil
	}
	return &pb.CertificateVersionMetadata{
		Id:               v.ID,
		ConfigSnapshot:   issueConfigSnapshotToProto(v.ConfigSnapshot),
		LeafFingerprint:  v.LeafFingerprint,
		SerialNumber:     v.SerialNumber,
		IssuerCommonName: v.IssuerCommonName,
		NotBefore:        unixToTS(v.NotBefore),
		NotAfter:         unixToTS(v.NotAfter),
		KeyType:          certificateKeyTypeToProto(v.KeyType),
		StagingUntrusted: v.StagingUntrusted,
		CreatedAt:        unixToTS(v.CreatedAt),
	}
}

func certificateBundleToProto(b certmanager.CertificateBundle) *pb.CertificateBundle {
	return &pb.CertificateBundle{
		ManagedCertificateId: b.ManagedCertificateID,
		VersionId:            b.VersionID,
		Domains:              append([]string(nil), b.Domains...),
		KeyType:              certificateKeyTypeToProto(b.KeyType),
		LeafFingerprint:      b.LeafFingerprint,
		NotBefore:            unixToTS(b.NotBefore),
		NotAfter:             unixToTS(b.NotAfter),
		Validity:             certificateValidityToProto(b.Validity),
		StagingUntrusted:     b.StagingUntrusted,
		FullchainPem:         b.FullchainPEM,
		PrivateKeyPem:        b.PrivateKeyPEM,
	}
}

func unixToTS(sec int64) *timestamppb.Timestamp {
	if sec == 0 {
		return nil
	}
	return timestamppb.New(time.Unix(sec, 0).UTC())
}

func serverCertificateToProto(c certmanager.ServerCertificate) *pb.ServerCertificate {
	out := &pb.ServerCertificate{
		ManagedCertificateId: c.ManagedCertificateID,
		Name:                 c.Name,
		Domains:              append([]string(nil), c.Domains...),
		ActiveVersionId:      c.ActiveVersionID,
		Available:            c.Available,
		Validity:             certificateValidityToProto(c.Validity),
		KeyType:              certificateKeyTypeToProto(c.KeyType),
		LeafFingerprint:      c.LeafFingerprint,
		NotBefore:            unixToTS(c.NotBefore),
		NotAfter:             unixToTS(c.NotAfter),
		StagingUntrusted:     c.StagingUntrusted,
		ErrorSummary:         c.ErrorSummary,
	}
	return out
}

func managedCertificateToProto(c certmanager.PublicManagedCertificate) *pb.ManagedCertificate {
	out := &pb.ManagedCertificate{
		Id:                   c.ID,
		Name:                 c.Name,
		Domains:              append([]string(nil), c.Domains...),
		CertificateIssuerId:  c.CertificateIssuerID,
		DnsProviderAccountId: c.DNSProviderAccountID,
		KeyType:              certificateKeyTypeToProto(c.KeyType),
		RenewBeforeDays:      c.RenewBeforeDays,
		NotifyGroupIds:       append([]string(nil), c.NotifyGroupIDs...),
		ServerIds:            append([]string(nil), c.ServerIDs...),
		ActiveValidity:       certificateValidityToProto(c.ActiveValidity),
		BundleAvailable:      c.BundleAvailable,
		CreatedAt:            timeToTS(c.CreatedAt),
		UpdatedAt:            timeToTS(c.UpdatedAt),
	}
	auto := c.AutoRenewEnabled
	out.AutoRenewEnabled = &auto
	if c.LatestOperation != nil {
		out.LatestOperation = certificateOperationToProto(*c.LatestOperation)
	}
	if c.ActiveVersion != nil {
		out.ActiveVersion = certificateVersionToProto(c.ActiveVersion)
	}
	return out
}

func timeToTSPtr(t *time.Time) *timestamppb.Timestamp {
	if t == nil || t.IsZero() {
		return nil
	}
	return timestamppb.New(*t)
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

func certificateAccessTokenToProto(t store.CertificateAccessToken) *pb.CertificateAccessToken {
	now := time.Now().UTC()
	out := &pb.CertificateAccessToken{
		Id:        t.ID,
		ServerId:  t.ServerID,
		Name:      t.Name,
		Prefix:    t.Prefix,
		CreatedAt: timeToTS(t.CreatedAt),
		Expired:   storeCertificateAccessTokenExpired(t, now),
	}
	if t.LastUsedAt != nil && !t.LastUsedAt.IsZero() {
		out.LastUsedAt = timeToTS(*t.LastUsedAt)
	}
	if t.ExpiresAt != nil && !t.ExpiresAt.IsZero() {
		out.ExpiresAt = timeToTS(*t.ExpiresAt)
	}
	return out
}

func storeCertificateAccessTokenExpired(t store.CertificateAccessToken, now time.Time) bool {
	if t.ExpiresAt == nil || t.ExpiresAt.IsZero() {
		return false
	}
	return !t.ExpiresAt.After(now)
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
