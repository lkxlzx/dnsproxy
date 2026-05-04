package main

import (
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"gopkg.in/yaml.v3"
)

// Server represents the API server
type Server struct {
	configPath string
	config     *Config
	ugc        *proxy.UpstreamGroupConfig
	stats      map[string]proxy.DomainListStats
	mu         sync.RWMutex
	logger     *slog.Logger
}

// Config represents the configuration
type Config struct {
	DNS            DNSConfig                     `yaml:"dns"`
	UpstreamGroups []proxy.UpstreamGroupSpec     `yaml:"upstream_groups"`
	DomainGroups   map[string]interface{}        `yaml:"domain_groups"`
	DomainsLists   []proxy.DomainListSpec        `yaml:"domains_lists"`
	DefaultGroup   string                        `yaml:"default_group"`
	Cache          *proxy.CacheConfigSpec        `yaml:"cache"`
}

type DNSConfig struct {
	BindHosts   []string `yaml:"bind_hosts"`
	Port        int      `yaml:"port"`
	UpstreamDNS []string `yaml:"upstream_dns"`
}

func main() {
	configPath := "test-cache-config.yaml"
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	
	// Create server
	server := &Server{
		configPath: configPath,
		logger:     logger,
	}
	
	// Load initial configuration
	if err := server.LoadConfig(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	
	// Setup HTTP routes
	http.HandleFunc("/api/domain-lists", server.handleGetDomainLists)
	http.HandleFunc("/api/domain-lists/stats", server.handleGetStats)
	http.HandleFunc("/api/domain-lists/refresh", server.handleRefreshAll)
	http.HandleFunc("/api/config", server.handleGetConfig)
	http.HandleFunc("/api/health", server.handleHealth)
	
	// Start server
	addr := ":8080"
	fmt.Printf("\n🚀 API Server starting on %s\n\n", addr)
	fmt.Println("Available endpoints:")
	fmt.Println("  GET  /api/domain-lists        - 获取所有域名列表")
	fmt.Println("  GET  /api/domain-lists/stats  - 获取统计信息")
	fmt.Println("  POST /api/domain-lists/refresh - 刷新所有列表")
	fmt.Println("  GET  /api/config              - 获取完整配置")
	fmt.Println("  GET  /api/health              - 健康检查")
	fmt.Println("\n示例:")
	fmt.Println("  curl http://localhost:8080/api/domain-lists/stats")
	fmt.Println("  curl -X POST http://localhost:8080/api/domain-lists/refresh")
	fmt.Println()
	
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// LoadConfig loads and parses the configuration
func (s *Server) LoadConfig() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.logger.Info("loading configuration", "path", s.configPath)
	
	// Read config file
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	
	// Parse config
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	
	s.config = &config
	
	// Parse upstream groups
	spec := &proxy.UpstreamGroupsSpec{
		Groups:       config.UpstreamGroups,
		DomainGroups: config.DomainGroups,
		DomainLists:  config.DomainsLists,
		DefaultGroup: config.DefaultGroup,
		Cache:        config.Cache,
	}
	
	opts := &upstream.Options{
		Logger:  s.logger,
		Timeout: 10 * time.Second,
	}
	
	ugc, err := proxy.ParseUpstreamGroups(spec, opts)
	if err != nil {
		return fmt.Errorf("parse upstream groups: %w", err)
	}
	
	s.ugc = ugc
	
	// Collect statistics
	stats, err := proxy.CollectDomainListStats(spec)
	if err != nil {
		return fmt.Errorf("collect stats: %w", err)
	}
	
	s.stats = stats
	
	// Update config file
	if err := proxy.UpdateConfigFileStats(s.configPath, stats); err != nil {
		s.logger.Warn("failed to update config file", "error", err)
	}
	
	s.logger.Info("configuration loaded successfully",
		"groups", len(ugc.Groups),
		"domains", len(ugc.DomainGroups),
		"lists", len(stats))
	
	return nil
}

// handleGetDomainLists returns all domain lists
func (s *Server) handleGetDomainLists(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	lists := make([]map[string]interface{}, 0)
	
	for _, list := range s.config.DomainsLists {
		stat, exists := s.stats[list.Name]
		
		listData := map[string]interface{}{
			"name":             list.Name,
			"source":           list.Source,
			"group":            list.Group,
			"enabled":          list.Enabled,
			"format":           list.Format,
			"auto_update":      list.AutoUpdate,
			"refresh_interval": list.RefreshInterval,
		}
		
		if exists {
			listData["domain_count"] = stat.DomainCount
			listData["last_updated"] = stat.LastUpdated.Format(time.RFC3339)
		}
		
		lists = append(lists, listData)
	}
	
	response := map[string]interface{}{
		"lists":     lists,
		"timestamp": time.Now().Format(time.RFC3339),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleGetStats returns statistics
func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	totalDomains := 0
	lists := make([]map[string]interface{}, 0)
	
	for name, stat := range s.stats {
		lists = append(lists, map[string]interface{}{
			"name":         name,
			"domain_count": stat.DomainCount,
			"last_updated": stat.LastUpdated.Format(time.RFC3339),
		})
		totalDomains += stat.DomainCount
	}
	
	response := map[string]interface{}{
		"lists":         lists,
		"total_lists":   len(lists),
		"total_domains": totalDomains,
		"timestamp":     time.Now().Format(time.RFC3339),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleRefreshAll refreshes all domain lists
func (s *Server) handleRefreshAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	s.logger.Info("refreshing all domain lists")
	
	// Reload configuration (will download lists again)
	if err := s.LoadConfig(); err != nil {
		s.logger.Error("failed to refresh", "error", err)
		http.Error(w, fmt.Sprintf("Refresh failed: %v", err), http.StatusInternalServerError)
		return
	}
	
	s.mu.RLock()
	totalDomains := 0
	for _, stat := range s.stats {
		totalDomains += stat.DomainCount
	}
	s.mu.RUnlock()
	
	response := map[string]interface{}{
		"success":       true,
		"message":       "All domain lists refreshed successfully",
		"total_lists":   len(s.stats),
		"total_domains": totalDomains,
		"timestamp":     time.Now().Format(time.RFC3339),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleGetConfig returns the full configuration
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.config)
}

// handleHealth returns health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	response := map[string]interface{}{
		"status":        "healthy",
		"groups":        len(s.ugc.Groups),
		"domain_mappings": len(s.ugc.DomainGroups),
		"lists":         len(s.stats),
		"timestamp":     time.Now().Format(time.RFC3339),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
