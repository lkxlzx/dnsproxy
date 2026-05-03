# AdGuard Home UI 集成方案

## 📋 目录
1. [架构设计](#架构设计)
2. [API 接口设计](#api-接口设计)
3. [前端 UI 设计](#前端-ui-设计)
4. [后端实现](#后端实现)
5. [配置文件格式](#配置文件格式)
6. [部署方案](#部署方案)

---

## 🏗️ 架构设计

### 整体架构

```
┌─────────────────────────────────────────────────────────┐
│                   AdGuard Home UI                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │ 上游组管理   │  │ 域名列表管理 │  │ 统计监控     │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
└─────────────────────────────────────────────────────────┘
                            │
                            ↓ REST API
┌─────────────────────────────────────────────────────────┐
│              AdGuard Home Backend (Go)                   │
│  ┌──────────────────────────────────────────────────┐  │
│  │           API Handler Layer                       │  │
│  │  - /api/upstream_groups                          │  │
│  │  - /api/domain_lists                             │  │
│  │  - /api/stats                                    │  │
│  └──────────────────────────────────────────────────┘  │
│                            │                             │
│  ┌──────────────────────────────────────────────────┐  │
│  │      DNSProxy Integration Layer                   │  │
│  │  - DomainListManager                             │  │
│  │  - UpstreamGroupConfig                           │  │
│  │  - DomainListCache                               │  │
│  └──────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
                            │
                            ↓
┌─────────────────────────────────────────────────────────┐
│                    DNSProxy Core                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │ RadixTree    │  │ DNS Resolver │  │ Cache        │  │
│  └──────────────┘  └──────────────┘  └──────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### 设计原则

1. **职责分离**
   - AdGuard Home UI: 用户界面和配置管理
   - DNSProxy Core: DNS 解析和域名匹配

2. **最小侵入**
   - 不修改 AdGuard Home 核心代码
   - 通过插件/扩展方式集成

3. **向后兼容**
   - 保持 AdGuard Home 原有功能
   - 新功能作为可选模块

---

## 🔌 API 接口设计

### 1. 上游组管理 API

#### 获取所有上游组
```http
GET /api/upstream_groups
```

**响应**:
```json
{
  "groups": [
    {
      "name": "china",
      "upstreams": ["223.5.5.5", "119.29.29.29"],
      "mode": "load_balance",
      "timeout": "5s",
      "enabled": true,
      "priority": 0,
      "stats": {
        "queries": 12345,
        "avg_response_time": "7.5ms"
      }
    },
    {
      "name": "overseas",
      "upstreams": ["8.8.8.8", "1.1.1.1"],
      "mode": "parallel",
      "timeout": "10s",
      "enabled": true,
      "priority": 0,
      "stats": {
        "queries": 8765,
        "avg_response_time": "198ms"
      }
    }
  ],
  "default_group": "overseas"
}
```

#### 创建上游组
```http
POST /api/upstream_groups
Content-Type: application/json

{
  "name": "custom",
  "upstreams": ["1.1.1.1", "8.8.8.8"],
  "mode": "load_balance",
  "timeout": "5s",
  "enabled": true,
  "priority": 0
}
```

#### 更新上游组
```http
PUT /api/upstream_groups/{name}
Content-Type: application/json

{
  "upstreams": ["1.1.1.1", "8.8.8.8", "9.9.9.9"],
  "mode": "parallel",
  "enabled": true
}
```

#### 删除上游组
```http
DELETE /api/upstream_groups/{name}
```

---

### 2. 域名列表管理 API

#### 获取所有域名列表
```http
GET /api/domain_lists
```

**响应**:
```json
{
  "lists": [
    {
      "name": "china",
      "source": "https://example.com/china-domains.txt",
      "group": "china",
      "enabled": true,
      "auto_update": true,
      "last_update": "2026-05-03T10:00:00Z",
      "domain_count": 50000,
      "format": "plain",
      "status": "active"
    },
    {
      "name": "adblock",
      "source": "./domains/adblock.txt",
      "group": "adblock",
      "enabled": true,
      "auto_update": false,
      "last_update": "2026-05-02T15:30:00Z",
      "domain_count": 100000,
      "format": "adblock",
      "status": "active"
    }
  ]
}
```

#### 添加域名列表
```http
POST /api/domain_lists
Content-Type: application/json

{
  "name": "custom_list",
  "source": "https://example.com/domains.txt",
  "group": "china",
  "enabled": true,
  "auto_update": true
}
```

**响应**:
```json
{
  "success": true,
  "message": "Domain list added successfully",
  "list": {
    "name": "custom_list",
    "status": "downloading"
  }
}
```

#### 更新域名列表
```http
POST /api/domain_lists/{name}/update
```

**响应**:
```json
{
  "success": true,
  "message": "Domain list updated successfully",
  "domain_count": 50123,
  "download_time": "2.5s"
}
```

#### 删除域名列表
```http
DELETE /api/domain_lists/{name}
```

---

### 3. 统计和监控 API

#### 获取统计信息
```http
GET /api/stats/upstream_groups
```

**响应**:
```json
{
  "total_queries": 21110,
  "groups": {
    "china": {
      "queries": 12345,
      "avg_response_time": "7.5ms",
      "success_rate": 99.8,
      "cache_hit_rate": 85.2
    },
    "overseas": {
      "queries": 8765,
      "avg_response_time": "198ms",
      "success_rate": 98.5,
      "cache_hit_rate": 72.1
    }
  },
  "domain_tree_stats": {
    "exact_matches": 50000,
    "wildcard_patterns": 1500,
    "total_domains": 51500,
    "tree_depth": 12
  }
}
```

#### 获取域名匹配测试
```http
POST /api/test/domain_match
Content-Type: application/json

{
  "domain": "www.baidu.com"
}
```

**响应**:
```json
{
  "domain": "www.baidu.com",
  "matched_group": "china",
  "matched_pattern": "*.baidu.com",
  "upstream_servers": ["223.5.5.5", "119.29.29.29"],
  "match_time": "0.015ms"
}
```

---

## 🎨 前端 UI 设计

### 1. 上游组管理页面

```
┌─────────────────────────────────────────────────────────┐
│  上游 DNS 服务器组管理                    [+ 添加新组]   │
├─────────────────────────────────────────────────────────┤
│                                                           │
│  ┌─────────────────────────────────────────────────┐    │
│  │ 🌏 中国大陆 DNS                      [启用] ✓   │    │
│  │                                                   │    │
│  │ 上游服务器:                                      │    │
│  │   • 223.5.5.5 (阿里 DNS)                        │    │
│  │   • 119.29.29.29 (腾讯 DNS)                     │    │
│  │   • 114.114.114.114 (114 DNS)                   │    │
│  │                                                   │    │
│  │ 模式: 负载均衡  超时: 5s  优先级: 0             │    │
│  │                                                   │    │
│  │ 统计: 12,345 次查询 | 平均响应: 7.5ms           │    │
│  │                                                   │    │
│  │                          [编辑] [删除] [测试]    │    │
│  └─────────────────────────────────────────────────┘    │
│                                                           │
│  ┌─────────────────────────────────────────────────┐    │
│  │ 🌍 海外 DNS                          [启用] ✓   │    │
│  │                                                   │    │
│  │ 上游服务器:                                      │    │
│  │   • 8.8.8.8 (Google DNS)                        │    │
│  │   • 1.1.1.1 (Cloudflare DNS)                    │    │
│  │   • https://dns.google/dns-query (Google DoH)   │    │
│  │                                                   │    │
│  │ 模式: 并行查询  超时: 10s  优先级: 0            │    │
│  │                                                   │    │
│  │ 统计: 8,765 次查询 | 平均响应: 198ms            │    │
│  │                                                   │    │
│  │                          [编辑] [删除] [测试]    │    │
│  └─────────────────────────────────────────────────┘    │
│                                                           │
│  默认上游组: [海外 DNS ▼]                               │
│                                                           │
└─────────────────────────────────────────────────────────┘
```

### 2. 域名列表管理页面

```
┌─────────────────────────────────────────────────────────┐
│  域名列表管理                        [+ 添加域名列表]    │
├─────────────────────────────────────────────────────────┤
│                                                           │
│  筛选: [全部 ▼] [中国大陆 ▼] [海外 ▼] [广告拦截 ▼]    │
│                                                           │
│  ┌─────────────────────────────────────────────────┐    │
│  │ 📋 中国域名列表                  [启用] ✓       │    │
│  │                                                   │    │
│  │ 来源: https://example.com/china-domains.txt      │    │
│  │ 目标组: 中国大陆 DNS                             │    │
│  │ 格式: Plain Text                                 │    │
│  │                                                   │    │
│  │ 域名数量: 50,000                                 │    │
│  │ 最后更新: 2026-05-03 10:00:00                   │    │
│  │ 自动更新: ✓ 每 12 小时                          │    │
│  │ 状态: 🟢 活跃                                    │    │
│  │                                                   │    │
│  │              [立即更新] [编辑] [删除] [查看]     │    │
│  └─────────────────────────────────────────────────┘    │
│                                                           │
│  ┌─────────────────────────────────────────────────┐    │
│  │ 🚫 广告拦截列表                  [启用] ✓       │    │
│  │                                                   │    │
│  │ 来源: https://adguard.com/filter.txt             │    │
│  │ 目标组: 广告拦截                                 │    │
│  │ 格式: AdBlock Plus                               │    │
│  │                                                   │    │
│  │ 域名数量: 100,000                                │    │
│  │ 最后更新: 2026-05-02 15:30:00                   │    │
│  │ 自动更新: ✓ 每 24 小时                          │    │
│  │ 状态: 🟢 活跃                                    │    │
│  │                                                   │    │
│  │              [立即更新] [编辑] [删除] [查看]     │    │
│  └─────────────────────────────────────────────────┘    │
│                                                           │
└─────────────────────────────────────────────────────────┘
```

### 3. 统计监控页面

```
┌─────────────────────────────────────────────────────────┐
│  DNS 查询统计                                            │
├─────────────────────────────────────────────────────────┤
│                                                           │
│  总查询数: 21,110  │  缓存命中率: 78.5%                 │
│                                                           │
│  ┌─────────────────────────────────────────────────┐    │
│  │  按上游组统计                                    │    │
│  │                                                   │    │
│  │  中国大陆 DNS:  12,345 (58.5%)  ████████████     │    │
│  │  平均响应: 7.5ms  成功率: 99.8%                 │    │
│  │                                                   │    │
│  │  海外 DNS:      8,765 (41.5%)  ████████          │    │
│  │  平均响应: 198ms  成功率: 98.5%                 │    │
│  │                                                   │    │
│  └─────────────────────────────────────────────────┘    │
│                                                           │
│  ┌─────────────────────────────────────────────────┐    │
│  │  域名树统计                                      │    │
│  │                                                   │    │
│  │  精确匹配: 50,000                                │    │
│  │  通配符模式: 1,500                               │    │
│  │  总域名数: 51,500                                │    │
│  │  树深度: 12                                      │    │
│  │                                                   │    │
│  └─────────────────────────────────────────────────┘    │
│                                                           │
│  ┌─────────────────────────────────────────────────┐    │
│  │  域名匹配测试                                    │    │
│  │                                                   │    │
│  │  测试域名: [www.baidu.com          ] [测试]     │    │
│  │                                                   │    │
│  │  结果:                                           │    │
│  │  ✓ 匹配组: 中国大陆 DNS                         │    │
│  │  ✓ 匹配模式: *.baidu.com                        │    │
│  │  ✓ 上游服务器: 223.5.5.5, 119.29.29.29          │    │
│  │  ✓ 匹配时间: 0.015ms                            │    │
│  │                                                   │    │
│  └─────────────────────────────────────────────────┘    │
│                                                           │
└─────────────────────────────────────────────────────────┘
```

---


## 💻 后端实现

### 1. API Handler 实现

```go
// internal/home/upstream_groups_handler.go
package home

import (
    "encoding/json"
    "net/http"
    
    "github.com/AdguardTeam/dnsproxy/proxy"
)

// UpstreamGroupsHandler handles upstream groups API requests
type UpstreamGroupsHandler struct {
    config  *proxy.UpstreamGroupConfig
    manager *proxy.DomainListManager
}

// NewUpstreamGroupsHandler creates a new handler
func NewUpstreamGroupsHandler(
    config *proxy.UpstreamGroupConfig,
    manager *proxy.DomainListManager,
) *UpstreamGroupsHandler {
    return &UpstreamGroupsHandler{
        config:  config,
        manager: manager,
    }
}

// RegisterHandlers registers all API handlers
func (h *UpstreamGroupsHandler) RegisterHandlers(mux *http.ServeMux) {
    // Upstream groups
    mux.HandleFunc("/api/upstream_groups", h.handleUpstreamGroups)
    mux.HandleFunc("/api/upstream_groups/", h.handleUpstreamGroup)
    
    // Domain lists
    mux.HandleFunc("/api/domain_lists", h.handleDomainLists)
    mux.HandleFunc("/api/domain_lists/", h.handleDomainList)
    
    // Stats
    mux.HandleFunc("/api/stats/upstream_groups", h.handleStats)
    mux.HandleFunc("/api/test/domain_match", h.handleDomainMatch)
}

// handleUpstreamGroups handles GET /api/upstream_groups
func (h *UpstreamGroupsHandler) handleUpstreamGroups(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        h.getUpstreamGroups(w, r)
    case http.MethodPost:
        h.createUpstreamGroup(w, r)
    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
}

// getUpstreamGroups returns all upstream groups
func (h *UpstreamGroupsHandler) getUpstreamGroups(w http.ResponseWriter, r *http.Request) {
    groups := make([]map[string]interface{}, 0, len(h.config.Groups))
    
    for name, group := range h.config.Groups {
        upstreams := make([]string, len(group.Upstreams))
        for i, u := range group.Upstreams {
            upstreams[i] = u.Address()
        }
        
        groups = append(groups, map[string]interface{}{
            "name":      name,
            "upstreams": upstreams,
            "mode":      string(group.Mode),
            "timeout":   group.Timeout.String(),
            "enabled":   group.Enabled,
            "priority":  group.Priority,
        })
    }
    
    response := map[string]interface{}{
        "groups":        groups,
        "default_group": h.config.DefaultGroup,
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

// createUpstreamGroup creates a new upstream group
func (h *UpstreamGroupsHandler) createUpstreamGroup(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Name      string   `json:"name"`
        Upstreams []string `json:"upstreams"`
        Mode      string   `json:"mode"`
        Timeout   string   `json:"timeout"`
        Enabled   bool     `json:"enabled"`
        Priority  int      `json:"priority"`
    }
    
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    
    // Create upstream group
    // ... implementation ...
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "success": true,
        "message": "Upstream group created successfully",
    })
}

// handleDomainLists handles domain lists API
func (h *UpstreamGroupsHandler) handleDomainLists(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        h.getDomainLists(w, r)
    case http.MethodPost:
        h.addDomainList(w, r)
    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
}

// getDomainLists returns all domain lists
func (h *UpstreamGroupsHandler) getDomainLists(w http.ResponseWriter, r *http.Request) {
    lists := h.manager.ListAll()
    
    response := make([]map[string]interface{}, 0, len(lists))
    for _, list := range lists {
        response = append(response, map[string]interface{}{
            "name":         list.Name,
            "source":       list.Source,
            "group":        list.Group,
            "enabled":      list.Enabled,
            "auto_update":  list.AutoUpdate,
            "last_update":  list.LastUpdate,
            "domain_count": list.DomainCount,
            "format":       list.Format,
        })
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "lists": response,
    })
}

// addDomainList adds a new domain list
func (h *UpstreamGroupsHandler) addDomainList(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Name       string `json:"name"`
        Source     string `json:"source"`
        Group      string `json:"group"`
        Enabled    bool   `json:"enabled"`
        AutoUpdate bool   `json:"auto_update"`
    }
    
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    
    // Add domain list
    list := &proxy.ManagedList{
        Name:       req.Name,
        Source:     req.Source,
        Group:      req.Group,
        Enabled:    req.Enabled,
        AutoUpdate: req.AutoUpdate,
    }
    
    if err := h.manager.AddList(list); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    // Start download in background
    go func() {
        localPath := h.manager.GetCachePath(req.Source)
        h.manager.DownloadAndCache(req.Source, localPath)
    }()
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "success": true,
        "message": "Domain list added successfully",
        "list": map[string]interface{}{
            "name":   req.Name,
            "status": "downloading",
        },
    })
}

// handleDomainMatch tests domain matching
func (h *UpstreamGroupsHandler) handleDomainMatch(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    
    var req struct {
        Domain string `json:"domain"`
    }
    
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    
    // Test domain matching
    group, err := h.config.GetGroupForDomain(req.Domain)
    if err != nil {
        http.Error(w, err.Error(), http.StatusNotFound)
        return
    }
    
    upstreams := make([]string, len(group.Upstreams))
    for i, u := range group.Upstreams {
        upstreams[i] = u.Address()
    }
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "domain":           req.Domain,
        "matched_group":    group.Name,
        "upstream_servers": upstreams,
    })
}
```

### 2. 配置加载实现

```go
// internal/home/config.go
package home

