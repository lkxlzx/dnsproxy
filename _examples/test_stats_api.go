package main

import (
	"encoding/json"
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

	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelWarn, // 减少日志输出
	}))

	// Create upstream options
	opts := &upstream.Options{
		Logger:  logger,
		Timeout: 10 * time.Second,
	}

	fmt.Println("\n=== Parsing upstream groups (this will download domain lists) ===")
	
	// Parse upstream groups (this will download and cache domain lists)
	ugc, err := proxy.ParseUpstreamGroups(spec, opts)
	if err != nil {
		log.Fatalf("Failed to parse upstream groups: %v", err)
	}

	fmt.Println("✓ Parsing completed")

	// 获取 DomainListManager（需要从 ParseUpstreamGroups 返回或存储）
	// 这里我们需要重新创建 manager 来访问统计信息
	cacheDir := "./cache"
	if spec.Cache != nil && spec.Cache.Directory != "" {
		cacheDir = spec.Cache.Directory
	}
	
	manager := proxy.NewDomainListManager(cacheDir, logger)
	
	// 重新加载列表信息（从缓存文件）
	fmt.Println("\n=== Loading domain list statistics ===")
	
	for _, listSpec := range spec.DomainLists {
		if !listSpec.Enabled {
			continue
		}

		// 检查缓存文件
		if listSpec.File != "" {
			info, err := os.Stat(listSpec.File)
			if err == nil {
				// 读取缓存文件获取域名数量
				content, err := os.ReadFile(listSpec.File)
				if err == nil {
					var cacheData struct {
						Domains []string `yaml:"domains"`
					}
					if err := yaml.Unmarshal(content, &cacheData); err == nil {
						fmt.Printf("\n📊 %s:\n", listSpec.Name)
						fmt.Printf("   Source: %s\n", listSpec.Source)
						fmt.Printf("   Group: %s\n", listSpec.Group)
						fmt.Printf("   Cache File: %s\n", listSpec.File)
						fmt.Printf("   Domain Count: %d\n", len(cacheData.Domains))
						fmt.Printf("   Last Modified: %s\n", info.ModTime().Format("2006-01-02 15:04:05"))
						fmt.Printf("   File Size: %.2f MB\n", float64(info.Size())/1024/1024)
					}
				}
			}
		}
	}

	// 如果有 ManagedList，也可以从那里获取
	fmt.Println("\n=== Domain List Manager Statistics ===")
	allLists := manager.ListAll()
	if len(allLists) > 0 {
		fmt.Printf("Managed lists: %d\n", len(allLists))
		for _, list := range allLists {
			fmt.Printf("\n📋 %s:\n", list.Name)
			fmt.Printf("   Domain Count: %d\n", list.DomainCount)
			fmt.Printf("   Last Update: %s\n", list.LastUpdate.Format("2006-01-02 15:04:05"))
			fmt.Printf("   Auto Update: %v\n", list.AutoUpdate)
			fmt.Printf("   Format: %s\n", list.Format)
		}
	} else {
		fmt.Println("(No managed lists found - they may not have been registered)")
	}

	// 生成 JSON API 响应示例
	fmt.Println("\n=== JSON API Response Example ===")
	
	apiResponse := make([]map[string]interface{}, 0)
	for _, listSpec := range spec.DomainLists {
		if !listSpec.Enabled {
			continue
		}

		stat := map[string]interface{}{
			"name":    listSpec.Name,
			"source":  listSpec.Source,
			"group":   listSpec.Group,
			"enabled": listSpec.Enabled,
			"format":  listSpec.Format,
		}

		// 从缓存文件读取统计信息
		if listSpec.File != "" {
			info, err := os.Stat(listSpec.File)
			if err == nil {
				content, err := os.ReadFile(listSpec.File)
				if err == nil {
					var cacheData struct {
						Domains []string `yaml:"domains"`
					}
					if err := yaml.Unmarshal(content, &cacheData); err == nil {
						stat["domain_count"] = len(cacheData.Domains)
						stat["last_updated"] = info.ModTime().Format(time.RFC3339)
						stat["file_size"] = info.Size()
					}
				}
			}
		}

		apiResponse = append(apiResponse, stat)
	}

	jsonData, _ := json.MarshalIndent(apiResponse, "", "  ")
	fmt.Println(string(jsonData))

	fmt.Println("\n=== Summary ===")
	fmt.Printf("Total groups: %d\n", len(ugc.Groups))
	fmt.Printf("Total domain mappings: %d\n", len(ugc.DomainGroups))
	fmt.Println("\n✅ Statistics are available from:")
	fmt.Println("   1. Cache files (read domain count from YAML)")
	fmt.Println("   2. ManagedList objects (if registered)")
	fmt.Println("   3. File system metadata (last modified time)")
}
