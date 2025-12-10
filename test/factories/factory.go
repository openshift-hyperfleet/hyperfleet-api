package factories

import (
	"encoding/base32"

	"github.com/segmentio/ksuid"
)

type Factories struct {
}

// NewID generates a new unique identifier using KSUID with lowercase Base32 encoding.
// The resulting identifier is compatible with Kubernetes DNS-1123 subdomain naming requirements.
func (f *Factories) NewID() string {
	return uidEncoding.EncodeToString(ksuid.New().Bytes())
}

// uidAlphabet is the lowercase alphabet used to encode unique identifiers.
const uidAlphabet = "0123456789abcdefghijklmnopqrstuv"

// uidEncoding is the lowercase variant of Base32 used to encode unique identifiers.
var uidEncoding = base32.NewEncoding(uidAlphabet).WithPadding(base32.NoPadding)
