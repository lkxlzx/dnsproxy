package main

import (
	"fmt"
	"log"

	"github.com/AdguardTeam/dnsproxy/proxy"
)

// 演示上游分组管理功能
func main() {
	fmt.Println("=== DNSProxy 上游分组管理示例 ===\n")

	// 创建上游分组管理器
	mgr := proxy.NewUpstreamGroupManager()

	// 添加多个上游分组（带主备模式）
	groups := []proxy.UpstreamGroup{
		{
			ID:          "china-dns",
			Description: "中国大陆DNS服务器（带备用）",
			Upstreams: []string{
				"223.5.5.5",   // 阿里DNS（主要）
				"223.6.6.6",   // 阿里DNS备用（主要）
				"119.29.29.29", // 腾讯DNS（主要）
			},
			FallbackUpstreams: []string{
				"114.114.114.114", // 114DNS（备用）
				"180.76.76.76",    // 百度DNS（备用）
			},
		},
		{
			ID:          "global-dns",
			Description: "国际DNS服务器（带备用）",
			Upstreams: []string{
				"8.8.8.8", // Google DNS（主要）
				"8.8.4.4", // Google DNS备用（主要）
			},
			FallbackUpstreams: []string{
				"1.1.1.1", // Cloudflare DNS（备用）
				"1.0.0.1", // Cloudflare DNS备用（备用）
			},
		},
		{
			ID:          "adblock-dns",
			Description: "广告拦截DNS（带备用）",
			Upstreams: []string{
				"94.140.14.14", // AdGuard DNS（主要）
				"94.140.15.15", // AdGuard DNS备用（主要）
			},
			FallbackUpstreams: []string{
				"9.9.9.9", // Quad9（备用）
			},
		},
		{
			ID:          "privacy-dns",
			Description: "隐私保护DNS（无备用）",
			Upstreams: []string{
				"9.9.9.9",        // Quad9
				"149.112.112.112", // Quad9备用
			},
			// 不设置FallbackUpstreams
		},
	}

	// 添加所有分组
	for _, group := range groups {
		if err := mgr.AddGroup(&group); err != nil {
			log.Fatalf("添加分组失败: %v", err)
		}
		fmt.Printf("✓ 添加分组: %s (%s)\n", group.ID, group.Description)
		fmt.Printf("  上游服务器: %v\n\n", group.Upstreams)
	}

	// 列出所有分组
	fmt.Println("=== 所有上游分组 ===")
	allGroups := mgr.ListGroups()
	fmt.Printf("共有 %d 个分组: %v\n\n", len(allGroups), allGroups)

	// 获取特定分组的上游服务器
	fmt.Println("=== 获取分组上游 ===")
	testGroups := []string{"china-dns", "global-dns", "adblock-dns"}
	for _, groupID := range testGroups {
		upstreams, err := mgr.GetUpstreams(groupID)
		if err != nil {
			log.Printf("获取分组 %s 失败: %v", groupID, err)
			continue
		}
		fmt.Printf("分组 '%s' 的上游服务器:\n", groupID)
		for i, upstream := range upstreams {
			fmt.Printf("  %d. %s\n", i+1, upstream)
		}
		fmt.Println()
	}

	// 演示解析上游（使用分组ID）
	fmt.Println("=== 解析上游（使用分组ID）===")
	primaryResolved, fallbackResolved, err := mgr.ResolveUpstreams("china-dns", nil)
	if err != nil {
		log.Fatalf("解析失败: %v", err)
	}
	fmt.Printf("分组ID 'china-dns' 解析为:\n")
	fmt.Printf("  主要上游: %v\n", primaryResolved)
	fmt.Printf("  备用上游: %v\n\n", fallbackResolved)

	// 演示解析上游（使用直接地址）
	fmt.Println("=== 解析上游（使用直接地址）===")
	directUpstreams := []string{"8.8.8.8", "1.1.1.1"}
	primaryResolved, fallbackResolved, err = mgr.ResolveUpstreams("", directUpstreams)
	if err != nil {
		log.Fatalf("解析失败: %v", err)
	}
	fmt.Printf("直接地址 %v 解析为:\n", directUpstreams)
	fmt.Printf("  主要上游: %v\n", primaryResolved)
	fmt.Printf("  备用上游: %v\n\n", fallbackResolved)

	// 演示域名分组配置（使用上游分组ID）
	fmt.Println("=== 域名分组配置示例 ===")
	domainGroups := []struct {
		Name            string
		UpstreamGroupID string
		Description     string
	}{
		{
			Name:            "china",
			UpstreamGroupID: "china-dns",
			Description:     "中国域名使用国内DNS",
		},
		{
			Name:            "ads",
			UpstreamGroupID: "adblock-dns",
			Description:     "广告域名使用拦截DNS",
		},
		{
			Name:            "global",
			UpstreamGroupID: "global-dns",
			Description:     "国际域名使用国际DNS",
		},
	}

	for _, dg := range domainGroups {
		upstreams, err := mgr.GetUpstreams(dg.UpstreamGroupID)
		if err != nil {
			log.Printf("获取上游失败: %v", err)
			continue
		}
		fmt.Printf("域名分组: %s\n", dg.Name)
		fmt.Printf("  描述: %s\n", dg.Description)
		fmt.Printf("  上游分组ID: %s\n", dg.UpstreamGroupID)
		fmt.Printf("  实际上游: %v\n\n", upstreams)
	}

	// 演示更新分组
	fmt.Println("=== 更新上游分组 ===")
	updatedGroup := &proxy.UpstreamGroup{
		ID:          "china-dns",
		Description: "中国大陆DNS服务器（已更新）",
		Upstreams: []string{
			"223.5.5.5",   // 阿里DNS
			"119.29.29.29", // 腾讯DNS
			"180.76.76.76", // 百度DNS（新增）
		},
	}
	if err := mgr.AddGroup(updatedGroup); err != nil {
		log.Fatalf("更新分组失败: %v", err)
	}
	fmt.Printf("✓ 更新分组: %s\n", updatedGroup.ID)
	fmt.Printf("  新的上游服务器: %v\n\n", updatedGroup.Upstreams)

	// 演示删除分组
	fmt.Println("=== 删除上游分组 ===")
	if mgr.RemoveGroup("privacy-dns") {
		fmt.Println("✓ 成功删除分组: privacy-dns")
	} else {
		fmt.Println("✗ 删除分组失败: privacy-dns")
	}

	// 最终分组列表
	fmt.Println("\n=== 最终分组列表 ===")
	finalGroups := mgr.ListGroups()
	fmt.Printf("剩余 %d 个分组: %v\n", len(finalGroups), finalGroups)

	fmt.Println("\n=== 示例完成 ===")
	fmt.Println("\n优势:")
	fmt.Println("1. 集中管理上游DNS服务器")
	fmt.Println("2. 多个域名分组可以共享同一个上游分组")
	fmt.Println("3. 修改上游分组会自动影响所有引用它的域名分组")
	fmt.Println("4. 配置更简洁，易于维护")
	fmt.Println("5. 支持主备模式，提高可靠性")

	// 演示获取主备上游
	fmt.Println("\n=== 获取主备上游 ===")
	testGroupsWithFallback := []string{"china-dns", "global-dns", "privacy-dns"}
	for _, groupID := range testGroupsWithFallback {
		primary, fallback, err := mgr.GetAllUpstreams(groupID)
		if err != nil {
			log.Printf("获取分组 %s 失败: %v", groupID, err)
			continue
		}
		fmt.Printf("分组 '%s':\n", groupID)
		fmt.Printf("  主要上游: %v\n", primary)
		if len(fallback) > 0 {
			fmt.Printf("  备用上游: %v (仅在主要全部失败时使用)\n", fallback)
		} else {
			fmt.Printf("  备用上游: 无\n")
		}
		fmt.Println()
	}

	fmt.Println("\n=== 主备模式说明 ===")
	fmt.Println("主备模式工作原理:")
	fmt.Println("1. 优先使用主要上游服务器（upstreams）")
	fmt.Println("2. 只有当所有主要上游都失败时，才使用备用上游（fallback-upstreams）")
	fmt.Println("3. 这样可以确保在主要DNS服务器出现问题时，仍然有备用方案")
	fmt.Println("4. 备用上游不会参与负载均衡，只作为故障转移使用")
}
