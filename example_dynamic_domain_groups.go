package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
)

// 演示如何使用动态域名分组功能
// 支持实时启用/禁用规则，无需重启服务
func main() {
	// 创建域名分组配置
	domainGroups := []proxy.DomainGroupConfig{
		{
			GroupName:        "china-domains",
			DomainFile:       "china_domains.txt",
			DomainFileFormat: proxy.FormatPlain,
			Upstreams:        []string{"223.5.5.5", "119.29.29.29"}, // 国内DNS
			SubdomainsOnly:   false,
			Enabled:          true,
		},
		{
			GroupName:        "blocked-ads",
			DomainFile:       "ad_domains.txt",
			DomainFileFormat: proxy.FormatPlain,
			Upstreams:        []string{"0.0.0.0"}, // 广告域名返回0.0.0.0
			SubdomainsOnly:   false,
			Enabled:          true,
		},
		{
			GroupName:        "custom-routing",
			DomainFile:       "custom_domains.txt",
			DomainFileFormat: proxy.FormatPlain,
			Upstreams:        []string{"1.1.1.1", "8.8.8.8"},
			SubdomainsOnly:   false,
			Enabled:          true,
		},
	}

	// 创建默认上游配置
	defaultUpstreams, err := proxy.ParseUpstreamsConfig(
		[]string{"https://dns.google/dns-query"},
		&upstream.Options{},
	)
	if err != nil {
		log.Fatalf("Failed to parse default upstreams: %v", err)
	}

	// 创建代理配置
	config := &proxy.Config{
		UDPListenAddr:   []*net.UDPAddr{{IP: net.ParseIP("127.0.0.1"), Port: 5353}},
		TCPListenAddr:   []*net.TCPAddr{{IP: net.ParseIP("127.0.0.1"), Port: 5353}},
		UpstreamConfig:  defaultUpstreams,
		DomainGroups:    domainGroups, // 设置域名分组
		CacheEnabled:    true,
		CacheSizeBytes:  64 * 1024 * 1024,
	}

	// 创建代理实例
	dnsProxy, err := proxy.New(config)
	if err != nil {
		log.Fatalf("Failed to create proxy: %v", err)
	}

	// 启动代理
	ctx := context.Background()
	if err := dnsProxy.Start(ctx); err != nil {
		log.Fatalf("Failed to start proxy: %v", err)
	}

	log.Println("DNS Proxy started on 127.0.0.1:5353")
	log.Println("Domain groups loaded:")
	
	// 显示所有域名组状态
	printDomainGroups(dnsProxy)

	// 启动管理协程，演示动态管理
	go manageDomainGroups(dnsProxy)

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	if err := dnsProxy.Shutdown(ctx); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}
}

// printDomainGroups 打印所有域名组的状态
func printDomainGroups(p *proxy.Proxy) {
	groups := p.GetDomainGroups()
	for _, g := range groups {
		status := "enabled"
		if !g.Enabled {
			status = "disabled"
		}
		fmt.Printf("  - %s: %d domains, %d upstreams (%s)\n",
			g.GroupName, g.DomainCount, g.UpstreamCount, status)
	}
}

// manageDomainGroups 演示如何动态管理域名组
func manageDomainGroups(p *proxy.Proxy) {
	// 等待10秒后禁用广告拦截
	time.Sleep(10 * time.Second)
	log.Println("Disabling ad blocking...")
	if err := p.DisableDomainGroup("blocked-ads"); err != nil {
		log.Printf("Failed to disable group: %v", err)
	} else {
		log.Println("Ad blocking disabled")
		printDomainGroups(p)
	}

	// 再等10秒后重新启用
	time.Sleep(10 * time.Second)
	log.Println("Re-enabling ad blocking...")
	if err := p.EnableDomainGroup("blocked-ads"); err != nil {
		log.Printf("Failed to enable group: %v", err)
	} else {
		log.Println("Ad blocking re-enabled")
		printDomainGroups(p)
	}

	// 再等10秒后重新加载自定义路由
	time.Sleep(10 * time.Second)
	log.Println("Reloading custom routing domains...")
	if err := p.ReloadDomainGroup("custom-routing"); err != nil {
		log.Printf("Failed to reload group: %v", err)
	} else {
		log.Println("Custom routing reloaded")
		printDomainGroups(p)
	}
}
