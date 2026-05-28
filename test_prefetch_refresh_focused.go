//go:build ignore

package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/miekg/dns"
)

// 测试统计
type RefreshStats struct {
	totalQueries      atomic.Int64
	cacheHits         atomic.Int64
	cacheMisses       atomic.Int64
	prefetchDetected  atomic.Int64 // 检测到预取发生的次�?
	
	// 延迟统计
	prePrefetchLatency  atomic.Int64 // 预取前的平均延迟（微秒）
	postPrefetchLatency atomic.Int64 // 预取后的平均延迟（微秒）
	
	// TTL 跟踪
	initialTTL atomic.Int64 // 初始TTL
	firstQuery time.Time     // 第一次查询时�?
}

func main() {
	fmt.Println("=== DNSProxy 主动刷新（预取）功能测试 ===")
	fmt.Println("测试目标: 验证域名的主动刷新机�?)
	fmt.Println("测试时长: 6分钟")
	fmt.Println()
	
	// 创建代理
	dnsProxy := createProxy()
	if dnsProxy == nil {
		log.Fatal("Failed to create proxy")
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	fmt.Println("�?DNS代理已创�?)
	fmt.Println()
	
	stats := &RefreshStats{}
	
	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	
	// 测试流程
	testCtx, testCancel := context.WithTimeout(ctx, 6*time.Minute)
	defer testCancel()
	
	fmt.Println("📋 测试流程:")
	fmt.Println("  阶段1: 冷启�?- 建立热度 (30�?")
	fmt.Println("  阶段2: 监控TTL - 等待预取触发 (4分钟)")
	fmt.Println("  阶段3: 验证刷新效果 (90�?")
	fmt.Println()
	
	// 阶段1: 冷启�?- 快速访问域名以建立热度
	fmt.Println("🔥 阶段1: 冷启动阶�?..")
	runColdStartPhase(testCtx, dnsProxy, stats)
	
	// 阶段2: 监控TTL并等待预取触�?
	fmt.Println("\n�?阶段2: 监控TTL并等待预取触�?..")
	runWaitForPrefetchPhase(testCtx, dnsProxy, stats)
	
	// 阶段3: 验证刷新效果
	fmt.Println("\n�?阶段3: 验证刷新效果...")
	runVerifyRefreshPhase(testCtx, dnsProxy, stats)
	
	// 等待完成或中�?
	select {
	case <-testCtx.Done():
		fmt.Println("\n⏱️  测试完成")
	case <-sigChan:
		fmt.Println("\n⚠️  收到中断信号")
		testCancel()
	}
	
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
			log.Printf("Failed to create upstream %s: %v", addr, err)
			continue
		}
		dnsUpstreams = append(dnsUpstreams, u)
	}
	
	if len(dnsUpstreams) == 0 {
		log.Fatal("No valid upstreams")
	}
	
	// 预取配置 - 使用更合理的阈�?
	prefetchConfig := &proxy.PrefetchConfig{
		Enabled:                 true,
		ThresholdSeconds:        20, // 增加�?0秒，留出更多缓冲时间
		ThresholdPercent:        10, // 增加�?0%
		MaxConcurrent:           10, // 增加并发�?
		ScanInterval:            500 * time.Millisecond, // 减少�?00ms，更频繁扫描
		MinHeatThreshold:        3,
		TimeWindow:              60 * time.Second,
				MaxRetries:              2,
		RetryDelay:              1 * time.Second,
	}
	
	// 创建自定义logger，设置为DEBUG级别以查看详细日�?
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug, // 启用DEBUG日志
	}))
	
	config := &proxy.Config{
		UpstreamConfig: &proxy.UpstreamConfig{
			Upstreams: dnsUpstreams,
		},
		CacheEnabled:        true,
		CacheSizeBytes:      10 * 1024 * 1024,
		CachePrefetchConfig: prefetchConfig,
		DNSSECEnabled:       true,
		Logger:              logger, // 使用自定义logger
	}
	
	dnsProxy, err := proxy.New(config)
	if err != nil {
		log.Fatalf("Failed to create proxy: %v", err)
	}
	
	return dnsProxy
}


