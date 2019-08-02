package notify

import (
	"bytes"
	"context"
	html "html/template"
	"path/filepath"
	text "text/template"

	"github.com/pkg/errors"
)

const (
	EmailCharSet = "UTF-8"
)

// Email defines method need to send an email disregarding the service provider.
type Email interface {
	Send(ctx context.Context, toEmail, subject, templateName string, data map[string]interface{}) error
	Verify() error
}

// MockEmail defines an implementation of the email interface for testing.
type MockEmail struct{}

// Send an email the provided email address.
func (n *MockEmail) Send(ctx context.Context, toEmail, subject, templateName string, data map[string]interface{}) error {
	return nil
}

// Verify ensures the provider works.
func (n *MockEmail) Verify() error {
	return nil
}

func parseEmailTemplates(templateDir, templateName string, data map[string]interface{}) ([]byte, []byte, error) {
	htmlFile := filepath.Join(templateDir, templateName+".html")
	htmlTmpl, err := html.ParseFiles(htmlFile)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "Failed to load HTML email template.")
	}

	var htmlDat bytes.Buffer
	if err := htmlTmpl.Execute(&htmlDat, data); err != nil {
		return nil, nil, errors.WithMessage(err, "Failed to parse HTML email template.")
	}

	txtFile := filepath.Join(templateDir, templateName+".txt")
	txtTmpl, err := text.ParseFiles(txtFile)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "Failed to load text email template.")
	}

	var txtDat bytes.Buffer
	if err := txtTmpl.Execute(&txtDat, data); err != nil {
		return nil, nil, errors.WithMessage(err, "Failed to parse text email template.")
	}

	return htmlDat.Bytes(), txtDat.Bytes(), nil
}