import (
    "log/slog"
    "os"
    
    "github.com/AdguardTeam/dnsproxy/proxy"
    "github.com/AdguardTeam/dnsproxy/upstream"
    "gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
    UpstreamGroups *proxy.UpstreamGroupsSpec `yaml:"upstream_groups"`
    DomainGroups   map[string]string         `yaml:"domain_groups"`
    DefaultGroup   string                    `yaml:"default_group"`
    Cache          *CacheConfig              `yaml:"cache"`
}

// CacheConfig represents cache configuration
type CacheConfig struct {
    Enabled        bool   `yaml:"enabled"`
    Directory      string `yaml:"directory"`
    TTL            string `yaml:"ttl"`
    AutoUpdate     bool   `yaml:"auto_update"`
    UpdateInterval string `yaml:"update_interval"`
}

// LoadConfig loads configuration from file
func LoadConfig(path string) (*Config, error) {
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

// InitializeUpstreamGroups initializes upstream groups from config
func InitializeUpstreamGroups(config *Config, logger *slog.Logger) (
    *proxy.UpstreamGroupConfig,
    *proxy.DomainListManager,
    error,
) {
    // Parse upstream groups
    opts := &upstream.Options{
        Logger: logger,
    }
    
    spec := &proxy.UpstreamGroupsSpec{
        Groups:       config.UpstreamGroups.Groups,
        DomainGroups: config.DomainGroups,
        DefaultGroup: config.DefaultGroup,
    }
    
    ugc, err := proxy.ParseUpstreamGroups(spec, opts)
    if err != nil {
        return nil, nil, err
    }
    
    // Initialize domain list manager
    cacheDir := "./cache"
    if config.Cache != nil && config.Cache.Directory != "" {
        cacheDir = config.Cache.Directory
    }
    
    manager := proxy.NewDomainListManager(cacheDir, logger)
    
    return ugc, manager, nil
}
```

### 3. 主程序集成

```go
// main.go
package main

import (
    "log"
    "log/slog"
    "net/http"
    "os"
    
    "github.com/AdguardTeam/AdGuardHome/internal/home"
)

func main() {
    // Initialize logger
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
    
    // Load configuration
    config, err := home.LoadConfig("config.yaml")
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }
    
    // Initialize upstream groups
    ugc, manager, err := home.InitializeUpstreamGroups(config, logger)
    if err != nil {
        log.Fatalf("Failed to initialize upstream groups: %v", err)
    }
    
    // Log configuration
    ugc.LogGroupInfo(logger)
    
    // Create API handler
    handler := home.NewUpstreamGroupsHandler(ugc, manager)
    
    // Register routes
    mux := http.NewServeMux()
    handler.RegisterHandlers(mux)
    
    // Serve static files (UI)
    mux.Handle("/", http.FileServer(http.Dir("./web")))
    
    // Start server
    logger.Info("Starting AdGuard Home with upstream groups support", "port", 3000)
    if err := http.ListenAndServe(":3000", mux); err != nil {
        log.Fatalf("Failed to start server: %v", err)
    }
}
```

---

## 📄 配置文件格式

### 完整配置示例

参见 `config-adguardhome-integration.yaml`

### 配置字段说明

#### upstream_groups
- `name`: 组名（必填）
- `upstreams`: 上游服务器列表（必填）
- `mode`: 负载均衡模式（可选，默认 `load_balance`）
  - `load_balance`: 轮询负载均衡
  - `parallel`: 并行查询，选择最快响应
  - `fastest_addr`: 选择最快 IP
- `timeout`: 超时时间（可选，默认 `10s`）
- `enabled`: 是否启用（可选，默认 `true`）
- `priority`: 优先级（可选，默认 `0`）

#### domain_groups
- 键：组名或域名模式
- 值：
  - 组名：直接指定上游组
  - 文件路径：从本地文件加载
  - URL：从远程 URL 加载

#### cache
- `enabled`: 是否启用缓存
- `directory`: 缓存目录
- `ttl`: 缓存有效期
- `auto_update`: 是否自动更新
- `update_interval`: 更新间隔

---

## 🚀 部署方案

### 方案 1: 独立部署（推荐）

```
┌─────────────────────────────────────────┐
│         AdGuard Home (原版)              │
│         Port: 80 (Web UI)                │
│         Port: 53 (DNS)                   │
└─────────────────────────────────────────┘
                    │
                    ↓ Proxy
