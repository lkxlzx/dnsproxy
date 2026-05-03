package proxy

import (
	"log/slog"
	"os"
	"testing"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdGuardHomeFormat tests the AdGuard Home configuration format.
func TestAdGuardHomeFormat(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: testTimeout,
	}

	// AdGuard Home format configuration (simplified - using ID as key)
	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				ID:        "baf42657-598c-4f35-9185-36b9706785d0",
				Name:      "overseas",
				Upstreams: []string{"1.1.1.1", "8.8.8.8"},
				Mode:      "load_balance",
				Timeout:   "10s",
				Enabled:   true,
			},
			{
				ID:        "dd1c17fb-9196-409e-b350-82c829c39d7a",
				Name:      "国内",
				Upstreams: []string{"223.5.5.5", "119.29.29.29"},
				Mode:      "load_balance",
				Timeout:   "10s",
				Enabled:   true,
			},
			{
				ID:        "1376532a-a05d-46ed-a8df-8fefc123d7c4",
				Name:      "默认",
				Upstreams: []string{"114.114.114.114"},
				Mode:      "load_balance",
				Timeout:   "10s",
				Enabled:   true,
			},
		},
		DomainGroups: map[string]interface{}{
			// Using ID as key with nested "domains" field
			"dd1c17fb-9196-409e-b350-82c829c39d7a": map[string]interface{}{
				"domains": []interface{}{
					"baidu.com",
					"taobao.com",
					"qq.com",
					"weixin.qq.com",
					"*.cn",
					"*.com.cn",
				},
			},
			"baf42657-598c-4f35-9185-36b9706785d0": map[string]interface{}{
				"domains": []interface{}{
					"google.com",
					"*.google.com",
					"youtube.com",
					"*.youtube.com",
				},
			},
		},
		DefaultGroup: "1376532a-a05d-46ed-a8df-8fefc123d7c4", // Using ID
	}

	// Parse configuration
	ugc, err := ParseUpstreamGroups(spec, opts)
	require.NoError(t, err, "Should parse AdGuard Home format config")
	require.NotNil(t, ugc)

	t.Cleanup(func() {
		_ = ugc.Close()
	})

	// Verify default group was resolved
	assert.Equal(t, "默认", ugc.DefaultGroup, "Default group should be resolved to name")

	// Test routing
	testCases := []struct {
		name          string
		domain        string
		expectedGroup string
		expectedID    string
	}{
		{
			name:          "Baidu (China)",
			domain:        "baidu.com.",
			expectedGroup: "国内",
			expectedID:    "dd1c17fb-9196-409e-b350-82c829c39d7a",
		},
		{
			name:          "Taobao (China)",
			domain:        "taobao.com.",
			expectedGroup: "国内",
			expectedID:    "dd1c17fb-9196-409e-b350-82c829c39d7a",
		},
		{
			name:          "QQ (China)",
			domain:        "qq.com.",
			expectedGroup: "国内",
			expectedID:    "dd1c17fb-9196-409e-b350-82c829c39d7a",
		},
		{
			name:          "Weixin (China)",
			domain:        "weixin.qq.com.",
			expectedGroup: "国内",
			expectedID:    "dd1c17fb-9196-409e-b350-82c829c39d7a",
		},
		{
			name:          ".cn domain (China)",
			domain:        "example.cn.",
			expectedGroup: "国内",
			expectedID:    "dd1c17fb-9196-409e-b350-82c829c39d7a",
		},
		{
			name:          ".com.cn domain (China)",
			domain:        "test.com.cn.",
			expectedGroup: "国内",
			expectedID:    "dd1c17fb-9196-409e-b350-82c829c39d7a",
		},
		{
			name:          "Google (Overseas)",
			domain:        "google.com.",
			expectedGroup: "overseas",
			expectedID:    "baf42657-598c-4f35-9185-36b9706785d0",
		},
		{
			name:          "Google subdomain (Overseas)",
			domain:        "www.google.com.",
			expectedGroup: "overseas",
			expectedID:    "baf42657-598c-4f35-9185-36b9706785d0",
		},
		{
			name:          "YouTube (Overseas)",
			domain:        "youtube.com.",
			expectedGroup: "overseas",
			expectedID:    "baf42657-598c-4f35-9185-36b9706785d0",
		},
		{
			name:          "YouTube subdomain (Overseas)",
			domain:        "www.youtube.com.",
			expectedGroup: "overseas",
			expectedID:    "baf42657-598c-4f35-9185-36b9706785d0",
		},
		{
			name:          "Unknown domain (Default)",
			domain:        "example.com.",
			expectedGroup: "默认",
			expectedID:    "1376532a-a05d-46ed-a8df-8fefc123d7c4",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			group, err := ugc.GetGroupForDomain(tc.domain)
			require.NoError(t, err, "Should get group for domain %s", tc.domain)
			require.NotNil(t, group, "Group should not be nil for domain %s", tc.domain)
			
			assert.Equal(t, tc.expectedGroup, group.Name,
				"Domain %s should route to group %s", tc.domain, tc.expectedGroup)
			assert.Equal(t, tc.expectedID, group.ID,
				"Domain %s should route to group with ID %s", tc.domain, tc.expectedID)
			
			t.Logf("✓ %s → %s (ID: %s)", tc.domain, group.Name, group.ID)
		})
	}

	t.Log("✓ AdGuard Home format configuration works correctly")
}

