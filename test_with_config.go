package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/miekg/dns"
	"gopkg.in/yaml.v3"
)

// 使用配置文件的综合测试脚本

type TestConfig struct {
	ListenAddrs                     []string                       `yaml:"listen-addrs"`
	ListenPorts                     []int                          `yaml:"listen-ports"`
	Upstream                        []string                       `yaml:"upstream"`
	Cache                           bool                           `yaml:"cache"`
	CacheSize                       int                            `yaml:"cache-size"`
	CachePrefetchEnabled            bool                           `yaml:"cache-prefetch-enabled"`
	CachePrefetchThresholdSeconds   int                            `yaml:"cache-prefetch-threshold-seconds"`
	CachePrefetchThresholdPercent   int                            `yaml:"cache-prefetch-threshold-percent"`
	CachePrefetchMaxConcurrent      int                            `yaml:"cache-prefetch-max-concurrent"`
	CachePrefetchMinHeatThreshold   int                            `yaml:"cache-prefetch-min-heat-threshold"`
	CachePrefetchTimeWindow         string                         `yaml:"cache-prefetch-time-window"`
	CachePrefetchMaxRetries         int                            `yaml:"cache-prefetch-max-retries"`
	CachePrefetchInitialRetryDelay  string                         `yaml:"cache-prefetch-initial-retry-delay"`
	DomainGroups                    []*proxy.DomainGroupConfig     `yaml:"domain-groups"`
	Verbose                         bool                           `yaml:"verbose"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: test_with_config <config.yaml>")
		fmt.Println("示例: test_with_config test_comprehensive_config.yaml")
		os.Exit(1)
	}

	configFile := os.Args[1]
	fmt.Printf("=== 使用配置文件测试: %s ===\n\n", configFile)

	// 加载配置
	config, err := loadConfig(configFile)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
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

	fmt.Println("\n=== 测试完成 ===")
}

func loadConfig(filename string) (*proxy.Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var testConfig TestConfig
	if err := yaml.Unmarshal(data, &testConfig); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 转换为proxy.Config
	config := &proxy.Config{
		UDPListenAddr: make([]*proxy.BootstrapAddr, 0),
		UpstreamConfig: &proxy.UpstreamConfig{
			Upstreams: testConfig.Upstream,
		},
		CacheEnabled:   testConfig.Cache,
		CacheSizeBytes: testConfig.CacheSize * 1024 * 1024,
		DomainGroups:   testConfig.DomainGroups,
	}

	// 监听地址
	for _, addr := range testConfig.ListenAddrs {
		for _, port := range testConfig.ListenPorts {
			config.UDPListenAddr = append(config.UDPListenAddr, &proxy.BootstrapAddr{
				Address: fmt.Sprintf("%s:%d", addr, port),
			})
		}
	}

	// 预取配置
	if testConfig.CachePrefetchEnabled {
		timeWindow, _ := time.ParseDuration(testConfig.CachePrefetchTimeWindow)
		retryDelay, _ := time.ParseDuration(testConfig.CachePrefetchInitialRetryDelay)

		config.PrefetchConfig = &proxy.PrefetchConfig{
			Enabled:           true,
			ThresholdSeconds:  testConfig.CachePrefetchThresholdSeconds,
			ThresholdPercent:  testConfig.CachePrefetchThresholdPercent,
			MaxConcurrent:     testConfig.CachePrefetchMaxConcurrent,
			MinHeatThreshold:  testConfig.CachePrefetchMinHeatThreshold,
			TimeWindow:        timeWindow,
			MaxRetries:        testConfig.CachePrefetchMaxRetries,
			InitialRetryDelay: retryDelay,
		}
	}

	return config, nil
}

func runTests(dnsProxy *proxy.Proxy) {
	tests := []struct {
		name string
		fn   func(*proxy.Proxy)
	}{
		{"预取功能", testPrefetch},
		{"中国域名分组", testChinaDomains},
		{"广告拦截（关键字匹配）", testAdBlocking},
		{"通配符匹配", testWildcardMatching},
		{"动态启用/禁用", testDynamicToggle},
		{"分组列表查询", testListGroups},
		{"预取与分组结合", testPrefetchWithGroup},
	}

	for i, test := range tests {
		fmt.Printf("【测试%d】%s\n", i+1, test.name)
		test.fn(dnsProxy)
		fmt.Println()
		time.Sleep(500 * time.Millisecond)
	}
}

func testPrefetch(dnsProxy *proxy.Proxy) {
	domain := "google.com."
	fmt.Printf("  域名: %s\n", domain)

	// 访问3次触发预取队列
	for i := 1; i <= 3; i++ {
		resp, err := query(dnsProxy, domain)
		if err != nil {
			fmt.Printf("  ✗ 查询%d失败: %v\n", i, err)
			return
		}
		fmt.Printf("  ✓ 查询%d: TTL=%ds\n", i, getTTL(resp))
		time.Sleep(300 * time.Millisecond)
	}

	// 等待接近阈值
	fmt.Println("  等待3秒...")
	time.Sleep(3 * time.Second)

	// 触发预取
	resp, _ := query(dnsProxy, domain)
	fmt.Printf("  ✓ 触发预取: TTL=%ds\n", getTTL(resp))

	// 验证刷新
	time.Sleep(1 * time.Second)
	resp, _ = query(dnsProxy, domain)
	ttl := getTTL(resp)
	fmt.Printf("  ✓ 验证刷新: TTL=%ds\n", ttl)

	if ttl > 5 {
		fmt.Println("  ✓ 预取成功")
	} else {
		fmt.Println("  ⚠ 预取可能未触发")
	}
}

func testChinaDomains(dnsProxy *proxy.Proxy) {
	domains := []string{"baidu.com.", "taobao.com.", "qq.com."}

	for _, domain := range domains {
		resp, err := query(dnsProxy, domain)
		if err != nil {
			fmt.Printf("  ✗ %s 查询失败: %v\n", domain, err)
			continue
		}
		fmt.Printf("  ✓ %s -> %d条记录\n", domain, len(resp.Answer))
	}
}

func testAdBlocking(dnsProxy *proxy.Proxy) {
	testCases := []struct {
		domain   string
		expected string
	}{
		{"ad-server.example.com.", "拦截"},
		{"tracker.analytics.com.", "拦截"},
		{"doubleclick.net.", "拦截"},
		{"normal-site.com.", "正常"},
	}

	for _, tc := range testCases {
		resp, err := query(dnsProxy, tc.domain)
		if err != nil {
			fmt.Printf("  ✗ %s 查询失败\n", tc.domain)
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
			fmt.Printf("  ✓ %s -> 已拦截 (0.0.0.0)\n", tc.domain)
		} else {
			fmt.Printf("  ✓ %s -> 正常解析\n", tc.domain)
		}
	}
}

func testWildcardMatching(dnsProxy *proxy.Proxy) {
	testCases := []string{
		"test.cdn.example.com.",
		"img.cdn.example.com.",
		"js.static.test.com.",
	}

	for _, domain := range testCases {
		resp, err := query(dnsProxy, domain)
		if err != nil {
			fmt.Printf("  ✗ %s 查询失败\n", domain)
			continue
		}
		fmt.Printf("  ✓ %s -> %d条记录\n", domain, len(resp.Answer))
	}
}

func testDynamicToggle(dnsProxy *proxy.Proxy) {
	domain := "example.com."
	groupName := "custom"

	// 初始禁用
	fmt.Printf("  %s (分组: %s)\n", domain, groupName)
	resp1, _ := query(dnsProxy, domain)
	fmt.Printf("  ✓ 禁用状态: %d条记录\n", len(resp1.Answer))

	// 启用
	if err := dnsProxy.EnableDomainGroup(groupName); err != nil {
		fmt.Printf("  ✗ 启用失败: %v\n", err)
		return
	}
	fmt.Println("  ✓ 已启用分组")

	resp2, _ := query(dnsProxy, domain)
	fmt.Printf("  ✓ 启用状态: %d条记录\n", len(resp2.Answer))

	// 禁用
	if err := dnsProxy.DisableDomainGroup(groupName); err != nil {
		fmt.Printf("  ✗ 禁用失败: %v\n", err)
		return
	}
	fmt.Println("  ✓ 已禁用分组")

	resp3, _ := query(dnsProxy, domain)
	fmt.Printf("  ✓ 禁用状态: %d条记录\n", len(resp3.Answer))
}

func testListGroups(dnsProxy *proxy.Proxy) {
	groups := dnsProxy.GetDomainGroups()
	fmt.Printf("  共有 %d 个域名分组:\n", len(groups))

	for _, group := range groups {
		status := "禁用"
		if group.Enabled {
			status = "启用"
		}
		fmt.Printf("  - %s: %s (%d个域名)\n", group.Name, status, len(group.Domains))
	}
}

func testPrefetchWithGroup(dnsProxy *proxy.Proxy) {
	domain := "baidu.com."
	fmt.Printf("  域名: %s (中国DNS分组)\n", domain)

	// 访问3次
	for i := 1; i <= 3; i++ {
		resp, _ := query(dnsProxy, domain)
		fmt.Printf("  ✓ 查询%d: TTL=%ds\n", i, getTTL(resp))
		time.Sleep(300 * time.Millisecond)
	}

	// 等待并触发预取
	time.Sleep(3 * time.Second)
	resp, _ := query(dnsProxy, domain)
	fmt.Printf("  ✓ 触发预取: TTL=%ds\n", getTTL(resp))

	time.Sleep(1 * time.Second)
	resp, _ = query(dnsProxy, domain)
	fmt.Printf("  ✓ 验证刷新: TTL=%ds\n", getTTL(resp))
}

func query(dnsProxy *proxy.Proxy, domain string) (*dns.Msg, error) {
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
