package proxy

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a domain group manager for testing
func createTestDomainGroupManager(t *testing.T, groups []DomainGroupConfig) *DomainGroupManager {
	t.Helper()
	opts := &upstream.Options{}
	mgr, err := NewDomainGroupManager(groups, []UpstreamGroup{}, opts, slog.Default())
	require.NoError(t, err)
	return mgr
}

func TestDomainGroupManager_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	domainFile := filepath.Join(tmpDir, "domains.txt")
	
	content := "example.com\ntest.org\n"
	err := os.WriteFile(domainFile, []byte(content), 0644)
	require.NoError(t, err)

	groups := []DomainGroupConfig{
		{
			GroupName:        "test-group",
			DomainFile:       domainFile,
			DomainFileFormat: FormatPlain,
			Upstreams:        []string{"1.1.1.1"},
			Enabled:          true,
		},
	}

	mgr := createTestDomainGroupManager(t, groups)

	// Test matching
	upstreams, err := mgr.MatchDomain("example.com")
	require.NoError(t, err)
	assert.NotEmpty(t, upstreams)

	// Test non-matching
	upstreams, err = mgr.MatchDomain("notinlist.com")
	require.NoError(t, err)
	assert.Empty(t, upstreams)
}

func TestDomainGroupManager_EnableDisable(t *testing.T) {
	tmpDir := t.TempDir()
	domainFile := filepath.Join(tmpDir, "domains.txt")
	
	content := "example.com\n"
	err := os.WriteFile(domainFile, []byte(content), 0644)
	require.NoError(t, err)

	groups := []DomainGroupConfig{
		{
			GroupName:        "test-group",
			DomainFile:       domainFile,
			DomainFileFormat: FormatPlain,
			Upstreams:        []string{"1.1.1.1"},
			Enabled:          true,
		},
	}

	mgr := createTestDomainGroupManager(t, groups)

	// Initially enabled
	upstreams, err := mgr.MatchDomain("example.com")
	require.NoError(t, err)
	assert.NotEmpty(t, upstreams)

	// Disable
	err = mgr.DisableGroup("test-group")
	require.NoError(t, err)

	upstreams, err = mgr.MatchDomain("example.com")
	require.NoError(t, err)
	assert.Empty(t, upstreams)

	// Re-enable
	err = mgr.EnableGroup("test-group")
	require.NoError(t, err)

	upstreams, err = mgr.MatchDomain("example.com")
	require.NoError(t, err)
	assert.NotEmpty(t, upstreams)
}
