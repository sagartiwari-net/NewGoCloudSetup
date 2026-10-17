package automation

import (
	"errors"
	"net/url"
	"strings"
)

var (
	ErrLoginPage      = errors.New("login page still showing")
	ErrRejected       = errors.New("portal rejected the username or password")
	ErrNoCookie       = errors.New("tool cookie was not set")
	ErrDecrypt        = errors.New("credential could not be decrypted")
	ErrCSRF           = errors.New("csrf token missing")
	ErrTimeout        = errors.New("timed out")
	ErrDenied         = errors.New("access denied")
	ErrToolClosed     = errors.New("saved cookie did not open the tool")
	ErrUpstream       = errors.New("upstream URL missing")
	ErrStoredCookie   = errors.New("paste a fresh cookie in Storage")
	ErrCookieRejected = errors.New("saved cookie was rejected, so the account was not updated")
	ErrUserAgent      = errors.New("set the user agent on the mapped account")
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

// Where is the host and path only. Query strings can carry tokens, so they stay out of logs.
func Where(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Hostname() == "" {
		return "unknown"
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	return parsed.Hostname() + path
}

func Names(cookies []Cookie) string {
	names := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		name := strings.TrimSpace(cookie.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ",")
}
