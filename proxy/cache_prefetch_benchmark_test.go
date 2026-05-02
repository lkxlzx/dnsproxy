package proxy

import (
	"testing"
	"time"

	"github.com/miekg/dns"
)

func BenchmarkCacheGet(b *testing.B) {
	c := newCache(&cacheConfig{size: 1024 * 1024})
	req := &dns.Msg{}
	req.SetQuestion("example.com.", dns.TypeA)
	c.set(makeTestResp(req, 300), nil, testLogger)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _ = c.get(req)
	}
}

func BenchmarkCacheGetWithPrefetch(b *testing.B) {
	baseCache := newCache(&cacheConfig{size: 1024 * 1024})
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
	p := &Proxy{Config: Config{CachePrefetchConfig: config}}
	cp := newCachePrefetch(baseCache, config, p, testLogger)
	defer cp.stop()

	req := &dns.Msg{}
	req.SetQuestion("example.com.", dns.TypeA)
	cp.set(makeTestResp(req, 300), nil, testLogger)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _ = cp.get(req)
	}
}

func BenchmarkCacheSet(b *testing.B) {
	c := newCache(&cacheConfig{size: 1024 * 1024})
	req := &dns.Msg{}
	req.SetQuestion("example.com.", dns.TypeA)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.set(makeTestResp(req, 300), nil, testLogger)
	}
}

func BenchmarkCacheSetWithPrefetch(b *testing.B) {
	baseCache := newCache(&cacheConfig{size: 1024 * 1024})
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
	p := &Proxy{Config: Config{CachePrefetchConfig: config}}
	cp := newCachePrefetch(baseCache, config, p, testLogger)
	defer cp.stop()

	req := &dns.Msg{}
	req.SetQuestion("example.com.", dns.TypeA)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cp.set(makeTestResp(req, 300), nil, testLogger)
	}
}

func BenchmarkPrefetchSchedulerShouldPrefetch(b *testing.B) {
	config := DefaultPrefetchConfig()
	ht := newHeatTracker(config.MinHeatThreshold, config.TimeWindow)
	baseCache := newCache(&cacheConfig{size: 1024 * 1024})
	p := &Proxy{Config: Config{CachePrefetchConfig: config}}
	cp := newCachePrefetch(baseCache, config, p, testLogger)
	defer cp.stop()

	now := time.Now()
	key := makeKey("example.com.", dns.TypeA)
	shard := cp.getShard(key)
	shard.mu.Lock()
	shard.entries[key] = newCacheEntryExt("example.com.", dns.TypeA, 300, now)
	shard.mu.Unlock()

	_ = ht

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = cp.shouldPrefetchDomain("example.com.", dns.TypeA)
	}
}
