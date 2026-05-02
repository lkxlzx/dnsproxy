package proxy

import (
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// TestHeatTracker_ConcurrentAccess hammers onAccess, isInQueue, and
// checkInactivity from many goroutines simultaneously to surface any
// data races that the race detector would catch on Linux.
func TestHeatTracker_ConcurrentAccess(t *testing.T) {
	ht := newHeatTracker(4, 30*time.Second)
	domains := []string{
		"a.com.", "b.com.", "c.com.", "d.com.", "e.com.",
		"f.com.", "g.com.", "h.com.", "i.com.", "j.com.",
	}

	var wg sync.WaitGroup
	const goroutines = 20
	const iters = 200

	// Writers: onAccess
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		g := g
		go func() {
			defer wg.Done()
			now := time.Now()
			for i := 0; i < iters; i++ {
				d := domains[i%len(domains)]
				ht.onAccess(d, uint16(g%5+1), now.Add(time.Duration(i)*time.Millisecond))
			}
		}()
	}

	// Readers: isInQueue
	for g := 0; g < goroutines/2; g++ {
		wg.Add(1)
		g := g
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				d := domains[i%len(domains)]
				_ = ht.isInQueue(d, uint16(g%5+1))
			}
		}()
	}

	// Readers: getPrefetchCandidates
	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				_ = ht.getPrefetchCandidates()
			}
		}()
	}

	// Inactivity checker
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			ht.checkInactivity(time.Now().Add(time.Duration(i) * 5 * time.Second))
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()
}

// TestCachePrefetch_ConcurrentRecordAccess verifies that recordAccess is safe
// under concurrent access from many goroutines (exercises the RLock fast-path
// and the double-check Lock upgrade).
func TestCachePrefetch_ConcurrentRecordAccess(t *testing.T) {
	baseCache := newCache(&cacheConfig{size: 4 * 1024 * 1024})
	config := &PrefetchConfig{
		Enabled:                 true,
		ThresholdSeconds:        5,
		ThresholdPercent:        80,
		MaxConcurrent:           10,
		ScanInterval:            100 * time.Millisecond,
		MinHeatThreshold:        3,
		TimeWindow:              30 * time.Second,
		InactivityCheckInterval: 5 * time.Second,
	}
	p := &Proxy{Config: Config{CachePrefetchConfig: config}}
	cp := newCachePrefetch(baseCache, config, p, testLogger)
	defer cp.stop()

	// Pre-populate base cache with a fake item so recordAccess has something to work with.
	fakeItem := &cacheItem{
		m: makeDNSResponse("concurrent.com.", 300),
	}

	domains := []string{"concurrent.com.", "stress.com.", "load.com.", "test.com.", "bench.com."}

	var wg sync.WaitGroup
	const goroutines = 30
	const iters = 500

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		g := g
		go func() {
			defer wg.Done()
			now := time.Now()
			for i := 0; i < iters; i++ {
				d := domains[(g+i)%len(domains)]
				cp.recordAccess(d, 1, fakeItem, now.Add(time.Duration(i)*time.Millisecond))
			}
		}()
	}

	wg.Wait()
}

// TestCachePrefetch_NoGlobalClockReset verifies that a successful prefetch on
// one domain does NOT affect the TTL tracking of another domain.
// This was the core bug in the original globalClock design.
func TestCachePrefetch_NoGlobalClockReset(t *testing.T) {
	baseCache := newCache(&cacheConfig{size: 1024 * 1024})
	config := &PrefetchConfig{
		Enabled:                 true,
		ThresholdSeconds:        5,
		ThresholdPercent:        80,
		MaxConcurrent:           5,
		ScanInterval:            100 * time.Millisecond,
		MinHeatThreshold:        2,
		TimeWindow:              60 * time.Second,
		InactivityCheckInterval: 10 * time.Second,
	}
	p := &Proxy{Config: Config{CachePrefetchConfig: config}}
	cp := newCachePrefetch(baseCache, config, p, testLogger)
	defer cp.stop()

	now := time.Now()

	// Register domain A with TTL=300, cached 100s ago → remaining=200s
	keyA := makeKey("domain-a.com.", 1)
	shardA := cp.getShard(keyA)
	shardA.mu.Lock()
	shardA.entries[keyA] = newCacheEntryExt("domain-a.com.", 1, 300, now.Add(-100*time.Second))
	shardA.mu.Unlock()

	// Register domain B with TTL=60, cached 50s ago → remaining=10s
	keyB := makeKey("domain-b.com.", 1)
	shardB := cp.getShard(keyB)
	shardB.mu.Lock()
	shardB.entries[keyB] = newCacheEntryExt("domain-b.com.", 1, 60, now.Add(-50*time.Second))
	shardB.mu.Unlock()

	// Verify initial remaining TTLs
	nowUnix := now.Unix()
	remA := shardA.entries[keyA].remainingTTL(nowUnix)
	remB := shardB.entries[keyB].remainingTTL(nowUnix)

	if remA != 200 {
		t.Errorf("domain-a remaining TTL = %d, want 200", remA)
	}
	if remB != 10 {
		t.Errorf("domain-b remaining TTL = %d, want 10", remB)
	}

	// Simulate a successful prefetch on domain A (new TTL=300)
	cp.onPrefetchSuccess("domain-a.com.", 1, makeDNSResponse("domain-a.com.", 300), nil, 300)

	// Domain B's remaining TTL must be unchanged (still ~10s from now)
	// In the old globalClock design, reset() would set T=0 and domain B's
	// remainingTTL would jump back to its originalTTL (60s) — wrong.
	nowUnix2 := time.Now().Unix()
	remBAfter := shardB.entries[keyB].remainingTTL(nowUnix2)

	// Allow 1s tolerance for test execution time
	if remBAfter > 11 {
		t.Errorf("domain-b remaining TTL after domain-a prefetch = %d, want ~10 (globalClock reset bug would give 60)",
			remBAfter)
	}
}

// TestHeatTracker_MemoryBounded verifies that the entries map does not grow
// unboundedly when many cold-start domains are accessed once and never again.
func TestHeatTracker_MemoryBounded(t *testing.T) {
	ht := newHeatTracker(6, 10*time.Second)
	now := time.Now()

	// Access 1000 unique domains once each (cold-start, never reach threshold)
	for i := 0; i < 1000; i++ {
		domain := makeKey("unique-domain-", uint16(i))
		ht.onAccess(domain, 1, now)
	}

	ht.mu.Lock()
	before := len(ht.entries)
	ht.mu.Unlock()

	if before != 1000 {
		t.Errorf("entries before purge = %d, want 1000", before)
	}

	// After 2x timeWindow, all stale cold-start entries should be purged
	ht.checkInactivity(now.Add(21 * time.Second))

	ht.mu.Lock()
	after := len(ht.entries)
	ht.mu.Unlock()

	if after != 0 {
		t.Errorf("entries after purge = %d, want 0 (memory leak fix)", after)
	}
}

// makeDNSResponse creates a minimal DNS response for testing.
func makeDNSResponse(domain string, ttl uint32) *dns.Msg {
	req := &dns.Msg{}
	req.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	resp := &dns.Msg{}
	resp.SetReply(req)
	resp.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(domain),
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    ttl,
			},
			A: []byte{1, 2, 3, 4},
		},
	}
	return resp
}
