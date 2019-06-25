package auth

import (
	"crypto/rsa"
	"fmt"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
)

// KeyFunc is used to map a JWT key id (kid) to the corresponding public key.
// It is a requirement for creating an Authenticator.
//
// * Private keys should be rotated. During the transition period, tokens
// signed with the old and new keys can coexist by looking up the correct
// public key by key id (kid).
//
// * Key-id-to-public-key resolution is usually accomplished via a public JWKS
// endpoint. See https://auth0.com/docs/jwks for more details.
type KeyFunc func(keyID string) (*rsa.PublicKey, error)

// NewKeyFunc is a multiple implementation of KeyFunc that
// supports a map of keys.
func NewKeyFunc(keys map[string]*PrivateKey) KeyFunc {
	return func(kid string) (*rsa.PublicKey, error) {
		key, ok := keys[kid]
		if !ok {
			return nil, fmt.Errorf("unrecognized kid %q", kid)
		}
		return key.Public().(*rsa.PublicKey), nil
	}
}

// Authenticator is used to authenticate clients. It can generate a token for a
// set of user claims and recreate the claims by parsing the token.
type Authenticator struct {
	privateKey *PrivateKey
	keyID      string
	algorithm  string
	kf         KeyFunc
	parser     *jwt.Parser
	Storage    Storage
}

// PrivateKey is used to associate a private key with a keyID and algorithm.
type PrivateKey struct {
	*rsa.PrivateKey
	keyID     string
	algorithm string
}

// NewAuthenticator creates an *Authenticator for use.
// key expiration is optional to filter out old keys
// It will error if:
// - The specified algorithm is unsupported.
// - No current private key exists.
func NewAuthenticator(storage Storage, now time.Time) (*Authenticator, error) {

	// Lookup function to be used by the middleware to validate the kid and
	// Return the associated public key.
	publicKeyLookup := NewKeyFunc(storage.Keys())

	// Validate the globally defined encryption algorithm is valid.
	if jwt.GetSigningMethod(algorithm) == nil {
		return nil, errors.Errorf("unknown algorithm %v", algorithm)
	}

	// Create the token parser to use. The algorithm used to sign the JWT must be
	// validated to avoid a critical vulnerability:
	// https://auth0.com/blog/critical-vulnerabilities-in-json-web-token-libraries/
	parser := jwt.Parser{
		ValidMethods: []string{algorithm},
	}

	// Load the current key from the storage engine.
	curKey := storage.Current()
	if curKey == nil {
		return nil, errors.New("Missing private key")
	}

	a := Authenticator{
		privateKey: curKey,
		keyID:      curKey.keyID,
		algorithm:  algorithm,
		kf:         publicKeyLookup,
		parser:     &parser,
	}

	return &a, nil
}

// GenerateToken generates a signed JWT token string representing the user Claims.
func (a *Authenticator) GenerateToken(claims Claims) (string, error) {
	method := jwt.GetSigningMethod(a.algorithm)

	tkn := jwt.NewWithClaims(method, claims)
	tkn.Header["kid"] = a.keyID

	str, err := tkn.SignedString(a.privateKey.PrivateKey)
	if err != nil {
		return "", errors.Wrap(err, "signing token")
	}

	return str, nil
}

// ParseClaims recreates the Claims that were used to generate a token. It
// verifies that the token was signed using our key.
func (a *Authenticator) ParseClaims(tknStr string) (Claims, error) {

	// f is a function that returns the public key for validating a token. We use
	// the parsed (but unverified) token to find the key id. That ID is passed to
	// our KeyFunc to find the public key to use for verification.
	f := func(t *jwt.Token) (interface{}, error) {
		kid, ok := t.Header["kid"]
		if !ok {
			return nil, errors.New("Missing key id (kid) in token header")
		}
		kidStr, ok := kid.(string)
		if !ok {
			return nil, errors.New("Token key id (kid) must be string")
		}

		return a.kf(kidStr)
	}

	var claims Claims
	tkn, err := a.parser.ParseWithClaims(tknStr, &claims, f)
	if err != nil {
		return Claims{}, errors.Wrap(err, "parsing token")
	}

	if !tkn.Valid {
		return Claims{}, errors.New("Invalid token")
	}

	return claims, nil
}
