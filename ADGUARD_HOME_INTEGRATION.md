# AdGuard Home 集成指南

## 概述

本文档说明如何在 AdGuard Home 中集成 dnsproxy 的域名分组和列表管理功能。

**架构原则**：
- **dnsproxy**: 提供完整的核心功能（下载、格式转换、缓存、路由）
- **AdGuard Home**: 仅提供 UI 层对接（调用 dnsproxy API）

## 架构设计

```
┌─────────────────────────────────────────────┐
│         AdGuard Home (UI 层)                 │
│  - Web UI 界面（显示、操作）                  │
│  - HTTP API 端点（转发到 dnsproxy）          │
│  - 配置持久化                                │
└─────────────────┬───────────────────────────┘
                  │ 简单的 API 调用
                  ↓
┌─────────────────────────────────────────────┐
│         dnsproxy (核心层)                    │
│  ✅ DomainListManager - 列表管理             │
│  ✅ DomainListCache - 缓存管理               │
│  ✅ DomainFileLoader - 下载和加载            │
│  ✅ FormatConverter - 格式转换（8种格式）     │
│  ✅ UpstreamGroupConfig - 域名路由           │
└─────────────────────────────────────────────┘
```

## dnsproxy 提供的完整功能

### ✅ 已实现的核心功能

1. **下载管理** (`DomainFileLoader`)
   - HTTP/HTTPS 下载
   - 超时控制
   - 错误处理

2. **格式转换** (`FormatConverter`)
   - 支持 8 种格式：Plain Text, Clash YAML, Surge, Dnsmasq, Hosts, AdBlock, GFWList, JSON
   - 自动格式检测
   - 格式互转

3. **缓存管理** (`DomainListCache`)
   - 本地缓存
   - TTL 控制
   - 过期清理

4. **列表管理** (`DomainListManager`)
   - 添加/删除/更新列表
   - 自动下载和缓存
   - 配置生成

5. **域名路由** (`UpstreamGroupConfig`)
   - 精确匹配
   - 通配符匹配
   - 默认组回退

## AdGuard Home 只需要做的事

### 1. 提供 UI 界面

显示列表、接收用户操作、调用 dnsproxy API。

### 2. 简单的 API 转发

```go
// AdGuard Home 的 HTTP 处理器
// 只需要简单地调用 dnsproxy 的方法

// 添加列表
func (h *Home) handleAddDomainList(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Name   string `json:"name"`
        URL    string `json:"url"`
        Group  string `json:"group"`
    }
    json.NewDecoder(r.Body).Decode(&req)
    
    // 1. 调用 dnsproxy 下载和缓存（自动处理格式转换）
    localPath := h.manager.GetCachePath(req.URL)
    err := h.manager.DownloadAndCache(req.URL, localPath)
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }
    
    // 2. 添加到管理器
    list := &proxy.ManagedList{
        Name:      req.Name,
        Source:    req.URL,
        LocalPath: localPath,
        Group:     req.Group,
        Enabled:   true,
    }
    h.manager.AddList(list)
    
    // 3. 保存配置
    h.saveConfig()
    
    // 4. 重载 DNS
    h.reloadDNS()
    
    json.NewEncoder(w).Encode(list)
}

// 更新列表
func (h *Home) handleUpdateDomainList(w http.ResponseWriter, r *http.Request) {
    name := mux.Vars(r)["name"]
    
    // 直接调用 dnsproxy 的更新方法
    // dnsproxy 会自动：下载 -> 格式转换 -> 缓存
    err := h.manager.UpdateList(name)
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }
    
    h.reloadDNS()
    w.WriteHeader(200)
}

// 获取列表
func (h *Home) handleGetDomainLists(w http.ResponseWriter, r *http.Request) {
    // 直接返回 dnsproxy 管理的列表
    lists := h.manager.ListAll()
    json.NewEncoder(w).Encode(lists)
}

// 删除列表
func (h *Home) handleDeleteDomainList(w http.ResponseWriter, r *http.Request) {
    name := mux.Vars(r)["name"]
    h.manager.RemoveList(name)
    h.saveConfig()
    h.reloadDNS()
    w.WriteHeader(204)
}
```

