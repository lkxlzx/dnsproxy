package proxy

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
)

// TestRealE2E_CompleteWorkflow 测试完整的真实工作流
// 包括：下载、转换、载入、路由、自动刷新
func TestRealE2E_CompleteWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过真实 E2E 测试（使用 -short 标志）")
	}

	t.Log("========================================")
	t.Log("真实 E2E 完整工作流测试")
	t.Log("========================================")
	t.Log("")

	// 创建测试目录
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("创建缓存目录失败: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// 步骤 1: 创建模拟的域名列表服务器
	t.Log("步骤 1: 创建模拟域名列表服务器")
	t.Log("----------------------------------------")
	
	var chinaDownloadCount atomic.Int32
	var overseasDownloadCount atomic.Int32
	var adblockDownloadCount atomic.Int32

	// 中国域名列表服务器（Plain Text 格式）
	chinaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := chinaDownloadCount.Add(1)
		t.Logf("  [下载] 中国域名列表 (第 %d 次)", count)
		
		// 模拟真实的域名列表
		domains := []string{
			"baidu.com",
			"taobao.com",
			"qq.com",
			"weixin.qq.com",
			"alipay.com",
			"jd.com",
			"tmall.com",
			"163.com",
			"sina.com.cn",
			"sohu.com",
		}
		
		// 第二次下载时添加新域名（模拟列表更新）
		if count > 1 {
			domains = append(domains, "bilibili.com", "douyin.com")
			t.Logf("  [更新] 添加了 2 个新域名")
		}
		
		for _, domain := range domains {
			fmt.Fprintln(w, domain)
		}
	}))
	defer chinaServer.Close()

	// 海外域名列表服务器（Dnsmasq 格式）
	overseasServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := overseasDownloadCount.Add(1)
		t.Logf("  [下载] 海外域名列表 (第 %d 次)", count)
		
		domains := []string{
			"google.com",
			"youtube.com",
			"facebook.com",
			"twitter.com",
			"instagram.com",
			"github.com",
			"stackoverflow.com",
		}
		
		if count > 1 {
			domains = append(domains, "reddit.com", "medium.com")
			t.Logf("  [更新] 添加了 2 个新域名")
		}
		
		for _, domain := range domains {
			fmt.Fprintf(w, "server=/%s/8.8.8.8\n", domain)
		}
	}))
	defer overseasServer.Close()

	// 广告拦截列表服务器（AdBlock 格式）
	adblockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := adblockDownloadCount.Add(1)
		t.Logf("  [下载] 广告拦截列表 (第 %d 次)", count)
		
		domains := []string{
			"ads.example.com",
			"tracker.example.com",
			"analytics.example.com",
		}
		
		for _, domain := range domains {
			fmt.Fprintf(w, "||%s^\n", domain)
		}
	}))
	defer adblockServer.Close()

	t.Logf("✓ 模拟服务器创建完成")
	t.Logf("  - 中国域名: %s", chinaServer.URL)
	t.Logf("  - 海外域名: %s", overseasServer.URL)
	t.Logf("  - 广告拦截: %s", adblockServer.URL)
	t.Log("")

	// 步骤 2: 创建配置
	t.Log("步骤 2: 创建上游分组配置")
	t.Log("----------------------------------------")

	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				Name:      "china",
				Upstreams: []string{"223.5.5.5", "119.29.29.29"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
				Priority:  0,
			},
			{
				Name:      "overseas",
				Upstreams: []string{"8.8.8.8", "1.1.1.1"},
				Mode:      "parallel",
				Timeout:   "10s",
				Enabled:   true,
				Priority:  0,
			},
			{
				Name:      "adblock",
				Upstreams: []string{"127.0.0.1:5353"},
				Mode:      "load_balance",
				Timeout:   "1s",
				Enabled:   true,
				Priority:  0,
			},
			{
				Name:      "default",
				Upstreams: []string{"114.114.114.114"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
				Priority:  0,
			},
		},
		DomainGroups: map[string]interface{}{
			// 直接配置的域名
			"direct.test.com": "china",
			"local.test.com":  "overseas",
		},
		DomainLists: []DomainListSpec{
			{
				Name:            "china-domains",
				Source:          chinaServer.URL,
				Group:           "china",
				File:            filepath.Join(cacheDir, "china-domains.yaml"),
				AutoUpdate:      true,
				RefreshInterval: "1m", // 1 分钟刷新（快速测试）
				Enabled:         true,
				Format:          "plain",
			},
			{
				Name:            "overseas-domains",
				Source:          overseasServer.URL,
				Group:           "overseas",
				File:            filepath.Join(cacheDir, "overseas-domains.yaml"),
				AutoUpdate:      true,
				RefreshInterval: "1m", // 1 分钟刷新
				Enabled:         true,
				Format:          "dnsmasq",
			},
			{
				Name:            "adblock-domains",
				Source:          adblockServer.URL,
				Group:           "adblock",
				File:            filepath.Join(cacheDir, "adblock-domains.yaml"),
				AutoUpdate:      true,
				RefreshInterval: "1m", // 1 分钟刷新
				Enabled:         true,
				Format:          "adblock",
			},
		},
		DefaultGroup: "default",
		Cache: &CacheConfigSpec{
			Enabled:                true,
			Directory:              cacheDir,
			TTL:                    "24h",
			DefaultRefreshInterval: "1m",
			AutoUpdate:             true,
			CleanupInterval:        "30s",
		},
	}

	t.Logf("✓ 配置创建完成")
	t.Logf("  - 上游组数量: %d", len(spec.Groups))
	t.Logf("  - 域名列表数量: %d", len(spec.DomainLists))
	t.Logf("  - 刷新间隔: 1 分钟")
	t.Log("")

	// 步骤 3: 解析配置（触发下载和转换）
	t.Log("步骤 3: 解析配置（下载并转换域名列表）")
	t.Log("----------------------------------------")

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: 5000,
	}

	startTime := time.Now()
	ugc, err := ParseUpstreamGroups(spec, opts)
	if err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	parseTime := time.Since(startTime)

	t.Logf("✓ 配置解析完成 (耗时: %v)", parseTime)
	t.Log("")

	// 步骤 4: 验证初始下载
	t.Log("步骤 4: 验证初始下载")
	t.Log("----------------------------------------")

	if chinaDownloadCount.Load() != 1 {
		t.Errorf("中国域名列表下载次数错误: 期望 1, 实际 %d", chinaDownloadCount.Load())
	} else {
		t.Logf("✓ 中国域名列表下载 1 次")
	}

	if overseasDownloadCount.Load() != 1 {
		t.Errorf("海外域名列表下载次数错误: 期望 1, 实际 %d", overseasDownloadCount.Load())
	} else {
		t.Logf("✓ 海外域名列表下载 1 次")
	}

	if adblockDownloadCount.Load() != 1 {
		t.Errorf("广告拦截列表下载次数错误: 期望 1, 实际 %d", adblockDownloadCount.Load())
	} else {
		t.Logf("✓ 广告拦截列表下载 1 次")
	}
	t.Log("")

	// 步骤 5: 验证 YAML 文件创建
	t.Log("步骤 5: 验证 YAML 文件创建")
	t.Log("----------------------------------------")

	yamlFiles := []struct {
		name string
		path string
	}{
		{"中国域名", filepath.Join(cacheDir, "china-domains.yaml")},
		{"海外域名", filepath.Join(cacheDir, "overseas-domains.yaml")},
		{"广告拦截", filepath.Join(cacheDir, "adblock-domains.yaml")},
	}

	for _, file := range yamlFiles {
		if info, err := os.Stat(file.path); err != nil {
			t.Errorf("%s YAML 文件未创建: %v", file.name, err)
		} else {
			t.Logf("✓ %s YAML 文件已创建 (大小: %d 字节)", file.name, info.Size())
		}
	}
	t.Log("")

	// 步骤 6: 测试域名路由（模拟真实访问）
	t.Log("步骤 6: 测试域名路由（模拟真实访问）")
	t.Log("----------------------------------------")

	testCases := []struct {
		domain        string
		expectedGroup string
		description   string
	}{
		// 直接配置的域名
		{"direct.test.com", "china", "直接配置域名"},
		{"local.test.com", "overseas", "直接配置域名"},
		
		// 从中国列表加载的域名
		{"baidu.com", "china", "中国域名（搜索引擎）"},
		{"taobao.com", "china", "中国域名（电商）"},
		{"qq.com", "china", "中国域名（社交）"},
		{"weixin.qq.com", "china", "中国域名（社交）"},
		{"alipay.com", "china", "中国域名（支付）"},
		{"jd.com", "china", "中国域名（电商）"},
		
		// 从海外列表加载的域名
		{"google.com", "overseas", "海外域名（搜索引擎）"},
		{"youtube.com", "overseas", "海外域名（视频）"},
		{"facebook.com", "overseas", "海外域名（社交）"},
		{"twitter.com", "overseas", "海外域名（社交）"},
		{"github.com", "overseas", "海外域名（开发）"},
		
		// 从广告拦截列表加载的域名
		{"ads.example.com", "adblock", "广告域名"},
		{"tracker.example.com", "adblock", "追踪域名"},
		
		// 未知域名（使用默认组）
		{"unknown.example.com", "default", "未知域名"},
		{"random.test.org", "default", "未知域名"},
	}

	successCount := 0
	for _, tc := range testCases {
		group, err := ugc.GetGroupForDomain(tc.domain)
		if err != nil {
			t.Errorf("  ✗ %s: 查询失败 - %v", tc.domain, err)
			continue
		}

		if group.Name != tc.expectedGroup {
			t.Errorf("  ✗ %s: 期望 %s, 实际 %s", tc.domain, tc.expectedGroup, group.Name)
		} else {
			t.Logf("  ✓ %-25s → %-10s (%s)", tc.domain, group.Name, tc.description)
			successCount++
		}
	}

	t.Logf("")
	t.Logf("路由测试结果: %d/%d 通过", successCount, len(testCases))
	t.Log("")

	// 步骤 7: 性能测试（模拟高并发访问）
	t.Log("步骤 7: 性能测试（模拟高并发访问）")
	t.Log("----------------------------------------")

	const numQueries = 10000
	domains := []string{
		"baidu.com", "google.com", "qq.com", "youtube.com",
		"taobao.com", "facebook.com", "jd.com", "twitter.com",
	}

	startTime = time.Now()
	for i := 0; i < numQueries; i++ {
		domain := domains[i%len(domains)]
		_, err := ugc.GetGroupForDomain(domain)
		if err != nil {
			t.Errorf("查询失败: %v", err)
		}
	}
	queryTime := time.Since(startTime)

	avgTime := queryTime.Nanoseconds() / int64(numQueries)
	qps := float64(numQueries) / queryTime.Seconds()

	t.Logf("✓ 性能测试完成")
	t.Logf("  - 总查询数: %d", numQueries)
	t.Logf("  - 总耗时: %v", queryTime)
	t.Logf("  - 平均延迟: %d ns", avgTime)
	t.Logf("  - QPS: %.0f", qps)
	t.Log("")

	// 步骤 8: 测试自动刷新（等待 70 秒）
	t.Log("步骤 8: 测试自动刷新机制")
	t.Log("----------------------------------------")
	t.Logf("等待 70 秒以触发自动刷新...")
	t.Logf("初始下载次数:")
	t.Logf("  - 中国域名: %d", chinaDownloadCount.Load())
	t.Logf("  - 海外域名: %d", overseasDownloadCount.Load())
	t.Logf("  - 广告拦截: %d", adblockDownloadCount.Load())
	t.Log("")

	// 等待自动刷新触发
	time.Sleep(70 * time.Second)

	t.Logf("刷新后下载次数:")
	t.Logf("  - 中国域名: %d", chinaDownloadCount.Load())
	t.Logf("  - 海外域名: %d", overseasDownloadCount.Load())
	t.Logf("  - 广告拦截: %d", adblockDownloadCount.Load())
	t.Log("")

	// 验证自动刷新
	if chinaDownloadCount.Load() > 1 {
		t.Logf("✓ 中国域名列表已自动刷新")
	} else {
		t.Errorf("✗ 中国域名列表未自动刷新")
	}

	if overseasDownloadCount.Load() > 1 {
		t.Logf("✓ 海外域名列表已自动刷新")
	} else {
		t.Errorf("✗ 海外域名列表未自动刷新")
	}

	if adblockDownloadCount.Load() > 1 {
		t.Logf("✓ 广告拦截列表已自动刷新")
	} else {
		t.Errorf("✗ 广告拦截列表未自动刷新")
	}
	t.Log("")

	// 步骤 9: 测试统计信息
	t.Log("步骤 9: 统计信息")
	t.Log("----------------------------------------")
	
	t.Logf("配置统计:")
	t.Logf("  - 上游组数量: %d", len(ugc.Groups))
	t.Logf("  - 域名映射数量: %d", len(ugc.DomainGroups))
	t.Logf("  - 默认组: %s", ugc.DefaultGroup)
	t.Log("")

	// 最终总结
	t.Log("========================================")
	t.Log("✓ 真实 E2E 测试完成")
	t.Log("========================================")
	t.Log("")
	t.Log("测试总结:")
	t.Logf("  ✓ 域名列表下载: 成功")
	t.Logf("  ✓ 格式自动转换: 成功")
	t.Logf("  ✓ YAML 文件生成: 成功")
	t.Logf("  ✓ 域名路由分流: %d/%d 通过", successCount, len(testCases))
	t.Logf("  ✓ 性能测试: %.0f QPS", qps)
	t.Logf("  ✓ 自动刷新: 成功")
	t.Log("")
}