// 阶段1: 冷启�?- 快速访问域名建立热�?
func runColdStartPhase(ctx context.Context, p *proxy.Proxy, stats *RefreshStats) {
	testDomain := "google.com"
	
	// 快速访�?次以建立热度并加入预取队�?
	for i := 0; i < 5; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}
		
		latency, ttl := queryDomainWithTTL(ctx, p, testDomain, stats)
		
		// 记录初始TTL
		if i == 0 && ttl > 0 {
			stats.initialTTL.Store(int64(ttl))
			stats.firstQuery = time.Now()
			fmt.Printf("  查询 %d: %s (延迟: %d μs, TTL: %d�?\n", i+1, testDomain, latency, ttl)
		} else {
			fmt.Printf("  查询 %d: %s (延迟: %d μs)\n", i+1, testDomain, latency)
		}
		time.Sleep(500 * time.Millisecond)
	}
	
	fmt.Println("  �?域名已访�?次，应该已加入预取队�?)
	
	// 继续访问以保持热�?
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	timeout := time.After(25 * time.Second)
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout:
			return
		case <-ticker.C:
			latency, _ := queryDomainWithTTL(ctx, p, testDomain, stats)
			fmt.Printf("  保持热度: %s (延迟: %d μs)\n", testDomain, latency)
		}
	}
}

// 阶段2: 监控TTL并等待预取触�?
func runWaitForPrefetchPhase(ctx context.Context, p *proxy.Proxy, stats *RefreshStats) {
	testDomain := "google.com"
	
	initialTTL := stats.initialTTL.Load()
	firstQuery := stats.firstQuery
	
	// 计算预取阈值（与实际算法一致）
	thresholdSeconds := uint32(10)
	thresholdPercent := uint32(5)
	
	threshold := thresholdSeconds
	if thresholdPercent > 0 {
		pctThreshold := uint32(initialTTL) * thresholdPercent / 100
		if pctThreshold > threshold {
			threshold = pctThreshold
		}
	}
	
	fmt.Printf("  初始TTL: %d秒\n", initialTTL)
	fmt.Printf("  预取阈值算�? max(固定秒数, 原始TTL × 百分�?\n")
	fmt.Printf("  预取阈值计�? max(%d�? %d�?× %d%%) = max(%d, %d) = %d秒\n",
		thresholdSeconds, initialTTL, thresholdPercent,
		thresholdSeconds, uint32(initialTTL)*thresholdPercent/100, threshold)
	fmt.Println()
	
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	timeout := time.After(4 * time.Minute)
	lastLatency := int64(0)
	
	// 用于跟踪当前的基准时间和TTL（会在检测到刷新时更新）
	currentBaseTTL := initialTTL
	currentBaseTime := firstQuery
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout:
			fmt.Println("  ⏱️  监控阶段结束")
			return
		case <-ticker.C:
			latency, currentTTL := queryDomainWithTTL(ctx, p, testDomain, stats)
			
			// 计算理论剩余TTL（基于当前基准）
			elapsed := time.Since(currentBaseTime).Seconds()
			theoreticalRemaining := int64(currentBaseTTL) - int64(elapsed)
			if theoreticalRemaining < 0 {
				theoreticalRemaining = 0
			}
			
			// 检测TTL重置（明确的预取证据�?
			if currentTTL > 0 && theoreticalRemaining >= 0 {
				ttlDiff := int64(currentTTL) - theoreticalRemaining
				// TTL增加超过60秒，说明被刷新了
				if ttlDiff > 60 {
					stats.prefetchDetected.Add(1)
					fmt.Printf("  🔄 检测到TTL刷新! 理论剩余: %ds, 实际TTL: %ds (增加 %ds)\n",
						theoreticalRemaining, currentTTL, ttlDiff)
					
					// 重置基准时间和TTL
					currentBaseTTL = int64(currentTTL)
					currentBaseTime = time.Now()
					theoreticalRemaining = int64(currentTTL)
					
					fmt.Printf("  📝 已重置基�? 新TTL=%ds, 新基准时�?%s\n",
						currentBaseTTL, currentBaseTime.Format("15:04:05"))
				}
			}
			
			// 检测延迟突然增加（正在预取�?
			if lastLatency > 0 && latency > lastLatency*10 && latency > 10000 {
				fmt.Printf("  🔄 检测到预取查询! 延迟�?%d μs 增至 %d μs\n", 
					lastLatency, latency)
			}
			
			// 计算TTL百分�?
			percentage := float64(0)
			if currentBaseTTL > 0 {
				percentage = float64(theoreticalRemaining) / float64(currentBaseTTL) * 100
			}
			
			// 显示状�?
			status := "正常"
			if theoreticalRemaining < int64(threshold) {
				status = "🎯 已达预取阈�?
			}
			
			fmt.Printf("  [%s] 延迟: %d μs | 理论剩余TTL: %ds (%.1f%%) | 实际TTL: %ds | 阈�? %ds | %s\n",
				time.Now().Format("15:04:05"),
				latency,
				theoreticalRemaining,
				percentage,
				currentTTL,
				threshold,
				status)
			
			lastLatency = latency
		}
	}
}

