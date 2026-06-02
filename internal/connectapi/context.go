package connectapi

import (
	"context"
	"strings"

	"github.com/orvice/neo-line/internal/store"
)

type ctxKey int

const sessionCtxKey ctxKey = iota

func withSession(ctx context.Context, s store.Session) context.Context {
	return context.WithValue(ctx, sessionCtxKey, s)
}

// sessionFromContext returns the authenticated session attached by the auth
// interceptor. Public procedures have no session.
func sessionFromContext(ctx context.Context) (store.Session, bool) {
	s, ok := ctx.Value(sessionCtxKey).(store.Session)
	return s, ok
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}
