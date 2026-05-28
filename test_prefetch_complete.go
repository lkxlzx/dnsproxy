//go:build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
)

// 测试统计
type TestStats struct {
	totalQueries    atomic.Int64
	cacheHits       atomic.Int64
	cacheMisses     atomic.Int64
	prefetchCount   atomic.Int64
	lastTTL         atomic.Int64
	lastLatency     atomic.Int64
	initialTTL      int64
	currentBaseTTL  int64 // 当前用于计算阈值的基准TTL
	startTime       time.Time
	mu              sync.RWMutex
	prefetchHistory []PrefetchEvent
	lastIP          string
	ipChangeCount   int
}

type PrefetchEvent struct {
	timestamp time.Time
	oldTTL    uint32
	newTTL    uint32
	oldIP     string
	newIP     string
	elapsed   float64
	ipChanged bool
}

func (s *TestStats) recordPrefetch(oldTTL, newTTL uint32, oldIP, newIP string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prefetchCount.Add(1)
	ipChanged := oldIP != newIP
	if ipChanged {
		s.ipChangeCount++
	}
	s.prefetchHistory = append(s.prefetchHistory, PrefetchEvent{
		timestamp: time.Now(),
		oldTTL:    oldTTL,
		newTTL:    newTTL,
		oldIP:     oldIP,
		newIP:     newIP,
		elapsed:   time.Since(s.startTime).Seconds(),
		ipChanged: ipChanged,
	})
	// 更新基准TTL用于阈值计�?
	s.currentBaseTTL = int64(newTTL)
}

