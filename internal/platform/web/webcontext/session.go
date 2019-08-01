package webcontext

import (
	"context"

	"github.com/gorilla/sessions"
)

// ctxKeySession represents the type of value for the context key.
type ctxKeySession int

// KeySession is used to store/retrieve a Session from a context.Context.
const KeySession ctxKeySession = 1

// ContextWithSession appends a universal translator to a context.
func ContextWithSession(ctx context.Context, session *sessions.Session) context.Context {
	return context.WithValue(ctx, KeySession, session)
}

// ContextSession returns the session from a context.
func ContextSession(ctx context.Context) *sessions.Session {
	return ctx.Value(KeySession).(*sessions.Session)
}

func ContextAccessToken(ctx context.Context) (string, bool) {
	session := ContextSession(ctx)

	return SessionAccessToken(session)
}

func SessionAccessToken(session *sessions.Session) (string, bool) {
	if sv, ok := session.Values["AccessToken"].(string); ok {
		return sv, true
	}

	return "", false
}

func SessionWithAccessToken(session *sessions.Session, accessToken string) *sessions.Session {

	if accessToken != "" {
		session.Values["AccessToken"] = accessToken
	} else {
		delete(session.Values, "AccessToken")
	}

	return session
}
