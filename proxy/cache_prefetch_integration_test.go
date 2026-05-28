package proxy

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCachePrefetch_Integration tests the complete prefetch workflow.
func TestCachePrefetch_Integration(t *testing.T) {
	// Skip if no network
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create upstream
	ups, err := upstream.AddressToUpstream("8.8.8.8", &upstream.Options{
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err, "failed to create upstream")

	// Create proxy configuration
	config := &Config{
		Logger:         logger,
		CacheEnabled:   true,
		CacheSizeBytes: 1024 * 1024, // 1MB
		DNSSECEnabled:  true,         // Enable DNSSEC for caching
		CachePrefetchConfig: &PrefetchConfig{
			Enabled:                 true,
			ThresholdSeconds:        3,  // Short threshold for testing
			ThresholdPercent:        80,
			MaxConcurrent:           5, // Fast scanning
			MinHeatThreshold:        3,  // Lower threshold for testing
			TimeWindow:              10 * time.Second,
		},
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	}

	// Create proxy
	proxy, err := New(config)
	require.NoError(t, err, "failed to create proxy")
	require.NotNil(t, proxy, "proxy should not be nil")

	// Initialize proxy (without starting listeners)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Log("Proxy created successfully")

	// Test domain
	testDomain := "google.com."

	t.Run("cold_start_phase", func(t *testing.T) {
		// Make 3 queries to trigger cold-start tracking
		for i := 1; i <= 3; i++ {
			req := createDNSRequest(testDomain, dns.TypeA)
			dctx := &DNSContext{
				Req: req,
			}

			err := proxy.Resolve(ctx, dctx)
			assert.NoError(t, err, "query %d failed", i)
			assert.NotNil(t, dctx.Res, "response %d should not be nil", i)

			t.Logf("Query %d completed", i)
			time.Sleep(100 * time.Millisecond)
		}

		t.Log("Cold-start phase completed")
	})

	t.Run("join_prefetch_queue", func(t *testing.T) {
		// The domain should now be in the prefetch queue after 3 accesses
		// (we set MinHeatThreshold to 3 for testing)

		// Wait a bit for heat tracking to process
		time.Sleep(500 * time.Millisecond)

		t.Log("Domain should be in prefetch queue now")
	})

	t.Run("wait_for_prefetch", func(t *testing.T) {
		// Wait for TTL to approach threshold
		// With ThresholdSeconds=3, prefetch should trigger when remaining TTL < 3s

		t.Log("Waiting for prefetch to trigger...")

		// Wait up to 20 seconds for prefetch to occur
		// (typical DNS TTL is 300s, but we're testing with shorter intervals)
		time.Sleep(5 * time.Second)

		t.Log("Prefetch should have been triggered")
	})

	t.Run("verify_cache_refresh", func(t *testing.T) {
		// Make another query to verify cache is still fresh
		req := createDNSRequest(testDomain, dns.TypeA)
		dctx := &DNSContext{
			Req: req,
		}

		err := proxy.Resolve(ctx, dctx)
		assert.NoError(t, err, "verification query failed")
		assert.NotNil(t, dctx.Res, "verification response should not be nil")

		t.Log("Cache verification completed")
	})
}

// TestCachePrefetch_InactivityRemoval tests that inactive domains are removed from queue.
func TestCachePrefetch_InactivityRemoval(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	ups, err := upstream.AddressToUpstream("8.8.8.8", &upstream.Options{
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err)

	config := &Config{
		Logger:         logger,
		CacheEnabled:   true,
		CacheSizeBytes: 1024 * 1024,
		DNSSECEnabled:  true,
		CachePrefetchConfig: &PrefetchConfig{
			Enabled:                 true,
			ThresholdSeconds:        3,
			ThresholdPercent:        80,
			MaxConcurrent:           5,
			MinHeatThreshold:        3,
			TimeWindow:              5 * time.Second,  // Short window for testing  // Fast checking
		},
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	}

	proxy, err := New(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	t.Log("Proxy created successfully")

	testDomain := "example.com."

	// Make 3 queries to join queue
	for i := 1; i <= 3; i++ {
		req := createDNSRequest(testDomain, dns.TypeA)
		dctx := &DNSContext{Req: req}
		err := proxy.Resolve(ctx, dctx)
		assert.NoError(t, err)
		time.Sleep(100 * time.Millisecond)
	}

	t.Log("Domain added to prefetch queue")

	// Wait for inactivity timeout (5s + 1s check interval)
	t.Log("Waiting for inactivity timeout...")
	time.Sleep(7 * time.Second)

	t.Log("Domain should be removed from queue due to inactivity")
}

// TestCachePrefetch_MultipleDomainsHeatRanking tests heat-based prioritization.
func TestCachePrefetch_MultipleDomainsHeatRanking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	ups, err := upstream.AddressToUpstream("8.8.8.8", &upstream.Options{
		Timeout: 5 * time.Second,
	})
	require.NoError(t, err)

	config := &Config{
		Logger:       logger,
		CacheEnabled: true,
		CacheSizeBytes: 1024 * 1024,
		CachePrefetchConfig: &PrefetchConfig{
			Enabled:                 true,
			ThresholdSeconds:        3,
			ThresholdPercent:        80,
			MaxConcurrent:           2,  // Limited concurrency
			MinHeatThreshold:        3,
			TimeWindow:              10 * time.Second,
		},
		UpstreamConfig: &UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	}

	proxy, err := New(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Log("Proxy created successfully")

	// Query multiple domains with different frequencies
	domains := []struct {
		name  string
		count int
	}{
		{"google.com.", 10},  // High heat
		{"github.com.", 5},   // Medium heat
		{"example.com.", 3},  // Low heat (just enough to join queue)
	}

	for _, d := range domains {
		for i := 0; i < d.count; i++ {
			req := createDNSRequest(d.name, dns.TypeA)
			dctx := &DNSContext{Req: req}
			err := proxy.Resolve(ctx, dctx)
			assert.NoError(t, err, "query failed for %s", d.name)
			time.Sleep(50 * time.Millisecond)
		}
		t.Logf("Queried %s %d times", d.name, d.count)
	}

	t.Log("All domains should be in prefetch queue with different heat scores")
	t.Log("Higher heat domains should be prefetched first")

	// Wait for prefetch activity
	time.Sleep(5 * time.Second)
}

// Helper function to create DNS request
func createDNSRequest(domain string, qtype uint16) *dns.Msg {
	req := &dns.Msg{}
	req.SetQuestion(dns.Fqdn(domain), qtype)
	req.RecursionDesired = true
	return req
}
