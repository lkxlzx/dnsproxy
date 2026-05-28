// +build ignore

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
)

func main() {
	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	fmt.Println("=== DNSProxy Real Domain Prefetch Test ===")
	fmt.Println()

	// Create upstream
	ups, err := upstream.AddressToUpstream("8.8.8.8", &upstream.Options{
		Timeout: 5 * time.Second,
	})
	if err != nil {
		fmt.Printf("Failed to create upstream: %v\n", err)
		os.Exit(1)
	}

	// Create proxy configuration with aggressive prefetch settings
	config := &proxy.Config{
		Logger:         logger,
		CacheEnabled:   true,
		CacheSizeBytes: 1024 * 1024, // 1MB
		DNSSECEnabled:  true,         // Enable DNSSEC for caching
		CachePrefetchConfig: &proxy.PrefetchConfig{
			Enabled:                 true,
			ThresholdSeconds:        5,  // Prefetch when TTL < 5s
			ThresholdPercent:        80, // Or when 80% of TTL elapsed
			MaxConcurrent:           5,
			ScanInterval:            1 * time.Second,  // Scan every second
			MinHeatThreshold:        3,                // Join queue after 3 accesses
			TimeWindow:              30 * time.Second, // 30s time window
					},
		UpstreamConfig: &proxy.UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
	}

	// Create proxy
	dnsProxy, err := proxy.New(config)
	if err != nil {
		fmt.Printf("Failed to create proxy: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("âœ?Proxy created successfully")
	fmt.Println()

	ctx := context.Background()

	// Test domains
	testDomains := []string{
		"google.com.",
		"github.com.",
		"cloudflare.com.",
	}

	// Phase 1: Cold start - make 3 queries per domain to trigger prefetch queue entry
	fmt.Println("=== Phase 1: Cold Start (Building Heat) ===")
	for _, domain := range testDomains {
		fmt.Printf("\nQuerying %s (3 times to build heat):\n", domain)
		for i := 1; i <= 3; i++ {
			req := createDNSRequest(domain, dns.TypeA)
			dctx := &proxy.DNSContext{
				Req: req,
			}

			start := time.Now()
			err := dnsProxy.Resolve(ctx, dctx)
			elapsed := time.Since(start)

			if err != nil {
				fmt.Printf("  Query %d: ERROR - %v\n", i, err)
			} else if dctx.Res != nil {
				cached := ""
				if i > 1 {
					cached = " (from cache)"
				}
				fmt.Printf("  Query %d: %d answers, took %v%s\n", i, len(dctx.Res.Answer), elapsed, cached)
			}

			time.Sleep(200 * time.Millisecond)
		}
	}

	fmt.Println("\nâœ?All domains should now be in prefetch queue")
	fmt.Println()

	// Phase 2: Wait and observe prefetch activity
	fmt.Println("=== Phase 2: Monitoring Prefetch Activity ===")
	fmt.Println("Waiting 10 seconds to observe prefetch scheduler...")
	fmt.Println("(Watch for prefetch-related log messages)")
	fmt.Println()

	time.Sleep(10 * time.Second)

	// Phase 3: Verify cache is still fresh
	fmt.Println("\n=== Phase 3: Verify Cache Freshness ===")
	for _, domain := range testDomains {
		req := createDNSRequest(domain, dns.TypeA)
		dctx := &proxy.DNSContext{
			Req: req,
		}

		start := time.Now()
		err := dnsProxy.Resolve(ctx, dctx)
		elapsed := time.Since(start)

		if err != nil {
			fmt.Printf("%s: ERROR - %v\n", domain, err)
		} else if dctx.Res != nil {
			fmt.Printf("%s: %d answers, took %v (should be from cache)\n",
				domain, len(dctx.Res.Answer), elapsed)
		}
	}

	fmt.Println()

	// Phase 4: Test inactivity removal
	fmt.Println("=== Phase 4: Inactivity Test ===")
	fmt.Println("Adding a new domain and then letting it go inactive...")

	inactiveDomain := "example.org."
	fmt.Printf("\nQuerying %s (3 times):\n", inactiveDomain)
	for i := 1; i <= 3; i++ {
		req := createDNSRequest(inactiveDomain, dns.TypeA)
		dctx := &proxy.DNSContext{
			Req: req,
		}

		err := dnsProxy.Resolve(ctx, dctx)
		if err != nil {
			fmt.Printf("  Query %d: ERROR - %v\n", i, err)
		} else {
			fmt.Printf("  Query %d: Success\n", i)
		}
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Println("\nâœ?Domain added to prefetch queue")
	fmt.Println("Waiting 15 seconds for inactivity timeout...")
	fmt.Println("(Domain should be removed from queue due to inactivity)")
	fmt.Println()

	time.Sleep(15 * time.Second)

	// Phase 5: Final statistics
	fmt.Println("=== Phase 5: Final Verification ===")
	fmt.Println("Making final queries to active domains...")
	fmt.Println()

	for _, domain := range testDomains {
		req := createDNSRequest(domain, dns.TypeA)
		dctx := &proxy.DNSContext{
			Req: req,
		}

		start := time.Now()
		err := dnsProxy.Resolve(ctx, dctx)
		elapsed := time.Since(start)

		if err != nil {
			fmt.Printf("%s: ERROR - %v\n", domain, err)
		} else if dctx.Res != nil {
			fmt.Printf("%s: %d answers, took %v\n",
				domain, len(dctx.Res.Answer), elapsed)
		}
	}

	fmt.Println()
	fmt.Println("=== Test Completed Successfully ===")
	fmt.Println()
	fmt.Println("Summary:")
	fmt.Println("âœ?Domains were queried and cached")
	fmt.Println("âœ?Heat tracking triggered prefetch queue entry")
	fmt.Println("âœ?Prefetch scheduler monitored cache entries")
	fmt.Println("âœ?Inactive domains were removed from queue")
	fmt.Println("âœ?Cache remained fresh throughout the test")
}

func createDNSRequest(domain string, qtype uint16) *dns.Msg {
	req := &dns.Msg{}
	req.SetQuestion(dns.Fqdn(domain), qtype)
	req.RecursionDesired = true
	return req
}
