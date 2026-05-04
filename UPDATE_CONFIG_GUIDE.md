# 配置文件统计信息自动更新指南

## 功能说明

当域名列表从远程下载并缓存后，程序会自动更新配置文件中的 `domain_count` 和 `last_updated` 字段，方便前端 UI 直接读取这些统计信息。

## 工作流程

```
1. 读取配置文件
   ↓
2. 下载域名列表并缓存
   ↓
3. 统计域名数量和更新时间
   ↓
4. 写回配置文件
   ↓
5. 前端 UI 读取配置文件获取统计信息
```

## 配置文件格式

### 初始配置（domain_count 为 0）

```yaml
domains_lists:
  - name: china-domains
    source: https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/accelerated-domains.china.conf
    group: china-id
    file: ./cache/china-domains.yaml
    auto_update: true
    refresh_interval: 24h
    enabled: true
    format: dnsmasq
    domain_count: 0              # 初始值
    last_updated: ""             # 初始值
```

### 更新后的配置（自动填充）

```yaml
domains_lists:
  - name: china-domains
    source: https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/accelerated-domains.china.conf
    group: china-id
    file: ./cache/china-domains.yaml
    auto_update: true
    refresh_interval: 24h
    enabled: true
    format: dnsmasq
    domain_count: 114898                        # ✅ 自动更新
    last_updated: "2026-05-04T10:51:02+08:00"  # ✅ 自动更新
```

## 代码实现

### 1. 基本用法

```go
package main

import (
    "github.com/AdguardTeam/dnsproxy/proxy"
    "github.com/AdguardTeam/dnsproxy/upstream"
)

func main() {
    // 1. 解析配置文件
    spec := &proxy.UpstreamGroupsSpec{
        // ... 从配置文件加载
    }
    
    // 2. 解析上游组（会自动下载和缓存域名列表）
    opts := &upstream.Options{Logger: logger}
    _, err := proxy.ParseUpstreamGroups(spec, opts)
    if err != nil {
        log.Fatal(err)
    }
    
    // 3. 收集统计信息
    stats, err := proxy.CollectDomainListStats(spec)
    if err != nil {
        log.Fatal(err)
    }
    
    // 4. 更新配置文件
    err = proxy.UpdateConfigFileStats("config.yaml", stats)
    if err != nil {
        log.Fatal(err)
    }
}
```

### 2. 完整示例

参见 `test_config_update.go`：

```bash
# 运行测试
go run test_config_update.go

# 输出：
# ✓ Domain lists loaded and cached
# ✓ Configuration file updated
# ✅ Configuration file now contains updated statistics
```

### 3. 集成到主程序

在 `internal/cmd/proxy.go` 中添加：

```go
func startProxy(conf *configuration) error {
    // ... 现有代码 ...
    
    // 如果配置了 upstream_groups，解析并更新统计信息
    if len(conf.UpstreamGroups) > 0 {
        spec := &proxy.UpstreamGroupsSpec{
            Groups:       conf.UpstreamGroups,
            DomainGroups: conf.DomainGroups,
            DomainLists:  conf.DomainsLists,
            DefaultGroup: conf.DefaultGroup,
            Cache:        conf.CacheConfig,
        }
        
        // 解析上游组
        ugc, err := proxy.ParseUpstreamGroups(spec, opts)
        if err != nil {
            return err
        }
        
        // 收集统计信息
        stats, err := proxy.CollectDomainListStats(spec)
        if err == nil && len(stats) > 0 {
            // 更新配置文件
            _ = proxy.UpdateConfigFileStats(conf.ConfigPath, stats)
        }
        
        // 使用 ugc ...
    }
    
    // ... 现有代码 ...
}
```

## API 设计

### 核心函数

#### 1. CollectDomainListStats

收集域名列表的统计信息：

```go
func CollectDomainListStats(spec *UpstreamGroupsSpec) (map[string]DomainListStats, error)
```

**参数：**
- `spec`: 上游组配置规范

**返回：**
- `map[string]DomainListStats`: 列表名称到统计信息的映射
- `error`: 错误信息

**示例：**
```go
stats, err := proxy.CollectDomainListStats(spec)
// stats = {
//   "china-domains": {DomainCount: 114898, LastUpdated: time.Time},
//   "gfwlist": {DomainCount: 4161, LastUpdated: time.Time}
// }
```

#### 2. UpdateConfigFileStats

更新配置文件中的统计信息：

```go
func UpdateConfigFileStats(configPath string, stats map[string]DomainListStats) error
```

**参数：**
- `configPath`: 配置文件路径
- `stats`: 统计信息映射

**返回：**
- `error`: 错误信息

**特性：**
- ✅ 保留原有格式和注释
- ✅ 只更新 `domain_count` 和 `last_updated` 字段
- ✅ 如果字段不存在，自动添加
- ✅ 使用 YAML Node API 精确更新

#### 3. DomainListStats

统计信息结构：

```go
type DomainListStats struct {
    DomainCount int       // 域名数量
    LastUpdated time.Time // 最后更新时间
}
```

## 前端集成

### 读取配置文件

前端可以直接读取配置文件获取统计信息：

