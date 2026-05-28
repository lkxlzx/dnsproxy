package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
)

func main() {
	fmt.Println("=== DNSProxy Full Feature Test ===\n")

	// Test 1: Prefetch
	fmt.Println("[Test 1] Prefetch Feature")
	testPrefetch()

	// Test 2: Domain Groups
	fmt.Println("\n[Test 2] Domain Groups")
	testDomainGroups()

	// Test 3: Keyword Matching
	fmt.Println("\n[Test 3] Keyword Matching")
	testKeywordMatching()

	// Test 4: Wildcard Matching
	fmt.Println("\n[Test 4] Wildcard Matching")
	testWildcardMatching()

	fmt.Println("\n=== All Tests Completed ===")
}

func testPrefetch() {
	ups, _ := upstream.AddressToUpstream("8.8.8.8", &upstream.Options{Timeout: 5 * time.Second})

	config := &proxy.Config{
		UDPListenAddr: []*net.UDPAddr{{IP: net.ParseIP("127.0.0.1"), Port: 15353}},
		UpstreamConfig: &proxy.UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
		CacheEnabled:   true,
		CacheSizeBytes: 10 * 1024 * 1024,
		CachePrefetchConfig: &proxy.PrefetchConfig{
			Enabled:          true,
			ThresholdSeconds: 3,
			ThresholdPercent: 80,
			MaxConcurrent:    5,
			MinHeatThreshold: 3,
			TimeWindow:       30 * time.Second,
			MaxRetries:       2,
		},
	}

	dnsProxy, err := proxy.New(config)
	if err != nil {
		log.Printf("  X Failed to create proxy: %v", err)
		return
	}

	ctx := context.Background()
	if err := dnsProxy.Start(ctx); err != nil {
		log.Printf("  X Failed to start proxy: %v", err)
		return
	}
	defer dnsProxy.Shutdown(ctx)

	domain := "google.com."
	fmt.Printf("  Testing domain: %s\n", domain)

	// Access 3 times to trigger prefetch queue
	for i := 1; i <= 3; i++ {
		resp, err := query(dnsProxy, domain)
		if err != nil {
			fmt.Printf("  X Query %d failed: %v\n", i, err)
			return
		}
		fmt.Printf("  + Query %d: TTL=%ds\n", i, getTTL(resp))
		time.Sleep(300 * time.Millisecond)
	}

	// Wait for TTL to approach threshold
	fmt.Println("  Waiting 3 seconds...")
	time.Sleep(3 * time.Second)

	// Trigger prefetch
	resp, _ := query(dnsProxy, domain)
	fmt.Printf("  + Trigger prefetch: TTL=%ds\n", getTTL(resp))

	// Verify refresh
	time.Sleep(1 * time.Second)
	resp, _ = query(dnsProxy, domain)
	ttl := getTTL(resp)
	fmt.Printf("  + Verify refresh: TTL=%ds\n", ttl)

	if ttl > 5 {
		fmt.Println("  + Prefetch SUCCESS")
	} else {
		fmt.Println("  ! Prefetch may not have triggered")
	}
}

func testDomainGroups() {
	ups1, _ := upstream.AddressToUpstream("8.8.8.8", &upstream.Options{Timeout: 5 * time.Second})

	config := &proxy.Config{
		UDPListenAddr: []*net.UDPAddr{{IP: net.ParseIP("127.0.0.1"), Port: 15354}},
		UpstreamConfig: &proxy.UpstreamConfig{
			Upstreams: []upstream.Upstream{ups1},
		},
		CacheEnabled:   true,
		CacheSizeBytes: 10 * 1024 * 1024,
		DomainGroups: []proxy.DomainGroupConfig{
			{
				GroupName:  "china",
				DomainFile: "china_domains.txt",
				Upstreams:  []string{"223.5.5.5"},
				Enabled:    true,
			},
		},
	}

	dnsProxy, err := proxy.New(config)
	if err != nil {
		log.Printf("  X Failed to create proxy: %v", err)
		return
	}

	ctx := context.Background()
	if err := dnsProxy.Start(ctx); err != nil {
		log.Printf("  X Failed to start proxy: %v", err)
		return
	}
	defer dnsProxy.Shutdown(ctx)

	domains := []string{"baidu.com.", "taobao.com.", "qq.com."}
	for _, domain := range domains {
		resp, err := query(dnsProxy, domain)
		if err != nil {
			fmt.Printf("  X %s query failed: %v\n", domain, err)
			continue
		}
		fmt.Printf("  + %s -> %d records\n", domain, len(resp.Answer))
	}

	// Test enable/disable
	fmt.Println("  Testing enable/disable...")
	if err := dnsProxy.DisableDomainGroup("china"); err != nil {
		fmt.Printf("  X Disable failed: %v\n", err)
	} else {
		fmt.Println("  + Disabled china group")
	}

	if err := dnsProxy.EnableDomainGroup("china"); err != nil {
		fmt.Printf("  X Enable failed: %v\n", err)
	} else {
		fmt.Println("  + Enabled china group")
	}
}

