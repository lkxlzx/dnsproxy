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

// TestQuickStress runs a quick comprehensive stress test (2 minutes total)
func TestQuickStress(t *testing.T) {
	// Create 3 upstream servers
	upstreams := []*multiUpstreamTestServer{
		{id: 1, ttl: 5, latency: 5 * time.Millisecond, failRate: 0.0},
		{id: 2, ttl: 10, latency: 10 * time.Millisecond, failRate: 0.05},
		{id: 3, ttl: 3, latency: 8 * time.Millisecond, failRate: 0.02},
	}

	upstreamList := make([]upstream.Upstream, len(upstreams))
	for i, u := range upstreams {
		upstreamList[i] = u
	}

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  512 * 1024,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       1000,
		CacheProactiveCooldownPeriod:    30,
		CacheProactiveCooldownThreshold: 3,
		UpstreamMode:                    UpstreamModeLoadBalance,
		UpstreamConfig: &UpstreamConfig{
			Upstreams: upstreamList,
		},
	})
	require.NoError(t, err)

	t.Log("=== Quick Stress Test (2 minutes) ===")

	// Mix of hot and cold domains
	hotDomains := []string{"google.com.", "facebook.com.", "youtube.com."}
	coldDomains := generateDomains(20)
	allDomains := append(hotDomains, coldDomains...)

	stats := &testStats{
		cacheHits:   &atomic.Int64{},
		cacheMisses: &atomic.Int64{},
		errors:      &atomic.Int64{},
	}

	ctx := make(chan struct{})
	var wg sync.WaitGroup

	// Start 8 concurrent workers
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-ctx:
					return
				default:
					// 60% hot domains, 40% cold domains
					var domain string
					if rand.Float64() < 0.6 {
						domain = hotDomains[rand.Intn(len(hotDomains))]
					} else {
						domain = allDomains[rand.Intn(len(allDomains))]
					}

					// 70% A records, 30% AAAA records
					qtype := dns.TypeA
					if rand.Float64() < 0.3 {
						qtype = dns.TypeAAAA
					}

					dctx := &DNSContext{
						Req:  createTestMsg(domain),
						Addr: netip.MustParseAddrPort(fmt.Sprintf("127.0.0.1:%d", 30000+workerID)),
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

					time.Sleep(time.Duration(10+rand.Intn(30)) * time.Millisecond)
				}
			}
		}(i)
	}

	// Run for 2 minutes, report every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()
	lastHits := int64(0)
	lastMisses := int64(0)

	for i := 0; i < 4; i++ {
		<-ticker.C
		elapsed := time.Since(startTime)

		hits := stats.cacheHits.Load()
		misses := stats.cacheMisses.Load()
		total := hits + misses

		intervalHits := hits - lastHits
		intervalMisses := misses - lastMisses
		intervalTotal := intervalHits + intervalMisses
		intervalHitRate := float64(0)
		if intervalTotal > 0 {
			intervalHitRate = float64(intervalHits) / float64(intervalTotal) * 100
		}

		t.Logf("[%v] Total: %d, Hits: %d, Misses: %d, Hit Rate: %.2f%%, Interval Hit Rate: %.2f%%",
			elapsed.Round(time.Second),
			total,
			hits,
			misses,
			float64(hits)/float64(total)*100,
			intervalHitRate,
		)

		lastHits = hits
		lastMisses = misses
	}

	close(ctx)
	wg.Wait()

	// Final statistics
	totalRequests := stats.cacheHits.Load() + stats.cacheMisses.Load()
	hitRate := float64(stats.cacheHits.Load()) / float64(totalRequests) * 100

	t.Log("\n=== Final Results ===")
	t.Logf("Total Requests: %d", totalRequests)
	t.Logf("Cache Hits: %d", stats.cacheHits.Load())
	t.Logf("Cache Misses: %d", stats.cacheMisses.Load())
	t.Logf("Cache Hit Rate: %.2f%%", hitRate)
	t.Logf("Errors: %d (%.2f%%)", stats.errors.Load(),
		float64(stats.errors.Load())/float64(totalRequests)*100)

	totalUpstreamRequests := int64(0)
	for i, u := range upstreams {
		count := u.requestCount.Load()
		totalUpstreamRequests += int64(count)
		t.Logf("Upstream %d: %d requests", i+1, count)
	}

	savedRequests := totalRequests - totalUpstreamRequests
	t.Logf("Requests saved by cache: %d (%.2f%%)",
		savedRequests,
		float64(savedRequests)/float64(totalRequests)*100)

	// Verify results
	assert.Greater(t, hitRate, 50.0, "Cache hit rate should be > 50%")
	assert.Less(t, stats.errors.Load(), totalRequests/20, "Error rate should be < 5%")
	assert.Greater(t, savedRequests, totalRequests/2, "Cache should save > 50% of requests")

	t.Log("\n✓ Quick stress test passed!")
}
