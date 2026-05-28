//go:build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
)

func main() {
	fmt.Println("╔════════════════════════════════════════════════════════════════╗")
	fmt.Println("║          DNS上游分组 - 从文件加载域名示例                        ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// 方法1: 使用简化版本
	fmt.Println("📋 方法1: 简化版本 - 单个文件")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	example1()
	fmt.Println()

	// 方法2: 使用完整版本 - 多个分组
	fmt.Println("📋 方法2: 完整版本 - 多个分组")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	example2()
}

// example1 简化版本示例
func example1() {
	upstreamConfig, err := proxy.LoadUpstreamConfigFromFileSimple(
		"example_domain_groups.txt",           // 域名列表文件
		[]string{"223.5.5.5:53", "119.29.29.29:53"}, // 这些域名使用的上游
		[]string{"8.8.8.8:53", "1.1.1.1:53"},  // 默认上游
		&upstream.Options{
			Timeout: 5 * time.Second,
		},
	)

	if err != nil {
		log.Fatalf("❌ 加载配置失败: %v", err)
	}

	fmt.Println("✅ 配置加载成功")
	fmt.Printf("   • 默认上游: %d 个\n", len(upstreamConfig.Upstreams))
	fmt.Printf("   • 域名专用上游: %d 个域名\n", len(upstreamConfig.DomainReservedUpstreams))

	// 创建代理并测试
	testProxy(upstreamConfig)
}

// example2 完整版本示例 - 多个分组
func example2() {
	groups := []proxy.DomainGroupConfig{
		{
			GroupName:      "中国域名",
			DomainFile:     "example_domain_groups.txt",
			Upstreams:      []string{"223.5.5.5:53", "119.29.29.29:53"},
			SubdomainsOnly: false, // 包括域名本身和子域名
		},
		// 可以添加更多分组
		// {
		//     GroupName:      "公司内网",
		//     DomainFile:     "company_domains.txt",
		//     Upstreams:      []string{"192.168.1.1:53"},
		//     SubdomainsOnly: false,
		// },
	}

	upstreamConfig, err := proxy.LoadUpstreamConfigFromFiles(
		groups,
		[]string{"8.8.8.8:53", "1.1.1.1:53"}, // 默认上游
		&upstream.Options{
			Timeout: 5 * time.Second,
		},
	)

	if err != nil {
		log.Fatalf("❌ 加载配置失败: %v", err)
	}

	fmt.Println("✅ 配置加载成功")
	fmt.Printf("   • 分组数量: %d\n", len(groups))
	fmt.Printf("   • 默认上游: %d 个\n", len(upstreamConfig.Upstreams))
	fmt.Printf("   • 域名专用上游: %d 个域名\n", len(upstreamConfig.DomainReservedUpstreams))

	// 创建代理并测试
	testProxy(upstreamConfig)
}

// testProxy 测试代理配置
func testProxy(upstreamConfig *proxy.UpstreamConfig) {
	config := &proxy.Config{
		UpstreamConfig: upstreamConfig,
		CacheEnabled:   true,
		CacheSizeBytes: 1024 * 1024,
	}

	dnsProxy, err := proxy.New(config)
	if err != nil {
		log.Fatalf("❌ 创建代理失败: %v", err)
	}
	defer upstreamConfig.Close()

	fmt.Println()
	fmt.Println("🔍 测试域名解析:")

	testDomains := []string{
		"baidu.com",      // 应该使用中国DNS
		"google.com",     // 应该使用默认DNS
		"taobao.com",     // 应该使用中国DNS
		"github.com",     // 应该使用默认DNS
	}

	ctx := context.Background()
	for _, domain := range testDomains {
		req := &dns.Msg{}
		req.SetQuestion(dns.Fqdn(domain), dns.TypeA)
		req.RecursionDesired = true

		dctx := &proxy.DNSContext{Req: req}
		err := dnsProxy.Resolve(ctx, dctx)

		if err != nil {
			fmt.Printf("   ❌ %s: 解析失败 - %v\n", domain, err)
			continue
		}

		if dctx.Res != nil && len(dctx.Res.Answer) > 0 {
			upstreamAddr := "缓存"
			if dctx.Upstream != nil {
				upstreamAddr = dctx.Upstream.Address()
			}
			fmt.Printf("   ✅ %s → %s (上游: %s)\n",
				domain,
				dctx.Res.Answer[0].String(),
				upstreamAddr)
		}
	}
}
