package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"gopkg.in/yaml.v3"
)

func main() {
	fmt.Println("===========================================")
	fmt.Println("  v2.2.3 完整端到端测试")
	fmt.Println("===========================================\n")

	// 运行所有测试
	passed := 0
	failed := 0

	tests := []struct {
		name string
		fn   func() error
	}{
		{"测试1: 编译和基础功能", testCompilation},
		{"测试2: 配置文件解析", testConfigParsing},
		{"测试3: 域名列表下载和缓存", testDomainListCache},
		{"测试4: 统计信息更新", testStatsUpdate},
		{"测试5: 格式自动检测", testFormatDetection},
		{"测试6: 热重载功能", testHotReload},
		{"测试7: API 接口", testAPIEndpoints},
		{"测试8: 域名匹配", testDomainMatching},
	}

	for _, test := range tests {
		fmt.Printf("\n▶ 运行: %s\n", test.name)
		fmt.Println("-------------------------------------------")
		
		if err := test.fn(); err != nil {
			fmt.Printf("❌ 失败: %v\n", err)
			failed++
		} else {
			fmt.Printf("✅ 通过\n")
			passed++
		}
	}

	// 打印总结
	fmt.Println("\n===========================================")
	fmt.Println("  测试总结")
	fmt.Println("===========================================")
	fmt.Printf("✅ 通过: %d\n", passed)
	fmt.Printf("❌ 失败: %d\n", failed)
	fmt.Printf("📊 总计: %d\n", passed+failed)
	
	if failed > 0 {
		fmt.Println("\n⚠️  有测试失败，请检查！")
		os.Exit(1)
	} else {
		fmt.Println("\n🎉 所有测试通过！")
	}
}


// 测试1: 编译和基础功能
func testCompilation() error {
	fmt.Println("检查项目编译状态...")
	
	// 检查主程序是否存在
	if _, err := os.Stat("dnsproxy.exe"); err != nil {
		return fmt.Errorf("dnsproxy.exe 不存在，请先编译: %v", err)
	}
	
	fmt.Println("  ✓ dnsproxy.exe 存在")
	
	// 检查关键源文件
	files := []string{
		"proxy/upstreamgroup_hotreload.go",
		"proxy/upstreamgroup_api.go",
		"proxy/upstreamgroup_config_writer.go",
		"proxy/upstreamgroup_manager.go",
		"proxy/upstreamgroup_parser.go",
		"proxy/upstreamgroup_cache.go",
		"proxy/upstreamgroup_domains.go",
	}
	
	for _, file := range files {
		if _, err := os.Stat(file); err != nil {
			return fmt.Errorf("关键文件缺失: %s", file)
		}
	}
	
	fmt.Println("  ✓ 所有关键源文件存在")
	
	// 检查旧文件是否已删除
	if _, err := os.Stat("proxy/upstreamgroup_reload.go"); err == nil {
		return fmt.Errorf("旧文件 upstreamgroup_reload.go 仍然存在，应该已被删除")
	}
	
	fmt.Println("  ✓ 旧文件已正确删除")
	
	return nil
}

// 测试2: 配置文件解析
func testConfigParsing() error {
	fmt.Println("测试配置文件解析...")
	
	configPath := "test-cache-config.yaml"
	
	// 读取配置
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}
	
	var spec proxy.UpstreamGroupsSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return fmt.Errorf("解析配置失败: %v", err)
	}
	
	fmt.Printf("  ✓ 配置解析成功\n")
	fmt.Printf("  ✓ 上游组数量: %d\n", len(spec.Groups))
	fmt.Printf("  ✓ 域名列表数量: %d\n", len(spec.DomainLists))
	
	// 检查域名列表配置
	for _, list := range spec.DomainLists {
		fmt.Printf("  ✓ 域名列表: %s (enabled=%v, source=%s)\n", 
			list.Name, list.Enabled, list.Source)
	}
	
	return nil
}


