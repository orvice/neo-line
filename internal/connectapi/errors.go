package connectapi

import (
	"errors"
	"log/slog"

	"connectrpc.com/connect"
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
	case errors.Is(err, store.ErrGroupNameTaken), errors.Is(err, store.ErrNotifyGroupNameTaken):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case errors.Is(err, store.ErrInvalidGroupIDs), errors.Is(err, store.ErrInvalidNotifyGroupIDs):
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
