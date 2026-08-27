// Package certnotify delivers certificate lifecycle notifications through the
// generic notify transport adapters. Events are independent from MonitorGroup
// AlertPolicy and use certificate-specific webhook JSON and human-readable text.
package certnotify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/orvice/neo-line/internal/notify"
	"github.com/orvice/neo-line/internal/store"
)

// Store is the persistence slice certnotify needs for NotifyGroup resolution
// and notification throttle markers.
type Store interface {
	GetManagedCertificate(ctx context.Context, id string) (store.ManagedCertificate, error)
	GetNotifyGroup(ctx context.Context, id string) (store.NotifyGroup, error)
	ListManagedCertificatesForNotifications(ctx context.Context) ([]store.ManagedCertificate, error)
	FindRunningCertificateOperation(ctx context.Context, managedCertificateID, opType string) (store.CertificateOperation, error)
	TryRecordOperationFailureNotification(ctx context.Context, certID string, now time.Time) (bool, error)
	TryRecordOperationFailureReminder(ctx context.Context, certID string, now time.Time) (bool, error)
	TryRecordOperationRecovery(ctx context.Context, certID string, now time.Time) (bool, error)
	TryRecordSevenDayReminder(ctx context.Context, certID, versionID string, now time.Time) (bool, error)
	TryRecordExpiredNotification(ctx context.Context, certID, versionID string, now time.Time) (bool, error)
	SetCertificateNotificationWarning(ctx context.Context, certID, warning string, at time.Time) error
}

// Clock supplies the current time; tests inject a fixed clock.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

// Dispatcher evaluates certificate lifecycle events and fans out to NotifyGroup
// channels. Delivery is best-effort and never changes ACME operations or versions.
type Dispatcher struct {
	store    Store
	notifier *notify.Notifier
	clock    Clock
	logger   *slog.Logger

	deliveries sync.WaitGroup
}

// New builds a Dispatcher backed by st with built-in channel adapters.
func New(st Store, logger *slog.Logger) *Dispatcher {
	if logger == nil {
		logger = slog.Default().With("component", "certnotify")
	}
	return NewWithNotifier(st, notify.New(&http.Client{Timeout: 5 * time.Second}, logger), logger)
}

// NewWithNotifier builds a Dispatcher with an explicit notifier (tests inject
// recording senders).
func NewWithNotifier(st Store, n *notify.Notifier, logger *slog.Logger) *Dispatcher {
	if logger == nil {
		logger = slog.Default().With("component", "certnotify")
	}
	return &Dispatcher{store: st, notifier: n, clock: realClock{}, logger: logger}
}

// SetClock configures the clock (tests).
func (d *Dispatcher) SetClock(c Clock) {
	if d != nil && c != nil {
		d.clock = c
	}
}

// Wait blocks until in-flight deliveries complete (tests).
func (d *Dispatcher) Wait() {
	if d == nil {
		return
	}
	d.deliveries.Wait()
}

// OnOperationFailure sends the first failure notification immediately, or a
// sustained-failure reminder at most once per 24 hours.
func (d *Dispatcher) OnOperationFailure(ctx context.Context, cert store.ManagedCertificate, op store.CertificateOperation, errorSummary string) {
	if d == nil || len(cert.NotifyGroupIDs) == 0 {
		return
	}
	now := d.clock.Now()
	recorded, err := d.store.TryRecordOperationFailureNotification(ctx, cert.ID, now)
	if err != nil {
		d.logger.Warn("record cert failure notification", "certificate_id", cert.ID, "error", err.Error())
		return
	}
	eventType := EventOperationFailed
	if !recorded {
		recorded, err = d.store.TryRecordOperationFailureReminder(ctx, cert.ID, now)
		if err != nil {
			d.logger.Warn("record cert failure reminder", "certificate_id", cert.ID, "error", err.Error())
			return
		}
		if !recorded {
			return
		}
		eventType = EventOperationFailedReminder
	}
	validity, _ := computeValidity(cert, now)
	p := Payload{
		EventType:            eventType,
		ManagedCertificateID: cert.ID,
		CertificateName:      cert.Name,
		Domains:              append([]string(nil), cert.Domains...),
		OperationType:        op.Type,
		OperationID:          op.ID,
		ErrorSummary:         errorSummary,
		ActiveValidity:       validity,
		OccurredAt:           now,
	}
	d.deliver(ctx, cert, p)
}

