package integration

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

type failingResourceLabelDao struct{ err error }

func (d failingResourceLabelDao) ReplaceLabels(
	context.Context, string, []api.ResourceLabel,
) error {
	return d.err
}

var _ dao.ResourceLabelDao = failingResourceLabelDao{}

func transactionTestCluster(h *test.Helper, name string) *api.Resource {
	return &api.Resource{
		Meta:      api.Meta{ID: h.NewID()},
		Kind:      "Cluster",
		Name:      name,
		CreatedBy: "test-user",
		UpdatedBy: "test-user",
		Spec:      []byte(`{"test": "transaction"}`),
	}
}

func TestTransactionRollbackWithDAO(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	resourceDao := h.Container.ResourceDao()
	cluster := transactionTestCluster(h, "transaction-rollback-"+h.NewID())
	wantErr := errors.New("force rollback")

	runner := db.NewTxRunner(h.DBFactory)
	err := runner.Do(context.Background(), func(ctx context.Context) error {
		_, err := resourceDao.Create(ctx, cluster)
		if err != nil {
			return err
		}
		return wantErr
	})
	Expect(err).To(MatchError(wantErr))

	var persisted api.Resource
	err = h.DBFactory.New(context.Background()).Where("id = ?", cluster.ID).First(&persisted).Error
	Expect(errors.Is(err, gorm.ErrRecordNotFound)).To(BeTrue())
}

func TestTransactionCommitWithDAO(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	resourceDao := h.Container.ResourceDao()
	cluster := transactionTestCluster(h, "transaction-commit-"+h.NewID())

	runner := db.NewTxRunner(h.DBFactory)
	err := runner.Do(context.Background(), func(ctx context.Context) error {
		_, err := resourceDao.Create(ctx, cluster)
		return err
	})
	Expect(err).NotTo(HaveOccurred())

	var persisted api.Resource
	err = h.DBFactory.New(context.Background()).Where("id = ?", cluster.ID).First(&persisted).Error
	Expect(err).NotTo(HaveOccurred())
	Expect(persisted.ID).To(Equal(cluster.ID))
}

func TestResourceServiceCreateRollsBackWhenLabelPersistenceFails(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	cluster := transactionTestCluster(h, "transaction-label-rollback-"+h.NewID())
	cluster.Labels = []api.ResourceLabel{{Key: "environment", Value: "test"}}
	labelErr := errors.New("label persistence failed")

	service, err := services.NewResourceService(
		h.Container.ResourceDao(),
		failingResourceLabelDao{err: labelErr},
		h.Container.AdapterStatusDao(),
		h.Container.ResourceConditionDao(),
		h.Container.GenericService(),
		h.Container.TxRunner(),
	)
	Expect(err).NotTo(HaveOccurred())

	created, svcErr := service.Create(context.Background(), "Cluster", cluster, nil)
	Expect(created).To(BeNil())
	Expect(svcErr).NotTo(BeNil())
	Expect(svcErr.Reason).To(ContainSubstring(labelErr.Error()))

	var persisted api.Resource
	err = h.DBFactory.New(context.Background()).Where("id = ?", cluster.ID).First(&persisted).Error
	Expect(errors.Is(err, gorm.ErrRecordNotFound)).To(BeTrue())
}

func TestMultipleDAOOperationsRollbackTogether(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	resourceDao := h.Container.ResourceDao()
	cluster1 := transactionTestCluster(h, "transaction-multi-1-"+h.NewID())
	cluster2 := transactionTestCluster(h, "transaction-multi-2-"+h.NewID())

	runner := db.NewTxRunner(h.DBFactory)
	err := runner.Do(context.Background(), func(ctx context.Context) error {
		if _, err := resourceDao.Create(ctx, cluster1); err != nil {
			return err
		}
		if _, err := resourceDao.Create(ctx, cluster2); err != nil {
			return err
		}
		return errors.New("rollback both writes")
	})
	Expect(err).To(HaveOccurred())

	fresh := h.DBFactory.New(context.Background())
	var persisted api.Resource
	Expect(errors.Is(fresh.Where("id = ?", cluster1.ID).First(&persisted).Error, gorm.ErrRecordNotFound)).To(BeTrue())
	Expect(errors.Is(fresh.Where("id = ?", cluster2.ID).First(&persisted).Error, gorm.ErrRecordNotFound)).To(BeTrue())
}
