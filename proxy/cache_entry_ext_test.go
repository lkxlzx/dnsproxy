package proxy

import (
	"testing"
	"time"
)

func TestCacheEntryExt_IsExpired(t *testing.T) {
	now := time.Now()
	entry := newCacheEntryExt("test.com.", 1, 300, now)

	// Not expired: 100s elapsed, TTL=300
	if entry.isExpired(now.Add(100 * time.Second).Unix()) {
		t.Error("should not be expired at 100s")
	}
	// Expired: 300s elapsed
	if !entry.isExpired(now.Add(300 * time.Second).Unix()) {
		t.Error("should be expired at 300s")
	}
	// Expired: 350s elapsed
	if !entry.isExpired(now.Add(350 * time.Second).Unix()) {
		t.Error("should be expired at 350s")
	}
}

func TestCacheEntryExt_RemainingTTL(t *testing.T) {
	now := time.Now()
	entry := newCacheEntryExt("test.com.", 1, 300, now)

	tests := []struct {
		elapsed time.Duration
		want    uint32
	}{
		{100 * time.Second, 200},
		{295 * time.Second, 5},
		{300 * time.Second, 0},
		{350 * time.Second, 0},
	}
	for _, tt := range tests {
		got := entry.remainingTTL(now.Add(tt.elapsed).Unix())
		if got != tt.want {
			t.Errorf("elapsed=%s: remainingTTL=%d, want %d", tt.elapsed, got, tt.want)
		}
	}
}

func TestCacheEntryExt_RefreshTTL(t *testing.T) {
	now := time.Now()
	entry := newCacheEntryExt("test.com.", 1, 300, now)

	later := now.Add(10 * time.Second)
	entry.refreshTTL(600, later)

	if entry.originalTTL != 600 {
		t.Errorf("originalTTL = %d, want 600", entry.originalTTL)
	}
	if entry.cachedAt != later.Unix() {
		t.Error("cachedAt not updated")
	}
	// After refresh, remaining should be ~600s
	remaining := entry.remainingTTL(later.Unix())
	if remaining != 600 {
		t.Errorf("remaining after refresh = %d, want 600", remaining)
	}
}

func TestCacheEntryExt_ResetColdStart(t *testing.T) {
	now := time.Now()
	entry := newCacheEntryExt("test.com.", 1, 300, now)
	entry.firstAccessTime = now
	entry.accessCount = 5
	entry.heatScore = 5

	entry.resetColdStart()

	if !entry.firstAccessTime.IsZero() {
		t.Error("firstAccessTime should be zero after reset")
	}
	if entry.accessCount != 0 {
		t.Errorf("accessCount = %d, want 0", entry.accessCount)
	}
	if entry.heatScore != 0 {
		t.Errorf("heatScore = %d, want 0", entry.heatScore)
	}
}
