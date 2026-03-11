package integration

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

// TestAdvisoryLocksConcurrently validates that advisory locks properly serialize
// concurrent access to shared resources. This test uses actual database operations
// to prove the lock prevents race conditions at the database level.
func TestAdvisoryLocksConcurrently(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	// Create a counter table and initialize to 0
	g2 := h.DBFactory.New(context.Background())
	err := g2.Exec("CREATE TABLE IF NOT EXISTS lock_test_counter (id INTEGER PRIMARY KEY, value INTEGER)").Error
	Expect(err).NotTo(HaveOccurred(), "Failed to create counter table")

	err = g2.Exec("INSERT INTO lock_test_counter (id, value) VALUES (1, 0)").Error
	Expect(err).NotTo(HaveOccurred(), "Failed to initialize counter")
	defer g2.Exec("DROP TABLE IF EXISTS lock_test_counter")

	total := 10
	var waiter sync.WaitGroup
	waiter.Add(total)

	// Simulate a race condition where multiple threads are trying to access and modify the counter.
	// The acquireLock func uses an advisory lock so the accesses should be properly serialized.
	for i := 0; i < total; i++ {
		go acquireLock(h, &waiter)
	}

	// Wait for all goroutines to complete
	waiter.Wait()

	// All goroutines should have incremented the counter by 1, resulting in 10
	var finalValue int
	err = g2.Raw("SELECT value FROM lock_test_counter WHERE id = 1").Scan(&finalValue).Error
	Expect(err).NotTo(HaveOccurred(), "Failed to read final counter value")
	Expect(finalValue).To(Equal(total), "Counter should equal total")
}

func acquireLock(h *test.Helper, waiter *sync.WaitGroup) {
	defer waiter.Done()

	ctx := context.Background()

	// Acquire advisory lock
	ctx, lockOwnerID, err := db.NewAdvisoryLockContext(ctx, h.DBFactory, "test-resource", db.Migrations)
	Expect(err).NotTo(HaveOccurred(), "Failed to acquire lock")
	defer db.Unlock(ctx, lockOwnerID)

	g2 := h.DBFactory.New(ctx)

	// Read current value from database
	var currentValue int
	err = g2.Raw("SELECT value FROM lock_test_counter WHERE id = 1").Scan(&currentValue).Error
	Expect(err).NotTo(HaveOccurred(), "Failed to read counter")

	// Some slow work to increase the likelihood of race conditions
	time.Sleep(20 * time.Millisecond)

	// Increment and save to database
	newValue := currentValue + 1
	err = g2.Exec("UPDATE lock_test_counter SET value = ? WHERE id = 1", newValue).Error
	Expect(err).NotTo(HaveOccurred(), "Failed to update counter")
}

// TestAdvisoryLocksWithTransactions validates that advisory locks work correctly
// when combined with database transactions in various orders. Uses actual database
// operations to prove serialization.
func TestAdvisoryLocksWithTransactions(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	// Create a counter table and initialize to 0
	g2 := h.DBFactory.New(context.Background())
	err := g2.Exec("CREATE TABLE IF NOT EXISTS lock_test_counter_tx (id INTEGER PRIMARY KEY, value INTEGER)").Error
	Expect(err).NotTo(HaveOccurred(), "Failed to create counter table")

	err = g2.Exec("INSERT INTO lock_test_counter_tx (id, value) VALUES (1, 0)").Error
	Expect(err).NotTo(HaveOccurred(), "Failed to initialize counter")
	defer g2.Exec("DROP TABLE IF EXISTS lock_test_counter_tx")

	total := 10
	var waiter sync.WaitGroup
	waiter.Add(total)

	for i := 0; i < total; i++ {
		go acquireLockWithTransaction(h, &waiter)
	}

	waiter.Wait()

	// All goroutines should have incremented the counter by 1, resulting in 10
	var finalValue int
	err = g2.Raw("SELECT value FROM lock_test_counter_tx WHERE id = 1").Scan(&finalValue).Error
	Expect(err).NotTo(HaveOccurred(), "Failed to read final counter value")
	Expect(finalValue).To(Equal(total), "Counter should equal total")
}

