# 动态域名分组功能

## 概述

动态域名分组功能允许你在运行时管理域名路由规则，无需重启 DNS 代理服务。这对于需要实时启用/禁用规则的场景非常有用，例如：

- 临时禁用广告拦截规则
- 动态切换不同的 DNS 上游服务器
- 实时更新域名列表而不中断服务

## 核心特性

### 1. 运行时规则管理

- **启用/禁用规则组**：无需重启即可启用或禁用整个规则组
- **重新加载域名列表**：从文件重新加载域名列表，立即生效
- **查询规则状态**：实时查看所有规则组的状态和统计信息

### 2. 与静态配置的区别

| 特性 | 静态配置 (UpstreamConfig) | 动态分组 (DomainGroups) |
|------|--------------------------|------------------------|
| 配置时转换 | ✅ 加载时转换为上游配置 | ❌ 保留原始域名列表 |
| 运行时修改 | ❌ 需要重启 | ✅ 实时生效 |
| 启用/禁用 | ❌ 不支持 | ✅ 支持 |
| 重新加载 | ❌ 需要重启 | ✅ 支持 |
| 性能 | 更快（预编译） | 稍慢（运行时匹配） |

## 使用方法

### 配置域名分组

```go
domainGroups := []proxy.DomainGroupConfig{
    {
        GroupName:        "china-domains",
        DomainFile:       "china_domains.txt",
        DomainFileFormat: proxy.FormatPlain,
        Upstreams:        []string{"223.5.5.5", "119.29.29.29"},
        SubdomainsOnly:   false,
        Enabled:          true,
    },
    {
        GroupName:        "blocked-ads",
        DomainFile:       "ad_domains.txt",
        DomainFileFormat: proxy.FormatPlain,
        Upstreams:        []string{"0.0.0.0"},
        SubdomainsOnly:   false,
        Enabled:          true,
    },
}

config := &proxy.Config{
    // ... 其他配置 ...
    DomainGroups: domainGroups,
}

dnsProxy, err := proxy.New(config)
```

### 运行时管理

#### 禁用规则组

```go
err := dnsProxy.DisableDomainGroup("blocked-ads")
if err != nil {
    log.Printf("Failed to disable group: %v", err)
}
```

#### 启用规则组

```go
err := dnsProxy.EnableDomainGroup("blocked-ads")
if err != nil {
    log.Printf("Failed to enable group: %v", err)
}
```

#### 重新加载域名列表

```go
// 修改域名文件后，重新加载
err := dnsProxy.ReloadDomainGroup("china-domains")
if err != nil {
    log.Printf("Failed to reload group: %v", err)
}
```

#### 查询规则状态

```go
groups := dnsProxy.GetDomainGroups()
for _, g := range groups {
    fmt.Printf("Group: %s\n", g.GroupName)
    fmt.Printf("  Enabled: %v\n", g.Enabled)
    fmt.Printf("  Domains: %d\n", g.DomainCount)
    fmt.Printf("  Upstreams: %d\n", g.UpstreamCount)
}
```

## 配置参数说明

### DomainGroupConfig

| 字段 | 类型 | 说明 |
|------|------|------|
| GroupName | string | 规则组名称（用于管理操作） |
| DomainFile | string | 域名列表文件路径 |
| DomainFileFormat | DomainListFormat | 文件格式（Plain/Dnsmasq/GFWList/Clash） |
| Upstreams | []string | 该组使用的上游 DNS 服务器 |
| SubdomainsOnly | bool | 是否仅匹配子域名 |
| Enabled | bool | 是否启用（默认 true） |

### 域名匹配规则

#### 普通域名格式

```
example.com
```

匹配域名本身及其所有子域名：
- `example.com` ✓ 匹配
- `www.example.com` ✓ 匹配
- `api.example.com` ✓ 匹配
- `sub.api.example.com` ✓ 匹配

#### 通配符格式

```
*.example.com
```

**仅**匹配子域名，不匹配域名本身：
- `example.com` ✗ 不匹配
- `www.example.com` ✓ 匹配
- `api.example.com` ✓ 匹配
- `sub.api.example.com` ✓ 匹配

#### 关键字匹配

```
keyword:ad
```

匹配包含该关键字的**任何**域名（不区分大小写）：
- `ad.example.com` ✓ 匹配
- `www.ad-server.com` ✓ 匹配
- `adservice.google.com` ✓ 匹配
- `my-ad-network.com` ✓ 匹配
- `example.com` ✗ 不匹配

**使用场景**：
- 广告拦截：`keyword:ad`, `keyword:ads`, `keyword:tracker`
- 分析追踪：`keyword:analytics`, `keyword:telemetry`
- CDN 匹配：`keyword:cdn`, `keyword:static`

**注意**：关键字匹配性能略低于精确匹配，建议优先使用精确域名或通配符。

#### SubdomainsOnly 参数

当 `SubdomainsOnly = true` 时，所有域名都按通配符方式处理：

```go
DomainGroupConfig{
    DomainFile:     "domains.txt",  // 内容: example.com
    SubdomainsOnly: true,
}
```

效果等同于域名文件中写 `*.example.com`

#### 混合使用

域名文件可以混合使用多种格式：

```
# domains.txt
example.com              # 匹配 example.com 及其子域名
*.cdn.example.com        # 仅匹配 cdn.example.com 的子域名
keyword:ad               # 匹配包含 "ad" 的任何域名
test.org                 # 匹配 test.org 及其子域名
```

