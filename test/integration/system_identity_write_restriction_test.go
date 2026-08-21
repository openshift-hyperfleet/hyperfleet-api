package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	"gorm.io/datatypes"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/tenant"
)

// systemCtx returns a context carrying a system-identity tenant (Sentinel, adapters).
// System identities get unscoped read access but may only write status and conditions.
func systemCtx() context.Context {
	return tenant.WithTenant(context.Background(), &tenant.ResolvedTenant{System: true})
}

// mandatoryAdapterConditionsJSON marshals the three conditions required on every
// adapter status report (Available, Applied, Health).
func mandatoryAdapterConditionsJSON(availableStatus api.AdapterConditionStatus) datatypes.JSON {
	conditions := []api.AdapterCondition{
		{Type: api.AdapterConditionTypeAvailable, Status: availableStatus},
		{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue},
		{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue},
	}
	b, _ := json.Marshal(conditions)
	return b
}

// expectSystemIdentityRejected asserts a system-identity write was rejected with the
// AUZ-001 error code.
func expectSystemIdentityRejected(t *testing.T, svcErr *errors.ServiceError) {
	t.Helper()
	Expect(svcErr).ToNot(BeNil(), "system identity write should have been rejected but succeeded")
	Expect(svcErr.HTTPCode).To(Equal(403))
	Expect(svcErr.RFC9457Code).To(Equal(errors.CodeAuthzPermissionDenied))
}

func TestSystemIdentityWritesAreRejected(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupResourceTest(t)

	tests := []struct {
		action func(t *testing.T) *errors.ServiceError
		op     string
	}{
		{
			op: "create",
			action: func(t *testing.T) *errors.ServiceError {
				channel := newChannelResource("si-create-" + uuid.NewString()[:8])
				_, svcErr := svc.Create(systemCtx(), "Channel", channel, nil)
				return svcErr
			},
		},
		{
			op: "patch",
			action: func(t *testing.T) *errors.ServiceError {
				channel := createChannel(t, svc, "si-patch-"+uuid.NewString()[:8])
				patch := &api.ResourcePatch{Spec: map[string]interface{}{"enabled_regex": ".+"}}
				_, svcErr := svc.Patch(systemCtx(), "Channel", channel.ID, patch)
				return svcErr
			},
		},
		{
			op: "delete",
			action: func(t *testing.T) *errors.ServiceError {
				channel := createChannel(t, svc, "si-delete-"+uuid.NewString()[:8])
				_, svcErr := svc.Delete(systemCtx(), "Channel", channel.ID)
				return svcErr
			},
		},
		{
			op: "force delete",
			action: func(t *testing.T) *errors.ServiceError {
				channel := createChannel(t, svc, "si-force-delete-"+uuid.NewString()[:8])
				return svc.ForceDelete(systemCtx(), "Channel", channel.ID, "test cleanup")
			},
		},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("system identity %s is rejected", tt.op)
		t.Run(name, func(t *testing.T) {
			expectSystemIdentityRejected(t, tt.action(t))
		})
	}
}

// TestSystemIdentityStatusWriteSucceeds verifies the one write path system identities are
// allowed to use continues to work: ProcessAdapterStatus persists the adapter status and
// triggers condition aggregation on the resource.
func TestSystemIdentityStatusWriteSucceeds(t *testing.T) {
	RegisterTestingT(t)
	svc, h := setupResourceTest(t)
	channel := createChannel(t, svc, "si-status-"+uuid.NewString()[:8])

	adapterStatus := &api.AdapterStatus{
		Adapter:            "test-adapter",
		ObservedGeneration: channel.Generation,
		LastReportTime:     time.Now().UTC(),
		Conditions:         mandatoryAdapterConditionsJSON(api.AdapterConditionTrue),
	}

	result, svcErr := svc.ProcessAdapterStatus(systemCtx(), "Channel", channel.ID, adapterStatus)
	Expect(svcErr).To(BeNil())
	Expect(result).ToNot(BeNil())

	// Adapter status should be stored.
	statuses, err := h.Container.AdapterStatusDao().FindByResource(t.Context(), "Channel", channel.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(statuses).To(HaveLen(1))

	// Conditions should be written on the resource (aggregation triggered by Available=True).
	updated, svcErr := svc.Get(t.Context(), "Channel", channel.ID)
	Expect(svcErr).To(BeNil())
	Expect(updated.Conditions).ToNot(BeEmpty())
}

// TestNonSystemCallerWritesUnaffected is a regression check: the system-identity guard must
// not affect ordinary callers. Create and Patch continue to work unchanged.
func TestNonSystemCallerWritesUnaffected(t *testing.T) {
	RegisterTestingT(t)
	svc, _ := setupResourceTest(t)

	channel := createChannel(t, svc, "si-non-system-"+uuid.NewString()[:8])
	Expect(channel).ToNot(BeNil())

	patch := &api.ResourcePatch{Spec: map[string]interface{}{"enabled_regex": ".+"}}
	updated, svcErr := svc.Patch(t.Context(), "Channel", channel.ID, patch)
	Expect(svcErr).To(BeNil())
	Expect(updated).ToNot(BeNil())
}
