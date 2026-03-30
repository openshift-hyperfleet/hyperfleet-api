package integration

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

// TestTransactionRollbackWithDAO verifies that when TransactionMiddleware creates
// a transaction and a DAO operation calls MarkForRollback, the data is actually
// rolled back. This test validates whether GORM operations truly participate in
// the TransactionMiddleware's sql.Tx.
//
// CRITICAL TEST: This proves/disproves whether current architecture has working transactions
//
// Expected behavior (if transactions work):
//   - Create transaction via db.NewContext()
//   - DAO inserts a cluster
//   - MarkForRollback is called
//   - Resolve() rollback the transaction
//   - Query database: cluster should NOT exist (rollback succeeded)
//
// Actual behavior (if transactions DON'T work):
//   - Create transaction via db.NewContext()
//   - DAO inserts a cluster (uses connection pool, not the sql.Tx)
//   - Data commits immediately (autocommit mode)
//   - MarkForRollback sets a flag
//   - Resolve() rollback the sql.Tx (but GORM operations already committed)
//   - Query database: cluster STILL EXISTS (rollback failed)
func TestTransactionRollbackWithDAO(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	// Step 1: Create a transaction context (simulating TransactionMiddleware)
	ctx := context.Background()
	ctx, err := db.NewContext(ctx, h.DBFactory)
	Expect(err).NotTo(HaveOccurred(), "Failed to create transaction context")

	// Step 2: Create a cluster using DAO
	// This simulates what happens in a real POST request handler
	cluster := &api.Cluster{
		Meta: api.Meta{
			ID: h.NewID(),
		},
		Name:      "transaction-test-cluster",
		CreatedBy: "test-user",
		UpdatedBy: "test-user",
		Spec:      []byte(`{"test": "rollback"}`),
	}

	// Use sessionFactory.New(ctx) - this is what DAOs do
	g2 := h.DBFactory.New(ctx)
	err = g2.Create(cluster).Error
	Expect(err).NotTo(HaveOccurred(), "Failed to create cluster via GORM")

	// Step 3: Verify cluster was inserted (in-transaction visibility)
	var inTransactionCluster api.Cluster
	err = g2.Where("id = ?", cluster.ID).First(&inTransactionCluster).Error
	Expect(err).NotTo(HaveOccurred(), "Should see cluster within transaction")

	// Step 4: Mark transaction for rollback (simulating DAO error handling)
	db.MarkForRollback(ctx, err)

	// Step 5: Resolve the transaction (this should rollback)
	db.Resolve(ctx)

	// Step 6: CRITICAL CHECK - Query database with NEW session (outside transaction)
	// If transactions work: cluster should NOT exist
	// If transactions DON'T work: cluster WILL exist
	freshCtx := context.Background()
	freshSession := h.DBFactory.New(freshCtx)

	var afterRollbackCluster api.Cluster
	err = freshSession.Where("id = ?", cluster.ID).First(&afterRollbackCluster).Error

	// THE MOMENT OF TRUTH
	if err == nil {
		// Cluster still exists after rollback
		t.Errorf("🔴 CRITICAL FAILURE: Cluster exists after Resolve() rollback")
		t.Errorf("   Cluster ID: %s", afterRollbackCluster.ID)
		t.Errorf("   This proves GORM operations do NOT use TransactionMiddleware's sql.Tx")
		t.Errorf("   Architecture assumption is WRONG")
		t.FailNow()
	} else {
		// Cluster does not exist - rollback worked
		t.Logf("✅ SUCCESS: Cluster was rolled back")
		t.Logf("   GORM operations DO use TransactionMiddleware's sql.Tx")
		t.Logf("   Architecture assumption is CORRECT")
	}
}

