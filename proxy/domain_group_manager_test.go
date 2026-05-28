package proxy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDomainGroupManager_MatchDomain(t *testing.T) {
	// Create temporary domain list file
	tmpDir := t.TempDir()
	domainFile := filepath.Join(tmpDir, "domains.txt")
	
	content := `# Test domains
example.com
test.org
google.com
`
	err := os.WriteFile(domainFile, []byte(content), 0644)
	require.NoError(t, err)

	groups := []DomainGroupConfig{
		{
			GroupName:      "test-group",
			DomainFile:     domainFile,
			DomainFileFormat: FormatPlain,
			Upstreams:      []string{"1.1.1.1"},
			SubdomainsOnly: false,
			Enabled:        true,
		},
	}

	opts := &upstream.Options{}
	mgr, err := NewDomainGroupManager(groups, opts)
	require.NoError(t, err)

	testCases := []struct {
		name     string
		domain   string
		wantMatch bool
	}{
		{
			name:     "exact_match",
			domain:   "example.com",
			wantMatch: true,
		},
		{
			name:     "subdomain_match",
			domain:   "www.example.com",
			wantMatch: true,
		},
		{
			name:     "no_match",
			domain:   "notinlist.com",
			wantMatch: false,
		},
		{
			name:     "trailing_dot",
			domain:   "test.org.",
			wantMatch: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			upstreams, err := mgr.MatchDomain(tc.domain)
			require.NoError(t, err)

			if tc.wantMatch {
				assert.NotEmpty(t, upstreams, "expected match for %s", tc.domain)
			} else {
				assert.Empty(t, upstreams, "expected no match for %s", tc.domain)
			}
		})
	}
}

func TestDomainGroupManager_SubdomainsOnly(t *testing.T) {
	tmpDir := t.TempDir()
	domainFile := filepath.Join(tmpDir, "domains.txt")
	
	content := "example.com\n"
	err := os.WriteFile(domainFile, []byte(content), 0644)
	require.NoError(t, err)

	groups := []DomainGroupConfig{
		{
			GroupName:      "subdomain-only",
			DomainFile:     domainFile,
			DomainFileFormat: FormatPlain,
			Upstreams:      []string{"1.1.1.1"},
			SubdomainsOnly: true,
			Enabled:        true,
		},
	}

	opts := &upstream.Options{}
	mgr, err := NewDomainGroupManager(groups, opts)
	require.NoError(t, err)

	// Should NOT match the domain itself
	upstreams, err := mgr.MatchDomain("example.com")
	require.NoError(t, err)
	assert.Empty(t, upstreams)

	// Should match subdomains
	upstreams, err = mgr.MatchDomain("www.example.com")
	require.NoError(t, err)
	assert.NotEmpty(t, upstreams)
}

func TestDomainGroupManager_WildcardSupport(t *testing.T) {
	tmpDir := t.TempDir()
	domainFile := filepath.Join(tmpDir, "domains.txt")
	
	// 测试通配符格式 (*.example.com)
	content := `# 通配符格式测试
*.example.com
*.test.org
normal.com
`
	err := os.WriteFile(domainFile, []byte(content), 0644)
	require.NoError(t, err)

	groups := []DomainGroupConfig{
		{
			GroupName:      "wildcard-test",
			DomainFile:     domainFile,
			DomainFileFormat: FormatPlain,
			Upstreams:      []string{"1.1.1.1"},
			SubdomainsOnly: false,
			Enabled:        true,
		},
	}

	opts := &upstream.Options{}
	mgr, err := NewDomainGroupManager(groups, opts)
	require.NoError(t, err)

	testCases := []struct {
		name      string
		domain    string
		wantMatch bool
	}{
		{
			name:      "wildcard_subdomain_match",
			domain:    "www.example.com",
			wantMatch: true,
		},
		{
			name:      "wildcard_deep_subdomain_match",
			domain:    "api.www.example.com",
			wantMatch: true,
		},
		{
			name:      "wildcard_no_match_base_domain",
			domain:    "example.com",
			wantMatch: false,
		},
		{
			name:      "wildcard_test_org_match",
			domain:    "sub.test.org",
			wantMatch: true,
		},
		{
			name:      "normal_domain_exact_match",
			domain:    "normal.com",
			wantMatch: true,
		},
		{
			name:      "normal_domain_subdomain_match",
			domain:    "www.normal.com",
			wantMatch: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			upstreams, err := mgr.MatchDomain(tc.domain)
			require.NoError(t, err)

			if tc.wantMatch {
				assert.NotEmpty(t, upstreams, "expected match for %s", tc.domain)
			} else {
				assert.Empty(t, upstreams, "expected no match for %s", tc.domain)
			}
		})
	}
}

func TestDomainGroupManager_EnableDisable(t *testing.T) {
	tmpDir := t.TempDir()
	domainFile := filepath.Join(tmpDir, "domains.txt")
	
	content := "example.com\n"
	err := os.WriteFile(domainFile, []byte(content), 0644)
	require.NoError(t, err)

	groups := []DomainGroupConfig{
		{
			GroupName:      "test-group",
			DomainFile:     domainFile,
			DomainFileFormat: FormatPlain,
			Upstreams:      []string{"1.1.1.1"},
			SubdomainsOnly: false,
			Enabled:        true,
		},
	}

	opts := &upstream.Options{}
	mgr, err := NewDomainGroupManager(groups, opts)
	require.NoError(t, err)

	// Initially enabled - should match
	upstreams, err := mgr.MatchDomain("example.com")
	require.NoError(t, err)
	assert.NotEmpty(t, upstreams)

	// Disable the group
	err = mgr.DisableGroup("test-group")
	require.NoError(t, err)

	// Should NOT match when disabled
	upstreams, err = mgr.MatchDomain("example.com")
	require.NoError(t, err)
	assert.Empty(t, upstreams)

	// Re-enable the group
	err = mgr.EnableGroup("test-group")
	require.NoError(t, err)

	// Should match again
	upstreams, err = mgr.MatchDomain("example.com")
	require.NoError(t, err)
	assert.NotEmpty(t, upstreams)
}

