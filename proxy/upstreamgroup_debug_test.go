package proxy

import (
	"log/slog"
	"os"
	"testing"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/stretchr/testify/require"
)

// TestDebugDefaultGroupRouting provides detailed debug output for routing logic.
func TestDebugDefaultGroupRouting(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: testTimeout,
	}

	// Configuration that might cause issues
	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				ID:        "default-id",
				Name:      "default",
				Upstreams: []string{"8.8.8.8"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
				Priority:  0,
			},
			{
				ID:        "china-id",
				Name:      "china",
				Upstreams: []string{"223.5.5.5"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
				Priority:  0,
			},
		},
		DomainGroups: map[string]interface{}{
			"china": []interface{}{
				"baidu.com",
				"*.baidu.com",
			},
		},
		DefaultGroup: "default",
	}

	ugc, err := ParseUpstreamGroups(spec, opts)
	require.NoError(t, err)
	require.NotNil(t, ugc)

	t.Cleanup(func() {
		_ = ugc.Close()
	})

	t.Log("========================================")
	t.Log("Configuration loaded:")
	t.Logf("  Default Group: %s", ugc.DefaultGroup)
	t.Logf("  Groups: %d", len(ugc.Groups))
	for name, group := range ugc.Groups {
		t.Logf("    - %s (ID: %s, Enabled: %v, Priority: %d)", name, group.ID, group.Enabled, group.Priority)
	}
	t.Logf("  Domain Mappings: %d", len(ugc.DomainGroups))
	for domain, groupName := range ugc.DomainGroups {
		t.Logf("    - %s → %s", domain, groupName)
	}
	t.Log("========================================")

	// Test domains
	testDomains := []string{
		"baidu.com.",
		"www.baidu.com.",
		"tieba.baidu.com.",
		"google.com.",
		"unknown.com.",
	}

	for _, domain := range testDomains {
		t.Logf("\nTesting domain: %s", domain)
		
		group, err := ugc.GetGroupForDomain(domain)
		if err != nil {
			t.Logf("  ERROR: %v", err)
			continue
		}
		
		if group == nil {
			t.Logf("  Result: nil group")
			continue
		}
		
		t.Logf("  Result: %s (ID: %s)", group.Name, group.ID)
		
		// Check if this is expected
		if domain == "baidu.com." || domain == "www.baidu.com." || domain == "tieba.baidu.com." {
			if group.Name != "china" {
				t.Errorf("  ✗ WRONG! Expected 'china', got '%s'", group.Name)
			} else {
				t.Logf("  ✓ Correct")
			}
		} else {
			if group.Name != "default" {
				t.Errorf("  ✗ WRONG! Expected 'default', got '%s'", group.Name)
			} else {
				t.Logf("  ✓ Correct")
			}
		}
	}
}

// TestDebugRadixTreeState checks the internal state of Radix Tree.
func TestDebugRadixTreeState(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: testTimeout,
	}

	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				Name:      "default",
				Upstreams: []string{"8.8.8.8"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
			{
				Name:      "china",
				Upstreams: []string{"223.5.5.5"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
		},
		DomainGroups: map[string]interface{}{
			"china": []interface{}{
				"baidu.com",
				"*.baidu.com",
			},
		},
		DefaultGroup: "default",
	}

	ugc, err := ParseUpstreamGroups(spec, opts)
	require.NoError(t, err)
	require.NotNil(t, ugc)

	t.Cleanup(func() {
		_ = ugc.Close()
	})

	t.Log("========================================")
	t.Log("Radix Tree State:")
	t.Logf("  Using Radix: %v", ugc.useRadix)
	t.Logf("  Radix Tree: %v", ugc.domainRadix != nil)
	t.Logf("  Trie Tree: %v", ugc.domainTrie != nil)
	t.Log("========================================")

	// Test exact match
	t.Log("\nTesting: baidu.com")
	if ugc.domainRadix != nil {
		groupName, found := ugc.domainRadix.Search("baidu.com")
		t.Logf("  Radix Search: found=%v, group=%s", found, groupName)
	}
	
	group, err := ugc.GetGroupForDomain("baidu.com.")
	require.NoError(t, err)
	require.NotNil(t, group)
	t.Logf("  Final Result: %s", group.Name)
	require.Equal(t, "china", group.Name, "baidu.com should route to china")

	// Test wildcard match
	t.Log("\nTesting: www.baidu.com")
	if ugc.domainRadix != nil {
		groupName, found := ugc.domainRadix.Search("www.baidu.com")
		t.Logf("  Radix Search: found=%v, group=%s", found, groupName)
	}
	
	group, err = ugc.GetGroupForDomain("www.baidu.com.")
	require.NoError(t, err)
	require.NotNil(t, group)
	t.Logf("  Final Result: %s", group.Name)
	require.Equal(t, "china", group.Name, "www.baidu.com should route to china")

	// Test unknown domain
	t.Log("\nTesting: unknown.com")
	if ugc.domainRadix != nil {
		groupName, found := ugc.domainRadix.Search("unknown.com")
		t.Logf("  Radix Search: found=%v, group=%s", found, groupName)
	}
	
	group, err = ugc.GetGroupForDomain("unknown.com.")
	require.NoError(t, err)
	require.NotNil(t, group)
	t.Logf("  Final Result: %s", group.Name)
	require.Equal(t, "default", group.Name, "unknown.com should route to default")
}
