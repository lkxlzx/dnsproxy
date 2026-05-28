package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
)

// 演示域名列表自动更新功能
func main() {
	fmt.Println("=== DNSProxy 域名列表自动更新示例 ===\n")

	// 创建logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// 创建域名列表更新器
	updater := proxy.NewDomainListUpdater(logger)
	defer updater.Stop()

	// 配置多个域名列表源
	sources := []struct {
		GroupName      string
		FilePath       string
		URL            string
		Format         string
		UpdateInterval time.Duration
		Description    string
	}{
		{
			GroupName:      "china",
			FilePath:       "china_domains.txt",
			URL:            "https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/accelerated-domains.china.conf",
			Format:         "dnsmasq",
			UpdateInterval: 24 * time.Hour,
			Description:    "中国域名列表（每24小时更新）",
		},
		{
			GroupName:      "ads",
			FilePath:       "ad_domains.txt",
			URL:            "https://raw.githubusercontent.com/privacy-protection-tools/anti-AD/master/anti-ad-domains.txt",
			Format:         "plain",
			UpdateInterval: 12 * time.Hour,
			Description:    "广告域名列表（每12小时更新）",
		},
		{
			GroupName:      "gfwlist",
			FilePath:       "gfw_domains.txt",
			URL:            "https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt",
			Format:         "gfwlist",
			UpdateInterval: 7 * 24 * time.Hour,
			Description:    "GFWList（每7天更新）",
		},
	}

	// 添加所有源
	fmt.Println("=== 添加域名列表源 ===")
	for _, src := range sources {
		source := &proxy.DomainListSource{
			FilePath:       src.FilePath,
			URL:            src.URL,
			Format:         src.Format,
			UpdateInterval: src.UpdateInterval,
		}

		if err := updater.AddSource(src.GroupName, source); err != nil {
			log.Fatalf("添加源失败 %s: %v", src.GroupName, err)
		}

		fmt.Printf("✓ 添加源: %s\n", src.GroupName)
		fmt.Printf("  描述: %s\n", src.Description)
		fmt.Printf("  URL: %s\n", src.URL)
		fmt.Printf("  本地文件: %s\n", src.FilePath)
		fmt.Printf("  格式: %s\n", src.Format)
		fmt.Printf("  更新间隔: %v\n\n", src.UpdateInterval)
	}

	// 手动触发更新（演示）
	fmt.Println("=== 手动触发更新 ===")
	fmt.Println("注意：实际使用时会从真实URL下载，这里仅演示流程\n")

	// 演示单个分组更新
	fmt.Println("1. 更新单个分组（china）...")
	if err := updater.UpdateNow("china"); err != nil {
		fmt.Printf("   更新失败: %v\n", err)
	} else {
		fmt.Println("   ✓ 更新成功")
	}

	// 演示批量更新
	fmt.Println("\n2. 更新所有分组...")
	results := updater.UpdateAll()
	for group, err := range results {
		if err != nil {
			fmt.Printf("   %s: ✗ 失败 - %v\n", group, err)
		} else {
			fmt.Printf("   %s: ✓ 成功\n", group)
		}
	}

	// 获取更新状态
	fmt.Println("\n=== 域名列表状态 ===")
	status := updater.GetStatus()
	for groupName, stat := range status {
		fmt.Printf("\n分组: %s\n", groupName)
		fmt.Printf("  本地文件: %s\n", stat.FilePath)
		fmt.Printf("  远程URL: %s\n", stat.URL)
		fmt.Printf("  格式: %s\n", stat.Format)
		fmt.Printf("  更新间隔: %v\n", stat.UpdateInterval)
		fmt.Printf("  域名数量: %d\n", stat.DomainCount)
		
		if !stat.LastUpdate.IsZero() {
			fmt.Printf("  最后更新: %s\n", stat.LastUpdate.Format(time.RFC3339))
		} else {
			fmt.Printf("  最后更新: 从未更新\n")
		}
		
		if stat.LastChecksum != "" {
			fmt.Printf("  校验和: %s...\n", stat.LastChecksum[:16])
		}
		
		if stat.LastError != "" {
			fmt.Printf("  最后错误: %s\n", stat.LastError)
		}
	}

	// 演示自动更新（后台运行）
	fmt.Println("\n=== 自动更新演示 ===")
	fmt.Println("自动更新已在后台运行...")
	fmt.Println("- china: 每24小时自动更新")
	fmt.Println("- ads: 每12小时自动更新")
	fmt.Println("- gfwlist: 每7天自动更新")
	fmt.Println("\n按 Ctrl+C 停止程序")

	// 模拟运行一段时间
	fmt.Println("\n等待5秒后退出...")
	time.Sleep(5 * time.Second)

	fmt.Println("\n=== 示例完成 ===")
	fmt.Println("\n功能特点:")
	fmt.Println("1. 支持从远程URL自动下载域名列表")
	fmt.Println("2. 支持多种格式（plain, dnsmasq, gfwlist, clash）")
	fmt.Println("3. 自动检测内容变化（SHA256校验）")
	fmt.Println("4. 定时自动更新（可配置间隔）")
	fmt.Println("5. 原子文件写入（避免损坏）")
	fmt.Println("6. 记录更新时间和域名数量")
	fmt.Println("7. 错误处理和重试机制")
	fmt.Println("\n配置示例:")
	fmt.Println("domain-groups:")
	fmt.Println("  - name: china")
	fmt.Println("    domain-file: china_domains.txt")
	fmt.Println("    domain-file-url: https://example.com/china-list.txt")
	fmt.Println("    update-interval: 24h")
	fmt.Println("    upstream-group-id: china-dns")
}
