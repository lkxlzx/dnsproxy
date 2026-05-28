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
	fmt.Println("║          DNS上游分组 - YAML配置文件示例                         ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// 方法1: 纯YAML配置（域名直接写在YAML中）
	fmt.Println("📋 方法1: 纯YAML配置")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	example1()
	fmt.Println()

	// 方法2: YAML + 外部域名文件
	fmt.Println("📋 方法2: YAML + 外部域名文件")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	example2()
}

// example1 纯YAML配置示例
func example1() {
	upstreamConfig, err := proxy.LoadUpstreamConfigFromYAML(
		"upstream_config.yaml",
		&upstream.Options{
			Timeout: 5 * time.Second,
		},
	)

	if err != nil {
		log.Fatalf("❌ 加载配置失败: %v", err)
	}

	fmt.Println("✅ 配置加载成功")
	fmt.Printf("   • 配置文件: upstream_config.yaml\n")
	fmt.Printf("   • 默认上游: %d 个\n", len(upstreamConfig.Upstreams))
	fmt.Printf("   • 域名专用上游: %d 个域名\n", len(upstreamConfig.DomainReservedUpstreams))

	// 创建代理并测试
	testProxy(upstreamConfig)
}

// example2 YAML + 外部域名文件示例
func example2() {
	upstreamConfig, err := proxy.LoadUpstreamConfigFromYAMLWithFiles(
		"upstream_config_with_files.yaml",
		&upstream.Options{
			Timeout: 5 * time.Second,
		},
	)

	if err != nil {
		log.Fatalf("❌ 加载配置失败: %v", err)
	}

	fmt.Println("✅ 配置加载成功")
	fmt.Printf("   • 配置文件: upstream_config_with_files.yaml\n")
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
		"baidu.com",   // 应该使用中国DNS
		"google.com",  // 应该使用默认DNS
		"taobao.com",  // 应该使用中国DNS
		"github.com",  // 应该使用默认DNS
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