┌─────────────────────────────────────────┐
│    DNSProxy with Upstream Groups         │
│         Port: 5353 (DNS)                 │
│         Port: 3000 (API)                 │
└─────────────────────────────────────────┘
```

**配置步骤**:

1. 安装 AdGuard Home
2. 部署 DNSProxy
3. 配置 AdGuard Home 使用 `127.0.0.1:5353` 作为上游
4. 在 AdGuard Home UI 中添加自定义页面链接到 DNSProxy API

### 方案 2: 集成部署

```
┌─────────────────────────────────────────┐
│    AdGuard Home (修改版)                 │
│    - 集成 DNSProxy 核心                  │
│    - 扩展 Web UI                         │
│    Port: 80 (Web UI + API)               │
│    Port: 53 (DNS)                        │
└─────────────────────────────────────────┘
```

**实现步骤**:

1. Fork AdGuard Home 仓库
2. 添加 DNSProxy 依赖
3. 集成 API Handler
4. 扩展前端 UI
5. 编译部署

### 方案 3: Docker 部署

```yaml
# docker-compose.yml
version: '3'

services:
  adguardhome:
    image: adguard/adguardhome:latest
    ports:
      - "80:80"      # Web UI
      - "53:53/udp"  # DNS
    volumes:
      - ./adguard/work:/opt/adguardhome/work
      - ./adguard/conf:/opt/adguardhome/conf
    environment:
      - UPSTREAM_DNS=127.0.0.1:5353

  dnsproxy:
    build: .
    ports:
      - "5353:5353/udp"  # DNS
      - "3000:3000"      # API
    volumes:
      - ./config.yaml:/app/config.yaml
      - ./cache:/app/cache
      - ./domains:/app/domains
    command: ["-c", "/app/config.yaml"]