func main() {
	fmt.Println("╔════════════════════════════════════════════════════════════════╗")
	fmt.Println("�?         DNSProxy 预取功能完整测试                              �?)
	fmt.Println("╚════════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("📋 测试配置:")
	fmt.Println("  �?预取阈�? 10% (剩余TTL �?10%时触�?")
	fmt.Println("  �?扫描间隔: 500ms")
	fmt.Println("  �?状态更�? 每秒一�?)
	fmt.Println("  �?测试时长: 5分钟")
	fmt.Println()

	// 创建代理
	dnsProxy := createProxy()
	if dnsProxy == nil {
		log.Fatal("�?创建代理失败")
	}
	fmt.Println("�?DNS代理已创建并启动")
	fmt.Println()

	ctx := context.Background()
	stats := &TestStats{
		startTime: time.Now(),
	}

	testDomain := "google.com"

	// 阶段1: 冷启�?
	runColdStart(ctx, dnsProxy, testDomain, stats)

	// 阶段2: 持续监控
	runMonitoring(ctx, dnsProxy, testDomain, stats)

	// 打印最终报�?
	printFinalReport(stats)
}


// createProxy 创建配置好的DNS代理
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
			log.Printf("创建上游失败 %s: %v", addr, err)
			continue
		}
		dnsUpstreams = append(dnsUpstreams, u)
	}

	if len(dnsUpstreams) == 0 {
		log.Fatal("没有可用的上游服务器")
	}

	// 预取配置 - 10%阈�?
	prefetchConfig := &proxy.PrefetchConfig{
		Enabled:                 true,
		ThresholdSeconds:        5,   // 固定阈�?�?
		ThresholdPercent:        10,  // 百分比阈�?0%
		MaxConcurrent:           10,
				MinHeatThreshold:        3,
		TimeWindow:              60 * time.Second,
				MaxRetries:              2,
		RetryDelay:              1 * time.Second,
	}

	// 创建WARN级别的logger以减少日志噪�?
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelWarn,
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


// runColdStart 冷启动阶�?- 建立热度
func runColdStart(ctx context.Context, p *proxy.Proxy, domain string, stats *TestStats) {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("🔥 阶段1: 冷启�?- 建立域名热度")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	for i := 0; i < 5; i++ {
		latency, ttl := queryDomain(ctx, p, domain, stats)
		
		if i == 0 {
			stats.initialTTL = int64(ttl)
			stats.currentBaseTTL = int64(ttl) // 初始化基准TTL
			threshold := calculateThreshold(ttl)
			fmt.Printf("📊 初始查询结果:\n")
			fmt.Printf("   �?域名: %s\n", domain)
			fmt.Printf("   �?延迟: %d μs (%.2f ms)\n", latency, float64(latency)/1000)
			fmt.Printf("   �?TTL: %d 秒\n", ttl)
			fmt.Printf("   �?预取阈�? %d �?(剩余 �?%.0f%%)\n", threshold, float64(threshold)/float64(ttl)*100)
			fmt.Println()
		} else {
			status := "�?
			if latency > 1000 {
				status = "⚠️"
			}
			fmt.Printf("   %s 查询 %d: 延迟 %d μs\n", status, i+1, latency)
		}
		
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Println()
	fmt.Println("�?冷启动完�?- 域名已加入预取队�?)
	fmt.Println()
}

// calculateThreshold 计算预取阈�?
func calculateThreshold(ttl uint32) uint32 {
	threshold := uint32(5)
	pctThreshold := ttl * 10 / 100
	if pctThreshold > threshold {
		threshold = pctThreshold
	}
	return threshold
}


// runMonitoring 持续监控阶段
func runMonitoring(ctx context.Context, p *proxy.Proxy, domain string, stats *TestStats) {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("⏱️  阶段2: 持续监控 - 观察预取行为")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeout := time.After(5 * time.Minute)
	lastPrefetchCount := int64(0)

	for {
		select {
		case <-timeout:
			fmt.Println()
			fmt.Println("⏱️  监控时间结束")
			return

		case <-ticker.C:
			latency, ttl := queryDomain(ctx, p, domain, stats)
			elapsed := time.Since(stats.startTime).Seconds()
			
			// 检测预�?
			currentPrefetchCount := stats.prefetchCount.Load()
			prefetchDetected := currentPrefetchCount > lastPrefetchCount
			lastPrefetchCount = currentPrefetchCount

			// 计算状�?
			stats.mu.RLock()
			baseTTL := uint32(stats.currentBaseTTL)
			stats.mu.RUnlock()
			
			threshold := calculateThreshold(baseTTL)
			needsPrefetch := ttl < threshold
			
			// 格式化输�?
			printStatus(elapsed, latency, ttl, threshold, needsPrefetch, prefetchDetected, stats)
		}
	}
}

// printStatus 打印当前状�?
func printStatus(elapsed float64, latency int64, ttl, threshold uint32, needsPrefetch, prefetchDetected bool, stats *TestStats) {
	// 时间�?
	timestamp := fmt.Sprintf("[%3.0fs]", elapsed)
	
	// 延迟状�?
	var latencyStr string
	if latency < 1000 {
		latencyStr = fmt.Sprintf("�?%4d μs", latency)
	} else {
		latencyStr = fmt.Sprintf("⚠️  %4d μs", latency)
	}
	
	// TTL状�?
	ttlStr := fmt.Sprintf("TTL:%3ds", ttl)
	
	// 阈值状�?
	var thresholdStr string
	if needsPrefetch {
		thresholdStr = fmt.Sprintf("🎯 �?ds", threshold)
	} else {
		thresholdStr = fmt.Sprintf("   >%ds", threshold)
	}
	
	// 预取状�?
	var prefetchStr string
	if prefetchDetected {
		stats.mu.RLock()
		lastEvent := stats.prefetchHistory[len(stats.prefetchHistory)-1]
		stats.mu.RUnlock()
		
		if lastEvent.ipChanged {
			prefetchStr = fmt.Sprintf("🔄 预取触发! IP变化: %s �?%s", lastEvent.oldIP, lastEvent.newIP)
		} else {
			prefetchStr = fmt.Sprintf("🔄 预取触发! IP不变: %s", lastEvent.newIP)
		}
	} else {
		prefetchStr = ""
	}
	
	// 统计
	total := stats.totalQueries.Load()
	hits := stats.cacheHits.Load()
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100
	}
	statsStr := fmt.Sprintf("查询:%d 命中�?%.1f%%", total, hitRate)
	
	// 组合输出
	if prefetchStr != "" {
		fmt.Printf("%s %s | %s | %s | %s\n", 
			timestamp, latencyStr, ttlStr, thresholdStr, statsStr)
		fmt.Printf("     %s\n", prefetchStr)
	} else {
		fmt.Printf("%s %s | %s | %s | %s\n", 
			timestamp, latencyStr, ttlStr, thresholdStr, statsStr)
	}
}


// queryDomain 执行DNS查询
func queryDomain(ctx context.Context, p *proxy.Proxy, domain string, stats *TestStats) (int64, uint32) {
	req := &dns.Msg{}
	req.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	req.RecursionDesired = true

	start := time.Now()

	dctx := &proxy.DNSContext{
		Req: req,
	}

	err := p.Resolve(ctx, dctx)

	latency := time.Since(start).Microseconds()

	stats.totalQueries.Add(1)

	if err != nil {
		stats.lastLatency.Store(latency)
		return latency, 0
	}

	if dctx.Res == nil {
		stats.lastLatency.Store(latency)
		return latency, 0
	}

	// 检测缓存命�?
	if dctx.Upstream == nil {
		stats.cacheHits.Add(1)
	} else {
		stats.cacheMisses.Add(1)
	}

	// 提取TTL和IP
	ttl := uint32(0)
	currentIP := ""
	if len(dctx.Res.Answer) > 0 {
		ttl = dctx.Res.Answer[0].Header().Ttl
		// 提取IP地址
		if aRecord, ok := dctx.Res.Answer[0].(*dns.A); ok {
			currentIP = aRecord.A.String()
		}
	}

	// 检测预取（TTL突然增加或IP变化�?
	oldTTL := uint32(stats.lastTTL.Load())
	stats.mu.RLock()
	oldIP := stats.lastIP
	stats.mu.RUnlock()
	
	if oldTTL > 0 && currentIP != "" {
		// TTL增加超过5秒，或IP变化，说明发生了预取
		if ttl > oldTTL+5 || (oldIP != "" && currentIP != oldIP) {
			stats.recordPrefetch(oldTTL, ttl, oldIP, currentIP)
		}
	}

	stats.lastTTL.Store(int64(ttl))
	stats.lastLatency.Store(latency)
	
	stats.mu.Lock()
	stats.lastIP = currentIP
	stats.mu.Unlock()

	return latency, ttl
}


// printFinalReport 打印最终报�?
func printFinalReport(stats *TestStats) {
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════════╗")
	fmt.Println("�?                   📊 测试报告                                  �?)
	fmt.Println("╚════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	total := stats.totalQueries.Load()
	hits := stats.cacheHits.Load()
	misses := stats.cacheMisses.Load()
	prefetchCount := stats.prefetchCount.Load()

	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100
	}

	duration := time.Since(stats.startTime)

	fmt.Println("⏱️  测试时长:")
	fmt.Printf("   �?总时�? %.1f 秒\n", duration.Seconds())
	fmt.Println()

	fmt.Println("📈 查询统计:")
	fmt.Printf("   �?总查询数: %d\n", total)
	fmt.Printf("   �?缓存命中: %d (%.2f%%)\n", hits, hitRate)
	fmt.Printf("   �?缓存未命�? %d (%.2f%%)\n", misses, float64(misses)/float64(total)*100)
	fmt.Println()

	fmt.Println("🔄 预取统计:")
	fmt.Printf("   �?预取触发次数: %d\n", prefetchCount)
	
	stats.mu.RLock()
	ipChangeCount := stats.ipChangeCount
	if len(stats.prefetchHistory) > 0 {
		fmt.Println("   �?预取历史:")
		for i, event := range stats.prefetchHistory {
			if event.ipChanged {
				fmt.Printf("      %d. [%.1fs] TTL: %ds �?%ds | IP变化: %s �?%s ⭐\n", 
					i+1, event.elapsed, event.oldTTL, event.newTTL, event.oldIP, event.newIP)
			} else {
				fmt.Printf("      %d. [%.1fs] TTL: %ds �?%ds | IP不变: %s\n", 
					i+1, event.elapsed, event.oldTTL, event.newTTL, event.newIP)
			}
		}
	}
	if ipChangeCount > 0 {
		fmt.Printf("   �?IP地址变化次数: %d ⭐\n", ipChangeCount)
	}
	stats.mu.RUnlock()
	fmt.Println()

	fmt.Println("�?功能评估:")
	if prefetchCount > 0 {
		fmt.Println("   �?预取功能: �?正常工作")
		fmt.Println("   �?预取触发: �?按预期触�?)
	} else {
		fmt.Println("   �?预取功能: ⚠️  未检测到预取")
		fmt.Println("   �?可能原因: TTL过长或阈值设置不�?)
	}

	if hitRate >= 95 {
		fmt.Println("   �?缓存命中�? ⭐⭐⭐⭐�?优秀")
	} else if hitRate >= 85 {
		fmt.Println("   �?缓存命中�? ⭐⭐⭐⭐ 良好")
	} else if hitRate >= 70 {
		fmt.Println("   �?缓存命中�? ⭐⭐�?一�?)
	} else {
		fmt.Println("   �?缓存命中�? ⭐⭐ 需改进")
	}

	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════════╗")
	fmt.Println("�?                   �?测试完成                                  �?)
	fmt.Println("╚════════════════════════════════════════════════════════════════╝")
}
