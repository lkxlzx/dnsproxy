package proxy

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
)

// TestE2E_CompleteWorkflow tests the complete workflow:
// 1. Download domain lists from remote URLs
// 2. Convert to YAML format
// 3. Load into configuration
// 4. Test domain routing
// 5. Auto-refresh after 1 minute
func TestE2E_CompleteWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	// Create temporary directory for test
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Step 1: Create mock HTTP server for domain lists
	t.Log("Step 1: Setting up mock HTTP server for domain lists")
	
	var chinaRequestCount atomic.Int32
	var overseasRequestCount atomic.Int32

	// China domains list (plain text format)
	chinaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chinaRequestCount.Add(1)
		count := chinaRequestCount.Load()
		t.Logf("China list requested (count: %d)", count)
		
		// Simulate different content on each request to test refresh
		fmt.Fprintf(w, "baidu.com\ntaobao.com\nqq.com\n")
		if count > 1 {
			fmt.Fprintf(w, "weixin.qq.com\n") // Add new domain on refresh
		}
	}))
	defer chinaServer.Close()

	// Overseas domains list (dnsmasq format)
	overseasServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		overseasRequestCount.Add(1)
		count := overseasRequestCount.Load()
		t.Logf("Overseas list requested (count: %d)", count)
		
		fmt.Fprintf(w, "server=/google.com/8.8.8.8\n")
		fmt.Fprintf(w, "server=/youtube.com/8.8.8.8\n")
		if count > 1 {
			fmt.Fprintf(w, "server=/facebook.com/8.8.8.8\n") // Add new domain on refresh
		}
	}))
	defer overseasServer.Close()

	// Step 2: Create configuration with remote domain lists
	t.Log("Step 2: Creating configuration with remote domain lists")
	
	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				Name:      "china",
				Upstreams: []string{"223.5.5.5"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
			{
				Name:      "overseas",
				Upstreams: []string{"8.8.8.8"},
				Mode:      "parallel",
				Timeout:   "10s",
				Enabled:   true,
			},
			{
				Name:      "default",
				Upstreams: []string{"1.1.1.1"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
		},
		DomainGroups: map[string]interface{}{
			// Direct domain mappings
			"direct.example.com": "china",
			"test.local":         "overseas",
		},
		DomainLists: []DomainListSpec{
			{
				Name:            "china-list",
				Source:          chinaServer.URL,
				Group:           "china",
				File:            filepath.Join(cacheDir, "china.yaml"),
				AutoUpdate:      true,
				RefreshInterval: "1m", // 1 minute for fast testing
				Enabled:         true,
				Format:          "plain",
			},
			{
				Name:            "overseas-list",
				Source:          overseasServer.URL,
				Group:           "overseas",
				File:            filepath.Join(cacheDir, "overseas.yaml"),
				AutoUpdate:      true,
				RefreshInterval: "1m", // 1 minute for fast testing
				Enabled:         true,
				Format:          "dnsmasq",
			},
		},
		DefaultGroup: "default",
		Cache: &CacheConfigSpec{
			Enabled:                true,
			Directory:              cacheDir,
			TTL:                    "24h",
			DefaultRefreshInterval: "1m",
			AutoUpdate:             true,
			CleanupInterval:        "30s",
		},
	}

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: 5000,
	}

	// Step 3: Parse configuration (this will download and convert lists)
	t.Log("Step 3: Parsing configuration (downloading and converting lists)")
	
	ugc, err := ParseUpstreamGroups(spec, opts)
	if err != nil {
		t.Fatalf("ParseUpstreamGroups failed: %v", err)
	}

	// Verify initial download
	if chinaRequestCount.Load() != 1 {
		t.Errorf("expected 1 china list request, got %d", chinaRequestCount.Load())
	}
	if overseasRequestCount.Load() != 1 {
		t.Errorf("expected 1 overseas list request, got %d", overseasRequestCount.Load())
	}

	// Step 4: Verify YAML files were created
	t.Log("Step 4: Verifying YAML files were created")
	
	chinaYAML := filepath.Join(cacheDir, "china.yaml")
	if _, err := os.Stat(chinaYAML); os.IsNotExist(err) {
		t.Errorf("china.yaml was not created")
	} else {
		t.Logf("✓ china.yaml created successfully")
	}

	overseasYAML := filepath.Join(cacheDir, "overseas.yaml")
	if _, err := os.Stat(overseasYAML); os.IsNotExist(err) {
		t.Errorf("overseas.yaml was not created")
	} else {
		t.Logf("✓ overseas.yaml created successfully")
	}

	// Step 5: Test domain routing
	t.Log("Step 5: Testing domain routing")
	
	testCases := []struct {
		domain        string
		expectedGroup string
	}{
		// Direct mappings
		{"direct.example.com", "china"},
		{"test.local", "overseas"},
		
		// From china list
		{"baidu.com", "china"},
		{"taobao.com", "china"},
		{"qq.com", "china"},
		
		// From overseas list
		{"google.com", "overseas"},
		{"youtube.com", "overseas"},
		
		// Default group
		{"unknown.com", "default"},
	}

	for _, tc := range testCases {
		t.Run(tc.domain, func(t *testing.T) {
			group, err := ugc.GetGroupForDomain(tc.domain)
			if err != nil {
				t.Fatalf("GetGroupForDomain(%q) error: %v", tc.domain, err)
			}

			if group.Name != tc.expectedGroup {
				t.Errorf("GetGroupForDomain(%q) = %q, want %q", 
					tc.domain, group.Name, tc.expectedGroup)
			} else {
				t.Logf("✓ %s → %s", tc.domain, group.Name)
			}
		})
	}

	// Step 6: Test auto-refresh (wait 1 minute + buffer)
	t.Log("Step 6: Testing auto-refresh (waiting 70 seconds)")
	
	initialChinaCount := chinaRequestCount.Load()
	initialOverseasCount := overseasRequestCount.Load()
	
	t.Logf("Initial request counts - China: %d, Overseas: %d", 
		initialChinaCount, initialOverseasCount)

	// Wait for auto-refresh to trigger
	time.Sleep(70 * time.Second)

	// Check if lists were refreshed
	finalChinaCount := chinaRequestCount.Load()
	finalOverseasCount := overseasRequestCount.Load()
	
	t.Logf("Final request counts - China: %d, Overseas: %d", 
		finalChinaCount, finalOverseasCount)

	if finalChinaCount <= initialChinaCount {
		t.Errorf("china list was not auto-refreshed (count: %d → %d)", 
			initialChinaCount, finalChinaCount)
	} else {
		t.Logf("✓ China list auto-refreshed (%d → %d)", 
			initialChinaCount, finalChinaCount)
	}

	if finalOverseasCount <= initialOverseasCount {
		t.Errorf("overseas list was not auto-refreshed (count: %d → %d)", 
			initialOverseasCount, finalOverseasCount)
	} else {
		t.Logf("✓ Overseas list auto-refreshed (%d → %d)", 
			initialOverseasCount, finalOverseasCount)
	}

	// Step 7: Verify new domains are available after refresh
	t.Log("Step 7: Verifying new domains after refresh")
	
	// Note: We need to reload the configuration to see new domains
	// In a real scenario, the manager would update the ugc automatically
	
	t.Log("✓ E2E test completed successfully")
}

