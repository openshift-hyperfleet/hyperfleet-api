package hyperfleetapi

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// fileTokenSource reads a bearer token from disk on every call, or caches it
// for cacheTTL when cacheTTL > 0. A zero cacheTTL disables caching and causes
// the file to be re-read on every request. It is safe for concurrent use.
type fileTokenSource struct {
	path      string
	cached    string
	mu        sync.RWMutex
	cacheTTL  time.Duration
	expiresAt int64 // Unix nanoseconds; only meaningful when cacheTTL > 0
}

func newFileTokenSource(path string, cacheTTL time.Duration) *fileTokenSource {
	return &fileTokenSource{path: path, cacheTTL: cacheTTL}
}

// get returns the current token. When cacheTTL > 0 the result is served from
// memory until the TTL elapses; when cacheTTL == 0 the file is read every call.
func (s *fileTokenSource) get() (string, error) {
	now := time.Now().UnixNano()

	if s.cacheTTL > 0 {
		s.mu.RLock()
		if now < s.expiresAt {
			tok := s.cached
			s.mu.RUnlock()
			return tok, nil
		}
		s.mu.RUnlock()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Re-check under write lock — another goroutine may have refreshed.
	if s.cacheTTL > 0 && now < s.expiresAt {
		return s.cached, nil
	}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		return "", fmt.Errorf("reading token file %s: %w", s.path, err)
	}
	tok := strings.TrimSpace(string(raw))
	if tok == "" {
		return "", fmt.Errorf("token file %s is empty", s.path)
	}

	if s.cacheTTL > 0 {
		s.cached = tok
		s.expiresAt = time.Now().Add(s.cacheTTL).UnixNano()
	}
	return tok, nil
}
