package auth

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
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
func NewKeyFunc(keys map[string]*rsa.PrivateKey) KeyFunc {
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
	privateKey *rsa.PrivateKey
	keyID      string
	algorithm  string
	kf         KeyFunc
	parser     *jwt.Parser
}

// NewAuthenticator creates an *Authenticator for use.
// key expiration is optional to filter out old keys
// It will error if:
// - The aws session is nil.
// - The aws secret id is blank.
// - The specified algorithm is unsupported.
func NewAuthenticator(awsSession *session.Session, awsSecretID string, now time.Time, keyExpiration time.Duration) (*Authenticator, error) {
	if awsSession == nil {
		return nil, errors.New("aws session cannot be nil")
	}

	if awsSecretID == "" {
		return nil, errors.New("aws secret id cannot be empty")
	}

	if now.IsZero() {
		now = time.Now().UTC()
	}

	// Time threshold to stop loading keys, any key with a created date
	// before this value will not be loaded.
	var disabledCreatedDate time.Time

	// Time threshold to create a new key. If a current key exists and the
	// created date of the key is before this value, a new key will be created.
	var activeCreatedDate time.Time

	// If an expiration duration is included, convert to past time from now.
	if keyExpiration.Seconds() != 0 {
		// Ensure the expiration is a time in the past for comparison below.
		if keyExpiration.Seconds() > 0 {
			keyExpiration = keyExpiration * -1
		}
		// Stop loading keys when the created date exceeds two times the key expiration
		disabledCreatedDate = now.UTC().Add(keyExpiration * 2)

		// Time used to determine when a new key should be created.
		activeCreatedDate = now.UTC().Add(keyExpiration)
	}

	// Init new AWS Secret Manager using provided AWS session.
	secretManager := secretsmanager.New(awsSession)

	// A List of version ids for the stored secret. All keys will be stored under
	// the same name in AWS secret manager. We still want to load old keys for a
	// short period of time to ensure any requests in flight have the opportunity
	// to be completed.
	var versionIds []string

	// Exec call to AWS secret manager to return a list of version ids for the
	// provided secret ID.
	listParams := &secretsmanager.ListSecretVersionIdsInput{
		SecretId: aws.String(awsSecretID),
	}
	err := secretManager.ListSecretVersionIdsPages(listParams,
		func(page *secretsmanager.ListSecretVersionIdsOutput, lastPage bool) bool {
			for _, v := range page.Versions {
				// When disabled CreatedDate is not empty, compare the created date
				// for each key version to the disabled cut off time.
				if !disabledCreatedDate.IsZero() && v.CreatedDate != nil && !v.CreatedDate.IsZero() {
					// Skip any version ids that are less than the expiration time.
					if v.CreatedDate.UTC().Unix() < disabledCreatedDate.UTC().Unix() {
						continue
					}
				}

				if v.VersionId != nil {
					versionIds = append(versionIds, *v.VersionId)
				}
			}
			return !lastPage
		},
	)

	// Flag whether the secret exists and update needs to be used
	// instead of create.
	var awsSecretIDNotFound bool
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case secretsmanager.ErrCodeResourceNotFoundException:
				awsSecretIDNotFound = true
			}
		}

		if !awsSecretIDNotFound {
			return nil, errors.Wrapf(err, "aws list secret version ids for secret ID %s failed", awsSecretID)
		}
	}

	// Map of keys stored by version id. version id is kid.
	keyContents := make(map[string][]byte)

	// The current key id if there is an active one.
	var curKeyId string

	// If the list of version ids is not empty, load the keys from secret manager.
	if len(versionIds) > 0 {
		// The max created data to determine the most recent key.
		var lastCreatedDate time.Time

		for _, id := range versionIds {
			res, err := secretManager.GetSecretValue(&secretsmanager.GetSecretValueInput{
				SecretId:  aws.String(awsSecretID),
				VersionId: aws.String(id),
			})
			if err != nil {
				return nil, errors.Wrapf(err, "aws secret id %s, version id %s value failed", awsSecretID, id)
			}

			if len(res.SecretBinary) == 0 {
				continue
			}

			keyContents[*res.VersionId] = res.SecretBinary

			if lastCreatedDate.IsZero() || res.CreatedDate.UTC().Unix() > lastCreatedDate.UTC().Unix() {
				curKeyId = *res.VersionId
				lastCreatedDate = res.CreatedDate.UTC()
			}
		}

		//
		if !activeCreatedDate.IsZero() && lastCreatedDate.UTC().Unix() < activeCreatedDate.UTC().Unix() {
			curKeyId = ""
		}
	}

	// If there are no keys stored in secret manager, create a new one or
	// if the current key needs to be rotated, generate a new key and update the secret.
	// @TODO: When a new key is generated and there are multiple instances of the service running
	//  		its possible based on the key expiration set that requests fail because keys are only
	// 			refreshed on instance launch. Could store keys in a kv store and update that value
	// 			when new keys are generated
	if len(keyContents) == 0 || curKeyId == "" {
		privateKey, err := keygen()
		if err != nil {
			return nil, errors.Wrap(err, "failed to generate new private key")
		}

		if awsSecretIDNotFound {
			res, err := secretManager.CreateSecret(&secretsmanager.CreateSecretInput{
				Name:         aws.String(awsSecretID),
				SecretBinary: privateKey,
			})
			if err != nil {
				return nil, errors.Wrap(err, "failed to create new secret with private key")
			}
			curKeyId = *res.VersionId
		} else {
			res, err := secretManager.UpdateSecret(&secretsmanager.UpdateSecretInput{
				SecretId:     aws.String(awsSecretID),
				SecretBinary: privateKey,
			})
			if err != nil {
				return nil, errors.Wrap(err, "failed to create new secret with private key")
			}
			curKeyId = *res.VersionId
		}

		keyContents[curKeyId] = privateKey
	}

	// Map of keys by kid (version id).
	keys := make(map[string]*rsa.PrivateKey)

	// The current active key to be used.
	var curPrivateKey *rsa.PrivateKey

	// Loop through all the key bytes and load the private key.
	for kid, keyContent := range keyContents {
		key, err := jwt.ParseRSAPrivateKeyFromPEM(keyContent)
		if err != nil {
			return nil, errors.Wrap(err, "parsing auth private key")
		}
		keys[kid] = key
		if kid == curKeyId {
			curPrivateKey = key
		}
	}

	// Lookup function to be used by the middleware to validate the kid and
	// Return the associated public key.
	publicKeyLookup := NewKeyFunc(keys)

	// Algorithm to be used to for the private key.
	algorithm := "RS256"
	if jwt.GetSigningMethod(algorithm) == nil {
		return nil, errors.Errorf("unknown algorithm %v", algorithm)
	}

	// Create the token parser to use. The algorithm used to sign the JWT must be
	// validated to avoid a critical vulnerability:
	// https://auth0.com/blog/critical-vulnerabilities-in-json-web-token-libraries/
	parser := jwt.Parser{
		ValidMethods: []string{algorithm},
	}

	a := Authenticator{
		privateKey: curPrivateKey,
		keyID:      curKeyId,
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

	str, err := tkn.SignedString(a.privateKey)
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

// keygen creates an x509 private key for signing auth tokens.
func keygen() ([]byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return []byte{}, errors.Wrap(err, "generating keys")
	}

	block := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	buf := new(bytes.Buffer)
	if err := pem.Encode(buf, &block); err != nil {
		return []byte{}, errors.Wrap(err, "encoding to private file")
	}

	return buf.Bytes(), nil
}
