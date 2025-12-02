package main

import (
	"log"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
)

// 主动缓存刷新示例
func main() {
	// 创建上游配置
	upstreamConfig, err := proxy.ParseUpstreamsConfig(
		[]string{"8.8.8.8:53"},
		&upstream.Options{},
	)
	if err != nil {
		log.Fatalf("Failed to parse upstreams: %v", err)
	}

	// 配置代理
	dnsProxy := &proxy.Proxy{
		Config: proxy.Config{
			// 启用缓存
			CacheEnabled:    true,
			CacheSizeBytes:  64 * 1024,
			CacheOptimistic: true,

			// 主动刷新配置（毫秒）
			CacheProactiveRefreshTime: 30000, // TTL 到期前 30 秒刷新

			// 冷却机制配置
			CacheProactiveCooldownPeriod:    1800, // 30 分钟（秒）
			CacheProactiveCooldownThreshold: 3,    // 至少 3 次请求

			// 上游服务器
			UpstreamConfig: upstreamConfig,
		},
	}

	// 启动代理
	err = dnsProxy.Start(nil)
	if err != nil {
		log.Fatalf("Failed to start proxy: %v", err)
	}
	defer dnsProxy.Shutdown(nil)

	log.Println("DNS Proxy started with proactive cache refresh")
	log.Println("- Refresh time: 30 seconds before TTL expiration")
	log.Println("- Cooldown period: 30 minutes")
	log.Println("- Cooldown threshold: 3 requests")

	// 保持运行
	time.Sleep(time.Hour)
}
