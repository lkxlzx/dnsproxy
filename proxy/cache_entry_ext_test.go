package proxy

import (
	"testing"
	"time"
)

func TestCacheEntryExt_IsExpired(t *testing.T) {
	entry := &cacheEntryExt{
		expiresAt: 300, // Expires at T=300
	}
	
	tests := []struct {
		name      string
		currentT  uint32
		wantExpired bool
	}{
		{"before expiration", 100, false},
		{"at expiration", 300, true},
		{"after expiration", 350, true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := entry.isExpired(tt.currentT)
			if got != tt.wantExpired {
				t.Errorf("isExpired(%d) = %v, want %v", tt.currentT, got, tt.wantExpired)
			}
		})
	}
}

func TestCacheEntryExt_RemainingTTL(t *testing.T) {
	entry := &cacheEntryExt{
		expiresAt: 300,
	}
	
	tests := []struct {
		name     string
		currentT uint32
		want     uint32
	}{
		{"far from expiration", 100, 200},
		{"close to expiration", 295, 5},
		{"at expiration", 300, 0},
		{"after expiration", 350, 0},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := entry.remainingTTL(tt.currentT)
			if got != tt.want {
				t.Errorf("remainingTTL(%d) = %d, want %d", tt.currentT, got, tt.want)
			}
		})
	}
}

func TestCacheEntryExt_UpdateOnAccess(t *testing.T) {
	entry := &cacheEntryExt{
		domain:          "test.com",
		qtype:           1,
		inPrefetchQueue: false,
		heatScore:       0,
	}
	
	now := time.Now()
	entry.updateOnAccess(now)
	
	if !entry.lastAccessTime.Equal(now) {
		t.Error("lastAccessTime should be updated")
	}
	
	// When in queue, heat score should increment
	entry.inPrefetchQueue = true
	entry.heatScore = 5
	
	later := now.Add(10 * time.Second)
	entry.updateOnAccess(later)
	
	if entry.heatScore != 6 {
		t.Errorf("heatScore = %d, want 6", entry.heatScore)
	}
	
	if !entry.lastAccessTime.Equal(later) {
		t.Error("lastAccessTime should be updated to later time")
	}
}

func TestCacheEntryExt_UpdateOnPrefetch(t *testing.T) {
	oldItem := &cacheItem{ttl: 300}
	newItem := &cacheItem{ttl: 305}
	
	entry := &cacheEntryExt{
		item:        oldItem,
		expiresAt:   300,
		originalTTL: 300,
	}
	
	now := time.Now()
	entry.updateOnPrefetch(newItem, 305, now)
	
	if entry.item != newItem {
		t.Error("item should be updated to newItem")
	}
	
	if entry.expiresAt != 305 {
		t.Errorf("expiresAt = %d, want 305", entry.expiresAt)
	}
	
	if entry.originalTTL != 305 {
		t.Errorf("originalTTL = %d, want 305", entry.originalTTL)
	}
	
	if !entry.lastAccessTime.Equal(now) {
		t.Error("lastAccessTime should be updated (prefetch counts as activity)")
	}
}

func TestCacheEntryExt_ResetColdStart(t *testing.T) {
	now := time.Now()
	entry := &cacheEntryExt{
		firstAccessTime: now,
		accessCount:     5,
		heatScore:       5,
	}
	
	entry.resetColdStart()
	
	if !entry.firstAccessTime.IsZero() {
		t.Error("firstAccessTime should be reset to zero")
	}
	
	if entry.accessCount != 0 {
		t.Errorf("accessCount = %d, want 0", entry.accessCount)
	}
	
	if entry.heatScore != 0 {
		t.Errorf("heatScore = %d, want 0", entry.heatScore)
	}
}

func TestNewCacheEntryExt(t *testing.T) {
	item := &cacheItem{ttl: 300}
	domain := "example.com"
	qtype := uint16(1)
	ttl := uint32(300)
	
	entry := newCacheEntryExt(item, domain, qtype, ttl)
	
	if entry.item != item {
		t.Error("item not set correctly")
	}
	
	if entry.domain != domain {
		t.Errorf("domain = %s, want %s", entry.domain, domain)
	}
	
	if entry.qtype != qtype {
		t.Errorf("qtype = %d, want %d", entry.qtype, qtype)
	}
	
	if entry.expiresAt != ttl {
		t.Errorf("expiresAt = %d, want %d", entry.expiresAt, ttl)
	}
	
	if entry.originalTTL != ttl {
		t.Errorf("originalTTL = %d, want %d", entry.originalTTL, ttl)
	}
	
	if entry.lastAccessTime.IsZero() {
		t.Error("lastAccessTime should be initialized")
	}
	
	if !entry.firstAccessTime.IsZero() {
		t.Error("firstAccessTime should be zero initially")
	}
	
	if entry.accessCount != 0 {
		t.Errorf("accessCount = %d, want 0", entry.accessCount)
	}
	
	if entry.inPrefetchQueue {
		t.Error("inPrefetchQueue should be false initially")
	}
	
	if entry.heatScore != 0 {
		t.Errorf("heatScore = %d, want 0", entry.heatScore)
	}
}