### 3. 配置持久化

保存列表配置到 AdGuardHome.yaml。

```yaml
# AdGuardHome.yaml
dns:
  domain_lists:
    cache_dir: "./cache/domains"
    lists:
      - name: "china"
        url: "https://raw.githubusercontent.com/.../ChinaMax.yaml"
        group: "china"
        enabled: true
        auto_update: true
```

### 4. 定时任务（可选）

```go
// 简单的定时更新
func (h *Home) startAutoUpdate() {
    ticker := time.NewTicker(1 * time.Hour)
    go func() {
        for range ticker.C {
            lists := h.manager.ListAll()
            for _, list := range lists {
                if list.AutoUpdate && list.Enabled {
                    // dnsproxy 自动处理一切
                    h.manager.UpdateList(list.Name)
                }
            }
        }
    }()
}
```

## 完整的工作流程

### 用户添加列表

```
1. 用户在 UI 输入：
   - 名称: "china"
   - URL: "https://.../ChinaMax.yaml"
   - 组: "china"
   
2. AdGuard Home 调用:
   manager.DownloadAndCache(url, localPath)
   
3. dnsproxy 自动完成:
   ✅ 下载文件
   ✅ 检测格式（Clash YAML）
   ✅ 解析域名（116,479 个）
   ✅ 转换为 Plain Text
   ✅ 保存到缓存
   
4. AdGuard Home 调用:
   manager.AddList(list)
   
5. 保存配置，重载 DNS
   
完成！
```

### 用户更新列表

```
1. 用户点击"更新"按钮
   
2. AdGuard Home 调用:
   manager.UpdateList("china")
   
3. dnsproxy 自动完成:
   ✅ 重新下载
   ✅ 格式转换
   ✅ 更新缓存
   
4. 重载 DNS
   
完成！
```

### DNS 查询

```
1. 查询 www.baidu.com
   
2. dnsproxy 自动:
   ✅ 从内存中的域名映射查找
   ✅ 匹配到 "china" 组
   ✅ 使用 223.5.5.5 查询
   
3. 返回结果
   
完成！（< 1μs）
```

### 1. 数据模型

```go
// AdGuard Home 的域名列表配置
type DomainListConfig struct {
    Lists []DomainList `yaml:"domain_lists"`
}

type DomainList struct {
    Name         string `yaml:"name"`
    URL          string `yaml:"url"`
    Enabled      bool   `yaml:"enabled"`
    Group        string `yaml:"group"`
    AutoUpdate   bool   `yaml:"auto_update"`
    UpdateCron   string `yaml:"update_cron"`
    LastUpdate   string `yaml:"last_update"`
}
```

### 2. UI 界面设计

#### 列表管理页面

```
/control/domain_lists
```

**功能**:
- 显示所有域名列表
- 添加新列表
- 编辑列表
- 删除列表
- 手动更新
- 启用/禁用

#### API 端点

```go
// GET /control/domain_lists
// 获取所有列表
func handleGetDomainLists(w http.ResponseWriter, r *http.Request) {
    lists := manager.ListAll()
    json.NewEncoder(w).Encode(lists)
}

// POST /control/domain_lists
// 添加新列表
func handleAddDomainList(w http.ResponseWriter, r *http.Request) {
    var list proxy.ManagedList
    json.NewDecoder(r.Body).Decode(&list)
    
    // 下载并缓存
    err := manager.DownloadAndCache(list.Source, list.LocalPath)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    
    // 添加到管理器
    manager.AddList(&list)
    
    // 保存配置
    saveConfig()
    
    w.WriteHeader(http.StatusCreated)
}

// POST /control/domain_lists/{name}/update
// 更新列表
func handleUpdateDomainList(w http.ResponseWriter, r *http.Request) {
    name := mux.Vars(r)["name"]
    
    list, err := manager.GetList(name)
    if err != nil {
        http.Error(w, err.Error(), http.StatusNotFound)
        return
    }
    
    // 下载最新版本
    err = manager.DownloadAndCache(list.Source, list.LocalPath)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    // 更新列表信息
    manager.UpdateList(name)
    
    // 重载 DNS 配置
    reloadDNSConfig()
    
    w.WriteHeader(http.StatusOK)
}

// DELETE /control/domain_lists/{name}
// 删除列表
func handleDeleteDomainList(w http.ResponseWriter, r *http.Request) {
    name := mux.Vars(r)["name"]
    manager.RemoveList(name)
    saveConfig()
    w.WriteHeader(http.StatusNoContent)
}
```

