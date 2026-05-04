package main

import (
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"gopkg.in/yaml.v3"
)

// Config represents the configuration
type Config struct {
	UpstreamGroups []proxy.UpstreamGroupSpec `yaml:"upstream_groups"`
	DomainGroups   map[string]interface{}    `yaml:"domain_groups"`
	DomainsLists   []proxy.DomainListSpec    `yaml:"domains_lists"`
	DefaultGroup   string                    `yaml:"default_group"`
	Cache          *proxy.CacheConfigSpec    `yaml:"cache"`
}

func main() {
	configPath := "test-cache-config.yaml"

	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║          DNSProxy 热重载功能测试                           ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Load initial configuration
	fmt.Println("[1/4] 📖 加载初始配置...")
	config, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	spec := &proxy.UpstreamGroupsSpec{
		Groups:       config.UpstreamGroups,
		DomainGroups: config.DomainGroups,
		DomainLists:  config.DomainsLists,
		DefaultGroup: config.DefaultGroup,
		Cache:        config.Cache,
	}

	fmt.Printf("✓ 初始配置加载成功\n")
	fmt.Printf("  - 域名列表: %d 个\n", len(spec.DomainLists))

	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	opts := &upstream.Options{
		Logger:  logger,
		Timeout: 10 * time.Second,
	}

	// Create hot reload manager
	fmt.Println("\n[2/4] 🔄 创建热重载管理器...")
	
	var currentUGC *proxy.UpstreamGroupConfig
	
	hotReload := proxy.NewHotReloadManager(
		configPath,
		spec,
		opts,
		func(ugc *proxy.UpstreamGroupConfig) error {
			currentUGC = ugc
			fmt.Printf("\n🔔 配置已重载！\n")
			fmt.Printf("  - 上游组: %d 个\n", len(ugc.Groups))
			fmt.Printf("  - 域名映射: %d 条\n", len(ugc.DomainGroups))
			return nil
		},
	)

	// Start hot reload (check every 2 seconds for demo)
	hotReload.Start(2 * time.Second)
	fmt.Println("✓ 热重载已启动 (每 2 秒检查一次)")

	// Setup API handlers
	fmt.Println("\n[3/4] 🌐 启动 API 服务器...")
	
	apiHandler := proxy.NewAPIHandler(hotReload)
	
	http.HandleFunc("/api/domain-lists", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			apiHandler.HandleAddDomainList(w, r)
		case http.MethodGet:
			handleGetDomainLists(w, r, spec)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	
	http.HandleFunc("/api/domain-lists/remove", apiHandler.HandleRemoveDomainList)
	http.HandleFunc("/api/domain-lists/update", apiHandler.HandleUpdateDomainList)
	http.HandleFunc("/api/reload", apiHandler.HandleReloadConfig)
	http.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		handleHealth(w, r, currentUGC)
	})

	addr := ":8080"
	fmt.Printf("✓ API 服务器启动在 %s\n", addr)
	
	fmt.Println("\n[4/4] 📡 可用的 API 端点:")
	fmt.Println("  POST   /api/domain-lists          - 添加新的域名列表")
	fmt.Println("  GET    /api/domain-lists          - 获取所有域名列表")
	fmt.Println("  DELETE /api/domain-lists/remove   - 删除域名列表")
	fmt.Println("  PUT    /api/domain-lists/update   - 更新域名列表")
	fmt.Println("  POST   /api/reload                - 手动重载配置")
	fmt.Println("  GET    /api/health                - 健康检查")
	
	fmt.Println("\n╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    使用示例                                ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("1. 添加新的域名列表:")
	fmt.Println(`   curl -X POST http://localhost:8080/api/domain-lists \
     -H "Content-Type: application/json" \
     -d '{
       "name": "anti-ad",
       "source": "https://raw.githubusercontent.com/privacy-protection-tools/anti-AD/master/anti-ad-domains.txt",
       "group": "adblock",
       "enabled": true,
       "auto_update": true,
       "refresh_interval": "12h",
       "format": "hosts"
     }'`)
	fmt.Println()
	fmt.Println("2. 删除域名列表:")
	fmt.Println("   curl -X DELETE 'http://localhost:8080/api/domain-lists/remove?name=anti-ad'")
	fmt.Println()
	fmt.Println("3. 手动重载配置:")
	fmt.Println("   curl -X POST http://localhost:8080/api/reload")
	fmt.Println()
	fmt.Println("4. 查看健康状态:")
	fmt.Println("   curl http://localhost:8080/api/health")
	fmt.Println()
	fmt.Println("💡 提示: 修改配置文件后会自动重载（2秒内检测到）")
	fmt.Println()

	// Start server
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func handleGetDomainLists(w http.ResponseWriter, r *http.Request, spec *proxy.UpstreamGroupsSpec) {
	lists := make([]map[string]interface{}, 0)

	for _, list := range spec.DomainLists {
		lists = append(lists, map[string]interface{}{
			"name":             list.Name,
			"source":           list.Source,
			"group":            list.Group,
			"enabled":          list.Enabled,
			"format":           list.Format,
			"auto_update":      list.AutoUpdate,
			"refresh_interval": list.RefreshInterval,
			"domain_count":     list.DomainCount,
			"last_updated":     list.LastUpdated,
		})
	}

	response := map[string]interface{}{
		"lists":     lists,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleHealth(w http.ResponseWriter, r *http.Request, ugc *proxy.UpstreamGroupConfig) {
	status := "initializing"
	groups := 0
	domains := 0

	if ugc != nil {
		status = "healthy"
		groups = len(ugc.Groups)
		domains = len(ugc.DomainGroups)
	}

	response := map[string]interface{}{
		"status":          status,
		"groups":          groups,
		"domain_mappings": domains,
		"timestamp":       time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
