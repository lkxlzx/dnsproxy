package proxy

import (
	"testing"
	"time"
)

func TestHeatTracker_ColdStart(t *testing.T) {
	tracker := newHeatTracker(6, 180*time.Second)
	
	entry := &cacheEntryExt{
		domain: "google.com",
		qtype:  1, // A record
	}
	
	now := time.Now()
	
	// First 5 accesses should not join queue
	for i := 1; i <= 5; i++ {
		joined := tracker.onAccess(entry, now.Add(time.Duration(i)*10*time.Second))
		if joined {
			t.Errorf("access %d: should not join queue yet", i)
		}
		if entry.inPrefetchQueue {
			t.Errorf("access %d: inPrefetchQueue should be false", i)
		}
		if entry.accessCount != i {
			t.Errorf("access %d: accessCount = %d, want %d", i, entry.accessCount, i)
		}
	}
	
	// 6th access should join queue
	joined := tracker.onAccess(entry, now.Add(60*time.Second))
	if !joined {
		t.Error("6th access: should join queue")
	}
	if !entry.inPrefetchQueue {
		t.Error("6th access: inPrefetchQueue should be true")
	}
	if entry.accessCount != 6 {
		t.Errorf("6th access: accessCount = %d, want 6", entry.accessCount)
	}
}

func TestHeatTracker_TimeWindowExpired(t *testing.T) {
	tracker := newHeatTracker(6, 180*time.Second)
	
	entry := &cacheEntryExt{
		domain: "example.com",
		qtype:  1,
	}
	
	now := time.Now()
	
	// First access
	tracker.onAccess(entry, now)
	if entry.accessCount != 1 {
		t.Errorf("first access: accessCount = %d, want 1", entry.accessCount)
	}
	
	// Second access after time window expired (>180s)
	tracker.onAccess(entry, now.Add(200*time.Second))
	
	// Should reset and start counting again
	if entry.accessCount != 1 {
		t.Errorf("after time window: accessCount = %d, want 1 (reset)", entry.accessCount)
	}
	if entry.inPrefetchQueue {
		t.Error("after time window: should not be in queue")
	}
}

func TestHeatTracker_QueueManagement(t *testing.T) {
	tracker := newHeatTracker(6, 180*time.Second)
	
	entry1 := &cacheEntryExt{domain: "google.com", qtype: 1}
	entry2 := &cacheEntryExt{domain: "facebook.com", qtype: 1}
	
	now := time.Now()
	
	// Add entry1 to queue
	for i := 1; i <= 6; i++ {
		tracker.onAccess(entry1, now.Add(time.Duration(i)*10*time.Second))
	}
	
	// Add entry2 to queue
	for i := 1; i <= 6; i++ {
		tracker.onAccess(entry2, now.Add(time.Duration(i)*10*time.Second))
	}
	
	// Check candidates
	candidates := tracker.getPrefetchCandidates()
	if len(candidates) != 2 {
		t.Errorf("candidates count = %d, want 2", len(candidates))
	}
}

func TestHeatTracker_InactivityRemoval(t *testing.T) {
	tracker := newHeatTracker(6, 180*time.Second)
	
	entry := &cacheEntryExt{
		domain: "test.com",
		qtype:  1,
	}
	
	now := time.Now()
	
	// Add to queue (last access at 60s)
	for i := 1; i <= 6; i++ {
		tracker.onAccess(entry, now.Add(time.Duration(i)*10*time.Second))
	}
	
	if !entry.inPrefetchQueue {
		t.Fatal("entry should be in queue")
	}
	
	// Last access was at now+60s
	// Check inactivity at now+60s+181s = now+241s (>180s threshold)
	removed := tracker.checkInactivity(now.Add(241 * time.Second))
	
	if len(removed) != 1 {
		t.Errorf("removed count = %d, want 1", len(removed))
	}
	
	if entry.inPrefetchQueue {
		t.Error("entry should be removed from queue")
	}
	
	if entry.accessCount != 0 {
		t.Errorf("accessCount after removal = %d, want 0", entry.accessCount)
	}
}

func TestHeatTracker_HeatScoreIncrement(t *testing.T) {
	tracker := newHeatTracker(6, 180*time.Second)
	
	entry := &cacheEntryExt{
		domain: "popular.com",
		qtype:  1,
	}
	
	now := time.Now()
	
	// Add to queue (6 accesses)
	for i := 1; i <= 6; i++ {
		tracker.onAccess(entry, now.Add(time.Duration(i)*10*time.Second))
	}
	
	initialHeat := entry.heatScore
	if initialHeat != 6 {
		t.Errorf("initial heat score = %d, want 6", initialHeat)
	}
	
	// Additional accesses should increment heat score
	tracker.onAccess(entry, now.Add(70*time.Second))
	if entry.heatScore != 7 {
		t.Errorf("heat score after 7th access = %d, want 7", entry.heatScore)
	}
	
	tracker.onAccess(entry, now.Add(80*time.Second))
	if entry.heatScore != 8 {
		t.Errorf("heat score after 8th access = %d, want 8", entry.heatScore)
	}
}

func TestHeatTracker_PrefetchCountsAsActivity(t *testing.T) {
	tracker := newHeatTracker(6, 180*time.Second)
	
	entry := &cacheEntryExt{
		domain: "active.com",
		qtype:  1,
	}
	
	now := time.Now()
	
	// Add to queue
	for i := 1; i <= 6; i++ {
		tracker.onAccess(entry, now.Add(time.Duration(i)*10*time.Second))
	}
	
	lastAccess := now.Add(60 * time.Second)
	entry.lastAccessTime = lastAccess
	
	// Check inactivity at 60s + 179s = 239s (should still be active)
	removed := tracker.checkInactivity(now.Add(239 * time.Second))
	
	if len(removed) != 0 {
		t.Error("entry should still be active (within 180s window)")
	}
	
	// Check at 60s + 181s = 241s (should be inactive)
	removed = tracker.checkInactivity(now.Add(241 * time.Second))
	
	if len(removed) != 1 {
		t.Error("entry should be removed (exceeded 180s window)")
	}
}