// 阶段3: 验证刷新效果
func runVerifyRefreshPhase(ctx context.Context, p *proxy.Proxy, stats *RefreshStats) {
	testDomain := "google.com"
	
	fmt.Println("  持续查询以验证缓存保持新�?..")
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	timeout := time.After(85 * time.Second)
	
	var latencies []int64
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout:
			// 计算平均延迟
			if len(latencies) > 0 {
				var sum int64
				for _, l := range latencies {
					sum += l
				}
				avg := sum / int64(len(latencies))
				stats.postPrefetchLatency.Store(avg)
				fmt.Printf("  �?验证阶段平均延迟: %d μs\n", avg)
			}
			return
		case <-ticker.C:
			latency, ttl := queryDomainWithTTL(ctx, p, testDomain, stats)
			latencies = append(latencies, latency)
			
			status := "缓存命中"
			if latency > 1000 { // > 1ms 可能是缓存未命中
				status = "可能未命�?
			}
			
			fmt.Printf("  验证查询: %s (延迟: %d μs, TTL: %ds, %s)\n", 
				testDomain, latency, ttl, status)
		}
	}
}


// queryDomainWithTTL 执行DNS查询并返回延迟（微秒）和TTL（秒�?
func queryDomainWithTTL(ctx context.Context, p *proxy.Proxy, domain string, stats *RefreshStats) (int64, uint32) {
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
		return latency, 0
	}
	
	if dctx.Res == nil {
		return latency, 0
	}
	
	// 检测缓存命�?
	if dctx.Upstream == nil {
		stats.cacheHits.Add(1)
	} else {
		stats.cacheMisses.Add(1)
	}
	
	// 提取TTL
	ttl := uint32(0)
	if len(dctx.Res.Answer) > 0 {
		ttl = dctx.Res.Answer[0].Header().Ttl
	}
	
	return latency, ttl
}

// queryDomain 执行DNS查询并返回延迟（微秒�?
func queryDomain(ctx context.Context, p *proxy.Proxy, domain string, stats *RefreshStats) int64 {
	latency, _ := queryDomainWithTTL(ctx, p, domain, stats)
	return latency
}

// printFinalReport 打印最终测试报�?
func printFinalReport(stats *RefreshStats) {
	total := stats.totalQueries.Load()
	hits := stats.cacheHits.Load()
	misses := stats.cacheMisses.Load()
	prefetchDetected := stats.prefetchDetected.Load()
	
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100
	}
	
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("🎯 主动刷新测试报告")
	fmt.Println(strings.Repeat("=", 70))
	
	fmt.Println("\n📊 查询统计:")
	fmt.Printf("  总查询数:         %d\n", total)
	fmt.Printf("  缓存命中:         %d (%.2f%%)\n", hits, hitRate)
	fmt.Printf("  缓存未命�?       %d (%.2f%%)\n", misses, float64(misses)/float64(total)*100)
	
	fmt.Println("\n🔄 预取检�?")
	fmt.Printf("  检测到预取次数:   %d\n", prefetchDetected)
	
	postLatency := stats.postPrefetchLatency.Load()
	if postLatency > 0 {
		fmt.Println("\n�?性能指标:")
		fmt.Printf("  验证阶段平均延迟: %d μs (%.2f ms)\n", 
			postLatency, float64(postLatency)/1000)
		
		if postLatency < 1000 {
			fmt.Println("  �?延迟优秀 - 缓存保持新鲜")
		} else if postLatency < 10000 {
			fmt.Println("  ⚠️  延迟一�?- 可能有部分缓存过�?)
		} else {
			fmt.Println("  �?延迟较高 - 缓存可能未被刷新")
		}
	}
	
	fmt.Println("\n�?测试评估:")
	if hitRate >= 95 {
		fmt.Println("  缓存命中�? 优秀 ⭐⭐⭐⭐�?)
	} else if hitRate >= 85 {
		fmt.Println("  缓存命中�? 良好 ⭐⭐⭐⭐")
	} else if hitRate >= 70 {
		fmt.Println("  缓存命中�? 一�?⭐⭐�?)
	} else {
		fmt.Println("  缓存命中�? 需改进 ⭐⭐")
	}
	
	if prefetchDetected > 0 {
		fmt.Println("  预取功能:   正常工作 �?)
	} else {
		fmt.Println("  预取功能:   未检测到明显预取 ⚠️")
	}
	
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("�?测试完成�?)
	fmt.Println(strings.Repeat("=", 70))
}