func acquireLockWithTransaction(h *test.Helper, waiter *sync.WaitGroup) {
	defer waiter.Done()

	ctx := context.Background()

	// Lock and Tx can be stored within the same context. They should be independent of each other.
	// It doesn't matter if a Tx coexists or not, nor does it matter if it occurs before or after the lock
	r := rand.Intn(3) // no Tx if r == 2
	txBeforeLock := r == 0
	txAfterLock := r == 1

	var dberr error

	// Randomly add Tx before lock to demonstrate it works
	if txBeforeLock {
		ctx, dberr = db.NewContext(ctx, h.DBFactory)
		Expect(dberr).NotTo(HaveOccurred(), "Failed to create transaction context")
		defer db.Resolve(ctx)
	}

	// Acquire advisory lock
	ctx, lockOwnerID, dberr := db.NewAdvisoryLockContext(ctx, h.DBFactory, "test-resource-tx", db.Migrations)
	Expect(dberr).NotTo(HaveOccurred(), "Failed to acquire lock")
	defer db.Unlock(ctx, lockOwnerID)

	// Randomly add Tx after lock to demonstrate it works
	if txAfterLock {
		ctx, dberr = db.NewContext(ctx, h.DBFactory)
		Expect(dberr).NotTo(HaveOccurred(), "Failed to create transaction context")
		defer db.Resolve(ctx)
	}

	g2 := h.DBFactory.New(ctx)

	// Read current value from database
	var currentValue int
	err := g2.Raw("SELECT value FROM lock_test_counter_tx WHERE id = 1").Scan(&currentValue).Error
	Expect(err).NotTo(HaveOccurred(), "Failed to read counter")

	// Some slow work
	time.Sleep(20 * time.Millisecond)

	// Increment and save to database
	newValue := currentValue + 1
	err = g2.Exec("UPDATE lock_test_counter_tx SET value = ? WHERE id = 1", newValue).Error
	Expect(err).NotTo(HaveOccurred(), "Failed to update counter")
}

// TestLocksAndExpectedWaits validates the behavior of advisory locks:
// - Nested locks with the same (id, lockType) should not create additional locks
// - Different (id, lockType) combinations should create separate locks
// - Unlocking should only affect the lock matching the owner ID
func TestLocksAndExpectedWaits(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	// Start lock
	ctx := context.Background()
	ctx, lockOwnerID, err := db.NewAdvisoryLockContext(ctx, h.DBFactory, "system", db.Migrations)
	Expect(err).NotTo(HaveOccurred(), "Failed to acquire lock")
	defer db.Unlock(ctx, lockOwnerID) // Ensure lock is released on test exit

	// It should have 1 lock
	g2 := h.DBFactory.New(ctx)
	var pgLocks []struct{ Granted bool }
	g2.Raw("select granted from pg_locks WHERE locktype = 'advisory' and granted = true").Scan(&pgLocks)
	Expect(len(pgLocks)).To(Equal(1), "Expected 1 lock")

	// Successive locking should have no effect (nested lock with same id/type)
	// Pretend this runs in a nested func
	ctx, lockOwnerID2, err := db.NewAdvisoryLockContext(ctx, h.DBFactory, "system", db.Migrations)
	Expect(err).NotTo(HaveOccurred(), "Failed to acquire nested lock")
	defer db.Unlock(ctx, lockOwnerID2) // Ensure lock is released on test exit

	// It should still have 1 lock
	pgLocks = nil
	g2.Raw("select granted from pg_locks WHERE locktype = 'advisory' and granted = true").Scan(&pgLocks)
	Expect(len(pgLocks)).To(Equal(1), "Expected 1 lock after nested acquire")

	// Unlock should have no effect either (unlocking nested lock)
	// Pretend this runs in the nested func
	db.Unlock(ctx, lockOwnerID2)
	// It should still have 1 lock
	pgLocks = nil
	g2.Raw("select granted from pg_locks WHERE locktype = 'advisory' and granted = true").Scan(&pgLocks)
	Expect(len(pgLocks)).To(Equal(1), "Expected 1 lock after nested unlock")

	// Lock on a different (id, lockType) should work
	// Pretend this runs in a nested func
	ctx, lockOwnerID3, err := db.NewAdvisoryLockContext(ctx, h.DBFactory, "diff_system", db.Migrations)
	Expect(err).NotTo(HaveOccurred(), "Failed to acquire different lock")
	defer db.Unlock(ctx, lockOwnerID3) // Ensure lock is released on test exit

	// It should have 2 locks
	pgLocks = nil
	g2.Raw("select granted from pg_locks WHERE locktype = 'advisory' and granted = true").Scan(&pgLocks)
	Expect(len(pgLocks)).To(Equal(2), "Expected 2 locks")

	// Pretend it releases the new lock in the nested func
	db.Unlock(ctx, lockOwnerID3)
	// It should have 1 lock
	pgLocks = nil
	g2.Raw("select granted from pg_locks WHERE locktype = 'advisory' and granted = true").Scan(&pgLocks)
	Expect(len(pgLocks)).To(Equal(1), "Expected 1 lock after releasing different lock")

	// Unlock the topmost lock
	// Pretend it returns back to the parent func
	db.Unlock(ctx, lockOwnerID)
	// The lock should be gone
	pgLocks = nil
	g2.Raw("select granted from pg_locks WHERE locktype = 'advisory' and granted = true").Scan(&pgLocks)
	Expect(len(pgLocks)).To(Equal(0), "Expected 0 locks after final unlock")
}

