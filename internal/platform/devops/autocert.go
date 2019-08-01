package devops

import (
	"context"
	"log"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/pkg/errors"
	"golang.org/x/crypto/acme/autocert"
)

// SecretManagerAutocertCache implements the autocert.Cache interface for AWS Secrets Manager that is used by Manager
// to store and retrieve previously obtained certificates and other account data as opaque blobs.
type SecretManagerAutocertCache struct {
	awsSession   *session.Session
	log          *log.Logger
	secretPrefix string
	cache        autocert.Cache
}

// NewSecretManagerAutocertCache provides the functionality to keep config files sync'd between running tasks and across deployments.
func NewSecretManagerAutocertCache(log *log.Logger, awsSession *session.Session, secretPrefix string, cache autocert.Cache) (*SecretManagerAutocertCache, error) {
	return &SecretManagerAutocertCache{
		awsSession,
		log,
		secretPrefix,
		cache,
	}, nil
}

// Get returns a certificate data for the specified key.
// If there's no such key, Get returns ErrCacheMiss.
func (c *SecretManagerAutocertCache) Get(ctx context.Context, key string) ([]byte, error) {

	// Check short term cache.
	if c.cache != nil {
		v, err := c.cache.Get(ctx, key)
		if err != nil && err != autocert.ErrCacheMiss {
			return nil, errors.WithStack(err)
		} else if len(v) > 0 {
			return v, nil
		}
	}

	secretID := filepath.Join(c.secretPrefix, key)

	// Load the secret by ID from Secrets Manager.
	res, err := SecretManagerGetString(c.awsSession, secretID)
	if err != nil {
		if err == ErrSecreteNotFound {
			return nil, autocert.ErrCacheMiss
		}
		return nil, err
	}

	log.Printf("AWS Secrets Manager : Secret %s found", secretID)

	return []byte(res), nil
}

// Put stores the data in the cache under the specified key.
// Underlying implementations may use any data storage format,
// as long as the reverse operation, Get, results in the original data.
func (c *SecretManagerAutocertCache) Put(ctx context.Context, key string, data []byte) error {

	secretID := filepath.Join(c.secretPrefix, key)

	err := SecretManagerPutString(c.awsSession, secretID, string(data))
	if err != nil {
		return err
	}

	log.Printf("AWS Secrets Manager : Secret %s updated", secretID)

	if c.cache != nil {
		err = c.cache.Put(ctx, key, data)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// Delete removes a certificate data from the cache under the specified key.
// If there's no such key in the cache, Delete returns nil.
func (c *SecretManagerAutocertCache) Delete(ctx context.Context, key string) error {

	svc := secretsmanager.New(c.awsSession)

	secretID := filepath.Join(c.secretPrefix, key)

	// Create the new entry in AWS Secret Manager for the file.
	_, err := svc.DeleteSecret(&secretsmanager.DeleteSecretInput{
		SecretId: aws.String(secretID),

		// (Optional) Specifies that the secret is to be deleted without any recovery
		// window. You can't use both this parameter and the RecoveryWindowInDays parameter
		// in the same API call.
		//
		// An asynchronous background process performs the actual deletion, so there
		// can be a short delay before the operation completes. If you write code to
		// delete and then immediately recreate a secret with the same name, ensure
		// that your code includes appropriate back off and retry logic.
		//
		// Use this parameter with caution. This parameter causes the operation to skip
		// the normal waiting period before the permanent deletion that AWS would normally
		// impose with the RecoveryWindowInDays parameter. If you delete a secret with
		// the ForceDeleteWithouRecovery parameter, then you have no opportunity to
		// recover the secret. It is permanently lost.
		ForceDeleteWithoutRecovery: aws.Bool(false),

		// (Optional) Specifies the number of days that Secrets Manager waits before
		// it can delete the secret. You can't use both this parameter and the ForceDeleteWithoutRecovery
		// parameter in the same API call.
		//
		// This value can range from 7 to 30 days.
		RecoveryWindowInDays: aws.Int64(30),
	})
	if err != nil {
		return errors.Wrapf(err, "autocert failed to delete secret %s", secretID)
	}

	log.Printf("AWS Secrets Manager : Secret %s deleted", secretID)

	if c.cache != nil {
		err = c.cache.Delete(ctx, key)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}