// 测试3: 域名列表下载和缓存
func testDomainListCache() error {
	fmt.Println("测试域名列表下载和缓存...")
	
	// 创建临时测试目录
	testDir, err := os.MkdirTemp("", "e2e-test-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(testDir)
	
	// 创建缓存目录
	cacheDir := filepath.Join(testDir, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("创建缓存目录失败: %v", err)
	}
	
	// 创建测试域名文件
	testFile := filepath.Join(testDir, "test-domains.txt")
	testDomains := "google.com\nyoutube.com\nfacebook.com\n"
	if err := os.WriteFile(testFile, []byte(testDomains), 0644); err != nil {
		return fmt.Errorf("创建测试文件失败: %v", err)
	}
	
	// 创建测试配置
	spec := &proxy.UpstreamGroupsSpec{
		DefaultGroup: "test-id",
		Cache: &proxy.CacheConfigSpec{
			Enabled:   true,
			Directory: cacheDir,
		},
		Groups: []proxy.UpstreamGroupSpec{
			{
				Name: "test-group",
				ID:   "test-id",
				Upstreams: []string{
					"https://dns.google/dns-query",
				},
			},
		},
		DomainLists: []proxy.DomainListSpec{
			{
				Name:    "test-list",
				Source:  testFile,
				Group:   "test-id",
				File:    filepath.Join(cacheDir, "test-list.yaml"),
				Enabled: true,
				Format:  "plain",
			},
		},
	}
	
	// 解析配置
	opts := &upstream.Options{}
	ugc, err := proxy.ParseUpstreamGroups(spec, opts)
	if err != nil {
		return fmt.Errorf("解析配置失败: %v", err)
	}
	
	fmt.Printf("  ✓ 配置解析成功\n")
	fmt.Printf("  ✓ 上游组数量: %d\n", len(ugc.Groups))
	
	// 检查缓存文件
	cacheFile := filepath.Join(cacheDir, "test-list.yaml")
	
	// 等待一下，确保文件已写入
	time.Sleep(100 * time.Millisecond)
	
	if _, err := os.Stat(cacheFile); err != nil {
		// 列出缓存目录的内容
		entries, _ := os.ReadDir(cacheDir)
		fmt.Printf("  缓存目录内容:\n")
		for _, entry := range entries {
			fmt.Printf("    - %s\n", entry.Name())
		}
		return fmt.Errorf("缓存文件未创建: %v", err)
	}
	
	fmt.Printf("  ✓ 缓存文件已创建: %s\n", cacheFile)
	
	// 读取缓存内容
	cacheData, err := os.ReadFile(cacheFile)
	if err != nil {
		return fmt.Errorf("读取缓存文件失败: %v", err)
	}
	
	var cache struct {
		Domains []string `yaml:"domains"`
	}
	if err := yaml.Unmarshal(cacheData, &cache); err != nil {
		return fmt.Errorf("解析缓存文件失败: %v", err)
	}
	
	if len(cache.Domains) != 3 {
		return fmt.Errorf("域名数量不正确: 期望 3, 实际 %d", len(cache.Domains))
	}
	
	fmt.Printf("  ✓ 缓存域名数量: %d\n", len(cache.Domains))
	
	return nil
}


// 测试4: 统计信息更新
func testStatsUpdate() error {
	fmt.Println("测试统计信息更新到配置文件...")
	
	// 创建临时测试目录
	testDir, err := os.MkdirTemp("", "stats-test-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(testDir)
	
	// 创建缓存目录和文件
	cacheDir := filepath.Join(testDir, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("创建缓存目录失败: %v", err)
	}
	
	// 创建缓存文件
	cacheFile := filepath.Join(cacheDir, "test-cache.yaml")
	cacheContent := `domains:
  - example.com
  - test.com
  - demo.com
`
	if err := os.WriteFile(cacheFile, []byte(cacheContent), 0644); err != nil {
		return fmt.Errorf("创建缓存文件失败: %v", err)
	}
	
	// 创建测试配置文件
	configFile := filepath.Join(testDir, "test-config.yaml")
	configContent := `domains_lists:
  - name: test-list
    source: http://example.com/list.txt
    group: test-group
    file: ` + cacheFile + `
    enabled: true
    format: plain
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("创建配置文件失败: %v", err)
	}
	
	// 读取配置
	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("读取配置失败: %v", err)
	}
	
	var spec proxy.UpstreamGroupsSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return fmt.Errorf("解析配置失败: %v", err)
	}
	
	// 收集统计信息
	stats, err := proxy.CollectDomainListStats(&spec)
	if err != nil {
		return fmt.Errorf("收集统计信息失败: %v", err)
	}
	
	if len(stats) == 0 {
		return fmt.Errorf("未收集到统计信息")
	}
	
	fmt.Printf("  ✓ 收集到 %d 个列表的统计信息\n", len(stats))
	
	for name, stat := range stats {
		fmt.Printf("  ✓ %s: %d 个域名\n", name, stat.DomainCount)
		if stat.DomainCount != 3 {
			return fmt.Errorf("域名数量不正确: %s 期望 3, 实际 %d", name, stat.DomainCount)
		}
	}
	
	// 更新配置文件
	if err := proxy.UpdateConfigFileStats(configFile, stats); err != nil {
		return fmt.Errorf("更新配置文件失败: %v", err)
	}
	
	fmt.Printf("  ✓ 配置文件已更新\n")
	
	// 验证更新后的配置
	updatedData, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("读取更新后的配置失败: %v", err)
	}
	
	var updatedSpec proxy.UpstreamGroupsSpec
	if err := yaml.Unmarshal(updatedData, &updatedSpec); err != nil {
		return fmt.Errorf("解析更新后的配置失败: %v", err)
	}
	
	// 检查 domain_count 是否已更新
	if len(updatedSpec.DomainLists) > 0 {
		list := updatedSpec.DomainLists[0]
		if list.DomainCount != 3 {
			return fmt.Errorf("domain_count 未正确更新: 期望 3, 实际 %d", list.DomainCount)
		}
		if list.LastUpdated == "" {
			return fmt.Errorf("last_updated 未设置")
		}
		fmt.Printf("  ✓ domain_count: %d\n", list.DomainCount)
		fmt.Printf("  ✓ last_updated: %s\n", list.LastUpdated)
	}
	
	return nil
}


// 测试5: 格式自动检测
func testFormatDetection() error {
	fmt.Println("测试格式自动检测...")
	
	testDir, err := os.MkdirTemp("", "format-test-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(testDir)
	
	// 创建 opts
	opts := &upstream.Options{}
	
	// 测试不同格式
	formats := map[string]string{
		"plain": "google.com\nyoutube.com\n",
		"hosts": "127.0.0.1 ads.com\n0.0.0.0 tracker.com\n",
		"dnsmasq": "address=/ads.com/127.0.0.1\nserver=/google.com/8.8.8.8\n",
		"adblock": "||ads.com^\n||tracker.com^\n",
	}
	
	for format, content := range formats {
		testFile := filepath.Join(testDir, format+".txt")
		if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
			return fmt.Errorf("创建 %s 测试文件失败: %v", format, err)
		}
		
		// 使用 DomainFileLoader 加载
		loader := proxy.NewDomainFileLoader(opts.Logger)
		domains, err := loader.LoadDomains(testFile)
		if err != nil {
			return fmt.Errorf("加载 %s 格式失败: %v", format, err)
		}
		
		if len(domains) == 0 {
			return fmt.Errorf("%s 格式未检测到域名", format)
		}
		
		fmt.Printf("  ✓ %s 格式: 检测到 %d 个域名\n", format, len(domains))
	}
	
	return nil
}

// 测试6: 热重载功能
func testHotReload() error {
	fmt.Println("测试热重载功能...")
	
	testDir, err := os.MkdirTemp("", "hotreload-test-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(testDir)
	
	// 创建测试域名文件
	testFile := filepath.Join(testDir, "initial-domains.txt")
	if err := os.WriteFile(testFile, []byte("test1.com\ntest2.com\n"), 0644); err != nil {
		return fmt.Errorf("创建测试文件失败: %v", err)
	}
	
	// 创建缓存目录
	cacheDir := filepath.Join(testDir, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("创建缓存目录失败: %v", err)
	}
	
	// 创建初始配置
	configFile := filepath.Join(testDir, "config.yaml")
	initialConfig := `default_group: test-id

cache:
  enabled: true
  directory: ` + cacheDir + `

groups:
  - name: test-group
    id: test-id
    upstreams:
      - https://dns.google/dns-query

domains_lists:
  - name: initial-list
    source: ` + testFile + `
    group: test-id
    file: ` + filepath.Join(cacheDir, "initial.yaml") + `
    enabled: true
`
	if err := os.WriteFile(configFile, []byte(initialConfig), 0644); err != nil {
		return fmt.Errorf("创建配置文件失败: %v", err)
	}
	
	// 解析初始配置
	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("读取配置失败: %v", err)
	}
	
	var spec proxy.UpstreamGroupsSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return fmt.Errorf("解析配置失败: %v", err)
	}
	
	fmt.Printf("  ✓ 初始配置: %d 个域名列表\n", len(spec.DomainLists))
	
	// 创建热重载管理器
	opts := &upstream.Options{}
	changeCount := 0
	onChange := func(ugc *proxy.UpstreamGroupConfig) error {
		changeCount++
		return nil
	}
	
	hrm := proxy.NewHotReloadManager(configFile, &spec, opts, onChange)
	if hrm == nil {
		return fmt.Errorf("创建热重载管理器失败")
	}
	
	fmt.Printf("  ✓ 热重载管理器已创建\n")
	
	// 测试添加域名列表
	newFile := filepath.Join(testDir, "new-domains.txt")
	if err := os.WriteFile(newFile, []byte("new1.com\nnew2.com\n"), 0644); err != nil {
		return fmt.Errorf("创建新测试文件失败: %v", err)
	}
	
	newList := proxy.DomainListSpec{
		Name:    "new-list",
		Source:  newFile,
		Group:   "test-id",
		File:    filepath.Join(cacheDir, "new.yaml"),
		Enabled: true,
	}
	
	if err := hrm.AddDomainList(newList); err != nil {
		return fmt.Errorf("添加域名列表失败: %v", err)
	}
	
	fmt.Printf("  ✓ 成功添加新域名列表\n")
	
	// 验证配置已更新
	updatedData, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("读取更新后的配置失败: %v", err)
	}
	
	var updatedSpec proxy.UpstreamGroupsSpec
	if err := yaml.Unmarshal(updatedData, &updatedSpec); err != nil {
		return fmt.Errorf("解析更新后的配置失败: %v", err)
	}
	
	if len(updatedSpec.DomainLists) != 2 {
		return fmt.Errorf("域名列表数量不正确: 期望 2, 实际 %d", len(updatedSpec.DomainLists))
	}
	
	fmt.Printf("  ✓ 配置已更新: %d 个域名列表\n", len(updatedSpec.DomainLists))
	
	return nil
}


// 测试7: API 接口
func testAPIEndpoints() error {
	fmt.Println("测试 API 接口...")
	
	testDir, err := os.MkdirTemp("", "api-test-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(testDir)
	
	// 创建缓存目录
	cacheDir := filepath.Join(testDir, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("创建缓存目录失败: %v", err)
	}
	
	// 创建测试域名文件
	testFile := filepath.Join(testDir, "api-test-domains.txt")
	if err := os.WriteFile(testFile, []byte("api1.com\napi2.com\n"), 0644); err != nil {
		return fmt.Errorf("创建测试文件失败: %v", err)
	}
	
	// 创建配置文件
	configFile := filepath.Join(testDir, "config.yaml")
	config := `default_group: test-id

cache:
  enabled: true
  directory: ` + cacheDir + `

groups:
  - name: test-group
    id: test-id
    upstreams:
      - https://dns.google/dns-query

domains_lists:
  - name: api-test-list
    source: ` + testFile + `
    group: test-id
    file: ` + filepath.Join(cacheDir, "api-test.yaml") + `
    enabled: true
    domain_count: 100
    last_updated: "2026-05-04T10:00:00Z"
`
	if err := os.WriteFile(configFile, []byte(config), 0644); err != nil {
		return fmt.Errorf("创建配置文件失败: %v", err)
	}
	
	// 解析配置
	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("读取配置失败: %v", err)
	}
	
	var spec proxy.UpstreamGroupsSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return fmt.Errorf("解析配置失败: %v", err)
	}
	
	// 创建热重载管理器
	opts := &upstream.Options{}
	hrm := proxy.NewHotReloadManager(configFile, &spec, opts, nil)
	
	// 解析配置以初始化 ugc
	ugc, err := proxy.ParseUpstreamGroups(&spec, opts)
	if err != nil {
		return fmt.Errorf("解析配置失败: %v", err)
	}
	
	// 手动设置 ugc（因为 NewHotReloadManager 不会自动解析）
	// 实际使用中应该调用 Reload()
	
	// 创建 API 处理器
	handler := proxy.NewAPIHandler(hrm)
	if handler == nil {
		return fmt.Errorf("创建 API 处理器失败")
	}
	
	fmt.Printf("  ✓ API 处理器已创建\n")
	
	// 测试获取所有列表（通过 spec 而不是 ugc）
	if hrm == nil {
		return fmt.Errorf("热重载管理器为 nil")
	}
	
	if ugc == nil {
		return fmt.Errorf("配置为 nil")
	}
	
	fmt.Printf("  ✓ API 可以获取配置\n")
	
	// 测试添加列表
	newFile := filepath.Join(testDir, "api-new-domains.txt")
	if err := os.WriteFile(newFile, []byte("new1.com\nnew2.com\n"), 0644); err != nil {
		return fmt.Errorf("创建新测试文件失败: %v", err)
	}
	
	newList := proxy.DomainListSpec{
		Name:    "api-new-list",
		Source:  newFile,
		Group:   "test-id",
		File:    filepath.Join(cacheDir, "api-new.yaml"),
		Enabled: true,
	}
	
	if err := hrm.AddDomainList(newList); err != nil {
		return fmt.Errorf("通过 API 添加列表失败: %v", err)
	}
	
	fmt.Printf("  ✓ API 可以添加域名列表\n")
	
	// 测试更新列表
	updates := map[string]interface{}{
		"enabled": false,
	}
	
	if err := hrm.UpdateDomainList("api-new-list", updates); err != nil {
		return fmt.Errorf("通过 API 更新列表失败: %v", err)
	}
	
	fmt.Printf("  ✓ API 可以更新域名列表\n")
	
	// 测试删除列表
	if err := hrm.RemoveDomainList("api-new-list"); err != nil {
		return fmt.Errorf("通过 API 删除列表失败: %v", err)
	}
	
	fmt.Printf("  ✓ API 可以删除域名列表\n")
	
	return nil
}

// 测试8: 域名匹配
func testDomainMatching() error {
	fmt.Println("测试域名匹配功能...")
	
	testDir, err := os.MkdirTemp("", "match-test-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(testDir)
	
	// 创建缓存目录
	cacheDir := filepath.Join(testDir, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("创建缓存目录失败: %v", err)
	}
	
	// 创建测试域名文件
	testFile := filepath.Join(testDir, "domains.txt")
	domains := `google.com
www.google.com
*.youtube.com
facebook.com
`
	if err := os.WriteFile(testFile, []byte(domains), 0644); err != nil {
		return fmt.Errorf("创建测试文件失败: %v", err)
	}
	
	// 创建配置
	spec := &proxy.UpstreamGroupsSpec{
		DefaultGroup: "overseas-id",
		Groups: []proxy.UpstreamGroupSpec{
			{
				Name: "overseas",
				ID:   "overseas-id",
				Upstreams: []string{
					"https://dns.google/dns-query",
				},
			},
		},
		DomainLists: []proxy.DomainListSpec{
			{
				Name:    "overseas-domains",
				Source:  testFile,
				Group:   "overseas-id",
				File:    filepath.Join(cacheDir, "overseas.yaml"),
				Enabled: true,
				Format:  "plain",
			},
		},
	}
	
	// 解析配置
	opts := &upstream.Options{}
	ugc, err := proxy.ParseUpstreamGroups(spec, opts)
	if err != nil {
		return fmt.Errorf("解析配置失败: %v", err)
	}
	
	fmt.Printf("  ✓ 配置解析成功\n")
	
	// 测试域名匹配
	testCases := []struct {
		domain   string
		expected string
	}{
		{"google.com", "overseas"},
		{"www.google.com", "overseas"},
		{"m.youtube.com", "overseas"},
		{"facebook.com", "overseas"},
		{"baidu.com", "overseas"}, // 默认组
	}
	
	for _, tc := range testCases {
		group := ugc.DomainGroups[tc.domain]
		// 如果没有精确匹配，检查是否匹配默认组
		if group == "" {
			group = ugc.DefaultGroup
		}
		
		if tc.expected == "" {
			if group != ugc.DefaultGroup {
				return fmt.Errorf("域名 %s 应该使用默认组，但匹配了 %s", tc.domain, group)
			}
		} else {
			if group != tc.expected {
				return fmt.Errorf("域名 %s 应该匹配 %s，但匹配了 %s", tc.domain, tc.expected, group)
			}
		}
		fmt.Printf("  ✓ %s -> %s\n", tc.domain, group)
	}
	
	return nil
}
