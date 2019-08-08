package webcontext

import (
	"context"

	"github.com/gorilla/sessions"
	"github.com/pborman/uuid"
)

// ctxKeySession represents the type of value for the context key.
type ctxKeySession int

// KeySession is used to store/retrieve a Session from a context.Context.
const KeySession ctxKeySession = 1

// Session keys used to store values.
const (
	SessionKeyAccessToken = iota
)

// KeySessionID is the key used to store the ID of the session in its values.
const KeySessionID = "_sid"

// ContextWithSession appends a universal translator to a context.
func ContextWithSession(ctx context.Context, session *sessions.Session) context.Context {
	return context.WithValue(ctx, KeySession, session)
}

// ContextSession returns the session from a context.
func ContextSession(ctx context.Context) *sessions.Session {
	if s, ok := ctx.Value(KeySession).(*sessions.Session); ok {
		if sid, ok := s.Values[KeySessionID].(string); ok {
			s.ID = sid
		}

		return s
	}
	return nil
}

// ContextAccessToken returns the JWT access token from the context session.
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

// SessionInit creates a new session with a valid JWT access token.
func SessionInit(session *sessions.Session, accessToken string) *sessions.Session {

	// Always create a new session ID to ensure when session ID is being used as a cache key, logout/login
	// forces any cache to be flushed.
	session.ID = uuid.NewRandom().String()

	// Not sure why sessions.Session has the ID prop but it is not persisted by default.
	session.Values[KeySessionID] = session.ID

	session.Values[SessionKeyAccessToken] = accessToken

	return session
}

// SessionUpdateAccessToken updates the JWT access token stored in the session.
func SessionUpdateAccessToken(session *sessions.Session, accessToken string) *sessions.Session {
	session.Values[SessionKeyAccessToken] = accessToken
	return session
}

// SessionDestroy removes the access token from the session which revokes authentication for the user.
func SessionDestroy(session *sessions.Session) *sessions.Session {

	delete(session.Values, SessionKeyAccessToken)

	return session
}
