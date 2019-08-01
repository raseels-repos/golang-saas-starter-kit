package devops

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/pkg/errors"
)

var ErrSecreteNotFound = errors.New("secret not found")

// SecretManagerGetString loads a key from AWS Secrets Manager.
// when UnrecognizedClientException its likely the AWS IAM permissions are not correct.
func SecretManagerGetString(awsSession *session.Session, secretID string) (string, error) {

	svc := secretsmanager.New(awsSession)

	// Load the secret by ID from Secrets Manager.
	res, err := svc.GetSecretValue(&secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && (aerr.Code() == secretsmanager.ErrCodeResourceNotFoundException || aerr.Code() == secretsmanager.ErrCodeInvalidRequestException) {
			return "", ErrSecreteNotFound
		}

		return "", errors.Wrapf(err, "failed to get value for secret id %s", secretID)
	}

	return *res.SecretString, nil
}

// SecretManagerPutString saves a value to AWS Secrets Manager.
// If the secret ID does not exist, it will create it.
// If the secret ID was deleted, it will restore it and then update the value.
func SecretManagerPutString(awsSession *session.Session, secretID, value string) error {

	svc := secretsmanager.New(awsSession)

	// Create the new entry in AWS Secret Manager for the file.
	_, err := svc.CreateSecret(&secretsmanager.CreateSecretInput{
		Name:         aws.String(secretID),
		SecretString: aws.String(value),
	})
	if err != nil {
		aerr, ok := err.(awserr.Error)

		if ok && aerr.Code() == secretsmanager.ErrCodeInvalidRequestException {
			// InvalidRequestException: You can't create this secret because a secret with this
			// 							 name is already scheduled for deletion.

			// Restore secret after it was already previously deleted.
			_, err = svc.RestoreSecret(&secretsmanager.RestoreSecretInput{
				SecretId: aws.String(secretID),
			})
			if err != nil {
				return errors.Wrapf(err, "failed to restore secret %s", secretID)
			}

		} else if !ok || aerr.Code() != secretsmanager.ErrCodeResourceExistsException {
			return errors.Wrapf(err, "failed to create secret %s", secretID)
		}

		// If where was a resource exists error for create, then need to update the secret instead.
		_, err = svc.UpdateSecret(&secretsmanager.UpdateSecretInput{
			SecretId:     aws.String(secretID),
			SecretString: aws.String(value),
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update secret %s", secretID)
		}
	}

	return nil
}
