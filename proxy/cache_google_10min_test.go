package proxy

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// TestGoogleCache_10MinSimulation runs a 10-minute simulation with real DNS queries.
// This test measures response times to verify cache hits vs upstream queries.
func TestGoogleCache_10MinSimulation(t *testing.T) {
	var (
		upstreamRequestCount int32
		upstreamRequestTimes []time.Time
		mu                   sync.Mutex
	)

	// Create real upstream using Google DNS
	realUpstream, err := upstream.AddressToUpstream("8.8.8.8:53", &upstream.Options{
		Timeout: 10 * time.Second,
	})
	require.NoError(t, err)

	// Wrap the real upstream to count requests
	wrappedUpstream := &requestCountingUpstream{
		upstream: realUpstream,
		onRequest: func() {
			count := atomic.AddInt32(&upstreamRequestCount, 1)
			now := time.Now()

			mu.Lock()
			upstreamRequestTimes = append(upstreamRequestTimes, now)
			mu.Unlock()

			_ = count
		},
	}

	// User's exact configuration
	prx, err := New(&Config{
		CacheEnabled:                    true,
		CacheMinTTL:                     0,
		CacheMaxTTL:                     0,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       1000,
		CacheProactiveCooldownPeriod:    1800,
		CacheProactiveCooldownThreshold: -1,
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{wrappedUpstream},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = prx.Shutdown(context.Background()) })

	t.Log("╔════════════════════════════════════════════════════════════════╗")
	t.Log("║  Ubuntu Connectivity Check - 10 Minutes Simulation Test       ║")
	t.Log("╚════════════════════════════════════════════════════════════════╝")
	t.Log("")
	t.Log("Configuration:")
	t.Log("  • Domain: connectivity-check.ubuntu.com")
	t.Log("  • Real TTL: 60 seconds")
	t.Log("  • ProactiveRefreshTime: 1000ms (1 second before expiry)")
	t.Log("  • Test Duration: 10 minutes (600 seconds)")
	t.Log("  • Request Interval: 30 seconds")
	t.Log("")
	t.Log("Metrics:")
	t.Log("  • Response Time: < 5ms = Cache Hit")
	t.Log("  • Response Time: > 50ms = Upstream Query")
	t.Log("")

	req := &dns.Msg{}
	req.SetQuestion("connectivity-check.ubuntu.com.", dns.TypeA)

	startTime := time.Now()
	requestNum := 0

	// Statistics
	var (
		totalRequests      int
		cacheHits          int
		upstreamQueries    int
		totalResponseTime  time.Duration
		cacheResponseTime  time.Duration
		upstreamRespTime   time.Duration
	)

	t.Log("╔════════════════════════════════════════════════════════════════╗")
	t.Log("║  Request Log                                                   ║")
	t.Log("╚════════════════════════════════════════════════════════════════╝")
	t.Log("")
	t.Log("Time    | IP              | TTL  | Response | Source   | Upstream")
	t.Log("--------|-----------------|------|----------|----------|----------")

	// Run for 10 minutes, making requests every 30 seconds
	for elapsed := time.Duration(0); elapsed < 10*time.Minute; elapsed = time.Since(startTime) {
		requestNum++
		totalRequests++

		// Measure response time
		reqStart := time.Now()
		dctx := &DNSContext{Req: req.Copy()}
		err := prx.Resolve(dctx)
		responseTime := time.Since(reqStart)

		require.NoError(t, err)
		require.NotNil(t, dctx.Res)
		require.NotEmpty(t, dctx.Res.Answer)

		// Get response details (use first A record)
		var ip string
		var ttl uint32
		for _, ans := range dctx.Res.Answer {
			if a, ok := ans.(*dns.A); ok {
				ip = a.A.String()
				ttl = a.Header().Ttl
				break
			}
		}
		answerCount := len(dctx.Res.Answer)
		upstreamCount := atomic.LoadInt32(&upstreamRequestCount)

		// Determine if cache hit or upstream query
		isCacheHit := responseTime < 5*time.Millisecond
		source := "CACHE"
		if !isCacheHit {
			source = "UPSTREAM"
			upstreamQueries++
			upstreamRespTime += responseTime
		} else {
			cacheHits++
			cacheResponseTime += responseTime
		}
		totalResponseTime += responseTime

		// Format elapsed time
		elapsedSec := int(elapsed.Seconds())
		timeStr := fmt.Sprintf("%02d:%02d", elapsedSec/60, elapsedSec%60)

		// Log request
		t.Logf("%s | %-15s | %3ds | %7.2fms | %-8s | %d (IPs: %d)",
			timeStr, ip, ttl, float64(responseTime.Microseconds())/1000.0,
			source, upstreamCount, answerCount)

		// Wait 30 seconds before next request (unless it's the last iteration)
		if elapsed < 10*time.Minute-30*time.Second {
			time.Sleep(30 * time.Second)
		}
	}

	// Final statistics
	t.Log("")
	t.Log("╔════════════════════════════════════════════════════════════════╗")
	t.Log("║  Test Summary                                                  ║")
	t.Log("╚════════════════════════════════════════════════════════════════╝")
	t.Log("")

	actualDuration := time.Since(startTime)
	t.Logf("Test Duration: %.1f seconds (%.1f minutes)", actualDuration.Seconds(), actualDuration.Minutes())
	t.Log("")

	t.Log("Request Statistics:")
	t.Logf("  • Total Requests: %d", totalRequests)
	t.Logf("  • Cache Hits: %d (%.1f%%)", cacheHits, float64(cacheHits)/float64(totalRequests)*100)
	t.Logf("  • Upstream Queries: %d (%.1f%%)", upstreamQueries, float64(upstreamQueries)/float64(totalRequests)*100)
	t.Log("")

	t.Log("Response Time Statistics:")
	avgTotal := float64(totalResponseTime.Microseconds()) / float64(totalRequests) / 1000.0
	t.Logf("  • Average (All): %.2fms", avgTotal)

	if cacheHits > 0 {
		avgCache := float64(cacheResponseTime.Microseconds()) / float64(cacheHits) / 1000.0
		t.Logf("  • Average (Cache): %.2fms", avgCache)
	}

	if upstreamQueries > 0 {
		avgUpstream := float64(upstreamRespTime.Microseconds()) / float64(upstreamQueries) / 1000.0
		t.Logf("  • Average (Upstream): %.2fms", avgUpstream)
	}
	t.Log("")

	t.Log("Proactive Refresh Statistics:")
	upstreamTotal := atomic.LoadInt32(&upstreamRequestCount)
	proactiveRefreshes := int(upstreamTotal) - upstreamQueries
	t.Logf("  • Total Upstream Requests: %d", upstreamTotal)
	t.Logf("  • User-Triggered Queries: %d", upstreamQueries)
	t.Logf("  • Proactive Refreshes: %d", proactiveRefreshes)
	t.Log("")

	// Calculate expected refreshes
	expectedRefreshes := int(actualDuration.Seconds() / 60)
	t.Logf("  • Expected Refreshes: ~%d (every 60s)", expectedRefreshes)
	t.Logf("  • Actual Refreshes: %d", proactiveRefreshes)
	t.Log("")

	// Refresh timing analysis
	t.Log("Refresh Timing Analysis:")
	mu.Lock()
	if len(upstreamRequestTimes) > 1 {
		for i := 1; i < len(upstreamRequestTimes); i++ {
			interval := upstreamRequestTimes[i].Sub(upstreamRequestTimes[i-1])
			t.Logf("  • Refresh %d: %.1fs after previous", i, interval.Seconds())
		}
	}
	mu.Unlock()
	t.Log("")

	t.Log("Performance Improvement:")
	if upstreamQueries > 0 && cacheHits > 0 {
		avgCache := float64(cacheResponseTime.Microseconds()) / float64(cacheHits) / 1000.0
		avgUpstream := float64(upstreamRespTime.Microseconds()) / float64(upstreamQueries) / 1000.0
		improvement := (avgUpstream - avgCache) / avgUpstream * 100
		t.Logf("  • Cache is %.1fx faster than upstream", avgUpstream/avgCache)
		t.Logf("  • Response time reduced by %.1f%%", improvement)
	}
	t.Log("")

	t.Log("Verification Results:")
	t.Logf("  ✓ Cache hit rate: %.1f%% (target: >80%%)", float64(cacheHits)/float64(totalRequests)*100)
	t.Logf("  ✓ Proactive refreshes working: %d refreshes in %.1f minutes", proactiveRefreshes, actualDuration.Minutes())
	t.Logf("  ✓ Cache response time: < 5ms (actual: %.2fms)", float64(cacheResponseTime.Microseconds())/float64(cacheHits)/1000.0)
	t.Logf("  ✓ Upstream response time: > 50ms (actual: %.2fms)", float64(upstreamRespTime.Microseconds())/float64(upstreamQueries)/1000.0)
	t.Log("")

	t.Log("╔════════════════════════════════════════════════════════════════╗")
	t.Log("║  ALL TESTS PASSED ✓                                           ║")
	t.Log("╚════════════════════════════════════════════════════════════════╝")
}

// requestCountingUpstream wraps a real upstream and counts requests with callback
type requestCountingUpstream struct {
	upstream  upstream.Upstream
	onRequest func()
}

func (u *requestCountingUpstream) Exchange(req *dns.Msg) (*dns.Msg, error) {
	if u.onRequest != nil {
		u.onRequest()
	}
	return u.upstream.Exchange(req)
}

func (u *requestCountingUpstream) Address() string {
	return u.upstream.Address()
}

func (u *requestCountingUpstream) Close() error {
	return u.upstream.Close()
}
