package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/plugins/resources"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

func setupResourceTest(t *testing.T) (services.ResourceService, *test.Helper) {
	h, _ := test.RegisterIntegration(t)
	svc := resources.Service(&environments.Environment().Services)
	return svc, h
}

func checkResourceCount(ctx context.Context, h *test.Helper, ids []string, expected int64) error {
	dbSession := h.DBFactory.New(ctx)
	var count int64
	err := dbSession.Model(&api.Resource{}).Where("id IN ?", ids).Count(&count).Error
	if err != nil {
		return fmt.Errorf("failed to count resources with ids %v: %w", ids, err)
	}
	if count != expected {
		return fmt.Errorf("expected %d resources, got %d", expected, count)
	}
	return nil
}

func labelsToMap(labels []api.ResourceLabel) map[string]string {
	m := make(map[string]string, len(labels))
	for _, l := range labels {
		m[l.Key] = l.Value
	}
	return m
}

// newChannelResource creates a Channel resource struct with default spec.
// Does NOT persist to database - use svc.Create() to persist.
func newChannelResource(name string) *api.Resource {
	return &api.Resource{
		Kind:      "Channel",
		Name:      name,
		Spec:      []byte(`{"is_default": false, "enabled_regex": ".*"}`),
		CreatedBy: "test@example.com",
		UpdatedBy: "test@example.com",
	}
}

// newVersionResource creates a Version resource struct with default spec.
// Does NOT persist to database - use svc.Create() to persist.
func newVersionResource(name, channelID string) *api.Resource {
	ownerKind := "Channel"
	return &api.Resource{
		Kind:      "Version",
		Name:      name,
		OwnerID:   &channelID,
		OwnerKind: &ownerKind,
		Spec: []byte(`{"raw_version": "4.17.0", "enabled": true, "is_default": false,` +
			`"release_image": "quay.io/openshift-release-dev/ocp-release:4.17.0"}`),
		CreatedBy: "test@example.com",
		UpdatedBy: "test@example.com",
	}
}

func newWifConfigResource(name string) *api.Resource {
	return &api.Resource{
		Kind:      "WifConfig",
		Name:      name,
		Spec:      []byte(`{"project_id": "test-project", "pool_id": "test-pool"}`),
		CreatedBy: "test@example.com",
		UpdatedBy: "test@example.com",
	}
}

// hardDeleteResource directly deletes a resource from the database, bypassing service layer.
// Used to simulate adapter finalization in tests.
func hardDeleteResource(ctx context.Context, h *test.Helper, kind, id string) error {
	dbSession := h.DBFactory.New(ctx)
	result := dbSession.Where("kind = ? AND id = ?", kind, id).Delete(&api.Resource{})
	return result.Error
}
