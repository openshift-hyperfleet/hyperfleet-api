package integration

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

func createTestChannel(t *testing.T, svc services.ResourceService) *api.Resource {
	t.Helper()
	// unique string for channel name
	uniqueSuffix := uuid.NewString()[:8]
	channelName := fmt.Sprintf("channel-%s", uniqueSuffix)

	channel := newChannelResource(channelName)
	created, err := svc.Create(t.Context(), "Channel", channel)
	if err != nil {
		t.Fatalf("Failed to create channel: %v", err)
	}
	return created
}

func createTestVersion(t *testing.T, svc services.ResourceService, name, channelID string) *api.Resource {
	t.Helper()
	version := newVersionResource(name, channelID)
	created, err := svc.Create(t.Context(), "Version", version)
	if err != nil {
		t.Fatalf("Failed to create version: %v", err)
	}
	return created
}

func expectCreateError(t *testing.T, svc services.ResourceService,
	resource *api.Resource, expectedCode int, msg string,
) {
	t.Helper()
	_, svcErr := svc.Create(t.Context(), resource.Kind, resource)
	Expect(svcErr).ToNot(BeNil(), msg)
	Expect(svcErr.HTTPCode).To(Equal(expectedCode))
}

// Version Create
func TestVersionCreate(t *testing.T) {
	t.Run("UniqueConstraintPerChannel", func(t *testing.T) {
		svc, _ := setupResourceTest(t)
		channel := createTestChannel(t, svc)

		versionName := "4.17.0"
		version1 := createTestVersion(t, svc, versionName, channel.ID)
		Expect(version1.ID).NotTo(BeEmpty())

		// Attempt to create duplicate - should fail
		duplicate := newVersionResource(versionName, channel.ID)
		expectCreateError(t, svc, duplicate, 409, "duplicate version name should fail")
	})

	t.Run("SameVersionNameInDifferentChannels", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channel1 := createTestChannel(t, svc)
		channel2 := createTestChannel(t, svc)

		versionName := "version"
		version1 := createTestVersion(t, svc, versionName, channel1.ID)
		version2 := createTestVersion(t, svc, versionName, channel2.ID)

		Expect(version1.ID).NotTo(BeEmpty())
		Expect(version2.ID).NotTo(BeEmpty())
	})

	t.Run("EmptyName", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channel := createTestChannel(t, svc)
		version := newVersionResource("", channel.ID)
		expectCreateError(t, svc, version, 400, "empty version name should fail")
	})

	t.Run("WrongParentKind", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		ownerKind := "RandomKind_NotChannel"
		randomID := uuid.NewString()
		version := &api.Resource{
			Kind:      "Version",
			Name:      "version-wrong-parent-kind",
			OwnerID:   &randomID,
			OwnerKind: &ownerKind,
			Spec:      []byte(`{"raw_version": "4.17.0", "enabled": true}`),
			CreatedBy: "test@example.com",
			UpdatedBy: "test@example.com",
		}
		expectCreateError(t, svc, version, 404, "wrong parent kind should fail")
	})

	t.Run("ParentNotFound", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		nonExistentParentID := "non-existent-channel-id"
		version := newVersionResource("orphan-version", nonExistentParentID)
		expectCreateError(t, svc, version, 404, "non-existent parent should fail")
	})

	t.Run("WithLabels", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channel := createTestChannel(t, svc)
		version := newVersionResource("version-with-labels", channel.ID)
		labels := map[string]string{
			"environment": "test",
			"team":        "platform",
		}

		var err error
		version.Labels, err = json.Marshal(labels)
		Expect(err).To(BeNil(), "should marshal labels")
		createdVersion, svcErr := svc.Create(t.Context(), "Version", version)
		Expect(svcErr).To(BeNil())
		Expect(createdVersion.Labels).NotTo(BeNil())

		// Get the resource and verify labels persisted
		retrieved, getErr := svc.Get(t.Context(), "Version", createdVersion.ID)
		Expect(getErr).To(BeNil(), "should retrieve version")
		var retrievedLabels map[string]string
		jsonErr := json.Unmarshal(retrieved.Labels, &retrievedLabels)
		Expect(jsonErr).To(BeNil(), "should unmarshal retrieved labels")
		Expect(retrievedLabels).To(Equal(labels), "retrieved labels should match")
	})

	t.Run("SetsTimestamps", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channel := createTestChannel(t, svc)
		before := time.Now()
		version := createTestVersion(t, svc, "timestamp-test-version", channel.ID)
		after := time.Now()

		Expect(version.CreatedTime).To(BeTemporally(">=", before),
			"created_time should be set during create")
		Expect(version.CreatedTime).To(BeTemporally("<=", after),
			"created_time should be set during create")
		Expect(version.UpdatedTime).To(BeTemporally(">=", before),
			"updated_time should be set during create")
		Expect(version.UpdatedTime).To(BeTemporally("<=", after),
			"updated_time should be set during create")
	})
}

