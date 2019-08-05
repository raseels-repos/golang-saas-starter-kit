package webcontext

import (
	"context"
	"github.com/gorilla/sessions"
)

// ctxKeySession represents the type of value for the context key.
type ctxKeySession int

// KeySession is used to store/retrieve a Session from a context.Context.
const KeySession ctxKeySession = 1

// Session keys used to store values.
const (
	SessionKeyAccessToken = iota
)

// ContextWithSession appends a universal translator to a context.
func ContextWithSession(ctx context.Context, session *sessions.Session) context.Context {
	return context.WithValue(ctx, KeySession, session)
}

// ContextSession returns the session from a context.
func ContextSession(ctx context.Context) *sessions.Session {
	if s, ok := ctx.Value(KeySession).(*sessions.Session); ok {
		return s
	}
	return nil
}

func ContextAccessToken(ctx context.Context) (string, bool) {
	sess := ContextSession(ctx)
	if sess == nil {
		return "", false
	}
	if sv, ok := sess.Values[SessionKeyAccessToken].(string); ok {
		return sv, true
	}
	return "", false
}

func SessionInit(session *sessions.Session, accessToken string) *sessions.Session {

	session.Values[SessionKeyAccessToken] = accessToken

	return session
}

func SessionUpdateAccessToken(session *sessions.Session, accessToken string) *sessions.Session {
	session.Values[SessionKeyAccessToken] = accessToken
	return session
}

func SessionDestroy(session *sessions.Session) *sessions.Session {

	delete(session.Values, SessionKeyAccessToken)

	return session
}
