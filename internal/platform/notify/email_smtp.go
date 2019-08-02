package notify

import (
	"context"
	"github.com/pkg/errors"
	"gopkg.in/gomail.v2"
	"os"
	"path/filepath"
)

// EmailAws defines the data needed to send an email with AWS SES.
type EmailSmtp struct {
	dialer             gomail.Dialer
	senderEmailAddress string
	templateDir        string
}

// NewEmailSmtp creates an implementation of the Email interface used to send email with SMTP.
func NewEmailSmtp(dialer gomail.Dialer, sharedTemplateDir, senderEmailAddress string) (*EmailSmtp, error) {

	if senderEmailAddress == "" {
		return nil, errors.New("Sender email address is required.")
	}

	templateDir := filepath.Join(sharedTemplateDir, "emails")
	if _, err := os.Stat(templateDir); os.IsNotExist(err) {
		return nil, errors.WithMessage(err, "Email template directory does not exist.")
	}

	return &EmailSmtp{
		dialer:             dialer,
		templateDir:        templateDir,
		senderEmailAddress: senderEmailAddress,
	}, nil
}

// Verify ensures the provider works.
func (n *EmailSmtp) Verify() error {
	return nil
}

// Send initials the delivery of an email the provided email address.
func (n *EmailSmtp) Send(ctx context.Context, toEmail, subject, templateName string, data map[string]interface{}) error {

	htmlDat, txtDat, err := parseEmailTemplates(n.templateDir, templateName, data)
	if err != nil {
		return err
	}

	m := gomail.NewMessage()
	m.SetHeader("From", n.senderEmailAddress)
	m.SetHeader("To", toEmail)
	m.SetHeader("Subject", subject)

	m.SetBody("text/plain", string(txtDat))
	if err := n.dialer.DialAndSend(m); err != nil {
		return errors.WithStack(err)
	}

	m.SetBody("text/html", string(htmlDat))
	if err := n.dialer.DialAndSend(m); err != nil {
		return errors.WithStack(err)
	}

	return nil
}
