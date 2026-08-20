package closer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

type Closer struct {
	result error
	fns    []func() error
	once   sync.Once
	mu     sync.Mutex
	closed bool
}

func New() *Closer {
	return &Closer{}
}

func (c *Closer) Add(fn func() error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		panic("closer: Add called after Close started")
	}
	c.fns = append(c.fns, fn)
}

func (c *Closer) Close() error {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
	}
	fns := c.fns
	c.mu.Unlock()

	c.once.Do(func() {
		ctx := context.Background()
		var joined error
		for i := len(fns) - 1; i >= 0; i-- {
			start := time.Now()
			err := fns[i]()
			elapsed := time.Since(start)
			if err != nil {
				logger.With(ctx, "step", i, "duration", elapsed).WithError(err).Error("closer: step failed")
				joined = errors.Join(joined, fmt.Errorf("step %d: %w", i, err))
			} else {
				logger.With(ctx, "step", i, "duration", elapsed).Info("closer: step completed")
			}
		}

		c.result = joined
	})
	return c.result
}
