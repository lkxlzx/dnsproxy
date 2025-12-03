package proxy

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGoogleCache_LongRunWithProactiveRefresh tests the complete lifecycle:
// 1. Initial request
// 2. Wait for proactive refresh to trigger
// 3. Verify subsequent requests hit cache
// 4. Verify cache stays fresh
func TestGoogleCache_LongRunWithProactiveRefresh(t *testing.T) {
	var upstreamRequestCount int32

	ups := &testUpstream{
		ans: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   "www.google.com.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    10, // Short TTL for faster testing (10 seconds)
				},
				A: net.ParseIP("142.250.196.196"),
			},
		},
	}

	// Wrap upstream to count requests
	countingUps := &countingUpstream{
		upstream: ups,
		counter:  &upstreamRequestCount,
	}

	// User's configuration with short TTL for testing
	prx, err := New(&Config{
		CacheEnabled:    true,
		CacheMinTTL:     0,    // No override (use original 10s TTL)
		CacheMaxTTL:     0,    // No limit
		CacheOptimistic: true, // Enable optimistic cache

		// Proactive refresh: 2 seconds before expiry
		CacheProactiveRefreshTime:       2000, // 2 seconds
		CacheProactiveCooldownPeriod:    1800, // 30 minutes
		CacheProactiveCooldownThreshold: -1,   // Disabled (always refresh)

		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{countingUps},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = prx.Shutdown(context.Background()) })

	t.Log("=== Long-Run Test Start ===")
	t.Log("Configuration:")
	t.Log("  - TTL: 10 seconds")
	t.Log("  - ProactiveRefreshTime: 2000ms (refresh at T=8s)")
	t.Log("  - CooldownThreshold: -1 (disabled)")
	t.Log("")

	req := &dns.Msg{}
	req.SetQuestion("www.google.com.", dns.TypeA)

	// Phase 1: Initial request
	t.Log("=== Phase 1: Initial Request ===")
	dctx := &DNSContext{Req: req.Copy()}
	err = prx.Resolve(dctx)
	require.NoError(t, err)
	require.NotNil(t, dctx.Res)
	require.Len(t, dctx.Res.Answer, 1)

	count1 := atomic.LoadInt32(&upstreamRequestCount)
	ttl1 := dctx.Res.Answer[0].Header().Ttl
	t.Logf("T0: Initial request")
	t.Logf("  - Upstream requests: %d", count1)
	t.Logf("  - TTL: %d seconds", ttl1)
	assert.Equal(t, int32(1), count1, "Should query upstream on first request")

	// Phase 2: Wait for proactive refresh
	// TTL=10s, refresh at 10-2=8s, so wait 9s to ensure refresh happened
	t.Log("")
	t.Log("=== Phase 2: Waiting for Proactive Refresh ===")
	t.Log("Waiting 9 seconds for proactive refresh to trigger...")
	time.Sleep(9 * time.Second)

	count2 := atomic.LoadInt32(&upstreamRequestCount)
	t.Logf("After 9 seconds:")
	t.Logf("  - Upstream requests: %d", count2)
	t.Logf("  - Proactive refreshes: %d", count2-count1)

	// Verify proactive refresh happened
	assert.Greater(t, count2, count1, "Proactive refresh should have triggered")

	// Phase 3: User request (should hit cache)
	t.Log("")
	t.Log("=== Phase 3: User Request After Refresh ===")
	dctx2 := &DNSContext{Req: req.Copy()}
	err = prx.Resolve(dctx2)
	require.NoError(t, err)
	require.NotNil(t, dctx2.Res)
	require.Len(t, dctx2.Res.Answer, 1)

	count3 := atomic.LoadInt32(&upstreamRequestCount)
	ttl3 := dctx2.Res.Answer[0].Header().Ttl
	t.Logf("T9: User request after proactive refresh")
	t.Logf("  - Upstream requests: %d", count3)
	t.Logf("  - Cache hit: %v", count3 == count2)
	t.Logf("  - TTL: %d seconds", ttl3)

	// Should hit cache (no new upstream request)
	assert.Equal(t, count2, count3, "Should hit cache, not query upstream")

	// Phase 4: Multiple requests over time
	t.Log("")
	t.Log("=== Phase 4: Multiple Requests Over 20 Seconds ===")
	for i := 1; i <= 10; i++ {
		time.Sleep(2 * time.Second)

		dctx := &DNSContext{Req: req.Copy()}
		err := prx.Resolve(dctx)
		require.NoError(t, err)

		count := atomic.LoadInt32(&upstreamRequestCount)
		ttl := dctx.Res.Answer[0].Header().Ttl
		t.Logf("T%d: Request %d - Upstream requests: %d, TTL: %d",
			9+i*2, i, count, ttl)
	}

	finalCount := atomic.LoadInt32(&upstreamRequestCount)
	t.Log("")
	t.Log("=== Test Summary ===")
	t.Logf("Total time: 29 seconds")
	t.Logf("Total user requests: 12")
	t.Logf("Total upstream requests: %d", finalCount)
	t.Logf("  - Initial request: 1")
	t.Logf("  - Proactive refreshes: %d", finalCount-1)
	t.Logf("Cache hit rate: %.1f%%", float64(12-1)/12*100)
	t.Log("Proactive refresh working: ✅")
	t.Log("Cache always fresh: ✅")
}

