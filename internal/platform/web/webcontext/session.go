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
	//SessionKeyPreferenceDatetimeFormat
	//SessionKeyPreferenceDateFormat
	//SessionKeyPreferenceTimeFormat
	//SessionKeyTimezone
)

func init() {
	//gob.Register(&Session{})
}

// Session represents a user with authentication.
type Session struct {
	*sessions.Session
}

// ContextWithSession appends a universal translator to a context.
func ContextWithSession(ctx context.Context, session *sessions.Session) context.Context {
	return context.WithValue(ctx, KeySession, session)
}

// ContextSession returns the session from a context.
func ContextSession(ctx context.Context) *Session {
	if s, ok := ctx.Value(KeySession).(*Session); ok {
		return s
	}
	return nil
}

func ContextAccessToken(ctx context.Context) (string, bool) {
	return ContextSession(ctx).AccessToken()
}

func (sess *Session) AccessToken() (string, bool) {
	if sess == nil {
		return "", false
	}
	if sv, ok := sess.Values[SessionKeyAccessToken].(string); ok {
		return sv, true
	}
	return "", false
}

/*
func(sess *Session) PreferenceDatetimeFormat() (string, bool) {
	if sess == nil {
		return "", false
	}
	if sv, ok := sess.Values[SessionKeyPreferenceDatetimeFormat].(string); ok {
		return sv, true
	}
	return "", false
}

func(sess *Session) PreferenceDateFormat() (string, bool) {
	if sess == nil {
		return "", false
	}
	if sv, ok := sess.Values[SessionKeyPreferenceDateFormat].(string); ok {
		return sv, true
	}
	return "", false
}

func(sess *Session) PreferenceTimeFormat() (string, bool) {
	if sess == nil {
		return "", false
	}
	if sv, ok := sess.Values[SessionKeyPreferenceTimeFormat].(string); ok {
		return sv, true
	}
	return "", false
}

func(sess *Session) Timezone() (*time.Location, bool) {
	if sess != nil {
		if sv, ok := sess.Values[SessionKeyTimezone].(*time.Location); ok {
			return sv, true
		}
	}

	return nil, false
}
*/

func SessionInit(session *Session, accessToken string) *Session {

	session.Values[SessionKeyAccessToken] = accessToken
	//session.Values[SessionKeyPreferenceDatetimeFormat] = datetimeFormat
	//session.Values[SessionKeyPreferenceDateFormat] = dateFormat
	//session.Values[SessionKeyPreferenceTimeFormat] = timeFormat
	//session.Values[SessionKeyTimezone] = timezone

	return session
}

func SessionDestroy(session *Session) *Session {

	delete(session.Values, SessionKeyAccessToken)

	return session
}
