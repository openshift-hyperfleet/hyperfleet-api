package integration

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
)

func TestResourceConditions_UpdateAndPreload(t *testing.T) {
	RegisterTestingT(t)

	svc, h := setupResourceTest(t)
	ctx := context.Background()

	channel, svcErr := svc.Create(ctx, "Channel", newChannelResource("cond-test-update"))
	Expect(svcErr).To(BeNil())

	condDao := dao.NewResourceConditionDao(h.DBFactory)
	resourceDao := dao.NewResourceDao(h.DBFactory)

	now := time.Now().UTC().Truncate(time.Microsecond)
	conditions := []api.ResourceCondition{
		{
			Type:               "Available",
			Status:             api.ConditionTrue,
			ObservedGeneration: 1,
			CreatedTime:        now,
			LastUpdatedTime:    now,
			LastTransitionTime: now,
		},
		{
			Type:               "Reconciled",
			Status:             api.ConditionFalse,
			ObservedGeneration: 1,
			CreatedTime:        now,
			LastUpdatedTime:    now,
			LastTransitionTime: now,
		},
	}

	err := condDao.UpdateConditions(ctx, channel.ID, conditions)
	Expect(err).ToNot(HaveOccurred())

	fetched, err := resourceDao.Get(ctx, "Channel", channel.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(fetched.Conditions).To(HaveLen(2))

	condByType := make(map[string]api.ResourceCondition)
	for _, c := range fetched.Conditions {
		condByType[c.Type] = c
	}

	Expect(condByType["Available"].Status).To(Equal(api.ConditionTrue))
	Expect(condByType["Reconciled"].Status).To(Equal(api.ConditionFalse))
}

func TestResourceConditions_LastTransitionTimePreserved(t *testing.T) {
	RegisterTestingT(t)

	svc, h := setupResourceTest(t)
	ctx := context.Background()

	channel, svcErr := svc.Create(ctx, "Channel", newChannelResource("cond-test-transition"))
	Expect(svcErr).To(BeNil())

	condDao := dao.NewResourceConditionDao(h.DBFactory)
	resourceDao := dao.NewResourceDao(h.DBFactory)

	originalTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	conditions := []api.ResourceCondition{
		{
			Type:               "Available",
			Status:             api.ConditionTrue,
			ObservedGeneration: 1,
			CreatedTime:        originalTime,
			LastUpdatedTime:    originalTime,
			LastTransitionTime: originalTime,
		},
	}

	err := condDao.UpdateConditions(ctx, channel.ID, conditions)
	Expect(err).ToNot(HaveOccurred())

	// Same status — LastTransitionTime must be preserved
	laterTime := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	conditions2 := []api.ResourceCondition{
		{
			Type:               "Available",
			Status:             api.ConditionTrue,
			ObservedGeneration: 2,
			CreatedTime:        laterTime,
			LastUpdatedTime:    laterTime,
			LastTransitionTime: laterTime,
		},
	}

	err = condDao.UpdateConditions(ctx, channel.ID, conditions2)
	Expect(err).ToNot(HaveOccurred())

	fetched, err := resourceDao.Get(ctx, "Channel", channel.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(fetched.Conditions).To(HaveLen(1))
	Expect(fetched.Conditions[0].LastTransitionTime).To(
		BeTemporally("==", originalTime),
		"LastTransitionTime should be preserved when status unchanged",
	)

	// Status changes — LastTransitionTime must update
	transitionTime := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	conditions3 := []api.ResourceCondition{
		{
			Type:               "Available",
			Status:             api.ConditionFalse,
			ObservedGeneration: 3,
			CreatedTime:        transitionTime,
			LastUpdatedTime:    transitionTime,
			LastTransitionTime: transitionTime,
		},
	}

	err = condDao.UpdateConditions(ctx, channel.ID, conditions3)
	Expect(err).ToNot(HaveOccurred())

	fetched, err = resourceDao.Get(ctx, "Channel", channel.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(fetched.Conditions[0].LastTransitionTime).To(
		BeTemporally("==", transitionTime),
		"LastTransitionTime should update when status changes",
	)
}

func TestResourceConditions_AtomicReplace(t *testing.T) {
	RegisterTestingT(t)

	svc, h := setupResourceTest(t)
	ctx := context.Background()

	channel, svcErr := svc.Create(ctx, "Channel", newChannelResource("cond-test-replace"))
	Expect(svcErr).To(BeNil())

	condDao := dao.NewResourceConditionDao(h.DBFactory)
	resourceDao := dao.NewResourceDao(h.DBFactory)

	now := time.Now().UTC().Truncate(time.Microsecond)

	// Insert two conditions
	err := condDao.UpdateConditions(ctx, channel.ID, []api.ResourceCondition{
		{Type: "Available", Status: api.ConditionTrue, ObservedGeneration: 1,
			CreatedTime: now, LastUpdatedTime: now, LastTransitionTime: now},
		{Type: "Reconciled", Status: api.ConditionFalse, ObservedGeneration: 1,
			CreatedTime: now, LastUpdatedTime: now, LastTransitionTime: now},
	})
	Expect(err).ToNot(HaveOccurred())

	// Replace with one condition — old one must be gone
	err = condDao.UpdateConditions(ctx, channel.ID, []api.ResourceCondition{
		{Type: "Available", Status: api.ConditionTrue, ObservedGeneration: 2,
			CreatedTime: now, LastUpdatedTime: now, LastTransitionTime: now},
	})
	Expect(err).ToNot(HaveOccurred())

	fetched, err := resourceDao.Get(ctx, "Channel", channel.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(fetched.Conditions).To(HaveLen(1))
	Expect(fetched.Conditions[0].Type).To(Equal("Available"))
}

func TestResourceConditions_SaveDoesNotTouchConditions(t *testing.T) {
	RegisterTestingT(t)

	svc, h := setupResourceTest(t)
	ctx := context.Background()

	channel, svcErr := svc.Create(ctx, "Channel", newChannelResource("cond-test-save"))
	Expect(svcErr).To(BeNil())

	condDao := dao.NewResourceConditionDao(h.DBFactory)
	resourceDao := dao.NewResourceDao(h.DBFactory)

	now := time.Now().UTC().Truncate(time.Microsecond)
	err := condDao.UpdateConditions(ctx, channel.ID, []api.ResourceCondition{
		{Type: "Available", Status: api.ConditionTrue, ObservedGeneration: 1,
			CreatedTime: now, LastUpdatedTime: now, LastTransitionTime: now},
	})
	Expect(err).ToNot(HaveOccurred())

	// Modify spec via Save — conditions must remain
	channel.Spec = []byte(`{"is_default": true, "enabled_regex": ".*"}`)
	err = resourceDao.Save(ctx, channel)
	Expect(err).ToNot(HaveOccurred())

	fetched, err := resourceDao.Get(ctx, "Channel", channel.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(fetched.Conditions).To(HaveLen(1))
	Expect(fetched.Conditions[0].Type).To(Equal("Available"))
}

func TestResourceConditions_CreatedTimePreserved(t *testing.T) {
	RegisterTestingT(t)

	svc, h := setupResourceTest(t)
	ctx := context.Background()

	channel, svcErr := svc.Create(ctx, "Channel", newChannelResource("cond-test-created"))
	Expect(svcErr).To(BeNil())

	condDao := dao.NewResourceConditionDao(h.DBFactory)
	resourceDao := dao.NewResourceDao(h.DBFactory)

	originalCreated := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	err := condDao.UpdateConditions(ctx, channel.ID, []api.ResourceCondition{
		{Type: "Available", Status: api.ConditionTrue, ObservedGeneration: 1,
			CreatedTime: originalCreated, LastUpdatedTime: originalCreated, LastTransitionTime: originalCreated},
	})
	Expect(err).ToNot(HaveOccurred())

	// Second update — CreatedTime must be preserved from first call
	laterTime := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	err = condDao.UpdateConditions(ctx, channel.ID, []api.ResourceCondition{
		{Type: "Available", Status: api.ConditionFalse, ObservedGeneration: 2,
			CreatedTime: laterTime, LastUpdatedTime: laterTime, LastTransitionTime: laterTime},
	})
	Expect(err).ToNot(HaveOccurred())

	fetched, err := resourceDao.Get(ctx, "Channel", channel.ID)
	Expect(err).ToNot(HaveOccurred())
	Expect(fetched.Conditions[0].CreatedTime).To(
		BeTemporally("==", originalCreated),
		"CreatedTime should be preserved from the first UpdateConditions call",
	)
}
