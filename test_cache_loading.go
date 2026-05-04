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
	// Read config file
	configPath := "test-cache-config.yaml"
	fmt.Printf("Loading config from: %s\n", configPath)

	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}

	// Parse full config
	var fullConfig map[string]interface{}
	if err := yaml.Unmarshal(data, &fullConfig); err != nil {
		log.Fatalf("Failed to parse config: %v", err)
	}

	// Extract upstream groups config
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

	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Create upstream options
	opts := &upstream.Options{
		Logger:  logger,
		Timeout: 10 * time.Second,
	}

	fmt.Println("\n=== Starting to parse upstream groups ===")
	fmt.Printf("Groups: %d\n", len(spec.Groups))
	fmt.Printf("Domain Lists: %d\n", len(spec.DomainLists))
	fmt.Printf("Cache Enabled: %v\n", spec.Cache != nil && spec.Cache.Enabled)

	// Parse upstream groups (this will download and cache domain lists)
	ugc, err := proxy.ParseUpstreamGroups(spec, opts)
	if err != nil {
		log.Fatalf("Failed to parse upstream groups: %v", err)
	}

	fmt.Println("\n=== Parsing completed successfully ===")
	fmt.Printf("Total groups: %d\n", len(ugc.Groups))
	fmt.Printf("Total domain mappings: %d\n", len(ugc.DomainGroups))
	fmt.Printf("Default group: %s\n", ugc.DefaultGroup)

	// Check cache files
	fmt.Println("\n=== Checking cache files ===")
	for _, listSpec := range spec.DomainLists {
		if listSpec.File != "" {
			info, err := os.Stat(listSpec.File)
			if err != nil {
				fmt.Printf("❌ %s: NOT FOUND\n", listSpec.File)
			} else {
				fmt.Printf("✓ %s: %d bytes, modified: %s\n", 
					listSpec.File, 
					info.Size(), 
					info.ModTime().Format("2006-01-02 15:04:05"))
				
				// Read and show first few lines
				content, err := os.ReadFile(listSpec.File)
				if err == nil {
					lines := string(content)
					if len(lines) > 300 {
						lines = lines[:300] + "..."
					}
					fmt.Printf("  Content preview:\n%s\n\n", lines)
				}
			}
		}
	}

	// Test domain resolution
	fmt.Println("\n=== Testing domain resolution ===")
	testDomains := []string{
		"baidu.com",      // Should use china group
		"google.com",     // Should use overseas group
		"example.com",    // Should use default group
	}

	for _, domain := range testDomains {
		group, err := ugc.GetGroupForDomain(domain)
		if err != nil {
			fmt.Printf("❌ %s -> error: %v\n", domain, err)
		} else if group != nil {
			fmt.Printf("✓ %s -> %s (ID: %s)\n", domain, group.Name, group.ID)
		} else {
			fmt.Printf("✓ %s -> default group\n", domain)
		}
	}

	fmt.Println("\n=== Test completed successfully ===")
}
