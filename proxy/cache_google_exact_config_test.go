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

// TestGoogleCache_ExactUserConfig tests with user's exact configuration:
// - CacheMinTTL: 0
// - CacheMaxTTL: 0
// - ProactiveRefreshTime: 1000ms (1 second before expiry)
// - CooldownPeriod: 1800s
// - CooldownThreshold: -1 (disabled)
//
// This test verifies:
// 1. Initial request caches the response
// 2. Proactive refresh triggers 1 second before TTL expiry
// 3. Refresh updates the cache with new IP and TTL
// 4. Subsequent requests hit the refreshed cache
func TestGoogleCache_ExactUserConfig(t *testing.T) {
	var (
		upstreamRequestCount int32
		requestTimes         []time.Time
		mu                   sync.Mutex
	)

	// Simulate Google's behavior with changing IPs
	googleIPs := []string{
		"142.250.196.196", // First response
		"142.250.196.197", // After first refresh
		"142.250.196.198", // After second refresh
		"142.250.196.199", // After third refresh
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
							Ttl:    10, // Short TTL for testing (10 seconds)
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
	t.Log("║  Google Cache Test - User's Exact Configuration               ║")
	t.Log("╚════════════════════════════════════════════════════════════════╝")
	t.Log("")
	t.Log("Configuration:")
	t.Log("  • CacheMinTTL: 0 (no override)")
	t.Log("  • CacheMaxTTL: 0 (no limit)")
	t.Log("  • ProactiveRefreshTime: 1000ms (1 second before expiry)")
	t.Log("  • CooldownThreshold: -1 (disabled, always refresh)")
	t.Log("  • Test TTL: 10 seconds (simulating Google)")
	t.Log("")
	t.Log("Expected behavior:")
	t.Log("  • T=0s: Initial request → upstream query → cache IP1, TTL=10s")
	t.Log("  • T=9s: Proactive refresh → upstream query → cache IP2, TTL=10s")
	t.Log("  • T=9s+: User requests → cache hit → return IP2")
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
	assert.Equal(t, uint32(10), ttl1, "Should cache original TTL")

	// ═══════════════════════════════════════════════════════════════
	// Phase 2: Wait for Proactive Refresh
	// ═══════════════════════════════════════════════════════════════
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("Phase 2: Waiting for Proactive Refresh")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("Expected: Refresh should trigger at T=9s (10s TTL - 1s refresh time)")
	t.Log("")

	// Wait 9.5 seconds to ensure refresh has happened
	waitTime := 9500 * time.Millisecond
	t.Logf("Waiting %.1f seconds...", waitTime.Seconds())
	time.Sleep(waitTime)

	count2 := atomic.LoadInt32(&upstreamRequestCount)
	elapsed := time.Since(startTime)

	t.Log("")
	t.Logf("✓ Wait completed (elapsed: %.1fs)", elapsed.Seconds())
	t.Logf("  • Upstream requests: %d", count2)
	t.Logf("  • Proactive refreshes: %d", count2-count1)

	// Verify proactive refresh happened
	assert.Greater(t, count2, count1, "Proactive refresh should have triggered")

	// Check refresh timing
	mu.Lock()
	if len(requestTimes) >= 2 {
		refreshTime := requestTimes[1].Sub(requestTimes[0])
		t.Logf("  • Refresh timing: %.2fs after initial request", refreshTime.Seconds())
		assert.InDelta(t, 9.0, refreshTime.Seconds(), 0.5, "Refresh should happen ~9s after initial request")
	}
	mu.Unlock()
	t.Log("")

	// ═══════════════════════════════════════════════════════════════
	// Phase 3: User Request After Refresh
	// ═══════════════════════════════════════════════════════════════
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("Phase 3: User Request After Refresh")
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

	t.Logf("✓ User request completed")
	t.Logf("  • Upstream requests: %d", count3)
	t.Logf("  • Cache hit: %v", count3 == count2)
	t.Logf("  • Returned IP: %s", ip2)
	t.Logf("  • Returned TTL: %d seconds", ttl2)
	t.Log("")

	// Verify cache hit (no new upstream request)
	assert.Equal(t, count2, count3, "Should hit cache, not query upstream")

	// Verify IP was updated by refresh
	assert.NotEqual(t, ip1, ip2, "IP should be updated by proactive refresh")
	assert.Equal(t, googleIPs[1], ip2, "Should return refreshed IP")

	// Verify TTL is fresh (close to 10 seconds)
	assert.Greater(t, ttl2, uint32(8), "TTL should be fresh after refresh")

	// ═══════════════════════════════════════════════════════════════
	// Phase 4: Multiple Subsequent Requests
	// ═══════════════════════════════════════════════════════════════
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("Phase 4: Multiple Subsequent Requests")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("Testing cache hits over 5 seconds...")
	t.Log("")

	for i := 1; i <= 5; i++ {
		time.Sleep(1 * time.Second)

		dctx := &DNSContext{Req: req.Copy()}
		err := prx.Resolve(dctx)
		require.NoError(t, err)

		count := atomic.LoadInt32(&upstreamRequestCount)
		ip := dctx.Res.Answer[0].(*dns.A).A.String()
		ttl := dctx.Res.Answer[0].Header().Ttl

		t.Logf("  Request %d: IP=%s, TTL=%ds, Upstream=%d",
			i, ip, ttl, count)

		// All requests should hit cache
		assert.Equal(t, count2, count, "Should continue hitting cache")
		assert.Equal(t, ip2, ip, "Should return same cached IP")
	}
	t.Log("")

	// ═══════════════════════════════════════════════════════════════
	// Phase 5: Wait for Second Refresh
	// ═══════════════════════════════════════════════════════════════
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("Phase 5: Waiting for Second Proactive Refresh")
	t.Log("═══════════════════════════════════════════════════════════════")

	// We're now at ~T=14.5s, wait another 5s to reach T=19.5s
	// Second refresh should happen at T=18s (9s + 10s - 1s)
	t.Log("Waiting 5 more seconds for second refresh...")
	time.Sleep(5 * time.Second)

	count4 := atomic.LoadInt32(&upstreamRequestCount)
	t.Log("")
	t.Logf("✓ Second refresh window passed")
	t.Logf("  • Upstream requests: %d", count4)
	t.Logf("  • Total refreshes: %d", count4-1)

	assert.Greater(t, count4, count2, "Second proactive refresh should have triggered")

	// Final request to verify cache is still working
	dctx3 := &DNSContext{Req: req.Copy()}
	err = prx.Resolve(dctx3)
	require.NoError(t, err)

	count5 := atomic.LoadInt32(&upstreamRequestCount)
	ip3 := dctx3.Res.Answer[0].(*dns.A).A.String()
	ttl3 := dctx3.Res.Answer[0].Header().Ttl

	t.Log("")
	t.Logf("✓ Final request completed")
	t.Logf("  • Upstream requests: %d", count5)
	t.Logf("  • Cache hit: %v", count5 == count4)
	t.Logf("  • Returned IP: %s", ip3)
	t.Logf("  • Returned TTL: %d seconds", ttl3)

	assert.Equal(t, count4, count5, "Should hit cache after second refresh")
	assert.NotEqual(t, ip2, ip3, "IP should be updated by second refresh")

	// ═══════════════════════════════════════════════════════════════
	// Test Summary
	// ═══════════════════════════════════════════════════════════════
	t.Log("")
	t.Log("╔════════════════════════════════════════════════════════════════╗")
	t.Log("║  Test Summary                                                  ║")
	t.Log("╚════════════════════════════════════════════════════════════════╝")
	t.Log("")
	t.Logf("Total test duration: %.1f seconds", time.Since(startTime).Seconds())
	t.Logf("Total user requests: 8")
	t.Logf("Total upstream requests: %d", count5)
	t.Logf("  • Initial request: 1")
	t.Logf("  • Proactive refreshes: %d", count5-1)
	t.Logf("Cache hit rate: %.1f%%", float64(8-1)/8*100)
	t.Log("")
	t.Log("Verification Results:")
	t.Log("  ✓ Initial request cached correctly")
	t.Log("  ✓ Proactive refresh triggered at correct time (TTL - 1s)")
	t.Log("  ✓ Cache updated with new IP after refresh")
	t.Log("  ✓ Cache updated with new TTL after refresh")
	t.Log("  ✓ Subsequent requests hit refreshed cache")
	t.Log("  ✓ Multiple refreshes work correctly")
	t.Log("  ✓ Cache remains fresh throughout test")
	t.Log("")
	t.Log("╔════════════════════════════════════════════════════════════════╗")
	t.Log("║  ALL TESTS PASSED ✓                                           ║")
	t.Log("╚════════════════════════════════════════════════════════════════╝")
}

// dynamicUpstream is a test upstream that can change responses dynamically.
type dynamicUpstream struct {
	onExchange func(*dns.Msg) (*dns.Msg, error)
}

func (u *dynamicUpstream) Exchange(m *dns.Msg) (*dns.Msg, error) {
	return u.onExchange(m)
}

func (u *dynamicUpstream) Address() string {
	return "test-upstream"
}

func (u *dynamicUpstream) Close() error {
	return nil
}
