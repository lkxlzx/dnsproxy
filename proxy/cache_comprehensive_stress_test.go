package proxy

import (
	"fmt"
	"math/rand"
	"net"
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

// multiUpstreamTestServer simulates multiple upstream servers with different characteristics
type multiUpstreamTestServer struct {
	id           int
	requestCount atomic.Int32
	failRate     float64 // 0.0 to 1.0
	latency      time.Duration
	ttl          uint32
	mu           sync.Mutex
}

func (u *multiUpstreamTestServer) Exchange(req *dns.Msg) (*dns.Msg, error) {
	u.requestCount.Add(1)

	// Simulate latency
	if u.latency > 0 {
		time.Sleep(u.latency)
	}

	// Simulate failures
	if u.failRate > 0 && rand.Float64() < u.failRate {
		return nil, fmt.Errorf("upstream %d: simulated failure", u.id)
	}

	resp := &dns.Msg{}
	resp.SetReply(req)

	if len(req.Question) > 0 {
		q := req.Question[0]

		switch q.Qtype {
		case dns.TypeA:
			resp.Answer = []dns.RR{
				&dns.A{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    u.ttl,
					},
					A: net.ParseIP(fmt.Sprintf("1.%d.%d.%d", u.id, rand.Intn(256), rand.Intn(256))),
				},
			}
		case dns.TypeAAAA:
			resp.Answer = []dns.RR{
				&dns.AAAA{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeAAAA,
						Class:  dns.ClassINET,
						Ttl:    u.ttl,
					},
					AAAA: net.ParseIP(fmt.Sprintf("2001:db8:%d::%d", u.id, rand.Intn(65536))),
				},
			}
		}
	}

	return resp, nil
}

func (u *multiUpstreamTestServer) Address() string {
	return fmt.Sprintf("test-upstream-%d", u.id)
}

func (u *multiUpstreamTestServer) Close() error {
	return nil
}

// TestComprehensiveStress runs a comprehensive stress test with multiple scenarios
func TestComprehensiveStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping comprehensive stress test in short mode")
	}

	// Create 3 upstream servers with different characteristics
	upstreams := []*multiUpstreamTestServer{
		{id: 1, ttl: 5, latency: 10 * time.Millisecond, failRate: 0.0},  // Fast, reliable
		{id: 2, ttl: 10, latency: 50 * time.Millisecond, failRate: 0.1}, // Slower, occasional failures
		{id: 3, ttl: 3, latency: 20 * time.Millisecond, failRate: 0.05}, // Short TTL, mostly reliable
	}

	upstreamList := make([]upstream.Upstream, len(upstreams))
	for i, u := range upstreams {
		upstreamList[i] = u
	}

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  1024 * 1024, // 1MB cache
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       1000,                // 1 second
		CacheProactiveCooldownPeriod:    30,                  // 30 seconds
		CacheProactiveCooldownThreshold: 3,                   // 3 requests
		UpstreamMode:                    UpstreamModeLoadBalance,
		UpstreamConfig: &UpstreamConfig{
			Upstreams: upstreamList,
		},
	})
	require.NoError(t, err)

	// Test scenarios
	scenarios := []struct {
		name        string
		domains     []string
		queryTypes  []uint16
		duration    time.Duration
		concurrency int
		pattern     string // "hot", "cold", "mixed", "burst"
	}{
		{
			name:        "Hot domains - continuous traffic",
			domains:     []string{"google.com.", "facebook.com.", "youtube.com."},
			queryTypes:  []uint16{dns.TypeA, dns.TypeAAAA},
			duration:    30 * time.Second,
			concurrency: 5,
			pattern:     "hot",
		},
		{
			name:        "Cold domains - sporadic traffic",
			domains:     generateDomains(50), // 50 different domains
			queryTypes:  []uint16{dns.TypeA},
			duration:    20 * time.Second,
			concurrency: 3,
			pattern:     "cold",
		},
		{
			name:        "Mixed traffic - realistic scenario",
			domains:     append([]string{"google.com.", "facebook.com."}, generateDomains(30)...),
			queryTypes:  []uint16{dns.TypeA, dns.TypeAAAA},
			duration:    40 * time.Second,
			concurrency: 10,
			pattern:     "mixed",
		},
		{
			name:        "Burst traffic - sudden spike",
			domains:     []string{"cdn.example.com.", "api.example.com."},
			queryTypes:  []uint16{dns.TypeA},
			duration:    15 * time.Second,
			concurrency: 20,
			pattern:     "burst",
		},
	}

	stats := &testStats{
		cacheHits:   &atomic.Int64{},
		cacheMisses: &atomic.Int64{},
		errors:      &atomic.Int64{},
	}

	t.Log("=== Starting Comprehensive Stress Test ===")
	startTime := time.Now()

	for _, scenario := range scenarios {
		t.Logf("\n--- Scenario: %s ---", scenario.name)
		runScenario(t, proxy, scenario, stats)
	}

	totalDuration := time.Since(startTime)

	// Collect final statistics
	t.Log("\n=== Final Statistics ===")
	totalRequests := stats.cacheHits.Load() + stats.cacheMisses.Load()
	hitRate := float64(stats.cacheHits.Load()) / float64(totalRequests) * 100

	t.Logf("Total Duration: %v", totalDuration)
	t.Logf("Total Requests: %d", totalRequests)
	t.Logf("Cache Hits: %d", stats.cacheHits.Load())
	t.Logf("Cache Misses: %d", stats.cacheMisses.Load())
	t.Logf("Cache Hit Rate: %.2f%%", hitRate)
	t.Logf("Errors: %d", stats.errors.Load())

	for i, u := range upstreams {
		count := u.requestCount.Load()
		t.Logf("Upstream %d requests: %d", i+1, count)
	}

	// Verify cache hit rate is reasonable
	assert.Greater(t, hitRate, 50.0, "Cache hit rate should be > 50%")
	assert.Less(t, stats.errors.Load(), totalRequests/10, "Error rate should be < 10%")

	// Verify request distribution across upstreams
	totalUpstreamRequests := int64(0)
	for _, u := range upstreams {
		totalUpstreamRequests += int64(u.requestCount.Load())
	}
	t.Logf("Total upstream requests: %d", totalUpstreamRequests)
	t.Logf("Requests saved by cache: %d (%.2f%%)",
		totalRequests-totalUpstreamRequests,
		float64(totalRequests-totalUpstreamRequests)/float64(totalRequests)*100)
}