// OnOperationSuccess sends a recovery notification when a prior failure existed.
// The first successful Issue/Renew does not count as recovery.
func (d *Dispatcher) OnOperationSuccess(ctx context.Context, cert store.ManagedCertificate, op store.CertificateOperation) {
	if d == nil || len(cert.NotifyGroupIDs) == 0 {
		return
	}
	now := d.clock.Now()
	recorded, err := d.store.TryRecordOperationRecovery(ctx, cert.ID, now)
	if err != nil {
		d.logger.Warn("record cert recovery notification", "certificate_id", cert.ID, "error", err.Error())
		return
	}
	if !recorded {
		return
	}
	validity, _ := computeValidity(cert, now)
	p := Payload{
		EventType:            EventOperationRecovered,
		ManagedCertificateID: cert.ID,
		CertificateName:      cert.Name,
		Domains:              append([]string(nil), cert.Domains...),
		OperationType:        op.Type,
		OperationID:          op.ID,
		ActiveValidity:       validity,
		OccurredAt:           now,
	}
	d.deliver(ctx, cert, p)
}

// ScanValidityNotifications checks active certificates for seven-day remaining
// and expired reminders. Called from the certificate reconciler hourly scan.
func (d *Dispatcher) ScanValidityNotifications(ctx context.Context) {
	if d == nil {
		return
	}
	certs, err := d.store.ListManagedCertificatesForNotifications(ctx)
	if err != nil {
		d.logger.Warn("list certificates for notifications", "error", err.Error())
		return
	}
	now := d.clock.Now()
	for _, cert := range certs {
		if cert.ActiveVersion == nil || len(cert.NotifyGroupIDs) == 0 {
			continue
		}
		validity, _ := computeValidity(cert, now)
		v := cert.ActiveVersion
		remaining := v.NotAfter.Sub(now)
		if validity == store.CertValidityExpired {
			recorded, err := d.store.TryRecordExpiredNotification(ctx, cert.ID, v.ID, now)
			if err != nil {
				d.logger.Warn("record cert expired notification", "certificate_id", cert.ID, "error", err.Error())
				continue
			}
			if !recorded {
				continue
			}
			p := Payload{
				EventType:            EventExpired,
				ManagedCertificateID: cert.ID,
				CertificateName:      cert.Name,
				Domains:              append([]string(nil), cert.Domains...),
				ActiveValidity:       validity,
				DaysRemaining:        0,
				OccurredAt:           now,
			}
			d.deliver(ctx, cert, p)
			continue
		}
		if remaining <= store.CertNotificationSevenDayThreshold && remaining > 0 {
			if d.sevenDayReminderBlockedByInFlightOperation(ctx, cert) {
				continue
			}
			recorded, err := d.store.TryRecordSevenDayReminder(ctx, cert.ID, v.ID, now)
			if err != nil {
				d.logger.Warn("record cert seven-day reminder", "certificate_id", cert.ID, "error", err.Error())
				continue
			}
			if !recorded {
				continue
			}
			p := Payload{
				EventType:            EventExpiringSoon,
				ManagedCertificateID: cert.ID,
				CertificateName:      cert.Name,
				Domains:              append([]string(nil), cert.Domains...),
				ActiveValidity:       validity,
				DaysRemaining:        daysRemaining(v, now),
				OccurredAt:           now,
			}
			d.deliver(ctx, cert, p)
		}
	}
}

