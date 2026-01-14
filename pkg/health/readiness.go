package health

import (
	"sync"
)

// ReadinessState represents the current readiness state of the application
type ReadinessState struct {
	mu           sync.RWMutex
	ready        bool
	shuttingDown bool
}

var (
	globalState     *ReadinessState
	globalStateOnce sync.Once
)

// GetReadinessState returns the singleton ReadinessState instance
func GetReadinessState() *ReadinessState {
	globalStateOnce.Do(func() {
		globalState = &ReadinessState{
			ready:        false,
			shuttingDown: false,
		}
	})
	return globalState
}

// SetReady marks the application as ready to receive traffic
func (r *ReadinessState) SetReady() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ready = true
	r.shuttingDown = false
}

// SetShuttingDown marks the application as shutting down
// This will cause the readiness check to fail, signaling to Kubernetes
// to stop routing traffic to this instance
func (r *ReadinessState) SetShuttingDown() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shuttingDown = true
}

// IsReady returns true if the application is ready and not shutting down
func (r *ReadinessState) IsReady() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ready && !r.shuttingDown
}

// IsShuttingDown returns true if the application is shutting down
func (r *ReadinessState) IsShuttingDown() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.shuttingDown
}
