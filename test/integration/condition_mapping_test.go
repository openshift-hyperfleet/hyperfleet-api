package integration

import (
	"net/http"
	"os"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

const (
	conditionTypeQuotaValid = "QuotaValid"
)

// TestConditionMapping_BEFORE tests behavior without CEL mapping configured
// Expected: adapter custom conditions do NOT appear in public status.conditions
// NOTE: This test will PASS when the API is configured WITHOUT CEL mapping.
func TestConditionMapping_BEFORE(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Report validation adapter status with custom condition QuotaSufficient
	statusInput := newAdapterStatusRequest(
		"validation",
		cluster.Generation,
		[]openapi.ConditionRequest{
			{
				Type:    api.AdapterConditionTypeAvailable,
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("ValidationPassed"),
				Message: util.PtrString("Validation passed"),
			},
			{
				Type:    api.AdapterConditionTypeApplied,
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("Applied"),
				Message: util.PtrString("Applied"),
			},
			{
				Type:    api.AdapterConditionTypeHealth,
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("Healthy"),
				Message: util.PtrString("Healthy"),
			},
			{
				Type:    "QuotaSufficient", // Custom condition
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("QuotaOK"),
				Message: util.PtrString("Cluster quota is sufficient"),
			},
		},
		nil,
	)

	resp, err := client.PutClusterStatusesWithResponse(
		ctx, cluster.ID,
		openapi.PutClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusCreated))

	// Get the cluster and verify conditions
	getResp, err := client.GetClusterByIdWithResponse(ctx, cluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(getResp.StatusCode()).To(Equal(http.StatusOK))
	Expect(getResp.JSON200).NotTo(BeNil())

	resource := getResp.JSON200

	// Verify conditions (BEFORE - without mapping)
	conditions := resource.Status.Conditions
	Expect(conditions).NotTo(BeEmpty())

	// Should have standard conditions
	hasReconciled := false
	hasLastKnownReconciled := false
	hasValidationSuccessful := false
	hasQuotaValid := false

	for _, cond := range conditions {
		switch cond.Type {
		case "Reconciled":
			hasReconciled = true
		case "LastKnownReconciled":
			hasLastKnownReconciled = true
		case "ValidationSuccessful":
			hasValidationSuccessful = true
		case conditionTypeQuotaValid, "quotavalid":
			hasQuotaValid = true
		}
	}

	// BEFORE expectations
	Expect(hasReconciled).To(BeTrue(), "should have Reconciled condition")
	Expect(hasLastKnownReconciled).To(BeTrue(), "should have LastKnownReconciled condition")
	Expect(hasValidationSuccessful).To(BeTrue(), "should have ValidationSuccessful condition")
	Expect(hasQuotaValid).To(BeFalse(), "BEFORE: should NOT have QuotaValid/quotavalid condition (no CEL mapping)")
}

// TestConditionMapping_AFTER tests behavior with CEL mapping configured
// This test expects the API to be started with condition mapping configured in config.yaml
// Expected: adapter custom conditions ARE mapped and appear in public status.conditions
//
// To enable this test, set environment variable:
//
//	HYPERFLEET_TEST_CONDITION_MAPPING=1
//
// And ensure your config.yaml has the QuotaValid mapping configured:
//
//	conditions:
//	  clusters:
//	    QuotaValid:
//	      when:
//	        expression: 'statuses.exists(s, s.adapter == "validation" && s.conditions.exists(c, c.type == "QuotaSufficient"))'
//	      output:
//	        status:
//	          expression: 'statuses.filter(s, s.adapter == "validation")[0].conditions.filter(c, c.type == "QuotaSufficient")[0].status'
//	        reason:
//	          expression: 'statuses.filter(s, s.adapter == "validation")[0].conditions.filter(c, c.type == "QuotaSufficient")[0].reason'
//	        message:
//	          expression: '"Quota: " + statuses.filter(s, s.adapter == "validation")[0].conditions.filter(c, c.type == "QuotaSufficient")[0].message'
func TestConditionMapping_AFTER(t *testing.T) {
	if os.Getenv("HYPERFLEET_TEST_CONDITION_MAPPING") == "" {
		t.Skip("Skipped by default - set HYPERFLEET_TEST_CONDITION_MAPPING=1 to enable. Requires API configured with CEL mapping.")
	}

	h, client := test.RegisterIntegration(t)
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Report validation adapter status with custom condition QuotaSufficient
	statusInput := newAdapterStatusRequest(
		"validation",
		cluster.Generation,
		[]openapi.ConditionRequest{
			{
				Type:    api.AdapterConditionTypeAvailable,
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("ValidationPassed"),
				Message: util.PtrString("Validation passed"),
			},
			{
				Type:    api.AdapterConditionTypeApplied,
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("Applied"),
				Message: util.PtrString("Applied"),
			},
			{
				Type:    api.AdapterConditionTypeHealth,
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("Healthy"),
				Message: util.PtrString("Healthy"),
			},
			{
				Type:    "QuotaSufficient", // Custom condition
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("QuotaOK"),
				Message: util.PtrString("Cluster quota is sufficient"),
			},
		},
		nil,
	)

	resp, err := client.PutClusterStatusesWithResponse(
		ctx, cluster.ID,
		openapi.PutClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusCreated))

	// Get the cluster and verify conditions
	getResp, err := client.GetClusterByIdWithResponse(ctx, cluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(getResp.StatusCode()).To(Equal(http.StatusOK))
	Expect(getResp.JSON200).NotTo(BeNil())

	resource := getResp.JSON200

	// Verify conditions (AFTER - with mapping)
	conditions := resource.Status.Conditions
	Expect(conditions).NotTo(BeEmpty())

	// Should have standard conditions + mapped condition
	hasReconciled := false
	hasLastKnownReconciled := false
	hasValidationSuccessful := false
	hasQuotaValid := false
	var quotaValidCondition *openapi.ResourceCondition

	for _, cond := range conditions {
		switch cond.Type {
		case "Reconciled":
			hasReconciled = true
		case "LastKnownReconciled":
			hasLastKnownReconciled = true
		case "ValidationSuccessful":
			hasValidationSuccessful = true
		case conditionTypeQuotaValid, "quotavalid":
			hasQuotaValid = true
			quotaValidCondition = &cond
		}
	}

	// AFTER expectations
	Expect(hasReconciled).To(BeTrue(), "should have Reconciled condition")
	Expect(hasLastKnownReconciled).To(BeTrue(), "should have LastKnownReconciled condition")
	Expect(hasValidationSuccessful).To(BeTrue(), "should have ValidationSuccessful condition")
	Expect(hasQuotaValid).To(BeTrue(), "AFTER: should have QuotaValid/quotavalid condition (CEL mapping active)")

	// Verify the mapped condition details
	Expect(quotaValidCondition).NotTo(BeNil())
	Expect(string(quotaValidCondition.Status)).To(Equal(string(api.ConditionTrue)))
	Expect(*quotaValidCondition.Reason).To(Equal("QuotaOK"))
	Expect(*quotaValidCondition.Message).To(ContainSubstring("Quota"))
}

// TestConditionMapping_MultipleRules tests multiple mapping rules active simultaneously
//
// To enable this test, set environment variable:
//
//	HYPERFLEET_TEST_CONDITION_MAPPING=1
//
// And ensure your config.yaml has multiple mapping rules configured.
func TestConditionMapping_MultipleRules(t *testing.T) {
	if os.Getenv("HYPERFLEET_TEST_CONDITION_MAPPING") == "" {
		t.Skip("Skipped by default - set HYPERFLEET_TEST_CONDITION_MAPPING=1 to enable. Requires API configured with multiple CEL mappings.")
	}

	h, client := test.RegisterIntegration(t)
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Report validation adapter with TWO custom conditions
	statusInput := newAdapterStatusRequest(
		"validation",
		cluster.Generation,
		[]openapi.ConditionRequest{
			{Type: api.AdapterConditionTypeAvailable, Status: openapi.AdapterConditionStatusTrue, Reason: util.PtrString("OK"), Message: util.PtrString("OK")},
			{Type: api.AdapterConditionTypeApplied, Status: openapi.AdapterConditionStatusTrue, Reason: util.PtrString("OK"), Message: util.PtrString("OK")},
			{Type: api.AdapterConditionTypeHealth, Status: openapi.AdapterConditionStatusTrue, Reason: util.PtrString("OK"), Message: util.PtrString("OK")},
			{Type: "QuotaSufficient", Status: openapi.AdapterConditionStatusTrue, Reason: util.PtrString("QuotaOK"), Message: util.PtrString("Quota OK")},
			{Type: "PolicyValid", Status: openapi.AdapterConditionStatusTrue, Reason: util.PtrString("PolicyOK"), Message: util.PtrString("Policy OK")},
		},
		nil,
	)

	resp, err := client.PutClusterStatusesWithResponse(
		ctx, cluster.ID,
		openapi.PutClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusCreated))

	// Get the cluster
	getResp, err := client.GetClusterByIdWithResponse(ctx, cluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(getResp.JSON200).NotTo(BeNil())

	resource := getResp.JSON200

	// Verify both custom conditions were mapped
	// The test assumes config has QuotaValid and PolicyValid mapping rules configured
	var hasQuotaValid, hasPolicyValid bool
	for _, cond := range resource.Status.Conditions {
		if cond.Type == "QuotaValid" {
			hasQuotaValid = true
			Expect(string(cond.Status)).To(Equal(string(api.ConditionTrue)))
		}
		if cond.Type == "PolicyValid" {
			hasPolicyValid = true
			Expect(string(cond.Status)).To(Equal(string(api.ConditionTrue)))
		}
	}

	Expect(hasQuotaValid).To(BeTrue(), "QuotaValid condition should be mapped from QuotaSufficient adapter condition")
	Expect(hasPolicyValid).To(BeTrue(), "PolicyValid condition should be mapped from PolicyValid adapter condition")
}
