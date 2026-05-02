package proxy

import (
	"time"
)

// cacheEntryExt extends cacheItem with heat tracking and prefetch queue information.
// TTL tracking uses real Unix timestamps (cachedAt + originalTTL) instead of a
// global clock, so each entry is independent and there is no shared mutable state
// for expiry calculation.
type cacheEntryExt struct {
	// TTL management (real Unix timestamps, never reset globally)
	cachedAt    int64  // Unix timestamp when this entry was cached / last refreshed
	originalTTL uint32 // TTL at cache time, used for percentage threshold

	// Heat management (real time, never resets)
	lastAccessTime  time.Time // Last access time
	firstAccessTime time.Time // First access time for cold-start phase
	accessCount     int       // Access count during cold-start phase
	inPrefetchQueue bool      // Whether this domain is in the prefetch queue
	heatScore       int64     // Current heat score (incremented on each access)

	// Domain and query type for identification
	domain string
	qtype  uint16
}

// newCacheEntryExt creates a new extended cache entry.
func newCacheEntryExt(domain string, qtype uint16, ttl uint32, now time.Time) *cacheEntryExt {
	return &cacheEntryExt{
		cachedAt:        now.Unix(),
		originalTTL:     ttl,
		lastAccessTime:  now,
		firstAccessTime: time.Time{}, // set on first onAccess call
		accessCount:     0,
		inPrefetchQueue: false,
		heatScore:       0,
		domain:          domain,
		qtype:           qtype,
	}
}

// remainingTTL returns remaining TTL in seconds relative to nowUnix.
func (e *cacheEntryExt) remainingTTL(nowUnix int64) uint32 {
	elapsed := nowUnix - e.cachedAt
	if elapsed < 0 {
		elapsed = 0
	}
	if uint32(elapsed) >= e.originalTTL {
		return 0
	}
	return e.originalTTL - uint32(elapsed)
}

// isExpired reports whether the entry has passed its TTL.
func (e *cacheEntryExt) isExpired(nowUnix int64) bool {
	return e.remainingTTL(nowUnix) == 0
}

// refreshTTL resets cachedAt and originalTTL after a successful prefetch.
func (e *cacheEntryExt) refreshTTL(newTTL uint32, now time.Time) {
	e.cachedAt = now.Unix()
	e.originalTTL = newTTL
	e.lastAccessTime = now
}

// resetColdStart resets the cold-start phase tracking.
func (e *cacheEntryExt) resetColdStart() {
	e.firstAccessTime = time.Time{}
	e.accessCount = 0
	e.heatScore = 0
}
