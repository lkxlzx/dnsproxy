package proxy

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

// TestLoadDomainsFromConfig_StringFormat tests loading domains with simple string format.
func TestLoadDomainsFromConfig_StringFormat(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	domainGroups := map[string]interface{}{
		"baidu.com":  "china",    // domain -> group
		"google.com": "overseas", // domain -> group
	}

	result, err := LoadDomainsFromConfig(domainGroups, logger, nil)
	if err != nil {
		t.Fatalf("LoadDomainsFromConfig failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 domains, got %d", len(result))
	}

	if result["baidu.com"] != "china" {
		t.Errorf("expected baidu.com -> china, got %s", result["baidu.com"])
	}

	if result["google.com"] != "overseas" {
		t.Errorf("expected google.com -> overseas, got %s", result["google.com"])
	}
}

// TestLoadDomainsFromConfig_ArrayFormat tests loading domains with array format.
func TestLoadDomainsFromConfig_ArrayFormat(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	domainGroups := map[string]interface{}{
		"china": []interface{}{
			"baidu.com",
			"taobao.com",
			"*.cn",
		},
	}

	result, err := LoadDomainsFromConfig(domainGroups, logger, nil)
	if err != nil {
		t.Fatalf("LoadDomainsFromConfig failed: %v", err)
	}

	if len(result) != 3 {
		t.Errorf("expected 3 domains, got %d", len(result))
	}

	expectedDomains := []string{"baidu.com", "taobao.com", "*.cn"}
	for _, domain := range expectedDomains {
		if result[domain] != "china" {
			t.Errorf("expected %s -> china, got %s", domain, result[domain])
		}
	}
}

