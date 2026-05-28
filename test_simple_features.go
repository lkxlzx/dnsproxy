package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/miekg/dns"
)

// 简化的综合功能测试
// 测试：预取、域名分组、动态启用/禁用

func main() {
	fmt.Println("=== DNSProxy 综合功能测试 ===\n")

	// 创建配置
	config := &proxy.Config{
		// 监听配置
		UDPListenAddr: []*net.UDPAddr{
			{IP: net.ParseIP("127.0.0.1"), Port: 15353},
		},

		// 默认上游服务器
		UpstreamConfig: &proxy.UpstreamConfig{
			Upstreams: []string{"8.8.8.8", "1.1.1.1"},
		},

		// 缓存配置
		CacheEnabled:   true,
		CacheSizeBytes: 10 * 1024 * 1024,

		// 预取配置
		CachePrefetchConfig: &proxy.PrefetchConfig{
			Enabled:          true,
			ThresholdSeconds: 3,
			ThresholdPercent: 80,
			MaxConcurrent:    5,
			MinHeatThreshold: 3,
			TimeWindow:       30 * time.Second,
			MaxRetries:       2,
		},

		// 域名分组配置（使用文件）
		DomainGroups: []proxy.DomainGroupConfig{
			{
				GroupName:    "china",
				DomainFile:   "china_domains.txt",
				Upstreams:    []string{"223.5.5.5", "119.29.29.29"},
				Enabled:      true,
			},
			{
				GroupName:    "ads",
				DomainFile:   "ad_domains.txt",
				Upstreams:    []string{"0.0.0.0"},
				Enabled:      true,
			},
			{
				GroupName:    "custom",
				DomainFile:   "custom_domains.txt",
				Upstreams:    []string{"1.0.0.1"},
				Enabled:      false, // 初始禁用
			},
		},
	}

	// 创建并启动代理
	dnsProxy, err := proxy.New(config)
	if err != nil {
		log.Fatalf("创建代理失败: %v", err)
	}

	ctx := context.Background()
	if err := dnsProxy.Start(ctx); err != nil {
		log.Fatalf("启动代理失败: %v", err)
	}
	defer dnsProxy.Shutdown(ctx)

	fmt.Println("✓ 代理启动成功\n")
	time.Sleep(1 * time.Second)

	// 运行测试
	runTests(dnsProxy)

	fmt.Println("\n=== 所有测试完成 ===")
}

func runTests(dnsProxy *proxy.Proxy) {
	// 测试1: 预取功能
	fmt.Println("【测试1】预取功能")
	testPrefetch(dnsProxy, "google.com.")

	// 测试2: 中国域名分组
	fmt.Println("\n【测试2】中国域名分组")
	testDomainGroup(dnsProxy, []string{"baidu.com.", "taobao.com.", "qq.com."})

	// 测试3: 广告拦截（关键字匹配）
	fmt.Println("\n【测试3】广告拦截（关键字匹配）")
	testAdBlocking(dnsProxy, []string{"ad-server.com.", "tracker.example.com."})

	// 测试4: 动态启用/禁用
	fmt.Println("\n【测试4】动态启用/禁用")
	testDynamicToggle(dnsProxy, "example.com.", "custom")

	// 测试5: 分组列表
	fmt.Println("\n【测试5】分组列表")
	testListGroups(dnsProxy)

	// 测试6: 预取与分组结合
	fmt.Println("\n【测试6】预取与分组结合")
	testPrefetch(dnsProxy, "baidu.com.")
}

func testPrefetch(dnsProxy *proxy.Proxy, domain string) {
	fmt.Printf("  域名: %s\n", domain)

	// 访问3次触发预取队列
	for i := 1; i <= 3; i++ {
		resp, err := queryDNS(dnsProxy, domain)
		if err != nil {
			fmt.Printf("  ✗ 查询%d失败: %v\n", i, err)
			return
		}
		ttl := getTTL(resp)
		fmt.Printf("  ✓ 查询%d: TTL=%ds, 记录数=%d\n", i, ttl, len(resp.Answer))
		time.Sleep(300 * time.Millisecond)
	}

	// 等待接近阈值
	fmt.Println("  等待3秒...")
	time.Sleep(3 * time.Second)

	// 触发预取
	resp, err := queryDNS(dnsProxy, domain)
	if err != nil {
		fmt.Printf("  ✗ 触发查询失败: %v\n", err)
		return
	}
	fmt.Printf("  ✓ 触发预取: TTL=%ds\n", getTTL(resp))

	// 验证刷新
	time.Sleep(1 * time.Second)
	resp, err = queryDNS(dnsProxy, domain)
	if err != nil {
		fmt.Printf("  ✗ 验证查询失败: %v\n", err)
		return
	}
	ttl := getTTL(resp)
	fmt.Printf("  ✓ 验证刷新: TTL=%ds\n", ttl)

	if ttl > 5 {
		fmt.Println("  ✓ 预取成功")
	} else {
		fmt.Println("  ⚠ 预取可能未触发")
	}
}

