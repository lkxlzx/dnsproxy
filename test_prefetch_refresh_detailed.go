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

// 测试预取刷新的详细流程
// 验证：访问3次加入队列、TTL到期前刷新、刷新后新TTL

type RefreshEvent struct {
	Time        time.Time
	Event       string
	TTL         uint32
	RemainingTTL int64
	InQueue     bool
	AccessCount int
}

var events []RefreshEvent

func main() {
	fmt.Println("===========================================")
	fmt.Println("  DNS Prefetch Refresh Logic Test")
	fmt.Println("===========================================")
	fmt.Println()

	// 创建代理
	dnsProxy := createProxy()
	if dnsProxy == nil {
		log.Fatal("Failed to create proxy")
	}

	ctx := context.Background()
	if err := dnsProxy.Start(ctx); err != nil {
		log.Fatal("Failed to start proxy:", err)
	}
	defer dnsProxy.Shutdown(ctx)

	fmt.Println("+ Proxy started successfully")
	fmt.Println()

	// 运行测试
	testDomain := "google.com."
	runRefreshTest(dnsProxy, testDomain)

	// 打印事件时间线
	printTimeline()
}


func createProxy() *proxy.Proxy {
	// 创建上游
	ups, err := upstream.AddressToUpstream("8.8.8.8", &upstream.Options{
		Timeout: 5 * time.Second,
	})
	if err != nil {
		log.Printf("Failed to create upstream: %v", err)
		return nil
	}

	// 预取配置 - 使用较短的阈值便于测试
	prefetchConfig := &proxy.PrefetchConfig{
		Enabled:          true,
		ThresholdSeconds: 3,   // 剩余3秒时触发
		ThresholdPercent: 80,  // 或剩余20%时触发
		MaxConcurrent:    5,
		MinHeatThreshold: 3,   // 访问3次加入队列
		TimeWindow:       60 * time.Second,
		MaxRetries:       2,
	}

	config := &proxy.Config{
		UDPListenAddr: []*net.UDPAddr{
			{IP: net.ParseIP("127.0.0.1"), Port: 15357},
		},
		UpstreamConfig: &proxy.UpstreamConfig{
			Upstreams: []upstream.Upstream{ups},
		},
		CacheEnabled:        true,
		CacheSizeBytes:      10 * 1024 * 1024,
		CachePrefetchConfig: prefetchConfig,
	}

	dnsProxy, err := proxy.New(config)
	if err != nil {
		log.Printf("Failed to create proxy: %v", err)
		return nil
	}

	return dnsProxy
}


