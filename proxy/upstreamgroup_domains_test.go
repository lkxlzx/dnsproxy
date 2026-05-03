package proxy

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// TestDomainFileLoader_LoadFromFile tests loading domains from local files.
func TestDomainFileLoader_LoadFromFile(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	loader := NewDomainFileLoader(logger)

	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test-domains.txt")

	content := `# Test domains
example.com
*.google.com
test.local

# Another comment
another-domain.com
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Load domains
	domains, err := loader.LoadDomains(testFile)
	if err != nil {
		t.Fatalf("LoadDomains failed: %v", err)
	}

	// Verify results
	if len(domains) != 4 {
		t.Errorf("expected 4 domains, got %d", len(domains))
	}

	expectedDomains := map[string]bool{
		"example.com":       true,
		"*.google.com":      true,
		"test.local":        true,
		"another-domain.com": true,
	}

	for _, domain := range domains {
		if !expectedDomains[domain] {
			t.Errorf("unexpected domain: %s", domain)
		}
	}
}

// TestDomainFileLoader_ParsePlainText tests plain text format parsing.
func TestDomainFileLoader_ParsePlainText(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	loader := NewDomainFileLoader(logger)

	content := []byte(`# Comment
example.com
*.google.com

! Another comment
test.local
`)

	domains, err := loader.parsePlainText(content)
	if err != nil {
		t.Fatalf("parsePlainText failed: %v", err)
	}

	if len(domains) != 3 {
		t.Errorf("expected 3 domains, got %d", len(domains))
	}
}

// TestDomainFileLoader_ParseClashYAML tests Clash YAML format parsing.
func TestDomainFileLoader_ParseClashYAML(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	loader := NewDomainFileLoader(logger)

	content := []byte(`payload:
  - DOMAIN,example.com
  - DOMAIN-SUFFIX,google.com
  - DOMAIN-KEYWORD,youtube
`)

	domains, err := loader.parseClashYAML(content)
	if err != nil {
		t.Fatalf("parseClashYAML failed: %v", err)
	}

	if len(domains) != 3 {
		t.Errorf("expected 3 domains, got %d", len(domains))
	}

	// Verify domain formats
	expectedFormats := map[string]bool{
		"example.com.":   true, // DOMAIN
		"*.google.com.":  true, // DOMAIN-SUFFIX
		"*youtube*.":     true, // DOMAIN-KEYWORD
	}

	for _, domain := range domains {
		if !expectedFormats[domain] {
			t.Errorf("unexpected domain format: %s", domain)
		}
	}
}

// TestDomainFileLoader_ParseSurge tests Surge format parsing.
func TestDomainFileLoader_ParseSurge(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	loader := NewDomainFileLoader(logger)

	content := []byte(`# Surge rules
DOMAIN,example.com
DOMAIN-SUFFIX,google.com
DOMAIN-KEYWORD,youtube
`)

	domains, err := loader.parseSurge(content)
	if err != nil {
		t.Fatalf("parseSurge failed: %v", err)
	}

	if len(domains) != 3 {
		t.Errorf("expected 3 domains, got %d", len(domains))
	}
}

// TestDomainFileLoader_ParseDnsmasq tests Dnsmasq format parsing.
func TestDomainFileLoader_ParseDnsmasq(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	loader := NewDomainFileLoader(logger)

	content := []byte(`# Dnsmasq config
server=/example.com/8.8.8.8
address=/ads.com/0.0.0.0
server=/google.com/1.1.1.1
`)

	domains, err := loader.parseDnsmasq(content)
	if err != nil {
		t.Fatalf("parseDnsmasq failed: %v", err)
	}

	if len(domains) != 3 {
		t.Errorf("expected 3 domains, got %d", len(domains))
	}

	expectedDomains := map[string]bool{
		"example.com.": true,
		"ads.com.":     true,
		"google.com.":  true,
	}

	for _, domain := range domains {
		if !expectedDomains[domain] {
			t.Errorf("unexpected domain: %s", domain)
		}
	}
}

// TestDomainFileLoader_ParseHosts tests Hosts format parsing.
func TestDomainFileLoader_ParseHosts(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	loader := NewDomainFileLoader(logger)

	content := []byte(`# Hosts file
127.0.0.1 localhost
0.0.0.0 ads.example.com
192.168.1.1 router.local gateway.local
`)

	domains, err := loader.parseHosts(content)
	if err != nil {
		t.Fatalf("parseHosts failed: %v", err)
	}

	if len(domains) != 4 {
		t.Errorf("expected 4 domains, got %d", len(domains))
	}

	expectedDomains := map[string]bool{
		"localhost.":        true,
		"ads.example.com.":  true,
		"router.local.":     true,
		"gateway.local.":    true,
	}

	for _, domain := range domains {
		if !expectedDomains[domain] {
			t.Errorf("unexpected domain: %s", domain)
		}
	}
}

// TestDomainFileLoader_ParseAdblock tests AdBlock format parsing.
func TestDomainFileLoader_ParseAdblock(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	loader := NewDomainFileLoader(logger)

	content := []byte(`! AdBlock filters
||ads.example.com^
||tracker.com^
@@||whitelist.com^
! Comment
||malware.net^
`)

	domains, err := loader.parseAdblock(content)
	if err != nil {
		t.Fatalf("parseAdblock failed: %v", err)
	}

	// Should skip whitelisted domains (@@)
	if len(domains) != 3 {
		t.Errorf("expected 3 domains, got %d", len(domains))
	}

	expectedDomains := map[string]bool{
		"ads.example.com.": true,
		"tracker.com.":     true,
		"malware.net.":     true,
	}

	for _, domain := range domains {
		if !expectedDomains[domain] {
			t.Errorf("unexpected domain: %s", domain)
		}
	}
}

// TestDomainFileLoader_ParseJSON tests JSON format parsing.
func TestDomainFileLoader_ParseJSON(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	loader := NewDomainFileLoader(logger)

	// Test array format
	t.Run("array format", func(t *testing.T) {
		content := []byte(`["example.com", "google.com", "test.local"]`)

		domains, err := loader.parseJSON(content)
		if err != nil {
			t.Fatalf("parseJSON failed: %v", err)
		}

		if len(domains) != 3 {
			t.Errorf("expected 3 domains, got %d", len(domains))
		}
	})

	// Test object format
	t.Run("object format", func(t *testing.T) {
		content := []byte(`{
			"generated_at": "2024-01-01T00:00:00Z",
			"count": 2,
			"domains": ["example.com", "google.com"]
		}`)

		domains, err := loader.parseJSON(content)
		if err != nil {
			t.Fatalf("parseJSON failed: %v", err)
		}

		if len(domains) != 2 {
			t.Errorf("expected 2 domains, got %d", len(domains))
		}
	})
}

// TestDomainFileLoader_DetectFormat tests format detection.
func TestDomainFileLoader_DetectFormat(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	loader := NewDomainFileLoader(logger)

	tests := []struct {
		name     string
		content  string
		source   string
		expected string
	}{
		{
			name:     "plain text by extension",
			content:  "example.com\ngoogle.com",
			source:   "domains.txt",
			expected: "plain",
		},
		{
			name:     "clash yaml by extension",
			content:  "payload:\n  - DOMAIN,example.com",
			source:   "rules.yaml",
			expected: "clash",
		},
		{
			name:     "json by extension",
			content:  `["example.com"]`,
			source:   "domains.json",
			expected: "json",
		},
		{
			name:     "clash by content",
			content:  "payload:\n  - DOMAIN-SUFFIX,google.com",
			source:   "unknown",
			expected: "clash",
		},
		{
			name:     "dnsmasq by content",
			content:  "server=/example.com/8.8.8.8",
			source:   "unknown",
			expected: "dnsmasq",
		},
		{
			name:     "surge by content",
			content:  "DOMAIN-SUFFIX,google.com",
			source:   "unknown",
			expected: "surge",
		},
		{
			name:     "adblock by content",
			content:  "||ads.example.com^",
			source:   "unknown",
			expected: "adblock",
		},
		{
			name:     "hosts by content",
			content:  "127.0.0.1 localhost",
			source:   "unknown",
			expected: "hosts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format := loader.detectFormat(tt.content, tt.source)
			if format != tt.expected {
				t.Errorf("expected format %q, got %q", tt.expected, format)
			}
		})
	}
}

// TestDomainFileLoader_CleanDomain tests domain cleaning.
func TestDomainFileLoader_CleanDomain(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	loader := NewDomainFileLoader(logger)

	tests := []struct {
		input    string
		expected string
	}{
		{"example.com", "example.com"},
		{"EXAMPLE.COM", "example.com"},
		{"  example.com  ", "example.com"},
		{"http://example.com", "example.com"},
		{"https://example.com", "example.com"},
		{"example.com:8080", "example.com"},
		{"example.com/path", "example.com"},
		{"192.168.1.1", ""}, // IP addresses should be filtered
		{"", ""},
		{"invalid domain", ""}, // Spaces should be filtered
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := loader.cleanDomain(tt.input)
			if result != tt.expected {
				t.Errorf("cleanDomain(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestDomainFileLoader_CleanAndDeduplicate tests deduplication.
func TestDomainFileLoader_CleanAndDeduplicate(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	loader := NewDomainFileLoader(logger)

	input := []string{
		"example.com",
		"EXAMPLE.COM",
		"google.com",
		"example.com",
		"  test.local  ",
		"",
		"google.com",
	}

	result := loader.cleanAndDeduplicate(input)

	// Should have 3 unique domains
	if len(result) != 3 {
		t.Errorf("expected 3 unique domains, got %d", len(result))
	}

	// Check for duplicates
	seen := make(map[string]bool)
	for _, domain := range result {
		if seen[domain] {
			t.Errorf("duplicate domain found: %s", domain)
		}
		seen[domain] = true
	}
}

// TestLoadDomainsFromConfig tests the configuration loading function.
func TestLoadDomainsFromConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create temporary test files
	tmpDir := t.TempDir()
	
	// Create plain text file
	plainFile := filepath.Join(tmpDir, "plain.txt")
	plainContent := `example.com
google.com
test.local
`
	if err := os.WriteFile(plainFile, []byte(plainContent), 0644); err != nil {
		t.Fatalf("failed to create plain file: %v", err)
	}

	// Create YAML file
	yamlFile := filepath.Join(tmpDir, "clash.yaml")
	yamlContent := `payload:
  - DOMAIN,baidu.com
  - DOMAIN-SUFFIX,qq.com
`
	if err := os.WriteFile(yamlFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to create yaml file: %v", err)
	}

	// Test configuration - keys can be any string, not just "file.xxx"
	domainGroups := map[string]string{
		"direct.example.com": "direct",           // Direct domain mapping
		"china":              plainFile,          // Group name as key, file as value
		"local":              yamlFile,           // Group name as key, file as value
	}

	result, err := LoadDomainsFromConfig(domainGroups, logger)
	if err != nil {
		t.Fatalf("LoadDomainsFromConfig failed: %v", err)
	}

	// Debug: print all results
	t.Logf("Result has %d entries:", len(result))
	for domain, group := range result {
		t.Logf("  %s -> %s", domain, group)
	}

	// Should have direct mapping + domains from files (1 + 3 + 2 = 6)
	if len(result) < 6 {
		t.Errorf("expected at least 6 domain mappings, got %d", len(result))
	}

	// Verify direct mapping
	if result["direct.example.com"] != "direct" {
		t.Errorf("direct mapping failed")
	}

	// Verify file-loaded domains are mapped to correct groups
	foundChina := false
	foundLocal := false
	for domain, group := range result {
		// Plain text domains should be mapped to "china" group
		if domain == "example.com" && group == "china" {
			foundChina = true
		}
		// YAML domains should be mapped to "local" group
		if domain == "baidu.com." && group == "local" {
			foundLocal = true
		}
	}

	if !foundChina {
		t.Error("domains from plain file not loaded correctly")
	}
	if !foundLocal {
		t.Error("domains from yaml file not loaded correctly")
	}
}

// TestFormatConverter_ToPlainText tests conversion to plain text.
func TestFormatConverter_ToPlainText(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	converter := NewFormatConverter(logger)

	domains := []string{"example.com.", "google.com.", "test.local."}
	result := converter.toPlainText(domains)

	content := string(result)
	if !contains(content, "example.com") {
		t.Error("plain text should contain example.com")
	}
	if !contains(content, "google.com") {
		t.Error("plain text should contain google.com")
	}
	if !contains(content, "test.local") {
		t.Error("plain text should contain test.local")
	}
}

// TestFormatConverter_ToClashYAML tests conversion to Clash YAML.
func TestFormatConverter_ToClashYAML(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	converter := NewFormatConverter(logger)

	domains := []string{
		"example.com.",
		"*.google.com.",
		"*youtube*.",
	}

	result, err := converter.toClashYAML(domains)
	if err != nil {
		t.Fatalf("toClashYAML failed: %v", err)
	}

	content := string(result)
	if !contains(content, "payload:") {
		t.Error("Clash YAML should contain payload:")
	}
	if !contains(content, "DOMAIN,example.com") {
		t.Error("Clash YAML should contain DOMAIN rule")
	}
	if !contains(content, "DOMAIN-SUFFIX,google.com") {
		t.Error("Clash YAML should contain DOMAIN-SUFFIX rule")
	}
	if !contains(content, "DOMAIN-KEYWORD,youtube") {
		t.Error("Clash YAML should contain DOMAIN-KEYWORD rule")
	}
}

// TestFormatConverter_ToSurge tests conversion to Surge format.
func TestFormatConverter_ToSurge(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	converter := NewFormatConverter(logger)

	domains := []string{
		"example.com.",
		"*.google.com.",
	}

	result := converter.toSurge(domains)
	content := string(result)

	if !contains(content, "DOMAIN,example.com") {
		t.Error("Surge format should contain DOMAIN rule")
	}
	if !contains(content, "DOMAIN-SUFFIX,google.com") {
		t.Error("Surge format should contain DOMAIN-SUFFIX rule")
	}
}

// TestFormatConverter_ToDnsmasq tests conversion to Dnsmasq format.
func TestFormatConverter_ToDnsmasq(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	converter := NewFormatConverter(logger)

	domains := []string{"example.com.", "google.com."}
	result := converter.toDnsmasq(domains)
	content := string(result)

	if !contains(content, "server=/example.com/") {
		t.Error("Dnsmasq format should contain server= directive")
	}
	if !contains(content, "server=/google.com/") {
		t.Error("Dnsmasq format should contain server= directive")
	}
}

// TestFormatConverter_ToHosts tests conversion to Hosts format.
func TestFormatConverter_ToHosts(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	converter := NewFormatConverter(logger)

	domains := []string{"example.com.", "google.com."}
	result := converter.toHosts(domains)
	content := string(result)

	if !contains(content, "0.0.0.0 example.com") {
		t.Error("Hosts format should contain IP mapping")
	}
	if !contains(content, "0.0.0.0 google.com") {
		t.Error("Hosts format should contain IP mapping")
	}
}

// TestFormatConverter_ToAdblock tests conversion to AdBlock format.
func TestFormatConverter_ToAdblock(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	converter := NewFormatConverter(logger)

	domains := []string{"ads.example.com.", "tracker.com."}
	result := converter.toAdblock(domains)
	content := string(result)

	if !contains(content, "||ads.example.com^") {
		t.Error("AdBlock format should contain || rule")
	}
	if !contains(content, "||tracker.com^") {
		t.Error("AdBlock format should contain || rule")
	}
}

// TestFormatConverter_ToJSON tests conversion to JSON format.
func TestFormatConverter_ToJSON(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	converter := NewFormatConverter(logger)

	domains := []string{"example.com.", "google.com."}
	result, err := converter.toJSON(domains)
	if err != nil {
		t.Fatalf("toJSON failed: %v", err)
	}

	content := string(result)
	if !contains(content, "domains") {
		t.Error("JSON should contain domains field")
	}
	if !contains(content, "example.com") {
		t.Error("JSON should contain example.com")
	}
	if !contains(content, "google.com") {
		t.Error("JSON should contain google.com")
	}
}

// TestFormatConverter_Convert tests the main Convert function.
func TestFormatConverter_Convert(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	converter := NewFormatConverter(logger)

	domains := []string{"example.com.", "google.com."}

	formats := []string{"plain", "clash", "surge", "dnsmasq", "hosts", "adblock", "json"}

	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			result, err := converter.Convert(domains, format)
			if err != nil {
				t.Errorf("Convert to %s failed: %v", format, err)
			}
			if len(result) == 0 {
				t.Errorf("Convert to %s returned empty result", format)
			}
		})
	}

	// Test unsupported format
	t.Run("unsupported", func(t *testing.T) {
		_, err := converter.Convert(domains, "unsupported")
		if err == nil {
			t.Error("Convert should fail for unsupported format")
		}
	})
}

// Helper function to check if string contains substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && 
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || 
		len(s) > len(substr) && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
