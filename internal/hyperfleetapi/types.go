package hyperfleetapi

import (
	"context"
	"time"
)

// -----------------------------------------------------------------------------
// Retry Backoff Strategies
// -----------------------------------------------------------------------------

// BackoffStrategy defines the retry backoff strategy
type BackoffStrategy string

const (
	// BackoffExponential doubles the delay after each retry (1s, 2s, 4s, 8s...)
	BackoffExponential BackoffStrategy = "exponential"
	// BackoffLinear increases the delay linearly (1s, 2s, 3s, 4s...)
	BackoffLinear BackoffStrategy = "linear"
	// BackoffConstant uses the same delay between retries
	BackoffConstant BackoffStrategy = "constant"
)

// Default configuration values
const (
	DefaultTimeout       = 10 * time.Second
	DefaultRetryAttempts = 3
	DefaultRetryBackoff  = BackoffExponential
	DefaultBaseDelay     = 1 * time.Second
	DefaultMaxDelay      = 30 * time.Second
)

// -----------------------------------------------------------------------------
// Client Configuration
// -----------------------------------------------------------------------------

// AuthConfig holds optional JWT bearer token authentication configuration.
// When set, a bearer token is read from TokenPath and injected as an
// Authorization header on every outbound request.
type AuthConfig struct {
	// TokenPath is the absolute path to a file containing the bearer token.
	TokenPath string `yaml:"token_path,omitempty" mapstructure:"token_path"`
	// TokenCacheTTL controls how long the token is cached in memory.
	// Zero means the file is re-read on every request.
	TokenCacheTTL time.Duration `yaml:"token_cache_ttl,omitempty" mapstructure:"token_cache_ttl"`
}

// ClientConfig holds the configuration for the HTTP client
type ClientConfig struct {
	// DefaultHeaders are headers added to all requests
	DefaultHeaders map[string]string `yaml:"default_headers,omitempty" mapstructure:"default_headers"`
	// BaseURL is the base URL for all API requests (must be set by caller)
	// Relative URLs in requests will be prefixed with this
	BaseURL string `yaml:"base_url,omitempty" mapstructure:"base_url"`
	// Version is the HyperFleet API version (e.g., "v1")
	Version string `yaml:"version,omitempty" mapstructure:"version"`
	// RetryBackoff is the backoff strategy for retries
	RetryBackoff BackoffStrategy `yaml:"retry_backoff,omitempty" mapstructure:"retry_backoff"`
	// Timeout is the HTTP client timeout for requests
	Timeout time.Duration `yaml:"timeout,omitempty" mapstructure:"timeout"`
	// BaseDelay is the initial delay for retry backoff
	BaseDelay time.Duration `yaml:"base_delay,omitempty" mapstructure:"base_delay"`
	// MaxDelay is the maximum delay for retry backoff
	MaxDelay time.Duration `yaml:"max_delay,omitempty" mapstructure:"max_delay"`
	// RetryAttempts is the number of retry attempts for failed requests
	RetryAttempts int `yaml:"retry_attempts,omitempty" mapstructure:"retry_attempts"`
	// Auth configures optional JWT bearer token authentication.
	// When nil, requests are sent without an Authorization header.
	Auth *AuthConfig `yaml:"auth,omitempty" mapstructure:"auth"`
}

// DefaultClientConfig returns a ClientConfig with default values
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		Version:        "v1",
		Timeout:        DefaultTimeout,
		RetryAttempts:  DefaultRetryAttempts,
		RetryBackoff:   DefaultRetryBackoff,
		BaseDelay:      DefaultBaseDelay,
		MaxDelay:       DefaultMaxDelay,
		DefaultHeaders: make(map[string]string),
	}
}

// -----------------------------------------------------------------------------
// Request Types
// -----------------------------------------------------------------------------

// Request represents an HTTP request to the HyperFleet API
type Request struct {
	// Headers are the HTTP headers for the request
	Headers map[string]string
	// RetryBackoff overrides the client retry backoff for this request
	RetryBackoff *BackoffStrategy
	// RetryAttempts overrides the client retry attempts for this request
	RetryAttempts *int
	// Method is the HTTP method (GET, POST, PUT, PATCH, DELETE)
	Method string
	// URL is the full URL for the request
	URL string
	// Body is the request body (for POST, PUT, PATCH)
	Body []byte
	// Timeout overrides the client timeout for this request
	Timeout time.Duration
}

