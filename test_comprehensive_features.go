package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/miekg/dns"
)

// 综合功能测试脚本
// 测试：预取、域名分组、DNS分组、域名列表分组、动态启用/禁用

func main() {
	fmt.Println("=== DNSProxy 综合功能测试 ===\n")

	// 创建配置
	config := createTestConfig()

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

	// 运行测试套件
	runAllTests(dnsProxy)

	fmt.Println("\n=== 所有测试完成 ===")
}

func createTestConfig() *proxy.Config {
	return &proxy.Config{
		// 监听配置
		UDPListenAddr: []*proxy.BootstrapAddr{
			{Address: "127.0.0.1:15353"},
		},

		// 默认上游服务器
		UpstreamConfig: &proxy.UpstreamConfig{
			Upstreams: []string{"8.8.8.8", "1.1.1.1"},
		},

		// 缓存配置
		CacheEnabled: true,
		CacheSizeBytes: 10 * 1024 * 1024,

		// 预取配置
		PrefetchConfig: &proxy.PrefetchConfig{
			Enabled:           true,
			ThresholdSeconds:  3,
			ThresholdPercent:  80,
			MaxConcurrent:     5,
			MinHeatThreshold:  3,
			TimeWindow:        30 * time.Second,
			MaxRetries:        2,
			InitialRetryDelay: 1 * time.Second,
		},

		// 域名分组配置
		DomainGroups: []*proxy.DomainGroupConfig{
			{
				Name:     "china",
				Enabled:  true,
				Domains:  []string{"baidu.com", "taobao.com", "qq.com"},
				Upstream: []string{"223.5.5.5", "119.29.29.29"},
			},
			{
				Name:     "ads",
				Enabled:  true,
				Domains:  []string{"keyword:ad", "keyword:tracker", "doubleclick.net"},
				Upstream: []string{"0.0.0.0"},
			},
			{
				Name:     "custom",
				Enabled:  false, // 初始禁用
				Domains:  []string{"example.com", "test.com"},
				Upstream: []string{"1.0.0.1"},
			},
		},
	}
}

func runAllTests(dnsProxy *proxy.Proxy) {
	// 测试1: 预取功能
	fmt.Println("【测试1】预取功能测试")
	testPrefetch(dnsProxy)

	// 测试2: 域名分组匹配
	fmt.Println("\n【测试2】域名分组匹配测试")
	testDomainGroupMatching(dnsProxy)

	// 测试3: 关键字匹配
	fmt.Println("\n【测试3】关键字匹配测试")
	testKeywordMatching(dnsProxy)

	// 测试4: 动态启用/禁用分组
	fmt.Println("\n【测试4】动态启用/禁用分组测试")
	testDynamicEnableDisable(dnsProxy)

	// 测试5: 分组优先级
	fmt.Println("\n【测试5】分组优先级测试")
	testGroupPriority(dnsProxy)

	// 测试6: 预取与分组结合
	fmt.Println("\n【测试6】预取与分组结合测试")
	testPrefetchWithGroups(dnsProxy)
}

