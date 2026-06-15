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

// Test helpers

func createChannel(t *testing.T, svc services.ResourceService, name string) *api.Resource {
	t.Helper()
	channel := newChannelResource(name)
	created, err := svc.Create(t.Context(), "Channel", channel)
	if err != nil {
		t.Fatalf("Failed to create channel: %v", err)
	}
	return created
}

// Channel Create
func TestChannelCreate(t *testing.T) {
	t.Run("UniqueConstraint", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channelName := fmt.Sprintf("unique-channel-%s", uuid.NewString()[:8])
		channel1 := createChannel(t, svc, channelName)
		Expect(channel1.ID).NotTo(BeEmpty())

		// Attempt to create duplicate - should fail
		duplicate := newChannelResource(channelName)
		_, svcErr := svc.Create(t.Context(), "Channel", duplicate)
		Expect(svcErr).ToNot(BeNil(), "duplicate channel name should fail")
		Expect(svcErr.HTTPCode).To(Equal(409))
	})

	t.Run("EmptyName", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channel := newChannelResource("")
		_, svcErr := svc.Create(t.Context(), "Channel", channel)
		Expect(svcErr).ToNot(BeNil(), "empty channel name should fail")
		Expect(svcErr.HTTPCode).To(Equal(400))
	})

	t.Run("WithLabels", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channelName := fmt.Sprintf("channel-with-labels-%s", uuid.NewString()[:8])
		channel := newChannelResource(channelName)
		labels := map[string]string{
			"environment": "test",
			"team":        "platform",
		}

		var err error
		channel.Labels, err = json.Marshal(labels)
		Expect(err).To(BeNil(), "should marshal labels")
		createdChannel, svcErr := svc.Create(t.Context(), "Channel", channel)
		Expect(svcErr).To(BeNil())
		Expect(createdChannel.Labels).NotTo(BeNil())

		// Get the resource and verify labels persisted
		retrieved, getErr := svc.Get(t.Context(), "Channel", createdChannel.ID)
		Expect(getErr).To(BeNil(), "should retrieve channel")
		var retrievedLabels map[string]string
		err = json.Unmarshal(retrieved.Labels, &retrievedLabels)
		Expect(err).To(BeNil(), "should unmarshal retrieved labels")
		Expect(retrievedLabels).To(Equal(labels), "retrieved labels should match")
	})

	t.Run("SetsTimestamps", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channelName := fmt.Sprintf("timestamp-test-%s", uuid.NewString()[:8])
		before := time.Now()
		channel := createChannel(t, svc, channelName)
		after := time.Now()

		Expect(channel.CreatedTime).To(BeTemporally(">=", before))
		Expect(channel.CreatedTime).To(BeTemporally("<=", after))
		Expect(channel.UpdatedTime).To(BeTemporally(">=", before))
		Expect(channel.UpdatedTime).To(BeTemporally("<=", after))
	})
}

// Channel Delete
func TestChannelDelete(t *testing.T) {
	t.Run("NotFound", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		_, svcErr := svc.Delete(t.Context(), "Channel", "nonexistent-id")
		Expect(svcErr).ToNot(BeNil(), "delete of nonexistent channel should fail")
		Expect(svcErr.HTTPCode).To(Equal(404))
	})

	t.Run("SetsDeletedTime", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channelName := fmt.Sprintf("delete-timestamp-%s", uuid.NewString()[:8])
		channel := createChannel(t, svc, channelName)

		before := time.Now()
		deleted, svcErr := svc.Delete(t.Context(), "Channel", channel.ID)
		after := time.Now()

		Expect(svcErr).To(BeNil())
		Expect(deleted.DeletedTime).NotTo(BeNil())
		Expect(*deleted.DeletedTime).To(BeTemporally(">=", before))
		Expect(*deleted.DeletedTime).To(BeTemporally("<=", after))
	})

	t.Run("ReDeleteReturns404", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channelName := fmt.Sprintf("redelete-test-%s", uuid.NewString()[:8])
		channel := createChannel(t, svc, channelName)

		// Delete once - should succeed
		_, svcErr := svc.Delete(t.Context(), "Channel", channel.ID)
		Expect(svcErr).To(BeNil())

		// Delete again - should return 404
		_, svcErr = svc.Delete(t.Context(), "Channel", channel.ID)
		Expect(svcErr).ToNot(BeNil())
		Expect(svcErr.HTTPCode).To(Equal(404))
	})

	t.Run("HardDeleteRemovesRow", func(t *testing.T) {
		svc, h := setupResourceTest(t)

		channelName := fmt.Sprintf("hard-delete-%s", uuid.NewString()[:8])
		channel := createChannel(t, svc, channelName)

		// Delete the channel
		result, svcErr := svc.Delete(t.Context(), "Channel", channel.ID)
		Expect(svcErr).To(BeNil())
		Expect(result.DeletedTime).ToNot(BeNil())

		// Verify hard delete removed the row
		dbErr := checkResourceCount(t.Context(), h, []string{channel.ID}, 0)
		Expect(dbErr).To(BeNil())
	})
}

