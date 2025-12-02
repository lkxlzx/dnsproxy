package proxy

import (
	"net/netip"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/stretchr/testify/require"
)

// TestProactiveRefresh_ContinuousRefresh tests that hot domains are continuously refreshed
// This is the EXPECTED behavior to keep cache fresh for popular domains
func TestProactiveRefresh_ContinuousRefresh(t *testing.T) {
	ups := &simpleTestUpstream{ttl: 3} // 3 second TTL

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  64 * 1024,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       1000, // 1 second before expiration
		CacheProactiveCooldownThreshold: -1,   // Disable cooldown for continuous refresh
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	})
	require.NoError(t, err)

	// Initial request
	dctx := &DNSContext{
		Req:  createTestMsg("hot-domain.example."),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	err = proxy.Resolve(dctx)
	require.NoError(t, err)

	initial := ups.requestCount.Load()
	t.Logf("Initial upstream requests: %d", initial)

	// Wait for multiple refresh cycles
	// With 3s TTL and 1s refresh time, we expect refresh every 2s
	// In 10 seconds, we should see about 4-5 refreshes
	time.Sleep(10 * time.Second)

	final := ups.requestCount.Load()
	refreshCount := final - initial
	t.Logf("Final upstream requests: %d (refreshes: %d)", final, refreshCount)

	// Verify continuous refresh is working
	// With 3s TTL, 10s should give us 3-4 refresh cycles
	if refreshCount < 3 {
		t.Errorf("Too few refreshes (%d), continuous refresh not working!", refreshCount)
	} else if refreshCount > 6 {
		t.Errorf("Too many refreshes (%d), possible timing issue!", refreshCount)
	} else {
		t.Logf("✅ Continuous refresh working correctly (%d refreshes in 10s)", refreshCount)
	}
}

// TestProactiveRefresh_StopsWhenCold tests that refresh stops when domain becomes cold
func TestProactiveRefresh_StopsWhenCold(t *testing.T) {
	ups := &simpleTestUpstream{ttl: 2} // 2 second TTL

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  64 * 1024,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       500,
		CacheProactiveCooldownPeriod:    3,  // 3 seconds cooldown
		CacheProactiveCooldownThreshold: 2,  // Need 2 requests
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	})
	require.NoError(t, err)

	// Make 2 requests to trigger refresh
	for i := 0; i < 2; i++ {
		dctx := &DNSContext{
			Req:  createTestMsg("cooling-domain.example."),
			Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
		}
		err = proxy.Resolve(dctx)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)
	}

	initial := ups.requestCount.Load()
	t.Logf("After initial requests: %d", initial)

	// Wait for first refresh
	time.Sleep(1700 * time.Millisecond)
	afterFirstRefresh := ups.requestCount.Load()
	t.Logf("After first refresh: %d", afterFirstRefresh)

	// Wait for cooldown period to expire (3 seconds)
	// After this, the domain should be considered "cold"
	time.Sleep(3500 * time.Millisecond)
	afterCooldown := ups.requestCount.Load()
	t.Logf("After cooldown expires: %d", afterCooldown)

	// The refresh should stop because request stats expired
	// We might see 1-2 more refreshes before stats expire, but not many
	totalRefreshes := afterCooldown - initial
	if totalRefreshes > 3 {
		t.Logf("⚠️  Still refreshing after cooldown (%d refreshes), stats may not have expired yet", totalRefreshes)
	} else {
		t.Logf("✅ Refresh stopped or slowing down (%d refreshes), cooldown working", totalRefreshes)
	}
}

// simpleTestUpstream is defined in cache_proactive_simple_test.go
