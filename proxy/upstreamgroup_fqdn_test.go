package proxy

import (
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFQDNDomainMatching tests that domains with and without trailing dots match correctly.
// This is critical for AdGuard Home integration where DNS queries use FQDN format (with trailing dot).
func TestFQDNDomainMatching(t *testing.T) {
	t.Parallel()

	// Create test upstreams
	chinaUpstream, err := upstream.AddressToUpstream("223.5.5.5", &upstream.Options{Timeout: 5 * time.Second})
	require.NoError(t, err)

	overseasUpstream, err := upstream.AddressToUpstream("8.8.8.8", &upstream.Options{Timeout: 5 * time.Second})
	require.NoError(t, err)

	defaultUpstream, err := upstream.AddressToUpstream("114.114.114.114", &upstream.Options{Timeout: 5 * time.Second})
	require.NoError(t, err)

	// Create config
	config := NewUpstreamGroupConfig()

	// Add groups
	chinaGroup := &UpstreamGroup{
		ID:       "china-id",
		Name:     "国内",
		Upstreams: []upstream.Upstream{chinaUpstream},
		Mode:     UpstreamModeLoadBalance,
		Timeout:  5 * time.Second,
		Enabled:  true,
	}
	require.NoError(t, config.AddGroup(chinaGroup))

	overseasGroup := &UpstreamGroup{
		ID:       "overseas-id",
		Name:     "overseas",
		Upstreams: []upstream.Upstream{overseasUpstream},
		Mode:     UpstreamModeLoadBalance,
		Timeout:  5 * time.Second,
		Enabled:  true,
	}
	require.NoError(t, config.AddGroup(overseasGroup))

	defaultGroup := &UpstreamGroup{
		ID:       "default-id",
		Name:     "默认",
		Upstreams: []upstream.Upstream{defaultUpstream},
		Mode:     UpstreamModeLoadBalance,
		Timeout:  5 * time.Second,
		Enabled:  true,
	}
	require.NoError(t, config.AddGroup(defaultGroup))

	config.DefaultGroup = "默认"

	// Add domain mappings (stored without trailing dot)
	require.NoError(t, config.SetDomainGroup("baidu.com", "国内"))
	require.NoError(t, config.SetDomainGroup("qq.com", "国内"))
	require.NoError(t, config.SetDomainGroup("*.cn", "国内"))
	require.NoError(t, config.SetDomainGroup("google.com", "overseas"))
	require.NoError(t, config.SetDomainGroup("*.google.com", "overseas"))

	tests := []struct {
		name          string
		queryDomain   string // Domain as it appears in DNS query (FQDN with trailing dot)
		expectedGroup string
		expectedID    string
	}{
		{
			name:          "Exact match with FQDN (baidu.com.)",
			queryDomain:   "baidu.com.",
			expectedGroup: "国内",
			expectedID:    "china-id",
		},
		{
			name:          "Exact match without trailing dot (baidu.com)",
			queryDomain:   "baidu.com",
			expectedGroup: "国内",
			expectedID:    "china-id",
		},
		{
			name:          "Exact match with FQDN (qq.com.)",
			queryDomain:   "qq.com.",
			expectedGroup: "国内",
			expectedID:    "china-id",
		},
		{
			name:          "Wildcard match with FQDN (example.cn.)",
			queryDomain:   "example.cn.",
			expectedGroup: "国内",
			expectedID:    "china-id",
		},
		{
			name:          "Wildcard match without trailing dot (example.cn)",
			queryDomain:   "example.cn",
			expectedGroup: "国内",
			expectedID:    "china-id",
		},
		{
			name:          "Subdomain wildcard with FQDN (www.google.com.)",
			queryDomain:   "www.google.com.",
			expectedGroup: "overseas",
			expectedID:    "overseas-id",
		},
		{
			name:          "Subdomain wildcard without trailing dot (www.google.com)",
			queryDomain:   "www.google.com",
			expectedGroup: "overseas",
			expectedID:    "overseas-id",
		},
		{
			name:          "Exact match takes precedence (google.com.)",
			queryDomain:   "google.com.",
			expectedGroup: "overseas",
			expectedID:    "overseas-id",
		},
		{
			name:          "Unknown domain with FQDN (unknown.com.)",
			queryDomain:   "unknown.com.",
			expectedGroup: "默认",
			expectedID:    "default-id",
		},
		{
			name:          "Unknown domain without trailing dot (unknown.com)",
			queryDomain:   "unknown.com",
			expectedGroup: "默认",
			expectedID:    "default-id",
		},
		{
			name:          "Mixed case with FQDN (BAIDU.COM.)",
			queryDomain:   "BAIDU.COM.",
			expectedGroup: "国内",
			expectedID:    "china-id",
		},
		{
			name:          "Mixed case without trailing dot (QQ.COM)",
			queryDomain:   "QQ.COM",
			expectedGroup: "国内",
			expectedID:    "china-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group, err := config.GetGroupForDomain(tt.queryDomain)
			require.NoError(t, err)
			require.NotNil(t, group)

			assert.Equal(t, tt.expectedGroup, group.Name, "Group name mismatch for %s", tt.queryDomain)
			assert.Equal(t, tt.expectedID, group.ID, "Group ID mismatch for %s", tt.queryDomain)

			t.Logf("✓ %s → %s (ID: %s)", tt.queryDomain, group.Name, group.ID)
		})
	}

	t.Log("✓ FQDN domain matching works correctly")
}

