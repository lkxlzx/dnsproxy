package proxy

import (
	"fmt"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Simple mock upstream for testing
type simpleTestUpstream struct {
	requestCount atomic.Int32
	ttl          uint32
}

func (u *simpleTestUpstream) Exchange(req *dns.Msg) (*dns.Msg, error) {
	u.requestCount.Add(1)

	resp := &dns.Msg{}
	resp.SetReply(req)
	if len(req.Question) > 0 {
		resp.Answer = []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   req.Question[0].Name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    u.ttl,
				},
				A: net.ParseIP("1.2.3.4"),
			},
		}
	}
	return resp, nil
}

func (u *simpleTestUpstream) Address() string {
	return "test-upstream"
}

func (u *simpleTestUpstream) Close() error {
	return nil
}

// TestProactiveRefresh_Basic tests basic proactive refresh functionality
func TestProactiveRefresh_Basic(t *testing.T) {
	ups := &simpleTestUpstream{ttl: 3} // 3 seconds TTL

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  64 * 1024,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       500, // 500ms before expiration
		CacheProactiveCooldownThreshold: -1,  // Disable cooldown
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	})
	require.NoError(t, err)

	// Initial request
	dctx := &DNSContext{
		Req:  createTestMsg("test.example."),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	err = proxy.Resolve(dctx)
	require.NoError(t, err)
	require.NotNil(t, dctx.Res)

	initialCount := ups.requestCount.Load()
	assert.Equal(t, int32(1), initialCount, "should have 1 initial request")

	// Cache hit
	dctx2 := &DNSContext{
		Req:  createTestMsg("test.example."),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	err = proxy.Resolve(dctx2)
	require.NoError(t, err)

	cacheCount := ups.requestCount.Load()
	assert.Equal(t, initialCount, cacheCount, "should hit cache")

	// Wait for proactive refresh (3s - 500ms = 2.5s)
	time.Sleep(2700 * time.Millisecond)

	refreshCount := ups.requestCount.Load()
	assert.Greater(t, refreshCount, initialCount, "should have refreshed proactively")

	t.Logf("Request counts: initial=%d, after_refresh=%d", initialCount, refreshCount)
}

// TestProactiveRefresh_Cooldown tests cooldown mechanism
func TestProactiveRefresh_Cooldown(t *testing.T) {
	// Test with low frequency (should NOT refresh)
	t.Run("low_frequency", func(t *testing.T) {
		ups := &simpleTestUpstream{ttl: 3}

		proxy, err := New(&Config{
			CacheEnabled:                    true,
			CacheSizeBytes:                  64 * 1024,
			CacheOptimistic:                 true,
			CacheProactiveRefreshTime:       500,
			CacheProactiveCooldownPeriod:    5, // 5 seconds
			CacheProactiveCooldownThreshold: 3, // Need 3 requests
			UpstreamConfig: &UpstreamConfig{
				Upstreams: []upstream.Upstream{ups},
			},
		})
		require.NoError(t, err)

		// Only 1 request
		dctx := &DNSContext{
			Req:  createTestMsg("low-freq.example."),
			Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
		}
		err = proxy.Resolve(dctx)
		require.NoError(t, err)

		initial := ups.requestCount.Load()

		// Wait for would-be refresh time
		time.Sleep(2700 * time.Millisecond)

		after := ups.requestCount.Load()
		assert.Equal(t, initial, after, "should NOT refresh with only 1 request")
	})

	// Test with cooldown disabled (threshold = -1)
	t.Run("cooldown_disabled", func(t *testing.T) {
		ups := &simpleTestUpstream{ttl: 3}

		proxy, err := New(&Config{
			CacheEnabled:                    true,
			CacheSizeBytes:                  64 * 1024,
			CacheOptimistic:                 true,
			CacheProactiveRefreshTime:       500,
			CacheProactiveCooldownThreshold: -1, // Disabled
			UpstreamConfig: &UpstreamConfig{
				Upstreams: []upstream.Upstream{ups},
			},
		})
		require.NoError(t, err)

		// Single request
		dctx := &DNSContext{
			Req:  createTestMsg("any-freq.example."),
			Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
		}
		err = proxy.Resolve(dctx)
		require.NoError(t, err)

		initial := ups.requestCount.Load()

		// Wait for proactive refresh
		time.Sleep(2700 * time.Millisecond)

		after := ups.requestCount.Load()
		assert.Greater(t, after, initial, "should refresh even with 1 request when cooldown disabled")
	})
}

// TestProactiveRefresh_MultiDomain tests multiple domains
func TestProactiveRefresh_MultiDomain(t *testing.T) {
	testCases := []struct {
		name        string
		ttl         uint32
		refreshWait time.Duration
	}{
		{"short.example.", 2, 1700 * time.Millisecond},   // 2s - 500ms = 1.5s
		{"medium.example.", 5, 4700 * time.Millisecond},  // 5s - 500ms = 4.5s
		{"long.example.", 10, 9700 * time.Millisecond},   // 10s - 500ms = 9.5s
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ups := &simpleTestUpstream{ttl: tc.ttl}

			proxy, err := New(&Config{
				CacheEnabled:                    true,
				CacheSizeBytes:                  64 * 1024,
				CacheOptimistic:                 true,
				CacheProactiveRefreshTime:       500,
				CacheProactiveCooldownThreshold: -1,
				UpstreamConfig: &UpstreamConfig{
					Upstreams: []upstream.Upstream{ups},
				},
			})
			require.NoError(t, err)

			// Initial request
			dctx := &DNSContext{
				Req:  createTestMsg(tc.name),
				Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
			}
			err = proxy.Resolve(dctx)
			require.NoError(t, err)

			initial := ups.requestCount.Load()

			// Wait for refresh
			time.Sleep(tc.refreshWait)

			after := ups.requestCount.Load()
			assert.Greater(t, after, initial,
				"domain %s with TTL %ds should refresh", tc.name, tc.ttl)

			t.Logf("Domain: %s, TTL: %ds, Initial: %d, After: %d",
				tc.name, tc.ttl, initial, after)
		})
	}
}