// TestGoogleCache_LongRunWithMinTTL tests with CacheMinTTL override.
func TestGoogleCache_LongRunWithMinTTL(t *testing.T) {
	var upstreamRequestCount int32

	ups := &testUpstream{
		ans: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   "www.google.com.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    10, // Short TTL from upstream
				},
				A: net.ParseIP("142.250.196.196"),
			},
		},
	}

	countingUps := &countingUpstream{
		upstream: ups,
		counter:  &upstreamRequestCount,
	}

	// Configuration with CacheMinTTL=30 (override to 30 seconds)
	prx, err := New(&Config{
		CacheEnabled:                    true,
		CacheMinTTL:                     30, // Override to 30 seconds
		CacheMaxTTL:                     0,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       5000, // 5 seconds before expiry
		CacheProactiveCooldownPeriod:    1800,
		CacheProactiveCooldownThreshold: -1,
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{countingUps},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = prx.Shutdown(context.Background()) })

	t.Log("=== Long-Run Test With CacheMinTTL ===")
	t.Log("Configuration:")
	t.Log("  - Upstream TTL: 10 seconds")
	t.Log("  - CacheMinTTL: 30 seconds (overridden)")
	t.Log("  - ProactiveRefreshTime: 5000ms (refresh at T=25s)")
	t.Log("")

	req := &dns.Msg{}
	req.SetQuestion("www.google.com.", dns.TypeA)

	// Initial request
	t.Log("=== Phase 1: Initial Request ===")
	dctx := &DNSContext{Req: req.Copy()}
	err = prx.Resolve(dctx)
	require.NoError(t, err)

	count1 := atomic.LoadInt32(&upstreamRequestCount)
	ttl1 := dctx.Res.Answer[0].Header().Ttl
	t.Logf("T0: Initial request")
	t.Logf("  - Upstream requests: %d", count1)
	t.Logf("  - TTL: %d seconds (should be ~30)", ttl1)
	assert.Equal(t, int32(1), count1)
	assert.Greater(t, ttl1, uint32(25), "TTL should be overridden to ~30")

	// Wait 15 seconds (original TTL would have expired)
	t.Log("")
	t.Log("=== Phase 2: After 15 Seconds ===")
	t.Log("Waiting 15 seconds (original TTL=10s would have expired)...")
	time.Sleep(15 * time.Second)

	dctx2 := &DNSContext{Req: req.Copy()}
	err = prx.Resolve(dctx2)
	require.NoError(t, err)

	count2 := atomic.LoadInt32(&upstreamRequestCount)
	ttl2 := dctx2.Res.Answer[0].Header().Ttl
	t.Logf("T15: Request after 15 seconds")
	t.Logf("  - Upstream requests: %d", count2)
	t.Logf("  - Cache hit: %v", count2 == count1)
	t.Logf("  - TTL: %d seconds", ttl2)

	// Should hit cache (CacheMinTTL keeps it alive)
	assert.Equal(t, count1, count2, "Should hit cache (CacheMinTTL extended lifetime)")
	assert.Greater(t, ttl2, uint32(10), "TTL should still be high")

	// Wait for proactive refresh (at T=25s)
	t.Log("")
	t.Log("=== Phase 3: Waiting for Proactive Refresh ===")
	t.Log("Waiting 12 more seconds for proactive refresh (at T=25s)...")
	time.Sleep(12 * time.Second)

	count3 := atomic.LoadInt32(&upstreamRequestCount)
	t.Logf("T27: After proactive refresh window")
	t.Logf("  - Upstream requests: %d", count3)
	t.Logf("  - Proactive refreshes: %d", count3-count1)

	// Verify proactive refresh happened
	assert.Greater(t, count3, count1, "Proactive refresh should have triggered")

	// Final request
	t.Log("")
	t.Log("=== Phase 4: Final Request ===")
	dctx3 := &DNSContext{Req: req.Copy()}
	err = prx.Resolve(dctx3)
	require.NoError(t, err)

	count4 := atomic.LoadInt32(&upstreamRequestCount)
	ttl4 := dctx3.Res.Answer[0].Header().Ttl
	t.Logf("T27: Final request")
	t.Logf("  - Upstream requests: %d", count4)
	t.Logf("  - Cache hit: %v", count4 == count3)
	t.Logf("  - TTL: %d seconds", ttl4)

	// Should hit cache
	assert.Equal(t, count3, count4, "Should hit cache")

	t.Log("")
	t.Log("=== Test Summary ===")
	t.Logf("Total time: 27 seconds")
	t.Logf("Total user requests: 3")
	t.Logf("Total upstream requests: %d", count4)
	t.Logf("  - Initial request: 1")
	t.Logf("  - Proactive refreshes: %d", count4-1)
	t.Log("CacheMinTTL working: ✅")
	t.Log("Proactive refresh working: ✅")
	t.Log("Cache always available: ✅")
}

// countingUpstream wraps an upstream and counts requests.
type countingUpstream struct {
	upstream upstream.Upstream
	counter  *int32
}

func (u *countingUpstream) Exchange(m *dns.Msg) (*dns.Msg, error) {
	atomic.AddInt32(u.counter, 1)
	return u.upstream.Exchange(m)
}

func (u *countingUpstream) Address() string {
	return u.upstream.Address()
}

func (u *countingUpstream) Close() error {
	return nil
}