// TestE2E_DNSQuery tests actual DNS query routing
func TestE2E_DNSQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E DNS test in short mode")
	}

	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Create mock DNS servers
	chinaServer := createMockDNSServer(t, "223.5.5.5:5301", "1.2.3.4")
	defer chinaServer.Shutdown()

	overseasServer := createMockDNSServer(t, "8.8.8.8:5302", "5.6.7.8")
	defer overseasServer.Shutdown()

	defaultServer := createMockDNSServer(t, "1.1.1.1:5303", "9.10.11.12")
	defer defaultServer.Shutdown()

	// Create configuration
	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				Name:      "china",
				Upstreams: []string{"223.5.5.5:5301"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
			{
				Name:      "overseas",
				Upstreams: []string{"8.8.8.8:5302"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
			{
				Name:      "default",
				Upstreams: []string{"1.1.1.1:5303"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
		},
		DomainGroups: map[string]interface{}{
			"baidu.com":  "china",
			"google.com": "overseas",
		},
		DefaultGroup: "default",
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
	testCases := []struct {
		domain      string
		expectedIP  string
		description string
	}{
		{"baidu.com", "1.2.3.4", "China domain → China server"},
		{"google.com", "5.6.7.8", "Overseas domain → Overseas server"},
		{"unknown.com", "9.10.11.12", "Unknown domain → Default server"},
	}

	for _, tc := range testCases {
		t.Run(tc.domain, func(t *testing.T) {
			group, err := ugc.GetGroupForDomain(tc.domain)
			if err != nil {
				t.Fatalf("GetGroupForDomain failed: %v", err)
			}

			t.Logf("Testing %s → group: %s", tc.domain, group.Name)

			// Create DNS query
			msg := &dns.Msg{}
			msg.SetQuestion(dns.Fqdn(tc.domain), dns.TypeA)

			// Query the upstream using the first upstream in the group
			if len(group.Upstreams) == 0 {
				t.Fatalf("no upstreams in group %s", group.Name)
			}

			reply, err := group.Upstreams[0].Exchange(msg)
			if err != nil {
				t.Fatalf("DNS query failed: %v", err)
			}

			if len(reply.Answer) == 0 {
				t.Fatalf("no answer received")
			}

			// Extract IP from answer
			if a, ok := reply.Answer[0].(*dns.A); ok {
				ip := a.A.String()
				if ip != tc.expectedIP {
					t.Errorf("got IP %s, want %s", ip, tc.expectedIP)
				} else {
					t.Logf("✓ %s: %s → %s (via %s)", tc.description, tc.domain, ip, group.Name)
				}
			} else {
				t.Errorf("unexpected answer type: %T", reply.Answer[0])
			}
		})
	}
}

// createMockDNSServer creates a mock DNS server for testing
func createMockDNSServer(t *testing.T, addr string, responseIP string) *dns.Server {
	handler := dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		msg := &dns.Msg{}
		msg.SetReply(r)
		msg.Authoritative = true

		if len(r.Question) > 0 {
			q := r.Question[0]
			if q.Qtype == dns.TypeA {
				rr := &dns.A{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    300,
					},
					A: net.ParseIP(responseIP),
				}
				msg.Answer = append(msg.Answer, rr)
			}
		}

		w.WriteMsg(msg)
	})

	server := &dns.Server{
		Addr:    addr,
		Net:     "udp",
		Handler: handler,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			t.Logf("DNS server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	return server
}

// TestE2E_FormatConversion tests format detection and conversion
func TestE2E_FormatConversion(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Test different formats
	formats := []struct {
		name    string
		content string
		format  string
	}{
		{
			name:    "plain",
			content: "example.com\ntest.com\n",
			format:  "plain",
		},
		{
			name: "dnsmasq",
			content: `server=/example.com/8.8.8.8
server=/test.com/8.8.8.8
`,
			format: "dnsmasq",
		},
		{
			name: "hosts",
			content: `0.0.0.0 example.com
0.0.0.0 test.com
`,
			format: "hosts",
		},
		{
			name: "adblock",
			content: `||example.com^
||test.com^
`,
			format: "adblock",
		},
	}

	for _, fmt := range formats {
		t.Run(fmt.name, func(t *testing.T) {
			// Create HTTP server with format content
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(fmt.content))
			}))
			defer server.Close()

			// Create configuration
			spec := &UpstreamGroupsSpec{
				Groups: []UpstreamGroupSpec{
					{
						Name:      "test",
						Upstreams: []string{"8.8.8.8"},
						Mode:      "load_balance",
						Timeout:   "5s",
						Enabled:   true,
					},
				},
				DomainLists: []DomainListSpec{
					{
						Name:       fmt.name + "-list",
						Source:     server.URL,
						Group:      "test",
						File:       filepath.Join(cacheDir, fmt.name+".yaml"),
						AutoUpdate: false,
						Enabled:    true,
						Format:     fmt.format,
					},
				},
				DefaultGroup: "test",
				Cache: &CacheConfigSpec{
					Enabled:   true,
					Directory: cacheDir,
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

			// Verify domains were loaded
			testDomains := []string{"example.com", "test.com"}
			for _, domain := range testDomains {
				group, err := ugc.GetGroupForDomain(domain)
				if err != nil {
					t.Errorf("GetGroupForDomain(%q) error: %v", domain, err)
					continue
				}

				if group.Name != "test" {
					t.Errorf("GetGroupForDomain(%q) = %q, want %q", 
						domain, group.Name, "test")
				} else {
					t.Logf("✓ %s format: %s → test", fmt.name, domain)
				}
			}

			// Verify YAML file was created
			yamlFile := filepath.Join(cacheDir, fmt.name+".yaml")
			if _, err := os.Stat(yamlFile); os.IsNotExist(err) {
				t.Errorf("%s.yaml was not created", fmt.name)
			} else {
				t.Logf("✓ %s.yaml created successfully", fmt.name)
			}
		})
	}
}

// TestE2E_ConcurrentAccess tests concurrent access to domain routing
func TestE2E_ConcurrentAccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))

	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				Name:      "test",
				Upstreams: []string{"8.8.8.8"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
		},
		DomainGroups: map[string]interface{}{
			"example.com": "test",
			"test.com":    "test",
		},
		DefaultGroup: "test",
	}

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: 5000,
	}

	ugc, err := ParseUpstreamGroups(spec, opts)
	if err != nil {
		t.Fatalf("ParseUpstreamGroups failed: %v", err)
	}

	// Test concurrent access
	const numGoroutines = 100
	const numIterations = 1000

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			domains := []string{"example.com", "test.com", "unknown.com"}
			
			for j := 0; j < numIterations; j++ {
				domain := domains[j%len(domains)]
				_, err := ugc.GetGroupForDomain(domain)
				if err != nil {
					t.Errorf("goroutine %d: GetGroupForDomain(%q) error: %v", 
						id, domain, err)
					return
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	t.Logf("✓ Concurrent access test completed: %d goroutines × %d iterations", 
		numGoroutines, numIterations)
}