// Version Delete
func TestVersionDelete(t *testing.T) {
	t.Run("VersionDeleteWithChannel", func(t *testing.T) {
		svc, h := setupResourceTest(t)

		channel := createTestChannel(t, svc)
		version1 := createTestVersion(t, svc, "version1", channel.ID)
		version2 := createTestVersion(t, svc, "version2", channel.ID)

		// Delete all versions first
		_, svcErr := svc.Delete(t.Context(), "Version", version1.ID)
		Expect(svcErr).To(BeNil())
		_, svcErr = svc.Delete(t.Context(), "Version", version2.ID)
		Expect(svcErr).To(BeNil())

		// Now delete channel
		_, svcErr = svc.Delete(t.Context(), "Channel", channel.ID)
		Expect(svcErr).To(BeNil())

		// Verify all deleted
		dbErr := checkResourceCount(t.Context(), h, []string{channel.ID, version1.ID, version2.ID}, 0)
		Expect(dbErr).To(BeNil())
	})

	t.Run("NotFound", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		_, svcErr := svc.Delete(t.Context(), "Version", "nonexistent-id")
		Expect(svcErr).ToNot(BeNil(), "delete of nonexistent version should fail")
		Expect(svcErr.HTTPCode).To(Equal(404), "should return 404 Not Found")
	})

	t.Run("RestrictParentDeleteWithActiveChild", func(t *testing.T) {
		svc, h := setupResourceTest(t)

		channel := createTestChannel(t, svc)
		version := createTestVersion(t, svc, "restrict-test", channel.ID)

		// Attempt to delete parent with active child - should fail with 409
		_, svcErr := svc.Delete(t.Context(), "Channel", channel.ID)
		Expect(svcErr).ToNot(BeNil())
		Expect(svcErr.HTTPCode).To(Equal(409))

		// Verify both still exist
		dbErr := checkResourceCount(t.Context(), h, []string{channel.ID, version.ID}, 2)
		Expect(dbErr).To(BeNil())

		_, svcErr = svc.Delete(t.Context(), "Version", version.ID)
		Expect(svcErr).To(BeNil())

		_, svcErr = svc.Delete(t.Context(), "Channel", channel.ID)
		Expect(svcErr).To(BeNil())

		dbErr = checkResourceCount(t.Context(), h, []string{channel.ID, version.ID}, 0)
		Expect(dbErr).To(BeNil())
	})

	t.Run("SetsDeletedTime", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channel := createTestChannel(t, svc)
		version := createTestVersion(t, svc, "delete-timestamp-test", channel.ID)

		before := time.Now()
		deleted, svcErr := svc.Delete(t.Context(), "Version", version.ID)
		after := time.Now()

		Expect(svcErr).To(BeNil())
		Expect(deleted.DeletedTime).NotTo(BeNil(), "deleted_time should be set on delete")
		Expect(*deleted.DeletedTime).To(BeTemporally(">=", before))
		Expect(*deleted.DeletedTime).To(BeTemporally("<=", after))
	})

	t.Run("ReDeleteReturns404", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channel := createTestChannel(t, svc)
		version := createTestVersion(t, svc, "redelete-test", channel.ID)

		// Delete once - should succeed
		_, svcErr := svc.Delete(t.Context(), "Version", version.ID)
		Expect(svcErr).To(BeNil())

		// Delete again - should return 404
		_, svcErr = svc.Delete(t.Context(), "Version", version.ID)
		Expect(svcErr).ToNot(BeNil())
		Expect(svcErr.HTTPCode).To(Equal(404))
	})

	t.Run("HardDeleteRemovesRow", func(t *testing.T) {
		svc, h := setupResourceTest(t)

		channel := createTestChannel(t, svc)
		version := createTestVersion(t, svc, "hard-delete-test", channel.ID)

		// Delete the version
		result, svcErr := svc.Delete(t.Context(), "Version", version.ID)
		Expect(svcErr).To(BeNil())
		Expect(result.DeletedTime).ToNot(BeNil())

		// Verify hard delete removed the row
		dbErr := checkResourceCount(t.Context(), h, []string{version.ID}, 0)
		Expect(dbErr).To(BeNil())
	})
}

