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
	fmt.Println("🔍 调试预取触发逻辑")
	fmt.Println()

	// 创建代理 - 使用INFO级别日志
	dnsProxy := createProxy()
	if dnsProxy == nil {
		log.Fatal("�?创建代理失败")
	}
	fmt.Println("�?DNS代理已创�?)
	fmt.Println()

	ctx := context.Background()
	testDomain := "google.com"

	// 冷启�?- 建立热度
	fmt.Println("🔥 冷启动阶�?..")
	for i := 0; i < 5; i++ {
		queryDomain(ctx, dnsProxy, testDomain)
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Println("�?冷启动完�?)
	fmt.Println()

	// 持续查询直到TTL降到阈值以�?
	fmt.Println("⏱️  持续查询直到预取触发...")
	timeout := time.After(3 * time.Minute)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			fmt.Println("�?测试完成")
			return
		case <-ticker.C:
			queryDomain(ctx, dnsProxy, testDomain)
		}
	}
}

func createProxy() *proxy.Proxy {
	upstreams := []string{"8.8.8.8:53"}

	dnsUpstreams := make([]upstream.Upstream, 0)
	for _, addr := range upstreams {
		u, err := upstream.AddressToUpstream(addr, &upstream.Options{
			Timeout: 5 * time.Second,
		})
		if err != nil {
			log.Printf("创建上游失败 %s: %v", addr, err)
			continue
		}
		dnsUpstreams = append(dnsUpstreams, u)
	}

	prefetchConfig := &proxy.PrefetchConfig{
		Enabled:                 true,
		ThresholdSeconds:        5,
		ThresholdPercent:        10,
		MaxConcurrent:           10,
				MinHeatThreshold:        3,
		TimeWindow:              60 * time.Second,
				MaxRetries:              2,
		RetryDelay:              1 * time.Second,
	}

	// 使用INFO级别日志
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
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
		log.Fatalf("创建代理失败: %v", err)
	}

	return dnsProxy
}

func queryDomain(ctx context.Context, p *proxy.Proxy, domain string) {
	req := &dns.Msg{}
	req.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	req.RecursionDesired = true

	dctx := &proxy.DNSContext{Req: req}
	_ = p.Resolve(ctx, dctx)
	
	if dctx.Res != nil {
		ttl := uint32(0)
		if len(dctx.Res.Answer) > 0 {
			ttl = dctx.Res.Answer[0].Header().Ttl
		}
		cached := dctx.Upstream == nil
		fmt.Printf("查询: TTL=%ds 缓存=%v\n", ttl, cached)
	}
}