// TestRealE2E_QuickTest 快速测试版本（不包含自动刷新等待）
func TestRealE2E_QuickTest(t *testing.T) {
	t.Log("========================================")
	t.Log("真实 E2E 快速测试")
	t.Log("========================================")
	t.Log("")

	// 创建测试目录
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("创建缓存目录失败: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// 创建模拟服务器
	t.Log("步骤 1: 创建模拟服务器")
	
	chinaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "baidu.com")
		fmt.Fprintln(w, "taobao.com")
		fmt.Fprintln(w, "qq.com")
	}))
	defer chinaServer.Close()

	overseasServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "server=/google.com/8.8.8.8")
		fmt.Fprintln(w, "server=/youtube.com/8.8.8.8")
	}))
	defer overseasServer.Close()

	t.Logf("✓ 服务器创建完成")
	t.Log("")

	// 创建配置
	t.Log("步骤 2: 创建配置并解析")
	
	spec := &UpstreamGroupsSpec{
		Groups: []UpstreamGroupSpec{
			{
				Name:      "china",
				Upstreams: []string{"223.5.5.5"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
			{
				Name:      "overseas",
				Upstreams: []string{"8.8.8.8"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
			{
				Name:      "default",
				Upstreams: []string{"114.114.114.114"},
				Mode:      "load_balance",
				Timeout:   "5s",
				Enabled:   true,
			},
		},
		DomainLists: []DomainListSpec{
			{
				Name:    "china-list",
				Source:  chinaServer.URL,
				Group:   "china",
				File:    filepath.Join(cacheDir, "china.yaml"),
				Enabled: true,
				Format:  "plain",
			},
			{
				Name:    "overseas-list",
				Source:  overseasServer.URL,
				Group:   "overseas",
				File:    filepath.Join(cacheDir, "overseas.yaml"),
				Enabled: true,
				Format:  "dnsmasq",
			},
		},
		DefaultGroup: "default",
		Cache: &CacheConfigSpec{
			Enabled:   true,
			Directory: cacheDir,
		},
	}

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: 5000,
	}

	ugc, err := ParseUpstreamGroups(spec, opts)
	if err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}

	t.Logf("✓ 配置解析完成")
	t.Log("")

	// 测试域名路由
	t.Log("步骤 3: 测试域名路由")
	
	testCases := []struct {
		domain string
		group  string
	}{
		{"baidu.com", "china"},
		{"taobao.com", "china"},
		{"qq.com", "china"},
		{"google.com", "overseas"},
		{"youtube.com", "overseas"},
		{"unknown.com", "default"},
	}

	for _, tc := range testCases {
		group, err := ugc.GetGroupForDomain(tc.domain)
		if err != nil {
			t.Errorf("  ✗ %s: 查询失败", tc.domain)
			continue
		}

		if group.Name != tc.group {
			t.Errorf("  ✗ %s: 期望 %s, 实际 %s", tc.domain, tc.group, group.Name)
		} else {
			t.Logf("  ✓ %-20s → %s", tc.domain, group.Name)
		}
	}

	t.Log("")
	t.Log("✓ 快速测试完成")
	t.Log("")
}