// Version List
func TestVersionList(t *testing.T) {
	t.Run("ListByOwnerID", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channel := createTestChannel(t, svc)

		version1 := createTestVersion(t, svc, "version1", channel.ID)
		version2 := createTestVersion(t, svc, "version2", channel.ID)
		version3 := createTestVersion(t, svc, "version3", channel.ID)

		// List versions in channel1
		args := &services.ListArguments{
			Page:   1,
			Size:   100,
			Search: "owner_id='" + channel.ID + "'",
		}
		list, _, svcErr := svc.List(t.Context(), "Version", args)
		Expect(svcErr).To(BeNil(), "list should succeed")
		Expect(list).To(HaveLen(3), "channel1 should have 3 versions")

		// Verify all versions belong to channel1
		foundIDs := make(map[string]bool)
		for _, item := range list {
			Expect(item.OwnerID).NotTo(BeNil())
			Expect(*item.OwnerID).To(Equal(channel.ID), "all items should belong to channel")
			foundIDs[item.ID] = true
		}

		// Verify we got all 3 expected versions
		Expect(foundIDs[version1.ID]).To(BeTrue(), "should find version1")
		Expect(foundIDs[version2.ID]).To(BeTrue(), "should find version2")
		Expect(foundIDs[version3.ID]).To(BeTrue(), "should find version3")
	})

	t.Run("ListByLabel", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channel := createTestChannel(t, svc)
		uniqueLabel := uuid.NewString()[:8]

		// Create version with unique label
		version1 := newVersionResource("version-with-label-1", channel.ID)
		labels := map[string]string{
			"environment": uniqueLabel,
		}
		var err error
		version1.Labels, err = json.Marshal(labels)
		Expect(err).To(BeNil(), "should marshal labels")
		created1, svcErr := svc.Create(t.Context(), "Version", version1)
		Expect(svcErr).To(BeNil())
		Expect(created1.Labels).NotTo(BeNil())

		// Create another version with the same label
		version2 := newVersionResource("version-with-label-2", channel.ID)
		version2.Labels, err = json.Marshal(labels)
		Expect(err).To(BeNil(), "should marshal labels")
		created2, svcErr := svc.Create(t.Context(), "Version", version2)
		Expect(svcErr).To(BeNil())
		Expect(created2.Labels).NotTo(BeNil())

		// Create version without the label
		version3 := createTestVersion(t, svc, "version-no-label", channel.ID)

		// List by label
		args := &services.ListArguments{
			Page:   1,
			Size:   100,
			Search: "labels.environment='" + uniqueLabel + "'",
		}
		list, _, svcErr := svc.List(t.Context(), "Version", args)
		Expect(svcErr).To(BeNil(), "list by label should succeed")
		Expect(len(list)).To(BeNumerically(">=", 2), "should find at least 2 versions with the label")

		// Verify all returned versions have the label
		for _, item := range list {
			var itemLabels map[string]string
			err := json.Unmarshal(item.Labels, &itemLabels)
			Expect(err).To(BeNil())
			Expect(itemLabels["environment"]).To(Equal(uniqueLabel))
		}

		// Verify version3 is not in the list
		for _, item := range list {
			Expect(item.ID).NotTo(Equal(version3.ID), "version without label should not be in results")
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channel := createTestChannel(t, svc)
		for i := range 15 {
			version := createTestVersion(t, svc, fmt.Sprintf("version-%d", i), channel.ID)
			Expect(version.ID).ToNot(BeEmpty())
		}
		// Page 1
		args := &services.ListArguments{
			Page:   1,
			Size:   10,
			Search: "owner_id='" + channel.ID + "'",
		}
		list, _, svcErr := svc.List(t.Context(), "Version", args)
		Expect(svcErr).To(BeNil(), "list should succeed")
		Expect(list).To(HaveLen(10), "page 1 should have 10 items")

		// Page 2
		args.Page = 2
		list, _, svcErr = svc.List(t.Context(), "Version", args)
		Expect(svcErr).To(BeNil(), "list should succeed")
		Expect(len(list)).To(BeNumerically(">=", 5), "page 2 should have at least 5 items")
	})

	t.Run("ByOwner", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channel := createTestChannel(t, svc)
		version1 := createTestVersion(t, svc, "version1", channel.ID)
		version2 := createTestVersion(t, svc, "version2", channel.ID)
		version3 := createTestVersion(t, svc, "version3", channel.ID)

		// List versions by owner
		list, _, svcErr := svc.ListByOwner(t.Context(), "Version", channel.ID, &services.ListArguments{
			Page: 1,
			Size: 100,
		})
		Expect(svcErr).To(BeNil(), "list by owner should succeed")
		Expect(list).To(HaveLen(3), "should have 3 versions")

		// Verify all versions belong to the channel
		foundIDs := make(map[string]bool)
		for _, item := range list {
			Expect(item.OwnerID).NotTo(BeNil())
			Expect(*item.OwnerID).To(Equal(channel.ID), "all items should belong to channel")
			foundIDs[item.ID] = true
		}

		// Verify we got all 3 expected versions
		Expect(foundIDs[version1.ID]).To(BeTrue(), "should find version1")
		Expect(foundIDs[version2.ID]).To(BeTrue(), "should find version2")
		Expect(foundIDs[version3.ID]).To(BeTrue(), "should find version3")
	})
}