// TestLoadDomainsFromConfig_ObjectFormat tests loading domains with object format.
func TestLoadDomainsFromConfig_ObjectFormat(t *testing.T) {
	// Create a temporary test file
	tmpFile, err := os.CreateTemp("", "test-domains-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write test domains
	testDomains := "example.com\ntest.com\n"
	if _, err := tmpFile.WriteString(testDomains); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	tmpFile.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager := NewDomainListManager("./test-cache", logger)

	domainGroups := map[string]interface{}{
		"test": map[string]interface{}{
			"source":           tmpFile.Name(),
			"refresh_interval": "6h",
			"enabled":          true,
		},
	}

	result, err := LoadDomainsFromConfig(domainGroups, logger, manager)
	if err != nil {
		t.Fatalf("LoadDomainsFromConfig failed: %v", err)
	}

	// Debug: print all results
	t.Logf("Result has %d entries:", len(result))
	for domain, group := range result {
		t.Logf("  %s -> %s", domain, group)
	}

	// Should have loaded 2 domains
	if len(result) != 2 {
		t.Errorf("expected 2 domains, got %d", len(result))
	}

	// Check domains are mapped to correct group
	if result["example.com"] != "test" {
		t.Errorf("expected example.com -> test, got %s", result["example.com"])
	}

	if result["test.com"] != "test" {
		t.Errorf("expected test.com -> test, got %s", result["test.com"])
	}

	// Check manager has the list
	lists := manager.ListAll()
	if len(lists) != 1 {
		t.Errorf("expected 1 managed list, got %d", len(lists))
	}

	if len(lists) > 0 {
		list := lists[0]
		if list.Group != "test" {
			t.Errorf("expected group 'test', got %s", list.Group)
		}
		if list.RefreshInterval != 6*time.Hour {
			t.Errorf("expected refresh interval 6h, got %v", list.RefreshInterval)
		}
		if list.DomainCount != 2 {
			t.Errorf("expected 2 domains, got %d", list.DomainCount)
		}
	}
}

// TestLoadDomainsFromConfig_MixedFormat tests loading domains with mixed formats.
func TestLoadDomainsFromConfig_MixedFormat(t *testing.T) {
	// Create a temporary test file
	tmpFile, err := os.CreateTemp("", "test-domains-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write test domains
	testDomains := "file-domain.com\n"
	if _, err := tmpFile.WriteString(testDomains); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	tmpFile.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager := NewDomainListManager("./test-cache", logger)

	domainGroups := map[string]interface{}{
		"mixed": []interface{}{
			"direct-domain.com",
			tmpFile.Name(),
			map[string]interface{}{
				"source":           tmpFile.Name(),
				"refresh_interval": "12h",
				"enabled":          true,
			},
		},
	}

	result, err := LoadDomainsFromConfig(domainGroups, logger, manager)
	if err != nil {
		t.Fatalf("LoadDomainsFromConfig failed: %v", err)
	}

	// Should have: 1 direct domain + 1 from file + 1 from object = 3 domains
	// But file-domain.com appears twice, so should be deduplicated to 2
	if len(result) != 2 {
		t.Errorf("expected 2 unique domains, got %d: %v", len(result), result)
	}

	// Check all domains are mapped to correct group
	if result["direct-domain.com"] != "mixed" {
		t.Errorf("expected direct-domain.com -> mixed, got %s", result["direct-domain.com"])
	}

	if result["file-domain.com"] != "mixed" {
		t.Errorf("expected file-domain.com -> mixed, got %s", result["file-domain.com"])
	}
}

// TestLoadDomainsFromConfig_DisabledSource tests that disabled sources are skipped.
func TestLoadDomainsFromConfig_DisabledSource(t *testing.T) {
	// Create a temporary test file
	tmpFile, err := os.CreateTemp("", "test-domains-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write test domains
	testDomains := "disabled-domain.com\n"
	if _, err := tmpFile.WriteString(testDomains); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	tmpFile.Close()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	domainGroups := map[string]interface{}{
		"test": map[string]interface{}{
			"source":  tmpFile.Name(),
			"enabled": false,
		},
	}

	result, err := LoadDomainsFromConfig(domainGroups, logger, nil)
	if err != nil {
		t.Fatalf("LoadDomainsFromConfig failed: %v", err)
	}

	// Should have no domains since source is disabled
	if len(result) != 0 {
		t.Errorf("expected 0 domains, got %d", len(result))
	}
}

// TestDomainListManager_NeedsRefresh tests the NeedsRefresh method.
func TestDomainListManager_NeedsRefresh(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager := NewDomainListManager("./test-cache", logger)

	// Add a list with custom refresh interval
	list := &ManagedList{
		Name:            "test-list",
		Source:          "https://example.com/domains.txt",
		Group:           "test",
		Enabled:         true,
		LastUpdate:      time.Now().Add(-7 * time.Hour), // Updated 7 hours ago
		AutoUpdate:      true,
		RefreshInterval: 6 * time.Hour, // Refresh every 6 hours
	}

	if err := manager.AddList(list); err != nil {
		t.Fatalf("failed to add list: %v", err)
	}

	// Should need refresh (7h > 6h)
	if !manager.NeedsRefresh("test-list", 24*time.Hour) {
		t.Error("expected list to need refresh")
	}

	// Update the list
	list.LastUpdate = time.Now()

	// Should not need refresh now
	if manager.NeedsRefresh("test-list", 24*time.Hour) {
		t.Error("expected list to not need refresh")
	}
}

// TestDomainListManager_NeedsRefresh_DefaultInterval tests using default interval.
func TestDomainListManager_NeedsRefresh_DefaultInterval(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager := NewDomainListManager("./test-cache", logger)

	// Add a list without custom refresh interval
	list := &ManagedList{
		Name:            "test-list",
		Source:          "https://example.com/domains.txt",
		Group:           "test",
		Enabled:         true,
		LastUpdate:      time.Now().Add(-25 * time.Hour), // Updated 25 hours ago
		AutoUpdate:      true,
		RefreshInterval: 0, // Use default
	}

	if err := manager.AddList(list); err != nil {
		t.Fatalf("failed to add list: %v", err)
	}

	// Should need refresh with default 24h (25h > 24h)
	if !manager.NeedsRefresh("test-list", 24*time.Hour) {
		t.Error("expected list to need refresh with default interval")
	}

	// Should not need refresh with default 48h (25h < 48h)
	if manager.NeedsRefresh("test-list", 48*time.Hour) {
		t.Error("expected list to not need refresh with longer default interval")
	}
}

// TestHashSource tests the hashSource helper function.
func TestHashSource(t *testing.T) {
	tests := []struct {
		source   string
		expected string
	}{
		{
			source:   "https://example.com/domains.txt",
			expected: "domains.",
		},
		{
			source:   "https://example.com/very-long-filename.txt",
			expected: "very-lon",
		},
		{
			source:   "short",
			expected: "short",
		},
		{
			source:   "verylongstring",
			expected: "verylong",
		},
	}

	for _, tt := range tests {
		result := hashSource(tt.source)
		if result != tt.expected {
			t.Errorf("hashSource(%q) = %q, expected %q", tt.source, result, tt.expected)
		}
	}
}
