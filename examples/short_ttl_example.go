package main

import (
	"log"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
)

// 极短 TTL 场景的主动缓存刷新示例
// 适用于 TTL 为 1-5 秒的域名
func main() {
	// 创建上游配置
	upstreamConfig, err := proxy.ParseUpstreamsConfig(
		[]string{"8.8.8.8:53"},
		&upstream.Options{},
	)
	if err != nil {
		log.Fatalf("Failed to parse upstreams: %v", err)
	}

	// 配置代理 - 针对极短 TTL 优化
	dnsProxy := &proxy.Proxy{
		Config: proxy.Config{
			// 启用缓存
			CacheEnabled:    true,
			CacheSizeBytes:  64 * 1024,
			CacheOptimistic: true,

			// 极短 TTL 的主动刷新配置
			CacheProactiveRefreshTime: 500, // TTL 到期前 500ms 刷新

			// 冷却机制配置 - 更严格的阈值
			CacheProactiveCooldownPeriod:    300, // 5 分钟
			CacheProactiveCooldownThreshold: 5,   // 至少 5 次请求

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

	log.Println("DNS Proxy started for short TTL domains")
	log.Println("Configuration:")
	log.Println("- Refresh time: 500ms before TTL expiration")
	log.Println("- Suitable for TTL >= 1 second")
	log.Println("- Cooldown period: 5 minutes")
	log.Println("- Cooldown threshold: 5 requests")
	log.Println("")
	log.Println("Examples:")
	log.Println("- TTL = 5s → refresh at 4.5s")
	log.Println("- TTL = 2s → refresh at 1.5s")
	log.Println("- TTL = 1s → refresh at 0.5s")

	// 保持运行
	time.Sleep(time.Hour)
}
