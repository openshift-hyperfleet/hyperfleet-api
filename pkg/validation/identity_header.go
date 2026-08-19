package validation

import (
	"net/http"
	"regexp"
)

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

var forbiddenJWTSourceHeaderNames = map[string]struct{}{
	"Cookie":     {},
	"Set-Cookie": {},
}

// IsForbiddenJWTSourceHeaderName reports whether name must not be used as the JWT token source header.
func IsForbiddenJWTSourceHeaderName(name string) bool {
	_, ok := forbiddenJWTSourceHeaderNames[http.CanonicalHeaderKey(name)]
	return ok
}

// headerTokenPattern matches the RFC 7230 "token" grammar for HTTP header
// field names: one or more tchar, which excludes whitespace and separators.
var headerTokenPattern = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

// IsValidHeaderName reports whether name is a syntactically valid HTTP header
// field name. A value like "X Tenant" or "   " isn't caught by an empty-string
// check but can never match a real request header, so config-sourced names
// should be validated here before being trusted for r.Header.Get lookups.
func IsValidHeaderName(name string) bool {
	return headerTokenPattern.MatchString(name)
}
