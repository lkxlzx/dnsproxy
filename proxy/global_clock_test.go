package proxy

import (
	"context"
	"testing"
	"time"
)

func TestGlobalClock_Basic(t *testing.T) {
	gc := newGlobalClock()
	
	// Initial value should be 0
	if got := gc.get(); got != 0 {
		t.Errorf("initial clock value = %d, want 0", got)
	}
}

func TestGlobalClock_Increment(t *testing.T) {
	gc := newGlobalClock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	
	gc.start(ctx)
	
	// Wait for clock to increment
	time.Sleep(2500 * time.Millisecond)
	
	got := gc.get()
	// Should be around 2 (allowing for timing variance)
	if got < 2 || got > 3 {
		t.Errorf("clock value after 2.5s = %d, want 2 or 3", got)
	}
}

func TestGlobalClock_Reset(t *testing.T) {
	gc := newGlobalClock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	
	gc.start(ctx)
	
	// Wait for clock to increment
	time.Sleep(1500 * time.Millisecond)
	
	if got := gc.get(); got < 1 {
		t.Errorf("clock value before reset = %d, want >= 1", got)
	}
	
	// Reset clock
	gc.reset()
	
	if got := gc.get(); got != 0 {
		t.Errorf("clock value after reset = %d, want 0", got)
	}
	
	// Wait and verify it continues incrementing
	time.Sleep(1500 * time.Millisecond)
	
	got := gc.get()
	if got < 1 || got > 2 {
		t.Errorf("clock value after reset and 1.5s = %d, want 1 or 2", got)
	}
}

func TestGlobalClock_ConcurrentAccess(t *testing.T) {
	gc := newGlobalClock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	gc.start(ctx)
	
	// Multiple goroutines reading concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = gc.get()
				time.Sleep(1 * time.Millisecond)
			}
			done <- true
		}()
	}
	
	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// Should not panic or race
	t.Log("Concurrent access test passed")
}