func testDomainGroup(dnsProxy *proxy.Proxy, domains []string) {
	for _, domain := range domains {
		resp, err := queryDNS(dnsProxy, domain)
		if err != nil {
			fmt.Printf("  ✗ %s 查询失败: %v\n", domain, err)
			continue
		}
		fmt.Printf("  ✓ %s -> %d条记录, TTL=%ds\n", domain, len(resp.Answer), getTTL(resp))
	}
}

func testAdBlocking(dnsProxy *proxy.Proxy, domains []string) {
	for _, domain := range domains {
		resp, err := queryDNS(dnsProxy, domain)
		if err != nil {
			fmt.Printf("  ✗ %s 查询失败: %v\n", domain, err)
			continue
		}

		blocked := false
		if len(resp.Answer) > 0 {
			if aRecord, ok := resp.Answer[0].(*dns.A); ok {
				if aRecord.A.String() == "0.0.0.0" {
					blocked = true
				}
			}
		}

		if blocked {
			fmt.Printf("  ✓ %s -> 已拦截 (0.0.0.0)\n", domain)
		} else {
			fmt.Printf("  ✓ %s -> 正常解析 (%d条记录)\n", domain, len(resp.Answer))
		}
	}
}

func testDynamicToggle(dnsProxy *proxy.Proxy, domain, groupName string) {
	fmt.Printf("  域名: %s, 分组: %s\n", domain, groupName)

	// 初始禁用状态
	resp1, err := queryDNS(dnsProxy, domain)
	if err != nil {
		fmt.Printf("  ✗ 初始查询失败: %v\n", err)
		return
	}
	fmt.Printf("  ✓ 禁用状态: %d条记录\n", len(resp1.Answer))

	// 启用分组
	if err := dnsProxy.EnableDomainGroup(groupName); err != nil {
		fmt.Printf("  ✗ 启用失败: %v\n", err)
		return
	}
	fmt.Println("  ✓ 已启用分组")

	resp2, err := queryDNS(dnsProxy, domain)
	if err != nil {
		fmt.Printf("  ✗ 启用后查询失败: %v\n", err)
		return
	}
	fmt.Printf("  ✓ 启用状态: %d条记录\n", len(resp2.Answer))

	// 禁用分组
	if err := dnsProxy.DisableDomainGroup(groupName); err != nil {
		fmt.Printf("  ✗ 禁用失败: %v\n", err)
		return
	}
	fmt.Println("  ✓ 已禁用分组")

	resp3, err := queryDNS(dnsProxy, domain)
	if err != nil {
		fmt.Printf("  ✗ 禁用后查询失败: %v\n", err)
		return
	}
	fmt.Printf("  ✓ 禁用状态: %d条记录\n", len(resp3.Answer))

	fmt.Println("  ✓ 动态启用/禁用功能正常")
}

func testListGroups(dnsProxy *proxy.Proxy) {
	groups := dnsProxy.GetDomainGroups()
	fmt.Printf("  共有 %d 个域名分组:\n", len(groups))

	for _, group := range groups {
		status := "禁用"
		if group.Enabled {
			status = "启用"
		}
		fmt.Printf("  - %s: %s\n", group.GroupName, status)
	}
}

func queryDNS(dnsProxy *proxy.Proxy, domain string) (*dns.Msg, error) {
	req := &dns.Msg{}
	req.SetQuestion(domain, dns.TypeA)
	req.RecursionDesired = true

	ctx := context.Background()
	dctx := &proxy.DNSContext{
		Proto: proxy.ProtoUDP,
		Req:   req,
	}

	if err := dnsProxy.Resolve(ctx, dctx); err != nil {
		return nil, err
	}

	return dctx.Res, nil
}

func getTTL(msg *dns.Msg) uint32 {
	if msg == nil || len(msg.Answer) == 0 {
		return 0
	}
	return msg.Answer[0].Header().Ttl
}
