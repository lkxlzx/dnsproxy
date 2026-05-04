package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"gopkg.in/yaml.v3"
)

// TestConfig represents the test configuration structure
type TestConfig struct {
	UpstreamGroups []proxy.UpstreamGroupSpec `yaml:"upstream_groups"`
	DomainGroups   map[string]interface{}    `yaml:"domain_groups"`
	DomainsLists   []proxy.DomainListSpec    `yaml:"domains_lists"`
	DefaultGroup   string                    `yaml:"default_group"`
	Cache          *proxy.CacheConfigSpec    `yaml:"cache"`
}

func main() {
	configPath := "test-cache-config.yaml"
	
	fmt.Println("=== Step 1: Load and parse configuration ===")
	
	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}

	// Parse config
	var testConfig TestConfig
	if err := yaml.Unmarshal(data, &testConfig); err != nil {
		log.Fatalf("Failed to parse test config: %v", err)
	}

	// Build UpstreamGroupsSpec
	spec := &proxy.UpstreamGroupsSpec{
		Groups:       testConfig.UpstreamGroups,
		DomainGroups: testConfig.DomainGroups,
		DomainLists:  testConfig.DomainsLists,
		DefaultGroup: testConfig.DefaultGroup,
		Cache:        testConfig.Cache,
	}

	// Show initial values
	fmt.Println("\n📄 Initial config values:")
	for _, list := range spec.DomainLists {
		fmt.Printf("   %s: domain_count=%d, last_updated=%q\n", 
			list.Name, list.DomainCount, list.LastUpdated)
	}

	fmt.Println("\n=== Step 2: Parse upstream groups (download if needed) ===")
	
	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))

	// Create upstream options
	opts := &upstream.Options{
		Logger:  logger,
		Timeout: 10 * time.Second,
	}

	// Parse upstream groups (this will download and cache domain lists)
	_, err = proxy.ParseUpstreamGroups(spec, opts)
	if err != nil {
		log.Fatalf("Failed to parse upstream groups: %v", err)
	}

	fmt.Println("✓ Domain lists loaded and cached")

	fmt.Println("\n=== Step 3: Collect statistics from cache files ===")
	
	// Collect statistics
	stats, err := proxy.CollectDomainListStats(spec)
	if err != nil {
		log.Fatalf("Failed to collect stats: %v", err)
	}

	fmt.Println("\n📊 Collected statistics:")
	for name, stat := range stats {
		fmt.Printf("   %s:\n", name)
		fmt.Printf("      Domain Count: %d\n", stat.DomainCount)
		fmt.Printf("      Last Updated: %s\n", stat.LastUpdated.Format("2006-01-02 15:04:05"))
	}

	fmt.Println("\n=== Step 4: Update configuration file ===")
	
	// Update config file with statistics
	if err := proxy.UpdateConfigFileStats(configPath, stats); err != nil {
		log.Fatalf("Failed to update config file: %v", err)
	}

	fmt.Println("✓ Configuration file updated")

	fmt.Println("\n=== Step 5: Verify updated configuration ===")
	
	// Re-read config to verify
	data, err = os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("Failed to re-read config file: %v", err)
	}

	var verifyConfig TestConfig
	if err := yaml.Unmarshal(data, &verifyConfig); err != nil {
		log.Fatalf("Failed to parse updated config: %v", err)
	}

	fmt.Println("\n📄 Updated config values:")
	for _, list := range verifyConfig.DomainsLists {
		fmt.Printf("   %s:\n", list.Name)
		fmt.Printf("      domain_count: %d\n", list.DomainCount)
		fmt.Printf("      last_updated: %s\n", list.LastUpdated)
	}

	fmt.Println("\n=== Success! ===")
	fmt.Println("✅ Configuration file now contains updated statistics")
	fmt.Println("✅ Frontend UI can read domain_count and last_updated from config file")
	fmt.Printf("\n💡 Check the file: %s\n", configPath)
}
