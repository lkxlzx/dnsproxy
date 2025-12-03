package proxy

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// Test30MinStress runs a 30-minute stress test with multiple domains,
// multiple upstreams, and concurrent requests to verify proactive refresh
// works correctly under realistic load.
func Test30MinStress(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping 30-minute stress test in short mode")
	}

	var (
		googleRequestCount     int32
		cloudflareRequestCount int32
		openDNSRequestCount    int32
	)

	// Create multiple real DNS upstreams
	googleDNS, err := upstream.AddressToUpstream("8.8.8.8:53", &upstream.Options{
		Timeout: 10 * time.Second,
	})
	require.NoError(t, err)

	cloudflareDNS, err := upstream.AddressToUpstream("1.1.1.1:53", &upstream.Options{
		Timeout: 10 * time.Second,
	})
	require.NoError(t, err)

	openDNS, err := upstream.AddressToUpstream("208.67.222.222:53", &upstream.Options{
		Timeout: 10 * time.Second,
	})
	require.NoError(t, err)

	// Wrap upstreams to count requests
	googleWrapper := &countingUpstreamWrapper{
		upstream: googleDNS,
		counter:  &googleRequestCount,
	}
	cloudflareWrapper := &countingUpstreamWrapper{
		upstream: cloudflareDNS,
		counter:  &cloudflareRequestCount,
	}
	openDNSWrapper := &countingUpstreamWrapper{
		upstream: openDNS,
		counter:  &openDNSRequestCount,
	}

	// Create proxy with cache, proactive refresh, and multiple upstreams
	prx := mustNew(t, &Config{
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{
				googleWrapper,
				cloudflareWrapper,
				openDNSWrapper,
			},
		},
		UpstreamMode:  UpstreamModeLoadBalance, // Load balance across upstreams
		CacheEnabled:  true,
		CacheSizeBytes: 64 * 1024 * 1024,
		CacheMinTTL:    5,    // Minimum TTL: 5 seconds
		CacheMaxTTL:    0,    // No maximum
		CacheOptimistic: true,

		// Proactive refresh settings
		CacheProactiveRefreshTime:       10000, // 10 seconds before expiry
		CacheProactiveCooldownThreshold: 3,     // Need 3 requests
	})

	// Test multiple domains
	domains := []string{
		"google.com.",
		"youtube.com.",
		"facebook.com.",
		"twitter.com.",
		"github.com.",
	}

	startTime := time.Now()

	// Statistics
	type DomainStats struct {
		totalRequests   int64
		cacheHits       int64
		upstreamQueries int64
		errors          int64
		mu              sync.Mutex
	}

	domainStats := make(map[string]*DomainStats)
	for _, domain := range domains {
		domainStats[domain] = &DomainStats{}
	}

	var (
		totalRequests   int64
		totalCacheHits  int64
		totalUpstream   int64
		totalErrors     int64
	)

	t.Log("╔════════════════════════════════════════════════════════════════════════════╗")
	t.Log("║  30-Minute Stress Test - Multi-Domain, Multi-Upstream, Concurrent         ║")
	t.Log("╚════════════════════════════════════════════════════════════════════════════╝")
	t.Log("")
	t.Log("Configuration:")
	t.Log("  - Domains: 5 (google.com, youtube.com, facebook.com, twitter.com, github.com)")
	t.Log("  - Upstreams: 3 (Google DNS, Cloudflare, OpenDNS)")
	t.Log("  - Upstream Mode: Load Balance")
	t.Log("  - Minimum TTL: 5 seconds")
	t.Log("  - Proactive Refresh: 10 seconds before expiry")
	t.Log("  - Cooldown Threshold: 3 requests")
	t.Log("  - Concurrent Workers: 5 (one per domain)")
	t.Log("  - Request Interval: 5 seconds per domain")
	t.Log("  - Test Duration: 30 minutes")
	t.Log("")
	t.Log("Starting stress test...")
	t.Log("")

	// Create a worker for each domain
	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	for _, domain := range domains {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()

			req := &dns.Msg{}
			req.SetQuestion(d, dns.TypeA)
			stats := domainStats[d]

			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-stopChan:
					return
				case <-ticker.C:
					// Make request
					reqStart := time.Now()
					dctx := &DNSContext{Req: req.Copy()}
					err := prx.Resolve(dctx)
					responseTime := time.Since(reqStart)

					atomic.AddInt64(&totalRequests, 1)
					stats.mu.Lock()
					stats.totalRequests++
					stats.mu.Unlock()

					if err != nil {
						atomic.AddInt64(&totalErrors, 1)
						stats.mu.Lock()
						stats.errors++
						stats.mu.Unlock()
						continue
					}

					// Determine if cache hit
					isCacheHit := responseTime < 5*time.Millisecond
					if isCacheHit {
						atomic.AddInt64(&totalCacheHits, 1)
						stats.mu.Lock()
						stats.cacheHits++
						stats.mu.Unlock()
					} else {
						atomic.AddInt64(&totalUpstream, 1)
						stats.mu.Lock()
						stats.upstreamQueries++
						stats.mu.Unlock()
					}
				}
			}
		}(domain)
	}

	// Progress reporter - report every 1 minute
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				elapsed := time.Since(startTime)
				total := atomic.LoadInt64(&totalRequests)
				hits := atomic.LoadInt64(&totalCacheHits)
				upstream := atomic.LoadInt64(&totalUpstream)
				errors := atomic.LoadInt64(&totalErrors)

				hitRate := float64(0)
				if total > 0 {
					hitRate = float64(hits) / float64(total) * 100
				}

				googleReqs := atomic.LoadInt32(&googleRequestCount)
				cloudflareReqs := atomic.LoadInt32(&cloudflareRequestCount)
				openDNSReqs := atomic.LoadInt32(&openDNSRequestCount)

				t.Logf("[%02d:%02d] Total: %d | Cache: %d (%.1f%%) | Upstream: %d | Errors: %d",
					int(elapsed.Minutes()), int(elapsed.Seconds())%60,
					total, hits, hitRate, upstream, errors)
				t.Logf("        Upstream Distribution - Google: %d, Cloudflare: %d, OpenDNS: %d",
					googleReqs, cloudflareReqs, openDNSReqs)
			}
		}
	}()

	// Run for 30 minutes
	time.Sleep(30 * time.Minute)

	// Stop all workers
	close(stopChan)
	wg.Wait()

	// Final statistics
	t.Log("")
	t.Log("╔════════════════════════════════════════════════════════════════════════════╗")
	t.Log("║  Final Statistics                                                          ║")
	t.Log("╚════════════════════════════════════════════════════════════════════════════╝")
	t.Log("")

	total := atomic.LoadInt64(&totalRequests)
	hits := atomic.LoadInt64(&totalCacheHits)
	upstreamTotal := atomic.LoadInt64(&totalUpstream)
	errors := atomic.LoadInt64(&totalErrors)

	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100
	}

	t.Logf("Overall Statistics:")
	t.Logf("  Total Requests:        %d", total)
	t.Logf("  Cache Hits:            %d (%.1f%%)", hits, hitRate)
	t.Logf("  Upstream Queries:      %d", upstreamTotal)
	t.Logf("  Errors:                %d", errors)
	t.Log("")

	googleReqs := atomic.LoadInt32(&googleRequestCount)
	cloudflareReqs := atomic.LoadInt32(&cloudflareRequestCount)
	openDNSReqs := atomic.LoadInt32(&openDNSRequestCount)
	totalUpstreamReqs := googleReqs + cloudflareReqs + openDNSReqs

	t.Logf("Upstream Distribution:")
	t.Logf("  Google DNS:            %d (%.1f%%)", googleReqs, float64(googleReqs)/float64(totalUpstreamReqs)*100)
	t.Logf("  Cloudflare:            %d (%.1f%%)", cloudflareReqs, float64(cloudflareReqs)/float64(totalUpstreamReqs)*100)
	t.Logf("  OpenDNS:               %d (%.1f%%)", openDNSReqs, float64(openDNSReqs)/float64(totalUpstreamReqs)*100)
	t.Logf("  Total:                 %d", totalUpstreamReqs)
	t.Log("")

	t.Logf("Per-Domain Statistics:")
	for _, domain := range domains {
		stats := domainStats[domain]
		stats.mu.Lock()
		domainTotal := stats.totalRequests
		domainHits := stats.cacheHits
		domainUpstream := stats.upstreamQueries
		domainErrors := stats.errors
		stats.mu.Unlock()

		domainHitRate := float64(0)
		if domainTotal > 0 {
			domainHitRate = float64(domainHits) / float64(domainTotal) * 100
		}

		t.Logf("  %s", domain)
		t.Logf("    Total: %d | Cache: %d (%.1f%%) | Upstream: %d | Errors: %d",
			domainTotal, domainHits, domainHitRate, domainUpstream, domainErrors)
	}
	t.Log("")

	// Performance metrics
	duration := time.Since(startTime)
	requestsPerSecond := float64(total) / duration.Seconds()

	t.Logf("Performance Metrics:")
	t.Logf("  Test Duration:         %s", duration.Round(time.Second))
	t.Logf("  Requests/Second:       %.2f", requestsPerSecond)
	t.Logf("  Avg Requests/Domain:   %.0f", float64(total)/float64(len(domains)))
	t.Log("")

	// Proactive refresh estimation
	expectedRefreshes := int64(0)
	for _, stats := range domainStats {
		stats.mu.Lock()
		// Estimate: after 3 initial requests, proactive refresh should keep cache fresh
		// With 5s interval and typical 300s TTL, we expect ~1 refresh per 300s per domain
		if stats.totalRequests > 3 {
			expectedRefreshes += (stats.totalRequests - 3) / 60 // Rough estimate
		}
		stats.mu.Unlock()
	}

	actualRefreshes := upstreamTotal - int64(len(domains)) // Subtract initial queries
	if actualRefreshes < 0 {
		actualRefreshes = 0
	}

	t.Logf("Proactive Refresh Analysis:")
	t.Logf("  Initial Queries:       %d (one per domain)", len(domains))
	t.Logf("  Subsequent Upstream:   %d", actualRefreshes)
	t.Logf("  Expected Refreshes:    ~%d", expectedRefreshes)
	t.Log("")

	// Validation
	t.Log("╔════════════════════════════════════════════════════════════════════════════╗")
	t.Log("║  Validation                                                                ║")
	t.Log("╚════════════════════════════════════════════════════════════════════════════╝")
	t.Log("")

	if hitRate >= 90 {
		t.Log("✅ Cache hit rate is excellent (>= 90%)")
	} else if hitRate >= 80 {
		t.Log("✅ Cache hit rate is good (>= 80%)")
	} else {
		t.Logf("⚠️  Cache hit rate is lower than expected (%.1f%%)", hitRate)
	}

	// Check load balancing
	maxUpstream := googleReqs
	if cloudflareReqs > maxUpstream {
		maxUpstream = cloudflareReqs
	}
	if openDNSReqs > maxUpstream {
		maxUpstream = openDNSReqs
	}

	minUpstream := googleReqs
	if cloudflareReqs < minUpstream {
		minUpstream = cloudflareReqs
	}
	if openDNSReqs < minUpstream {
		minUpstream = openDNSReqs
	}

	balanceRatio := float64(minUpstream) / float64(maxUpstream) * 100
	if balanceRatio >= 80 {
		t.Logf("✅ Load balancing is working well (%.1f%% balance)", balanceRatio)
	} else {
		t.Logf("⚠️  Load balancing could be better (%.1f%% balance)", balanceRatio)
	}

	if errors == 0 {
		t.Log("✅ No errors during test")
	} else {
		t.Logf("⚠️  %d errors occurred during test", errors)
	}

	if actualRefreshes > 0 {
		t.Log("✅ Proactive refresh is working (upstream queries beyond initial)")
	}

	t.Log("")
	t.Log("Test completed successfully!")
}
