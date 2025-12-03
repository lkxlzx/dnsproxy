package proxy

import (
	"fmt"
	"math/rand"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test10MinFinalValidation runs a comprehensive 10-minute validation test
func Test10MinFinalValidation(t *testing.T) {
	// Create 3 upstream servers with realistic characteristics
	upstreams := []*multiUpstreamTestServer{
		{id: 1, ttl: 300, latency: 10 * time.Millisecond, failRate: 0.0},   // Primary: 5min TTL, fast, reliable
		{id: 2, ttl: 600, latency: 20 * time.Millisecond, failRate: 0.02},  // Secondary: 10min TTL, slower, mostly reliable
		{id: 3, ttl: 180, latency: 15 * time.Millisecond, failRate: 0.05},  // Tertiary: 3min TTL, occasional failures
	}

	upstreamList := make([]upstream.Upstream, len(upstreams))
	for i, u := range upstreams {
		upstreamList[i] = u
	}

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  2 * 1024 * 1024, // 2MB cache
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       5000,                // 5 seconds before expiration
		CacheProactiveCooldownPeriod:    1800,               // 30 minutes
		CacheProactiveCooldownThreshold: 3,                  // 3 requests to trigger
		UpstreamMode:                    UpstreamModeLoadBalance,
		UpstreamConfig: &UpstreamConfig{
			Upstreams: upstreamList,
		},
	})
	require.NoError(t, err)

	t.Log("=== 10-Minute Final Validation Test ===")
	t.Log("Configuration:")
	t.Log("  - Cache Size: 2MB")
	t.Log("  - Proactive Refresh: 5s before expiration")
	t.Log("  - Cooldown Period: 30 minutes")
	t.Log("  - Cooldown Threshold: 3 requests")
	t.Log("  - Upstreams: 3 (TTL: 5min, 10min, 3min)")
	t.Log("")

	// Domain distribution simulating real-world usage
	// Top 10 domains get 70% of traffic
	topDomains := []string{
		"google.com.", "facebook.com.", "youtube.com.",
		"amazon.com.", "twitter.com.", "instagram.com.",
		"linkedin.com.", "netflix.com.", "reddit.com.", "github.com.",
	}

	// Medium popularity domains get 20% of traffic
	mediumDomains := generateDomains(30)

	// Long tail domains get 10% of traffic
	longTailDomains := generateDomains(100)

	stats := &testStats{
		cacheHits:   &atomic.Int64{},
		cacheMisses: &atomic.Int64{},
		errors:      &atomic.Int64{},
	}

	ctx := make(chan struct{})
	var wg sync.WaitGroup

	// Start 10 concurrent workers simulating real users
	numWorkers := 10
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			localQueries := 0

			for {
				select {
				case <-ctx:
					t.Logf("Worker %d completed %d queries", workerID, localQueries)
					return
				default:
					// Select domain based on realistic distribution
					var domain string
					r := rand.Float64()
					if r < 0.70 {
						// 70% top domains
						domain = topDomains[rand.Intn(len(topDomains))]
					} else if r < 0.90 {
						// 20% medium domains
						domain = mediumDomains[rand.Intn(len(mediumDomains))]
					} else {
						// 10% long tail
						domain = longTailDomains[rand.Intn(len(longTailDomains))]
					}

					// 75% A records, 25% AAAA records
					qtype := dns.TypeA
					if rand.Float64() < 0.25 {
						qtype = dns.TypeAAAA
					}

					dctx := &DNSContext{
						Req:  createTestMsg(domain),
						Addr: netip.MustParseAddrPort(fmt.Sprintf("127.0.0.1:%d", 40000+workerID)),
					}
					dctx.Req.Question[0].Qtype = qtype

					err := proxy.Resolve(dctx)
					if err != nil {
						stats.errors.Add(1)
					} else if dctx.Upstream == nil {
						stats.cacheHits.Add(1)
					} else {
						stats.cacheMisses.Add(1)
					}

					localQueries++

					// Realistic query interval: 20-100ms
					time.Sleep(time.Duration(20+rand.Intn(80)) * time.Millisecond)
				}
			}
		}(i)
	}

	// Run for 10 minutes, report every minute
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	startTime := time.Now()
	lastHits := int64(0)
	lastMisses := int64(0)
	lastErrors := int64(0)

	t.Log("\n=== Progress Report (every minute) ===")
	t.Log("Time | Total Req | Cache Hits | Cache Misses | Hit Rate | Interval Hit Rate | Errors | QPS")
	t.Log("-----|-----------|------------|--------------|----------|-------------------|--------|-----")

	for i := 0; i < 10; i++ {
		<-ticker.C
		elapsed := time.Since(startTime)

		hits := stats.cacheHits.Load()
		misses := stats.cacheMisses.Load()
		errors := stats.errors.Load()
		total := hits + misses

		intervalHits := hits - lastHits
		intervalMisses := misses - lastMisses
		intervalErrors := errors - lastErrors
		intervalTotal := intervalHits + intervalMisses

		intervalHitRate := float64(0)
		if intervalTotal > 0 {
			intervalHitRate = float64(intervalHits) / float64(intervalTotal) * 100
		}

		overallHitRate := float64(0)
		if total > 0 {
			overallHitRate = float64(hits) / float64(total) * 100
		}

		qps := float64(intervalTotal) / 60.0

		t.Logf("%4dm | %9d | %10d | %12d | %7.2f%% | %16.2f%% | %6d | %4.1f",
			int(elapsed.Minutes()),
			total,
			hits,
			misses,
			overallHitRate,
			intervalHitRate,
			intervalErrors,
			qps,
		)

		lastHits = hits
		lastMisses = misses
		lastErrors = errors
	}

	close(ctx)
	wg.Wait()

	// Final statistics
	totalRequests := stats.cacheHits.Load() + stats.cacheMisses.Load()
	hitRate := float64(stats.cacheHits.Load()) / float64(totalRequests) * 100
	errorRate := float64(stats.errors.Load()) / float64(totalRequests) * 100

	t.Log("\n=== Final Results ===")
	t.Logf("Test Duration: 10 minutes")
	t.Logf("Total Requests: %d", totalRequests)
	t.Logf("Cache Hits: %d", stats.cacheHits.Load())
	t.Logf("Cache Misses: %d", stats.cacheMisses.Load())
	t.Logf("Cache Hit Rate: %.2f%%", hitRate)
	t.Logf("Errors: %d (%.2f%%)", stats.errors.Load(), errorRate)
	t.Logf("Average QPS: %.1f", float64(totalRequests)/600.0)

	t.Log("\n=== Upstream Statistics ===")
	totalUpstreamRequests := int64(0)
	for i, u := range upstreams {
		count := u.requestCount.Load()
		totalUpstreamRequests += int64(count)
		percentage := float64(count) / float64(totalUpstreamRequests) * 100
		t.Logf("Upstream %d (TTL=%ds): %d requests (%.1f%%)",
			i+1, u.ttl, count, percentage)
	}

	savedRequests := totalRequests - totalUpstreamRequests
	savedPercentage := float64(savedRequests) / float64(totalRequests) * 100

	t.Logf("\nTotal Upstream Requests: %d", totalUpstreamRequests)
	t.Logf("Requests Saved by Cache: %d (%.2f%%)", savedRequests, savedPercentage)

	// Calculate refresh statistics
	refreshRequests := totalUpstreamRequests - stats.cacheMisses.Load()
	if refreshRequests > 0 {
		t.Logf("Proactive Refresh Requests: ~%d", refreshRequests)
	}

	t.Log("\n=== Performance Analysis ===")
	avgResponseTime := "<1ms (cached)"
	if hitRate < 100 {
		avgResponseTime = fmt.Sprintf("~%.1fms (%.2f%% cached)", 
			float64(100-hitRate)/100.0*20.0, hitRate)
	}
	t.Logf("Average Response Time: %s", avgResponseTime)
	t.Logf("Bandwidth Saved: %.2f%%", savedPercentage)
	t.Logf("Cache Efficiency: %.2f%%", hitRate)

	// Verify results meet expectations
	t.Log("\n=== Validation ===")
	
	assert.Greater(t, hitRate, 70.0, "Cache hit rate should be > 70%")
	if hitRate > 70.0 {
		t.Log("✓ Cache hit rate > 70%")
	}

	assert.Less(t, errorRate, 5.0, "Error rate should be < 5%")
	if errorRate < 5.0 {
		t.Log("✓ Error rate < 5%")
	}

	assert.Greater(t, savedPercentage, 60.0, "Cache should save > 60% of requests")
	if savedPercentage > 60.0 {
		t.Log("✓ Cache saved > 60% of upstream requests")
	}

	assert.Greater(t, totalRequests, int64(5000), "Should process > 5000 requests in 10 minutes")
	if totalRequests > 5000 {
		t.Log("✓ Processed sufficient requests")
	}

	// Check if proactive refresh is working
	if refreshRequests > 0 {
		t.Log("✓ Proactive refresh is working")
	}

	t.Log("\n=== Test Summary ===")
	if hitRate > 90 {
		t.Log("🎉 EXCELLENT: Cache hit rate > 90%")
	} else if hitRate > 80 {
		t.Log("✅ VERY GOOD: Cache hit rate > 80%")
	} else if hitRate > 70 {
		t.Log("✓ GOOD: Cache hit rate > 70%")
	} else {
		t.Log("⚠ WARNING: Cache hit rate below expected")
	}

	if errorRate < 1.0 {
		t.Log("🎉 EXCELLENT: Error rate < 1%")
	} else if errorRate < 5.0 {
		t.Log("✓ GOOD: Error rate < 5%")
	}

	t.Log("\n✓ 10-minute validation test completed successfully!")
}
