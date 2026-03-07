package integration

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

// TestAdvisoryLocksConcurrently validates that advisory locks properly serialize
// concurrent access to shared resources. This simulates a race condition where
// multiple threads try to access and modify the same variable.
func TestAdvisoryLocksConcurrently(t *testing.T) {
	helper := test.NewHelper(t)

	total := 10
	var waiter sync.WaitGroup
	waiter.Add(total)

	// Simulate a race condition where multiple threads are trying to access and modify the "total" var.
	// The acquireLock func uses an advisory lock so the accesses to "total" should be properly serialized.
	for i := 0; i < total; i++ {
		go acquireLock(helper, &total, &waiter)
	}

	// Wait for all goroutines to complete
	waiter.Wait()

	// All goroutines should have decremented total by 1, resulting in 0
	if total != 0 {
		t.Errorf("Expected total to be 0, got %d", total)
	}
}

func acquireLock(helper *test.Helper, total *int, waiter *sync.WaitGroup) {
	ctx := context.Background()

	// Acquire advisory lock
	ctx, lockOwnerID, err := db.NewAdvisoryLockContext(ctx, helper.DBFactory, "test-resource", db.Migrations)
	if err != nil {
		helper.T.Errorf("Failed to acquire lock: %v", err)
		waiter.Done()
		return
	}
	defer db.Unlock(ctx, lockOwnerID)

	// Pretend loading "total" from DB
	initTotal := *total

	// Some slow work to increase the likelihood of race conditions
	time.Sleep(20 * time.Millisecond)

	// Pretend saving "total" to DB
	finalTotal := initTotal - 1
	*total = finalTotal

	waiter.Done()
}

// TestAdvisoryLocksWithTransactions validates that advisory locks work correctly
// when combined with database transactions in various orders
func TestAdvisoryLocksWithTransactions(t *testing.T) {
	helper := test.NewHelper(t)

	total := 10
	var waiter sync.WaitGroup
	waiter.Add(total)

	for i := 0; i < total; i++ {
		go acquireLockWithTransaction(helper, &total, &waiter)
	}

	waiter.Wait()

	if total != 0 {
		t.Errorf("Expected total to be 0, got %d", total)
	}
}

func acquireLockWithTransaction(helper *test.Helper, total *int, waiter *sync.WaitGroup) {
	ctx := context.Background()

	// Lock and Tx can be stored within the same context. They should be independent of each other.
	// It doesn't matter if a Tx coexists or not, nor does it matter if it occurs before or after the lock
	r := rand.Intn(3) // no Tx if r == 2
	txBeforeLock := r == 0
	txAfterLock := r == 1

	var dberr error

	// Randomly add Tx before lock to demonstrate it works
	if txBeforeLock {
		ctx, dberr = db.NewContext(ctx, helper.DBFactory)
		if dberr != nil {
			helper.T.Errorf("Failed to create transaction context: %v", dberr)
			waiter.Done()
			return
		}
		defer db.Resolve(ctx)
	}

	// Acquire advisory lock
	ctx, lockOwnerID, dberr := db.NewAdvisoryLockContext(ctx, helper.DBFactory, "test-resource-tx", db.Migrations)
	if dberr != nil {
		helper.T.Errorf("Failed to acquire lock: %v", dberr)
		waiter.Done()
		return
	}
	defer db.Unlock(ctx, lockOwnerID)

	// Randomly add Tx after lock to demonstrate it works
	if txAfterLock {
		ctx, dberr = db.NewContext(ctx, helper.DBFactory)
		if dberr != nil {
			helper.T.Errorf("Failed to create transaction context: %v", dberr)
			waiter.Done()
			return
		}
		defer db.Resolve(ctx)
	}

	// Pretend loading "total" from DB
	initTotal := *total

	// Some slow work
	time.Sleep(20 * time.Millisecond)

	// Pretend saving "total" to DB
	finalTotal := initTotal - 1
	*total = finalTotal

	waiter.Done()
}