// Channel List
func TestChannelList(t *testing.T) {
	t.Run("Basic", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		// Create test channels with unique names
		channels := make([]*api.Resource, 3)
		for i := range 3 {
			name := fmt.Sprintf("list-test-%s-%d", uuid.NewString()[:8], i)
			channels[i] = createChannel(t, svc, name)
		}

		// List channels
		args := &services.ListArguments{
			Page: 1,
			Size: 100,
		}
		list, _, svcErr := svc.List(t.Context(), "Channel", args)
		Expect(svcErr).To(BeNil(), "list should succeed")
		Expect(len(list)).To(BeNumerically(">=", 3), "should have at least 3 channels")

		// Verify our channels are in the list
		foundIDs := make(map[string]bool)
		for _, item := range list {
			foundIDs[item.ID] = true
		}

		for _, ch := range channels {
			Expect(foundIDs[ch.ID]).To(BeTrue(), "should find created channel")
		}
	})

	t.Run("Pagination", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		// Create 15 channels with unique prefix
		prefix := uuid.NewString()[:8]
		for i := range 15 {
			name := fmt.Sprintf("pagination-%s-%d", prefix, i)
			createChannel(t, svc, name)
		}

		// Page 1
		args := &services.ListArguments{
			Page: 1,
			Size: 10,
		}
		list, _, svcErr := svc.List(t.Context(), "Channel", args)
		Expect(svcErr).To(BeNil(), "list should succeed")
		Expect(len(list)).To(BeNumerically(">=", 10), "page 1 should have at least 10 items")

		// Page 2
		args.Page = 2
		list, _, svcErr = svc.List(t.Context(), "Channel", args)
		Expect(svcErr).To(BeNil(), "list should succeed")
		Expect(len(list)).To(BeNumerically(">=", 5), "page 2 should have at least 5 items")
	})

	t.Run("WithOrdering", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		// Create channels with specific names for ordering
		prefix := uuid.NewString()[:8]
		ch1 := createChannel(t, svc, fmt.Sprintf("order-c-%s", prefix))
		ch2 := createChannel(t, svc, fmt.Sprintf("order-a-%s", prefix))
		ch3 := createChannel(t, svc, fmt.Sprintf("order-b-%s", prefix))

		// Test ordering by name ascending
		args := &services.ListArguments{
			Page:    1,
			Size:    100,
			OrderBy: []string{"name asc"},
		}
		list, _, svcErr := svc.List(t.Context(), "Channel", args)
		Expect(svcErr).To(BeNil(), "list should succeed")

		// Find our test channels in the list
		channelPositions := make(map[string]int)
		for i, item := range list {
			if item.ID == ch1.ID || item.ID == ch2.ID || item.ID == ch3.ID {
				channelPositions[item.ID] = i
			}
		}

		Expect(len(channelPositions)).To(Equal(3), "should find all 3 test channels")

		// Verify order-a comes before order-b comes before order-c
		Expect(channelPositions[ch2.ID]).To(BeNumerically("<", channelPositions[ch3.ID]),
			"order-a should come before order-b")
		Expect(channelPositions[ch3.ID]).To(BeNumerically("<", channelPositions[ch1.ID]),
			"order-b should come before order-c")
	})
}