// Version Get
func TestVersionGet(t *testing.T) {
	t.Run("ByOwnerWrongParent", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channel1 := createTestChannel(t, svc)
		version := createTestVersion(t, svc, "version", channel1.ID)

		channel2 := createTestChannel(t, svc)

		// Attempt to get version from channel2 (wrong parent)
		_, svcErr := svc.GetByOwner(t.Context(), "Version", version.ID, channel2.ID)
		Expect(svcErr).ToNot(BeNil(), "version should not be found under wrong parent")
		Expect(svcErr.HTTPCode).To(Equal(404), "should return 404 Not Found")
	})

	t.Run("NotFound", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		_, svcErr := svc.Get(t.Context(), "Version", "nonexistent-id")
		Expect(svcErr).ToNot(BeNil(), "get of nonexistent version should fail")
		Expect(svcErr.HTTPCode).To(Equal(404), "should return 404 Not Found")
	})

	t.Run("ByOwnerNotFound", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channel := createTestChannel(t, svc)

		// Try to get non-existent version under a real parent
		_, svcErr := svc.GetByOwner(t.Context(), "Version", "nonexistent-version-id", channel.ID)
		Expect(svcErr).ToNot(BeNil(), "get of nonexistent version should fail")
		Expect(svcErr.HTTPCode).To(Equal(404), "should return 404 Not Found")
	})

	t.Run("ByOwnerSuccess", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channel := createTestChannel(t, svc)
		version := createTestVersion(t, svc, "test-version", channel.ID)

		// Get version by owner should succeed
		retrieved, svcErr := svc.GetByOwner(t.Context(), "Version", version.ID, channel.ID)
		Expect(svcErr).To(BeNil(), "get by owner should succeed")
		Expect(retrieved.ID).To(Equal(version.ID))
		Expect(retrieved.Name).To(Equal(version.Name))
		Expect(retrieved.OwnerID).NotTo(BeNil())
		Expect(*retrieved.OwnerID).To(Equal(channel.ID))
	})
}

