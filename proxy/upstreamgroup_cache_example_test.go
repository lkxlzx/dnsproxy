package proxy_test

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
)

// ExampleDomainListCache demonstrates basic cache usage.
func ExampleDomainListCache() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	
	// Create cache manager
	cache := proxy.NewDomainListCache("./cache/domains", logger)
	
	// Example URL
	source := "https://example.com/domains.txt"
	content := []byte("example.com\ntest.com\n")
	
	// Save to cache
	err := cache.SaveToCache(source, content)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	// Check if cached
	if cache.IsCached(source, 24*time.Hour) {
		fmt.Println("Domain list is cached")
	}
	
	// Load from cache
	cached, err := cache.LoadFromCache(source)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	fmt.Printf("Loaded %d bytes from cache\n", len(cached))
}

// ExampleDomainListManager demonstrates list management.
func ExampleDomainListManager() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	
	// Create manager
	manager := proxy.NewDomainListManager("./cache/domains", logger)
	
	// Add a managed list
	list := &proxy.ManagedList{
		Name:       "china",
		Source:     "https://example.com/china.txt",
		LocalPath:  "./cache/china.txt",
		Group:      "china",
		Enabled:    true,
		AutoUpdate: true,
	}
	
	err := manager.AddList(list)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	// List all managed lists
	lists := manager.ListAll()
	fmt.Printf("Managing %d lists\n", len(lists))
	
	// Build configuration for ParseUpstreamGroups
	config := manager.BuildDomainGroupsConfig()
	fmt.Printf("Generated config with %d groups\n", len(config))
}

// ExampleDomainListManager_downloadAndCache demonstrates downloading and caching.
func ExampleDomainListManager_downloadAndCache() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	
	// Create manager
	manager := proxy.NewDomainListManager("./cache/domains", logger)
	
	// Download and cache a remote list
	// This will:
	// 1. Download the file
	// 2. Detect format (Clash YAML, GFWList, etc.)
	// 3. Parse domains
	// 4. Convert to plain text
	// 5. Save to cache (SHA256 hash filename)
	// 6. Save to local path
	source := "https://raw.githubusercontent.com/gfwlist/gfwlist/refs/heads/master/gfwlist.txt"
	localPath := "./cache/gfwlist.txt"
	
	err := manager.DownloadAndCache(source, localPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	fmt.Println("Download and cache completed")
}

// ExampleLoadDomainsWithCache demonstrates loading with cache support.
func ExampleLoadDomainsWithCache() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cache := proxy.NewDomainListCache("./cache/domains", logger)
	
	source := "https://example.com/domains.txt"
	ttl := 24 * time.Hour
	fallbackToStale := true
	
	// Load domains with caching
	// This will:
	// 1. Check if cached and not expired -> use cache
	// 2. If not cached or expired -> download
	// 3. If download fails and fallbackToStale=true -> use stale cache
	domains, err := proxy.LoadDomainsWithCache(source, cache, ttl, fallbackToStale)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	fmt.Printf("Loaded %d domains\n", len(domains))
}

// ExampleDomainListCache_integration demonstrates complete integration workflow.
func ExampleDomainListCache_integration() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	
	// Step 1: Create manager
	manager := proxy.NewDomainListManager("./cache/domains", logger)
	
	// Step 2: Add lists (like AdGuard Home would do)
	lists := []*proxy.ManagedList{
		{
			Name:       "china",
			Source:     "https://example.com/china.yaml",
			LocalPath:  "./cache/china.txt",
			Group:      "china",
			Enabled:    true,
			AutoUpdate: true,
		},
		{
			Name:       "gfw",
			Source:     "https://example.com/gfwlist.txt",
			LocalPath:  "./cache/gfw.txt",
			Group:      "overseas",
			Enabled:    true,
			AutoUpdate: true,
		},
	}
	
	for _, list := range lists {
		// Download and cache each list
		err := manager.DownloadAndCache(list.Source, list.LocalPath)
		if err != nil {
			fmt.Printf("Failed to download %s: %v\n", list.Name, err)
			continue
		}
		
		// Add to manager
		err = manager.AddList(list)
		if err != nil {
			fmt.Printf("Failed to add %s: %v\n", list.Name, err)
			continue
		}
	}
	
	// Step 3: Build configuration for dnsproxy
	config := manager.BuildDomainGroupsConfig()
	
	// Step 4: Use with ParseUpstreamGroups
	// spec := &proxy.UpstreamGroupsSpec{
	//     DefaultGroup: "overseas",
	//     Groups: [...],
	//     DomainGroups: config,  // <- Use generated config
	// }
	// ugc, err := proxy.ParseUpstreamGroups(spec, opts)
	
	fmt.Printf("Configuration ready with %d domain groups\n", len(config))
}

// ExampleDomainListCache_cleanExpired demonstrates cache cleanup.
func ExampleDomainListCache_cleanExpired() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cache := proxy.NewDomainListCache("./cache/domains", logger)
	
	// Clean caches older than 7 days
	ttl := 7 * 24 * time.Hour
	err := cache.CleanCache(ttl)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	fmt.Println("Cache cleanup completed")
}

// ExampleDomainListManager_autoUpdate demonstrates auto-update workflow.
func ExampleDomainListManager_autoUpdate() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	manager := proxy.NewDomainListManager("./cache/domains", logger)
	
	// Add a list with auto-update enabled
	list := &proxy.ManagedList{
		Name:       "china",
		Source:     "https://example.com/china.txt",
		LocalPath:  "./cache/china.txt",
		Group:      "china",
		Enabled:    true,
		AutoUpdate: true,
	}
	manager.AddList(list)
	
	// Simulate auto-update (would be called by a timer)
	lists := manager.ListAll()
	for _, l := range lists {
		if l.AutoUpdate && l.Enabled {
			// Check if update is needed (e.g., based on time)
			// For this example, we always update
			err := manager.UpdateList(l.Name)
			if err != nil {
				fmt.Printf("Failed to update %s: %v\n", l.Name, err)
				continue
			}
			fmt.Printf("Updated %s\n", l.Name)
		}
	}
}
