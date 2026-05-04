# 热重载功能完整指南

## 🔥 功能概述

热重载功能允许你在**不重启程序**的情况下：
- ✅ 动态添加新的域名列表
- ✅ 动态删除域名列表
- ✅ 动态更新域名列表配置
- ✅ 自动检测配置文件变化
- ✅ 自动下载并加载新的域名列表

## 🚀 快速开始

### 1. 启动热重载服务器

```bash
go run test_hotreload.go
```

服务器将在 `http://localhost:8080` 启动，并自动监控配置文件变化。

### 2. 添加新的域名列表

```bash
curl -X POST http://localhost:8080/api/domain-lists \
  -H "Content-Type: application/json" \
  -d '{
    "name": "anti-ad",
    "source": "https://raw.githubusercontent.com/privacy-protection-tools/anti-AD/master/anti-ad-domains.txt",
    "group": "adblock",
    "enabled": true,
    "auto_update": true,
    "refresh_interval": "12h",
    "format": "hosts"
  }'
```

**响应:**
```json
{
  "success": true,
  "message": "Domain list \"anti-ad\" added successfully",
  "list": {
    "name": "anti-ad",
    "source": "https://...",
    "group": "adblock",
    ...
  }
}
```

**自动执行的操作:**
1. ✅ 添加到配置文件
2. ✅ 从远程 URL 下载域名列表
3. ✅ 解析并转换为 YAML 格式
4. ✅ 保存到缓存文件
5. ✅ 更新域名路由规则
6. ✅ 更新统计信息到配置文件
7. ✅ **立即生效，无需重启**

## 📡 API 端点

### 1. 添加域名列表

**请求:**
```http
POST /api/domain-lists
Content-Type: application/json

{
  "name": "list-name",
  "source": "https://example.com/list.txt",
  "group": "group-id",
  "file": "./cache/list-name.yaml",
  "enabled": true,
  "auto_update": true,
  "refresh_interval": "24h",
  "format": "hosts"
}
```

**必填字段:**
- `name`: 列表名称（唯一）
- `source`: 源 URL 或文件路径
- `group`: 目标上游组

**可选字段:**
- `file`: 缓存文件路径（默认: `./cache/{name}.yaml`）
- `enabled`: 是否启用（默认: `true`）
- `auto_update`: 是否自动更新（默认: `false`）
- `refresh_interval`: 刷新间隔（默认: `24h`）
- `format`: 文件格式（自动检测）

**响应:**
```json
{
  "success": true,
  "message": "Domain list \"list-name\" added successfully",
  "list": { ... }
}
```

### 2. 删除域名列表

**请求:**
```http
DELETE /api/domain-lists/remove?name=list-name
```

**响应:**
```json
{
  "success": true,
  "message": "Domain list \"list-name\" removed successfully"
}
```

### 3. 更新域名列表

**请求:**
```http
PUT /api/domain-lists/update?name=list-name
Content-Type: application/json

{
  "enabled": false,
  "refresh_interval": "12h"
}
```

**可更新字段:**
- `source`: 源 URL
- `group`: 目标组
- `enabled`: 启用状态
- `auto_update`: 自动更新
- `refresh_interval`: 刷新间隔

**响应:**
```json
{
  "success": true,
  "message": "Domain list \"list-name\" updated successfully"
}
```

### 4. 手动重载配置

**请求:**
```http
POST /api/reload
```

**响应:**
```json
{
  "success": true,
  "message": "Configuration reloaded successfully"
}
```

### 5. 获取所有列表

**请求:**
```http
GET /api/domain-lists
```

**响应:**
```json
{
  "lists": [
    {
      "name": "china-domains",
      "source": "https://...",
      "group": "china-id",
      "enabled": true,
      "domain_count": 114898,
      "last_updated": "2026-05-04T11:19:15+08:00",
      ...
    }
  ],
  "timestamp": "2026-05-04T11:19:16+08:00"
}
```

### 6. 健康检查

**请求:**
```http
GET /api/health
```

