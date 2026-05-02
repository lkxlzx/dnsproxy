package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// BenchmarkBasicCacheGet benchmarks basic cache get operations
func BenchmarkBasicCacheGet(b *testing.B) {
	baseCache := newCache(&cacheConfig{
		size:       1024 * 1024,
		optimistic: false,
	})

	req := &dns.Msg{}
	req.SetQuestion("example.com.", dns.TypeA)

	resp := &dns.Msg{}
	resp.SetReply(req)
	resp.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   "example.com.",
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    300,
			},
			A: []byte{1, 2, 3, 4},
		},
	}

	baseCache.set(resp, nil, testLogger)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _, _ = baseCache.get(req)
	}
}

// BenchmarkBasicCacheSet benchmarks basic cache set operations
func BenchmarkBasicCacheSet(b *testing.B) {
	baseCache := newCache(&cacheConfig{
		size:       1024 * 1024,
		optimistic: false,
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := &dns.Msg{}
		req.SetQuestion("example.com.", dns.TypeA)

		resp := &dns.Msg{}
		resp.SetReply(req)
		resp.Answer = []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   "example.com.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    300,
				},
				A: []byte{1, 2, 3, 4},
			},
		}

		baseCache.set(resp, nil, testLogger)
	}
}

// BenchmarkHeatTrackerAccess benchmarks heat tracker operations
func BenchmarkHeatTrackerAccess(b *testing.B) {
	tracker := newHeatTracker(6, 180*time.Second)

	entry := &cacheEntryExt{
		domain: "example.com.",
		qtype:  dns.TypeA,
	}

	now := time.Now()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tracker.onAccess(entry, now)
	}
}

// BenchmarkGlobalClockRead benchmarks global clock read operations
func BenchmarkGlobalClockRead(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clock := newGlobalClock()
	clock.start(ctx)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = clock.get()
	}
}

// BenchmarkCacheEntryExtIsExpired benchmarks expiration check
func BenchmarkCacheEntryExtIsExpired(b *testing.B) {
	entry := &cacheEntryExt{
		expiresAt: 1000,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = entry.isExpired(500)
	}
}

// BenchmarkCacheEntryExtRemainingTTL benchmarks TTL calculation
func BenchmarkCacheEntryExtRemainingTTL(b *testing.B) {
	entry := &cacheEntryExt{
		expiresAt: 1000,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = entry.remainingTTL(500)
	}
}

// BenchmarkPrefetchShouldPrefetch benchmarks prefetch decision
func BenchmarkPrefetchShouldPrefetch(b *testing.B) {
	config := DefaultPrefetchConfig()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	clock := newGlobalClock()
	clock.start(ctx)

	tracker := newHeatTracker(config.MinHeatThreshold, config.TimeWindow)

	proxy := &Proxy{
		Config: Config{
			CachePrefetchConfig: config,
		},
	}

	scheduler := newPrefetchScheduler(config, proxy, clock, tracker, testLogger)

	entry := &cacheEntryExt{
		domain:          "example.com.",
		qtype:           dns.TypeA,
		expiresAt:       100,
		originalTTL:     300,
		inPrefetchQueue: true,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = scheduler.shouldPrefetch(entry, 50)
	}
}

// BenchmarkConcurrentCacheRead benchmarks concurrent cache reads
func BenchmarkConcurrentCacheRead(b *testing.B) {
	baseCache := newCache(&cacheConfig{
		size:       1024 * 1024,
		optimistic: false,
	})

	req := &dns.Msg{}
	req.SetQuestion("example.com.", dns.TypeA)

	resp := &dns.Msg{}
	resp.SetReply(req)
	resp.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   "example.com.",
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    300,
			},
			A: []byte{1, 2, 3, 4},
		},
	}

	baseCache.set(resp, nil, testLogger)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = baseCache.get(req)
		}
	})
}
