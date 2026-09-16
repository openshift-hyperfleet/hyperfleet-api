package db

import (
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"

	"github.com/lib/pq"
)

func TestIsInvalidColumnError(t *testing.T) {
	assertInvalidColumnError(t, "undefined column", &pq.Error{Code: "42703"}, true)
	assertInvalidColumnError(t, "ambiguous column", &pq.Error{Code: "42702"}, true)
	assertInvalidColumnError(t, "other database error", &pq.Error{Code: "23505"}, false)
	assertInvalidColumnError(t, "ordinary error", errors.New("ordinary error"), false)
}

func TestIsDBConnectionError(t *testing.T) {
	assertDBConnectionError(t, "nil", nil, false)
	assertDBConnectionError(t, "connection refused", &net.OpError{
		Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED,
	}, true)
	assertDBConnectionError(t, "EOF", io.EOF, true)
	assertDBConnectionError(t, "unexpected EOF", io.ErrUnexpectedEOF, true)
	assertDBConnectionError(t, "wrapped connection refused", fmt.Errorf(
		"driver: %w", &net.OpError{Err: syscall.ECONNREFUSED},
	), true)
	assertDBConnectionError(t, "ordinary error", errors.New("ordinary error"), false)
}

func assertInvalidColumnError(t *testing.T, name string, err error, want bool) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		if got := IsInvalidColumnError(err); got != want {
			t.Fatalf("IsInvalidColumnError() = %v, want %v", got, want)
		}
	})
}

func assertDBConnectionError(t *testing.T, name string, err error, want bool) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		if got := IsDBConnectionError(err); got != want {
			t.Fatalf("IsDBConnectionError() = %v, want %v", got, want)
		}
	})
}
