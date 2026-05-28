package validation

import "net/http"

var forbiddenIdentityHeaderNames = map[string]struct{}{
	"Authorization":             {},
	"Cookie":                    {},
	"Set-Cookie":                {},
	"X-Api-Key":                 {},
	"X-Auth-Token":              {},
	"X-Forwarded-Authorization": {},
	"Proxy-Authorization":       {},
}

// IsForbiddenIdentityHeaderName reports whether name must not be used as the caller identity header.
func IsForbiddenIdentityHeaderName(name string) bool {
	_, ok := forbiddenIdentityHeaderNames[http.CanonicalHeaderKey(name)]
	return ok
}
