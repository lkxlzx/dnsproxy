//go:build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
)

func main() {
	fmt.Println("=== DNSProxy 预取功能快速验证测�?===")
	fmt.Println("测试策略: 使用激进的阈值配置快速触发预�?)
	fmt.Println()

	// 创建代理 - 使用激进的阈�?
	dnsProxy := createProxy()
	if dnsProxy == nil {
		log.Fatal("Failed to create proxy")
	}

	ctx := context.Background()
	fmt.Println("�?DNS代理已创�?)
	fmt.Println()

	testDomain := "google.com"

	// 阶段1: 冷启�?- 建立热度
	fmt.Println("🔥 阶段1: 建立热度...")
	for i := 0; i < 5; i++ {
		latency, ttl := queryDomain(ctx, dnsProxy, testDomain)
		if i == 0 {
			fmt.Printf("  初始查询: %s (延迟: %d μs, TTL: %d�?\n", testDomain, latency, ttl)
			fmt.Printf("  预取阈�? 90%% = %d秒\n", ttl*90/100)
		} else {
			fmt.Printf("  查询 %d: 延迟 %d μs\n", i+1, latency)
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Println("  �?域名已加入预取队�?)
	fmt.Println()

	// 阶段2: 等待并观察预�?
	fmt.Println("�?阶段2: 等待预取触发...")
	fmt.Println("  (预取应该在TTL剩余<90%时自动触�?")
	fmt.Println()

	startTime := time.Now()
	lastLatency := int64(0)
	prefetchDetected := false

	// 持续查询2分钟
	for i := 0; i < 24; i++ {
		time.Sleep(5 * time.Second)

		latency, ttl := queryDomain(ctx, dnsProxy, testDomain)
		elapsed := time.Since(startTime).Seconds()

		// 检测预取（延迟突然增加�?
		if lastLatency > 0 && latency > lastLatency*10 && latency > 10000 {
			fmt.Printf("  [%3.0fs] 🔄 检测到预取! 延迟�?%d μs 增至 %d μs, TTL=%ds\n",
				elapsed, lastLatency, latency, ttl)
			prefetchDetected = true
		} else if latency < 1000 {
			fmt.Printf("  [%3.0fs] �?缓存命中 (延迟: %d μs, TTL: %ds)\n",
				elapsed, latency, ttl)
		} else {
			fmt.Printf("  [%3.0fs] ⚠️  可能未命�?(延迟: %d μs, TTL: %ds)\n",
				elapsed, latency, ttl)
		}

		lastLatency = latency
	}

	// 结果
	fmt.Println()
	fmt.Println("=" + "=")
	fmt.Println("📊 测试结果:")
	if prefetchDetected {
		fmt.Println("  �?预取功能正常工作 - 检测到预取刷新")
	} else {
		fmt.Println("  ⚠️  未检测到明显的预取行�?)
	}
	fmt.Println("=" + "=")
}

func createProxy() *proxy.Proxy {
	upstreams := []string{
		"8.8.8.8:53",
		"1.1.1.1:53",
	}

	dnsUpstreams := make([]upstream.Upstream, 0, len(upstreams))
	for _, addr := range upstreams {
		u, err := upstream.AddressToUpstream(addr, &upstream.Options{
			Timeout: 5 * time.Second,
		})
		if err != nil {
			log.Printf("Failed to create upstream %s: %v", addr, err)
			continue
		}
		dnsUpstreams = append(dnsUpstreams, u)
	}

	if len(dnsUpstreams) == 0 {
		log.Fatal("No valid upstreams")
	}

	// 激进的预取配置 - 在TTL剩余90%时就触发预取
	prefetchConfig := &proxy.PrefetchConfig{
		Enabled:                 true,
		ThresholdSeconds:        5,   // 固定阈�?�?
		ThresholdPercent:        90,  // 百分比阈�?0% - 非常激�?
		MaxConcurrent:           10,
				MinHeatThreshold:        3,
		TimeWindow:              60 * time.Second,
				MaxRetries:              2,
		RetryDelay:              1 * time.Second,
	}

	// 创建DEBUG级别的logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	config := &proxy.Config{
		UpstreamConfig: &proxy.UpstreamConfig{
			Upstreams: dnsUpstreams,
		},
		CacheEnabled:        true,
		CacheSizeBytes:      10 * 1024 * 1024,
		CachePrefetchConfig: prefetchConfig,
		DNSSECEnabled:       true,
		Logger:              logger,
	}

	dnsProxy, err := proxy.New(config)
	if err != nil {
		log.Fatalf("Failed to create proxy: %v", err)
	}

	return dnsProxy
}

func queryDomain(ctx context.Context, p *proxy.Proxy, domain string) (int64, uint32) {
	req := &dns.Msg{}
	req.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	req.RecursionDesired = true

	start := time.Now()

	dctx := &proxy.DNSContext{
		Req: req,
	}

	err := p.Resolve(ctx, dctx)

	latency := time.Since(start).Microseconds()

	if err != nil {
		return latency, 0
	}

	if dctx.Res == nil {
		return latency, 0
	}

	// 提取TTL
	ttl := uint32(0)
	if len(dctx.Res.Answer) > 0 {
		ttl = dctx.Res.Answer[0].Header().Ttl
	}

	return latency, ttl
}
