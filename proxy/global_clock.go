package proxy

import (
	"context"
	"sync/atomic"
	"time"
)

// globalClock represents a global clock T for TTL management.
// It starts at 0 and increments every second.
// It can be reset to 0 after successful prefetch operations.
type globalClock struct {
	// value is the current clock value in seconds (atomic access).
	value atomic.Uint32

	// startTime is the real time when the clock started or was last reset.
	startTime atomic.Int64
}

// newGlobalClock creates a new global clock starting at 0.
func newGlobalClock() *globalClock {
	gc := &globalClock{}
	gc.startTime.Store(time.Now().Unix())
	return gc
}

// start begins the clock increment goroutine.
func (gc *globalClock) start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				gc.value.Add(1)
			}
		}
	}()
}

// get returns the current clock value.
func (gc *globalClock) get() uint32 {
	return gc.value.Load()
}

// reset resets the clock to 0.
// This should be called after a successful prefetch operation.
func (gc *globalClock) reset() {
	gc.value.Store(0)
	gc.startTime.Store(time.Now().Unix())
}
