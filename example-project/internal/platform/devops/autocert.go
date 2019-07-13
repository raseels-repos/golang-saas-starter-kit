package devops

import (
	"context"
	"log"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/pkg/errors"
	"golang.org/x/crypto/acme/autocert"
	"github.com/aws/aws-sdk-go/aws/session"
)

// SecretManagerAutocertCache implements the autocert.Cache interface for AWS Secrets Manager that is used by Manager
// to store and retrieve previously obtained certificates and other account data as opaque blobs.
type SecretManagerAutocertCache struct  {
	awsSession *session.Session
	log *log.Logger
	secretPrefix string
}

// NewSecretManagerAutocertCache provides the functionality to keep config files sync'd between running tasks and across deployments.
func NewSecretManagerAutocertCache(log *log.Logger, awsSession *session.Session, secretPrefix string ) (*SecretManagerAutocertCache, error) {
	return &SecretManagerAutocertCache{
		awsSession,
		log,
		secretPrefix,
	}, nil
}

// Get returns a certificate data for the specified key.
// If there's no such key, Get returns ErrCacheMiss.
func (c *SecretManagerAutocertCache) Get(ctx context.Context, key string) ([]byte, error) {

	svc := secretsmanager.New(c.awsSession)

	secretID := filepath.Join(c.secretPrefix, key)

	// Load the secret by ID from Secrets Manager.
	res, err := svc.GetSecretValue(&secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == secretsmanager.ErrCodeResourceNotFoundException {
			return nil, autocert.ErrCacheMiss
		}

		return nil, errors.Wrapf(err, "failed to get value for secret id %s", secretID)
	}

	log.Printf("AWS Secrets Manager : Secret %s found", secretID)

	return res.SecretBinary, nil
}

// Put stores the data in the cache under the specified key.
// Underlying implementations may use any data storage format,
// as long as the reverse operation, Get, results in the original data.
func (c *SecretManagerAutocertCache) Put(ctx context.Context, key string, data []byte) error {

	svc := secretsmanager.New(c.awsSession)

	secretID := filepath.Join(c.secretPrefix, key)

	// Create the new entry in AWS Secret Manager for the file.
	_, err := svc.CreateSecret(&secretsmanager.CreateSecretInput{
		Name:         aws.String(secretID),
		SecretString: aws.String(string(data)),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); !ok {

			if aerr.Code() == secretsmanager.ErrCodeInvalidRequestException {
				// InvalidRequestException: You can't create this secret because a secret with this
				// 							 name is already scheduled for deletion.

				// Restore secret after it was already previously deleted.
				_, err = svc.RestoreSecret(&secretsmanager.RestoreSecretInput{
					SecretId:         aws.String(secretID),
				})
				if err != nil {
					return errors.Wrapf(err, "autocert failed to restore secret %s", secretID)
				}

			} else if aerr.Code() != secretsmanager.ErrCodeResourceExistsException {
				return errors.Wrapf(err, "autocert failed to create secret %s", secretID)
			}
		}

		// If where was a resource exists error for create, then need to update the secret instead.
		_, err = svc.UpdateSecret(&secretsmanager.UpdateSecretInput{
			SecretId:         aws.String(secretID),
			SecretString: aws.String(string(data)),
		})
		if err != nil {
			return errors.Wrapf(err, "autocert failed to update secret %s", secretID)
		}

		log.Printf("AWS Secrets Manager : Secret %s updated", secretID)
	} else {
		log.Printf("AWS Secrets Manager : Secret %s created", secretID)
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
		SecretId:         aws.String(secretID),

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

	log.Printf("AWS Secrets Manager : Secret %s deleted for %s", secretID)

	return nil
}