func testKeywordMatching() {
	ups, _ := upstream.AddressToUpstream("8.8.8.8", &upstream.Options{Timeout: 5 * time.Second})

	config := &proxy.Config{
		UDPListenAddr: []*net.UDPAddr{{IP: net.ParseIP("127.0.0.1"), Port: 15355}},
		UpstreamConfig: &proxy.UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
		CacheEnabled:   true,
		CacheSizeBytes: 10 * 1024 * 1024,
		DomainGroups: []proxy.DomainGroupConfig{
			{
				GroupName:  "ads",
				DomainFile: "ad_domains.txt",
				Upstreams:  []string{"0.0.0.0"},
				Enabled:    true,
			},
		},
	}

	dnsProxy, err := proxy.New(config)
	if err != nil {
		log.Printf("  X Failed to create proxy: %v", err)
		return
	}

	ctx := context.Background()
	if err := dnsProxy.Start(ctx); err != nil {
		log.Printf("  X Failed to start proxy: %v", err)
		return
	}
	defer dnsProxy.Shutdown(ctx)

	testCases := []string{
		"ad-server.example.com.",
		"tracker.analytics.com.",
		"normal-site.com.",
	}

	for _, domain := range testCases {
		resp, err := query(dnsProxy, domain)
		if err != nil {
			fmt.Printf("  X %s query failed\n", domain)
			continue
		}

		blocked := false
		if len(resp.Answer) > 0 {
			if aRecord, ok := resp.Answer[0].(*dns.A); ok {
				if aRecord.A.String() == "0.0.0.0" {
					blocked = true
				}
			}
		}

		if blocked {
			fmt.Printf("  + %s -> BLOCKED (0.0.0.0)\n", domain)
		} else {
			fmt.Printf("  + %s -> Normal (%d records)\n", domain, len(resp.Answer))
		}
	}
}

func testWildcardMatching() {
	ups, _ := upstream.AddressToUpstream("8.8.8.8", &upstream.Options{Timeout: 5 * time.Second})

	config := &proxy.Config{
		UDPListenAddr: []*net.UDPAddr{{IP: net.ParseIP("127.0.0.1"), Port: 15356}},
		UpstreamConfig: &proxy.UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
		CacheEnabled:   true,
		CacheSizeBytes: 10 * 1024 * 1024,
		DomainGroups: []proxy.DomainGroupConfig{
			{
				GroupName:  "wildcard",
				DomainFile: "test_wildcard_domains.txt",
				Upstreams:  []string{"8.8.4.4"},
				Enabled:    true,
			},
		},
	}

	dnsProxy, err := proxy.New(config)
	if err != nil {
		log.Printf("  X Failed to create proxy: %v", err)
		return
	}

	ctx := context.Background()
	if err := dnsProxy.Start(ctx); err != nil {
		log.Printf("  X Failed to start proxy: %v", err)
		return
	}
	defer dnsProxy.Shutdown(ctx)

	testCases := []string{
		"test.cdn.example.com.",
		"img.cdn.example.com.",
		"normal.example.com.",
	}

	for _, domain := range testCases {
		resp, err := query(dnsProxy, domain)
		if err != nil {
			fmt.Printf("  X %s query failed\n", domain)
			continue
		}
		fmt.Printf("  + %s -> %d records\n", domain, len(resp.Answer))
	}
}

func query(dnsProxy *proxy.Proxy, domain string) (*dns.Msg, error) {
	req := &dns.Msg{}
	req.SetQuestion(domain, dns.TypeA)
	req.RecursionDesired = true

	ctx := context.Background()
	dctx := &proxy.DNSContext{
		Proto: proxy.ProtoUDP,
		Req:   req,
	}

	if err := dnsProxy.Resolve(ctx, dctx); err != nil {
		return nil, err
	}

	return dctx.Res, nil
}

func getTTL(msg *dns.Msg) uint32 {
	if msg == nil || len(msg.Answer) == 0 {
		return 0
	}
	return msg.Answer[0].Header().Ttl
}
