package notify

import (
	"context"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/pkg/errors"
)

var (
	// ErrAwsSesIdentityNotVerified
	ErrAwsSesIdentityNotVerified = errors.New("AWS SES sending identity not verified.")

	// ErrAwsSesSendingDisabled
	ErrAwsSesSendingDisabled = errors.New("AWS SES sending disabled.")
)

// EmailAws defines the data needed to send an email with AWS SES.
type EmailAws struct {
	awsSession         *session.Session
	senderEmailAddress string
	templateDir        string
}

// NewEmailAws creates an implementation of the Email interface used to send email with AWS SES.
func NewEmailAws(awsSession *session.Session, sharedTemplateDir, senderEmailAddress string) (*EmailAws, error) {

	templateDir := filepath.Join(sharedTemplateDir, "emails")
	if _, err := os.Stat(templateDir); os.IsNotExist(err) {
		return nil, errors.WithMessage(err, "Email template directory does not exist.")
	}

	if senderEmailAddress == "" {
		return nil, errors.New("Sender email address is required.")
	}

	return &EmailAws{
		awsSession:         awsSession,
		templateDir:        templateDir,
		senderEmailAddress: senderEmailAddress,
	}, nil
}

// Verify ensures the provider works.
func (n *EmailAws) Verify() error {

	svc := ses.New(n.awsSession)

	var isVerified bool
	err := svc.ListIdentitiesPages(&ses.ListIdentitiesInput{}, func(res *ses.ListIdentitiesOutput, lastPage bool) bool {
		for _, r := range res.Identities {
			if *r == n.senderEmailAddress {
				isVerified = true
				return true
			}
		}

		return !lastPage
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if !isVerified {
		return errors.WithMessagef(ErrAwsSesIdentityNotVerified, "Email address '%s' not verified.", n.senderEmailAddress)
	}

	enabledRes, err := svc.GetAccountSendingEnabled(&ses.GetAccountSendingEnabledInput{})
	if err != nil {
		return errors.WithStack(err)
	} else if !*enabledRes.Enabled {
		return errors.WithMessage(ErrAwsSesSendingDisabled, "Sending has not be enabled for recipients are "+
			"not verified. Submit support ticket with AWS for SES approval.")
	}

	return nil
}

// Send initials the delivery of an email the provided email address.
func (n *EmailAws) Send(ctx context.Context, toEmail, subject, templateName string, data map[string]interface{}) error {

	htmlDat, txtDat, err := parseEmailTemplates(n.templateDir, templateName, data)
	if err != nil {
		return err
	}

	svc := ses.New(n.awsSession)

	// Assemble the email.
	input := &ses.SendEmailInput{
		Destination: &ses.Destination{
			ToAddresses: []*string{
				aws.String(toEmail),
			},
		},
		Message: &ses.Message{
			Body: &ses.Body{
				Html: &ses.Content{
					Charset: aws.String(EmailCharSet),
					Data:    aws.String(string(htmlDat)),
				},
				Text: &ses.Content{
					Charset: aws.String(EmailCharSet),
					Data:    aws.String(string(txtDat)),
				},
			},
			Subject: &ses.Content{
				Charset: aws.String(EmailCharSet),
				Data:    aws.String(subject),
			},
		},
		Source: aws.String(n.senderEmailAddress),
	}

	// Send the email
	_, err = svc.SendEmail(input)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
