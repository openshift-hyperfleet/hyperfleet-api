package services

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

func extractPrevReconciledStatus(ctx context.Context, raw []byte) *api.ResourceConditionStatus {
	prevReconciled, _, _ := parsePrevConditions(ctx, raw)
	if prevReconciled == nil {
		return nil
	}
	return &prevReconciled.Status
}
