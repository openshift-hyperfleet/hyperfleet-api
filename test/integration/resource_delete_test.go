package integration

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
)

// TestResourceDelete_ParentChildWithRequiredAdapters tests the parent/child delete behavior
// when the child has RequiredAdapters configured.
//
// This test validates the fix for preventing parent hard-delete while child is soft-deleted.
func TestResourceDelete_ParentChildWithRequiredAdapters(t *testing.T) {
	t.Run("ParentSoftDeletedWhileChildSoftDeleted", func(t *testing.T) {
		RegisterTestingT(t)
		svc, h := setupResourceTest(t)

		// Temporarily add RequiredAdapters to Version for this test
		registry.UpdateDescriptor("Version", func(d *registry.EntityDescriptor) {
			d.RequiredAdapters = []string{"test-adapter"}
		})
		t.Cleanup(func() {
			registry.UpdateDescriptor("Version", func(d *registry.EntityDescriptor) {
				d.RequiredAdapters = nil
			})
		})

		// Create Channel (parent)
		channelName := fmt.Sprintf("test-delete-channel-%s", uuid.NewString()[:8])
		channel := newChannelResource(channelName)
		createdChannel, svcErr := svc.Create(t.Context(), "Channel", channel, nil)
		Expect(svcErr).To(BeNil(), "Channel creation should succeed")
		Expect(createdChannel.ID).NotTo(BeEmpty())

		// Create Version (child with RequiredAdapters)
		versionName := fmt.Sprintf("v1.0.0-%s", uuid.NewString()[:8])
		version := newVersionResource(versionName, createdChannel.ID)
		createdVersion, svcErr := svc.Create(t.Context(), "Version", version, nil)
		Expect(svcErr).To(BeNil(), "Version creation should succeed")
		Expect(createdVersion.ID).NotTo(BeEmpty())

		// Verify Version was created
		retrievedVersion, svcErr := svc.Get(t.Context(), "Version", createdVersion.ID)
		Expect(svcErr).To(BeNil())
		Expect(retrievedVersion.DeletedTime).To(BeNil(), "Version should not be deleted yet")

		// Try to delete Channel with active child - should fail with 409
		_, svcErr = svc.Delete(t.Context(), "Channel", createdChannel.ID)
		Expect(svcErr).ToNot(BeNil(), "Deleting Channel with active child should fail")
		Expect(svcErr.HTTPCode).To(Equal(409), "Should return 409 Conflict")
		Expect(svcErr.Reason).To(ContainSubstring("active"), "Error should mention active children")

		// Delete Version - should be soft-deleted (has RequiredAdapters)
		deletedVersion, svcErr := svc.Delete(t.Context(), "Version", createdVersion.ID)
		Expect(svcErr).To(BeNil(), "Version deletion should succeed")
		Expect(deletedVersion.DeletedTime).ToNot(BeNil(), "Version should be soft-deleted")

		// Verify Version is soft-deleted in database
		versionAfterDelete, svcErr := svc.Get(t.Context(), "Version", createdVersion.ID)
		Expect(svcErr).To(BeNil(), "Should retrieve soft-deleted Version")
		Expect(versionAfterDelete.DeletedTime).ToNot(BeNil(), "Version should have deleted_time set")

		// Delete Channel - should be SOFT-DELETED (not hard-deleted)
		// This is the key test: parent must be soft-deleted while child is soft-deleted
		deletedChannel, svcErr := svc.Delete(t.Context(), "Channel", createdChannel.ID)
		Expect(svcErr).To(BeNil(), "Channel deletion should succeed")
		Expect(deletedChannel.DeletedTime).ToNot(BeNil(), "Channel should be soft-deleted")

		// VALIDATION: Verify Channel is soft-deleted in database (not hard-deleted)
		channelAfterDelete, svcErr := svc.Get(t.Context(), "Channel", createdChannel.ID)
		Expect(svcErr).To(BeNil(), "Should retrieve soft-deleted Channel")
		Expect(channelAfterDelete.DeletedTime).ToNot(BeNil(), "Channel should have deleted_time set")

		// Verify both resources still exist in database (soft-deleted)
		err := checkResourceCount(t.Context(), h, []string{createdChannel.ID, createdVersion.ID}, 2)
		Expect(err).To(BeNil(), "Both Channel and Version should still exist in DB (soft-deleted)")

		t.Logf("✓ Channel was soft-deleted while Version is soft-deleted (fix working)")

		// CONVERGENCE TEST: Simulate adapter finalization and verify parent can be hard-deleted
		// This tests the re-evaluation path: soft-deleted parent → child removed → hard-delete
		err = hardDeleteResource(t.Context(), h, "Version", createdVersion.ID)
		Expect(err).To(BeNil(), "Direct DB deletion should succeed (simulates adapter finalization)")

		// Verify Version is gone from database
		err = checkResourceCount(t.Context(), h, []string{createdVersion.ID}, 0)
		Expect(err).To(BeNil(), "Version should be hard-deleted from DB")

		// Re-delete the soft-deleted Channel - should now hard-delete
		// This exercises the re-evaluation: Channel was soft-deleted, child is now gone,
		// so Delete() should detect no blockers and hard-delete the parent
		_, svcErr = svc.Delete(t.Context(), "Channel", createdChannel.ID)
		Expect(svcErr).To(BeNil(), "Channel re-delete should succeed")

		// Verify Channel is hard-deleted (404)
		_, svcErr = svc.Get(t.Context(), "Channel", createdChannel.ID)
		Expect(svcErr).ToNot(BeNil(), "Should not retrieve hard-deleted Channel")
		Expect(svcErr.HTTPCode).To(Equal(404), "Should return 404 for hard-deleted Channel")

		// Verify Channel is gone from database
		err = checkResourceCount(t.Context(), h, []string{createdChannel.ID}, 0)
		Expect(err).To(BeNil(), "Channel should be hard-deleted after re-evaluation")

		t.Logf("✓ Channel hard-deleted after child finalization (convergence working)")
	})

	t.Run("ParentHardDeletedAfterChildrenGone", func(t *testing.T) {
		RegisterTestingT(t)
		svc, h := setupResourceTest(t)

		// Ensure Channel has NO RequiredAdapters for this test
		registry.UpdateDescriptor("Channel", func(d *registry.EntityDescriptor) {
			d.RequiredAdapters = nil
		})
		t.Cleanup(func() {
			registry.UpdateDescriptor("Channel", func(d *registry.EntityDescriptor) {
				d.RequiredAdapters = nil
			})
		})

		// Create Channel without children
		channelName := fmt.Sprintf("test-delete-orphan-%s", uuid.NewString()[:8])
		channel := newChannelResource(channelName)
		createdChannel, svcErr := svc.Create(t.Context(), "Channel", channel, nil)
		Expect(svcErr).To(BeNil(), "Channel creation should succeed")

		// Delete Channel (no children, no RequiredAdapters) - should be HARD-DELETED
		_, svcErr = svc.Delete(t.Context(), "Channel", createdChannel.ID)
		Expect(svcErr).To(BeNil(), "Channel deletion should succeed")

		// Verify Channel is hard-deleted (removed from DB)
		_, svcErr = svc.Get(t.Context(), "Channel", createdChannel.ID)
		Expect(svcErr).ToNot(BeNil(), "Should not retrieve hard-deleted Channel")
		Expect(svcErr.HTTPCode).To(Equal(404), "Should return 404 for hard-deleted resource")

		// Verify Channel no longer exists in database
		err := checkResourceCount(t.Context(), h, []string{createdChannel.ID}, 0)
		Expect(err).To(BeNil(), "Channel should be hard-deleted (not in DB)")

		t.Logf("✓ Channel was hard-deleted when it has no children (no regression)")
	})

	t.Run("ActiveChildBlocksParentDelete", func(t *testing.T) {
		RegisterTestingT(t)
		svc, _ := setupResourceTest(t)

		// Create Channel
		channelName := fmt.Sprintf("test-restrict-%s", uuid.NewString()[:8])
		channel := newChannelResource(channelName)
		createdChannel, svcErr := svc.Create(t.Context(), "Channel", channel, nil)
		Expect(svcErr).To(BeNil())

		// Create active Version
		versionName := fmt.Sprintf("v1.0.0-%s", uuid.NewString()[:8])
		version := newVersionResource(versionName, createdChannel.ID)
		_, svcErr = svc.Create(t.Context(), "Version", version, nil)
		Expect(svcErr).To(BeNil())

		// Try to delete Channel - should fail (OnParentDelete=Restrict)
		_, svcErr = svc.Delete(t.Context(), "Channel", createdChannel.ID)
		Expect(svcErr).ToNot(BeNil(), "Should not delete Channel with active child")
		Expect(svcErr.HTTPCode).To(Equal(409))
		Expect(svcErr.Reason).To(ContainSubstring("active"), "Error should mention active children")

		t.Logf("✓ Active child blocks parent delete (AC1 validated)")
	})
}