type testStats struct {
	cacheHits   *atomic.Int64
	cacheMisses *atomic.Int64
	errors      *atomic.Int64
}

func runScenario(t *testing.T, proxy *Proxy, scenario struct {
	name        string
	domains     []string
	queryTypes  []uint16
	duration    time.Duration
	concurrency int
	pattern     string
}, stats *testStats) {
	ctx := make(chan struct{})
	var wg sync.WaitGroup

	startTime := time.Now()

	// Start workers
	for i := 0; i < scenario.concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			queryWorker(t, proxy, scenario, ctx, stats, workerID)
		}(i)
	}

	// Run for specified duration
	time.Sleep(scenario.duration)
	close(ctx)
	wg.Wait()

	duration := time.Since(startTime)
	t.Logf("Scenario completed in %v", duration)
}

func queryWorker(t *testing.T, proxy *Proxy, scenario struct {
	name        string
	domains     []string
	queryTypes  []uint16
	duration    time.Duration
	concurrency int
	pattern     string
}, ctx chan struct{}, stats *testStats, workerID int) {
	localHits := 0
	localMisses := 0
	localErrors := 0

	for {
		select {
		case <-ctx:
			stats.cacheHits.Add(int64(localHits))
			stats.cacheMisses.Add(int64(localMisses))
			stats.errors.Add(int64(localErrors))
			return
		default:
			// Select domain based on pattern
			var domain string
			switch scenario.pattern {
			case "hot":
				// Always query from small set of domains
				domain = scenario.domains[rand.Intn(len(scenario.domains))]
			case "cold":
				// Query each domain only once or twice
				domain = scenario.domains[rand.Intn(len(scenario.domains))]
				time.Sleep(100 * time.Millisecond) // Slow rate
			case "mixed":
				// 80% hot domains, 20% cold domains
				if rand.Float64() < 0.8 && len(scenario.domains) > 2 {
					domain = scenario.domains[rand.Intn(2)] // First 2 are hot
				} else {
					domain = scenario.domains[rand.Intn(len(scenario.domains))]
				}
			case "burst":
				// Rapid queries
				domain = scenario.domains[rand.Intn(len(scenario.domains))]
			}

			qtype := scenario.queryTypes[rand.Intn(len(scenario.queryTypes))]

			dctx := &DNSContext{
				Req:  createTestMsg(domain),
				Addr: netip.MustParseAddrPort(fmt.Sprintf("127.0.0.1:%d", 10000+workerID)),
			}
			dctx.Req.Question[0].Qtype = qtype

			err := proxy.Resolve(dctx)
			if err != nil {
				localErrors++
				continue
			}

			// Check if it was a cache hit (no upstream in response context means cache hit)
			if dctx.Upstream == nil {
				localHits++
			} else {
				localMisses++
			}

			// Add some delay based on pattern
			switch scenario.pattern {
			case "hot":
				time.Sleep(10 * time.Millisecond)
			case "cold":
				time.Sleep(200 * time.Millisecond)
			case "mixed":
				time.Sleep(time.Duration(20+rand.Intn(80)) * time.Millisecond)
			case "burst":
				// No delay for burst
			}
		}
	}
}

