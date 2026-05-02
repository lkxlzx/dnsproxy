package proxy

import (
	"sync"
	"time"
)

// heatTracker tracks domain access heat and manages the prefetch queue.
type heatTracker struct {
	// Configuration
	minHeatThreshold int           // Minimum access count to join prefetch queue
	timeWindow       time.Duration // Time window for cold-start and inactivity checks

	// Tracking data
	entries map[string]*cacheEntryExt // All tracked entries (domain+qtype as key)
	mu      sync.RWMutex              // Protects entries map
	
	// Object pool for key strings to reduce allocations
	keyPool sync.Pool
}

// newHeatTracker creates a new heat tracker.
func newHeatTracker(minHeatThreshold int, timeWindow time.Duration) *heatTracker {
	return &heatTracker{
		minHeatThreshold: minHeatThreshold,
		timeWindow:       timeWindow,
		entries:          make(map[string]*cacheEntryExt),
		keyPool: sync.Pool{
			New: func() interface{} {
				// Pre-allocate buffer for typical domain:qtype key
				buf := make([]byte, 0, 256)
				return &buf
			},
		},
	}
}

// makeKey creates a unique key for domain and query type.
// Optimized version using string builder to reduce allocations.
func makeKey(domain string, qtype uint16) string {
	// Fast path for common case
	if len(domain) < 240 {
		// Use stack-allocated buffer for small domains
		buf := make([]byte, 0, len(domain)+6)
		buf = append(buf, domain...)
		buf = append(buf, ':')
		buf = appendUint16(buf, qtype)
		return string(buf)
	}
	// Slow path for very long domains
	return domain + ":" + string(rune(qtype))
}

// appendUint16 appends uint16 to byte slice efficiently.
func appendUint16(buf []byte, n uint16) []byte {
	if n < 10 {
		return append(buf, byte('0'+n))
	}
	if n < 100 {
		return append(buf, byte('0'+n/10), byte('0'+n%10))
	}
	if n < 1000 {
		return append(buf, byte('0'+n/100), byte('0'+(n/10)%10), byte('0'+n%10))
	}
	if n < 10000 {
		return append(buf, byte('0'+n/1000), byte('0'+(n/100)%10), byte('0'+(n/10)%10), byte('0'+n%10))
	}
	return append(buf, byte('0'+n/10000), byte('0'+(n/1000)%10), byte('0'+(n/100)%10), byte('0'+(n/10)%10), byte('0'+n%10))
}

// onAccess handles domain access and updates heat tracking.
// Returns true if the domain should be added to the prefetch queue.
func (ht *heatTracker) onAccess(entry *cacheEntryExt, now time.Time) bool {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	key := makeKey(entry.domain, entry.qtype)
	ht.entries[key] = entry

	if entry.inPrefetchQueue {
		// Already in queue, just update heat score
		entry.heatScore++
		entry.lastAccessTime = now
		return false
	}

	// Cold-start phase
	if entry.firstAccessTime.IsZero() {
		// First access
		entry.firstAccessTime = now
		entry.accessCount = 1
		return false
	}

	// Check if still within time window
	elapsed := now.Sub(entry.firstAccessTime)
	if elapsed <= ht.timeWindow {
		// Within time window, increment access count
		entry.accessCount++

		// Check if reached threshold
		if entry.accessCount >= ht.minHeatThreshold {
			// Add to prefetch queue
			entry.inPrefetchQueue = true
			entry.heatScore = int64(entry.accessCount)
			entry.lastAccessTime = now
			return true
		}
	} else {
		// Exceeded time window, reset cold-start tracking
		entry.firstAccessTime = now
		entry.accessCount = 1
	}

	entry.lastAccessTime = now
	return false
}

// checkInactivity removes inactive domains from the prefetch queue.
// Returns the list of domains that were removed.
func (ht *heatTracker) checkInactivity(now time.Time) []string {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	var removed []string
	for key, entry := range ht.entries {
		if !entry.inPrefetchQueue {
			continue
		}

		// Check if inactive (no access for more than timeWindow)
		// Use >= to match the design spec (>180s means >=181s)
		interval := now.Sub(entry.lastAccessTime)
		if interval > ht.timeWindow {
			// Remove from prefetch queue
			entry.inPrefetchQueue = false
			entry.resetColdStart()
			removed = append(removed, key)
		}
	}

	return removed
}

// getPrefetchCandidates returns all entries that are in the prefetch queue.
func (ht *heatTracker) getPrefetchCandidates() []*cacheEntryExt {
	ht.mu.RLock()
	defer ht.mu.RUnlock()

	var candidates []*cacheEntryExt
	for _, entry := range ht.entries {
		if entry.inPrefetchQueue {
			candidates = append(candidates, entry)
		}
	}

	return candidates
}

// getEntry returns the tracked entry for the given domain and qtype.
func (ht *heatTracker) getEntry(domain string, qtype uint16) *cacheEntryExt {
	ht.mu.RLock()
	defer ht.mu.RUnlock()

	key := makeKey(domain, qtype)
	return ht.entries[key]
}

// removeEntry removes an entry from tracking.
func (ht *heatTracker) removeEntry(domain string, qtype uint16) {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	key := makeKey(domain, qtype)
	delete(ht.entries, key)
}
