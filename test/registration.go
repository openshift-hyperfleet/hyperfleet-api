package test

import (
	"testing"

	gm "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
)

// RegisterIntegration Register a test
// This should be run before every integration test
func RegisterIntegration(t *testing.T) (*Helper, *openapi.ClientWithResponses) {
	gm.RegisterTestingT(t)
	helper := NewHelper()
	if err := helper.ResetDB(); err != nil {
		t.Fatalf("failed to reset database: %v", err)
	}
	client := helper.NewAPIClient()
	return helper, client
}
