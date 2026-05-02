package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// BenchmarkCacheGet benchmarks cache get operations
func BenchmarkCacheGet(b *testing.B) {
	// Create a basic cache
	baseCache := newCache(&cacheConfig{
		size:       1024 * 1024,
		optimistic: false,
	})

	// Prepare test data
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

	// Store in cache
	baseCache.set(resp, nil, testLogger)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _, _ = baseCache.get(req)
	}
}

// BenchmarkCacheGetWithPrefetch benchmarks cache get with prefetch enabled
func BenchmarkCacheGetWithPrefetch(b *testing.B) {
	// Create cache with prefetch
	baseCache := newCache(&cacheConfig{
		size:       1024 * 1024,
		optimistic: false,
	})

	config := &PrefetchConfig{
		Enabled:                 true,
		ThresholdSeconds:        5,
		ThresholdPercent:        80,
		MaxConcurrent:           5,
		ScanInterval:            1 * time.Second,
		MinHeatThreshold:        3,
		TimeWindow:              180 * time.Second,
		InactivityCheckInterval: 10 * time.Second,
	}

	// Create minimal proxy for testing
	proxy := &Proxy{
		Config: Config{
			CachePrefetchConfig: config,
		},
	}

	cachePrefetch := newCachePrefetch(baseCache, config, proxy, testLogger)

	// Prepare test data
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

	// Store in cache
	cachePrefetch.set(resp, nil, testLogger)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _, _ = cachePrefetch.get(req)
	}

	cachePrefetch.stop()
}

// BenchmarkCacheSet benchmarks cache set operations
func BenchmarkCacheSet(b *testing.B) {
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

// BenchmarkCacheSetWithPrefetch benchmarks cache set with prefetch enabled
func BenchmarkCacheSetWithPrefetch(b *testing.B) {
	baseCache := newCache(&cacheConfig{
		size:       1024 * 1024,
		optimistic: false,
	})

	config := &PrefetchConfig{
		Enabled:                 true,
		ThresholdSeconds:        5,
		ThresholdPercent:        80,
		MaxConcurrent:           5,
		ScanInterval:            1 * time.Second,
		MinHeatThreshold:        3,
		TimeWindow:              180 * time.Second,
		InactivityCheckInterval: 10 * time.Second,
	}

	proxy := &Proxy{
		Config: Config{
			CachePrefetchConfig: config,
		},
	}

	cachePrefetch := newCachePrefetch(baseCache, config, proxy, testLogger)

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

		cachePrefetch.set(resp, nil, testLogger)
	}

	cachePrefetch.stop()
}

// BenchmarkHeatTrackerOnAccess benchmarks heat tracker access recording
func BenchmarkHeatTrackerOnAccess(b *testing.B) {
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

// BenchmarkGlobalClockGet benchmarks global clock read operations
func BenchmarkGlobalClockGet(b *testing.B) {
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

// BenchmarkPrefetchSchedulerShouldPrefetch benchmarks prefetch decision logic
func BenchmarkPrefetchSchedulerShouldPrefetch(b *testing.B) {
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

// BenchmarkConcurrentCacheAccess benchmarks concurrent cache access
func BenchmarkConcurrentCacheAccess(b *testing.B) {
	baseCache := newCache(&cacheConfig{
		size:       1024 * 1024,
		optimistic: false,
	})

	// Prepare test data
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