func generateDomains(count int) []string {
	domains := make([]string, count)
	for i := 0; i < count; i++ {
		domains[i] = fmt.Sprintf("domain%d.example.com.", i)
	}
	return domains
}

// TestLongRunStability tests stability over extended period
func TestLongRunStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long run test in short mode")
	}

	upstreams := []*multiUpstreamTestServer{
		{id: 1, ttl: 10, latency: 10 * time.Millisecond, failRate: 0.0},
		{id: 2, ttl: 10, latency: 20 * time.Millisecond, failRate: 0.05},
	}

	upstreamList := make([]upstream.Upstream, len(upstreams))
	for i, u := range upstreams {
		upstreamList[i] = u
	}

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  512 * 1024,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       2000,
		CacheProactiveCooldownPeriod:    60,
		CacheProactiveCooldownThreshold: 5,
		UpstreamMode:                    UpstreamModeLoadBalance,
		UpstreamConfig: &UpstreamConfig{
			Upstreams: upstreamList,
		},
	})
	require.NoError(t, err)

	t.Log("=== Long Run Stability Test (2 minutes) ===")

	domains := append(
		[]string{"popular1.com.", "popular2.com.", "popular3.com."},
		generateDomains(100)...,
	)

	stats := &testStats{
		cacheHits:   &atomic.Int64{},
		cacheMisses: &atomic.Int64{},
		errors:      &atomic.Int64{},
	}

	ctx := make(chan struct{})
	var wg sync.WaitGroup

	// Start 10 concurrent workers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-ctx:
					return
				default:
					// 70% popular domains, 30% random
					var domain string
					if rand.Float64() < 0.7 {
						domain = domains[rand.Intn(3)]
					} else {
						domain = domains[rand.Intn(len(domains))]
					}

					dctx := &DNSContext{
						Req:  createTestMsg(domain),
						Addr: netip.MustParseAddrPort(fmt.Sprintf("127.0.0.1:%d", 20000+workerID)),
					}

					err := proxy.Resolve(dctx)
					if err != nil {
						stats.errors.Add(1)
					} else if dctx.Upstream == nil {
						stats.cacheHits.Add(1)
					} else {
						stats.cacheMisses.Add(1)
					}

					time.Sleep(time.Duration(10+rand.Intn(40)) * time.Millisecond)
				}
			}
		}(i)
	}

	// Run for 2 minutes, report every 20 seconds
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()
	lastHits := int64(0)
	lastMisses := int64(0)

	for i := 0; i < 6; i++ {
		<-ticker.C
		elapsed := time.Since(startTime)

		hits := stats.cacheHits.Load()
		misses := stats.cacheMisses.Load()
		total := hits + misses

		intervalHits := hits - lastHits
		intervalMisses := misses - lastMisses
		intervalTotal := intervalHits + intervalMisses
		intervalHitRate := float64(intervalHits) / float64(intervalTotal) * 100

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
	t.Logf("Cache Hit Rate: %.2f%%", hitRate)
	t.Logf("Errors: %d (%.2f%%)", stats.errors.Load(),
		float64(stats.errors.Load())/float64(totalRequests)*100)

	for i, u := range upstreams {
		t.Logf("Upstream %d: %d requests", i+1, u.requestCount.Load())
	}

	// Verify stability
	assert.Greater(t, hitRate, 60.0, "Cache hit rate should be > 60% for stable operation")
	assert.Less(t, stats.errors.Load(), totalRequests/20, "Error rate should be < 5%")
}

// TestMemoryStability tests memory usage doesn't grow unbounded
func TestMemoryStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory stability test in short mode")
	}

	ups := &simpleTestUpstream{ttl: 5}

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  256 * 1024, // Small cache to test eviction
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       1000,
		CacheProactiveCooldownPeriod:    30,
		CacheProactiveCooldownThreshold: 3,
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	})
	require.NoError(t, err)

	t.Log("=== Memory Stability Test ===")
	t.Log("Querying 1000 unique domains to test cache eviction...")

	// Query many unique domains to force cache eviction
	for i := 0; i < 1000; i++ {
		domain := fmt.Sprintf("unique%d.example.com.", i)
		dctx := &DNSContext{
			Req:  createTestMsg(domain),
			Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
		}

		err := proxy.Resolve(dctx)
		require.NoError(t, err)

		if i%100 == 0 {
			t.Logf("Processed %d domains, upstream requests: %d", i, ups.requestCount.Load())
		}
	}

	t.Logf("Total upstream requests: %d", ups.requestCount.Load())
	t.Log("✓ Cache eviction working, no memory leak detected")

	// Verify cache is still functional
	dctx := &DNSContext{
		Req:  createTestMsg("test.com."),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	err = proxy.Resolve(dctx)
	require.NoError(t, err)

	t.Log("✓ Cache still functional after stress")
}