// TestProactiveRefresh_VeryShortTTL tests 1-second TTL
func TestProactiveRefresh_VeryShortTTL(t *testing.T) {
	ups := &simpleTestUpstream{ttl: 1} // 1 second

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  64 * 1024,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       500, // 500ms
		CacheProactiveCooldownThreshold: -1,
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	})
	require.NoError(t, err)

	dctx := &DNSContext{
		Req:  createTestMsg("very-short.example."),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	err = proxy.Resolve(dctx)
	require.NoError(t, err)

	initial := ups.requestCount.Load()

	// Wait for refresh (1s - 500ms = 500ms)
	time.Sleep(700 * time.Millisecond)

	after := ups.requestCount.Load()
	assert.Greater(t, after, initial, "should refresh 1-second TTL")

	t.Logf("Very short TTL: initial=%d, after=%d", initial, after)
}

// Helper to create test DNS message
func createTestMsg(domain string) *dns.Msg {
	req := &dns.Msg{}
	req.SetQuestion(domain, dns.TypeA)
	req.RecursionDesired = true
	return req
}

// TestProactiveRefresh_DynamicThreshold tests dynamic threshold activation
func TestProactiveRefresh_DynamicThreshold(t *testing.T) {
	ups := &simpleTestUpstream{ttl: 3}

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  64 * 1024,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       500,
		CacheProactiveCooldownPeriod:    5, // 5 seconds
		CacheProactiveCooldownThreshold: 3, // Need 3 requests
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	})
	require.NoError(t, err)

	// First request - creates cache, only 1 request recorded
	dctx1 := &DNSContext{
		Req:  createTestMsg("dynamic.example."),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	err = proxy.Resolve(dctx1)
	require.NoError(t, err)

	// Second request - cache hit, 2 requests total
	dctx2 := &DNSContext{
		Req:  createTestMsg("dynamic.example."),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	err = proxy.Resolve(dctx2)
	require.NoError(t, err)

	// Third request - cache hit, 3 requests total, SHOULD trigger dynamic scheduling
	dctx3 := &DNSContext{
		Req:  createTestMsg("dynamic.example."),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	err = proxy.Resolve(dctx3)
	require.NoError(t, err)

	initial := ups.requestCount.Load()
	t.Logf("After 3 requests: upstream count = %d", initial)

	// Wait for proactive refresh (should happen now due to dynamic scheduling)
	time.Sleep(2700 * time.Millisecond)

	after := ups.requestCount.Load()
	t.Logf("After waiting: upstream count = %d", after)
	assert.Greater(t, after, initial, "should refresh after dynamically reaching threshold")
}

// TestProactiveRefresh_CrossCacheCycle tests statistics across cache expiration
func TestProactiveRefresh_CrossCacheCycle(t *testing.T) {
	ups := &simpleTestUpstream{ttl: 2} // 2 second TTL

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  64 * 1024,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       500,
		CacheProactiveCooldownPeriod:    10, // 10 seconds cooldown
		CacheProactiveCooldownThreshold: 3,  // Need 3 requests
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	})
	require.NoError(t, err)

	domain := "cross-cycle.example."

	// First request at T0
	dctx1 := &DNSContext{
		Req:  createTestMsg(domain),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	err = proxy.Resolve(dctx1)
	require.NoError(t, err)
	t.Log("T0: First request, stats = 1")

	// Wait for cache to expire (2 second TTL)
	time.Sleep(2200 * time.Millisecond)
	t.Log("Cache expired, but stats should persist (within 10s cooldown)")

	// Second request at T1 (cache expired, but within cooldown period)
	dctx2 := &DNSContext{
		Req:  createTestMsg(domain),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	err = proxy.Resolve(dctx2)
	require.NoError(t, err)
	t.Log("T1: Second request (cache miss), stats = 2")

	// Third request at T2 immediately (should hit new cache and trigger dynamic scheduling)
	dctx3 := &DNSContext{
		Req:  createTestMsg(domain),
		Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
	}
	err = proxy.Resolve(dctx3)
	require.NoError(t, err)
	t.Log("T2: Third request (cache hit), stats = 3, should trigger dynamic scheduling")

	initial := ups.requestCount.Load()
	t.Logf("Upstream requests so far: %d", initial)

	// Wait for proactive refresh (2s - 500ms = 1.5s)
	time.Sleep(1700 * time.Millisecond)

	after := ups.requestCount.Load()
	t.Logf("Upstream requests after refresh: %d", after)

	// Should have refreshed because stats accumulated across cache cycles
	assert.Greater(t, after, initial,
		"should refresh even though cache expired between requests (stats persist)")
}

// Benchmark proactive refresh overhead
func BenchmarkProactiveRefresh(b *testing.B) {
	ups := &simpleTestUpstream{ttl: 300}

	proxy, err := New(&Config{
		CacheEnabled:                    true,
		CacheSizeBytes:                  64 * 1024,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       30000, // 30s
		CacheProactiveCooldownThreshold: -1,
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	})
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dctx := &DNSContext{
			Req:  createTestMsg(fmt.Sprintf("bench%d.example.", i%100)),
			Addr: netip.MustParseAddrPort("127.0.0.1:12345"),
		}
		_ = proxy.Resolve(dctx)
	}
}
