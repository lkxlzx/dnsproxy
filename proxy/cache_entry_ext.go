package proxy

import (
	"time"
)

// cacheEntryExt extends cacheItem with heat tracking and prefetch queue information.
// This is used for the smart prefetch mechanism.
type cacheEntryExt struct {
	// Original cache item
	item *cacheItem

	// TTL management (uses global clock T, resets on prefetch)
	expiresAt   uint32 // Expiration time in global clock T
	originalTTL uint32 // Original TTL value for percentage threshold calculation

	// Heat management (uses real time, never resets)
	lastAccessTime  time.Time // Last access time (real time)
	firstAccessTime time.Time // First access time for cold-start phase (real time)
	accessCount     int       // Access count during cold-start phase
	inPrefetchQueue bool      // Whether this domain is in the prefetch queue
	heatScore       int64     // Current heat score

	// Domain and query type for identification
	domain string
	qtype  uint16
}

// newCacheEntryExt creates a new extended cache entry.
func newCacheEntryExt(item *cacheItem, domain string, qtype uint16, ttl uint32) *cacheEntryExt {
	now := time.Now()
	return &cacheEntryExt{
		item:            item,
		expiresAt:       ttl,
		originalTTL:     ttl,
		lastAccessTime:  now,
		firstAccessTime: time.Time{}, // Will be set on first access
		accessCount:     0,
		inPrefetchQueue: false,
		heatScore:       0,
		domain:          domain,
		qtype:           qtype,
	}
}

// isExpired checks if the entry is expired based on the global clock.
func (e *cacheEntryExt) isExpired(currentT uint32) bool {
	return currentT >= e.expiresAt
}

// remainingTTL calculates the remaining TTL in seconds.
func (e *cacheEntryExt) remainingTTL(currentT uint32) uint32 {
	if currentT >= e.expiresAt {
		return 0
	}
	return e.expiresAt - currentT
}

// updateOnAccess updates the entry when it's accessed.
// This is called for both cache hits and misses (after fetching).
func (e *cacheEntryExt) updateOnAccess(now time.Time) {
	e.lastAccessTime = now

	if e.inPrefetchQueue {
		// Already in prefetch queue, just increment heat score
		e.heatScore++
	}
	// Cold-start phase logic is handled by heatTracker
}

// updateOnPrefetch updates the entry after a successful prefetch.
func (e *cacheEntryExt) updateOnPrefetch(newItem *cacheItem, newTTL uint32, now time.Time) {
	e.item = newItem
	e.expiresAt = newTTL
	e.originalTTL = newTTL
	e.lastAccessTime = now // Prefetch counts as activity
}

// resetColdStart resets the cold-start phase tracking.
func (e *cacheEntryExt) resetColdStart() {
	e.firstAccessTime = time.Time{}
	e.accessCount = 0
	e.heatScore = 0
}
