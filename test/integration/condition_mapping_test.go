package integration

import (
	"net/http"
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

// TestConditionMapping_UnmappedConditionsFiltered verifies that adapter custom
// conditions without a corresponding CEL mapping rule do NOT leak into the
// public resource status.conditions, while mapped conditions DO appear.
func TestConditionMapping_UnmappedConditionsFiltered(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Report validation adapter status with:
	// - standard conditions (Available, Applied, Health)
	// - QuotaSufficient (has a CEL mapping → QuotaValid)
	// - InternalDebugMetric (NO mapping → must not appear)
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
				Type:    "QuotaSufficient",
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("QuotaOK"),
				Message: util.PtrString("Cluster quota is sufficient"),
			},
			{
				Type:    "InternalDebugMetric",
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("DebugOK"),
				Message: util.PtrString("Internal metric - should not leak"),
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

	getResp, err := client.GetClusterByIdWithResponse(ctx, cluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(getResp.StatusCode()).To(Equal(http.StatusOK))
	Expect(getResp.JSON200).NotTo(BeNil())

	conditions := getResp.JSON200.Status.Conditions
	Expect(conditions).NotTo(BeEmpty())

	hasReconciled := false
	hasLastKnownReconciled := false
	hasQuotaValid := false
	hasInternalDebug := false

	for _, cond := range conditions {
		switch cond.Type {
		case "Reconciled":
			hasReconciled = true
		case "LastKnownReconciled":
			hasLastKnownReconciled = true
		case conditionTypeQuotaValid, "quotavalid":
			hasQuotaValid = true
		case "InternalDebugMetric", "internaldebugmetric":
			hasInternalDebug = true
		}
	}

	Expect(hasReconciled).To(BeTrue(), "should have Reconciled condition")
	Expect(hasLastKnownReconciled).To(BeTrue(), "should have LastKnownReconciled condition")
	Expect(hasQuotaValid).To(BeTrue(), "mapped condition QuotaValid should appear")
	Expect(hasInternalDebug).To(BeFalse(), "unmapped condition InternalDebugMetric must not leak into public conditions")
}

// TestConditionMapping_MappedConditionValues verifies that CEL mapping correctly
// transforms adapter conditions into public resource conditions with the expected
// status, reason, and message values.
func TestConditionMapping_MappedConditionValues(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

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
				Type:    "QuotaSufficient",
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

	getResp, err := client.GetClusterByIdWithResponse(ctx, cluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(getResp.StatusCode()).To(Equal(http.StatusOK))
	Expect(getResp.JSON200).NotTo(BeNil())

	conditions := getResp.JSON200.Status.Conditions
	Expect(conditions).NotTo(BeEmpty())

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

	Expect(hasReconciled).To(BeTrue(), "should have Reconciled condition")
	Expect(hasLastKnownReconciled).To(BeTrue(), "should have LastKnownReconciled condition")
	Expect(hasValidationSuccessful).To(BeTrue(), "should have ValidationSuccessful condition")
	Expect(hasQuotaValid).To(BeTrue(), "should have QuotaValid condition (CEL mapping active)")

	Expect(quotaValidCondition).NotTo(BeNil())
	Expect(string(quotaValidCondition.Status)).To(Equal(string(api.ConditionTrue)))
	Expect(*quotaValidCondition.Reason).To(Equal("QuotaOK"))
	Expect(*quotaValidCondition.Message).To(ContainSubstring("Quota"))
}

// TestConditionMapping_MultipleRules tests multiple mapping rules active simultaneously.
func TestConditionMapping_MultipleRules(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	statusInput := newAdapterStatusRequest(
		"validation",
		cluster.Generation,
		[]openapi.ConditionRequest{
			{
				Type:    api.AdapterConditionTypeAvailable,
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("OK"),
				Message: util.PtrString("OK"),
			},
			{
				Type:    api.AdapterConditionTypeApplied,
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("OK"),
				Message: util.PtrString("OK"),
			},
			{
				Type:    api.AdapterConditionTypeHealth,
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("OK"),
				Message: util.PtrString("OK"),
			},
			{
				Type:    "QuotaSufficient",
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("QuotaOK"),
				Message: util.PtrString("Quota OK"),
			},
			{
				Type:    "PolicyCheckPassed",
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("PolicyOK"),
				Message: util.PtrString("Policy OK"),
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

	getResp, err := client.GetClusterByIdWithResponse(ctx, cluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(getResp.JSON200).NotTo(BeNil())

	var hasQuotaValid, hasPolicyValid bool
	for _, cond := range getResp.JSON200.Status.Conditions {
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
	Expect(hasPolicyValid).To(BeTrue(), "PolicyValid condition should be mapped from PolicyCheckPassed adapter condition")
}

// TestConditionMapping_CELError_RollsBackTransaction verifies that a CEL evaluation
// failure during mapping rolls back the ENTIRE write transaction: neither the adapter
// status nor any aggregated/mapped conditions are persisted, and the caller receives
// an RFC 9457 error response. The "MappingErrorProbe" rule (see testConditionMappingRules)
// divides by zero at runtime and only fires when a "TriggerMappingError" condition is reported.
func TestConditionMapping_CELError_RollsBackTransaction(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Baseline: a freshly created cluster already carries seed conditions
	// (Reconciled=False / LastKnownReconciled=False, "adapters have not reported").
	// The failing PUT must leave this state untouched.
	baselineResp, err := client.GetClusterByIdWithResponse(ctx, cluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(baselineResp.JSON200).NotTo(BeNil())
	baseline := make(map[string]openapi.ResourceConditionStatus)
	for _, cond := range baselineResp.JSON200.Status.Conditions {
		baseline[cond.Type] = cond.Status
	}

	// Mandatory conditions are present so validation passes and aggregation runs;
	// the TriggerMappingError condition then triggers the failing probe rule.
	statusInput := newAdapterStatusRequest(
		"validation",
		cluster.Generation,
		[]openapi.ConditionRequest{
			{
				Type:    api.AdapterConditionTypeAvailable,
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("OK"),
				Message: util.PtrString("OK"),
			},
			{
				Type:    api.AdapterConditionTypeApplied,
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("OK"),
				Message: util.PtrString("OK"),
			},
			{
				Type:    api.AdapterConditionTypeHealth,
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("OK"),
				Message: util.PtrString("OK"),
			},
			{
				Type:    "TriggerMappingError",
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("Trigger"),
				Message: util.PtrString("Triggers the failing probe rule"),
			},
		},
		nil,
	)

	// The PUT must fail with an RFC 9457 error (GeneralError → 500), not persist a partial state.
	resp, err := client.PutClusterStatusesWithResponse(
		ctx, cluster.ID,
		openapi.PutClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusInternalServerError),
		"CEL evaluation failure should surface as an error response")

	// Rollback proof #1: no adapter status was persisted.
	statusesResp, err := client.GetClusterStatusesWithResponse(ctx, cluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(statusesResp.StatusCode()).To(Equal(http.StatusOK))
	Expect(statusesResp.JSON200).NotTo(BeNil())
	Expect(statusesResp.JSON200.Items).To(BeEmpty(),
		"adapter status must not be persisted when the mapping transaction rolls back")

	// Rollback proof #2: the resource conditions are byte-for-byte the pre-PUT baseline.
	// A successful PUT would have flipped Reconciled to True and appended the
	// ValidationSuccessful + mapped conditions; after rollback none of that survives.
	getResp, err := client.GetClusterByIdWithResponse(ctx, cluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(getResp.StatusCode()).To(Equal(http.StatusOK))
	Expect(getResp.JSON200).NotTo(BeNil())

	after := make(map[string]openapi.ResourceConditionStatus)
	for _, cond := range getResp.JSON200.Status.Conditions {
		after[cond.Type] = cond.Status
	}
	Expect(after).To(Equal(baseline),
		"conditions must be unchanged after rollback: no new/mapped conditions and no status flipped")
}
