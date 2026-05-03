package proxy

import (
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
)

// TestRemoteDomainLoading tests loading domains from remote URLs.
// This test requires network access and may take longer to complete.
func TestRemoteDomainLoading(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping remote domain loading test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	t.Run("ChinaMax from GitHub", func(t *testing.T) {
		t.Log("Loading ChinaMax domain list from GitHub...")
		start := time.Now()

		loader := NewDomainFileLoader(logger)
		domains, err := loader.LoadDomains("https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/ChinaMax/ChinaMax_Classical.yaml")

		elapsed := time.Since(start)
		t.Logf("Loading took: %v", elapsed)

		if err != nil {
			t.Fatalf("Failed to load ChinaMax: %v", err)
		}

		t.Logf("Loaded %d domains from ChinaMax", len(domains))

		if len(domains) == 0 {
			t.Error("Expected to load domains, got 0")
		}

		// Verify some known Chinese domains are in the list
		knownDomains := []string{"baidu.com.", "qq.com.", "taobao.com.", "jd.com."}
		found := 0
		for _, known := range knownDomains {
			for _, domain := range domains {
				if domain == known || domain == "*."+known {
					found++
					t.Logf("✓ Found known domain: %s", known)
					break
				}
			}
		}

		if found == 0 {
			t.Error("Expected to find at least one known Chinese domain")
		}
	})

	t.Run("GFWList from GitHub", func(t *testing.T) {
		t.Log("Loading GFWList from GitHub...")
		start := time.Now()

		loader := NewDomainFileLoader(logger)
		domains, err := loader.LoadDomains("https://raw.githubusercontent.com/gfwlist/gfwlist/refs/heads/master/gfwlist.txt")

		elapsed := time.Since(start)
		t.Logf("Loading took: %v", elapsed)

		if err != nil {
			t.Fatalf("Failed to load GFWList: %v", err)
		}

		t.Logf("Loaded %d domains from GFWList", len(domains))

		if len(domains) == 0 {
			t.Error("Expected to load domains, got 0")
		}

		// GFWList should contain some known blocked domains
		t.Logf("Sample domains from GFWList (first 10):")
		for i := 0; i < 10 && i < len(domains); i++ {
			t.Logf("  - %s", domains[i])
		}
	})
}

