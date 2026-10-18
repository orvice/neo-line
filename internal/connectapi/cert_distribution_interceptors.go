package connectapi

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/orvice/neo-line/internal/metric"
	"github.com/orvice/neo-line/internal/store"
	"github.com/orvice/neo-line/pkg/proto/neoline/v1"
	"github.com/orvice/neo-line/pkg/proto/neoline/v1/neolinev1connect"
)

const (
	certDistributionRateLimit = 120
	certDistributionWindow    = time.Minute
)

// certDistributionAuditInterceptor records successful bundle downloads and all
// distribution auth failures. Successful ListCertificates calls are excluded.
func (s *Service) certDistributionAuditInterceptor() connect.UnaryInterceptorFunc {
	logger := slog.Default().With("component", "audit", "source", "server_cert")
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			start := time.Now()
			ctx, holder := withCertAccessTokenHolder(ctx)
			res, err := next(ctx, req)
			procedure := req.Spec().Procedure

			if err == nil && procedure == neolinev1connect.ServerCertificateServiceListCertificatesProcedure {
				metric.ServerCertListTotal.Inc()
				return res, err
			}
			if err == nil && procedure == neolinev1connect.ServerCertificateServiceGetCertificateBundleProcedure {
				metric.ServerCertBundleDownloadTotal.Inc()
			}

			audit := certDistributionAuditEntry(procedure, req, res, err, holder, start)
			if audit == nil {
				return res, err
			}
			audit.RemoteIP = req.Peer().Addr
			audit.UserAgent = req.Header().Get("User-Agent")
			audit.DurationMS = time.Since(start).Milliseconds()
			audit.OccurredAt = start.UTC()

			logger.Info("server cert distribution",
				"action", audit.Action,
				"procedure", procedure,
				"success", audit.Success,
				"status", audit.StatusCode,
				"server_id", audit.Metadata["server_id"],
				"certificate_id", audit.ResourceID,
				"version_id", audit.Metadata["version_id"],
				"token_prefix", audit.TokenPrefix,
			)

			if auditor, ok := s.store.(interface {
				SaveAuditLog(context.Context, store.AuditLog) error
			}); ok && auditor != nil {
				auditCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				if saveErr := auditor.SaveAuditLog(auditCtx, *audit); saveErr != nil {
					logger.Error("failed to save server cert audit log", "error", saveErr.Error())
				}
			}
			return res, err
		}
	}
}

func certDistributionAuditEntry(procedure string, req connect.AnyRequest, res connect.AnyResponse, err error, holder *certAccessTokenHolder, start time.Time) *store.AuditLog {
	code := connect.CodeOf(err)
	isAuthFailure := code == connect.CodeUnauthenticated
	isBundleDownload := procedure == neolinev1connect.ServerCertificateServiceGetCertificateBundleProcedure && err == nil

	if !isAuthFailure && !isBundleDownload {
		return nil
	}

	entry := &store.AuditLog{
		Source:     "server_cert",
		Method:     "POST",
		Path:       procedure,
		StatusCode: auditStatusCode(err),
		Success:    err == nil,
		Metadata:   map[string]string{},
	}
	if err != nil {
		entry.Error = err.Error()
	}
	if holder.token != nil {
		entry.TokenPrefix = holder.token.Prefix
		entry.Metadata["server_id"] = holder.token.ServerID
	} else if prefix := nlctTokenPrefix(req.Header().Get("Authorization")); prefix != "" {
		entry.TokenPrefix = prefix
	}

	if isBundleDownload {
		entry.Action = "download"
		entry.ResourceType = "managed_certificate"
		if res != nil {
			if msg, ok := res.Any().(*neolinev1.ServerCertificateServiceGetCertificateBundleResponse); ok && msg.GetBundle() != nil {
				entry.ResourceID = msg.GetBundle().GetManagedCertificateId()
				entry.Metadata["version_id"] = msg.GetBundle().GetVersionId()
			}
		}
		type certID interface{ GetManagedCertificateId() string }
		if entry.ResourceID == "" {
			if msg, ok := req.Any().(certID); ok {
				entry.ResourceID = msg.GetManagedCertificateId()
			}
		}
		return entry
	}

	entry.Action = "auth"
	return entry
}

func nlctTokenPrefix(authHeader string) string {
	token := bearerToken(authHeader)
	if !strings.HasPrefix(token, storeCertAccessTokenPrefix) || len(token) < len(storeCertAccessTokenPrefix)+8 {
		return ""
	}
	return token[:len(storeCertAccessTokenPrefix)+8]
}

// certDistributionAuthInterceptor validates Authorization: Bearer nlct_<secret>.
func (s *Service) certDistributionAuthInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			token := bearerToken(req.Header().Get("Authorization"))
			if token == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing bearer token"))
			}
			if !strings.HasPrefix(token, storeCertAccessTokenPrefix) {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid or expired token"))
			}
			record, err := s.store.LookupCertificateAccessToken(ctx, token)
			if err != nil {
				if store.IsNotFound(err) {
					return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid or expired token"))
				}
				slog.ErrorContext(ctx, "lookup certificate access token", "error", err)
				return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
			}
			ctx = withCertAccessToken(ctx, record)
			if holder, ok := ctx.Value(certAccessTokenHolderCtxKey).(*certAccessTokenHolder); ok {
				holder.token = &record
			}
			return next(ctx, req)
		}
	}
}

// certDistributionRateLimitInterceptor enforces per-token rate limits via Redis.
// Redis failures fail-open.
func (s *Service) certDistributionRateLimitInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			token, ok := certAccessTokenFromContext(ctx)
			if !ok {
				return next(ctx, req)
			}
			allowed, err := s.store.RateLimitAllow(ctx, token.ID, certDistributionRateLimit, certDistributionWindow)
			if err != nil {
				slog.ErrorContext(ctx, "certificate distribution rate limit check failed; allowing request", "error", err, "token_id", token.ID)
			}
			if !allowed {
				return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("rate limit exceeded"))
			}
			return next(ctx, req)
		}
	}
}
