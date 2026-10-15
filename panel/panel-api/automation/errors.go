package automation

import (
	"errors"
	"net/url"
)

var (
	ErrLoginPage = errors.New("login page still showing")
	ErrRejected  = errors.New("portal rejected the username or password")
	ErrNoCookie  = errors.New("tool cookie was not set")
	ErrCSRF      = errors.New("csrf token missing")
	ErrTimeout   = errors.New("timed out")
	ErrDenied    = errors.New("access denied")
	ErrUpstream  = errors.New("upstream URL missing")
)

// PageError keeps the failure and the host plus path that was open.
// The query string stays off the message so tokens are not logged.
type PageError struct {
	Kind error
	Host string
	Path string
}

func At(kind error, rawURL string) error {
	host, path := "", ""
	if parsed, err := url.Parse(rawURL); err == nil {
		host = parsed.Hostname()
		path = parsed.EscapedPath()
		if path == "" {
			path = "/"
		}
	}
	return &PageError{Kind: kind, Host: host, Path: path}
}

func (e *PageError) Error() string { return e.Public() }

func (e *PageError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Kind
}

func (e *PageError) Public() string {
	if e == nil || e.Kind == nil {
		return ""
	}
	if e.Host == "" {
		return e.Kind.Error()
	}
	return e.Kind.Error() + " at " + e.Host + e.Path
}