**响应:**
```json
{
  "status": "healthy",
  "groups": 3,
  "domain_mappings": 119039,
  "timestamp": "2026-05-04T11:19:16+08:00"
}
```

## 🔄 自动重载机制

### 配置文件监控

热重载管理器会自动监控配置文件的修改时间：

```go
// 每 5 秒检查一次配置文件
hotReload.Start(5 * time.Second)
```

当检测到配置文件变化时：
1. 读取新的配置文件
2. 解析配置
3. 下载新的域名列表
4. 更新路由规则
5. 调用 `onChange` 回调
6. **立即生效**

### 手动触发重载

```bash
# 方式 1: 通过 API
curl -X POST http://localhost:8080/api/reload

# 方式 2: 修改配置文件
# 编辑 config.yaml 后会自动检测并重载
```

## 💻 代码集成

### 基本用法

```go
package main

import (
    "github.com/AdguardTeam/dnsproxy/proxy"
    "github.com/AdguardTeam/dnsproxy/upstream"
)

func main() {
    // 加载配置
    spec := loadConfig("config.yaml")
    
    // 创建热重载管理器
    hotReload := proxy.NewHotReloadManager(
        "config.yaml",
        spec,
        &upstream.Options{},
        func(ugc *proxy.UpstreamGroupConfig) error {
            // 配置重载后的回调
            fmt.Printf("Config reloaded: %d groups\n", len(ugc.Groups))
            return nil
        },
    )
    
    // 启动自动监控（每 5 秒检查一次）
    hotReload.Start(5 * time.Second)
    
    // 程序继续运行...
}
```

### 集成到 HTTP 服务器

```go
// 创建 API 处理器
apiHandler := proxy.NewAPIHandler(hotReload)

// 注册路由
http.HandleFunc("/api/domain-lists", apiHandler.HandleAddDomainList)
http.HandleFunc("/api/domain-lists/remove", apiHandler.HandleRemoveDomainList)
http.HandleFunc("/api/domain-lists/update", apiHandler.HandleUpdateDomainList)
http.HandleFunc("/api/reload", apiHandler.HandleReloadConfig)

// 启动服务器
http.ListenAndServe(":8080", nil)
```

### 动态添加列表

```go
// 添加新的域名列表
newList := proxy.DomainListSpec{
    Name:            "anti-ad",
    Source:          "https://example.com/anti-ad.txt",
    Group:           "adblock",
    File:            "./cache/anti-ad.yaml",
    Enabled:         true,
    AutoUpdate:      true,
    RefreshInterval: "12h",
    Format:          "hosts",
}

err := hotReload.AddDomainList(newList)
if err != nil {
    log.Fatal(err)
}

// 列表会自动下载并立即生效
```

## 🎯 使用场景

### 场景 1: 用户通过 UI 添加新规则

```javascript
// 前端代码
async function addDomainList(list) {
  const response = await fetch('http://localhost:8080/api/domain-lists', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(list)
  });
  
  const result = await response.json();
  if (result.success) {
    alert('域名列表添加成功！');
    // 刷新列表显示
    refreshLists();
  }
}

// 调用
addDomainList({
  name: 'my-custom-list',
  source: 'https://example.com/my-list.txt',
  group: 'overseas',
  enabled: true,
  auto_update: true,
  refresh_interval: '24h'
});
```

### 场景 2: 管理员手动编辑配置文件

```yaml
# 编辑 config.yaml
domains_lists:
  - name: new-list
    source: https://example.com/new-list.txt
    group: china
    enabled: true
    auto_update: true
    refresh_interval: 6h
```

保存后，系统会在 5 秒内自动检测并重载。

### 场景 3: 定时任务更新规则

```bash
#!/bin/bash
# cron job: 每天凌晨 2 点更新规则

curl -X POST http://localhost:8080/api/domain-lists \
  -H "Content-Type: application/json" \
  -d @new-list.json

# 或者直接触发重载
curl -X POST http://localhost:8080/api/reload
```

## 🔒 安全考虑

### 1. 认证

建议添加 API 认证：

