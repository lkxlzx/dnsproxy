package proxy

import (
	"container/list"
	"sync"
	"time"
)

// heatEntry holds heat-tracking metadata for a single domain+qtype.
// Owned exclusively by heatTracker and protected by heatTracker.mu.
type heatEntry struct {
	firstAccessTime time.Time
	lastAccessTime  time.Time
	accessCount     int
	inPrefetchQueue bool
	heatScore       int64
	lruElement      *list.Element // Pointer to LRU list element
}

// heatTracker tracks domain access heat and manages the prefetch queue.
// Uses LRU eviction to automatically limit memory usage.
// Performs lazy cleanup of inactive entries on access.
type heatTracker struct {
	minHeatThreshold int
	timeWindow       time.Duration
	maxEntries       int // Maximum number of entries to track

	mu      sync.RWMutex
	entries map[string]*heatEntry
	lruList *list.List // LRU list for automatic eviction
}

func newHeatTracker(minHeatThreshold int, timeWindow time.Duration) *heatTracker {
	return &heatTracker{
		minHeatThreshold: minHeatThreshold,
		timeWindow:       timeWindow,
		maxEntries:       10000, // Default: track up to 10k domains
		entries:          make(map[string]*heatEntry),
		lruList:          list.New(),
	}
}

// makeKey creates "domain:qtype" using decimal qtype representation.
func makeKey(domain string, qtype uint16) string {
	buf := make([]byte, 0, len(domain)+6)
	buf = append(buf, domain...)
	buf = append(buf, ':')
	return string(appendUint16(buf, qtype))
}

// appendUint16 appends the decimal representation of n to buf.
func appendUint16(buf []byte, n uint16) []byte {
	switch {
	case n < 10:
		return append(buf, byte('0'+n))
	case n < 100:
		return append(buf, byte('0'+n/10), byte('0'+n%10))
	case n < 1000:
		return append(buf, byte('0'+n/100), byte('0'+(n/10)%10), byte('0'+n%10))
	case n < 10000:
		return append(buf, byte('0'+n/1000), byte('0'+(n/100)%10), byte('0'+(n/10)%10), byte('0'+n%10))
	default:
		return append(buf, byte('0'+n/10000), byte('0'+(n/1000)%10), byte('0'+(n/100)%10), byte('0'+(n/10)%10), byte('0'+n%10))
	}
}

// onAccess records an access for domain+qtype.
// Returns true if the domain just crossed the heat threshold and entered
// the prefetch queue for the first time.
// Performs lazy cleanup and LRU eviction automatically.
func (ht *heatTracker) onAccess(domain string, qtype uint16, now time.Time) bool {
	key := makeKey(domain, qtype)

	ht.mu.Lock()
	defer ht.mu.Unlock()

	e, ok := ht.entries[key]
	if !ok {
		// Check if we need to evict (LRU)
		if len(ht.entries) >= ht.maxEntries {
			ht.evictOldest()
		}

		// Create new entry
		e = &heatEntry{}
		ht.entries[key] = e
		e.lruElement = ht.lruList.PushFront(key)
	} else {
		// Move to front of LRU list (most recently used)
		ht.lruList.MoveToFront(e.lruElement)

		// Lazy cleanup: check if entry is inactive
		if e.inPrefetchQueue && now.Sub(e.lastAccessTime) >= ht.timeWindow {
			// Entry has been inactive, reset it
			e.inPrefetchQueue = false
			e.heatScore = 0
			e.accessCount = 0
			e.firstAccessTime = time.Time{}
		}
	}

	if e.inPrefetchQueue {
		e.heatScore++
		e.lastAccessTime = now
		return false
	}

	// Cold-start phase: first access initialises the window.
	if e.firstAccessTime.IsZero() {
		e.firstAccessTime = now
		e.accessCount = 1
		e.lastAccessTime = now
		return false
	}

	// Cold-start phase: check if within time window (exclusive boundary)
	if now.Sub(e.firstAccessTime) < ht.timeWindow {
		e.accessCount++
		e.lastAccessTime = now
		if e.accessCount >= ht.minHeatThreshold {
			e.inPrefetchQueue = true
			e.heatScore = int64(e.accessCount)
			return true
		}
	} else {
		// Time window expired — restart cold-start.
		e.firstAccessTime = now
		e.accessCount = 1
		e.lastAccessTime = now
	}
	return false
}

// evictOldest removes the least recently used entry.
// Must be called with ht.mu locked.
func (ht *heatTracker) evictOldest() {
	if ht.lruList.Len() == 0 {
		return
	}

	// Remove from back of LRU list (least recently used)
	oldest := ht.lruList.Back()
	if oldest != nil {
		key := oldest.Value.(string)
		ht.lruList.Remove(oldest)
		delete(ht.entries, key)
	}
}

// isInQueue reports whether domain+qtype is currently in the prefetch queue.
// Uses RLock because it is a read-only operation.
// Performs lazy cleanup if entry is inactive.
func (ht *heatTracker) isInQueue(domain string, qtype uint16) bool {
	key := makeKey(domain, qtype)
	ht.mu.RLock()
	e, ok := ht.entries[key]
	if !ok {
		ht.mu.RUnlock()
		return false
	}

	// Lazy cleanup check
	if e.inPrefetchQueue && time.Since(e.lastAccessTime) >= ht.timeWindow {
		ht.mu.RUnlock()
		// Upgrade to write lock for cleanup
		ht.mu.Lock()
		// Double-check after acquiring write lock
		if e.inPrefetchQueue && time.Since(e.lastAccessTime) >= ht.timeWindow {
			e.inPrefetchQueue = false
			e.heatScore = 0
		}
		result := e.inPrefetchQueue
		ht.mu.Unlock()
		return result
	}

	result := e.inPrefetchQueue
	ht.mu.RUnlock()
	return result
}

// heatSnapshot is a value-type point-in-time copy used by the scheduler.
// Returning value types (not pointers) avoids exposing internal state.
type heatSnapshot struct {
	domain    string
	qtype     uint16
	heatScore int64
}

// getPrefetchCandidates returns value snapshots of all queued entries.
// Returns a new slice to avoid exposing internal state.
func (ht *heatTracker) getPrefetchCandidates() []heatSnapshot {
	ht.mu.RLock()
	defer ht.mu.RUnlock()

	// Directly allocate result slice, no need for extra lock or buffer
	result := make([]heatSnapshot, 0, len(ht.entries))

	for key, e := range ht.entries {
		if e.inPrefetchQueue {
			d, q := splitKey(key)
			result = append(result, heatSnapshot{
				domain:    d,
				qtype:     q,
				heatScore: e.heatScore,
			})
		}
	}

	return result
}

// splitKey reverses makeKey: "domain:qtype" -> (domain, qtype).
func splitKey(key string) (domain string, qtype uint16) {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == ':' {
			domain = key[:i]
			q := uint16(0)
			for _, c := range key[i+1:] {
				q = q*10 + uint16(c-'0')
			}
			return domain, q
		}
	}
	return key, 0
}