// Version Patch
func TestVersionPatch(t *testing.T) {
	t.Run("NotFound", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		req := &api.ResourcePatch{
			Spec: map[string]any{"enabled": true},
		}

		_, svcErr := svc.Patch(t.Context(), "Version", "nonexistent-id", req)
		Expect(svcErr).ToNot(BeNil(), "patch of nonexistent version should fail")
		Expect(svcErr.HTTPCode).To(Equal(404), "should return 404 Not Found")
	})

	t.Run("SpecChanged", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channel := createTestChannel(t, svc)
		version := createTestVersion(t, svc, "patch-spec-test", channel.ID)
		Expect(version.Generation).To(Equal(int32(1)), "initial generation should be 1")

		// Patch spec
		newSpec := map[string]any{
			"raw_version":   "4.18.0",
			"enabled":       true,
			"is_default":    false,
			"release_image": "quay.io/openshift-release-dev/ocp-release:4.18.0",
		}
		req := &api.ResourcePatch{
			Spec: newSpec,
		}
		_, svcErr := svc.Patch(t.Context(), "Version", version.ID, req)

		Expect(svcErr).To(BeNil(), "patch should succeed")

		// Verify patched request persisted
		retrieved, svcErr := svc.Get(t.Context(), "Version", version.ID)
		Expect(svcErr).To(BeNil())
		// Verify updated version is incremented
		Expect(retrieved.Generation).To(Equal(int32(2)))
		// Verify updated spec persisted
		var retrievedSpecValues map[string]any
		jsonErr := json.Unmarshal(retrieved.Spec, &retrievedSpecValues)
		Expect(jsonErr).To(BeNil())
		Expect(retrievedSpecValues).To(Equal(newSpec), "patched spec should match retrieved spec")
	})

	t.Run("LabelsOnly", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channel := createTestChannel(t, svc)
		version := createTestVersion(t, svc, "patch-labels-test", channel.ID)
		Expect(version.Generation).To(Equal(int32(1)), "initial generation should be 1")

		// Patch labels only, no spec change
		newLabels := map[string]string{
			"patched":     "true",
			"environment": "test",
		}
		req := &api.ResourcePatch{
			Labels: newLabels,
		}
		_, svcErr := svc.Patch(t.Context(), "Version", version.ID, req)
		Expect(svcErr).To(BeNil())

		retrieved, getErr := svc.Get(t.Context(), "Version", version.ID)
		Expect(getErr).To(BeNil())
		// Verify updated version is incremented
		Expect(retrieved.Generation).To(Equal(int32(2)))
		var retrievedLabelValues map[string]string
		jsonErr := json.Unmarshal(retrieved.Labels, &retrievedLabelValues)
		Expect(jsonErr).To(BeNil())
		Expect(retrievedLabelValues).To(Equal(newLabels), "patched labels should match retrieved labels")
	})

	t.Run("UpdatesTimestamps", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channel := createTestChannel(t, svc)
		version := createTestVersion(t, svc, "timestamp-update-test", channel.ID)
		originalUpdatedTime := version.UpdatedTime

		// Patch the version
		newSpec := map[string]any{"enabled": false}
		req := &api.ResourcePatch{
			Spec: newSpec,
		}
		time.Sleep(5 * time.Millisecond)
		patched, svcErr := svc.Patch(t.Context(), "Version", version.ID, req)

		Expect(svcErr).To(BeNil())
		Expect(patched.UpdatedTime).To(BeTemporally(">", originalUpdatedTime),
			"updated_time should be later than original after patch")
	})
}