func runRefreshTest(dnsProxy *proxy.Proxy, domain string) {
	fmt.Println("===========================================")
	fmt.Println("Phase 1: Cold Start (3 accesses to join queue)")
	fmt.Println("===========================================")
	fmt.Println()

	startTime := time.Now()
	var initialTTL uint32

	// 第1次访问
	fmt.Println("[Access 1] First query - cold start")
	resp1, err := query(dnsProxy, domain)
	if err != nil {
		log.Printf("Query 1 failed: %v", err)
		return
	}
	ttl1 := getTTL(resp1)
	initialTTL = ttl1
	recordEvent(startTime, "Access 1 (Cold Start)", ttl1, int64(ttl1), false, 1)
	fmt.Printf("  TTL: %ds\n", ttl1)
	fmt.Printf("  Status: Not in queue yet (need 3 accesses)\n")
	fmt.Println()
	time.Sleep(500 * time.Millisecond)

	// 第2次访问
	fmt.Println("[Access 2] Second query")
	resp2, err := query(dnsProxy, domain)
	if err != nil {
		log.Printf("Query 2 failed: %v", err)
		return
	}
	ttl2 := getTTL(resp2)
	elapsed2 := time.Since(startTime).Seconds()
	remaining2 := int64(ttl1) - int64(elapsed2)
	recordEvent(startTime, "Access 2", ttl2, remaining2, false, 2)
	fmt.Printf("  TTL: %ds (from cache)\n", ttl2)
	fmt.Printf("  Remaining: ~%ds\n", remaining2)
	fmt.Printf("  Status: Not in queue yet (need 3 accesses)\n")
	fmt.Println()
	time.Sleep(500 * time.Millisecond)

	// 第3次访问 - 应该加入预取队列
	fmt.Println("[Access 3] Third query - SHOULD JOIN PREFETCH QUEUE")
	resp3, err := query(dnsProxy, domain)
	if err != nil {
		log.Printf("Query 3 failed: %v", err)
		return
	}
	ttl3 := getTTL(resp3)
	elapsed3 := time.Since(startTime).Seconds()
	remaining3 := int64(ttl1) - int64(elapsed3)
	recordEvent(startTime, "Access 3 (Join Queue)", ttl3, remaining3, true, 3)
	fmt.Printf("  TTL: %ds (from cache)\n", ttl3)
	fmt.Printf("  Remaining: ~%ds\n", remaining3)
	fmt.Printf("  Status: *** JOINED PREFETCH QUEUE ***\n")
	fmt.Println()


	// Phase 2: 等待接近阈值
	fmt.Println("===========================================")
	fmt.Println("Phase 2: Waiting for TTL to approach threshold")
	fmt.Println("===========================================")
	fmt.Println()

	// 计算需要等待的时间
	// 阈值：剩余3秒 或 剩余20%
	thresholdSeconds := int64(3)
	thresholdPercent := int64(initialTTL) * 20 / 100
	
	fmt.Printf("Initial TTL: %ds\n", initialTTL)
	fmt.Printf("Threshold: %ds (fixed) or %ds (20%% of TTL)\n", thresholdSeconds, thresholdPercent)
	fmt.Printf("Will trigger when remaining < %ds\n", max(thresholdSeconds, thresholdPercent))
	fmt.Println()

	// 等待到接近阈值
	waitTime := time.Duration(int64(initialTTL)-max(thresholdSeconds, thresholdPercent)-2) * time.Second
	if waitTime > 0 {
		fmt.Printf("Waiting %v for TTL to approach threshold...\n", waitTime)
		time.Sleep(waitTime)
		fmt.Println()
	}

	// Phase 3: 触发预取
	fmt.Println("===========================================")
	fmt.Println("Phase 3: Trigger Prefetch")
	fmt.Println("===========================================")
	fmt.Println()

	fmt.Println("[Access 4] Query near threshold - SHOULD TRIGGER PREFETCH")
	resp4, err := query(dnsProxy, domain)
	if err != nil {
		log.Printf("Query 4 failed: %v", err)
		return
	}
	ttl4 := getTTL(resp4)
	elapsed4 := time.Since(startTime).Seconds()
	remaining4 := int64(ttl1) - int64(elapsed4)
	recordEvent(startTime, "Access 4 (Trigger Prefetch)", ttl4, remaining4, true, 4)
	fmt.Printf("  TTL: %ds (from cache)\n", ttl4)
	fmt.Printf("  Remaining: ~%ds\n", remaining4)
	fmt.Printf("  Status: *** PREFETCH TRIGGERED (async) ***\n")
	fmt.Println()


	// Phase 4: 验证刷新
	fmt.Println("===========================================")
	fmt.Println("Phase 4: Verify Cache Refresh")
	fmt.Println("===========================================")
	fmt.Println()

	// 等待预取完成
	fmt.Println("Waiting 2 seconds for prefetch to complete...")
	time.Sleep(2 * time.Second)
	fmt.Println()

	fmt.Println("[Access 5] Query after prefetch - VERIFY NEW TTL")
	resp5, err := query(dnsProxy, domain)
	if err != nil {
		log.Printf("Query 5 failed: %v", err)
		return
	}
	ttl5 := getTTL(resp5)
	elapsed5 := time.Since(startTime).Seconds()
	recordEvent(startTime, "Access 5 (After Refresh)", ttl5, int64(ttl5), true, 5)
	fmt.Printf("  TTL: %ds\n", ttl5)
	fmt.Printf("  Elapsed since start: %.1fs\n", elapsed5)
	fmt.Println()

	// 分析结果
	fmt.Println("===========================================")
	fmt.Println("Analysis")
	fmt.Println("===========================================")
	fmt.Println()

	if int64(ttl5) > remaining4 {
		fmt.Printf("✓ SUCCESS: Cache was refreshed!\n")
		fmt.Printf("  Before refresh: remaining ~%ds\n", remaining4)
		fmt.Printf("  After refresh: TTL %ds\n", ttl5)
		fmt.Printf("  TTL increased by ~%ds\n", int64(ttl5)-remaining4)
	} else {
		fmt.Printf("✗ FAILED: Cache was NOT refreshed\n")
		fmt.Printf("  Expected: TTL > %ds\n", remaining4)
		fmt.Printf("  Actual: TTL = %ds\n", ttl5)
	}
	fmt.Println()

	// 继续监控
	fmt.Println("===========================================")
	fmt.Println("Phase 5: Monitor Subsequent Refreshes")
	fmt.Println("===========================================")
	fmt.Println()

	fmt.Println("Continuing to query and monitor for next refresh cycle...")
	fmt.Println()

	for i := 6; i <= 10; i++ {
		time.Sleep(2 * time.Second)
		resp, err := query(dnsProxy, domain)
		if err != nil {
			continue
		}
		ttl := getTTL(resp)
		elapsed := time.Since(startTime).Seconds()
		recordEvent(startTime, fmt.Sprintf("Access %d", i), ttl, int64(ttl), true, i)
		fmt.Printf("[Access %d] TTL: %ds, Elapsed: %.1fs\n", i, ttl, elapsed)
	}
	fmt.Println()
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

func recordEvent(startTime time.Time, event string, ttl uint32, remaining int64, inQueue bool, accessCount int) {
	events = append(events, RefreshEvent{
		Time:        time.Now(),
		Event:       event,
		TTL:         ttl,
		RemainingTTL: remaining,
		InQueue:     inQueue,
		AccessCount: accessCount,
	})
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}


func printTimeline() {
	fmt.Println("===========================================")
	fmt.Println("Event Timeline")
	fmt.Println("===========================================")
	fmt.Println()

	if len(events) == 0 {
		fmt.Println("No events recorded")
		return
	}

	startTime := events[0].Time
	fmt.Printf("%-8s %-30s %-8s %-12s %-10s %-8s\n", 
		"Time", "Event", "TTL", "Remaining", "In Queue", "Access#")
	fmt.Println("------------------------------------------------------------------------------------")

	for _, e := range events {
		elapsed := e.Time.Sub(startTime).Seconds()
		queueStatus := "No"
		if e.InQueue {
			queueStatus = "Yes"
		}
		fmt.Printf("+%-7.1fs %-30s %-8ds %-12ds %-10s %-8d\n",
			elapsed, e.Event, e.TTL, e.RemainingTTL, queueStatus, e.AccessCount)
	}
	fmt.Println()

	// 分析刷新点
	fmt.Println("===========================================")
	fmt.Println("Refresh Analysis")
	fmt.Println("===========================================")
	fmt.Println()

	for i := 1; i < len(events); i++ {
		prev := events[i-1]
		curr := events[i]
		
		// 检测TTL增加（表示刷新发生）
		if curr.TTL > prev.TTL+5 { // 增加超过5秒认为是刷新
			fmt.Printf("✓ Refresh detected between Access %d and Access %d\n", 
				prev.AccessCount, curr.AccessCount)
			fmt.Printf("  Before: TTL=%ds, Remaining=%ds\n", prev.TTL, prev.RemainingTTL)
			fmt.Printf("  After:  TTL=%ds, Remaining=%ds\n", curr.TTL, curr.RemainingTTL)
			fmt.Printf("  TTL increased by: %ds\n", curr.TTL-prev.TTL)
			fmt.Println()
		}
	}

	fmt.Println("===========================================")
	fmt.Println("Test Complete")
	fmt.Println("===========================================")
}
