package proxy

import (
	"log/slog"
	"os"
	"testing"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultGroupPriority tests that default group has the lowest priority.
// Domains with specific group assignments should NOT use the default group.
func TestDefaultGroupPriority(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: testTimeout,
	}

	// Create configuration with default group and specific domain mappings
	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				ID:        "default-group-id",
				Name:      "default",
				Upstreams: []string{"8.8.8.8"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
				Priority:  0,
			},
			{
				ID:        "china-group-id",
				Name:      "china",
				Upstreams: []string{"223.5.5.5"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
				Priority:  0,
			},
			{
				ID:        "overseas-group-id",
				Name:      "overseas",
				Upstreams: []string{"1.1.1.1"},
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
				"qq.com",
				"*.qq.com",
			},
			"overseas": []interface{}{
				"google.com",
				"*.google.com",
			},
		},
		DefaultGroup: "default",
	}

	// Parse configuration
	ugc, err := ParseUpstreamGroups(spec, opts)
	require.NoError(t, err, "Failed to parse upstream groups")
	require.NotNil(t, ugc, "UpstreamGroupConfig should not be nil")

	t.Cleanup(func() {
		_ = ugc.Close()
	})

	// Test cases to verify default group has lowest priority
	testCases := []struct {
		name          string
		domain        string
		expectedGroup string
		description   string
	}{
		{
			name:          "Baidu exact match",
			domain:        "baidu.com.",
			expectedGroup: "china",
			description:   "Should use china group, NOT default",
		},
		{
			name:          "Baidu wildcard match",
			domain:        "www.baidu.com.",
			expectedGroup: "china",
			description:   "Should use china group, NOT default",
		},
		{
			name:          "QQ exact match",
			domain:        "qq.com.",
			expectedGroup: "china",
			description:   "Should use china group, NOT default",
		},
		{
			name:          "QQ wildcard match",
			domain:        "mail.qq.com.",
			expectedGroup: "china",
			description:   "Should use china group, NOT default",
		},
		{
			name:          "Google exact match",
			domain:        "google.com.",
			expectedGroup: "overseas",
			description:   "Should use overseas group, NOT default",
		},
		{
			name:          "Google wildcard match",
			domain:        "www.google.com.",
			expectedGroup: "overseas",
			description:   "Should use overseas group, NOT default",
		},
		{
			name:          "Unknown domain",
			domain:        "example.com.",
			expectedGroup: "default",
			description:   "Should use default group (no specific mapping)",
		},
		{
			name:          "Another unknown domain",
			domain:        "test.local.",
			expectedGroup: "default",
			description:   "Should use default group (no specific mapping)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			group, err := ugc.GetGroupForDomain(tc.domain)
			require.NoError(t, err, "Should get group for domain %s", tc.domain)
			require.NotNil(t, group, "Group should not be nil for domain %s", tc.domain)
			
			assert.Equal(t, tc.expectedGroup, group.Name,
				"Domain %s: %s (got %s, expected %s)",
				tc.domain, tc.description, group.Name, tc.expectedGroup)
			
			if group.Name == tc.expectedGroup {
				t.Logf("✓ %s: %s → %s (correct)", tc.domain, tc.description, group.Name)
			} else {
				t.Errorf("✗ %s: %s → %s (expected %s)", tc.domain, tc.description, group.Name, tc.expectedGroup)
			}
		})
	}
}

// TestDefaultGroupPriorityWithRadix tests default group priority with Radix Tree.
func TestDefaultGroupPriorityWithRadix(t *testing.T) {
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
				Name:      "specific",
				Upstreams: []string{"1.1.1.1"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
		},
		DomainGroups: map[string]interface{}{
			"specific": []interface{}{
				"test.com",
				"*.test.com",
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

	// Ensure Radix Tree is enabled
	ugc.EnableRadixTree()

	// Test that specific domain uses specific group, not default
	group, err := ugc.GetGroupForDomain("test.com.")
	require.NoError(t, err)
	require.NotNil(t, group)
	assert.Equal(t, "specific", group.Name, "test.com should use specific group, not default")

	// Test wildcard
	group, err = ugc.GetGroupForDomain("www.test.com.")
	require.NoError(t, err)
	require.NotNil(t, group)
	assert.Equal(t, "specific", group.Name, "www.test.com should use specific group, not default")

	// Test unknown domain uses default
	group, err = ugc.GetGroupForDomain("unknown.com.")
	require.NoError(t, err)
	require.NotNil(t, group)
	assert.Equal(t, "default", group.Name, "unknown.com should use default group")

	t.Log("✓ Radix Tree: Default group has lowest priority")
}

// TestDefaultGroupPriorityWithTrie tests default group priority with Trie.
func TestDefaultGroupPriorityWithTrie(t *testing.T) {
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
				Name:      "specific",
				Upstreams: []string{"1.1.1.1"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
		},
		DomainGroups: map[string]interface{}{
			"specific": []interface{}{
				"test.com",
				"*.test.com",
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

	// Switch to Trie Tree
	ugc.EnableTrieTree()

	// Test that specific domain uses specific group, not default
	group, err := ugc.GetGroupForDomain("test.com.")
	require.NoError(t, err)
	require.NotNil(t, group)
	assert.Equal(t, "specific", group.Name, "test.com should use specific group, not default")

	// Test wildcard
	group, err = ugc.GetGroupForDomain("www.test.com.")
	require.NoError(t, err)
	require.NotNil(t, group)
	assert.Equal(t, "specific", group.Name, "www.test.com should use specific group, not default")

	// Test unknown domain uses default
	group, err = ugc.GetGroupForDomain("unknown.com.")
	require.NoError(t, err)
	require.NotNil(t, group)
	assert.Equal(t, "default", group.Name, "unknown.com should use default group")

	t.Log("✓ Trie Tree: Default group has lowest priority")
}

// TestNoDefaultGroup tests behavior when no default group is set.
func TestNoDefaultGroup(t *testing.T) {
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
				Name:      "specific",
				Upstreams: []string{"1.1.1.1"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
		},
		DomainGroups: map[string]interface{}{
			"specific": []interface{}{"test.com"},
		},
		// No DefaultGroup set
	}

	ugc, err := ParseUpstreamGroups(spec, opts)
	require.NoError(t, err)
	require.NotNil(t, ugc)

	t.Cleanup(func() {
		_ = ugc.Close()
	})

	// Specific domain should work
	group, err := ugc.GetGroupForDomain("test.com.")
	require.NoError(t, err)
	require.NotNil(t, group)
	assert.Equal(t, "specific", group.Name)

	// Unknown domain should return error (no default group)
	group, err = ugc.GetGroupForDomain("unknown.com.")
	assert.Error(t, err, "Should return error when no default group is set")
	assert.Nil(t, group, "Group should be nil for unknown domain without default")

	t.Log("✓ No default group: Unknown domains return error")
}