```

---

## 📚 使用示例

### 1. 通过 API 添加上游组

```bash
curl -X POST http://localhost:3000/api/upstream_groups \
  -H "Content-Type: application/json" \
  -d '{
    "name": "custom",
    "upstreams": ["1.1.1.1", "8.8.8.8"],
    "mode": "parallel",
    "timeout": "5s",
    "enabled": true
  }'
```

### 2. 添加域名列表

```bash
curl -X POST http://localhost:3000/api/domain_lists \
  -H "Content-Type: application/json" \
  -d '{
    "name": "china_domains",
    "source": "https://example.com/china.txt",
    "group": "china",
    "enabled": true,
    "auto_update": true
  }'
```

### 3. 测试域名匹配

```bash
curl -X POST http://localhost:3000/api/test/domain_match \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "www.baidu.com"
  }'
```

### 4. 获取统计信息

```bash
curl http://localhost:3000/api/stats/upstream_groups
```

---

## 🔧 开发指南

### 前端开发

```bash
# 安装依赖
cd web
npm install

# 开发模式
npm run dev

# 构建生产版本
npm run build
```

### 后端开发

```bash
# 运行测试
go test ./...

# 构建
go build -o adguardhome-extended

# 运行
./adguardhome-extended -c config.yaml
```

---

## 📝 总结

这个集成方案提供了：

1. ✅ **完整的 API 接口** - RESTful API 用于管理上游组和域名列表
2. ✅ **友好的 Web UI** - 直观的界面管理配置
3. ✅ **灵活的部署方式** - 支持独立部署、集成部署、Docker 部署
4. ✅ **强大的功能** - 支持多种域名文件格式、自动更新、统计监控
5. ✅ **高性能** - 基于 Radix Tree 的极速域名匹配

**推荐部署方案**: 方案 1（独立部署），最小侵入，易于维护。

---

**文档版本**: 1.0  
**最后更新**: 2026-05-03  
**作者**: Kiro AI