// TestTransactionCommitWithDAO verifies the positive case: when no error occurs,
// the transaction commits successfully and data persists.
func TestTransactionCommitWithDAO(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	// Step 1: Create a transaction context
	ctx := context.Background()
	ctx, err := db.NewContext(ctx, h.DBFactory)
	Expect(err).NotTo(HaveOccurred(), "Failed to create transaction context")

	// Step 2: Create a cluster using DAO
	cluster := &api.Cluster{
		Meta: api.Meta{
			ID: h.NewID(),
		},
		Name:      "transaction-commit-test",
		CreatedBy: "test-user",
		UpdatedBy: "test-user",
		Spec:      []byte(`{"test": "commit"}`),
	}

	g2 := h.DBFactory.New(ctx)
	err = g2.Create(cluster).Error
	Expect(err).NotTo(HaveOccurred(), "Failed to create cluster via GORM")

	// Step 3: Do NOT call MarkForRollback - let it commit normally
	// Step 4: Resolve the transaction (this should commit)
	db.Resolve(ctx)

	// Step 5: Query database with fresh session
	freshCtx := context.Background()
	freshSession := h.DBFactory.New(freshCtx)

	var committedCluster api.Cluster
	err = freshSession.Where("id = ?", cluster.ID).First(&committedCluster).Error

	// Should find the cluster
	Expect(err).NotTo(HaveOccurred(), "Cluster should exist after commit")
	Expect(committedCluster.ID).To(Equal(cluster.ID))
	Expect(committedCluster.Name).To(Equal(cluster.Name))
	t.Logf("✅ Cluster successfully committed: %s", committedCluster.ID)
}

// TestMultipleDAOOperationsInTransaction verifies atomicity across multiple
// DAO operations. If transactions work, both operations should rollback together.
func TestMultipleDAOOperationsInTransaction(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	ctx := context.Background()
	ctx, err := db.NewContext(ctx, h.DBFactory)
	Expect(err).NotTo(HaveOccurred(), "Failed to create transaction context")

	// Create two clusters in the same transaction
	cluster1 := &api.Cluster{
		Meta:      api.Meta{ID: h.NewID()},
		Name:      "multi-op-cluster-1",
		CreatedBy: "test-user",
		UpdatedBy: "test-user",
		Spec:      []byte(`{"op": 1}`),
	}

	cluster2 := &api.Cluster{
		Meta:      api.Meta{ID: h.NewID()},
		Name:      "multi-op-cluster-2",
		CreatedBy: "test-user",
		UpdatedBy: "test-user",
		Spec:      []byte(`{"op": 2}`),
	}

	g2 := h.DBFactory.New(ctx)

	// First operation succeeds
	err = g2.Create(cluster1).Error
	Expect(err).NotTo(HaveOccurred(), "Failed to create first cluster")

	// Second operation succeeds
	err = g2.Create(cluster2).Error
	Expect(err).NotTo(HaveOccurred(), "Failed to create second cluster")

	// Mark for rollback (simulating error in third operation)
	db.MarkForRollback(ctx, err)

	// Resolve (should rollback both operations)
	db.Resolve(ctx)

	// Check if BOTH clusters were rolled back
	freshCtx := context.Background()
	freshSession := h.DBFactory.New(freshCtx)

	var check1 api.Cluster
	err1 := freshSession.Where("id = ?", cluster1.ID).First(&check1).Error

	var check2 api.Cluster
	err2 := freshSession.Where("id = ?", cluster2.ID).First(&check2).Error

	// Both should NOT exist if transactions work
	if err1 == nil || err2 == nil {
		t.Errorf("🔴 ATOMICITY FAILURE:")
		if err1 == nil {
			t.Errorf("   Cluster 1 still exists: %s", cluster1.ID)
		}
		if err2 == nil {
			t.Errorf("   Cluster 2 still exists: %s", cluster2.ID)
		}
		t.Errorf("   Multiple DAO operations are NOT atomic")
		t.FailNow()
	} else {
		t.Logf("✅ ATOMICITY SUCCESS: Both clusters rolled back together")
	}
}

