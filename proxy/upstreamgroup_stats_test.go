package proxy

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDomainListStats tests domain count and last updated tracking.
func TestDomainListStats(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test domain list file
	testDomains := `google.com
youtube.com
facebook.com`
	
	tmpFile := tmpDir + "/test-domains.txt"
	err := os.WriteFile(tmpFile, []byte(testDomains), 0644)
	require.NoError(t, err)

	// Create manager
	manager := NewDomainListManager(tmpDir, nil)

	// Add a managed list
	managedList := &ManagedList{
		Name:        "test-list",
		Source:      tmpFile,
		LocalPath:   tmpDir + "/test-cache.yaml",
		Group:       "test-group",
		Enabled:     true,
		LastUpdate:  time.Now(),
		DomainCount: 3,
		AutoUpdate:  true,
		Format:      "hosts",
	}

	err = manager.AddList(managedList)
	require.NoError(t, err)

	// Get stats
	stats := manager.GetAllStats()
	require.Len(t, stats, 1)

	// Verify stats
	assert.Equal(t, "test-list", stats[0]["name"])
	assert.Equal(t, 3, stats[0]["domain_count"])
	assert.NotEmpty(t, stats[0]["last_updated"])
	assert.Equal(t, true, stats[0]["enabled"])
	assert.Equal(t, "test-group", stats[0]["group"])

	t.Log("✓ Domain list stats tracking works correctly")
}

// TestManagedListGetStats tests the GetStats method.
func TestManagedListGetStats(t *testing.T) {
	now := time.Now()
	
	list := &ManagedList{
		Name:        "gfwlist",
		Source:      "https://example.com/gfwlist.txt",
		LocalPath:   "./cache/gfwlist.yaml",
		Group:       "overseas",
		Enabled:     true,
		LastUpdate:  now,
		DomainCount: 1234,
		AutoUpdate:  true,
		Format:      "gfwlist",
	}

	stats := list.GetStats()

	assert.Equal(t, "gfwlist", stats["name"])
	assert.Equal(t, "https://example.com/gfwlist.txt", stats["source"])
	assert.Equal(t, "overseas", stats["group"])
	assert.Equal(t, true, stats["enabled"])
	assert.Equal(t, 1234, stats["domain_count"])
	assert.Equal(t, now.Format(time.RFC3339), stats["last_updated"])
	assert.Equal(t, true, stats["auto_update"])
	assert.Equal(t, "gfwlist", stats["format"])

	t.Log("✓ ManagedList.GetStats() works correctly")
}