func TestDomainGroupManager_GetGroups(t *testing.T) {
	tmpDir := t.TempDir()
	domainFile := filepath.Join(tmpDir, "domains.txt")
	
	content := "example.com\ntest.org\n"
	err := os.WriteFile(domainFile, []byte(content), 0644)
	require.NoError(t, err)

	groups := []DomainGroupConfig{
		{
			GroupName:      "group1",
			DomainFile:     domainFile,
			DomainFileFormat: FormatPlain,
			Upstreams:      []string{"1.1.1.1", "8.8.8.8"},
			SubdomainsOnly: false,
			Enabled:        true,
		},
	}

	opts := &upstream.Options{}
	mgr, err := NewDomainGroupManager(groups, opts)
	require.NoError(t, err)

	status := mgr.GetGroups()
	require.Len(t, status, 1)

	assert.Equal(t, "group1", status[0].GroupName)
	assert.Equal(t, domainFile, status[0].DomainFile)
	assert.True(t, status[0].Enabled)
	assert.Equal(t, 2, status[0].DomainCount)
	assert.Equal(t, 2, status[0].UpstreamCount)
	assert.False(t, status[0].SubdomainsOnly)
}

func TestDomainGroupManager_ReloadGroup(t *testing.T) {
	tmpDir := t.TempDir()
	domainFile := filepath.Join(tmpDir, "domains.txt")
	
	// Initial content
	content := "example.com\n"
	err := os.WriteFile(domainFile, []byte(content), 0644)
	require.NoError(t, err)

	groups := []DomainGroupConfig{
		{
			GroupName:      "test-group",
			DomainFile:     domainFile,
			DomainFileFormat: FormatPlain,
			Upstreams:      []string{"1.1.1.1"},
			SubdomainsOnly: false,
			Enabled:        true,
		},
	}

	opts := &upstream.Options{}
	mgr, err := NewDomainGroupManager(groups, opts)
	require.NoError(t, err)

	// Should match example.com
	upstreams, err := mgr.MatchDomain("example.com")
	require.NoError(t, err)
	assert.NotEmpty(t, upstreams)

	// Should NOT match test.org yet
	upstreams, err = mgr.MatchDomain("test.org")
	require.NoError(t, err)
	assert.Empty(t, upstreams)

	// Update the file
	newContent := "example.com\ntest.org\n"
	err = os.WriteFile(domainFile, []byte(newContent), 0644)
	require.NoError(t, err)

	// Reload the group
	err = mgr.ReloadGroup("test-group")
	require.NoError(t, err)

	// Now should match test.org
	upstreams, err = mgr.MatchDomain("test.org")
	require.NoError(t, err)
	assert.NotEmpty(t, upstreams)
}

func TestDomainGroupManager_KeywordMatch(t *testing.T) {
	tmpDir := t.TempDir()
	domainFile := filepath.Join(tmpDir, "domains.txt")

	// 测试关键字匹配
	content := `# 关键字匹配测试
keyword:ad
keyword:tracker
keyword:analytics
example.com
`
	err := os.WriteFile(domainFile, []byte(content), 0644)
	require.NoError(t, err)

	groups := []DomainGroupConfig{
		{
			GroupName:        "keyword-test",
			DomainFile:       domainFile,
			DomainFileFormat: FormatPlain,
			Upstreams:        []string{"1.1.1.1"},
			SubdomainsOnly:   false,
			Enabled:          true,
		},
	}

	opts := &upstream.Options{}
	mgr, err := NewDomainGroupManager(groups, opts)
	require.NoError(t, err)

	testCases := []struct {
		name      string
		domain    string
		wantMatch bool
	}{
		{
			name:      "keyword_ad_match",
			domain:    "ad.example.com",
			wantMatch: true,
		},
		{
			name:      "keyword_ad_in_middle",
			domain:    "www.ad-server.com",
			wantMatch: true,
		},
		{
			name:      "keyword_tracker_match",
			domain:    "tracker.example.com",
			wantMatch: true,
		},
		{
			name:      "keyword_analytics_match",
			domain:    "analytics.google.com",
			wantMatch: true,
		},
		{
			name:      "keyword_no_match",
			domain:    "www.example.org",
			wantMatch: false,
		},
		{
			name:      "normal_domain_match",
			domain:    "example.com",
			wantMatch: true,
		},
		{
			name:      "normal_domain_subdomain",
			domain:    "www.example.com",
			wantMatch: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			upstreams, err := mgr.MatchDomain(tc.domain)
			require.NoError(t, err)

			if tc.wantMatch {
				assert.NotEmpty(t, upstreams, "expected match for %s", tc.domain)
			} else {
				assert.Empty(t, upstreams, "expected no match for %s", tc.domain)
			}
		})
	}
}
