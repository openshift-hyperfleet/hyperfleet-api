//go:build tools

package hack

// Blank import keeps hyperfleet-api-spec in go.mod so `go mod download` fetches it
// and `go list -m -f '{{.Dir}}'` in the Makefile generate target can locate the module cache path.
import _ "github.com/openshift-hyperfleet/hyperfleet-api-spec/schemas"
