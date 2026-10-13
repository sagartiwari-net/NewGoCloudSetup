package automation

import "context"

// Recipe is one portal's HTTP login. The panel worker calls these three steps.
// A new website adds a folder that implements Recipe and registers its id.
type Recipe interface {
	Check(ctx context.Context, cookies []Cookie) (stillValid bool, updated []Cookie, err error)
	Login(ctx context.Context, username, password string) ([]Cookie, error)
	Capture(ctx context.Context, portal []Cookie, upstreamURL string) ([]Cookie, error)
	// PortalHost is the login site, such as markhorseotool.com. Tool hosts are subdomains of it.
	PortalHost() string
}