// RequestOption is a functional option for configuring a request
type RequestOption func(*Request)

// WithHeaders adds headers to the request
func WithHeaders(headers map[string]string) RequestOption {
	return func(r *Request) {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		for k, v := range headers {
			r.Headers[k] = v
		}
	}
}

// WithHeader adds a single header to the request
func WithHeader(key, value string) RequestOption {
	return func(r *Request) {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers[key] = value
	}
}

// WithBody sets the request body
func WithBody(body []byte) RequestOption {
	return func(r *Request) {
		r.Body = body
	}
}

// WithJSONBody sets the request body and Content-Type header for JSON
func WithJSONBody(body []byte) RequestOption {
	return func(r *Request) {
		r.Body = body
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers["Content-Type"] = "application/json"
	}
}

// WithRequestTimeout sets a custom timeout for this specific request
func WithRequestTimeout(timeout time.Duration) RequestOption {
	return func(r *Request) {
		r.Timeout = timeout
	}
}

// WithRequestRetryAttempts sets custom retry attempts for this specific request
func WithRequestRetryAttempts(attempts int) RequestOption {
	return func(r *Request) {
		r.RetryAttempts = &attempts
	}
}

// WithRequestRetryBackoff sets custom retry backoff for this specific request
func WithRequestRetryBackoff(backoff BackoffStrategy) RequestOption {
	return func(r *Request) {
		r.RetryBackoff = &backoff
	}
}

// -----------------------------------------------------------------------------
// Response Types
// -----------------------------------------------------------------------------

// Response represents an HTTP response from the HyperFleet API
type Response struct {
	// Headers are the response headers
	Headers map[string][]string
	// Status is the HTTP status string (e.g., "200 OK")
	Status string
	// Body is the response body
	Body []byte
	// Duration is how long the request took
	Duration time.Duration
	// StatusCode is the HTTP status code
	StatusCode int
	// Attempts is how many attempts were made (including retries)
	Attempts int
}

// IsSuccess returns true if the response status code is 2xx
func (r *Response) IsSuccess() bool {
	return r.StatusCode >= 200 && r.StatusCode < 300
}

// IsClientError returns true if the response status code is 4xx
func (r *Response) IsClientError() bool {
	return r.StatusCode >= 400 && r.StatusCode < 500
}

// IsServerError returns true if the response status code is 5xx
func (r *Response) IsServerError() bool {
	return r.StatusCode >= 500 && r.StatusCode < 600
}

// IsRetryable returns true if the request should be retried based on status code.
// Retryable codes:
//   - All 5xx server errors (500, 502, 503, 504, etc.)
//   - 408 Request Timeout
//   - 429 Too Many Requests
func (r *Response) IsRetryable() bool {
	switch r.StatusCode {
	case 408, 429:
		return true
	default:
		return r.IsServerError()
	}
}

// BodyString returns the response body as a string
func (r *Response) BodyString() string {
	if r.Body == nil {
		return ""
	}
	return string(r.Body)
}

// -----------------------------------------------------------------------------
// Client Interface
// -----------------------------------------------------------------------------

// Client is the interface for the HyperFleet API HTTP client
type Client interface {
	// Do executes an HTTP request and returns the response
	Do(ctx context.Context, req *Request) (*Response, error)

	// Get performs a GET request
	Get(ctx context.Context, url string, opts ...RequestOption) (*Response, error)

	// Post performs a POST request
	Post(ctx context.Context, url string, body []byte, opts ...RequestOption) (*Response, error)

	// Put performs a PUT request
	Put(ctx context.Context, url string, body []byte, opts ...RequestOption) (*Response, error)

	// Patch performs a PATCH request
	Patch(ctx context.Context, url string, body []byte, opts ...RequestOption) (*Response, error)

	// Delete performs a DELETE request
	Delete(ctx context.Context, url string, opts ...RequestOption) (*Response, error)

	// BaseURL returns the configured base URL for API requests
	BaseURL() string
}
