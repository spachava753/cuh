package messages

import "errors"

var ErrNotImplemented = errors.New("messages: not implemented")

// SendSMS sends an SMS or iMessage from the local macOS host.
func SendSMS(to string, body string) error {
	if to == "" || body == "" {
		return errors.New("messages: to and body are required")
	}
	return ErrNotImplemented
}
