package server

import (
	"compress/gzip"
	"net/http"
	"strconv"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// gzipResponseWriter wraps an http.ResponseWriter, transparently gzip-encoding
// everything written to it. It defers deciding whether to compress until the
// first Write/WriteHeader call so that any Content-Length set by the
// downstream handler is stripped before it reaches the client.
type gzipResponseWriter struct {
	http.ResponseWriter
	writer      *gzip.Writer
	wroteHeader bool
}

func (w *gzipResponseWriter) WriteHeader(statusCode int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		w.Header().Del("Content-Length")
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		w.Header().Del("Content-Length")
	}
	return w.writer.Write(b)
}

// CompressMiddleware gzip-encodes the response body when the client indicates
// support for it via the Accept-Encoding header.
func CompressMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Honor Accept-Encoding quality values per RFC 9110 §12.5.3: only
		// enable gzip when a "gzip" or "*" token has a positive q value.
		accepts := false
		for token := range strings.SplitSeq(r.Header.Get("Accept-Encoding"), ",") {
			parts := strings.Split(token, ";")
			coding := strings.ToLower(strings.TrimSpace(parts[0]))
			if coding != "gzip" && coding != "*" {
				continue
			}

			q := 1.0
			for _, param := range parts[1:] {
				if val, ok := strings.CutPrefix(strings.TrimSpace(param), "q="); ok {
					if parsed, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil {
						q = parsed
					}
				}
			}
			if q > 0 {
				accepts = true
				break
			}
		}
		if !accepts {
			next.ServeHTTP(w, r)
			return
		}

		gz := gzip.NewWriter(w)
		defer func() {
			if err := gz.Close(); err != nil {
				logger.WithError(r.Context(), err).Warn("failed to finalize gzip response, response may be incomplete")
			}
		}()

		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, writer: gz}, r)
	})
}
