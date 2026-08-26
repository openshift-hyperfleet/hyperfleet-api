package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	"gopkg.in/resty.v1"

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
// an RFC 9457 error response.
//
// It runs against the dedicated "MappingProbe" entity (see defaultTestEntities), whose
// only mapping rule is the "MappingErrorProbe" (see probeConditionMappingRules): it
// compiles fine but divides by zero at CEL evaluation time and only fires when a
// "TriggerMappingError" condition is reported. Isolating the failing rule on its own
// entity keeps the failure injection out of every other entity's status flow.
func TestConditionMapping_CELError_RollsBackTransaction(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := h.Container.ResourceService()
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	token := test.GetAccessTokenFromContext(ctx)

	probe, svcErr := svc.Create(ctx, "MappingProbe", &api.Resource{
		Kind: "MappingProbe",
		Name: fmt.Sprintf("probe-%s", uuid.NewString()[:8]),
		Spec: []byte(`{}`),
	}, nil)
	Expect(svcErr).To(BeNil())

	statusesPath := "/mappingprobes/" + probe.ID + "/statuses"

	// Baseline: a freshly created MappingProbe already carries seed conditions
	// (Reconciled=False / LastKnownReconciled=False, "adapters have not reported")
	// because it declares required adapters. The failing PUT must leave this untouched.
	baselineResource, svcErr := svc.Get(ctx, "MappingProbe", probe.ID)
	Expect(svcErr).To(BeNil())
	baseline := conditionsByTypeJSON(baselineResource.Conditions)
	Expect(baseline).ToNot(BeEmpty(), "seed conditions should exist before the failing PUT")

	// Mandatory conditions are present so validation passes and aggregation runs;
	// the TriggerMappingError condition then triggers the failing probe rule.
	statusInput := newAdapterStatusRequest(
		"validation",
		probe.Generation,
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
	body, err := json.Marshal(statusInput)
	Expect(err).NotTo(HaveOccurred())

	// The PUT must fail with an RFC 9457 error (GeneralError → 500), not persist a partial state.
	putResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", token)).
		SetBody(body).
		Put(h.RestURL(statusesPath))
	Expect(err).NotTo(HaveOccurred())
	Expect(putResp.StatusCode()).To(Equal(http.StatusInternalServerError),
		"CEL evaluation failure should surface as an error response")

	// Assert it is specifically the CEL-mapping error surfaced as an RFC 9457
	// problem document — not some unrelated 500 that would let the mapping error
	// path silently stop being exercised.
	Expect(putResp.Header().Get("Content-Type")).To(HavePrefix("application/problem+json"),
		"error must be an RFC 9457 problem document")
	Expect(string(putResp.Body())).To(ContainSubstring("Condition mapping failed"),
		"problem detail must identify the CEL mapping failure as the cause")

	// Rollback proof #1: no adapter status was persisted.
	getStatusesResp, err := resty.R().
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", token)).
		Get(h.RestURL(statusesPath))
	Expect(err).NotTo(HaveOccurred())
	Expect(getStatusesResp.StatusCode()).To(Equal(http.StatusOK))
	var statusList openapi.AdapterStatusList
	Expect(json.Unmarshal(getStatusesResp.Body(), &statusList)).To(Succeed())
	Expect(statusList.Items).To(BeEmpty(),
		"adapter status must not be persisted when the mapping transaction rolls back")

	// Rollback proof #2: the resource conditions are byte-for-byte the pre-PUT baseline.
	// A successful PUT would have flipped Reconciled to True and appended the
	// ValidationSuccessful + mapped conditions; after rollback none of that survives.
	// Comparing the full JSON of each condition (not just its status) catches a
	// partial-commit bug that changed a reason, message, or timestamp while leaving
	// status values identical.
	afterResource, svcErr := svc.Get(ctx, "MappingProbe", probe.ID)
	Expect(svcErr).To(BeNil())
	Expect(conditionsByTypeJSON(afterResource.Conditions)).To(Equal(baseline),
		"conditions must be unchanged after rollback: no new/mapped conditions and no field altered")
}

// conditionsByTypeJSON indexes a resource's conditions by type, mapping each to
// the full JSON of the condition. Keying by type makes the comparison
// order-independent, while comparing the marshaled condition (rather than just
// its status) detects changes to any field, including reason, message, and
// timestamps.
func conditionsByTypeJSON(conditions []api.ResourceCondition) map[string]string {
	m := make(map[string]string, len(conditions))
	for _, cond := range conditions {
		b, err := json.Marshal(cond)
		Expect(err).NotTo(HaveOccurred())
		m[cond.Type] = string(b)
	}
	return m
}
