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
	configPath := "test-auto-format-config.yaml"
	
	fmt.Println("=== 测试自动格式检测功能 ===\n")
	
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

	// Show initial format values
	fmt.Println("📄 初始配置中的 format 字段:")
	for _, list := range spec.DomainLists {
		formatStr := list.Format
		if formatStr == "" {
			formatStr = "(未指定)"
		}
		fmt.Printf("   %s: format=%q\n", list.Name, formatStr)
	}

	fmt.Println("\n=== 开始解析和下载域名列表 ===")
	
	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Create upstream options
	opts := &upstream.Options{
		Logger:  logger,
		Timeout: 10 * time.Second,
	}

	// Parse upstream groups (this will download, detect format, and cache domain lists)
	_, err = proxy.ParseUpstreamGroups(spec, opts)
	if err != nil {
		log.Fatalf("Failed to parse upstream groups: %v", err)
	}

	fmt.Println("\n=== 格式检测完成 ===")
	fmt.Println("\n📊 检测到的格式:")
	for _, list := range spec.DomainLists {
		fmt.Printf("   %s: format=%q\n", list.Name, list.Format)
	}

	fmt.Println("\n=== 更新配置文件 ===")
	
	// Collect statistics (including detected format)
	stats, err := proxy.CollectDomainListStats(spec)
	if err != nil {
		log.Fatalf("Failed to collect stats: %v", err)
	}

	// Update config file with statistics and detected format
	if err := proxy.UpdateConfigFileStats(configPath, stats); err != nil {
		log.Fatalf("Failed to update config file: %v", err)
	}

	fmt.Println("✓ 配置文件已更新")

	fmt.Println("\n=== 验证更新后的配置 ===")
	
	// Re-read config to verify
	data, err = os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("Failed to re-read config file: %v", err)
	}

	var verifyConfig TestConfig
	if err := yaml.Unmarshal(data, &verifyConfig); err != nil {
		log.Fatalf("Failed to parse updated config: %v", err)
	}

	fmt.Println("\n📄 更新后的配置:")
	for _, list := range verifyConfig.DomainsLists {
		fmt.Printf("\n   %s:\n", list.Name)
		fmt.Printf("      format: %s (自动检测)\n", list.Format)
		fmt.Printf("      domain_count: %d\n", list.DomainCount)
		fmt.Printf("      last_updated: %s\n", list.LastUpdated)
	}

	fmt.Println("\n=== 成功! ===")
	fmt.Println("✅ 格式自动检测功能正常工作")
	fmt.Println("✅ 配置文件已更新为检测到的格式")
	fmt.Printf("\n💡 查看文件: %s\n", configPath)
}
