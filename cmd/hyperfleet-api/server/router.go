package server

import (
	"net/http"
	"slices"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

// Middleware wraps an http.Handler to produce a new http.Handler, allowing
// cross-cutting behavior (logging, auth, tracing, ...) to be composed around
// route handlers.
type Middleware func(http.Handler) http.Handler

// Router is a thin wrapper around http.ServeMux that adds gorilla/mux-style
// middleware chaining (.Use()) and grouping (.Group()) on top of Go's stdlib
// method-prefixed routing patterns (e.g. "GET /clusters/{id}").
//
// Groups can carry a path prefix (via Group("/v1")) that is automatically
// prepended to every pattern registered through that group, so route
// handlers only declare their own path suffix.
//
// Middlewares registered via Use() are captured at HandleFunc/Handle time, so
// Use() must be called before registering the routes it should apply to.
type Router struct {
	mux         *http.ServeMux
	prefix      string
	middlewares []Middleware
}

// NewRouter creates a new top-level Router backed by a fresh http.ServeMux.
func NewRouter() *Router {
	return &Router{mux: http.NewServeMux()}
}

// Use appends middlewares to this router's chain. Only routes registered
// after this call (on this router or a Group() derived from it afterwards)
// will be wrapped by them.
func (r *Router) Use(mw ...Middleware) {
	r.middlewares = append(r.middlewares, mw...)
}

// Group returns a child Router sharing the same underlying ServeMux but with
// an independent copy of the current middleware chain, so further Use() calls
// on the child don't affect the parent or any sibling groups.
//
// An optional path prefix can be passed to scope routes registered on the
// child. The prefix is cumulative: if a parent already carries "/api/v1" and
// the child adds "/clusters", routes on the child are prefixed with
// "/api/v1/clusters". Omitting the prefix inherits the parent's prefix as-is.
func (r *Router) Group(prefix ...string) *Router {
	p := r.prefix
	if len(prefix) > 0 {
		p += prefix[0]
	}
	return &Router{
		mux:         r.mux,
		middlewares: append([]Middleware(nil), r.middlewares...),
		prefix:      p,
	}
}

// HandleFunc registers handler for pattern, wrapped with this router's
// current middleware chain.
func (r *Router) HandleFunc(pattern string, handler http.HandlerFunc) {
	r.Handle(pattern, handler)
}

// Handle registers handler for pattern, wrapped with this router's current
// middleware chain. If the router carries a prefix, it is prepended to the
// path component of the pattern automatically.
func (r *Router) Handle(pattern string, handler http.Handler) {
	for _, mw := range slices.Backward(r.middlewares) {
		handler = mw(handler)
	}
	r.mux.Handle(r.withPrefix(pattern), handler)
}

// withPrefix prepends the router's prefix to the path component of a Go 1.22+
// routing pattern ("METHOD /path" or just "/path").
func (r *Router) withPrefix(pattern string) string {
	if r.prefix == "" {
		return pattern
	}
	if method, path, ok := strings.Cut(pattern, " "); ok {
		return method + " " + r.prefix + path
	}
	return r.prefix + pattern
}

// ServeHTTP satisfies http.Handler, allowing a Router to be used directly as
// an http.Server's Handler or wrapped by further http.Handler middleware.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	WithNotFoundHandler(r.mux).ServeHTTP(w, req)
}

// Handler returns the handler and matched pattern for req without invoking
// it, delegating to the underlying http.ServeMux. Mainly useful in tests that
// want to assert a route was registered without executing its side effects.
func (r *Router) Handler(req *http.Request) (http.Handler, string) {
	return r.mux.Handler(req)
}

// WithNotFoundHandler wraps mux so that requests matching no registered
// pattern get api.SendNotFound's JSON body instead of net/http's default
// plain-text 404.
//
// It must NOT be implemented by registering a catch-all "/" pattern on mux:
// net/http.ServeMux falls back to any matching less-specific pattern (like
// "/") when a request's method doesn't match a more specific one, which
// would silently swallow the automatic 405 Method Not Allowed responses
// (and their Allow header) stdlib provides for registered paths. Instead,
// mux.Handler is used to check ahead of time whether the request matches no
// pattern at all (as opposed to matching a pattern with the wrong method),
// and only that case is rewritten.
func WithNotFoundHandler(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if _, pattern := mux.Handler(req); pattern == "" {
			mux.ServeHTTP(&notFoundResponseWriter{ResponseWriter: w, req: req}, req)
			return
		}
		mux.ServeHTTP(w, req)
	})
}

// notFoundResponseWriter rewrites a 404 response body to api.SendNotFound's
// JSON format, passing every other status (including stdlib's automatic 405)
// through unchanged.
type notFoundResponseWriter struct {
	http.ResponseWriter
	req     *http.Request
	handled bool
}

func (w *notFoundResponseWriter) WriteHeader(code int) {
	if code == http.StatusNotFound {
		w.handled = true
		api.SendNotFound(w.ResponseWriter, w.req)
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *notFoundResponseWriter) Write(b []byte) (int, error) {
	if w.handled {
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}

// stripMethodPrefix removes the leading "METHOD " prefix from a Go 1.22+
// http.Request.Pattern (e.g. "GET /clusters/{id}" -> "/clusters/{id}"),
// leaving method-less patterns (e.g. "/") unchanged.
func stripMethodPrefix(pattern string) string {
	if _, path, ok := strings.Cut(pattern, " "); ok {
		return path
	}
	return pattern
}