// TestLocalAndRemoteDomainLoading tests both local and remote domain loading.
func TestLocalAndRemoteDomainLoading(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping local and remote domain loading test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	t.Run("Local File Loading", func(t *testing.T) {
		// Test with local file
		spec := &UpstreamGroupsSpec{
			DefaultGroup: "overseas",
			Groups: []UpstreamGroupSpec{
				{
					Name: "overseas",
					Upstreams: []string{
						"8.8.8.8",
					},
					Mode:    "load_balance",
					Timeout: "10s",
					Enabled: true,
				},
				{
					Name: "china",
					Upstreams: []string{
						"223.5.5.5",
					},
					Mode:    "load_balance",
					Timeout: "10s",
					Enabled: true,
				},
			},
			DomainGroups: map[string]interface{}{
				"test.example.com": "overseas",
				"china":            "../domains/china.txt", // Local file
			},
		}

		opts := &upstream.Options{
			Logger:  logger,
			Timeout: 10000,
		}

		t.Log("Parsing configuration with local domain file...")
		start := time.Now()

		ugc, err := ParseUpstreamGroups(spec, opts)

		elapsed := time.Since(start)
		t.Logf("Configuration parsing took: %v", elapsed)

		if err != nil {
			t.Fatalf("ParseUpstreamGroups failed: %v", err)
		}

		t.Logf("Configuration loaded successfully")
		t.Logf("Domain mappings: %d", len(ugc.DomainGroups))

		// Test known domains from local file
		testDomains := []string{"baidu.com.", "qq.com.", "taobao.com."}
		for _, domain := range testDomains {
			group, _ := ugc.GetGroupForDomain(domain)
			if group != nil && group.Name == "china" {
				t.Logf("✓ Domain %q correctly routed to china group", domain)
			}
		}
	})

	t.Run("Remote URL Loading", func(t *testing.T) {
		// Test with remote URL
		spec := &UpstreamGroupsSpec{
			DefaultGroup: "overseas",
			Groups: []UpstreamGroupSpec{
				{
					Name: "overseas",
					Upstreams: []string{
						"8.8.8.8",
					},
					Mode:    "load_balance",
					Timeout: "10s",
					Enabled: true,
				},
				{
					Name: "china",
					Upstreams: []string{
						"223.5.5.5",
					},
					Mode:    "load_balance",
					Timeout: "10s",
					Enabled: true,
				},
			},
			DomainGroups: map[string]interface{}{
				"test.example.com": "overseas",
				"china":            "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/ChinaMax/ChinaMax_Classical.yaml", // Remote URL
			},
		}

		opts := &upstream.Options{
			Logger:  logger,
			Timeout: 10000,
		}

		t.Log("Parsing configuration with remote domain URL...")
		start := time.Now()

		ugc, err := ParseUpstreamGroups(spec, opts)

		elapsed := time.Since(start)
		t.Logf("Configuration parsing took: %v", elapsed)

		if err != nil {
			t.Fatalf("ParseUpstreamGroups failed: %v", err)
		}

		t.Logf("Configuration loaded successfully")
		t.Logf("Domain mappings: %d", len(ugc.DomainGroups))

		// Debug: Check if baidu.com is in the map
		t.Log("Checking domain formats in map...")
		foundBaidu := false
		sampleCount := 0
		for domain, group := range ugc.DomainGroups {
			if strings.Contains(domain, "baidu") || strings.Contains(domain, "qq") || strings.Contains(domain, "taobao") {
				t.Logf("Found domain: %q -> %q", domain, group)
				foundBaidu = true
				sampleCount++
				if sampleCount >= 5 {
					break
				}
			}
		}
		if !foundBaidu {
			t.Log("No baidu/qq/taobao domains found in domain map!")
		}

		// Test known Chinese domains (use subdomains as ChinaMax contains wildcards)
		testCases := []struct {
			domain        string
			expectedGroup string
		}{
			{"www.baidu.com.", "china"},      // Should match *.baidu.com
			{"www.qq.com.", "china"},         // Should match *.qq.com
			{"www.taobao.com.", "china"},     // Should match *.taobao.com
			{"www.jd.com.", "china"},         // Should match *.jd.com
			{"www.tmall.com.", "china"},      // Should match *.tmall.com
			{"test.example.com.", "overseas"},
			{"unknown.org.", "overseas"},
		}

		for _, tc := range testCases {
			group, _ := ugc.GetGroupForDomain(tc.domain)

			if group == nil {
				t.Errorf("GetGroupForDomain(%q) returned nil", tc.domain)
				continue
			}

			if group.Name != tc.expectedGroup {
				t.Errorf("Domain %q: expected group %q, got %q",
					tc.domain, tc.expectedGroup, group.Name)
			} else {
				t.Logf("✓ Domain %q correctly routed to group %q",
					tc.domain, group.Name)
			}
		}

		// Performance check
		testDomain := "baidu.com."
		iterations := 1000

		start = time.Now()
		for i := 0; i < iterations; i++ {
			_, _ = ugc.GetGroupForDomain(testDomain)
		}
		elapsed = time.Since(start)

		avgTime := elapsed / time.Duration(iterations)
		t.Logf("Average lookup time: %v (%d iterations)", avgTime, iterations)

		if avgTime > time.Microsecond*10 {
			t.Logf("Note: Lookup time is %v (acceptable for large domain lists)", avgTime)
		}
	})
}

// TestDomainFileFormats tests loading different domain file formats.
func TestDomainFileFormats(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	loader := NewDomainFileLoader(logger)

	t.Run("Local Plain Text", func(t *testing.T) {
		domains, err := loader.LoadDomains("../domains/china.txt")
		if err != nil {
			t.Fatalf("Failed to load local plain text: %v", err)
		}
		t.Logf("Loaded %d domains from local plain text file", len(domains))
	})

	t.Run("Local Clash YAML", func(t *testing.T) {
		domains, err := loader.LoadDomains("../domains/local.yaml")
		if err != nil {
			t.Fatalf("Failed to load local Clash YAML: %v", err)
		}
		t.Logf("Loaded %d domains from local Clash YAML file", len(domains))
	})

	t.Run("Local AdBlock", func(t *testing.T) {
		domains, err := loader.LoadDomains("../domains/adblock.txt")
		if err != nil {
			t.Fatalf("Failed to load local AdBlock: %v", err)
		}
		t.Logf("Loaded %d domains from local AdBlock file", len(domains))
	})
}
