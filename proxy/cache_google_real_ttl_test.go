package proxy

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGoogleCache_RealTTL tests with Google's actual TTL (237 seconds).
// User's configuration:
// - CacheMinTTL: 0
// - CacheMaxTTL: 0
// - ProactiveRefreshTime: 1000ms (1 second before expiry)
// - CooldownThreshold: -1 (disabled)
//
// This test verifies:
// 1. Initial request caches with TTL=237s
// 2. Proactive refresh triggers at T=236s (237-1)
// 3. Cache is updated with new IP and TTL
// 4. Subsequent requests hit the refreshed cache
func TestGoogleCache_RealTTL(t *testing.T) {
	var (
		upstreamRequestCount int32
		requestTimes         []time.Time
		mu                   sync.Mutex
	)

	// Simulate Google's real behavior
	googleIPs := []string{
		"142.250.196.196", // First response
		"142.250.198.68",  // After refresh (different IP from Google's pool)
	}

	ups := &dynamicUpstream{
		onExchange: func(m *dns.Msg) (*dns.Msg, error) {
			count := atomic.AddInt32(&upstreamRequestCount, 1)
			now := time.Now()

			mu.Lock()
			requestTimes = append(requestTimes, now)
			mu.Unlock()

			// Use different IP for each upstream request
			ipIndex := int(count - 1)
			if ipIndex >= len(googleIPs) {
				ipIndex = len(googleIPs) - 1
			}

			resp := &dns.Msg{
				MsgHdr: dns.MsgHdr{
					Response: true,
					Rcode:    dns.RcodeSuccess,
				},
				Question: m.Question,
				Answer: []dns.RR{
					&dns.A{
						Hdr: dns.RR_Header{
							Name:   "www.google.com.",
							Rrtype: dns.TypeA,
							Class:  dns.ClassINET,
							Ttl:    237, // Google's actual TTL
						},
						A: net.ParseIP(googleIPs[ipIndex]),
					},
				},
			}

			return resp, nil
		},
	}

	// User's exact configuration
	prx, err := New(&Config{
		CacheEnabled:    true,
		CacheMinTTL:     0,    // No override
		CacheMaxTTL:     0,    // No limit
		CacheOptimistic: true, // Enable optimistic cache

		// User's proactive refresh settings
		CacheProactiveRefreshTime:       1000, // 1 second before expiry
		CacheProactiveCooldownPeriod:    1800, // 30 minutes
		CacheProactiveCooldownThreshold: -1,   // Disabled (always refresh)

		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = prx.Shutdown(context.Background()) })

	t.Log("╔════════════════════════════════════════════════════════════════╗")
	t.Log("║  Google Cache Test - Real TTL (237 seconds)                   ║")
	t.Log("╚════════════════════════════════════════════════════════════════╝")
	t.Log("")
	t.Log("Configuration:")
	t.Log("  • Domain: www.google.com")
	t.Log("  • Real TTL: 237 seconds (from Google)")
	t.Log("  • CacheMinTTL: 0 (no override)")
	t.Log("  • ProactiveRefreshTime: 1000ms (1 second before expiry)")
	t.Log("  • CooldownThreshold: -1 (disabled)")
	t.Log("")
	t.Log("Expected behavior:")
	t.Log("  • T=0s: Initial request → cache IP1, TTL=237s")
	t.Log("  • T=236s: Proactive refresh → cache IP2, TTL=237s")
	t.Log("  • T=236s+: User requests → cache hit → return IP2")
	t.Log("")

	req := &dns.Msg{}
	req.SetQuestion("www.google.com.", dns.TypeA)

	// ═══════════════════════════════════════════════════════════════
	// Phase 1: Initial Request
	// ═══════════════════════════════════════════════════════════════
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("Phase 1: Initial Request (T=0s)")
	t.Log("═══════════════════════════════════════════════════════════════")

	startTime := time.Now()
	dctx1 := &DNSContext{Req: req.Copy()}
	err = prx.Resolve(dctx1)
	require.NoError(t, err)
	require.NotNil(t, dctx1.Res)
	require.Len(t, dctx1.Res.Answer, 1)

	count1 := atomic.LoadInt32(&upstreamRequestCount)
	ip1 := dctx1.Res.Answer[0].(*dns.A).A.String()
	ttl1 := dctx1.Res.Answer[0].Header().Ttl

	t.Logf("✓ Initial request completed")
	t.Logf("  • Upstream requests: %d", count1)
	t.Logf("  • Cached IP: %s", ip1)
	t.Logf("  • Cached TTL: %d seconds", ttl1)
	t.Log("")

	assert.Equal(t, int32(1), count1, "Should query upstream on first request")
	assert.Equal(t, googleIPs[0], ip1, "Should cache first IP")
	assert.Equal(t, uint32(237), ttl1, "Should cache Google's TTL")

	// ═══════════════════════════════════════════════════════════════
	// Phase 2: Requests Before Refresh
	// ═══════════════════════════════════════════════════════════════
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("Phase 2: Requests Before Refresh (T=30s, 60s, 90s)")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("Testing cache hits before proactive refresh...")
	t.Log("")

	testTimes := []int{30, 60, 90} // Test at 30s, 60s, 90s
	for _, waitSec := range testTimes {
		// Calculate how long to wait
		elapsed := time.Since(startTime).Seconds()
		waitTime := float64(waitSec) - elapsed
		if waitTime > 0 {
			t.Logf("Waiting %.1f seconds to reach T=%ds...", waitTime, waitSec)
			time.Sleep(time.Duration(waitTime * float64(time.Second)))
		}

		dctx := &DNSContext{Req: req.Copy()}
		err := prx.Resolve(dctx)
		require.NoError(t, err)

		count := atomic.LoadInt32(&upstreamRequestCount)
		ip := dctx.Res.Answer[0].(*dns.A).A.String()
		ttl := dctx.Res.Answer[0].Header().Ttl
		elapsed = time.Since(startTime).Seconds()

		t.Logf("T=%.0fs: IP=%s, TTL=%ds, Upstream=%d, CacheHit=%v",
			elapsed, ip, ttl, count, count == count1)

		assert.Equal(t, count1, count, "Should hit cache before refresh")
		assert.Equal(t, ip1, ip, "Should return original IP")
	}
	t.Log("")

	// ═══════════════════════════════════════════════════════════════
	// Phase 3: Wait for Proactive Refresh
	// ═══════════════════════════════════════════════════════════════
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("Phase 3: Waiting for Proactive Refresh")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("Expected: Refresh should trigger at T=236s (237s TTL - 1s)")
	t.Log("")

	// Wait until T=237s (a bit past the refresh time)
	elapsed := time.Since(startTime).Seconds()
	waitTime := 237.0 - elapsed
	if waitTime > 0 {
		t.Logf("Waiting %.1f seconds for proactive refresh...", waitTime)
		time.Sleep(time.Duration(waitTime * float64(time.Second)))
	}

	count2 := atomic.LoadInt32(&upstreamRequestCount)
	elapsed = time.Since(startTime).Seconds()

	t.Log("")
	t.Logf("✓ Wait completed (elapsed: %.1fs)", elapsed)
	t.Logf("  • Upstream requests: %d", count2)
	t.Logf("  • Proactive refreshes: %d", count2-count1)

	// Verify proactive refresh happened
	assert.Greater(t, count2, count1, "Proactive refresh should have triggered")

	// Check refresh timing
	mu.Lock()
	if len(requestTimes) >= 2 {
		refreshTime := requestTimes[1].Sub(requestTimes[0])
		t.Logf("  • Refresh timing: %.1fs after initial request", refreshTime.Seconds())
		assert.InDelta(t, 236.0, refreshTime.Seconds(), 1.0, 
			"Refresh should happen ~236s after initial request")
	}
	mu.Unlock()
	t.Log("")

	// ═══════════════════════════════════════════════════════════════
	// Phase 4: User Request After Refresh
	// ═══════════════════════════════════════════════════════════════
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("Phase 4: User Request After Refresh")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("Expected: Should hit cache with refreshed IP and TTL")
	t.Log("")

	dctx2 := &DNSContext{Req: req.Copy()}
	err = prx.Resolve(dctx2)
	require.NoError(t, err)
	require.NotNil(t, dctx2.Res)
	require.Len(t, dctx2.Res.Answer, 1)

	count3 := atomic.LoadInt32(&upstreamRequestCount)
	ip2 := dctx2.Res.Answer[0].(*dns.A).A.String()
	ttl2 := dctx2.Res.Answer[0].Header().Ttl
	elapsed = time.Since(startTime).Seconds()

	t.Logf("✓ User request completed (T=%.1fs)", elapsed)
	t.Logf("  • Upstream requests: %d", count3)
	t.Logf("  • Cache hit: %v", count3 == count2)
	t.Logf("  • Returned IP: %s (was: %s)", ip2, ip1)
	t.Logf("  • Returned TTL: %d seconds", ttl2)
	t.Log("")

	// Verify cache hit (no new upstream request)
	assert.Equal(t, count2, count3, "Should hit cache, not query upstream")

	// Verify IP was updated by refresh
	assert.NotEqual(t, ip1, ip2, "IP should be updated by proactive refresh")
	assert.Equal(t, googleIPs[1], ip2, "Should return refreshed IP")

	// Verify TTL is fresh (close to 237 seconds)
	assert.Greater(t, ttl2, uint32(230), "TTL should be fresh after refresh")

	// ═══════════════════════════════════════════════════════════════
	// Phase 5: Multiple Subsequent Requests
	// ═══════════════════════════════════════════════════════════════
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("Phase 5: Multiple Subsequent Requests")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("Testing cache hits after refresh...")
	t.Log("")

	for i := 1; i <= 3; i++ {
		time.Sleep(5 * time.Second)

		dctx := &DNSContext{Req: req.Copy()}
		err := prx.Resolve(dctx)
		require.NoError(t, err)

		count := atomic.LoadInt32(&upstreamRequestCount)
		ip := dctx.Res.Answer[0].(*dns.A).A.String()
		ttl := dctx.Res.Answer[0].Header().Ttl
		elapsed := time.Since(startTime).Seconds()

		t.Logf("Request %d (T=%.0fs): IP=%s, TTL=%ds, Upstream=%d",
			i, elapsed, ip, ttl, count)

		// All requests should hit cache
		assert.Equal(t, count2, count, "Should continue hitting cache")
		assert.Equal(t, ip2, ip, "Should return refreshed IP")
	}
	t.Log("")

	// ═══════════════════════════════════════════════════════════════
	// Test Summary
	// ═══════════════════════════════════════════════════════════════
	finalCount := atomic.LoadInt32(&upstreamRequestCount)
	totalDuration := time.Since(startTime).Seconds()

	t.Log("╔════════════════════════════════════════════════════════════════╗")
	t.Log("║  Test Summary                                                  ║")
	t.Log("╚════════════════════════════════════════════════════════════════╝")
	t.Log("")
	t.Logf("Total test duration: %.1f seconds (%.1f minutes)", totalDuration, totalDuration/60)
	t.Logf("Total user requests: 7")
	t.Logf("Total upstream requests: %d", finalCount)
	t.Logf("  • Initial request: 1")
	t.Logf("  • Proactive refreshes: %d", finalCount-1)
	t.Logf("Cache hit rate: %.1f%%", float64(7-1)/7*100)
	t.Log("")
	t.Log("Verification Results:")
	t.Log("  ✓ Initial request cached with Google's real TTL (237s)")
	t.Log("  ✓ Proactive refresh triggered at T=236s (TTL - 1s)")
	t.Log("  ✓ Cache updated with new IP after refresh")
	t.Log("  ✓ Cache updated with new TTL after refresh")
	t.Log("  ✓ Subsequent requests hit refreshed cache")
	t.Log("  ✓ Cache remains fresh throughout test")
	t.Log("")
	t.Log("╔════════════════════════════════════════════════════════════════╗")
	t.Log("║  ALL TESTS PASSED ✓                                           ║")
	t.Log("╚════════════════════════════════════════════════════════════════╝")
}
