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

// TestCacheFileCreation tests that cache files are actually created.
func TestCacheFileCreation(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")

	// Create test domain list files
	chinaFile := filepath.Join(tmpDir, "china.txt")
	chinaContent := `baidu.com
taobao.com
qq.com`
	err := os.WriteFile(chinaFile, []byte(chinaContent), 0644)
	require.NoError(t, err)

	gfwFile := filepath.Join(tmpDir, "gfw.txt")
	gfwContent := `google.com
youtube.com
facebook.com`
	err = os.WriteFile(gfwFile, []byte(gfwContent), 0644)
	require.NoError(t, err)

	// Create configuration
	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				ID:       "china-id",
				Name:     "china",
				Enabled:  true,
				Upstreams: []string{"223.5.5.5"},
				Mode:     "load_balance",
				Timeout:  "10s",
			},
			{
				ID:       "overseas-id",
				Name:     "overseas",
				Enabled:  true,
				Upstreams: []string{"8.8.8.8"},
				Mode:     "load_balance",
				Timeout:  "10s",
			},
		},
		DefaultGroup: "china-id",
		DomainLists: []DomainListSpec{
			{
				Name:            "china-domains",
				Source:          chinaFile,
				Group:           "china-id",
				File:            filepath.Join(cacheDir, "china-domains.yaml"),
				AutoUpdate:      true,
				RefreshInterval: "6h",
				Enabled:         true,
				Format:          "hosts",
			},
			{
				Name:            "gfwlist",
				Source:          gfwFile,
				Group:           "overseas-id",
				File:            filepath.Join(cacheDir, "gfwlist.yaml"),
				AutoUpdate:      true,
				RefreshInterval: "24h",
				Enabled:         true,
				Format:          "hosts",
			},
		},
		Cache: &CacheConfigSpec{
			Enabled:   true,
			Directory: cacheDir,
			TTL:       "24h",
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	opts := &upstream.Options{
		Logger: logger,
	}

	// Parse configuration (this should create cache files)
	ugc, err := ParseUpstreamGroups(spec, opts)
	require.NoError(t, err)
	require.NotNil(t, ugc)

	// Check that cache directory was created
	_, err = os.Stat(cacheDir)
	assert.NoError(t, err, "cache directory should exist")

	// Check that china-domains.yaml was created
	chinaCache := filepath.Join(cacheDir, "china-domains.yaml")
	_, err = os.Stat(chinaCache)
	assert.NoError(t, err, "china-domains.yaml should exist")

	// Check that gfwlist.yaml was created
	gfwCache := filepath.Join(cacheDir, "gfwlist.yaml")
	_, err = os.Stat(gfwCache)
	assert.NoError(t, err, "gfwlist.yaml should exist")

	// Read and verify china cache content
	if _, err := os.Stat(chinaCache); err == nil {
		content, readErr := os.ReadFile(chinaCache)
		require.NoError(t, readErr)
		
		contentStr := string(content)
		assert.Contains(t, contentStr, "baidu.com")
		assert.Contains(t, contentStr, "taobao.com")
		assert.Contains(t, contentStr, "qq.com")
		
		t.Logf("✓ China cache file created: %s", chinaCache)
		t.Logf("Content preview:\n%s", contentStr[:minInt(len(contentStr), 200)])
	}

	// Read and verify gfw cache content
	if _, err := os.Stat(gfwCache); err == nil {
		content, readErr := os.ReadFile(gfwCache)
		require.NoError(t, readErr)
		
		contentStr := string(content)
		assert.Contains(t, contentStr, "google.com")
		assert.Contains(t, contentStr, "youtube.com")
		assert.Contains(t, contentStr, "facebook.com")
		
		t.Logf("✓ GFW cache file created: %s", gfwCache)
		t.Logf("Content preview:\n%s", contentStr[:minInt(len(contentStr), 200)])
	}

	t.Log("✓ Cache files are created successfully")
}
