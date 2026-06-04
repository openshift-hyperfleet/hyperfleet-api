package integration

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"

	_ "github.com/openshift-hyperfleet/hyperfleet-api/plugins/channels"
	"github.com/openshift-hyperfleet/hyperfleet-api/plugins/resources"
	_ "github.com/openshift-hyperfleet/hyperfleet-api/plugins/versions"
)

func resourceService() services.ResourceService {
	return resources.Service(&environments.Environment().Services)
}

func createChannel(svc services.ResourceService, name string) *api.Resource {
	channel := &api.Resource{
		Kind:      "Channel",
		Name:      name,
		Spec:      []byte(`{"stability": "stable"}`),
		CreatedBy: "test@example.com",
		UpdatedBy: "test@example.com",
	}
	created, svcErr := svc.Create(context.Background(), "Channel", channel)
	Expect(svcErr).To(BeNil())
	return created
}

func createVersion(svc services.ResourceService, name string, parent *api.Resource) *api.Resource {
	version := &api.Resource{
		Kind:      "Version",
		Name:      name,
		Spec:      []byte(`{"version": "` + name + `"}`),
		CreatedBy: "test@example.com",
		UpdatedBy: "test@example.com",
	}
	version.SetOwner(parent.ID, parent.Kind, parent.Href)
	created, svcErr := svc.Create(context.Background(), "Version", version)
	Expect(svcErr).To(BeNil())
	return created
}

func assertResourceCount(h *test.Helper, ids []string, expected int64) {
	dbSession := h.DBFactory.New(context.Background())
	var count int64
	err := dbSession.Model(&api.Resource{}).Where("id IN ?", ids).Count(&count).Error
	Expect(err).To(BeNil())
	Expect(count).To(Equal(expected))
}

func TestResourceDelete_HardDeleteRemovesRow(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := resourceService()

	created := createChannel(svc, "hard-delete-test")

	result, svcErr := svc.Delete(context.Background(), "Channel", created.ID)
	Expect(svcErr).To(BeNil())
	Expect(result.DeletedTime).ToNot(BeNil())

	assertResourceCount(h, []string{created.ID}, 0)
}

func TestResourceDelete_RestrictBlocksWithActiveChild(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := resourceService()

	parent := createChannel(svc, "restrict-parent")
	child := createVersion(svc, "4.18", parent)

	_, svcErr := svc.Delete(context.Background(), "Channel", parent.ID)
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(409))

	assertResourceCount(h, []string{parent.ID, child.ID}, 2)
}

func TestResourceDelete_ReDeleteReturns404(t *testing.T) {
	_, _ = test.RegisterIntegration(t)
	svc := resourceService()

	created := createChannel(svc, "redelete-test")

	_, svcErr := svc.Delete(context.Background(), "Channel", created.ID)
	Expect(svcErr).To(BeNil())

	_, svcErr = svc.Delete(context.Background(), "Channel", created.ID)
	Expect(svcErr).ToNot(BeNil())
	Expect(svcErr.HTTPCode).To(Equal(404))
}