匹配结果：
- `example.com` ✓ (普通匹配)
- `www.example.com` ✓ (普通匹配)
- `cdn.example.com` ✓ (普通匹配)
- `img.cdn.example.com` ✓ (通配符匹配)
- `ad.example.com` ✓ (关键字匹配)
- `adservice.google.com` ✓ (关键字匹配)
- `test.org` ✓ (普通匹配)
- `api.test.org` ✓ (普通匹配)

## 工作原理

### 查询流程

```
DNS 查询
  ↓
自定义上游配置？
  ↓ 否
域名分组匹配？
  ↓ 否
默认上游配置
```

1. **自定义上游优先**：如果请求指定了自定义上游，优先使用
2. **域名分组匹配**：检查是否匹配任何启用的域名分组
3. **默认上游**：如果没有匹配，使用默认上游配置

### 性能考虑

- **域名列表缓存**：域名列表在加载时缓存在内存中
- **读写锁保护**：使用 RWMutex 保护并发访问
- **快速匹配**：使用字符串后缀匹配，性能良好
- **建议**：对于大量域名（>10000），考虑使用静态配置以获得更好的性能

## 使用场景

### 场景 1：分地区 DNS 路由

```go
domainGroups := []proxy.DomainGroupConfig{
    {
        GroupName:  "china-domains",
        DomainFile: "china_domains.txt",
        Upstreams:  []string{"223.5.5.5"}, // 国内 DNS
    },
    {
        GroupName:  "global-domains",
        DomainFile: "global_domains.txt",
        Upstreams:  []string{"1.1.1.1"}, // 国际 DNS
    },
}
```

### 场景 2：广告拦截

```go
domainGroups := []proxy.DomainGroupConfig{
    {
        GroupName:  "ad-blocking",
        DomainFile: "ad_domains.txt",
        Upstreams:  []string{"0.0.0.0"}, // 返回无效地址
    },
}

// 临时禁用广告拦截
dnsProxy.DisableDomainGroup("ad-blocking")

// 稍后重新启用
dnsProxy.EnableDomainGroup("ad-blocking")
```

**ad_domains.txt 示例**：
```
# 使用关键字匹配拦截所有广告相关域名
keyword:ad
keyword:ads
keyword:adservice
keyword:tracker
keyword:analytics

# 精确域名
doubleclick.net
googlesyndication.com
```

### 场景 3：动态更新规则

```go
// 定期更新域名列表
ticker := time.NewTicker(1 * time.Hour)
go func() {
    for range ticker.C {
        // 下载最新的域名列表
        downloadLatestDomainList("ad_domains.txt")
        
        // 重新加载
        dnsProxy.ReloadDomainGroup("ad-blocking")
        log.Println("Ad blocking rules updated")
    }
}()
```

## API 参考

### Proxy 方法

#### EnableDomainGroup

```go
func (p *Proxy) EnableDomainGroup(groupName string) error
```

启用指定的域名分组。

#### DisableDomainGroup

```go
func (p *Proxy) DisableDomainGroup(groupName string) error
```

禁用指定的域名分组。

#### ReloadDomainGroup

```go
func (p *Proxy) ReloadDomainGroup(groupName string) error
```

重新加载指定域名分组的域名列表。

#### GetDomainGroups

```go
func (p *Proxy) GetDomainGroups() []DomainGroupStatus
```

获取所有域名分组的状态信息。

### DomainGroupStatus

```go
type DomainGroupStatus struct {
    GroupName      string  // 分组名称
    DomainFile     string  // 域名文件路径
    Enabled        bool    // 是否启用
    DomainCount    int     // 域名数量
    UpstreamCount  int     // 上游服务器数量
    SubdomainsOnly bool    // 是否仅匹配子域名
}
```

## 示例程序

完整的示例程序请参考：`example_dynamic_domain_groups.go`

运行示例：

```bash
go run example_dynamic_domain_groups.go
```

## 注意事项

1. **线程安全**：所有 API 都是线程安全的，可以在多个 goroutine 中调用
2. **文件格式**：支持多种域名列表格式（Plain/Dnsmasq/GFWList/Clash）
3. **性能影响**：动态匹配会有轻微的性能开销，对于大多数场景可以忽略
4. **错误处理**：所有操作都会返回错误，请妥善处理
5. **配置持久化**：运行时的修改不会自动保存到配置文件

## 与其他功能的集成

### 与缓存配合使用

域名分组的匹配结果会被缓存，提高性能：

```go
config := &proxy.Config{
    DomainGroups:   domainGroups,
    CacheEnabled:   true,
    CacheSizeBytes: 64 * 1024 * 1024,
}
```

### 与预取功能配合

域名分组可以与缓存预取功能配合使用：

```go
config := &proxy.Config{
    DomainGroups: domainGroups,
    CachePrefetchConfig: &proxy.PrefetchConfig{
        Enabled:           true,
        PrefetchThreshold: 0.1,
    },
}
```

## 故障排查

### 规则不生效

1. 检查规则组是否启用：`GetDomainGroups()`
2. 检查域名文件是否存在且格式正确
3. 检查域名匹配规则（SubdomainsOnly）

### 性能问题

1. 减少域名分组数量
2. 考虑使用静态配置（UpstreamConfig）
3. 启用缓存以减少重复查询

### 文件加载失败

1. 检查文件路径是否正确
2. 检查文件权限
3. 检查文件格式是否正确