### 3. 定时更新

```go
// 启动定时更新任务
func startAutoUpdate() {
    ticker := time.NewTicker(1 * time.Hour)
    go func() {
        for range ticker.C {
            updateAllLists()
        }
    }()
}

func updateAllLists() {
    lists := manager.ListAll()
    for _, list := range lists {
        if !list.AutoUpdate || !list.Enabled {
            continue
        }
        
        // 检查是否需要更新（根据 cron 表达式）
        if shouldUpdate(list) {
            log.Info("auto-updating list", "name", list.Name)
            err := manager.DownloadAndCache(list.Source, list.LocalPath)
            if err != nil {
                log.Error("failed to update list", "name", list.Name, "error", err)
                continue
            }
            
            manager.UpdateList(list.Name)
            
            // 重载 DNS 配置
            reloadDNSConfig()
        }
    }
}
```

### 4. 配置文件集成

```yaml
# AdGuardHome.yaml

# DNS 配置
dns:
  # ... 其他配置 ...
  
  # 上游分组配置
  upstream_groups:
    default_group: "default"
    
    groups:
      - name: "default"
        upstreams:
          - "8.8.8.8"
          - "1.1.1.1"
      
      - name: "china"
        upstreams:
          - "223.5.5.5"
          - "119.29.29.29"
    
    # 域名分组（由列表管理器生成）
    domain_groups:
      "china": "./cache/domains/china.yaml"
      "ads": "./cache/domains/adblock.txt"

# 域名列表配置
domain_lists:
  cache_dir: "./cache/domains"
  
  lists:
    - name: "china"
      url: "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/ChinaMax/ChinaMax_Classical.yaml"
      enabled: true
      group: "china"
      auto_update: true
      update_cron: "0 3 * * *"  # 每天凌晨3点
      last_update: "2024-01-01T03:00:00Z"
    
    - name: "adblock"
      url: "https://example.com/adblock.txt"
      enabled: true
      group: "ads"
      auto_update: true
      update_cron: "0 4 * * *"
```

### 5. 初始化流程

```go
// AdGuard Home 启动时
func initDNS() error {
    // 1. 创建域名列表管理器
    manager = proxy.NewDomainListManager(config.DomainLists.CacheDir, logger)
    
    // 2. 加载配置的列表
    for _, listConfig := range config.DomainLists.Lists {
        list := &proxy.ManagedList{
            Name:       listConfig.Name,
            Source:     listConfig.URL,
            LocalPath:  manager.GetCachePath(listConfig.URL),
            Group:      listConfig.Group,
            Enabled:    listConfig.Enabled,
            AutoUpdate: listConfig.AutoUpdate,
        }
        
        manager.AddList(list)
        
        // 如果本地缓存不存在，下载
        if !fileExists(list.LocalPath) {
            log.Info("downloading initial list", "name", list.Name)
            err := manager.DownloadAndCache(list.Source, list.LocalPath)
            if err != nil {
                log.Error("failed to download list", "name", list.Name, "error", err)
                // 继续，不阻塞启动
            }
        }
    }
    
    // 3. 构建域名分组配置
    domainGroups := manager.BuildDomainGroupsConfig()
    
    // 4. 创建上游分组配置
    spec := &proxy.UpstreamGroupsSpec{
        DefaultGroup: config.DNS.UpstreamGroups.DefaultGroup,
        Groups:       config.DNS.UpstreamGroups.Groups,
        DomainGroups: domainGroups,
    }
    
    // 5. 解析配置
    ugc, err := proxy.ParseUpstreamGroups(spec, opts)
    if err != nil {
        return fmt.Errorf("parse upstream groups: %w", err)
    }
    
    // 6. 启动定时更新
    startAutoUpdate()
    
    return nil
}
```

