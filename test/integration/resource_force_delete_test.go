package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	"gopkg.in/resty.v1"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

func createVersionForChannel(
	t *testing.T, svc services.ResourceService, channelID, name string,
) {
	t.Helper()
	version := newVersionResource(name, channelID)
	_, err := svc.Create(t.Context(), "Version", version, nil)
	if err != nil {
		t.Fatalf("Failed to create version: %v", err)
	}
}

func markFinalizing(t *testing.T, h *test.Helper, resourceID string) {
	t.Helper()
	dbSession := h.DBFactory.New(t.Context())
	err := dbSession.Exec(
		"UPDATE resources SET deleted_time = NOW(), deleted_by = 'admin' WHERE id = ?",
		resourceID,
	).Error
	Expect(err).ToNot(HaveOccurred())
}

func TestResourceForceDelete(t *testing.T) {
	t.Run("ChannelWithRestrictedChildren", func(t *testing.T) {
		RegisterTestingT(t)
		svc, h := setupResourceTest(t)
		prefix := uuid.NewString()[:8]

		channel := createChannel(t, svc, fmt.Sprintf("fd-ch-%s", prefix))
		createVersionForChannel(t, svc, channel.ID, fmt.Sprintf("fd-v1-%s", prefix))
		createVersionForChannel(t, svc, channel.ID, fmt.Sprintf("fd-v2-%s", prefix))

		args := &services.ListArguments{Page: 1, Size: 10}
		versions, _, listErr := svc.ListByOwner(
			t.Context(), "Version", channel.ID, args,
		)
		Expect(listErr).To(BeNil())
		Expect(versions).To(HaveLen(2))
		versionIDs := []string{versions[0].ID, versions[1].ID}

		_, deleteErr := svc.Delete(t.Context(), "Channel", channel.ID)
		Expect(deleteErr).ToNot(BeNil())
		Expect(deleteErr.HTTPCode).To(Equal(409))

		markFinalizing(t, h, channel.ID)

		forceErr := svc.ForceDelete(
			t.Context(), "Channel", channel.ID, "stuck in finalizing",
		)
		Expect(forceErr).To(BeNil())

		allIDs := append([]string{channel.ID}, versionIDs...)
		err := checkResourceCount(t.Context(), h, allIDs, 0)
		Expect(err).ToNot(HaveOccurred())
	})

	t.Run("ChannelNoChildren", func(t *testing.T) {
		RegisterTestingT(t)
		svc, h := setupResourceTest(t)
		prefix := uuid.NewString()[:8]

		channel := createChannel(t, svc, fmt.Sprintf("fd-solo-%s", prefix))
		markFinalizing(t, h, channel.ID)

		forceErr := svc.ForceDelete(
			t.Context(), "Channel", channel.ID, "cleanup",
		)
		Expect(forceErr).To(BeNil())

		err := checkResourceCount(t.Context(), h, []string{channel.ID}, 0)
		Expect(err).ToNot(HaveOccurred())
	})

	t.Run("NotInFinalizingState_Returns409", func(t *testing.T) {
		RegisterTestingT(t)
		svc, _ := setupResourceTest(t)
		prefix := uuid.NewString()[:8]

		channel := createChannel(t, svc, fmt.Sprintf("fd-active-%s", prefix))

		forceErr := svc.ForceDelete(
			t.Context(), "Channel", channel.ID, "should fail",
		)
		Expect(forceErr).ToNot(BeNil())
		Expect(forceErr.HTTPCode).To(Equal(409))
	})

	t.Run("NotFound_Returns404", func(t *testing.T) {
		RegisterTestingT(t)
		svc, _ := setupResourceTest(t)

		forceErr := svc.ForceDelete(
			t.Context(), "Channel", "nonexistent-id", "should fail",
		)
		Expect(forceErr).ToNot(BeNil())
		Expect(forceErr.HTTPCode).To(Equal(404))
	})

	t.Run("NestedVersionForceDelete", func(t *testing.T) {
		RegisterTestingT(t)
		svc, h := setupResourceTest(t)
		prefix := uuid.NewString()[:8]

		channel := createChannel(t, svc, fmt.Sprintf("fd-parent-%s", prefix))
		versionName := fmt.Sprintf("fd-ver-%s", prefix)
		createVersionForChannel(t, svc, channel.ID, versionName)

		args := &services.ListArguments{Page: 1, Size: 10}
		versions, _, listErr := svc.ListByOwner(
			t.Context(), "Version", channel.ID, args,
		)
		Expect(listErr).To(BeNil())
		Expect(versions).To(HaveLen(1))
		versionID := versions[0].ID

		markFinalizing(t, h, versionID)

		forceErr := svc.ForceDelete(
			t.Context(), "Version", versionID, "cleanup version",
		)
		Expect(forceErr).To(BeNil())

		err := checkResourceCount(t.Context(), h, []string{versionID}, 0)
		Expect(err).ToNot(HaveOccurred())

		err = checkResourceCount(t.Context(), h, []string{channel.ID}, 1)
		Expect(err).ToNot(HaveOccurred())
	})
}