```go
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        token := r.Header.Get("Authorization")
        if token != "Bearer your-secret-token" {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        next(w, r)
    }
}

http.HandleFunc("/api/domain-lists", authMiddleware(apiHandler.HandleAddDomainList))
```

### 2. 输入验证

```go
// 验证 URL
if !strings.HasPrefix(list.Source, "http://") && !strings.HasPrefix(list.Source, "https://") {
    return errors.New("invalid source URL")
}

// 验证名称
if !regexp.MustCompile(`^[a-zA-Z0-9-_]+$`).MatchString(list.Name) {
    return errors.New("invalid list name")
}
```

### 3. 限流

```go
import "golang.org/x/time/rate"

limiter := rate.NewLimiter(1, 5) // 每秒 1 个请求，突发 5 个

func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if !limiter.Allow() {
            http.Error(w, "Too many requests", http.StatusTooManyRequests)
            return
        }
        next(w, r)
    }
}
```

## 📊 监控和日志

### 日志记录

```go
hotReload := proxy.NewHotReloadManager(
    configPath,
    spec,
    opts,
    func(ugc *proxy.UpstreamGroupConfig) error {
        logger.Info("config reloaded",
            "groups", len(ugc.Groups),
            "domains", len(ugc.DomainGroups),
            "timestamp", time.Now())
        return nil
    },
)
```

### Prometheus 指标

```go
var (
    reloadCounter = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "config_reloads_total",
            Help: "Total number of config reloads",
        },
    )
    
    domainListsGauge = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "domain_lists_count",
            Help: "Current number of domain lists",
        },
    )
)

func (hrm *HotReloadManager) Reload() error {
    // ... reload logic ...
    
    reloadCounter.Inc()
    domainListsGauge.Set(float64(len(hrm.spec.DomainLists)))
    
    return nil
}
```

## 🧪 测试

### 单元测试

```go
func TestHotReload(t *testing.T) {
    // 创建临时配置文件
    tmpFile := createTempConfig(t)
    defer os.Remove(tmpFile)
    
    // 创建热重载管理器
    hotReload := proxy.NewHotReloadManager(tmpFile, spec, opts, nil)
    
    // 添加域名列表
    err := hotReload.AddDomainList(newList)
    assert.NoError(t, err)
    
    // 验证配置已更新
    config := loadConfig(tmpFile)
    assert.Len(t, config.DomainsLists, 3)
}
```

### 集成测试

```bash
# 运行完整测试
go run test_hotreload.go &
SERVER_PID=$!

# 测试添加
bash test_add_domain_list.sh

# 清理
kill $SERVER_PID
```

## 📝 最佳实践

### 1. 配置文件备份

```go
func (hrm *HotReloadManager) Reload() error {
    // 备份当前配置
    backup := hrm.configPath + ".backup"
    data, _ := os.ReadFile(hrm.configPath)
    os.WriteFile(backup, data, 0644)
    
    // 重载配置
    // ...
}
```

### 2. 回滚机制

```go
func (hrm *HotReloadManager) Reload() error {
    oldSpec := hrm.spec
    oldUGC := hrm.ugc
    
    // 尝试重载
    if err := hrm.doReload(); err != nil {
        // 回滚到旧配置
        hrm.spec = oldSpec
        hrm.ugc = oldUGC
        return err
    }
    
    return nil
}
```

### 3. 渐进式更新

```go
// 先验证新配置
newUGC, err := proxy.ParseUpstreamGroups(newSpec, opts)
if err != nil {
    return fmt.Errorf("invalid config: %w", err)
}

// 验证通过后再应用
hrm.ugc = newUGC
```

## 🎉 总结

热重载功能让你可以：

✅ **动态添加域名列表** - 通过 API 或编辑配置文件
✅ **自动下载和加载** - 无需手动操作
✅ **立即生效** - 无需重启程序
✅ **自动监控** - 配置文件变化自动检测
✅ **完整的 API** - 支持增删改查
✅ **安全可靠** - 支持认证、验证、限流

**现在你可以在运行时动态管理域名列表了！** 🚀
