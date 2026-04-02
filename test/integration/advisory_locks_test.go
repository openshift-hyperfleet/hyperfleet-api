package integration

import (
	"context"
	"errors"
	"fmt"
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

// TestRowLevelConcurrency validates the correct pattern for concurrent row updates
// using SELECT FOR UPDATE. This is the proper way to handle row-level concurrency
// in PostgreSQL, not advisory locks.
func TestRowLevelConcurrency(t *testing.T) {
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

	// Simulate concurrent updates using SELECT FOR UPDATE (correct pattern)
	for i := 0; i < total; i++ {
		go incrementCounterWithRowLock(h, &waiter)
	}

	waiter.Wait()

	// All goroutines should have incremented the counter by 1, resulting in 10
	var finalValue int
	err = g2.Raw("SELECT value FROM lock_test_counter_tx WHERE id = 1").Scan(&finalValue).Error
	Expect(err).NotTo(HaveOccurred(), "Failed to read final counter value")
	Expect(finalValue).To(Equal(total), "Counter should equal total (no lost updates)")
}

func incrementCounterWithRowLock(h *test.Helper, waiter *sync.WaitGroup) {
	defer waiter.Done()

	ctx := context.Background()
	g2 := h.DBFactory.New(ctx)

	// Begin transaction
	tx := g2.Begin()
	Expect(tx.Error).NotTo(HaveOccurred(), "Failed to begin transaction")
	defer tx.Rollback()

	// SELECT FOR UPDATE acquires row-level lock
	// Other transactions trying to SELECT FOR UPDATE on this row will BLOCK here
	var currentValue int
	err := tx.Raw("SELECT value FROM lock_test_counter_tx WHERE id = 1 FOR UPDATE").Scan(&currentValue).Error
	Expect(err).NotTo(HaveOccurred(), "Failed to read counter with row lock")

	// Some slow work to increase likelihood of contention
	time.Sleep(20 * time.Millisecond)

	// Increment and update
	newValue := currentValue + 1
	err = tx.Exec("UPDATE lock_test_counter_tx SET value = ? WHERE id = 1", newValue).Error
	Expect(err).NotTo(HaveOccurred(), "Failed to update counter")

	// Commit releases the row lock
	err = tx.Commit().Error
	Expect(err).NotTo(HaveOccurred(), "Failed to commit transaction")
}

// TestAdvisoryLockFailsWithExistingTransaction validates that advisory locks
// properly reject attempts to acquire a lock when a transaction already exists
// in the context. This prevents the race condition where the lock is released
// before the transaction commits.
func TestAdvisoryLockFailsWithExistingTransaction(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	ctx := context.Background()

	// Create transaction first
	ctx, err := db.NewContext(ctx, h.DBFactory)
	Expect(err).NotTo(HaveOccurred(), "Failed to create transaction context")
	defer db.Resolve(ctx)

	// Try to acquire advisory lock (should fail)
	_, lockID, err := db.NewAdvisoryLockContext(ctx, h.DBFactory, "test-resource", db.Migrations)

	// Should fail with clear error
	Expect(err).To(HaveOccurred(), "Advisory lock should fail when transaction exists")
	Expect(err.Error()).To(ContainSubstring("transaction"), "Error should mention transaction")
	Expect(err.Error()).To(ContainSubstring("SELECT FOR UPDATE"), "Error should suggest SELECT FOR UPDATE")
	Expect(lockID).To(BeEmpty(), "Lock ID should be empty on error")
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
	_ = g2.Raw("select granted from pg_locks WHERE locktype = 'advisory' and granted = true").Scan(&pgLocks).Error
	Expect(len(pgLocks)).To(Equal(1), "Expected 1 lock")

	// Successive locking should have no effect (nested lock with same id/type)
	// Pretend this runs in a nested func
	ctx, lockOwnerID2, err := db.NewAdvisoryLockContext(ctx, h.DBFactory, "system", db.Migrations)
	Expect(err).NotTo(HaveOccurred(), "Failed to acquire nested lock")
	defer db.Unlock(ctx, lockOwnerID2) // Ensure lock is released on test exit

	// It should still have 1 lock
	pgLocks = nil
	_ = g2.Raw("select granted from pg_locks WHERE locktype = 'advisory' and granted = true").Scan(&pgLocks).Error
	Expect(len(pgLocks)).To(Equal(1), "Expected 1 lock after nested acquire")

	// Unlock should have no effect either (unlocking nested lock)
	// Pretend this runs in the nested func
	db.Unlock(ctx, lockOwnerID2)
	// It should still have 1 lock
	pgLocks = nil
	_ = g2.Raw("select granted from pg_locks WHERE locktype = 'advisory' and granted = true").Scan(&pgLocks).Error
	Expect(len(pgLocks)).To(Equal(1), "Expected 1 lock after nested unlock")

	// Lock on a different (id, lockType) should work
	// Pretend this runs in a nested func
	ctx, lockOwnerID3, err := db.NewAdvisoryLockContext(ctx, h.DBFactory, "diff_system", db.Migrations)
	Expect(err).NotTo(HaveOccurred(), "Failed to acquire different lock")
	defer db.Unlock(ctx, lockOwnerID3) // Ensure lock is released on test exit

	// It should have 2 locks
	pgLocks = nil
	_ = g2.Raw("select granted from pg_locks WHERE locktype = 'advisory' and granted = true").Scan(&pgLocks).Error
	Expect(len(pgLocks)).To(Equal(2), "Expected 2 locks")

	// Pretend it releases the new lock in the nested func
	db.Unlock(ctx, lockOwnerID3)
	// It should have 1 lock
	pgLocks = nil
	_ = g2.Raw("select granted from pg_locks WHERE locktype = 'advisory' and granted = true").Scan(&pgLocks).Error
	Expect(len(pgLocks)).To(Equal(1), "Expected 1 lock after releasing different lock")

	// Unlock the topmost lock
	// Pretend it returns back to the parent func
	db.Unlock(ctx, lockOwnerID)
	// The lock should be gone
	pgLocks = nil
	_ = g2.Raw("select granted from pg_locks WHERE locktype = 'advisory' and granted = true").Scan(&pgLocks).Error
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
		go func() {
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
		}()
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
		ctx2, lockOwnerID2, err := db.NewAdvisoryLockContext(
			context.Background(), h.DBFactory, "blocking-test", db.Migrations)
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
		query := "SELECT granted FROM pg_locks WHERE locktype = 'advisory' AND granted = false"
		err := g2.Raw(query).Scan(&waitingLocks).Error
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

	// Track when the second goroutine gets canceled
	gotCancelError := make(chan bool, 1)

	// Create a cancelable context for the second goroutine
	ctx2, cancel := context.WithCancel(context.Background())

	// Use WaitGroup to ensure goroutine exits before test cleanup
	var wg sync.WaitGroup
	wg.Add(1)

	// Second goroutine tries to acquire the same lock with cancellable context
	go func() {
		defer wg.Done()
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
		query := "SELECT granted FROM pg_locks WHERE locktype = 'advisory' AND granted = false"
		err := g2.Raw(query).Scan(&waitingLocks).Error
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

	// Ensure goroutine exits before test cleanup
	wg.Wait()
}

// migrateWithLockAndCustomMigration mimics db.MigrateWithLock but accepts a custom migration function
// This allows testing the lock acquisition/release pattern with controlled success/failure
func migrateWithLockAndCustomMigration(
	ctx context.Context,
	factory db.SessionFactory,
	migrationFunc func(*gorm.DB) error,
) error {
	// Acquire advisory lock for migrations (same pattern as production MigrateWithLock)
	ctx, lockOwnerID, err := db.NewAdvisoryLockContext(ctx, factory, db.MigrationsLockID, db.Migrations)
	if err != nil {
		return err
	}
	defer db.Unlock(ctx, lockOwnerID)

	// Run custom migration with the locked context
	g2 := factory.New(ctx)
	if err := migrationFunc(g2); err != nil {
		return err
	}

	return nil
}

// TestMigrationFailureUnderLock validates that when a migration fails while holding
// the advisory lock, the lock is properly released via defer, allowing other waiters
// to proceed. This tests the error path and cleanup behavior of the MigrateWithLock pattern.
func TestMigrationFailureUnderLock(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	// Reset database to clean state
	err := h.ResetDB()
	Expect(err).NotTo(HaveOccurred(), "Failed to reset database")

	// Channels to coordinate goroutines
	firstLockAcquired := make(chan bool, 1)
	firstMigrationFailed := make(chan bool, 1)
	secondCanProceed := make(chan bool, 1)

	// Track results
	var mu sync.Mutex
	successCount := 0
	failureCount := 0
	var wg sync.WaitGroup

	// Create a failing migration function that signals when it acquires lock and fails
	failingMigration := func(_ *gorm.DB) error {
		firstLockAcquired <- true
		// Wait a bit to ensure second goroutine tries to acquire
		time.Sleep(50 * time.Millisecond)
		return fmt.Errorf("simulated migration failure")
	}

	// Create a successful migration function
	successfulMigration := func(_ *gorm.DB) error {
		return nil
	}

	// First goroutine: acquire lock and fail migration using production code path
	wg.Add(1)
	go func() {
		defer wg.Done()

		ctx := context.Background()
		err := migrateWithLockAndCustomMigration(ctx, h.DBFactory, failingMigration)

		mu.Lock()
		if err != nil {
			failureCount++
		}
		mu.Unlock()

		firstMigrationFailed <- true
		// Lock should be released via defer even though migration failed
	}()

	// Wait for first goroutine to acquire lock
	<-firstLockAcquired

	// Second goroutine: should block until first releases lock, then succeed
	wg.Add(1)
	go func() {
		defer wg.Done()

		ctx := context.Background()
		err := migrateWithLockAndCustomMigration(ctx, h.DBFactory, successfulMigration)

		mu.Lock()
		if err == nil {
			successCount++
		}
		mu.Unlock()

		secondCanProceed <- true
	}()

	// Wait for first migration to fail and release lock
	<-firstMigrationFailed

	// Wait for second migration to complete
	select {
	case <-secondCanProceed:
		// Expected: second goroutine acquired lock after first released it
	case <-time.After(3 * time.Second):
		t.Error("Second goroutine did not acquire lock after first failed")
	}

	wg.Wait()

	// Verify both completed as expected
	Expect(failureCount).To(Equal(1), "Expected 1 failure")
	Expect(successCount).To(Equal(1), "Expected 1 success")
}