func (d *Dispatcher) sevenDayReminderBlockedByInFlightOperation(ctx context.Context, cert store.ManagedCertificate) bool {
	if _, err := d.store.FindRunningCertificateOperation(ctx, cert.ID, store.CertOpTypeRenew); err == nil {
		return true
	}
	if cert.ActiveVersion != nil {
		if _, err := d.store.FindRunningCertificateOperation(ctx, cert.ID, store.CertOpTypeIssue); err == nil {
			return true
		}
	}
	return false
}

func (d *Dispatcher) deliver(ctx context.Context, cert store.ManagedCertificate, p Payload) {
	channels := d.resolveChannels(ctx, cert.NotifyGroupIDs)
	if len(channels) == 0 {
		d.logger.Warn("cert notification has no resolvable channels",
			"certificate_id", cert.ID, "event_type", p.EventType,
			"notify_group_ids", cert.NotifyGroupIDs)
		return
	}
	jsonBody, humanText, err := renderPayload(p)
	if err != nil {
		d.logger.Warn("render cert notification", "certificate_id", cert.ID, "error", err.Error())
		return
	}
	delivery := notify.Delivery{WebhookJSON: jsonBody, HumanText: humanText}
	for _, ch := range channels {
		ch := ch
		d.deliveries.Add(1)
		go func() {
			defer d.deliveries.Done()
			if err := d.notifier.Deliver(ch, delivery); err != nil {
				warn := fmt.Sprintf("cert %s notify %q failed: %s", p.EventType, channelLabel(ch), err.Error())
				d.logger.Warn("cert notification delivery failed",
					"certificate_id", cert.ID, "event_type", p.EventType,
					"channel_type", ch.Type, "error", err.Error())
				if setErr := d.store.SetCertificateNotificationWarning(ctx, cert.ID, warn, d.clock.Now()); setErr != nil {
					d.logger.Warn("persist cert notification warning",
						"certificate_id", cert.ID, "error", setErr.Error())
				}
			}
		}()
	}
}

func channelLabel(ch store.AlertChannel) string {
	kind := ch.Type
	if kind == "" {
		kind = "webhook"
	}
	if ch.Target != "" {
		return kind + ":" + ch.Target
	}
	return kind
}

func (d *Dispatcher) resolveChannels(ctx context.Context, notifyGroupIDs []string) []store.AlertChannel {
	var channels []store.AlertChannel
	for _, ngID := range notifyGroupIDs {
		ng, err := d.store.GetNotifyGroup(ctx, ngID)
		if err != nil {
			d.logger.Warn("load notify group for cert notification",
				"notify_group_id", ngID, "error", err.Error())
			continue
		}
		channels = append(channels, ng.Channels...)
	}
	return channels
}

func computeValidity(cert store.ManagedCertificate, now time.Time) (validity string, bundleAvailable bool) {
	v := cert.ActiveVersion
	if v == nil {
		return store.CertValidityMissing, false
	}
	if v.RevokedAt != nil || v.RevokePending {
		return store.CertValidityRevoked, false
	}
	bundleAvailable = true
	if now.After(v.NotAfter) {
		return store.CertValidityExpired, bundleAvailable
	}
	if !now.Before(v.NotBefore) {
		window := effectiveRenewalWindow(v.NotBefore, v.NotAfter, cert.RenewBeforeDays)
		if !now.Before(v.NotAfter.Add(-window)) {
			return store.CertValidityRenewalDue, bundleAvailable
		}
	}
	return store.CertValidityValid, bundleAvailable
}

func effectiveRenewalWindow(notBefore, notAfter time.Time, renewBeforeDays uint32) time.Duration {
	cfg := time.Duration(renewBeforeDays) * 24 * time.Hour
	lifetime := notAfter.Sub(notBefore)
	if lifetime <= 0 {
		return cfg
	}
	third := lifetime / 3
	if third < cfg {
		return third
	}
	return cfg
}

// ErrNilDispatcher indicates a nil dispatcher was used.
var ErrNilDispatcher = errors.New("nil certnotify dispatcher")