// TestResourceDelete_WithoutRequiredAdapters tests delete behavior when Version
// does NOT have RequiredAdapters configured (hard-delete scenario).
func TestResourceDelete_WithoutRequiredAdapters(t *testing.T) {
	t.Run("ChildHardDeletedImmediately", func(t *testing.T) {
		RegisterTestingT(t)

		// Ensure Version has NO RequiredAdapters for this test
		registry.UpdateDescriptor("Version", func(d *registry.EntityDescriptor) {
			d.RequiredAdapters = nil
		})
		t.Cleanup(func() {
			registry.UpdateDescriptor("Version", func(d *registry.EntityDescriptor) {
				d.RequiredAdapters = nil
			})
		})

		svc, h := setupResourceTest(t)

		// Create Channel
		channelName := fmt.Sprintf("test-harddelete-%s", uuid.NewString()[:8])
		channel := newChannelResource(channelName)
		createdChannel, svcErr := svc.Create(t.Context(), "Channel", channel, nil)
		Expect(svcErr).To(BeNil())

		// Create Version (no RequiredAdapters)
		versionName := fmt.Sprintf("v1.0.0-%s", uuid.NewString()[:8])
		version := newVersionResource(versionName, createdChannel.ID)
		createdVersion, svcErr := svc.Create(t.Context(), "Version", version, nil)
		Expect(svcErr).To(BeNil())

		// Delete Version - should be HARD-DELETED (no RequiredAdapters)
		_, svcErr = svc.Delete(t.Context(), "Version", createdVersion.ID)
		Expect(svcErr).To(BeNil())

		// Verify Version is hard-deleted (404)
		_, svcErr = svc.Get(t.Context(), "Version", createdVersion.ID)
		Expect(svcErr).ToNot(BeNil())
		Expect(svcErr.HTTPCode).To(Equal(404))

		// Verify Version removed from database
		err := checkResourceCount(t.Context(), h, []string{createdVersion.ID}, 0)
		Expect(err).To(BeNil(), "Version should be hard-deleted")

		// Delete Channel - should also be hard-deleted (no children left)
		_, svcErr = svc.Delete(t.Context(), "Channel", createdChannel.ID)
		Expect(svcErr).To(BeNil())

		// Verify Channel is hard-deleted
		_, svcErr = svc.Get(t.Context(), "Channel", createdChannel.ID)
		Expect(svcErr).ToNot(BeNil())
		Expect(svcErr.HTTPCode).To(Equal(404))

		t.Logf("ℹ Without RequiredAdapters: both parent and child hard-deleted immediately")
	})
}
