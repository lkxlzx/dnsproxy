package proxy

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDomainListManager tests the complete domain list management workflow.
func TestDomainListManager(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	
	// Create temporary cache directory
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	
	// Create manager
	manager := NewDomainListManager(cacheDir, logger)
	
	t.Run("AddList", func(t *testing.T) {
		list := &ManagedList{
			Name:       "test",
			Source:     "https://example.com/domains.txt",
			LocalPath:  filepath.Join(cacheDir, "test.txt"),
			Group:      "test-group",
			Enabled:    true,
			AutoUpdate: false,
		}
		
		err := manager.AddList(list)
		if err != nil {
			t.Fatalf("AddList failed: %v", err)
		}
		
		// Verify list was added
		retrieved, err := manager.GetList("test")
		if err != nil {
			t.Fatalf("GetList failed: %v", err)
		}
		
		if retrieved.Name != "test" {
			t.Errorf("Expected name 'test', got %q", retrieved.Name)
		}
	})
	
	t.Run("ListAll", func(t *testing.T) {
		lists := manager.ListAll()
		if len(lists) != 1 {
			t.Errorf("Expected 1 list, got %d", len(lists))
		}
	})
	
	t.Run("RemoveList", func(t *testing.T) {
		err := manager.RemoveList("test")
		if err != nil {
			t.Fatalf("RemoveList failed: %v", err)
		}
		
		// Verify list was removed
		_, err = manager.GetList("test")
		if err == nil {
			t.Error("Expected error when getting removed list")
		}
	})
}

// TestDomainListCache tests the caching functionality.
func TestDomainListCache(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	
	tmpDir := t.TempDir()
	cache := NewDomainListCache(tmpDir, logger)
	
	source := "https://example.com/test.txt"
	content := []byte("example.com\ntest.com\n")
	
	t.Run("SaveToCache", func(t *testing.T) {
		err := cache.SaveToCache(source, content)
		if err != nil {
			t.Fatalf("SaveToCache failed: %v", err)
		}
		
		// Verify cache file exists
		cachePath := cache.GetCachePath(source)
		if _, err := os.Stat(cachePath); os.IsNotExist(err) {
			t.Error("Cache file was not created")
		}
	})
	
	t.Run("IsCached", func(t *testing.T) {
		// Should be cached with no TTL
		if !cache.IsCached(source, 0) {
			t.Error("Expected source to be cached")
		}
		
		// Should be cached with long TTL
		if !cache.IsCached(source, 24*time.Hour) {
			t.Error("Expected source to be cached")
		}
	})
	
	t.Run("LoadFromCache", func(t *testing.T) {
		loaded, err := cache.LoadFromCache(source)
		if err != nil {
			t.Fatalf("LoadFromCache failed: %v", err)
		}
		
		if string(loaded) != string(content) {
			t.Errorf("Expected %q, got %q", string(content), string(loaded))
		}
	})
	
	t.Run("GetCacheInfo", func(t *testing.T) {
		info, err := cache.GetCacheInfo(source)
		if err != nil {
			t.Fatalf("GetCacheInfo failed: %v", err)
		}
		
		if info.Source != source {
			t.Errorf("Expected source %q, got %q", source, info.Source)
		}
	})
}

// TestDownloadAndCacheWorkflow tests the complete download and cache workflow.
func TestDownloadAndCacheWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping download test in short mode")
	}
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	
	manager := NewDomainListManager(cacheDir, logger)
	
	t.Run("DownloadAndCache", func(t *testing.T) {
		// Use a small test file
		source := "https://raw.githubusercontent.com/gfwlist/gfwlist/refs/heads/master/gfwlist.txt"
		localPath := filepath.Join(cacheDir, "gfwlist.txt")
		
		t.Log("Downloading and caching domain list...")
		start := time.Now()
		
		err := manager.DownloadAndCache(source, localPath)
		
		elapsed := time.Since(start)
		t.Logf("Download and cache took: %v", elapsed)
		
		if err != nil {
			t.Fatalf("DownloadAndCache failed: %v", err)
		}
		
		// Verify local file was created
		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			t.Error("Local file was not created")
		}
		
		// Verify cache was created
		cachePath := manager.GetCachePath(source)
		if _, err := os.Stat(cachePath); os.IsNotExist(err) {
			t.Error("Cache file was not created")
		}
		
		t.Log("✓ Download and cache successful")
	})
}

// TestBuildDomainGroupsConfig tests configuration building.
func TestBuildDomainGroupsConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	
	tmpDir := t.TempDir()
	manager := NewDomainListManager(tmpDir, logger)
	
	// Add some lists
	lists := []*ManagedList{
		{
			Name:      "china",
			Source:    "https://example.com/china.txt",
			LocalPath: "/cache/china.txt",
			Group:     "china",
			Enabled:   true,
		},
		{
			Name:      "ads",
			Source:    "https://example.com/ads.txt",
			LocalPath: "/cache/ads.txt",
			Group:     "adblock",
			Enabled:   true,
		},
		{
			Name:      "disabled",
			Source:    "https://example.com/disabled.txt",
			LocalPath: "/cache/disabled.txt",
			Group:     "test",
			Enabled:   false, // Disabled
		},
	}
	
	for _, list := range lists {
		manager.AddList(list)
	}
	
	// Build configuration
	config := manager.BuildDomainGroupsConfig()
	
	// Should have 2 entries (disabled one excluded)
	if len(config) != 2 {
		t.Errorf("Expected 2 config entries, got %d", len(config))
	}
	
	// Verify mappings
	if config["china"] != "/cache/china.txt" {
		t.Errorf("Expected china -> /cache/china.txt, got %q", config["china"])
	}
	
	if config["adblock"] != "/cache/ads.txt" {
		t.Errorf("Expected adblock -> /cache/ads.txt, got %q", config["adblock"])
	}
	
	// Disabled list should not be in config
	if _, exists := config["test"]; exists {
		t.Error("Disabled list should not be in config")
	}
	
	t.Log("✓ Configuration built correctly")
}
