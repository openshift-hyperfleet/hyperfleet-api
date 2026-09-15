package closer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
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
				slog.ErrorContext(ctx, "closer: step failed", "step", i, "duration", elapsed, "error", err)
				joined = errors.Join(joined, fmt.Errorf("step %d: %w", i, err))
			} else {
				slog.InfoContext(ctx, "closer: step completed", "step", i, "duration", elapsed)
			}
		}

		c.result = joined
	})
	return c.result
}
