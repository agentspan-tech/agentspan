package middleware

import (
	"net/url"
	"strings"
)

// BuildCSP returns the Content-Security-Policy header value for both API and
// SPA responses. When billingURL is non-empty (cloud), its scheme+host is added
// to connect-src so the dashboard can call the billing service cross-origin.
// Self-host leaves connect-src on 'self' only.
func BuildCSP(billingURL string) string {
	connectSrc := "'self' ws: wss:"
	if origin := normalizeOrigin(billingURL); origin != "" {
		connectSrc = "'self' ws: wss: " + origin
	}
	return strings.Join([]string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"connect-src " + connectSrc,
		"font-src 'self'",
		"object-src 'none'",
		"frame-ancestors 'none'",
		"base-uri 'self'",
	}, "; ")
}

func normalizeOrigin(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