```javascript
// 读取配置文件
fetch('/api/config')
  .then(res => res.json())
  .then(config => {
    config.domains_lists.forEach(list => {
      console.log(`${list.name}:`);
      console.log(`  域名数量: ${list.domain_count}`);
      console.log(`  最后更新: ${list.last_updated}`);
    });
  });
```

### 显示统计信息

```jsx
function DomainListCard({ list }) {
  return (
    <div className="card">
      <h3>{list.name}</h3>
      <div className="stats">
        <div className="stat">
          <label>域名数量</label>
          <value>{list.domain_count.toLocaleString()}</value>
        </div>
        <div className="stat">
          <label>最后更新</label>
          <value>{new Date(list.last_updated).toLocaleString()}</value>
        </div>
      </div>
      <div className="info">
        <p>来源: {list.source}</p>
        <p>分组: {list.group}</p>
        <p>格式: {list.format}</p>
      </div>
    </div>
  );
}
```

### 手动刷新

```javascript
// 触发手动更新
async function refreshDomainList(listName) {
  const response = await fetch(`/api/domain-lists/${listName}/refresh`, {
    method: 'POST'
  });
  
  if (response.ok) {
    // 重新读取配置文件获取更新后的统计信息
    const config = await fetch('/api/config').then(r => r.json());
    updateUI(config);
  }
}
```

## 自动更新机制

### 启动时更新

程序启动时自动检查并更新：

```go
func main() {
    // 加载配置
    config := loadConfig("config.yaml")
    
    // 解析上游组（会下载域名列表）
    ugc, _ := proxy.ParseUpstreamGroups(spec, opts)
    
    // 更新配置文件统计信息
    stats, _ := proxy.CollectDomainListStats(spec)
    proxy.UpdateConfigFileStats("config.yaml", stats)
    
    // 启动服务器
    startServer(ugc)
}
```

### 定期更新

配合自动刷新机制：

```go
// 在 DomainListManager 的 refreshExpiredLists 中
func (m *DomainListManager) refreshExpiredLists(defaultInterval time.Duration) {
    // ... 刷新列表 ...
    
    // 刷新后更新配置文件
    if len(refreshedLists) > 0 {
        stats, _ := proxy.CollectDomainListStats(spec)
        proxy.UpdateConfigFileStats(configPath, stats)
    }
}
```

## 测试

### 单元测试

```bash
# 运行配置更新测试
go run test_config_update.go
```

### 验证结果

```bash
# 查看更新后的配置文件
cat test-cache-config.yaml | grep -A 2 "domain_count"

# 输出：
# domain_count: 114898
# last_updated: "2026-05-04T10:51:02+08:00"
```

## 注意事项

### 1. 文件权限

确保程序有写入配置文件的权限：

```bash
chmod 644 config.yaml
```

### 2. 并发安全

如果多个进程同时运行，需要添加文件锁：

```go
import "github.com/gofrs/flock"

func UpdateConfigFileStats(configPath string, stats map[string]DomainListStats) error {
    // 获取文件锁
    lock := flock.New(configPath + ".lock")
    lock.Lock()
    defer lock.Unlock()
    
    // 更新配置文件
    // ...
}
```

### 3. 备份配置

更新前建议备份：

```go
func UpdateConfigFileStats(configPath string, stats map[string]DomainListStats) error {
    // 备份原配置
    backupPath := configPath + ".backup"
    data, _ := os.ReadFile(configPath)
    os.WriteFile(backupPath, data, 0644)
    
    // 更新配置
    // ...
}
```

### 4. 格式保留

使用 `yaml.Node` API 确保保留原有格式：

- ✅ 保留注释
- ✅ 保留缩进
- ✅ 保留字段顺序
- ✅ 只更新必要字段

## 最佳实践

### 1. 初始配置

配置文件中可以省略这两个字段，程序会自动添加：

```yaml
domains_lists:
  - name: gfwlist
    source: https://...
    group: overseas
    file: ./cache/gfwlist.yaml
    enabled: true
    # domain_count 和 last_updated 会自动添加
```

### 2. 只读模式

如果不想更新配置文件，可以只读取统计信息：

```go
stats, _ := proxy.CollectDomainListStats(spec)
// 不调用 UpdateConfigFileStats
```

### 3. 状态文件

也可以将统计信息保存到单独的状态文件：

```go
// 保存到 state.yaml 而不是 config.yaml
proxy.UpdateConfigFileStats("state.yaml", stats)
```

## 故障排查

### 配置文件未更新

1. 检查文件权限
2. 检查是否有错误日志
3. 验证缓存文件是否存在

### 统计信息不准确

1. 检查缓存文件格式
2. 重新下载域名列表
3. 手动验证域名数量

### YAML 格式错误

1. 使用 YAML 验证工具检查
2. 恢复备份文件
3. 重新生成配置

## 总结

✅ **自动更新**：域名列表下载后自动更新配置文件
✅ **前端友好**：前端可以直接读取配置文件获取统计信息
✅ **格式保留**：保留原有的注释和格式
✅ **易于集成**：简单的 API 调用即可实现

这个功能让前端 UI 可以方便地显示域名列表的统计信息，无需额外的 API 端点。
