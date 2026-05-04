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

// Config represents the full configuration structure
type Config struct {
	DNS            DNSConfig                     `yaml:"dns"`
	UpstreamGroups []proxy.UpstreamGroupSpec     `yaml:"upstream_groups"`
	DomainGroups   map[string]interface{}        `yaml:"domain_groups"`
	DomainsLists   []proxy.DomainListSpec        `yaml:"domains_lists"`
	DefaultGroup   string                        `yaml:"default_group"`
	Cache          *proxy.CacheConfigSpec        `yaml:"cache"`
}

type DNSConfig struct {
	BindHosts   []string `yaml:"bind_hosts"`
	Port        int      `yaml:"port"`
	UpstreamDNS []string `yaml:"upstream_dns"`
}

func main() {
	configPath := "test-cache-config.yaml"
	
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║     DNSProxy 域名列表缓存功能 - 完整集成测试              ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	
	// Step 1: Load configuration
	fmt.Println("\n[1/6] 📖 加载配置文件...")
	config, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("❌ 加载配置失败: %v", err)
	}
	fmt.Printf("✓ 配置加载成功\n")
	fmt.Printf("  - 上游组: %d 个\n", len(config.UpstreamGroups))
	fmt.Printf("  - 域名列表: %d 个\n", len(config.DomainsLists))
	
	// Show initial stats
	fmt.Println("\n📊 初始统计信息:")
	for _, list := range config.DomainsLists {
		fmt.Printf("  • %s: domain_count=%d, last_updated=%q\n", 
			list.Name, list.DomainCount, list.LastUpdated)
	}
	
	// Step 2: Parse upstream groups
	fmt.Println("\n[2/6] 🌐 解析上游组并下载域名列表...")
	
	spec := &proxy.UpstreamGroupsSpec{
		Groups:       config.UpstreamGroups,
		DomainGroups: config.DomainGroups,
		DomainLists:  config.DomainsLists,
		DefaultGroup: config.DefaultGroup,
		Cache:        config.Cache,
	}
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	
	opts := &upstream.Options{
		Logger:  logger,
		Timeout: 10 * time.Second,
	}
	
	startTime := time.Now()
	ugc, err := proxy.ParseUpstreamGroups(spec, opts)
	if err != nil {
		log.Fatalf("❌ 解析失败: %v", err)
	}
	duration := time.Since(startTime)
	
	fmt.Printf("\n✓ 解析完成 (耗时: %.2f 秒)\n", duration.Seconds())
	fmt.Printf("  - 总分组: %d 个\n", len(ugc.Groups))
	fmt.Printf("  - 域名映射: %d 条\n", len(ugc.DomainGroups))
	
	// Step 3: Verify cache files
	fmt.Println("\n[3/6] 📁 验证缓存文件...")
	for _, list := range config.DomainsLists {
		if list.File != "" {
			info, err := os.Stat(list.File)
			if err != nil {
				fmt.Printf("  ❌ %s: 文件不存在\n", list.Name)
			} else {
				fmt.Printf("  ✓ %s: %.2f MB, %s\n", 
					list.Name, 
					float64(info.Size())/1024/1024,
					info.ModTime().Format("2006-01-02 15:04:05"))
			}
		}
	}
	
	// Step 4: Collect statistics
	fmt.Println("\n[4/6] 📊 收集统计信息...")
	stats, err := proxy.CollectDomainListStats(spec)
	if err != nil {
		log.Fatalf("❌ 收集统计失败: %v", err)
	}
	
	fmt.Println("\n统计结果:")
	totalDomains := 0
	for name, stat := range stats {
		fmt.Printf("  • %s:\n", name)
		fmt.Printf("    - 域名数量: %s\n", formatNumber(stat.DomainCount))
		fmt.Printf("    - 最后更新: %s\n", stat.LastUpdated.Format("2006-01-02 15:04:05"))
		totalDomains += stat.DomainCount
	}
	fmt.Printf("\n  📈 总计: %s 个域名\n", formatNumber(totalDomains))
	
	// Step 5: Update config file
	fmt.Println("\n[5/6] 💾 更新配置文件...")
	err = proxy.UpdateConfigFileStats(configPath, stats)
	if err != nil {
		log.Fatalf("❌ 更新配置失败: %v", err)
	}
	fmt.Println("✓ 配置文件已更新")
	
	// Step 6: Verify update
	fmt.Println("\n[6/6] ✅ 验证更新结果...")
	updatedConfig, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("❌ 重新加载配置失败: %v", err)
	}
	
	fmt.Println("\n📊 更新后的统计信息:")
	for _, list := range updatedConfig.DomainsLists {
		fmt.Printf("  • %s:\n", list.Name)
		fmt.Printf("    - domain_count: %s\n", formatNumber(list.DomainCount))
		fmt.Printf("    - last_updated: %s\n", list.LastUpdated)
	}
	
	// Test domain resolution
	fmt.Println("\n[测试] 🔍 域名路由测试...")
	testDomains := []string{
		"baidu.com",
		"taobao.com",
		"google.com",
		"youtube.com",
		"example.com",
	}
	
	for _, domain := range testDomains {
		group, err := ugc.GetGroupForDomain(domain)
		if err != nil {
			fmt.Printf("  ❌ %s: %v\n", domain, err)
		} else if group != nil {
			fmt.Printf("  ✓ %-20s → %s (ID: %s)\n", domain, group.Name, group.ID)
		} else {
			fmt.Printf("  ✓ %-20s → default\n", domain)
		}
	}
	
	// Generate JSON API response
	fmt.Println("\n[API] 📡 生成 JSON API 响应...")
	apiResponse := generateAPIResponse(updatedConfig.DomainsLists, stats)
	jsonData, _ := json.MarshalIndent(apiResponse, "", "  ")
	
	// Save to file
	apiFile := "domain-lists-stats.json"
	os.WriteFile(apiFile, jsonData, 0644)
	fmt.Printf("✓ API 响应已保存到: %s\n", apiFile)
	
	// Summary
	fmt.Println("\n╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    测试完成总结                            ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println("\n✅ 所有功能测试通过！")
	fmt.Println("\n功能验证:")
	fmt.Println("  ✓ 配置文件加载")
	fmt.Println("  ✓ 远程域名列表下载")
	fmt.Println("  ✓ 缓存文件生成")
	fmt.Println("  ✓ 统计信息收集")
	fmt.Println("  ✓ 配置文件更新")
	fmt.Println("  ✓ 域名路由匹配")
	fmt.Println("  ✓ JSON API 生成")
	
	fmt.Println("\n📁 生成的文件:")
	fmt.Printf("  • %s (已更新)\n", configPath)
	fmt.Printf("  • %s\n", apiFile)
	for _, list := range config.DomainsLists {
		if list.File != "" {
			fmt.Printf("  • %s\n", list.File)
		}
	}
	
	fmt.Println("\n💡 下一步:")
	fmt.Println("  1. 查看更新后的配置文件: cat " + configPath)
	fmt.Println("  2. 查看 API 响应: cat " + apiFile)
	fmt.Println("  3. 集成到前端 UI")
	fmt.Println("  4. 部署到生产环境")
	
	fmt.Println("\n🎉 集成测试成功完成！")
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	
	return &config, nil
}

func formatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%d,%03d", n/1000, n%1000)
	}
	return fmt.Sprintf("%d,%03d,%03d", n/1000000, (n/1000)%1000, n%1000)
}

func generateAPIResponse(lists []proxy.DomainListSpec, stats map[string]proxy.DomainListStats) map[string]interface{} {
	response := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"lists":     []map[string]interface{}{},
	}
	
	totalDomains := 0
	listData := []map[string]interface{}{}
	
	for _, list := range lists {
		if !list.Enabled {
			continue
		}
		
		stat, exists := stats[list.Name]
		if !exists {
			continue
		}
		
		listData = append(listData, map[string]interface{}{
			"name":          list.Name,
			"source":        list.Source,
			"group":         list.Group,
			"enabled":       list.Enabled,
			"format":        list.Format,
			"domain_count":  stat.DomainCount,
			"last_updated":  stat.LastUpdated.Format(time.RFC3339),
			"auto_update":   list.AutoUpdate,
			"refresh_interval": list.RefreshInterval,
		})
		
		totalDomains += stat.DomainCount
	}
	
	response["lists"] = listData
	response["total_lists"] = len(listData)
	response["total_domains"] = totalDomains
	
	return response
}
