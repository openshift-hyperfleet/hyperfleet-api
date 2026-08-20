package closer

import (
	"errors"
	"sync"
	"testing"

	. "github.com/onsi/gomega"
)

func TestReverseOrder(t *testing.T) {
	RegisterTestingT(t)

	var order []int
	c := New()
	for i := range 5 {
		c.Add(func() error {
			order = append(order, i)
			return nil
		})
	}

	err := c.Close()
	Expect(err).NotTo(HaveOccurred())
	Expect(order).To(Equal([]int{4, 3, 2, 1, 0}))
}

func TestIdempotent(t *testing.T) {
	RegisterTestingT(t)

	calls := 0
	sentinel := errors.New("boom")
	c := New()
	c.Add(func() error {
		calls++
		return sentinel
	})

	err1 := c.Close()
	err2 := c.Close()

	Expect(calls).To(Equal(1))
	Expect(err1).To(MatchError(sentinel))
	Expect(err2).To(Equal(err1))
}

func TestIdempotentNilError(t *testing.T) {
	RegisterTestingT(t)

	calls := 0
	c := New()
	c.Add(func() error {
		calls++
		return nil
	})

	err1 := c.Close()
	err2 := c.Close()

	Expect(calls).To(Equal(1))
	Expect(err1).NotTo(HaveOccurred())
	Expect(err2).NotTo(HaveOccurred())
}

func TestOneFailureDoesNotAbortUnwind(t *testing.T) {
	RegisterTestingT(t)

	var order []int
	c := New()
	c.Add(func() error { order = append(order, 0); return nil })
	c.Add(func() error { order = append(order, 1); return errors.New("fail-1") })
	c.Add(func() error { order = append(order, 2); return nil })

	err := c.Close()

	Expect(order).To(Equal([]int{2, 1, 0}))
	Expect(err).To(HaveOccurred())
}

func TestEveryErrorRetrievableViaErrorsIs(t *testing.T) {
	RegisterTestingT(t)

	errA := errors.New("a")
	errB := errors.New("b")
	errC := errors.New("c")

	c := New()
	c.Add(func() error { return errA })
	c.Add(func() error { return errB })
	c.Add(func() error { return nil })
	c.Add(func() error { return errC })

	err := c.Close()

	Expect(errors.Is(err, errA)).To(BeTrue())
	Expect(errors.Is(err, errB)).To(BeTrue())
	Expect(errors.Is(err, errC)).To(BeTrue())
}

func TestAllNilErrorsProduceNilResult(t *testing.T) {
	RegisterTestingT(t)

	c := New()
	c.Add(func() error { return nil })
	c.Add(func() error { return nil })

	err := c.Close()
	Expect(err).NotTo(HaveOccurred())
}

func TestEmptyCloserReturnsNil(t *testing.T) {
	RegisterTestingT(t)

	c := New()
	err := c.Close()
	Expect(err).NotTo(HaveOccurred())
}

func TestConcurrentAdd(t *testing.T) {
	RegisterTestingT(t)

	c := New()
	var wg sync.WaitGroup
	n := 100
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			c.Add(func() error { return nil })
		}()
	}
	wg.Wait()

	Expect(c.Close()).NotTo(HaveOccurred())
	Expect(c.fns).To(HaveLen(n))
}

func TestTwoInstancesAreIndependent(t *testing.T) {
	RegisterTestingT(t)

	var orderA, orderB []int
	a := New()
	b := New()

	a.Add(func() error { orderA = append(orderA, 1); return nil })
	a.Add(func() error { orderA = append(orderA, 2); return nil })
	b.Add(func() error { orderB = append(orderB, 3); return nil })

	errA := a.Close()
	Expect(errA).NotTo(HaveOccurred())
	Expect(orderA).To(Equal([]int{2, 1}))
	Expect(orderB).To(BeEmpty())

	errB := b.Close()
	Expect(errB).NotTo(HaveOccurred())
	Expect(orderB).To(Equal([]int{3}))
}

func TestAddDuringClosePanics(t *testing.T) {
	RegisterTestingT(t)

	c := New()
	c.Add(func() error {
		Expect(func() {
			c.Add(func() error { return nil })
		}).To(PanicWith("closer: Add called after Close started"))
		return nil
	})

	Expect(c.Close()).NotTo(HaveOccurred())
}

func TestConcurrentClose(t *testing.T) {
	RegisterTestingT(t)

	sentinel := errors.New("boom")
	c := New()
	c.Add(func() error { return sentinel })

	var wg sync.WaitGroup
	errs := make([]error, 10)
	wg.Add(len(errs))
	for i := range errs {
		go func(idx int) {
			defer wg.Done()
			errs[idx] = c.Close()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		Expect(err).To(MatchError(sentinel), "goroutine %d", i)
	}
}
