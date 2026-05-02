package proxy

import (
"sync"
"time"
)

// heatEntry holds heat-tracking metadata for a single domain+qtype.
type heatEntry struct {
firstAccessTime time.Time
lastAccessTime  time.Time
accessCount     int
inPrefetchQueue bool
heatScore       int64
}

// heatTracker tracks domain access heat and manages the prefetch queue.
// It owns its own data and is protected by a single mutex, eliminating the
// previous double-lock pattern.
type heatTracker struct {
minHeatThreshold int
timeWindow       time.Duration

mu      sync.Mutex
entries map[string]*heatEntry
}

func newHeatTracker(minHeatThreshold int, timeWindow time.Duration) *heatTracker {
return &heatTracker{
minHeatThreshold: minHeatThreshold,
timeWindow:       timeWindow,
entries:          make(map[string]*heatEntry),
}
}

// makeKey creates a unique string key for domain + query type.
func makeKey(domain string, qtype uint16) string {
buf := make([]byte, 0, len(domain)+6)
buf = append(buf, domain...)
buf = append(buf, ':')
buf = appendUint16(buf, qtype)
return string(buf)
}

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

if e.firstAccessTime.IsZero() {
e.firstAccessTime = now
e.accessCount = 1
e.lastAccessTime = now
return false
}

elapsed := now.Sub(e.firstAccessTime)
if elapsed <= ht.timeWindow {
e.accessCount++
e.lastAccessTime = now
if e.accessCount >= ht.minHeatThreshold {
e.inPrefetchQueue = true
e.heatScore = int64(e.accessCount)
return true
}
} else {
e.firstAccessTime = now
e.accessCount = 1
e.lastAccessTime = now
}
return false
}

// heatSnapshot is a point-in-time view used by the scheduler.
type heatSnapshot struct {
domain    string
qtype     uint16
heatScore int64
}

func (ht *heatTracker) getPrefetchCandidates() []heatSnapshot {
ht.mu.Lock()
defer ht.mu.Unlock()

var out []heatSnapshot
for key, e := range ht.entries {
if !e.inPrefetchQueue {
continue
}
d, q := splitKey(key)
out = append(out, heatSnapshot{domain: d, qtype: q, heatScore: e.heatScore})
}
return out
}

func (ht *heatTracker) checkInactivity(now time.Time) (removed []string) {
ht.mu.Lock()
defer ht.mu.Unlock()

for key, e := range ht.entries {
if e.inPrefetchQueue {
if now.Sub(e.lastAccessTime) > ht.timeWindow {
e.inPrefetchQueue = false
e.firstAccessTime = time.Time{}
e.accessCount = 0
e.heatScore = 0
removed = append(removed, key)
}
continue
}
// Purge stale cold-start entries to prevent unbounded memory growth
if !e.firstAccessTime.IsZero() && now.Sub(e.firstAccessTime) > ht.timeWindow*2 {
delete(ht.entries, key)
}
}
return removed
}

// splitKey reverses makeKey.
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

// isInQueue reports whether domain+qtype is in the prefetch queue.
func (ht *heatTracker) isInQueue(domain string, qtype uint16) bool {
key := makeKey(domain, qtype)
ht.mu.Lock()
e, ok := ht.entries[key]
ht.mu.Unlock()
return ok && e.inPrefetchQueue
}
