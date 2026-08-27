package connectapi

import (
	"errors"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/orvice/neo-line/internal/certmanager"
	"github.com/orvice/neo-line/internal/store"
)

// toConnectError maps store-layer errors onto Connect status codes, mirroring
// the HTTP status mapping the legacy REST API used.
func toConnectError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case store.IsNotFound(err):
		return connect.NewError(connect.CodeNotFound, errors.New("not found"))
	case errors.Is(err, store.ErrInvalidCredentials):
		return connect.NewError(connect.CodeUnauthenticated, errors.New("invalid email or password"))
	case errors.Is(err, store.ErrGroupNameTaken), errors.Is(err, store.ErrNotifyGroupNameTaken), errors.Is(err, store.ErrDNSProviderAccountNameTaken), errors.Is(err, store.ErrCertificateIssuerNameTaken), errors.Is(err, store.ErrManagedCertificateNameTaken):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case errors.Is(err, store.ErrInvalidGroupIDs), errors.Is(err, store.ErrInvalidNotifyGroupIDs), errors.Is(err, store.ErrInvalidServerIDs):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, store.ErrInvalidDNSProvider), errors.Is(err, store.ErrInvalidPropagationTimeout), errors.Is(err, certmanager.ErrTokenRequired), errors.Is(err, certmanager.ErrManagedCertificateNameRequired), errors.Is(err, certmanager.ErrInvalidDomains), errors.Is(err, certmanager.ErrTooManyDomains), errors.Is(err, certmanager.ErrInvalidKeyType), errors.Is(err, certmanager.ErrCertificateIssuerRequired), errors.Is(err, certmanager.ErrDNSAccountRequired):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, store.ErrInvalidCertificateIssuerCAType), errors.Is(err, certmanager.ErrIssuerNameRequired), errors.Is(err, certmanager.ErrIssuerEmailRequired), errors.Is(err, certmanager.ErrTermsOfServiceRequired), errors.Is(err, certmanager.ErrEABRequired), errors.Is(err, certmanager.ErrCustomDirectoryRequired), errors.Is(err, certmanager.ErrInvalidDirectoryURL), errors.Is(err, certmanager.ErrIssuerRegistrationPending), errors.Is(err, certmanager.ErrIssuerNotReady):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, certmanager.ErrIssueFieldsLocked):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, certmanager.ErrBundleNotAvailable):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, store.ErrCertificateIssuerNotRetryable):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, certmanager.ErrCloudflareTokenInvalid):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case err.Error() == "invalid page_token":
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		return internalError(err)
	}
}

// internalError logs the real error server-side and returns a generic message
// so store/driver details never reach clients.
func internalError(err error) error {
	slog.Error("internal error", "error", err)
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}