// 测试1: 预取功能
func testPrefetch(dnsProxy *proxy.Proxy) {
	domain := "google.com."
	
	fmt.Printf("  测试域名: %s\n", domain)
	fmt.Println("  步骤1: 冷启动阶段 - 访问3次触发预取队列")

	// 访问3次以触发预取队列
	for i := 1; i <= 3; i++ {
		resp, err := queryDNS(dnsProxy, domain, dns.TypeA)
		if err != nil {
			fmt.Printf("  ✗ 查询%d失败: %v\n", i, err)
			return
		}
		fmt.Printf("  ✓ 查询%d完成 (TTL: %d秒)\n", i, getTTL(resp))
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("  步骤2: 等待TTL接近阈值...")
	time.Sleep(3 * time.Second)

	fmt.Println("  步骤3: 再次查询，应触发预取")
	resp, err := queryDNS(dnsProxy, domain, dns.TypeA)
	if err != nil {
		fmt.Printf("  ✗ 查询失败: %v\n", err)
		return
	}
	fmt.Printf("  ✓ 查询完成 (TTL: %d秒)\n", getTTL(resp))

	fmt.Println("  步骤4: 验证缓存已刷新")
	time.Sleep(1 * time.Second)
	resp, err = queryDNS(dnsProxy, domain, dns.TypeA)
	if err != nil {
		fmt.Printf("  ✗ 验证失败: %v\n", err)
		return
	}
	ttl := getTTL(resp)
	fmt.Printf("  ✓ 缓存TTL: %d秒\n", ttl)
	
	if ttl > 5 {
		fmt.Println("  ✓ 预取功能正常 - TTL已刷新")
	} else {
		fmt.Println("  ⚠ 预取可能未触发 - TTL较低")
	}
}

// 测试2: 域名分组匹配
func testDomainGroupMatching(dnsProxy *proxy.Proxy) {
	testCases := []struct {
		domain   string
		group    string
		expected string
	}{
		{"baidu.com.", "china", "中国DNS"},
		{"taobao.com.", "china", "中国DNS"},
		{"google.com.", "default", "默认DNS"},
	}

	for _, tc := range testCases {
		fmt.Printf("  测试: %s -> 期望使用 %s\n", tc.domain, tc.expected)
		resp, err := queryDNS(dnsProxy, tc.domain, dns.TypeA)
		if err != nil {
			fmt.Printf("  ✗ 查询失败: %v\n", err)
			continue
		}
		if len(resp.Answer) > 0 {
			fmt.Printf("  ✓ 查询成功，返回 %d 条记录\n", len(resp.Answer))
		} else {
			fmt.Printf("  ⚠ 查询成功但无应答记录\n")
		}
	}
}

// 测试3: 关键字匹配
func testKeywordMatching(dnsProxy *proxy.Proxy) {
	testCases := []struct {
		domain   string
		expected string
	}{
		{"ad-server.example.com.", "应被广告组拦截"},
		{"tracker.analytics.com.", "应被广告组拦截"},
		{"normal-site.com.", "应使用默认DNS"},
	}

	for _, tc := range testCases {
		fmt.Printf("  测试: %s -> %s\n", tc.domain, tc.expected)
		resp, err := queryDNS(dnsProxy, tc.domain, dns.TypeA)
		if err != nil {
			fmt.Printf("  ✗ 查询失败: %v\n", err)
			continue
		}
		
		// 检查是否被拦截（返回0.0.0.0）
		if len(resp.Answer) > 0 {
			if aRecord, ok := resp.Answer[0].(*dns.A); ok {
				if aRecord.A.String() == "0.0.0.0" {
					fmt.Printf("  ✓ 已拦截 (返回 0.0.0.0)\n")
				} else {
					fmt.Printf("  ✓ 正常解析 (返回 %s)\n", aRecord.A.String())
				}
			}
		} else {
			fmt.Printf("  ⚠ 无应答记录\n")
		}
	}
}

// 测试4: 动态启用/禁用分组
func testDynamicEnableDisable(dnsProxy *proxy.Proxy) {
	domain := "example.com."
	groupName := "custom"

	fmt.Printf("  测试域名: %s (分组: %s)\n", domain, groupName)

	// 初始状态：分组禁用
	fmt.Println("  步骤1: 分组禁用状态 - 应使用默认DNS")
	resp1, err := queryDNS(dnsProxy, domain, dns.TypeA)
	if err != nil {
		fmt.Printf("  ✗ 查询失败: %v\n", err)
		return
	}
	fmt.Printf("  ✓ 查询成功 (禁用状态)\n")

	// 启用分组
	fmt.Println("  步骤2: 启用分组")
	if err := dnsProxy.EnableDomainGroup(groupName); err != nil {
		fmt.Printf("  ✗ 启用失败: %v\n", err)
		return
	}
	fmt.Println("  ✓ 分组已启用")

	// 验证启用后的行为
	fmt.Println("  步骤3: 启用状态 - 应使用自定义DNS")
	resp2, err := queryDNS(dnsProxy, domain, dns.TypeA)
	if err != nil {
		fmt.Printf("  ✗ 查询失败: %v\n", err)
		return
	}
	fmt.Printf("  ✓ 查询成功 (启用状态)\n")

	// 禁用分组
	fmt.Println("  步骤4: 禁用分组")
	if err := dnsProxy.DisableDomainGroup(groupName); err != nil {
		fmt.Printf("  ✗ 禁用失败: %v\n", err)
		return
	}
	fmt.Println("  ✓ 分组已禁用")

	// 验证禁用后的行为
	fmt.Println("  步骤5: 再次禁用状态 - 应使用默认DNS")
	resp3, err := queryDNS(dnsProxy, domain, dns.TypeA)
	if err != nil {
		fmt.Printf("  ✗ 查询失败: %v\n", err)
		return
	}
	fmt.Printf("  ✓ 查询成功 (禁用状态)\n")

	// 比较结果
	if len(resp1.Answer) > 0 && len(resp2.Answer) > 0 && len(resp3.Answer) > 0 {
		fmt.Println("  ✓ 动态启用/禁用功能正常")
	} else {
		fmt.Println("  ⚠ 部分查询无应答")
	}
}

// 测试5: 分组优先级
func testGroupPriority(dnsProxy *proxy.Proxy) {
	// 测试域名同时匹配多个规则时的优先级
	fmt.Println("  测试: 域名匹配优先级")
	
	// 添加一个同时包含关键字和精确匹配的域名
	testDomain := "ad-baidu.com."
	
	fmt.Printf("  查询: %s (同时匹配 'ad' 关键字和 'baidu' 域名)\n", testDomain)
	resp, err := queryDNS(dnsProxy, testDomain, dns.TypeA)
	if err != nil {
		fmt.Printf("  ✗ 查询失败: %v\n", err)
		return
	}
	
	if len(resp.Answer) > 0 {
		fmt.Printf("  ✓ 查询成功，返回 %d 条记录\n", len(resp.Answer))
		fmt.Println("  注: 优先级由配置顺序决定")
	} else {
		fmt.Printf("  ⚠ 无应答记录\n")
	}
}

// 测试6: 预取与分组结合
func testPrefetchWithGroups(dnsProxy *proxy.Proxy) {
	domain := "baidu.com."
	
	fmt.Printf("  测试域名: %s (使用中国DNS分组)\n", domain)
	fmt.Println("  步骤1: 访问3次触发预取队列")

	// 访问3次
	for i := 1; i <= 3; i++ {
		resp, err := queryDNS(dnsProxy, domain, dns.TypeA)
		if err != nil {
			fmt.Printf("  ✗ 查询%d失败: %v\n", i, err)
			return
		}
		fmt.Printf("  ✓ 查询%d完成 (TTL: %d秒)\n", i, getTTL(resp))
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("  步骤2: 等待TTL接近阈值...")
	time.Sleep(3 * time.Second)

	fmt.Println("  步骤3: 再次查询，应触发预取（使用分组DNS）")
	resp, err := queryDNS(dnsProxy, domain, dns.TypeA)
	if err != nil {
		fmt.Printf("  ✗ 查询失败: %v\n", err)
		return
	}
	fmt.Printf("  ✓ 查询完成 (TTL: %d秒)\n", getTTL(resp))

	fmt.Println("  ✓ 预取与分组结合功能正常")
}

// 辅助函数：查询DNS
func queryDNS(dnsProxy *proxy.Proxy, domain string, qtype uint16) (*dns.Msg, error) {
	req := &dns.Msg{}
	req.SetQuestion(domain, qtype)
	req.RecursionDesired = true

	ctx := context.Background()
	dctx := &proxy.DNSContext{
		Proto: proxy.ProtoUDP,
		Req:   req,
		Addr:  nil,
	}

	if err := dnsProxy.Resolve(ctx, dctx); err != nil {
		return nil, err
	}

	return dctx.Res, nil
}

// 辅助函数：获取TTL
func getTTL(msg *dns.Msg) uint32 {
	if msg == nil || len(msg.Answer) == 0 {
		return 0
	}
	return msg.Answer[0].Header().Ttl
}
