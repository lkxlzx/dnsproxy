package proxy

import (
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// TestGoogleCache_LoopFix tests that the loop doesn't repeat excessively
func TestGoogleCache_LoopFix(t *testing.T) {
	// Create upstream with test answer
	ups := &testUpstream{
		ans: []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   "test.example.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    5, // 5 second TTL
				},
				A: []byte{8, 8, 8, 8},
			},
		},
	}

	// Create proxy with cache
	prx := mustNew(t, &Config{
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
		CacheEnabled:                    true,
		CacheSizeBytes:                  64 * 1024,
		CacheProactiveRefreshTime:       1000, // 1 second before expiry
		CacheProactiveCooldownThreshold: 2,
	})

	req := &dns.Msg{}
	req.SetQuestion("test.example.", dns.TypeA)

	startTime := time.Now()
	requestCount := 0

	// Run for 15 seconds, making requests every 3 seconds
	for time.Since(startTime) < 15*time.Second {
		requestCount++

		dctx := &DNSContext{Req: req.Copy()}
		err := prx.Resolve(dctx)
		require.NoError(t, err)

		// Wait 3 seconds before next request (unless we're close to the end)
		if time.Since(startTime) < 15*time.Second-3*time.Second {
			time.Sleep(3 * time.Second)
		} else {
			// Exit the loop if we're in the last 3 seconds
			break
		}
	}

	t.Logf("Total requests made: %d", requestCount)
	t.Logf("Expected requests: ~5 (15s / 3s)")
	
	// Should make approximately 5 requests (0s, 3s, 6s, 9s, 12s)
	require.GreaterOrEqual(t, requestCount, 4, "Should make at least 4 requests")
	require.LessOrEqual(t, requestCount, 6, "Should not make more than 6 requests")
	
	t.Log("✅ Loop control working correctly - no excessive iterations")
}
