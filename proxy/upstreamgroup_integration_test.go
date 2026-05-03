package proxy

import (
	"log/slog"
	"os"
	"testing"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
)

// TestUpstreamGroupIntegration tests the complete domain grouping workflow.
func TestUpstreamGroupIntegration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create upstream groups configuration
	spec := &UpstreamGroupsSpec{
		DefaultGroup: "overseas",
		Groups: []UpstreamGroupSpec{
			{
				Name: "overseas",
				Upstreams: []string{
					"8.8.8.8",
					"1.1.1.1",
				},
				Mode:    "load_balance",
				Timeout: "5s",
				Enabled: true,
			},
			{
				Name: "china",
				Upstreams: []string{
					"223.5.5.5",
					"119.29.29.29",
				},
				Mode:    "load_balance",
				Timeout: "5s",
				Enabled: true,
			},
		},
		DomainGroups: map[string]interface{}{
			// Overseas domains
			"google.com":      "overseas",
			"*.google.com":    "overseas",
			"youtube.com":     "overseas",
			"*.youtube.com":   "overseas",
			"facebook.com":    "overseas",
			"*.facebook.com":  "overseas",
			
			// China domains
			"baidu.com":       "china",
			"*.baidu.com":     "china",
			"qq.com":          "china",
			"*.qq.com":        "china",
			"taobao.com":      "china",
			"*.taobao.com":    "china",
			"jd.com":          "china",
			"*.jd.com":        "china",
		},
	}

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: 5000,
	}

	// Parse configuration
	ugc, err := ParseUpstreamGroups(spec, opts)
	if err != nil {
		t.Fatalf("ParseUpstreamGroups failed: %v", err)
	}

	t.Logf("Configuration loaded successfully")
	t.Logf("Groups: %d", len(ugc.Groups))
	t.Logf("Domain mappings: %d", len(ugc.DomainGroups))
	
	// Debug: print all domain mappings
	t.Log("Domain mappings:")
	for domain, group := range ugc.DomainGroups {
		t.Logf("  %q -> %q", domain, group)
	}

	// Test cases
	testCases := []struct {
		domain        string
		expectedGroup string
		description   string
	}{
		// Overseas domains
		{
			domain:        "google.com.",
			expectedGroup: "overseas",
			description:   "Google (exact match)",
		},
		{
			domain:        "www.google.com.",
			expectedGroup: "overseas",
			description:   "Google subdomain (wildcard match)",
		},
		{
			domain:        "youtube.com.",
			expectedGroup: "overseas",
			description:   "YouTube (exact match)",
		},
		{
			domain:        "www.youtube.com.",
			expectedGroup: "overseas",
			description:   "YouTube subdomain (wildcard match)",
		},
		{
			domain:        "facebook.com.",
			expectedGroup: "overseas",
			description:   "Facebook (exact match)",
		},
		
		// China domains
		{
			domain:        "baidu.com.",
			expectedGroup: "china",
			description:   "Baidu (exact match)",
		},
		{
			domain:        "www.baidu.com.",
			expectedGroup: "china",
			description:   "Baidu subdomain (wildcard match)",
		},
		{
			domain:        "qq.com.",
			expectedGroup: "china",
			description:   "QQ (exact match)",
		},
		{
			domain:        "mail.qq.com.",
			expectedGroup: "china",
			description:   "QQ subdomain (wildcard match)",
		},
		{
			domain:        "taobao.com.",
			expectedGroup: "china",
			description:   "Taobao (exact match)",
		},
		{
			domain:        "www.taobao.com.",
			expectedGroup: "china",
			description:   "Taobao subdomain (wildcard match)",
		},
		{
			domain:        "jd.com.",
			expectedGroup: "china",
			description:   "JD (exact match)",
		},
		
		// Default group (unspecified domains)
		{
			domain:        "example.com.",
			expectedGroup: "overseas",
			description:   "Unspecified domain (default group)",
		},
		{
			domain:        "test.local.",
			expectedGroup: "overseas",
			description:   "Local domain (default group)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// Get group for domain
			group, matched := ugc.GetGroupForDomain(tc.domain)
			
			if group == nil {
				t.Errorf("GetGroupForDomain(%q) returned nil", tc.domain)
				return
			}

			if group.Name != tc.expectedGroup {
				t.Errorf("Domain %q: expected group %q, got %q (matched: %v)", 
					tc.domain, tc.expectedGroup, group.Name, matched)
			} else {
				t.Logf("✓ Domain %q correctly routed to group %q (matched: %v)", 
					tc.domain, group.Name, matched)
			}

			// Verify upstreams
			if len(group.Upstreams) == 0 {
				t.Errorf("Group %q has no upstreams", group.Name)
			} else {
				t.Logf("  Group %q has %d upstream(s)", 
					group.Name, len(group.Upstreams))
			}
		})
	}
}

// TestUpstreamGroupDNSQuery tests actual DNS query routing.
func TestUpstreamGroupDNSQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Create configuration
	spec := &UpstreamGroupsSpec{
		DefaultGroup: "overseas",
		Groups: []UpstreamGroupSpec{
			{
				Name: "overseas",
				Upstreams: []string{
					"8.8.8.8",
				},
				Mode:    "load_balance",
				Timeout: "5s",
				Enabled: true,
			},
			{
				Name: "china",
				Upstreams: []string{
					"223.5.5.5",
				},
				Mode:    "load_balance",
				Timeout: "5s",
				Enabled: true,
			},
		},
		DomainGroups: map[string]interface{}{
			"google.com":   "overseas",
			"*.google.com": "overseas",
			"baidu.com":    "china",
			"*.baidu.com":  "china",
		},
	}

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: 5000,
	}

	ugc, err := ParseUpstreamGroups(spec, opts)
	if err != nil {
		t.Fatalf("ParseUpstreamGroups failed: %v", err)
	}

	// Test DNS queries
	testQueries := []struct {
		domain        string
		expectedGroup string
	}{
		{"google.com.", "overseas"},
		{"www.google.com.", "overseas"},
		{"baidu.com.", "china"},
		{"www.baidu.com.", "china"},
	}

	for _, tq := range testQueries {
		t.Run(tq.domain, func(t *testing.T) {
			// Create DNS query
			req := &dns.Msg{}
			req.SetQuestion(tq.domain, dns.TypeA)

			// Get group
			group, matched := ugc.GetGroupForDomain(tq.domain)
			if group == nil {
				t.Fatalf("No group found for domain %q", tq.domain)
			}

			if group.Name != tq.expectedGroup {
				t.Errorf("Expected group %q, got %q", tq.expectedGroup, group.Name)
			}

			t.Logf("Domain %q -> Group %q -> Upstream %s (matched: %v)", 
				tq.domain, group.Name, group.Upstreams[0].Address(), matched)

			// Try to query (may fail if no network, but that's ok for this test)
			resp, err := group.Upstreams[0].Exchange(req)
			if err != nil {
				t.Logf("Query failed (expected in test environment): %v", err)
			} else if resp != nil {
				t.Logf("Query successful, got %d answer(s)", len(resp.Answer))
			}
		})
	}
}
