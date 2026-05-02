package proxy

import (
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
}

// heatTracker tracks domain access heat and manages the prefetch queue.
// Uses sync.RWMutex so concurrent reads (isInQueue, getPrefetchCandidates)
// do not block each other.
type heatTracker struct {
	minHeatThreshold int
	timeWindow       time.Duration

	mu      sync.RWMutex
	entries map[string]*heatEntry
}

func newHeatTracker(minHeatThreshold int, timeWindow time.Duration) *heatTracker {
	return &heatTracker{
		minHeatThreshold: minHeatThreshold,
		timeWindow:       timeWindow,
		entries:          make(map[string]*heatEntry),
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
func (ht *heatTracker) onAccess(domain string, qtype uint16, now time.Time) bool {
	key := makeKey(domain, qtype)

	ht.mu.Lock()
	defer ht.mu.Unlock()

	e, ok := ht.entries[key]
	if !ok {
		e = &heatEntry{}
		ht.entries[key] = e
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

// isInQueue reports whether domain+qtype is currently in the prefetch queue.
// Uses RLock because it is a read-only operation.
func (ht *heatTracker) isInQueue(domain string, qtype uint16) bool {
	key := makeKey(domain, qtype)
	ht.mu.RLock()
	e, ok := ht.entries[key]
	ht.mu.RUnlock()
	return ok && e.inPrefetchQueue
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

// checkInactivity evicts queue entries that have been inactive for longer than
// timeWindow, and purges stale cold-start entries to prevent unbounded growth.
func (ht *heatTracker) checkInactivity(now time.Time) (removed []string) {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	for key, e := range ht.entries {
		if e.inPrefetchQueue {
			// Inactivity check: inactive if last access >= timeWindow ago (inclusive boundary)
			if now.Sub(e.lastAccessTime) >= ht.timeWindow {
				// Completely delete the entry to prevent memory leak
				delete(ht.entries, key)
				removed = append(removed, key)
			}
			continue
		}
		// Purge stale cold-start entries (never reached threshold).
		if !e.firstAccessTime.IsZero() && now.Sub(e.firstAccessTime) >= ht.timeWindow*2 {
			delete(ht.entries, key)
		}
	}
	return removed
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
