package certnotify

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

const (
	EventOperationFailed         = "certificate_operation_failed"
	EventOperationFailedReminder = "certificate_operation_failed_reminder"
	EventOperationRecovered      = "certificate_operation_recovered"
	EventExpiringSoon            = "certificate_expiring_soon"
	EventExpired                 = "certificate_expired"
)

// Payload is the webhook JSON body for certificate lifecycle events. It does not
// reuse monitor alert fields (monitor_id, group_id, health status).
type Payload struct {
	EventType            string    `json:"event_type"`
	ManagedCertificateID string    `json:"managed_certificate_id"`
	CertificateName      string    `json:"certificate_name"`
	Domains              []string  `json:"domains"`
	OperationType        string    `json:"operation_type,omitempty"`
	OperationID          string    `json:"operation_id,omitempty"`
	ErrorSummary         string    `json:"error_summary,omitempty"`
	ActiveValidity       string    `json:"active_validity"`
	DaysRemaining        int32     `json:"days_remaining,omitempty"`
	OccurredAt           time.Time `json:"occurred_at"`
}

func renderPayload(p Payload) (notifyJSON []byte, humanText string, err error) {
	notifyJSON, err = json.Marshal(p)
	if err != nil {
		return nil, "", err
	}
	return notifyJSON, formatHumanText(p), nil
}

func formatHumanText(p Payload) string {
	var b strings.Builder
	b.WriteString("[neo-line 证书] ")
	switch p.EventType {
	case EventOperationFailed:
		b.WriteString("签发/续期失败")
	case EventOperationFailedReminder:
		b.WriteString("签发/续期持续失败提醒")
	case EventOperationRecovered:
		b.WriteString("签发/续期已恢复")
	case EventExpiringSoon:
		b.WriteString("证书即将过期（7 天内）")
	case EventExpired:
		b.WriteString("证书已过期")
	default:
		b.WriteString("证书事件")
	}
	b.WriteString("\n证书：")
	b.WriteString(p.CertificateName)
	b.WriteString(" (")
	b.WriteString(p.ManagedCertificateID)
	b.WriteString(")\n域名：")
	b.WriteString(strings.Join(p.Domains, ", "))
	if p.OperationType != "" {
		b.WriteString("\n操作：")
		b.WriteString(p.OperationType)
	}
	if p.OperationID != "" {
		b.WriteString("\nOperation ID：")
		b.WriteString(p.OperationID)
	}
	if p.ErrorSummary != "" {
		b.WriteString("\n错误：")
		b.WriteString(p.ErrorSummary)
	}
	b.WriteString("\nActive 有效性：")
	b.WriteString(p.ActiveValidity)
	if p.DaysRemaining > 0 {
		b.WriteString(fmt.Sprintf("\n剩余天数：%d", p.DaysRemaining))
	}
	b.WriteString("\n时间：")
	b.WriteString(p.OccurredAt.UTC().Format(time.RFC3339))
	return b.String()
}

func daysRemaining(v *store.CertificateVersion, now time.Time) int32 {
	if v == nil {
		return 0
	}
	if now.After(v.NotAfter) {
		return 0
	}
	remaining := v.NotAfter.Sub(now)
	days := int32(remaining / (24 * time.Hour))
	if days == 0 && remaining > 0 {
		return 1
	}
	return days
}
