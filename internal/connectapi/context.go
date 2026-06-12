package connectapi

import (
	"context"
	"strings"

	"github.com/orvice/neo-line/internal/store"
)

type ctxKey int

const (
	sessionCtxKey ctxKey = iota
	sessionHolderCtxKey
)

// sessionHolder lets the audit interceptor (outermost) observe the session
// resolved by the auth interceptor (inner), so failed and successful calls are
// both attributed.
type sessionHolder struct {
	session *store.Session
}

func withSession(ctx context.Context, s store.Session) context.Context {
	if holder, ok := ctx.Value(sessionHolderCtxKey).(*sessionHolder); ok {
		holder.session = &s
	}
	return context.WithValue(ctx, sessionCtxKey, s)
}

func withSessionHolder(ctx context.Context) (context.Context, *sessionHolder) {
	holder := &sessionHolder{}
	return context.WithValue(ctx, sessionHolderCtxKey, holder), holder
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
