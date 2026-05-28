package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
)

func main() {
	fmt.Println("=== 通配符域名匹配测试 ===\n")

	// 创建域名分组
	groups := []proxy.DomainGroupConfig{
		{
			GroupName:        "wildcard-test",
			DomainFile:       "test_wildcard_domains.txt",
			DomainFileFormat: proxy.FormatPlain,
			Upstreams:        []string{"1.1.1.1"},
			SubdomainsOnly:   false,
			Enabled:          true,
		},
	}

	opts := &upstream.Options{}
	mgr, err := proxy.NewDomainGroupManager(groups, opts)
	if err != nil {
		log.Fatalf("Failed to create manager: %v", err)
	}

	fmt.Println("加载的域名规则:")
	status := mgr.GetGroups()
	for _, s := range status {
		fmt.Printf("  组名: %s\n", s.GroupName)
		fmt.Printf("  域名数量: %d\n", s.DomainCount)
		fmt.Printf("  上游数量: %d\n\n", s.UpstreamCount)
	}

	// 测试用例
	testCases := []struct {
		domain      string
		description string
	}{
		// 普通域名测试
		{"example.com", "普通域名 - 精确匹配"},
		{"www.example.com", "普通域名 - 子域名匹配"},
		{"api.www.example.com", "普通域名 - 深层子域名匹配"},
		
		// 通配符域名测试（example.com 已覆盖所有子域名）
		{"cdn.example.com", "被 example.com 覆盖"},
		{"img.cdn.example.com", "被 example.com 覆盖（也匹配 *.cdn.example.com）"},
		{"static.cdn.example.com", "被 example.com 覆盖（也匹配 *.cdn.example.com）"},
		
		// API 通配符测试（test.org 已覆盖所有子域名）
		{"api.test.org", "被 test.org 覆盖"},
		{"v1.api.test.org", "被 test.org 覆盖（也匹配 *.api.test.org）"},
		
		// GitHub 测试
		{"github.com", "GitHub - 精确匹配"},
		{"www.github.com", "GitHub - 子域名匹配"},
		{"github.io", "GitHub.io - 基础域名（不匹配）"},
		{"username.github.io", "GitHub.io - 通配符匹配"},
		{"raw.githubusercontent.com", "githubusercontent - 通配符匹配"},
		
		// 独立通配符测试（没有对应的普通域名）
		{"special.domain.com", "独立通配符 - 基础域名（不匹配）"},
		{"sub.special.domain.com", "独立通配符 - 子域名匹配"},
		
		// 不匹配的域名
		{"notinlist.com", "不在列表中的域名"},
		{"example.org", "不在列表中的域名"},
	}

	fmt.Println("域名匹配测试结果:")
	fmt.Println(strings.Repeat("-", 80))
	
	for _, tc := range testCases {
		upstreams, err := mgr.MatchDomain(tc.domain)
		if err != nil {
			log.Printf("Error matching %s: %v", tc.domain, err)
			continue
		}

		matched := len(upstreams) > 0
		status := "✗ 不匹配"
		if matched {
			status = "✓ 匹配"
		}

		fmt.Printf("%-40s %-30s %s\n", tc.domain, tc.description, status)
	}

	fmt.Println(strings.Repeat("-", 80))
	fmt.Println("\n=== 测试完成 ===")
}
