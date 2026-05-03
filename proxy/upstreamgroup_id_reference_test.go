package proxy

import (
	"log/slog"
	"os"
	"testing"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultGroupByID tests using ID to reference default group.
func TestDefaultGroupByID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: testTimeout,
	}

	// Configuration using ID for default_group
	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				ID:        "default-id-123",
				Name:      "默认",
				Upstreams: []string{"114.114.114.114"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
			{
				ID:        "china-id-456",
				Name:      "国内",
				Upstreams: []string{"223.5.5.5"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
		},
		DomainGroups: map[string]interface{}{
			"国内": []interface{}{
				"baidu.com",
				"*.baidu.com",
			},
		},
		DefaultGroup: "default-id-123", // Using ID instead of name
	}

	// Parse configuration
	ugc, err := ParseUpstreamGroups(spec, opts)
	require.NoError(t, err, "Should parse config with ID as default_group")
	require.NotNil(t, ugc)

	t.Cleanup(func() {
		_ = ugc.Close()
	})

	// Verify default group was resolved from ID to name
	assert.Equal(t, "默认", ugc.DefaultGroup, "Default group should be resolved to name")

	// Test routing
	t.Run("Specific domain uses specific group", func(t *testing.T) {
		group, err := ugc.GetGroupForDomain("baidu.com.")
		require.NoError(t, err)
		require.NotNil(t, group)
		assert.Equal(t, "国内", group.Name, "baidu.com should use 国内 group")
		assert.Equal(t, "china-id-456", group.ID)
	})

	t.Run("Unknown domain uses default group", func(t *testing.T) {
		group, err := ugc.GetGroupForDomain("google.com.")
		require.NoError(t, err)
		require.NotNil(t, group)
		assert.Equal(t, "默认", group.Name, "google.com should use 默认 group")
		assert.Equal(t, "default-id-123", group.ID)
	})

	t.Log("✓ Default group can be referenced by ID")
}

// TestDomainListGroupByID tests using ID to reference group in domains_lists.
func TestDomainListGroupByID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: testTimeout,
	}

	// Create a temporary test file
	testDomains := `test1.com
test2.com
test3.com`
	
	tmpFile := "./test-domains-temp.txt"
	err := os.WriteFile(tmpFile, []byte(testDomains), 0644)
	require.NoError(t, err)
	defer os.Remove(tmpFile)

	// Configuration using ID for group reference in domains_lists
	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				ID:        "test-group-id",
				Name:      "测试组",
				Upstreams: []string{"8.8.8.8"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
			{
				ID:        "default-id",
				Name:      "默认",
				Upstreams: []string{"1.1.1.1"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
		},
		DomainLists: []DomainListSpec{
			{
				Name:    "test-list",
				Source:  tmpFile,
				Group:   "test-group-id", // Using ID instead of name
				Enabled: true,
			},
		},
		DefaultGroup: "默认",
	}

	// Parse configuration
	ugc, err := ParseUpstreamGroups(spec, opts)
	require.NoError(t, err, "Should parse config with ID in domains_lists")
	require.NotNil(t, ugc)

	t.Cleanup(func() {
		_ = ugc.Close()
	})

	// Test that domains from list use the correct group
	testCases := []struct {
		domain        string
		expectedGroup string
		expectedID    string
	}{
		{"test1.com.", "测试组", "test-group-id"},
		{"test2.com.", "测试组", "test-group-id"},
		{"test3.com.", "测试组", "test-group-id"},
		{"other.com.", "默认", "default-id"},
	}

	for _, tc := range testCases {
		t.Run(tc.domain, func(t *testing.T) {
			group, err := ugc.GetGroupForDomain(tc.domain)
			require.NoError(t, err)
			require.NotNil(t, group)
			assert.Equal(t, tc.expectedGroup, group.Name)
			assert.Equal(t, tc.expectedID, group.ID)
			t.Logf("✓ %s → %s (ID: %s)", tc.domain, group.Name, group.ID)
		})
	}

	t.Log("✓ Domain list group can be referenced by ID")
}

// TestMixedIDAndNameReferences tests mixing ID and name references.
func TestMixedIDAndNameReferences(t *testing.T) {
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
				ID:        "group-a-id",
				Name:      "GroupA",
				Upstreams: []string{"1.1.1.1"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
			{
				ID:        "group-b-id",
				Name:      "GroupB",
				Upstreams: []string{"8.8.8.8"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
			{
				ID:        "default-id",
				Name:      "Default",
				Upstreams: []string{"114.114.114.114"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
		},
		DomainGroups: map[string]interface{}{
			"GroupA": []interface{}{"test-a.com"}, // Using name
			"group-b-id": []interface{}{"test-b.com"}, // Using ID
		},
		DefaultGroup: "default-id", // Using ID
	}

	ugc, err := ParseUpstreamGroups(spec, opts)
	require.NoError(t, err)
	require.NotNil(t, ugc)

	t.Cleanup(func() {
		_ = ugc.Close()
	})

	// Test mixed references
	testCases := []struct {
		domain        string
		expectedGroup string
	}{
		{"test-a.com.", "GroupA"},  // Referenced by name
		{"test-b.com.", "GroupB"},  // Referenced by ID
		{"other.com.", "Default"},  // Default group by ID
	}

	for _, tc := range testCases {
		t.Run(tc.domain, func(t *testing.T) {
			group, err := ugc.GetGroupForDomain(tc.domain)
			require.NoError(t, err)
			require.NotNil(t, group)
			assert.Equal(t, tc.expectedGroup, group.Name)
			t.Logf("✓ %s → %s", tc.domain, group.Name)
		})
	}

	t.Log("✓ Mixed ID and name references work correctly")
}
