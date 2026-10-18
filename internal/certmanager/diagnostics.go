package certmanager

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	cloudflareapi "github.com/cloudflare/cloudflare-go"
	legoacme "github.com/go-acme/lego/v4/acme"
	"github.com/orvice/neo-line/internal/store"
)

// operationStageError adds a safe execution stage without changing the
// underlying error returned to callers or persisted in the operation summary.
type operationStageError struct {
	stage string
	err   error
}

func (e *operationStageError) Error() string {
	if e == nil || e.err == nil {
		return e.stage
	}
	return e.err.Error()
}

func (e *operationStageError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *operationStageError) OperationStage() string {
	if e == nil {
		return ""
	}
	return e.stage
}

func withOperationStage(stage string, err error) error {
	if err == nil || operationStageOf(err) != "" {
		return err
	}
	return &operationStageError{stage: stage, err: err}
}

func operationStageOf(err error) string {
	for err != nil {
		if staged, ok := err.(*operationStageError); ok {
			return staged.stage
		}
		err = errors.Unwrap(err)
	}
	return ""
}

func operationLogAttrs(op store.CertificateOperation) []any {
	attrs := []any{
		"operation_id", op.ID,
		"managed_certificate_id", op.ManagedCertificateID,
		"operation_type", op.Type,
		"operation_status", op.Status,
		"attempt_count", op.AttemptCount,
		"consecutive_failures", op.ConsecutiveFailures,
		"issuer_id", op.ConfigSnapshot.CertificateIssuerID,
		"dns_provider_account_id", op.ConfigSnapshot.DNSProviderAccountID,
		"domain_count", len(op.ConfigSnapshot.Domains),
		"domains", append([]string(nil), op.ConfigSnapshot.Domains...),
		"pending_txt_count", len(op.PendingTXTRecords),
		"lease_owner", op.LeaseOwner,
	}
	if op.LeaseExpiresAt != nil {
		attrs = append(attrs, "lease_expires_at", *op.LeaseExpiresAt)
	}
	if op.DeadlineAt != nil {
		attrs = append(attrs, "deadline_at", *op.DeadlineAt)
	}
	return attrs
}

// safeErrorAttrs deliberately omits error.Error(). It keeps structured error
// information useful for diagnosis without risking tokens, URLs, or PEM data.
func safeErrorAttrs(err error) []any {
	if err == nil {
		return nil
	}
	attrs := []any{"error_class", safeErrorClass(err)}

	var acmeErr *legoacme.ProblemDetails
	if errors.As(err, &acmeErr) && acmeErr != nil {
		attrs = append(attrs, "acme_http_status", acmeErr.HTTPStatus)
		if acmeErr.Type != "" {
			attrs = append(attrs, "acme_problem_type", acmeErr.Type)
		}
	}

	var cloudflareErr *cloudflareapi.Error
	if errors.As(err, &cloudflareErr) && cloudflareErr != nil {
		attrs = append(attrs,
			"cloudflare_http_status", cloudflareErr.StatusCode,
			"cloudflare_error_type", cloudflareErr.Type,
			"cloudflare_error_codes", append([]int(nil), cloudflareErr.ErrorCodes...),
		)
		if cloudflareErr.RayID != "" {
			attrs = append(attrs, "cloudflare_ray_id", cloudflareErr.RayID)
		}
	}

	var networkErr net.Error
	if errors.As(err, &networkErr) {
		attrs = append(attrs, "network_timeout", networkErr.Timeout())
	}
	return attrs
}