// TestConcurrentMigrations validates that the MigrateWithLock function
// properly serializes concurrent migration attempts, ensuring only one
// instance actually runs migrations at a time.
func TestConcurrentMigrations(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	// First, reset the database to a clean state
	err := h.ResetDB()
	Expect(err).NotTo(HaveOccurred(), "Failed to reset database")

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
			err := db.MigrateWithLock(ctx, h.DBFactory)

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
	Expect(errors).To(BeEmpty(), "Expected no errors during concurrent migrations")

	// All goroutines should complete successfully
	Expect(successCount).To(Equal(total), "All migrations should succeed")
}

// TestAdvisoryLockBlocking validates that a second goroutine trying to acquire
// the same lock will block until the first goroutine releases it.
func TestAdvisoryLockBlocking(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	ctx := context.Background()

	// First goroutine acquires the lock
	ctx1, lockOwnerID1, err := db.NewAdvisoryLockContext(ctx, h.DBFactory, "blocking-test", db.Migrations)
	Expect(err).NotTo(HaveOccurred(), "Failed to acquire first lock")
	defer db.Unlock(ctx1, lockOwnerID1) // Ensure lock is released on test exit

	// Track when the second goroutine acquires the lock
	acquired := make(chan bool, 1)
	released := make(chan bool, 1)
	defer close(released) // ensure goroutine exits even on timeout

	// Second goroutine tries to acquire the same lock
	go func() {
		ctx2, lockOwnerID2, err := db.NewAdvisoryLockContext(context.Background(), h.DBFactory, "blocking-test", db.Migrations)
		Expect(err).NotTo(HaveOccurred(), "Failed to acquire second lock")
		defer db.Unlock(ctx2, lockOwnerID2)

		acquired <- true
		<-released // Wait for signal to release
	}()

	// Wait for the second goroutine to be actively waiting on the lock
	// by polling pg_locks for a non-granted advisory lock.
	// This is more reliable than sleep, especially in slow CI environments.
	g2 := h.DBFactory.New(ctx)
	waitingForLock := false
	for i := 0; i < 50; i++ { // Poll for up to 5 seconds (50 * 100ms)
		var waitingLocks []struct{ Granted bool }
		err := g2.Raw("SELECT granted FROM pg_locks WHERE locktype = 'advisory' AND granted = false").Scan(&waitingLocks).Error
		Expect(err).NotTo(HaveOccurred(), "Failed to query pg_locks")

		if len(waitingLocks) > 0 {
			waitingForLock = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	Expect(waitingForLock).To(BeTrue(), "Second goroutine should be waiting for lock")

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

// TestAdvisoryLockContextCancellation verifies that context cancellation properly
// terminates a waiting advisory lock acquisition. The context is passed through
// connection.New(ctx) and affects the blocking pg_advisory_xact_lock SQL call.
func TestAdvisoryLockContextCancellation(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	ctx := context.Background()

	// First goroutine acquires the lock
	ctx1, lockOwnerID1, err := db.NewAdvisoryLockContext(ctx, h.DBFactory, "cancel-test", db.Migrations)
	Expect(err).NotTo(HaveOccurred(), "Failed to acquire first lock")
	defer db.Unlock(ctx1, lockOwnerID1)

	// Track when the second goroutine gets cancelled
	gotCancelError := make(chan bool, 1)

	// Create a cancellable context for the second goroutine
	ctx2, cancel := context.WithCancel(context.Background())

	// Second goroutine tries to acquire the same lock with cancellable context
	go func() {
		_, _, err := db.NewAdvisoryLockContext(ctx2, h.DBFactory, "cancel-test", db.Migrations)
		if err != nil {
			// Check if this is a cancellation-type error
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
				strings.Contains(err.Error(), "canceling statement due to user request") {
				// Expected: context cancellation causes proper cancellation error
				gotCancelError <- true
				return
			}
			// Unexpected error - fail the test
			t.Errorf("Unexpected error from lock acquisition: %v", err)
			return
		}
		t.Error("Second goroutine acquired lock despite context cancellation (unexpected)")
	}()

	// Wait for the second goroutine to be actively waiting on the lock
	g2 := h.DBFactory.New(ctx)
	waitingForLock := false
	for i := 0; i < 50; i++ {
		var waitingLocks []struct{ Granted bool }
		err := g2.Raw("SELECT granted FROM pg_locks WHERE locktype = 'advisory' AND granted = false").Scan(&waitingLocks).Error
		Expect(err).NotTo(HaveOccurred(), "Failed to query pg_locks")

		if len(waitingLocks) > 0 {
			waitingForLock = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	Expect(waitingForLock).To(BeTrue(), "Second goroutine should be waiting for lock")

	// Cancel the context while the second goroutine is waiting
	cancel()

	// The second goroutine should exit with a cancellation error
	select {
	case <-gotCancelError:
		// Expected: context cancellation terminates the lock acquisition
	case <-time.After(2 * time.Second):
		t.Error("Second goroutine did not exit after context cancellation within timeout")
	}
}

// TestMigrationFailureUnderLock validates that when a migration fails while holding
// the advisory lock, the lock is properly released via defer, allowing other waiters
// to proceed. This tests the error path and cleanup behavior.
func TestMigrationFailureUnderLock(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	// Reset database to clean state
	err := h.ResetDB()
	Expect(err).NotTo(HaveOccurred(), "Failed to reset database")

	// Track results
	var mu sync.Mutex
	successCount := 0
	failureCount := 0
	var wg sync.WaitGroup

	// Create a failing migration function
	failingMigration := func(g2 *gorm.DB) error {
		return fmt.Errorf("simulated migration failure")
	}

	// First goroutine: acquire lock and fail migration
	wg.Add(1)
	go func() {
		defer wg.Done()

		ctx := context.Background()
		ctx, lockOwnerID, err := db.NewAdvisoryLockContext(ctx, h.DBFactory, "migration-fail-test", db.Migrations)
		Expect(err).NotTo(HaveOccurred(), "Failed to acquire lock")
		defer db.Unlock(ctx, lockOwnerID)

		// Simulate migration failure
		if err := failingMigration(h.DBFactory.New(ctx)); err != nil {
			mu.Lock()
			failureCount++
			mu.Unlock()
		}
		// Lock should be released via defer even though migration failed
	}()

	// Give first goroutine time to acquire lock and fail
	time.Sleep(100 * time.Millisecond)

	// Second goroutine: should be able to acquire lock after first fails
	wg.Add(1)
	go func() {
		defer wg.Done()

		ctx := context.Background()
		ctx, lockOwnerID, err := db.NewAdvisoryLockContext(ctx, h.DBFactory, "migration-fail-test", db.Migrations)
		Expect(err).NotTo(HaveOccurred(), "Failed to acquire lock after failure")
		defer db.Unlock(ctx, lockOwnerID)

		// This one succeeds
		mu.Lock()
		successCount++
		mu.Unlock()
	}()

	wg.Wait()

	// Verify both completed
	Expect(failureCount).To(Equal(1), "Expected 1 failure")
	Expect(successCount).To(Equal(1), "Expected 1 success")
}
