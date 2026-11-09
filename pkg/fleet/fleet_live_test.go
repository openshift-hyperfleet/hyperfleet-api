package fleet

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"
)

// TestLiveFleet checks a real fleet directory created by hack/fleet.sh.
func TestLiveFleet(t *testing.T) {
	dir := os.Getenv("CLM_FLEET_DIR")
	if dir == "" {
		t.Skip("CLM_FLEET_DIR not set")
	}
	RegisterTestingT(t)

	clusters, err := List(dir)

	Expect(err).ToNot(HaveOccurred())
	Expect(clusters).To(HaveLen(3))
}