func safeErrorClass(err error) string {
	if errors.Is(err, context.Canceled) {
		return "context_canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "context_deadline_exceeded"
	}
	if errors.Is(err, ErrIssuerNotReady) {
		return "issuer_not_ready"
	}
	if errors.Is(err, ErrIssuerAccountURIRequired) {
		return "acme_account_unavailable"
	}
	if errors.Is(err, store.ErrCertificateOperationConflict) {
		return "operation_conflict"
	}

	var cloudflareErr *cloudflareapi.Error
	if errors.As(err, &cloudflareErr) && cloudflareErr != nil {
		switch cloudflareErr.StatusCode {
		case http.StatusUnauthorized:
			return "cloudflare_authentication"
		case http.StatusForbidden:
			return "cloudflare_authorization"
		case http.StatusNotFound:
			return "cloudflare_not_found"
		case http.StatusTooManyRequests:
			return "cloudflare_rate_limit"
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return "cloudflare_service"
		default:
			return "cloudflare_api"
		}
	}

	var acmeErr *legoacme.ProblemDetails
	if errors.As(err, &acmeErr) && acmeErr != nil {
		return "acme_problem"
	}

	var networkErr net.Error
	if errors.As(err, &networkErr) {
		if networkErr.Timeout() {
			return "network_timeout"
		}
		return "network_error"
	}

	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "tls"), strings.Contains(lower, "x509"):
		return "tls_error"
	case strings.Contains(lower, "cloudflare"):
		return "cloudflare_error"
	case strings.Contains(lower, "dns") || strings.Contains(lower, "zone"):
		return "dns_error"
	case strings.Contains(lower, "acme") || strings.Contains(lower, "order"):
		return "acme_error"
	case strings.Contains(lower, "mongo") || strings.Contains(lower, "store"):
		return "storage_error"
	default:
		return "unknown_error"
	}
}

func safeURLHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return "invalid"
	}
	return u.Hostname()
}

func dnsProviderStep(err error) string {
	if err == nil {
		return "unknown"
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "create txt record"):
		return "create_txt_record"
	case strings.Contains(lower, "find zone") || strings.Contains(lower, "zone for domain"):
		return "find_zone"
	case strings.Contains(lower, "unknown record"):
		return "find_record"
	case strings.Contains(lower, "delete"):
		return "delete_record"
	default:
		return "provider"
	}
}

func acmeRequestKind(u *url.URL) string {
	if u == nil {
		return "unknown"
	}
	path := strings.ToLower(u.EscapedPath())
	switch {
	case strings.HasSuffix(path, "/directory"):
		return "directory"
	case strings.Contains(path, "/new-nonce"):
		return "new_nonce"
	case strings.Contains(path, "/new-account"):
		return "new_account"
	case strings.Contains(path, "/new-order"):
		return "new_order"
	case strings.Contains(path, "/authz/"):
		return "authorization"
	case strings.Contains(path, "/challenge/"):
		return "challenge"
	case strings.Contains(path, "/finalize/"):
		return "finalize"
	case strings.HasSuffix(path, "/certificate"):
		return "certificate"
	case strings.HasSuffix(path, "/revoke-cert"):
		return "revoke_certificate"
	default:
		return "other"
	}
}

func newACMEDiagnosticClient(client *http.Client, logger *slog.Logger, req IssueRequest) *http.Client {
	if logger == nil {
		return client
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	clone := *client
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	clone.Transport = acmeDiagnosticRoundTripper{
		base:   base,
		logger: logger,
		attrs: []any{
			"operation_id", req.OperationID,
			"managed_certificate_id", req.ManagedCertificateID,
			"attempt_count", req.AttemptCount,
		},
	}
	return &clone
}

type acmeDiagnosticRoundTripper struct {
	base   http.RoundTripper
	logger *slog.Logger
	attrs  []any
}

func (t acmeDiagnosticRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	started := time.Now()
	resp, err := t.base.RoundTrip(req)
	attrs := append([]any(nil), t.attrs...)
	attrs = append(attrs,
		"acme_request_kind", acmeRequestKind(req.URL),
		"acme_host", req.URL.Hostname(),
		"http_method", req.Method,
		"duration_ms", time.Since(started).Milliseconds(),
	)
	if err != nil {
		attrs = append(attrs, safeErrorAttrs(err)...)
		t.logger.ErrorContext(req.Context(), "acme HTTP request failed", attrs...)
		return nil, err
	}

	attrs = append(attrs, "http_status", resp.StatusCode)
	if resp.StatusCode >= http.StatusBadRequest {
		t.logger.WarnContext(req.Context(), "acme HTTP request returned error", attrs...)
	} else {
		t.logger.InfoContext(req.Context(), "acme HTTP request completed", attrs...)
	}
	return resp, nil
}
