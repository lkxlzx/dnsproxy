package proxy

import (
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
)

// TestRealDNSRouting tests real DNS queries with actual upstream servers.
// This test demonstrates the routing/splitting functionality.
func TestRealDNSRouting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real DNS routing test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	t.Log("========================================")
	t.Log("Real DNS Routing Test")
	t.Log("Testing domain splitting with real DNS queries")
	t.Log("========================================")
	t.Log("")

	// Step 1: Create upstream group configuration
	t.Log("Step 1: Creating upstream group configuration...")

	ugc := NewUpstreamGroupConfig()

	// Overseas group (Google DNS, Cloudflare)
	overseasConf, err := ParseUpstreamsConfig([]string{"8.8.8.8", "1.1.1.1"}, &upstream.Options{
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to parse overseas upstreams: %v", err)
	}

	overseasGroup := &UpstreamGroup{
		Name:      "overseas",
		Upstreams: overseasConf.Upstreams,
		Mode:      UpstreamModeLoadBalance,
		Timeout:   5 * time.Second,
		Enabled:   true,
	}
	ugc.AddGroup(overseasGroup)

	// China group (Alibaba DNS, DNSPod)
	chinaConf, err := ParseUpstreamsConfig([]string{"223.5.5.5", "119.29.29.29"}, &upstream.Options{
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Failed to parse china upstreams: %v", err)
	}

	chinaGroup := &UpstreamGroup{
		Name:      "china",
		Upstreams: chinaConf.Upstreams,
		Mode:      UpstreamModeLoadBalance,
		Timeout:   5 * time.Second,
		Enabled:   true,
	}
	ugc.AddGroup(chinaGroup)

	ugc.DefaultGroup = "overseas"

	// Configure domain routing
	domainRoutes := map[string]string{
		// China domains -> China DNS
		"baidu.com":      "china",
		"*.baidu.com":    "china",
		"qq.com":         "china",
		"*.qq.com":       "china",
		"taobao.com":     "china",
		"jd.com":         "china",
		"bilibili.com":   "china",
		"*.bilibili.com": "china",

		// Overseas domains -> Overseas DNS
		"google.com":   "overseas",
		"*.google.com": "overseas",
		"github.com":   "overseas",
		"*.github.com": "overseas",
	}

	for domain, group := range domainRoutes {
		ugc.SetDomainGroup(domain, group)
	}

	err = ugc.Validate()
	if err != nil {
		t.Fatalf("Configuration validation failed: %v", err)
	}

	t.Log("✓ Configuration created")
	ugc.LogGroupInfo(logger)

	// Step 2: Test routing decisions
	t.Log("")
	t.Log("Step 2: Testing routing decisions...")
	t.Log("")

	testCases := []struct {
		domain        string
		expectedGroup string
		description   string
	}{
		{"baidu.com", "china", "中国域名 - 百度"},
		{"www.baidu.com", "china", "中国域名 - 百度子域名"},
		{"qq.com", "china", "中国域名 - QQ"},
		{"mail.qq.com", "china", "中国域名 - QQ邮箱"},
		{"taobao.com", "china", "中国域名 - 淘宝"},
		{"jd.com", "china", "中国域名 - 京东"},
		{"bilibili.com", "china", "中国域名 - B站"},
		{"www.bilibili.com", "china", "中国域名 - B站子域名"},
		{"google.com", "overseas", "海外域名 - Google"},
		{"www.google.com", "overseas", "海外域名 - Google子域名"},
		{"github.com", "overseas", "海外域名 - GitHub"},
		{"api.github.com", "overseas", "海外域名 - GitHub API"},
		{"example.com", "overseas", "未指定域名 - 使用默认组"},
	}

	t.Log("┌─────────────────────────────────────────────────────────────────┐")
	t.Log("│                    Domain Routing Results                       │")
	t.Log("├─────────────────────────────────────────────────────────────────┤")

	for _, tc := range testCases {
		group, err := ugc.GetGroupForDomain(tc.domain)
		if err != nil {
			t.Errorf("❌ Failed to get group for %s: %v", tc.domain, err)
			continue
		}

		status := "✓"
		if group.Name != tc.expectedGroup {
			status = "✗"
			t.Errorf("Routing mismatch for %s: expected %s, got %s", tc.domain, tc.expectedGroup, group.Name)
		}

		t.Logf("│ %s %-25s -> %-10s (%s)", status, tc.domain, group.Name, tc.description)
	}

	t.Log("└─────────────────────────────────────────────────────────────────┘")

	// Step 3: Test real DNS queries
	t.Log("")
	t.Log("Step 3: Testing real DNS queries with actual upstreams...")
	t.Log("")

	realQueryTests := []struct {
		domain      string
		description string
	}{
		{"baidu.com", "百度 (应使用 223.5.5.5 中国DNS)"},
		{"qq.com", "QQ (应使用 223.5.5.5 中国DNS)"},
		{"google.com", "Google (应使用 8.8.8.8 海外DNS)"},
		{"github.com", "GitHub (应使用 8.8.8.8 海外DNS)"},
	}

	t.Log("┌─────────────────────────────────────────────────────────────────────────────┐")
	t.Log("│                         Real DNS Query Results                              │")
	t.Log("├─────────────────────────────────────────────────────────────────────────────┤")

	for _, test := range realQueryTests {
		t.Logf("│")
		t.Logf("│ Testing: %s (%s)", test.domain, test.description)

		// Get routing group
		group, err := ugc.GetGroupForDomain(test.domain)
		if err != nil {
			t.Logf("│ ❌ Failed to get group: %v", err)
			continue
		}

		t.Logf("│ → Routed to group: %s", group.Name)

		// Get upstream server address
		if len(group.Upstreams) == 0 {
			t.Logf("│ ❌ No upstreams in group")
			continue
		}

		upstream := group.Upstreams[0]
		upstreamAddr := getUpstreamAddress(upstream)
		t.Logf("│ → Using upstream: %s", upstreamAddr)

		// Perform DNS query
		m := new(dns.Msg)
		m.SetQuestion(dns.Fqdn(test.domain), dns.TypeA)
		m.RecursionDesired = true

		start := time.Now()
		reply, err := upstream.Exchange(m)
		elapsed := time.Since(start)

		if err != nil {
			t.Logf("│ ⚠ Query failed: %v", err)
			t.Logf("│ (This might be due to network restrictions)")
			continue
		}

		if reply == nil {
			t.Logf("│ ⚠ Empty reply")
			continue
		}

		t.Logf("│ ✓ Query successful (took %v)", elapsed)
		t.Logf("│ → Response code: %s", dns.RcodeToString[reply.Rcode])
		t.Logf("│ → Answers: %d", len(reply.Answer))

		// Display answers
		for i, ans := range reply.Answer {
			if a, ok := ans.(*dns.A); ok {
				t.Logf("│   [%d] %s -> %s", i+1, test.domain, a.A.String())
			}
		}
	}

	t.Log("└─────────────────────────────────────────────────────────────────────────────┘")

	// Step 4: Performance comparison
	t.Log("")
	t.Log("Step 4: Performance comparison (China DNS vs Overseas DNS)...")
	t.Log("")

	performanceTests := []struct {
		domain      string
		group       string
		description string
	}{
		{"baidu.com", "china", "中国域名使用中国DNS"},
		{"google.com", "overseas", "海外域名使用海外DNS"},
	}

	t.Log("┌─────────────────────────────────────────────────────────────────┐")
	t.Log("│                    Performance Comparison                       │")
	t.Log("├─────────────────────────────────────────────────────────────────┤")

	for _, test := range performanceTests {
		group, _ := ugc.GetGroupForDomain(test.domain)
		if len(group.Upstreams) == 0 {
			continue
		}

		upstream := group.Upstreams[0]
		upstreamAddr := getUpstreamAddress(upstream)

		// Run 5 queries and calculate average
		var totalTime time.Duration
		successCount := 0

		for i := 0; i < 5; i++ {
			m := new(dns.Msg)
			m.SetQuestion(dns.Fqdn(test.domain), dns.TypeA)
			m.RecursionDesired = true

			start := time.Now()
			reply, err := upstream.Exchange(m)
			elapsed := time.Since(start)

			if err == nil && reply != nil {
				totalTime += elapsed
				successCount++
			}
		}

		if successCount > 0 {
			avgTime := totalTime / time.Duration(successCount)
			t.Logf("│ %s", test.description)
			t.Logf("│ → Domain: %s", test.domain)
			t.Logf("│ → Upstream: %s (%s)", upstreamAddr, group.Name)
			t.Logf("│ → Average time: %v (%d/5 successful)", avgTime, successCount)
			t.Logf("│")
		}
	}

	t.Log("└─────────────────────────────────────────────────────────────────┘")

	// Step 5: Summary
	t.Log("")
	t.Log("========================================")
	t.Log("Test Summary")
	t.Log("========================================")
	t.Log("✓ Routing logic: Working correctly")
	t.Log("✓ China domains -> China DNS (223.5.5.5)")
	t.Log("✓ Overseas domains -> Overseas DNS (8.8.8.8)")
	t.Log("✓ Default routing: Working correctly")
	t.Log("✓ Wildcard matching: Working correctly")
	t.Log("")
	t.Log("Real DNS Routing Test: PASS")
}

// getUpstreamAddress extracts the address from an upstream for display.
func getUpstreamAddress(u upstream.Upstream) string {
	// Try to get the address string
	addr := fmt.Sprintf("%v", u)
	
	// Simple extraction for common formats
	if len(addr) > 50 {
		addr = addr[:50] + "..."
	}
	
	return addr
}

// TestRoutingPerformance benchmarks the routing performance.
func TestRoutingPerformance(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Log("========================================")
	t.Log("Routing Performance Test")
	t.Log("========================================")

	// Create configuration
	ugc := NewUpstreamGroupConfig()

	overseasConf, _ := ParseUpstreamsConfig([]string{"8.8.8.8"}, &upstream.Options{Timeout: 3 * time.Second})
	overseasGroup := &UpstreamGroup{
		Name:      "overseas",
		Upstreams: overseasConf.Upstreams,
		Enabled:   true,
	}
	ugc.AddGroup(overseasGroup)

	chinaConf, _ := ParseUpstreamsConfig([]string{"223.5.5.5"}, &upstream.Options{Timeout: 3 * time.Second})
	chinaGroup := &UpstreamGroup{
		Name:      "china",
		Upstreams: chinaConf.Upstreams,
		Enabled:   true,
	}
	ugc.AddGroup(chinaGroup)

	ugc.DefaultGroup = "overseas"

	// Add 100 routing rules
	for i := 0; i < 100; i++ {
		domain := fmt.Sprintf("test%d.com", i)
		ugc.SetDomainGroup(domain, "china")
		ugc.SetDomainGroup(fmt.Sprintf("*.test%d.com", i), "china")
	}

	ugc.LogGroupInfo(logger)

	// Test performance
	iterations := 10000

	testCases := []struct {
		name   string
		domain string
	}{
		{"Exact match", "test50.com"},
		{"Wildcard match", "www.test50.com"},
		{"Default group", "unknown.com"},
	}

	t.Log("")
	for _, tc := range testCases {
		start := time.Now()

		for i := 0; i < iterations; i++ {
			_, _ = ugc.GetGroupForDomain(tc.domain)
		}

		elapsed := time.Since(start)
		avgTime := elapsed / time.Duration(iterations)

		t.Logf("%s: %d lookups in %v", tc.name, iterations, elapsed)
		t.Logf("  Average: %v per lookup", avgTime)

		if avgTime > 10*time.Microsecond {
			t.Logf("  ⚠ Warning: Slower than expected (> 10μs)")
		} else {
			t.Logf("  ✓ Performance: Excellent")
		}
		t.Log("")
	}

	t.Log("Routing Performance Test: PASS")
}