// TestLocksAndExpectedWaits validates the behavior of advisory locks:
// - Nested locks with the same (id, lockType) should not create additional locks
// - Different (id, lockType) combinations should create separate locks
// - Unlocking should only affect the lock matching the owner ID
func TestLocksAndExpectedWaits(t *testing.T) {
	helper := test.NewHelper(t)

	// Start lock
	ctx := context.Background()
	ctx, lockOwnerID, err := db.NewAdvisoryLockContext(ctx, helper.DBFactory, "system", db.Migrations)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// It should have 1 lock
	g2 := helper.DBFactory.New(ctx)
	var pgLocks []struct{ Granted bool }
	g2.Raw("select granted from pg_locks WHERE locktype = 'advisory' and granted = true").Scan(&pgLocks)
	if len(pgLocks) != 1 {
		t.Errorf("Expected 1 lock, got %d", len(pgLocks))
	}

	// Successive locking should have no effect (nested lock with same id/type)
	// Pretend this runs in a nested func
	ctx, lockOwnerID2, err := db.NewAdvisoryLockContext(ctx, helper.DBFactory, "system", db.Migrations)
	if err != nil {
		t.Fatalf("Failed to acquire nested lock: %v", err)
	}
	// It should still have 1 lock
	pgLocks = nil
	g2.Raw("select granted from pg_locks WHERE locktype = 'advisory' and granted = true").Scan(&pgLocks)
	if len(pgLocks) != 1 {
		t.Errorf("Expected 1 lock after nested acquire, got %d", len(pgLocks))
	}

	// Unlock should have no effect either (unlocking nested lock)
	// Pretend this runs in the nested func
	db.Unlock(ctx, lockOwnerID2)
	// It should still have 1 lock
	pgLocks = nil
	g2.Raw("select granted from pg_locks WHERE locktype = 'advisory' and granted = true").Scan(&pgLocks)
	if len(pgLocks) != 1 {
		t.Errorf("Expected 1 lock after nested unlock, got %d", len(pgLocks))
	}

	// Lock on a different (id, lockType) should work
	// Pretend this runs in a nested func
	ctx, lockOwnerID3, err := db.NewAdvisoryLockContext(ctx, helper.DBFactory, "diff_system", db.Migrations)
	if err != nil {
		t.Fatalf("Failed to acquire different lock: %v", err)
	}
	// It should have 2 locks
	pgLocks = nil
	g2.Raw("select granted from pg_locks WHERE locktype = 'advisory' and granted = true").Scan(&pgLocks)
	if len(pgLocks) != 2 {
		t.Errorf("Expected 2 locks, got %d", len(pgLocks))
	}

	// Pretend it releases the new lock in the nested func
	db.Unlock(ctx, lockOwnerID3)
	// It should have 1 lock
	pgLocks = nil
	g2.Raw("select granted from pg_locks WHERE locktype = 'advisory' and granted = true").Scan(&pgLocks)
	if len(pgLocks) != 1 {
		t.Errorf("Expected 1 lock after releasing different lock, got %d", len(pgLocks))
	}

	// Unlock the topmost lock
	// Pretend it returns back to the parent func
	db.Unlock(ctx, lockOwnerID)
	// The lock should be gone
	pgLocks = nil
	g2.Raw("select granted from pg_locks WHERE locktype = 'advisory' and granted = true").Scan(&pgLocks)
	if len(pgLocks) != 0 {
		t.Errorf("Expected 0 locks after final unlock, got %d", len(pgLocks))
	}
}

// TestConcurrentMigrations validates that the MigrateWithLock function
// properly serializes concurrent migration attempts, ensuring only one
// instance actually runs migrations at a time.
func TestConcurrentMigrations(t *testing.T) {
	helper := test.NewHelper(t)

	// First, reset the database to a clean state
	if err := helper.ResetDB(); err != nil {
		t.Fatalf("Failed to reset database: %v", err)
	}

	total := 5
	var waiter sync.WaitGroup
	waiter.Add(total)

	// Track which goroutines successfully acquired the lock
	var successCount int
	var mu sync.Mutex
	errors := make([]error, 0)

	// Simulate multiple pods trying to run migrations concurrently
	for i := 0; i < total; i++ {
		go func(id int) {
			defer waiter.Done()

			ctx := context.Background()
			err := db.MigrateWithLock(ctx, helper.DBFactory)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				errors = append(errors, err)
			} else {
				successCount++
			}
		}(i)
	}

	waiter.Wait()

	// All migrations should succeed (they're idempotent)
	if len(errors) > 0 {
		t.Errorf("Expected no errors, but got %d: %v", len(errors), errors)
	}

	// All goroutines should complete successfully
	if successCount != total {
		t.Errorf("Expected %d successful migrations, got %d", total, successCount)
	}
}

// TestAdvisoryLockBlocking validates that a second goroutine trying to acquire
// the same lock will block until the first goroutine releases it.
func TestAdvisoryLockBlocking(t *testing.T) {
	helper := test.NewHelper(t)

	ctx := context.Background()

	// First goroutine acquires the lock
	ctx1, lockOwnerID1, err := db.NewAdvisoryLockContext(ctx, helper.DBFactory, "blocking-test", db.Migrations)
	if err != nil {
		t.Fatalf("Failed to acquire first lock: %v", err)
	}

	// Track when the second goroutine acquires the lock
	acquired := make(chan bool, 1)
	released := make(chan bool, 1)

	// Second goroutine tries to acquire the same lock
	go func() {
		ctx2, lockOwnerID2, err := db.NewAdvisoryLockContext(context.Background(), helper.DBFactory, "blocking-test", db.Migrations)
		if err != nil {
			t.Errorf("Failed to acquire second lock: %v", err)
			return
		}
		defer db.Unlock(ctx2, lockOwnerID2)

		acquired <- true
		<-released // Wait for signal to release
	}()

	// Give the second goroutine time to start waiting
	time.Sleep(100 * time.Millisecond)

	// The second goroutine should still be blocked
	select {
	case <-acquired:
		t.Error("Second goroutine acquired lock while first still holds it")
	default:
		// Expected: second goroutine is still blocked
	}

	// Release the first lock
	db.Unlock(ctx1, lockOwnerID1)

	// Now the second goroutine should acquire the lock
	select {
	case <-acquired:
		// Expected: second goroutine acquired the lock
		released <- true
	case <-time.After(5 * time.Second):
		t.Error("Second goroutine did not acquire lock after first was released")
	}
}
