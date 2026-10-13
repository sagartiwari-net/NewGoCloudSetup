package automation

import "errors"

var (
	ErrLoginPage = errors.New("login page still showing")
	ErrCSRF      = errors.New("csrf token missing")
	ErrTimeout   = errors.New("timed out")
	ErrDenied    = errors.New("access denied")
	ErrUpstream  = errors.New("upstream URL missing")
)
