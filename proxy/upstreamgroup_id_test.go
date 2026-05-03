package proxy

import (
	"log/slog"
	"os"
	"testing"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpstreamGroupWithID tests that groups with ID field work correctly.
func TestUpstreamGroupWithID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: testTimeout,
	}

	// Create configuration with IDs
	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				ID:       "550e8400-e29b-41d4-a716-446655440001",
				Name:     "china",
				Upstreams: []string{"223.5.5.5", "119.29.29.29"},
				Mode:     "load_balance",
				Timeout:  "5s",
				Enabled:  true,
				Priority: 0,
			},
			{
				ID:       "550e8400-e29b-41d4-a716-446655440002",
				Name:     "overseas",
				Upstreams: []string{"8.8.8.8", "1.1.1.1"},
				Mode:     "parallel",
				Timeout:  "10s",
				Enabled:  true,
				Priority: 0,
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
				"youtube.com",
				"*.youtube.com",
			},
		},
		DefaultGroup: "overseas",
	}

	// Parse configuration
	ugc, err := ParseUpstreamGroups(spec, opts)
	require.NoError(t, err, "Failed to parse upstream groups")
	require.NotNil(t, ugc, "UpstreamGroupConfig should not be nil")

	t.Cleanup(func() {
		_ = ugc.Close()
	})

	// Verify groups were created with IDs
	t.Run("Verify Groups Have IDs", func(t *testing.T) {
		chinaGroup, exists := ugc.Groups["china"]
		require.True(t, exists, "China group should exist")
		assert.Equal(t, "550e8400-e29b-41d4-a716-446655440001", chinaGroup.ID, "China group should have correct ID")
		assert.Equal(t, "china", chinaGroup.Name, "China group should have correct name")

		overseasGroup, exists := ugc.Groups["overseas"]
		require.True(t, exists, "Overseas group should exist")
		assert.Equal(t, "550e8400-e29b-41d4-a716-446655440002", overseasGroup.ID, "Overseas group should have correct ID")
		assert.Equal(t, "overseas", overseasGroup.Name, "Overseas group should have correct name")
	})

	// Test domain routing with IDs
	testCases := []struct {
		name          string
		domain        string
		expectedGroup string
		expectedID    string
	}{
		{
			name:          "Baidu (exact match)",
			domain:        "baidu.com.",
			expectedGroup: "china",
			expectedID:    "550e8400-e29b-41d4-a716-446655440001",
		},
		{
			name:          "Baidu subdomain (wildcard)",
			domain:        "www.baidu.com.",
			expectedGroup: "china",
			expectedID:    "550e8400-e29b-41d4-a716-446655440001",
		},
		{
			name:          "QQ (exact match)",
			domain:        "qq.com.",
			expectedGroup: "china",
			expectedID:    "550e8400-e29b-41d4-a716-446655440001",
		},
		{
			name:          "QQ subdomain (wildcard)",
			domain:        "mail.qq.com.",
			expectedGroup: "china",
			expectedID:    "550e8400-e29b-41d4-a716-446655440001",
		},
		{
			name:          "Google (exact match)",
			domain:        "google.com.",
			expectedGroup: "overseas",
			expectedID:    "550e8400-e29b-41d4-a716-446655440002",
		},
		{
			name:          "Google subdomain (wildcard)",
			domain:        "www.google.com.",
			expectedGroup: "overseas",
			expectedID:    "550e8400-e29b-41d4-a716-446655440002",
		},
		{
			name:          "YouTube (exact match)",
			domain:        "youtube.com.",
			expectedGroup: "overseas",
			expectedID:    "550e8400-e29b-41d4-a716-446655440002",
		},
		{
			name:          "YouTube subdomain (wildcard)",
			domain:        "www.youtube.com.",
			expectedGroup: "overseas",
			expectedID:    "550e8400-e29b-41d4-a716-446655440002",
		},
		{
			name:          "Unknown domain (default group)",
			domain:        "example.com.",
			expectedGroup: "overseas",
			expectedID:    "550e8400-e29b-41d4-a716-446655440002",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			group, err := ugc.GetGroupForDomain(tc.domain)
			require.NoError(t, err, "Should get group for domain %s", tc.domain)
			require.NotNil(t, group, "Group should not be nil for domain %s", tc.domain)
			assert.Equal(t, tc.expectedGroup, group.Name, "Domain %s should route to group %s", tc.domain, tc.expectedGroup)
			assert.Equal(t, tc.expectedID, group.ID, "Domain %s should route to group with ID %s", tc.domain, tc.expectedID)
			
			t.Logf("✓ Domain %q correctly routed to group %q (ID: %s)", tc.domain, group.Name, group.ID)
		})
	}
}