// Channel Get
func TestChannelGet(t *testing.T) {
	t.Run("NotFound", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		_, svcErr := svc.Get(t.Context(), "Channel", "nonexistent-id")
		Expect(svcErr).ToNot(BeNil(), "get of nonexistent channel should fail")
		Expect(svcErr.HTTPCode).To(Equal(404))
	})

	t.Run("Success", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channelName := fmt.Sprintf("get-test-%s", uuid.NewString()[:8])
		channel := createChannel(t, svc, channelName)

		// Get channel should succeed
		retrieved, svcErr := svc.Get(t.Context(), "Channel", channel.ID)
		Expect(svcErr).To(BeNil(), "get should succeed")
		Expect(retrieved.ID).To(Equal(channel.ID))
		Expect(retrieved.Name).To(Equal(channel.Name))
	})
}

// Channel Patch
func TestChannelPatch(t *testing.T) {
	t.Run("NotFound", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		req := &api.ResourcePatchRequest{
			Spec: &map[string]any{"enabled_regex": ".*"},
		}

		_, svcErr := svc.Patch(t.Context(), "Channel", "nonexistent-id", req)
		Expect(svcErr).ToNot(BeNil(), "patch of nonexistent channel should fail")
		Expect(svcErr.HTTPCode).To(Equal(404))
	})

	t.Run("SpecChanged", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channelName := fmt.Sprintf("patch-spec-%s", uuid.NewString()[:8])
		channel := createChannel(t, svc, channelName)
		Expect(channel.Generation).To(Equal(int32(1)), "initial generation should be 1")

		// Patch spec
		newSpec := map[string]any{
			"is_default":    true,
			"enabled_regex": "4\\.17\\..*",
		}
		req := &api.ResourcePatchRequest{
			Spec: &newSpec,
		}
		patched, svcErr := svc.Patch(t.Context(), "Channel", channel.ID, req)
		Expect(svcErr).To(BeNil(), "patch should succeed")
		Expect(patched.Generation).To(Equal(int32(2)), "generation should increment to 2")

		// Unmarshal and verify spec
		var patchedSpec map[string]any
		json.Unmarshal(patched.Spec, &patchedSpec)
		Expect(patchedSpec["is_default"]).To(Equal(true))
		Expect(patchedSpec["enabled_regex"]).To(Equal("4\\.17\\..*"))

		// Verify persisted
		retrieved, getErr := svc.Get(t.Context(), "Channel", channel.ID)
		Expect(getErr).To(BeNil())
		Expect(retrieved.Generation).To(Equal(int32(2)))

		var retrievedSpec map[string]any
		json.Unmarshal(retrieved.Spec, &retrievedSpec)
		Expect(retrievedSpec).To(Equal(patchedSpec))
	})

	t.Run("LabelsOnly", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channelName := fmt.Sprintf("patch-labels-%s", uuid.NewString()[:8])
		channel := createChannel(t, svc, channelName)
		Expect(channel.Generation).To(Equal(int32(1)), "initial generation should be 1")

		// Patch labels only
		newLabels := map[string]string{
			"patched": "true",
			"env":     "staging",
		}
		req := &api.ResourcePatchRequest{
			Labels: &newLabels,
		}
		patched, svcErr := svc.Patch(t.Context(), "Channel", channel.ID, req)
		Expect(svcErr).To(BeNil(), "patch should succeed")
		Expect(patched.Generation).To(Equal(int32(2)), "generation should increment to 2")

		// Verify labels
		var patchedLabels map[string]string
		json.Unmarshal(patched.Labels, &patchedLabels)
		Expect(patchedLabels).To(Equal(newLabels))
	})

	t.Run("UpdatesTimestamps", func(t *testing.T) {
		svc, _ := setupResourceTest(t)

		channelName := fmt.Sprintf("patch-timestamp-%s", uuid.NewString()[:8])
		channel := createChannel(t, svc, channelName)
		originalUpdatedTime := channel.UpdatedTime

		// Sleep briefly to ensure timestamp difference
		time.Sleep(5 * time.Millisecond)

		// Patch the channel
		newSpec := map[string]any{"is_default": true}
		req := &api.ResourcePatchRequest{
			Spec: &newSpec,
		}
		patched, svcErr := svc.Patch(t.Context(), "Channel", channel.ID, req)

		Expect(svcErr).To(BeNil())
		Expect(patched.UpdatedTime).To(BeTemporally(">", originalUpdatedTime),
			"updated_time should be later than original after patch")
	})
}
