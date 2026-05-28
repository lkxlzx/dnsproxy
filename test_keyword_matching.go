package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
)

func main() {
	fmt.Println("=== 关键字匹配功能测试 ===\n")

	// 创建测试域名文件内容
	testDomains := `# 关键字匹配测试
# 拦截所有包含广告相关关键字的域名

keyword:ad
keyword:ads
keyword:adservice
keyword:tracker
keyword:analytics
keyword:telemetry

# 精确域名
example.com
test.org
`

	// 写入临时文件
	tmpFile := "test_keyword_domains.txt"
	if err := writeFile(tmpFile, testDomains); err != nil {
		log.Fatalf("Failed to write test file: %v", err)
	}
	defer removeFile(tmpFile)

	// 创建域名分组
	groups := []proxy.DomainGroupConfig{
		{
			GroupName:        "keyword-test",
			DomainFile:       tmpFile,
			DomainFileFormat: proxy.FormatPlain,
			Upstreams:        []string{"0.0.0.0"}, // 拦截地址
			SubdomainsOnly:   false,
			Enabled:          true,
		},
	}

	opts := &upstream.Options{}
	mgr, err := proxy.NewDomainGroupManager(groups, opts)
	if err != nil {
		log.Fatalf("Failed to create manager: %v", err)
	}

	fmt.Println("加载的规则:")
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
		// 关键字 "ad" 匹配
		{"ad.example.com", "关键字 ad - 子域名"},
		{"adservice.google.com", "关键字 ad - 域名开头"},
		{"my-ad-network.com", "关键字 ad - 域名中间"},
		{"download.com", "关键字 ad - 包含但不独立"},

		// 关键字 "ads" 匹配
		{"ads.facebook.com", "关键字 ads - 子域名"},
		{"www.ads-server.com", "关键字 ads - 域名中间"},

		// 关键字 "tracker" 匹配
		{"tracker.example.com", "关键字 tracker - 子域名"},
		{"my-tracker.net", "关键字 tracker - 域名中间"},

		// 关键字 "analytics" 匹配
		{"analytics.google.com", "关键字 analytics - 子域名"},
		{"www.analytics-service.com", "关键字 analytics - 域名中间"},

		// 关键字 "telemetry" 匹配
		{"telemetry.microsoft.com", "关键字 telemetry - 子域名"},

		// 精确域名匹配
		{"example.com", "精确域名 - 匹配"},
		{"www.example.com", "精确域名 - 子域名匹配"},
		{"test.org", "精确域名 - 匹配"},

		// 不匹配的域名
		{"google.com", "不匹配 - 无关键字"},
		{"www.github.com", "不匹配 - 无关键字"},
		{"normal-site.com", "不匹配 - 无关键字"},
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
			status = "✓ 匹配 (拦截)"
		}

		fmt.Printf("%-40s %-35s %s\n", tc.domain, tc.description, status)
	}

	fmt.Println(strings.Repeat("-", 80))
	fmt.Println("\n=== 测试完成 ===")
	fmt.Println("\n关键字匹配说明:")
	fmt.Println("  - keyword:ad     匹配包含 'ad' 的任何域名")
	fmt.Println("  - keyword:ads    匹配包含 'ads' 的任何域名")
	fmt.Println("  - keyword:tracker 匹配包含 'tracker' 的任何域名")
	fmt.Println("  - 关键字匹配不区分大小写")
	fmt.Println("  - 适用于广告拦截、追踪拦截等场景")
}

func writeFile(filename, content string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	return err
}

func removeFile(filename string) {
	os.Remove(filename)
}
