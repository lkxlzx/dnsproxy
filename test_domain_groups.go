package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
)

func main() {
	fmt.Println("=== 动态域名分组功能测试 ===\n")

	// 创建域名分组配置
	domainGroups := []proxy.DomainGroupConfig{
		{
			GroupName:        "china-domains",
			DomainFile:       "china_domains.txt",
			DomainFileFormat: proxy.FormatPlain,
			Upstreams:        []string{"223.5.5.5"},
			SubdomainsOnly:   false,
			Enabled:          true,
		},
		{
			GroupName:        "ad-blocking",
			DomainFile:       "ad_domains.txt",
			DomainFileFormat: proxy.FormatPlain,
			Upstreams:        []string{"0.0.0.0"},
			SubdomainsOnly:   false,
			Enabled:          true,
		},
		{
			GroupName:        "custom-routing",
			DomainFile:       "custom_domains.txt",
			DomainFileFormat: proxy.FormatPlain,
			Upstreams:        []string{"1.1.1.1"},
			SubdomainsOnly:   false,
			Enabled:          true,
		},
	}

	// 创建默认上游配置
	defaultUpstreams, err := proxy.ParseUpstreamsConfig(
		[]string{"8.8.8.8"},
		&upstream.Options{Timeout: 5 * time.Second},
	)
	if err != nil {
		log.Fatalf("Failed to parse default upstreams: %v", err)
	}

	// 创建代理配置
	config := &proxy.Config{
		UDPListenAddr:  []*net.UDPAddr{{IP: net.ParseIP("127.0.0.1"), Port: 15353}},
		UpstreamConfig: defaultUpstreams,
		DomainGroups:   domainGroups,
		CacheEnabled:   true,
		CacheSizeBytes: 10 * 1024 * 1024,
	}

	// 创建代理实例
	dnsProxy, err := proxy.New(config)
	if err != nil {
		log.Fatalf("Failed to create proxy: %v", err)
	}

	// 启动代理
	ctx := context.Background()
	if err := dnsProxy.Start(ctx); err != nil {
		log.Fatalf("Failed to start proxy: %v", err)
	}
	defer dnsProxy.Shutdown(ctx)

	fmt.Println("DNS Proxy started on 127.0.0.1:15353\n")

	// 显示初始状态
	fmt.Println("初始规则组状态:")
	printDomainGroups(dnsProxy)
	fmt.Println()

	// 等待服务启动
	time.Sleep(500 * time.Millisecond)

	// 测试1: 查询中国域名
	fmt.Println("测试1: 查询中国域名 (baidu.com)")
	testQuery("127.0.0.1:15353", "baidu.com")
	fmt.Println()

	// 测试2: 查询广告域名
	fmt.Println("测试2: 查询广告域名 (ad.doubleclick.net)")
	testQuery("127.0.0.1:15353", "ad.doubleclick.net")
	fmt.Println()

	// 测试3: 禁用广告拦截
	fmt.Println("测试3: 禁用广告拦截规则组")
	if err := dnsProxy.DisableDomainGroup("ad-blocking"); err != nil {
		log.Printf("Failed to disable group: %v", err)
	} else {
		fmt.Println("✓ 广告拦截已禁用")
		printDomainGroups(dnsProxy)
	}
	fmt.Println()

	// 测试4: 再次查询广告域名（应该使用默认上游）
	fmt.Println("测试4: 禁用后再次查询广告域名")
	testQuery("127.0.0.1:15353", "ad.doubleclick.net")
	fmt.Println()

	// 测试5: 重新启用广告拦截
	fmt.Println("测试5: 重新启用广告拦截")
	if err := dnsProxy.EnableDomainGroup("ad-blocking"); err != nil {
		log.Printf("Failed to enable group: %v", err)
	} else {
		fmt.Println("✓ 广告拦截已重新启用")
		printDomainGroups(dnsProxy)
	}
	fmt.Println()

	// 测试6: 查询自定义路由域名
	fmt.Println("测试6: 查询自定义路由域名 (github.com)")
	testQuery("127.0.0.1:15353", "github.com")
	fmt.Println()

	// 测试7: 查询未匹配的域名（使用默认上游）
	fmt.Println("测试7: 查询未匹配的域名 (example.com)")
	testQuery("127.0.0.1:15353", "example.com")
	fmt.Println()

	fmt.Println("=== 测试完成 ===")
}

func printDomainGroups(p *proxy.Proxy) {
	groups := p.GetDomainGroups()
	for _, g := range groups {
		status := "✓ 启用"
		if !g.Enabled {
			status = "✗ 禁用"
		}
		fmt.Printf("  [%s] %s: %d 个域名, %d 个上游\n",
			status, g.GroupName, g.DomainCount, g.UpstreamCount)
	}
}

func testQuery(server, domain string) {
	c := new(dns.Client)
	c.Timeout = 3 * time.Second

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(domain), dns.TypeA)

	r, rtt, err := c.Exchange(m, server)
	if err != nil {
		fmt.Printf("  ✗ 查询失败: %v\n", err)
		return
	}

	fmt.Printf("  查询耗时: %v\n", rtt)
	fmt.Printf("  响应代码: %s\n", dns.RcodeToString[r.Rcode])
	
	if len(r.Answer) > 0 {
		fmt.Printf("  应答记录:\n")
		for _, ans := range r.Answer {
			if a, ok := ans.(*dns.A); ok {
				fmt.Printf("    - %s\n", a.A)
			}
		}
	} else {
		fmt.Printf("  无应答记录\n")
	}
}
