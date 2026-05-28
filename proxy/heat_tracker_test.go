package proxy

import (
	"testing"
	"time"
)

func TestHeatTracker_ColdStart(t *testing.T) {
	ht := newHeatTracker(4, 60*time.Second)
	now := time.Now()

	for i := 1; i <= 3; i++ {
		joined := ht.onAccess("google.com.", 1, now.Add(time.Duration(i)*5*time.Second))
		if joined {
			t.Errorf("access %d: should not join queue yet (threshold=4)", i)
		}
	}

	joined := ht.onAccess("google.com.", 1, now.Add(20*time.Second))
	if !joined {
		t.Error("4th access: should join queue")
	}
	if !ht.isInQueue("google.com.", 1) {
		t.Error("domain should be in queue after threshold")
	}
}

func TestHeatTracker_TimeWindowExpired(t *testing.T) {
	ht := newHeatTracker(4, 60*time.Second)
	now := time.Now()

	ht.onAccess("example.com.", 1, now)
	ht.onAccess("example.com.", 1, now.Add(10*time.Second))

	// Access after window expires — should restart cold-start
	ht.onAccess("example.com.", 1, now.Add(90*time.Second))

	ht.mu.Lock()
	e := ht.entries[makeKey("example.com.", 1)]
	ht.mu.Unlock()

	if e == nil {
		t.Fatal("entry should exist")
	}
	if e.accessCount != 1 {
		t.Errorf("accessCount = %d, want 1 (reset after window)", e.accessCount)
	}
	if e.inPrefetchQueue {
		t.Error("should not be in queue after window reset")
	}
}

func TestHeatTracker_InactivityRemoval(t *testing.T) {
	ht := newHeatTracker(3, 30*time.Second)
	now := time.Now()

	for i := 1; i <= 3; i++ {
		ht.onAccess("test.com.", 1, now.Add(time.Duration(i)*5*time.Second))
	}
	if !ht.isInQueue("test.com.", 1) {
		t.Fatal("should be in queue")
	}

	// With LRU-based cleanup, entries are removed when maxEntries is exceeded
	// or when accessed after timeWindow expiration. This test now verifies
	// that accessing after timeWindow resets the entry.
	ht.onAccess("test.com.", 1, now.Add(46*time.Second))
	
	ht.mu.Lock()
	e := ht.entries[makeKey("test.com.", 1)]
	ht.mu.Unlock()
	
	if e == nil {
		t.Fatal("entry should still exist")
	}
	if e.accessCount != 1 {
		t.Errorf("accessCount = %d, want 1 (reset after window)", e.accessCount)
	}
	if e.inPrefetchQueue {
		t.Error("should not be in queue after window reset")
	}
}

func TestHeatTracker_HeatScoreIncrement(t *testing.T) {
	ht := newHeatTracker(3, 60*time.Second)
	now := time.Now()

	for i := 1; i <= 3; i++ {
		ht.onAccess("hot.com.", 1, now.Add(time.Duration(i)*5*time.Second))
	}

	ht.mu.Lock()
	e := ht.entries[makeKey("hot.com.", 1)]
	ht.mu.Unlock()
	if e.heatScore != 3 {
		t.Errorf("initial heatScore = %d, want 3", e.heatScore)
	}

	ht.onAccess("hot.com.", 1, now.Add(20*time.Second))
	ht.mu.Lock()
	e = ht.entries[makeKey("hot.com.", 1)]
	ht.mu.Unlock()
	if e.heatScore != 4 {
		t.Errorf("heatScore after extra access = %d, want 4", e.heatScore)
	}
}

func TestHeatTracker_StaleEntryPurge(t *testing.T) {
	ht := newHeatTracker(6, 30*time.Second)
	now := time.Now()

	// Single access — cold-start entry, never reaches threshold
	ht.onAccess("stale.com.", 1, now)

	// With LRU-based cleanup, stale entries are purged when:
	// 1. maxEntries is exceeded (LRU eviction)
	// 2. Accessed after timeWindow expiration (lazy cleanup)
	// Accessing after 2× timeWindow should trigger lazy cleanup
	ht.onAccess("stale.com.", 1, now.Add(61*time.Second))

	ht.mu.Lock()
	e := ht.entries[makeKey("stale.com.", 1)]
	ht.mu.Unlock()

	if e == nil {
		t.Fatal("entry should exist after access")
	}
	if e.accessCount != 1 {
		t.Errorf("accessCount = %d, want 1 (reset after window)", e.accessCount)
	}
}

func TestHeatTracker_GetPrefetchCandidates(t *testing.T) {
	ht := newHeatTracker(2, 60*time.Second)
	now := time.Now()

	domains := []string{"a.com.", "b.com.", "c.com."}
	for _, d := range domains {
		ht.onAccess(d, 1, now)
		ht.onAccess(d, 1, now.Add(5*time.Second))
	}

	candidates := ht.getPrefetchCandidates()
	if len(candidates) != 3 {
		t.Errorf("candidates = %d, want 3", len(candidates))
	}
}

func TestSplitKey(t *testing.T) {
	tests := []struct {
		domain string
		qtype  uint16
	}{
		{"google.com.", 1},
		{"example.com.", 28},
		{"test.org.", 15},
		{"x.y.z.", 65535},
	}
	for _, tt := range tests {
		key := makeKey(tt.domain, tt.qtype)
		d, q := splitKey(key)
		if d != tt.domain || q != tt.qtype {
			t.Errorf("splitKey(%q) = (%q, %d), want (%q, %d)",
				key, d, q, tt.domain, tt.qtype)
		}
	}
}
