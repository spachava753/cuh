package gmail

import "errors"

var ErrNotImplemented = errors.New("gmail: not implemented")

// SendEmail sends an email through a configured Gmail account.
func SendEmail(to string, subject string, body string) error {
	if to == "" || subject == "" || body == "" {
		return errors.New("gmail: to, subject, and body are required")
	}
	return ErrNotImplemented
}
