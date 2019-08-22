package notify

import "context"

// DisableEmail defines an implementation of the email interface that doesn't send any email.
type DisableEmail struct{}

// NewEmailDisabled disables sending any emails with an empty implementation of the email interface.
func NewEmailDisabled() *DisableEmail {
	return &DisableEmail{}
}

// Send does nothing.
func (n *DisableEmail) Send(ctx context.Context, toEmail, subject, templateName string, data map[string]interface{}) error {
	return nil
}

// Verify does nothing.
func (n *DisableEmail) Verify() error {
	return nil
}