// TestAdGuardHomeFormatWithDomainsLists tests AdGuard Home format with domains_lists.
func TestAdGuardHomeFormatWithDomainsLists(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: testTimeout,
	}

	// Create temporary test files
	chinaDomainsFile := "./test-china-domains.txt"
	chinaContent := `server=/163.com/223.5.5.5
server=/alipay.com/223.5.5.5
server=/jd.com/223.5.5.5`
	err := os.WriteFile(chinaDomainsFile, []byte(chinaContent), 0644)
	require.NoError(t, err)
	defer os.Remove(chinaDomainsFile)

	overseasDomainsFile := "./test-overseas-domains.txt"
	overseasContent := `facebook.com
twitter.com
github.com`
	err = os.WriteFile(overseasDomainsFile, []byte(overseasContent), 0644)
	require.NoError(t, err)
	defer os.Remove(overseasDomainsFile)

	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				ID:        "overseas-id",
				Name:      "overseas",
				Upstreams: []string{"1.1.1.1"},
				Mode:      "load_balance",
				Timeout:   "10s",
				Enabled:   true,
			},
			{
				ID:        "china-id",
				Name:      "国内",
				Upstreams: []string{"223.5.5.5"},
				Mode:      "load_balance",
				Timeout:   "10s",
				Enabled:   true,
			},
			{
				ID:        "default-id",
				Name:      "默认",
				Upstreams: []string{"114.114.114.114"},
				Mode:      "load_balance",
				Timeout:   "10s",
				Enabled:   true,
			},
		},
		DomainGroups: map[string]interface{}{
			"china-id": map[string]interface{}{
				"domains": []interface{}{
					"baidu.com",
					"*.baidu.com",
				},
			},
		},
		DomainLists: []DomainListSpec{
			{
				Name:    "china-domains",
				Source:  chinaDomainsFile,
				Group:   "china-id", // Using ID
				Enabled: true,
				Format:  "dnsmasq",
			},
			{
				Name:    "overseas-domains",
				Source:  overseasDomainsFile,
				Group:   "overseas-id", // Using ID
				Enabled: true,
			},
		},
		DefaultGroup: "default-id",
	}

	ugc, err := ParseUpstreamGroups(spec, opts)
	require.NoError(t, err)
	require.NotNil(t, ugc)

	t.Cleanup(func() {
		_ = ugc.Close()
	})

	// Test domains from domain_groups
	t.Run("Domain from domain_groups", func(t *testing.T) {
		group, err := ugc.GetGroupForDomain("baidu.com.")
		require.NoError(t, err)
		require.NotNil(t, group)
		assert.Equal(t, "国内", group.Name)
		assert.Equal(t, "china-id", group.ID)
		t.Logf("✓ baidu.com → %s (ID: %s)", group.Name, group.ID)
	})

	// Test domains from domains_lists (china)
	chinaTestDomains := []string{"163.com.", "alipay.com.", "jd.com."}
	for _, domain := range chinaTestDomains {
		t.Run("China list: "+domain, func(t *testing.T) {
			group, err := ugc.GetGroupForDomain(domain)
			require.NoError(t, err)
			require.NotNil(t, group)
			assert.Equal(t, "国内", group.Name)
			assert.Equal(t, "china-id", group.ID)
			t.Logf("✓ %s → %s (ID: %s)", domain, group.Name, group.ID)
		})
	}

	// Test domains from domains_lists (overseas)
	overseasTestDomains := []string{"facebook.com.", "twitter.com.", "github.com."}
	for _, domain := range overseasTestDomains {
		t.Run("Overseas list: "+domain, func(t *testing.T) {
			group, err := ugc.GetGroupForDomain(domain)
			require.NoError(t, err)
			require.NotNil(t, group)
			assert.Equal(t, "overseas", group.Name)
			assert.Equal(t, "overseas-id", group.ID)
			t.Logf("✓ %s → %s (ID: %s)", domain, group.Name, group.ID)
		})
	}

	// Test unknown domain uses default
	t.Run("Unknown domain uses default", func(t *testing.T) {
		group, err := ugc.GetGroupForDomain("unknown.com.")
		require.NoError(t, err)
		require.NotNil(t, group)
		assert.Equal(t, "默认", group.Name)
		assert.Equal(t, "default-id", group.ID)
		t.Logf("✓ unknown.com → %s (ID: %s)", group.Name, group.ID)
	})

	t.Log("✓ AdGuard Home format with domains_lists works correctly")
}

// TestParseGroupReference tests the parseGroupReference function.
func TestParseGroupReference(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{
			input:    "dd1c17fb-9196-409e-b350-82c829c39d7a",
			expected: "dd1c17fb-9196-409e-b350-82c829c39d7a",
		},
		{
			input:    "baf42657-598c-4f35-9185-36b9706785d0",
			expected: "baf42657-598c-4f35-9185-36b9706785d0",
		},
		{
			input:    "simple-name",
			expected: "simple-name",
		},
		{
			input:    "国内",
			expected: "国内",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := parseGroupReference(tc.input)
			assert.Equal(t, tc.expected, result)
			t.Logf("✓ %q → %q", tc.input, result)
		})
	}
}
