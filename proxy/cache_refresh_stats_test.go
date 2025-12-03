package proxy

import (
	"net/netip"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRefreshDoesNotCountAsRequest verifies that proactive refresh operations
// do not count towards request statistics for cooldown mechanism.
func TestRefreshDoesNotCountAsRequest(t *testing.T) {
	ups := &simpleTestUpstream{ttl: 2} // 2 second TTL

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  64 * 1024,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       500,                // 500ms before expiration
		CacheProactiveCooldownPeriod:    10,                 // 10 seconds cooldown
		CacheProactiveCooldownThreshold: 3,                  // Need 3 requests
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	})
	require.NoError(t, err)

	domain := "test.example."

	// Request 1: Initial cache miss
	dctx1 := &DNSContext{
		Req:  createTestMsg(domain),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	err = proxy.Resolve(dctx1)
	require.NoError(t, err)
	t.Log("Request 1: Initial cache miss, stats = 1")

	// Request 2: Cache hit
	dctx2 := &DNSContext{
		Req:  createTestMsg(domain),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	err = proxy.Resolve(dctx2)
	require.NoError(t, err)
	t.Log("Request 2: Cache hit, stats = 2")

	// Request 3: Cache hit, should trigger dynamic scheduling
	dctx3 := &DNSContext{
		Req:  createTestMsg(domain),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	err = proxy.Resolve(dctx3)
	require.NoError(t, err)
	t.Log("Request 3: Cache hit, stats = 3, dynamic scheduling triggered")

	initialCount := ups.requestCount.Load()
	t.Logf("Upstream requests so far: %d", initialCount)

	// Wait for proactive refresh (2s - 500ms = 1.5s)
	time.Sleep(1700 * time.Millisecond)

	afterFirstRefresh := ups.requestCount.Load()
	t.Logf("After first refresh: %d upstream requests", afterFirstRefresh)
	assert.Greater(t, afterFirstRefresh, initialCount, "should have refreshed")

	// Wait for second refresh cycle
	time.Sleep(1700 * time.Millisecond)

	afterSecondRefresh := ups.requestCount.Load()
	t.Logf("After second refresh: %d upstream requests", afterSecondRefresh)
	assert.Greater(t, afterSecondRefresh, afterFirstRefresh, "should have refreshed again")

	// Wait for third refresh cycle
	time.Sleep(1700 * time.Millisecond)

	afterThirdRefresh := ups.requestCount.Load()
	t.Logf("After third refresh: %d upstream requests", afterThirdRefresh)
	assert.Greater(t, afterThirdRefresh, afterSecondRefresh, "should have refreshed again")

	// Key assertion: Check request statistics
	// The cache should have recorded only 3 user requests, not the refresh operations
	key := msgToKey(dctx1.Req)
	keyStr := string(key)
	
	val, ok := proxy.cache.requestStats.Load(keyStr)
	require.True(t, ok, "request stats should exist")
	
	stat := val.(*requestStat)
	stat.mu.Lock()
	validCount := 0
	cutoff := time.Now().Add(-10 * time.Second) // cooldown period
	for _, ts := range stat.timestamps {
		if ts.After(cutoff) {
			validCount++
		}
	}
	stat.mu.Unlock()

	t.Logf("Valid request count in stats: %d", validCount)
	
	// Should be exactly 3 (the user requests), not more despite multiple refreshes
	assert.Equal(t, 3, validCount, "refresh operations should not count as requests")
}

// TestColdDomainStopsRefreshing verifies that domains with low request frequency
// stop being refreshed after the cooldown period expires.
func TestColdDomainStopsRefreshing(t *testing.T) {
	ups := &simpleTestUpstream{ttl: 2} // 2 second TTL

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  64 * 1024,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       500, // 500ms before expiration
		CacheProactiveCooldownPeriod:    5,   // 5 seconds cooldown (short for testing)
		CacheProactiveCooldownThreshold: 3,   // Need 3 requests
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	})
	require.NoError(t, err)

	domain := "cold.example."

	// Make 3 requests to trigger refresh
	for i := 0; i < 3; i++ {
		dctx := &DNSContext{
			Req:  createTestMsg(domain),
			Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
		}
		err = proxy.Resolve(dctx)
		require.NoError(t, err)
		t.Logf("Request %d completed", i+1)
	}

	initialCount := ups.requestCount.Load()
	t.Logf("Initial upstream count: %d", initialCount)

	// Wait for first refresh
	time.Sleep(1700 * time.Millisecond)
	afterFirstRefresh := ups.requestCount.Load()
	t.Logf("After first refresh: %d", afterFirstRefresh)
	assert.Greater(t, afterFirstRefresh, initialCount, "should refresh initially")

	// Wait for cooldown period to expire (5 seconds) + some buffer
	t.Log("Waiting for cooldown period to expire...")
	time.Sleep(6 * time.Second)

	beforeCheck := ups.requestCount.Load()
	t.Logf("Before final check: %d", beforeCheck)

	// Wait for what would be another refresh cycle
	time.Sleep(2 * time.Second)
	afterCooldown := ups.requestCount.Load()
	t.Logf("After cooldown expired: %d", afterCooldown)

	// Should NOT have refreshed because request stats expired
	// Allow for one more refresh that was already scheduled, but no more after that
	assert.LessOrEqual(t, afterCooldown-beforeCheck, int32(1), 
		"should stop or nearly stop refreshing after cooldown period expires")
}

// TestHotDomainKeepsRefreshing verifies that frequently accessed domains
// continue to be refreshed.
func TestHotDomainKeepsRefreshing(t *testing.T) {
	ups := &simpleTestUpstream{ttl: 2} // 2 second TTL

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  64 * 1024,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       500, // 500ms before expiration
		CacheProactiveCooldownPeriod:    10,  // 10 seconds cooldown
		CacheProactiveCooldownThreshold: 3,   // Need 3 requests
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	})
	require.NoError(t, err)

	domain := "hot.example."

	// Make initial 3 requests
	for i := 0; i < 3; i++ {
		dctx := &DNSContext{
			Req:  createTestMsg(domain),
			Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
		}
		err = proxy.Resolve(dctx)
		require.NoError(t, err)
	}

	initialCount := ups.requestCount.Load()

	// Keep making requests every second to maintain "hot" status
	stopRequests := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopRequests:
				return
			case <-ticker.C:
				dctx := &DNSContext{
					Req:  createTestMsg(domain),
					Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
				}
				_ = proxy.Resolve(dctx)
			}
		}
	}()

	// Wait for multiple refresh cycles
	time.Sleep(6 * time.Second)
	close(stopRequests)

	finalCount := ups.requestCount.Load()
	t.Logf("Initial: %d, Final: %d", initialCount, finalCount)

	// Should have refreshed multiple times
	assert.Greater(t, finalCount, initialCount+2, 
		"hot domain should keep refreshing")
}
