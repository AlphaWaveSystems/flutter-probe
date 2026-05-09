package runner

import (
	"context"
	"sync"
)

// syncBarrier coordinates N goroutines at a named sync point. All goroutines
// must call Arrive before any of them proceed. If the context is cancelled
// (e.g. because another device failed), Arrive returns immediately.
//
// Each barrier is single-use: pre-initialized with N and each goroutine calls
// Arrive exactly once. Call Abort to unblock all waiters early (used on failure).
type syncBarrier struct {
	gate sync.Once
	ch   chan struct{}
	mu   sync.Mutex
	cnt  int
	n    int
}

func newSyncBarrier(n int) *syncBarrier {
	return &syncBarrier{n: n, ch: make(chan struct{})}
}

// Arrive signals that this goroutine has reached the barrier and blocks until
// all n goroutines have arrived, or until ctx is cancelled.
func (b *syncBarrier) Arrive(ctx context.Context) error {
	b.mu.Lock()
	b.cnt++
	last := b.cnt == b.n
	b.mu.Unlock()

	if last {
		b.gate.Do(func() { close(b.ch) })
		return nil
	}

	select {
	case <-b.ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Abort closes the gate immediately, unblocking all goroutines waiting in Arrive.
// Used when a device fails to prevent other devices from hanging at the barrier.
func (b *syncBarrier) Abort() {
	b.gate.Do(func() { close(b.ch) })
}
