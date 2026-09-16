package db

import (
	"errors"
	"io"
	"net"

	"github.com/lib/pq"
)

// IsInvalidColumnError reports undefined or ambiguous SQL column errors.
func IsInvalidColumnError(err error) bool {
	pqErr, ok := errors.AsType[*pq.Error](err)
	return ok && (pqErr.Code == "42703" || pqErr.Code == "42702")
}

// IsDBConnectionError reports infrastructure connection failures.
func IsDBConnectionError(err error) bool {
	if err == nil {
		return false
	}
	if netErr, ok := errors.AsType[*net.OpError](err); ok && netErr != nil {
		return true
	}
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}