func TestResourceForceDeleteHTTP(t *testing.T) {
	t.Run("HappyPath_204", func(t *testing.T) {
		RegisterTestingT(t)
		h, _ := test.RegisterIntegration(t)
		svc, _ := setupResourceTest(t)
		prefix := uuid.NewString()[:8]

		account := h.NewRandAccount()
		ctx := h.NewAuthenticatedContext(account)
		token := test.GetAccessTokenFromContext(ctx)

		channel := createChannel(t, svc, fmt.Sprintf("fd-http-%s", prefix))
		markFinalizing(t, h, channel.ID)

		resp, err := resty.R().
			SetHeader("Content-Type", "application/json").
			SetHeader("Authorization", fmt.Sprintf("Bearer %s", token)).
			SetBody(`{"reason": "admin cleanup"}`).
			Post(h.RestURL(fmt.Sprintf("/channels/%s/force-delete", channel.ID)))
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusNoContent))

		err = checkResourceCount(t.Context(), h, []string{channel.ID}, 0)
		Expect(err).ToNot(HaveOccurred())
	})

	t.Run("NestedVersion_204", func(t *testing.T) {
		RegisterTestingT(t)
		h, _ := test.RegisterIntegration(t)
		svc, _ := setupResourceTest(t)
		prefix := uuid.NewString()[:8]

		account := h.NewRandAccount()
		ctx := h.NewAuthenticatedContext(account)
		token := test.GetAccessTokenFromContext(ctx)

		channel := createChannel(t, svc, fmt.Sprintf("fd-nested-%s", prefix))
		createVersionForChannel(t, svc, channel.ID, fmt.Sprintf("fd-nv-%s", prefix))

		args := &services.ListArguments{Page: 1, Size: 10}
		versions, _, listErr := svc.ListByOwner(
			t.Context(), "Version", channel.ID, args,
		)
		Expect(listErr).To(BeNil())
		Expect(versions).To(HaveLen(1))
		versionID := versions[0].ID

		markFinalizing(t, h, versionID)

		url := fmt.Sprintf(
			"/channels/%s/versions/%s/force-delete",
			channel.ID, versionID,
		)
		resp, err := resty.R().
			SetHeader("Content-Type", "application/json").
			SetHeader("Authorization", fmt.Sprintf("Bearer %s", token)).
			SetBody(`{"reason": "nested cleanup"}`).
			Post(h.RestURL(url))
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusNoContent))

		err = checkResourceCount(t.Context(), h, []string{versionID}, 0)
		Expect(err).ToNot(HaveOccurred())

		err = checkResourceCount(t.Context(), h, []string{channel.ID}, 1)
		Expect(err).ToNot(HaveOccurred())
	})

	t.Run("WifConfig_204", func(t *testing.T) {
		RegisterTestingT(t)
		h, _ := test.RegisterIntegration(t)
		svc, _ := setupResourceTest(t)
		prefix := uuid.NewString()[:8]

		account := h.NewRandAccount()
		ctx := h.NewAuthenticatedContext(account)
		token := test.GetAccessTokenFromContext(ctx)

		wifConfig := newWifConfigResource(fmt.Sprintf("fd-wif-%s", prefix))
		created, createErr := svc.Create(t.Context(), "WifConfig", wifConfig, nil)
		Expect(createErr).To(BeNil())

		markFinalizing(t, h, created.ID)

		resp, err := resty.R().
			SetHeader("Content-Type", "application/json").
			SetHeader("Authorization", fmt.Sprintf("Bearer %s", token)).
			SetBody(`{"reason": "wifconfig cleanup"}`).
			Post(h.RestURL(fmt.Sprintf("/wifconfigs/%s/force-delete", created.ID)))
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusNoContent))

		err = checkResourceCount(t.Context(), h, []string{created.ID}, 0)
		Expect(err).ToNot(HaveOccurred())
	})

	t.Run("EmptyReason_400", func(t *testing.T) {
		RegisterTestingT(t)
		h, _ := test.RegisterIntegration(t)
		svc, _ := setupResourceTest(t)
		prefix := uuid.NewString()[:8]

		account := h.NewRandAccount()
		ctx := h.NewAuthenticatedContext(account)
		token := test.GetAccessTokenFromContext(ctx)

		channel := createChannel(t, svc, fmt.Sprintf("fd-noreason-%s", prefix))
		markFinalizing(t, h, channel.ID)

		resp, err := resty.R().
			SetHeader("Content-Type", "application/json").
			SetHeader("Authorization", fmt.Sprintf("Bearer %s", token)).
			SetBody(`{"reason": ""}`).
			Post(h.RestURL(fmt.Sprintf("/channels/%s/force-delete", channel.ID)))
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest))
	})
}
