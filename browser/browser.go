package browser

import "errors"

var ErrNotImplemented = errors.New("browser: not implemented")

// OpenURL opens a URL in a browser session.
func OpenURL(url string) error {
	if url == "" {
		return errors.New("browser: url is required")
	}
	return ErrNotImplemented
}
