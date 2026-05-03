package proxy

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCompleteWorkflow tests the complete workflow including:
// - Local file loading
// - Remote URL downloading
// - Caching
// - Auto-update
// - Format conversion
func TestCompleteWorkflow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	
	// Create temporary directories
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	domainsDir := filepath.Join(tmpDir, "domains")
	
	err := os.MkdirAll(domainsDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create domains dir: %v", err)
	}
	
	t.Run("Step1_LocalFileLoading", func(t *testing.T) {
		t.Log("=== Testing Local File Loading ===")
		
		// Create a local domain file
		localFile := filepath.Join(domainsDir, "local.txt")
		content := "example.com\ntest.com\nlocal.dev\n"
		err := os.WriteFile(localFile, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to write local file: %v", err)
		}
		
		// Load domains from local file
		loader := NewDomainFileLoader(logger)
		domains, err := loader.LoadDomains(localFile)
		if err != nil {
			t.Fatalf("Failed to load local file: %v", err)
		}
		
		t.Logf("✓ Loaded %d domains from local file", len(domains))
		
		if len(domains) != 3 {
			t.Errorf("Expected 3 domains, got %d", len(domains))
		}
		
		expectedDomains := map[string]bool{
			"example.com": true,
			"test.com":    true,
			"local.dev":   true,
		}
		
		for _, domain := range domains {
			if !expectedDomains[domain] {
				t.Errorf("Unexpected domain: %s", domain)
			}
		}
		
		t.Log("✓ Local file loading: PASS")
	})
	
	t.Run("Step2_RemoteURLDownloading", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping remote download in short mode")
		}
		
		t.Log("=== Testing Remote URL Downloading ===")
		
		// Use a real remote URL (GFWList)
		remoteURL := "https://raw.githubusercontent.com/gfwlist/gfwlist/refs/heads/master/gfwlist.txt"
		
		loader := NewDomainFileLoader(logger)
		
		t.Log("Downloading from remote URL...")
		start := time.Now()
		
		domains, err := loader.LoadDomains(remoteURL)
		
		elapsed := time.Since(start)
		t.Logf("Download took: %v", elapsed)
		
		if err != nil {
			t.Fatalf("Failed to download remote URL: %v", err)
		}
		
		t.Logf("✓ Downloaded and parsed %d domains", len(domains))
		
		if len(domains) == 0 {
			t.Error("Expected non-zero domains")
		}
		
		t.Log("✓ Remote URL downloading: PASS")
	})
	
	t.Run("Step3_CachingFunctionality", func(t *testing.T) {
		t.Log("=== Testing Caching Functionality ===")
		
		cache := NewDomainListCache(cacheDir, logger)
		
		// Test data
		source := "https://example.com/test.txt"
		content := []byte("example.com\ntest.com\n")
		
		// Step 3.1: Save to cache
		t.Log("Step 3.1: Saving to cache...")
		err := cache.SaveToCache(source, content)
		if err != nil {
			t.Fatalf("Failed to save to cache: %v", err)
		}
		t.Log("✓ Saved to cache")
		
		// Step 3.2: Check if cached
		t.Log("Step 3.2: Checking if cached...")
		if !cache.IsCached(source, 24*time.Hour) {
			t.Error("Expected source to be cached")
		}
		t.Log("✓ Cache exists and is valid")
		
		// Step 3.3: Load from cache
		t.Log("Step 3.3: Loading from cache...")
		cached, err := cache.LoadFromCache(source)
		if err != nil {
			t.Fatalf("Failed to load from cache: %v", err)
		}
		
		if string(cached) != string(content) {
			t.Errorf("Cache content mismatch")
		}
		t.Log("✓ Loaded from cache successfully")
		
		// Step 3.4: Get cache info
		t.Log("Step 3.4: Getting cache info...")
		info, err := cache.GetCacheInfo(source)
		if err != nil {
			t.Fatalf("Failed to get cache info: %v", err)
		}
		
		t.Logf("✓ Cache info: source=%s, path=%s, updated=%v",
			info.Source, info.CachedPath, info.LastUpdate)
		
		// Step 3.5: Test cache expiration
		t.Log("Step 3.5: Testing cache expiration...")
		// With very short TTL, cache should be considered expired
		if cache.IsCached(source, 1*time.Nanosecond) {
			t.Error("Cache should be expired with 1ns TTL")
		}
		t.Log("✓ Cache expiration works correctly")
		
		t.Log("✓ Caching functionality: PASS")
	})
	
	t.Run("Step4_DomainListManager", func(t *testing.T) {
		t.Log("=== Testing Domain List Manager ===")
		
		manager := NewDomainListManager(cacheDir, logger)
		
		// Step 4.1: Add lists
		t.Log("Step 4.1: Adding managed lists...")
		lists := []*ManagedList{
			{
				Name:       "local",
				Source:     filepath.Join(domainsDir, "local.txt"),
				LocalPath:  filepath.Join(cacheDir, "local.txt"),
				Group:      "local",
				Enabled:    true,
				AutoUpdate: false,
			},
			{
				Name:       "test",
				Source:     "https://example.com/test.txt",
				LocalPath:  filepath.Join(cacheDir, "test.txt"),
				Group:      "test",
				Enabled:    true,
				AutoUpdate: true,
			},
		}
		
		for _, list := range lists {
			err := manager.AddList(list)
			if err != nil {
				t.Fatalf("Failed to add list %s: %v", list.Name, err)
			}
			t.Logf("✓ Added list: %s", list.Name)
		}
		
		// Step 4.2: List all
		t.Log("Step 4.2: Listing all managed lists...")
		allLists := manager.ListAll()
		if len(allLists) != 2 {
			t.Errorf("Expected 2 lists, got %d", len(allLists))
		}
		t.Logf("✓ Total managed lists: %d", len(allLists))
		
		// Step 4.3: Get specific list
		t.Log("Step 4.3: Getting specific list...")
		list, err := manager.GetList("local")
		if err != nil {
			t.Fatalf("Failed to get list: %v", err)
		}
		t.Logf("✓ Retrieved list: %s (group=%s)", list.Name, list.Group)
		
		// Step 4.4: Build configuration
		t.Log("Step 4.4: Building domain groups configuration...")
		config := manager.BuildDomainGroupsConfig()
		t.Logf("✓ Generated config with %d groups", len(config))
		
		for group, path := range config {
			t.Logf("  - %s -> %s", group, path)
		}
		
		// Step 4.5: Remove list
		t.Log("Step 4.5: Removing a list...")
		err = manager.RemoveList("test")
		if err != nil {
			t.Fatalf("Failed to remove list: %v", err)
		}
		t.Log("✓ List removed successfully")
		
		allLists = manager.ListAll()
		if len(allLists) != 1 {
			t.Errorf("Expected 1 list after removal, got %d", len(allLists))
		}
		
		t.Log("✓ Domain list manager: PASS")
	})
	
	t.Run("Step5_DownloadAndCache", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping download in short mode")
		}
		
		t.Log("=== Testing Download and Cache ===")
		
		manager := NewDomainListManager(cacheDir, logger)
		
		// Use a real remote URL
		source := "https://raw.githubusercontent.com/gfwlist/gfwlist/refs/heads/master/gfwlist.txt"
		localPath := filepath.Join(cacheDir, "gfwlist.txt")
		
		t.Log("Step 5.1: Downloading and caching...")
		start := time.Now()
		
		err := manager.DownloadAndCache(source, localPath)
		
		elapsed := time.Since(start)
		t.Logf("Download and cache took: %v", elapsed)
		
		if err != nil {
			t.Fatalf("Failed to download and cache: %v", err)
		}
		
		t.Log("✓ Download and cache completed")
		
		// Step 5.2: Verify cache file exists
		t.Log("Step 5.2: Verifying cache file...")
		cachePath := manager.GetCachePath(source)
		if _, err := os.Stat(cachePath); os.IsNotExist(err) {
			t.Error("Cache file does not exist")
		}
		t.Logf("✓ Cache file exists: %s", cachePath)
		
		// Step 5.3: Verify local file exists
		t.Log("Step 5.3: Verifying local file...")
		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			t.Error("Local file does not exist")
		}
		t.Logf("✓ Local file exists: %s", localPath)
		
		// Step 5.4: Load from cache (should be fast)
		t.Log("Step 5.4: Loading from cache (should be fast)...")
		start = time.Now()
		
		loader := NewDomainFileLoader(logger)
		domains, err := loader.LoadDomains(localPath)
		
		elapsed = time.Since(start)
		t.Logf("Load from cache took: %v", elapsed)
		
		if err != nil {
			t.Fatalf("Failed to load from cache: %v", err)
		}
		
		t.Logf("✓ Loaded %d domains from cache", len(domains))
		
		if elapsed > 100*time.Millisecond {
			t.Logf("Warning: Cache load took longer than expected: %v", elapsed)
		}
		
		t.Log("✓ Download and cache: PASS")
	})
	
	t.Run("Step6_LoadDomainsWithCache", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping in short mode")
		}
		
		t.Log("=== Testing LoadDomainsWithCache ===")
		
		cache := NewDomainListCache(cacheDir, logger)
		source := "https://raw.githubusercontent.com/gfwlist/gfwlist/refs/heads/master/gfwlist.txt"
		
		// First load (should download)
		t.Log("Step 6.1: First load (should download)...")
		start := time.Now()
		
		domains1, err := LoadDomainsWithCache(source, cache, 24*time.Hour, true)
		
		elapsed1 := time.Since(start)
		t.Logf("First load took: %v", elapsed1)
		
		if err != nil {
			t.Fatalf("Failed first load: %v", err)
		}
		
		t.Logf("✓ First load: %d domains", len(domains1))
		
		// Second load (should use cache)
		t.Log("Step 6.2: Second load (should use cache)...")
		start = time.Now()
		
		domains2, err := LoadDomainsWithCache(source, cache, 24*time.Hour, true)
		
		elapsed2 := time.Since(start)
		t.Logf("Second load took: %v", elapsed2)
		
		if err != nil {
			t.Fatalf("Failed second load: %v", err)
		}
		
		t.Logf("✓ Second load: %d domains", len(domains2))
		
		// Second load should be much faster
		if elapsed2 >= elapsed1 {
			t.Logf("Warning: Second load not faster (first=%v, second=%v)", elapsed1, elapsed2)
		} else {
			speedup := float64(elapsed1) / float64(elapsed2)
			t.Logf("✓ Cache speedup: %.2fx faster", speedup)
		}
		
		// Domain count should match
		if len(domains1) != len(domains2) {
			t.Errorf("Domain count mismatch: first=%d, second=%d", len(domains1), len(domains2))
		}
		
		t.Log("✓ LoadDomainsWithCache: PASS")
	})
	
	t.Run("Step7_CacheCleanup", func(t *testing.T) {
		t.Log("=== Testing Cache Cleanup ===")
		
		cache := NewDomainListCache(cacheDir, logger)
		
		// Create some test cache files
		t.Log("Step 7.1: Creating test cache files...")
		testSources := []string{
			"https://example.com/old1.txt",
			"https://example.com/old2.txt",
			"https://example.com/new.txt",
		}
		
		for i, source := range testSources {
			content := []byte("test content")
			err := cache.SaveToCache(source, content)
			if err != nil {
				t.Fatalf("Failed to save test cache: %v", err)
			}
			
			// Make first two files old
			if i < 2 {
				cachePath := cache.GetCachePath(source)
				oldTime := time.Now().Add(-48 * time.Hour)
				os.Chtimes(cachePath, oldTime, oldTime)
			}
		}
		
		t.Logf("✓ Created %d test cache files", len(testSources))
		
		// Clean old caches (older than 24 hours)
		t.Log("Step 7.2: Cleaning old caches...")
		err := cache.CleanCache(24 * time.Hour)
		if err != nil {
			t.Fatalf("Failed to clean cache: %v", err)
		}
		
		t.Log("✓ Cache cleanup completed")
		
		// Verify old caches are removed
		t.Log("Step 7.3: Verifying cleanup...")
		for i, source := range testSources {
			exists := cache.IsCached(source, 0)
			if i < 2 {
				// Old files should be removed
				if exists {
					t.Errorf("Old cache file still exists: %s", source)
				}
			} else {
				// New file should still exist
				if !exists {
					t.Errorf("New cache file was removed: %s", source)
				}
			}
		}
		
		t.Log("✓ Cache cleanup: PASS")
	})
	
	t.Run("Step8_FormatConversion", func(t *testing.T) {
		t.Log("=== Testing Format Conversion ===")
		
		converter := NewFormatConverter(logger)
		testDomains := []string{"example.com", "test.com", "demo.org"}
		
		// Test Plain Text
		t.Log("Testing Plain Text format...")
		content := converter.toPlainText(testDomains)
		if len(content) == 0 {
			t.Error("Empty content for Plain Text format")
		}
		t.Logf("✓ Plain Text: %d bytes", len(content))
		
		// Test Clash YAML
		t.Log("Testing Clash YAML format...")
		content, err := converter.toClashYAML(testDomains)
		if err != nil {
			t.Errorf("Failed to convert to Clash YAML: %v", err)
		} else {
			t.Logf("✓ Clash YAML: %d bytes", len(content))
		}
		
		// Test Surge
		t.Log("Testing Surge format...")
		content = converter.toSurge(testDomains)
		if len(content) == 0 {
			t.Error("Empty content for Surge format")
		}
		t.Logf("✓ Surge: %d bytes", len(content))
		
		// Test Dnsmasq
		t.Log("Testing Dnsmasq format...")
		content = converter.toDnsmasq(testDomains)
		if len(content) == 0 {
			t.Error("Empty content for Dnsmasq format")
		}
		t.Logf("✓ Dnsmasq: %d bytes", len(content))
		
		// Test Hosts
		t.Log("Testing Hosts format...")
		content = converter.toHosts(testDomains)
		if len(content) == 0 {
			t.Error("Empty content for Hosts format")
		}
		t.Logf("✓ Hosts: %d bytes", len(content))
		
		// Test AdBlock
		t.Log("Testing AdBlock format...")
		content = converter.toAdblock(testDomains)
		if len(content) == 0 {
			t.Error("Empty content for AdBlock format")
		}
		t.Logf("✓ AdBlock: %d bytes", len(content))
		
		// Test JSON
		t.Log("Testing JSON format...")
		content, err = converter.toJSON(testDomains)
		if err != nil {
			t.Errorf("Failed to convert to JSON: %v", err)
		} else {
			t.Logf("✓ JSON: %d bytes", len(content))
		}
		
		t.Log("✓ Format conversion: PASS")
	})
}