// TestAdGuardHomeConfigWithFQDN tests AdGuard Home configuration format with FQDN queries.
func TestAdGuardHomeConfigWithFQDN(t *testing.T) {
	t.Parallel()

	// Create spec
	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				ID:       "dd1c17fb-9196-409e-b350-82c829c39d7a",
				Name:     "国内",
				Upstreams: []string{"223.5.5.5"},
				Mode:     "load_balance",
				Timeout:  "5s",
				Enabled:  true,
			},
			{
				ID:       "baf42657-598c-4f35-9185-36b9706785d0",
				Name:     "overseas",
				Upstreams: []string{"8.8.8.8"},
				Mode:     "load_balance",
				Timeout:  "5s",
				Enabled:  true,
			},
		},
		DomainGroups: map[string]interface{}{
			"dd1c17fb-9196-409e-b350-82c829c39d7a": map[string]interface{}{
				"domains": []interface{}{"baidu.com", "taobao.com", "qq.com"},
			},
			"baf42657-598c-4f35-9185-36b9706785d0": map[string]interface{}{
				"domains": []interface{}{"google.com", "youtube.com"},
			},
		},
		DefaultGroup: "dd1c17fb-9196-409e-b350-82c829c39d7a",
	}

	config, err := ParseUpstreamGroups(spec, &upstream.Options{Timeout: 5 * time.Second})
	require.NoError(t, err)
	require.NotNil(t, config)

	// Test with FQDN format (as DNS queries would use)
	tests := []struct {
		domain        string
		expectedGroup string
		expectedID    string
	}{
		{"baidu.com.", "国内", "dd1c17fb-9196-409e-b350-82c829c39d7a"},
		{"taobao.com.", "国内", "dd1c17fb-9196-409e-b350-82c829c39d7a"},
		{"qq.com.", "国内", "dd1c17fb-9196-409e-b350-82c829c39d7a"},
		{"google.com.", "overseas", "baf42657-598c-4f35-9185-36b9706785d0"},
		{"youtube.com.", "overseas", "baf42657-598c-4f35-9185-36b9706785d0"},
		{"unknown.com.", "国内", "dd1c17fb-9196-409e-b350-82c829c39d7a"}, // Default group
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			group, err := config.GetGroupForDomain(tt.domain)
			require.NoError(t, err)
			require.NotNil(t, group)

			assert.Equal(t, tt.expectedGroup, group.Name)
			assert.Equal(t, tt.expectedID, group.ID)

			t.Logf("✓ %s → %s (ID: %s)", tt.domain, group.Name, group.ID)
		})
	}

	t.Log("✓ AdGuard Home config with FQDN queries works correctly")
}

// TestDomainNormalization tests that domain normalization works correctly.
func TestDomainNormalization(t *testing.T) {
	t.Parallel()

	config := NewUpstreamGroupConfig()

	// Create a test group
	testUpstream, err := upstream.AddressToUpstream("8.8.8.8", &upstream.Options{Timeout: 5 * time.Second})
	require.NoError(t, err)

	testGroup := &UpstreamGroup{
		ID:       "test-id",
		Name:     "test",
		Upstreams: []upstream.Upstream{testUpstream},
		Mode:     UpstreamModeLoadBalance,
		Timeout:  5 * time.Second,
		Enabled:  true,
	}
	require.NoError(t, config.AddGroup(testGroup))

	// Test: Add domain without dot, query with dot
	err = config.SetDomainGroup("test1.com", "test")
	require.NoError(t, err)
	
	group, err := config.GetGroupForDomain("test1.com.")
	require.NoError(t, err)
	require.NotNil(t, group)
	assert.Equal(t, "test", group.Name)
	t.Log("✓ test1.com matches test1.com.")

	// Test: Add domain with dot, query without dot (normalized to same)
	err = config.SetDomainGroup("test2.com.", "test")
	require.NoError(t, err)
	
	group, err = config.GetGroupForDomain("test2.com")
	require.NoError(t, err)
	require.NotNil(t, group)
	assert.Equal(t, "test", group.Name)
	t.Log("✓ test2.com. matches test2.com")

	// Test: Case insensitive
	err = config.SetDomainGroup("TEST3.COM", "test")
	require.NoError(t, err)
	
	group, err = config.GetGroupForDomain("test3.com.")
	require.NoError(t, err)
	require.NotNil(t, group)
	assert.Equal(t, "test", group.Name)
	t.Log("✓ TEST3.COM matches test3.com. (case insensitive)")

	t.Log("✓ Domain normalization works correctly")
}
