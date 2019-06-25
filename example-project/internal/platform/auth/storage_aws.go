package auth

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
)

// StorageAws is a storage engine that uses AWS Secrets Manager to persist private keys.
type StorageAws struct {
	keyExpiration time.Duration
	// Map of keys by kid (version id).
	keys map[string]*PrivateKey
	// The current active key to be used.
	curPrivateKey *PrivateKey
}

// Keys returns a map of private keys by kID.
func (s *StorageAws) Keys() map[string]*PrivateKey {
	if s == nil || s.keys == nil {
		return map[string]*PrivateKey{}
	}
	return s.keys
}

// Current returns the most recently generated private key.
func (s *StorageAws) Current() *PrivateKey {
	if s == nil {
		return nil
	}
	return s.curPrivateKey
}

// NewAuthenticatorAws is a help function that inits a new Authenticator
// using the AWS storage.
func NewAuthenticatorAws(awsSession *session.Session, awsSecretID string, now time.Time, keyExpiration time.Duration) (*Authenticator, error) {
	storage, err := NewStorageAws(awsSession, awsSecretID, now, keyExpiration)
	if err != nil {
		return nil, err
	}

	return NewAuthenticator(storage, time.Now().UTC())
}

// NewStorageAws implements the interface Storage to support persisting private keys
// to AWS Secrets Manager.
// It will error if:
// - The aws session is nil.
// - The aws secret id is blank.
func NewStorageAws(awsSession *session.Session, awsSecretID string, now time.Time, keyExpiration time.Duration) (*StorageAws, error) {
	if awsSession == nil {
		return nil, errors.New("aws session cannot be nil")
	}

	if awsSecretID == "" {
		return nil, errors.New("aws secret id cannot be empty")
	}

	storage := &StorageAws{
		keyExpiration: keyExpiration,
		keys:          make(map[string]*PrivateKey),
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
		privateKey, err := KeyGen()
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

	// Loop through all the key bytes and load the private key.
	for kid, key := range keyContents {
		pk, err := jwt.ParseRSAPrivateKeyFromPEM(key)
		if err != nil {
			return nil, errors.Wrap(err, "parsing auth private key")
		}

		storage.keys[kid] = &PrivateKey{
			PrivateKey: pk,
			keyID:      kid,
			algorithm:  algorithm,
		}

		if kid == curKeyId {
			storage.curPrivateKey = storage.keys[kid]
		}
	}

	return storage, nil
}
