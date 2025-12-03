package proxy

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGoogleCache_UserConfig tests caching with user's configuration:
// - CacheMinTTL: 0
// - CacheMaxTTL: 0
// - ProactiveRefreshTime: 2000ms
// - CooldownThreshold: -1 (disabled)
func TestGoogleCache_UserConfig(t *testing.T) {
	ups := &testUpstream{
		ans: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   "www.google.com.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    237, // Google's typical TTL
				},
				A: net.ParseIP("142.250.196.196"),
			},
		},
	}

	prx, err := New(&Config{
		CacheEnabled:                    true,
		CacheMinTTL:                     0,
		CacheMaxTTL:                     0,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       2000,
		CacheProactiveCooldownPeriod:    1800,
		CacheProactiveCooldownThreshold: -1,
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = prx.Shutdown(context.Background()) })

	t.Log("Configuration: CacheMinTTL=0, ProactiveRefresh=2000ms, CooldownThreshold=-1")

	// First request
	req := &dns.Msg{}
	req.SetQuestion("www.google.com.", dns.TypeA)

	dctx := &DNSContext{Req: req}
	err = prx.Resolve(dctx)
	require.NoError(t, err)
	require.NotNil(t, dctx.Res)
	require.Len(t, dctx.Res.Answer, 1)

	ttl1 := dctx.Res.Answer[0].Header().Ttl
	t.Logf("T0: Initial request - TTL: %d seconds", ttl1)

	// Wait 3 seconds
	time.Sleep(3 * time.Second)

	// Second request
	dctx2 := &DNSContext{Req: req.Copy()}
	err = prx.Resolve(dctx2)
	require.NoError(t, err)
	require.NotNil(t, dctx2.Res)
	require.Len(t, dctx2.Res.Answer, 1)

	ttl2 := dctx2.Res.Answer[0].Header().Ttl
	t.Logf("T1: After 3 seconds - TTL: %d seconds", ttl2)

	// Verify cache hit
	assert.Less(t, ttl2, ttl1, "TTL should decrease (cache hit)")
	t.Logf("Cache working: ✅ (TTL decreased from %d to %d)", ttl1, ttl2)
}

// TestGoogleCache_WithMinTTL tests with CacheMinTTL=600.
func TestGoogleCache_WithMinTTL(t *testing.T) {
	ups := &testUpstream{
		ans: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   "www.google.com.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    237,
				},
				A: net.ParseIP("142.250.196.196"),
			},
		},
	}

	prx, err := New(&Config{
		CacheEnabled:                    true,
		CacheMinTTL:                     600, // 10 minutes
		CacheMaxTTL:                     0,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       30000,
		CacheProactiveCooldownPeriod:    1800,
		CacheProactiveCooldownThreshold: -1,
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = prx.Shutdown(context.Background()) })

	t.Log("Configuration: CacheMinTTL=600 (overrides Google's 237s)")

	// First request
	req := &dns.Msg{}
	req.SetQuestion("www.google.com.", dns.TypeA)

	dctx := &DNSContext{Req: req}
	err = prx.Resolve(dctx)
	require.NoError(t, err)

	ttl1 := dctx.Res.Answer[0].Header().Ttl
	t.Logf("T0: Initial request - TTL: %d seconds (should be ~600)", ttl1)
	assert.Greater(t, ttl1, uint32(590), "TTL should be overridden to ~600")

	// Wait 5 seconds
	time.Sleep(5 * time.Second)

	// Second request
	dctx2 := &DNSContext{Req: req.Copy()}
	err = prx.Resolve(dctx2)
	require.NoError(t, err)

	ttl2 := dctx2.Res.Answer[0].Header().Ttl
	t.Logf("T1: After 5 seconds - TTL: %d seconds", ttl2)

	// Verify cache hit and TTL override
	assert.Less(t, ttl2, ttl1, "TTL should decrease (cache hit)")
	assert.Greater(t, ttl2, uint32(580), "TTL should still be high (CacheMinTTL effect)")
	t.Logf("CacheMinTTL working: ✅ (TTL: %d → %d)", ttl1, ttl2)
}

// TestGoogleCache_MultipleRequests tests multiple requests over time.
func TestGoogleCache_MultipleRequests(t *testing.T) {
	ups := &testUpstream{
		ans: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   "www.google.com.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    237,
				},
				A: net.ParseIP("142.250.196.196"),
			},
		},
	}

	prx, err := New(&Config{
		CacheEnabled:                    true,
		CacheMinTTL:                     0,
		CacheMaxTTL:                     0,
		CacheOptimistic:                 true,
		CacheProactiveRefreshTime:       2000,
		CacheProactiveCooldownPeriod:    1800,
		CacheProactiveCooldownThreshold: -1,
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = prx.Shutdown(context.Background()) })

	t.Log("Testing multiple requests with 2-second intervals")

	req := &dns.Msg{}
	req.SetQuestion("www.google.com.", dns.TypeA)

	// Make 5 requests, 2 seconds apart
	for i := 0; i < 5; i++ {
		dctx := &DNSContext{Req: req.Copy()}
		err := prx.Resolve(dctx)
		require.NoError(t, err)

		ttl := dctx.Res.Answer[0].Header().Ttl
		t.Logf("Request %d (T=%ds): TTL=%d", i+1, i*2, ttl)

		if i < 4 {
			time.Sleep(2 * time.Second)
		}
	}

	t.Log("All requests completed ✅")
}