// TestCompleteIntegration tests the complete integration workflow.
func TestCompleteIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	
	t.Log("=== Complete Integration Test ===")
	
	// Create temporary directory
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	
	// Step 1: Create manager
	t.Log("Step 1: Creating domain list manager...")
	manager := NewDomainListManager(cacheDir, logger)
	t.Log("✓ Manager created")
	
	// Step 2: Add multiple lists
	t.Log("Step 2: Adding multiple domain lists...")
	lists := []*ManagedList{
		{
			Name:       "gfw",
			Source:     "https://raw.githubusercontent.com/gfwlist/gfwlist/refs/heads/master/gfwlist.txt",
			LocalPath:  filepath.Join(cacheDir, "gfw.txt"),
			Group:      "overseas",
			Enabled:    true,
			AutoUpdate: true,
		},
	}
	
	for _, list := range lists {
		t.Logf("Downloading and caching: %s", list.Name)
		start := time.Now()
		
		err := manager.DownloadAndCache(list.Source, list.LocalPath)
		if err != nil {
			t.Fatalf("Failed to download %s: %v", list.Name, err)
		}
		
		elapsed := time.Since(start)
		t.Logf("✓ %s downloaded in %v", list.Name, elapsed)
		
		err = manager.AddList(list)
		if err != nil {
			t.Fatalf("Failed to add %s: %v", list.Name, err)
		}
		
		t.Logf("✓ %s added to manager", list.Name)
	}
	
	// Step 3: Build configuration
	t.Log("Step 3: Building domain groups configuration...")
	config := manager.BuildDomainGroupsConfig()
	t.Logf("✓ Configuration built with %d groups", len(config))
	
	for group, path := range config {
		t.Logf("  - %s -> %s", group, path)
	}
	
	// Step 4: Simulate auto-update
	t.Log("Step 4: Simulating auto-update...")
	allLists := manager.ListAll()
	for _, list := range allLists {
		if list.AutoUpdate && list.Enabled {
			t.Logf("Updating: %s", list.Name)
			err := manager.UpdateList(list.Name)
			if err != nil {
				t.Errorf("Failed to update %s: %v", list.Name, err)
			} else {
				t.Logf("✓ %s updated", list.Name)
			}
		}
	}
	
	// Step 5: Verify cache performance
	t.Log("Step 5: Verifying cache performance...")
	for _, list := range allLists {
		start := time.Now()
		
		loader := NewDomainFileLoader(logger)
		domains, err := loader.LoadDomains(list.LocalPath)
		
		elapsed := time.Since(start)
		
		if err != nil {
			t.Errorf("Failed to load %s: %v", list.Name, err)
			continue
		}
		
		t.Logf("✓ %s: %d domains loaded in %v", list.Name, len(domains), elapsed)
		
		if elapsed > 100*time.Millisecond {
			t.Logf("Warning: Load time > 100ms for %s", list.Name)
		}
	}
	
	t.Log("✓ Complete integration test: PASS")
}
