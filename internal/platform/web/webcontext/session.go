package webcontext

import (
	"context"

	"github.com/gorilla/sessions"
)

// ctxKeySession represents the type of value for the context key.
type ctxKeySession int

// KeySession is used to store/retrieve a Session from a context.Context.
const KeySession ctxKeySession = 1

// KeyAccessToken is used to store the access token for the user in their session.
const KeyAccessToken = "AccessToken"

// KeyUser is used to store the user in the session.
const KeyUser = "User"

// KeyAccount is used to store the account in the session.
const KeyAccount = "Account"

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
	if sv, ok := session.Values[KeyAccessToken].(string); ok {
		return sv, true
	}

	return "", false
}

func SessionUser(session *sessions.Session) ( interface{}, bool) {
	if sv, ok := session.Values[KeyUser]; ok && sv != nil {
		return sv, true
	}

	return nil, false
}

func SessionAccount(session *sessions.Session) (interface{}, bool) {
	if sv, ok := session.Values[KeyAccount];  ok && sv != nil {
		return sv, true
	}

	return nil, false
}

func SessionInit(session *sessions.Session, accessToken string, usr interface{}, acc  interface{}) *sessions.Session {

	if accessToken != "" {
		session.Values[KeyAccessToken] = accessToken
	} else {
		delete(session.Values, KeyAccessToken)
	}

	if usr != nil {
		session.Values[KeyUser] = usr
	} else {
		delete(session.Values, KeyUser)
	}

	if acc != nil {
		session.Values[KeyAccount] = acc
	} else {
		delete(session.Values, KeyAccount)
	}

	return session
}

func SessionDestroy(session *sessions.Session) *sessions.Session {

	delete(session.Values, KeyAccessToken)
	delete(session.Values, KeyUser)
	delete(session.Values, KeyAccount)

	return session
}