## 工作流程

### 用户添加列表

```
1. 用户在 UI 输入列表信息
   ↓
2. AdGuard Home 调用 manager.DownloadAndCache()
   ↓
3. 下载远程文件到本地缓存
   ↓
4. 调用 manager.AddList() 添加到管理器
   ↓
5. 保存配置到 AdGuardHome.yaml
   ↓
6. 重载 DNS 配置
```

### 自动更新

```
1. 定时任务触发
   ↓
2. 检查哪些列表需要更新
   ↓
3. 对每个列表调用 manager.DownloadAndCache()
   ↓
4. 更新成功后调用 manager.UpdateList()
   ↓
5. 重载 DNS 配置
```

### DNS 查询

```
1. DNS 查询到达
   ↓
2. ugc.GetGroupForDomain() 查找域名所属组
   ↓
3. 从本地缓存文件读取域名列表（已在内存中）
   ↓
4. 匹配成功，使用对应组的上游
   ↓
5. 返回查询结果
```

## 优势

### 1. 关注点分离

- **dnsproxy**: 专注核心功能（加载、解析、路由）
- **AdGuard Home**: 专注用户体验（UI、下载、管理）

### 2. 性能优化

- DNS 查询时只读取本地文件（< 1ms）
- 下载由 AdGuard Home 在后台处理
- 不阻塞 DNS 服务启动

### 3. 可靠性

- 本地缓存保证离线可用
- 下载失败不影响现有服务
- 可以手动管理和审查列表

### 4. 灵活性

- 用户可以选择使用远程列表或本地文件
- 可以自定义更新策略
- 可以混合使用多个来源

## 示例代码

### 完整的 AdGuard Home 集成示例

```go
package home

import (
    "github.com/AdguardTeam/dnsproxy/proxy"
)

type Home struct {
    domainListManager *proxy.DomainListManager
    dnsProxy          *proxy.Proxy
}

func (h *Home) initDomainLists() error {
    // 创建管理器
    h.domainListManager = proxy.NewDomainListManager(
        config.DNS.DomainLists.CacheDir,
        log.Default(),
    )
    
    // 加载配置的列表
    for _, cfg := range config.DNS.DomainLists.Lists {
        list := &proxy.ManagedList{
            Name:       cfg.Name,
            Source:     cfg.URL,
            LocalPath:  h.domainListManager.GetCachePath(cfg.URL),
            Group:      cfg.Group,
            Enabled:    cfg.Enabled,
            AutoUpdate: cfg.AutoUpdate,
        }
        
        h.domainListManager.AddList(list)
    }
    
    return nil
}

func (h *Home) handleAddDomainList(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Name       string `json:"name"`
        URL        string `json:"url"`
        Group      string `json:"group"`
        AutoUpdate bool   `json:"auto_update"`
    }
    
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    
    // 下载并缓存
    localPath := h.domainListManager.GetCachePath(req.URL)
    if err := h.domainListManager.DownloadAndCache(req.URL, localPath); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    // 添加列表
    list := &proxy.ManagedList{
        Name:       req.Name,
        Source:     req.URL,
        LocalPath:  localPath,
        Group:      req.Group,
        Enabled:    true,
        AutoUpdate: req.AutoUpdate,
    }
    
    if err := h.domainListManager.AddList(list); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    
    // 保存配置
    h.saveConfig()
    
    // 重载 DNS
    h.reloadDNS()
    
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(list)
}
```

## 总结

这个架构设计：

1. **dnsproxy 提供核心功能**:
   - `DomainListManager` - 列表管理
   - `DomainListCache` - 缓存管理
   - `DomainFileLoader` - 文件加载
   - `UpstreamGroupConfig` - 路由配置

2. **AdGuard Home 提供用户界面**:
   - Web UI 管理界面
   - 下载和更新逻辑
   - 定时任务
   - 配置持久化

3. **优势**:
   - 关注点分离
   - 高性能（本地文件）
   - 高可靠性（离线可用）
   - 用户友好（UI 管理）

这样的设计既保持了 dnsproxy 的核心功能完整性，又为 AdGuard Home 提供了灵活的集成接口。