// TestUpstreamGroupWithoutID tests backward compatibility when ID is not provided.
func TestUpstreamGroupWithoutID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: testTimeout,
	}

	// Create configuration without IDs (backward compatibility)
	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				// No ID field
				Name:      "test-group",
				Upstreams: []string{"8.8.8.8"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
		},
		DomainGroups: map[string]interface{}{
			"test-group": []interface{}{"test.com"},
		},
		DefaultGroup: "test-group",
	}

	// Parse configuration
	ugc, err := ParseUpstreamGroups(spec, opts)
	require.NoError(t, err, "Failed to parse upstream groups without ID")
	require.NotNil(t, ugc, "UpstreamGroupConfig should not be nil")

	t.Cleanup(func() {
		_ = ugc.Close()
	})

	// Verify group works without ID
	group, exists := ugc.Groups["test-group"]
	require.True(t, exists, "Test group should exist")
	assert.Equal(t, "", group.ID, "Group ID should be empty when not provided")
	assert.Equal(t, "test-group", group.Name, "Group name should be correct")

	// Test routing still works
	routedGroup, err := ugc.GetGroupForDomain("test.com.")
	require.NoError(t, err, "Should get group for domain")
	require.NotNil(t, routedGroup, "Should route to group")
	assert.Equal(t, "test-group", routedGroup.Name, "Should route to correct group")

	t.Log("✓ Backward compatibility: Groups work correctly without ID field")
}

// TestUpstreamGroupIDUniqueness tests that duplicate IDs are handled.
func TestUpstreamGroupIDUniqueness(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: testTimeout,
	}

	// Create configuration with duplicate IDs
	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				ID:        "duplicate-id",
				Name:      "group1",
				Upstreams: []string{"8.8.8.8"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
			{
				ID:        "duplicate-id", // Same ID
				Name:      "group2",
				Upstreams: []string{"1.1.1.1"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
		},
		DefaultGroup: "group1",
	}

	// Parse configuration - should succeed (IDs are informational, names are unique)
	ugc, err := ParseUpstreamGroups(spec, opts)
	require.NoError(t, err, "Should parse even with duplicate IDs")
	require.NotNil(t, ugc, "UpstreamGroupConfig should not be nil")

	t.Cleanup(func() {
		_ = ugc.Close()
	})

	// Both groups should exist (indexed by name, not ID)
	group1, exists1 := ugc.Groups["group1"]
	require.True(t, exists1, "Group1 should exist")
	assert.Equal(t, "duplicate-id", group1.ID)

	group2, exists2 := ugc.Groups["group2"]
	require.True(t, exists2, "Group2 should exist")
	assert.Equal(t, "duplicate-id", group2.ID)

	t.Log("✓ Groups with duplicate IDs are allowed (indexed by name)")
}

// TestUpstreamGroupIDFormat tests various ID formats.
func TestUpstreamGroupIDFormat(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: testTimeout,
	}

	testCases := []struct {
		name        string
		id          string
		shouldWork  bool
		description string
	}{
		{
			name:        "Standard UUID v4",
			id:          "550e8400-e29b-41d4-a716-446655440000",
			shouldWork:  true,
			description: "Standard UUID format",
		},
		{
			name:        "Short ID",
			id:          "group-1",
			shouldWork:  true,
			description: "Simple string ID",
		},
		{
			name:        "Numeric ID",
			id:          "12345",
			shouldWork:  true,
			description: "Numeric string ID",
		},
		{
			name:        "Empty ID",
			id:          "",
			shouldWork:  true,
			description: "Empty ID (backward compatibility)",
		},
		{
			name:        "Chinese ID",
			id:          "中国组",
			shouldWork:  true,
			description: "Unicode ID",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			spec := &UpstreamGroupsSpec{
				Groups: []UpstreamGroupSpec{
					{
						ID:        tc.id,
						Name:      "test-" + tc.name,
						Upstreams: []string{"8.8.8.8"},
						Mode:      "load_balance",
						Timeout:   "5s",
						Enabled:   true,
					},
				},
				DefaultGroup: "test-" + tc.name,
			}

			ugc, err := ParseUpstreamGroups(spec, opts)
			
			if tc.shouldWork {
				require.NoError(t, err, "Should parse with ID: %s", tc.description)
				require.NotNil(t, ugc, "Config should not be nil")
				
				group, exists := ugc.Groups["test-"+tc.name]
				require.True(t, exists, "Group should exist")
				assert.Equal(t, tc.id, group.ID, "ID should match")
				
				_ = ugc.Close()
				t.Logf("✓ %s: ID=%q works correctly", tc.description, tc.id)
			} else {
				require.Error(t, err, "Should fail with ID: %s", tc.description)
				t.Logf("✓ %s: ID=%q correctly rejected", tc.description, tc.id)
			}
		})
	}
}
