# 完整使用指南

## 配置文件结构

最新的配置文件支持三个主要部分：

### 1. 上游服务器分组 (upstream_groups)

定义 DNS 上游服务器组：

```yaml
upstream_groups:
  - name: china          # 分组名称（支持中文）
    upstreams:
      - 223.5.5.5
      - 119.29.29.29
    mode: load_balance   # 模式：load_balance/parallel/fastest_addr
    timeout: 5s
    enabled: true
```

### 2. 域名分组 (domain_groups)

直接指定域名到分组的映射：

```yaml
domain_groups:
  china:                 # 分组名称
    - baidu.com          # 直接域名
    - "*.cn"             # 通配符域名
    - taobao.com
```

### 3. 域名列表 (domains_lists)

从远程下载域名列表并自动转换为 YAML：

```yaml
domains_lists:
  - name: china                    # 列表名称
    source: https://example.com/china.conf  # 远程 URL
    group: china                   # 关联到哪个分组
    file: ./cache/china.yaml       # 本地 YAML 文件（自动转换）
    auto_update: true              # 自动更新
    refresh_interval: 6h           # 刷新间隔
    enabled: true
    format: dnsmasq                # 源格式（可选）
```

## 工作流程

```
1. 启动时加载配置
   ↓
2. 解析 upstream_groups（创建上游组）
   ↓
3. 处理 domains_lists（下载远程列表）
   ↓
4. 自动转换为 YAML 格式
   ↓
5. 保存到本地缓存
   ↓
6. 加载 domain_groups（直接域名映射）
   ↓
7. 构建域名路由表
   ↓
8. 启动自动刷新（后台定期更新）
```

## 完整配置示例

```yaml
# 上游服务器分组
upstream_groups:
  - name: china
    upstreams:
      - 223.5.5.5
      - 119.29.29.29
    mode: load_balance
    timeout: 5s
    enabled: true

  - name: overseas
    upstreams:
      - 8.8.8.8
      - 1.1.1.1
    mode: parallel
    timeout: 10s
    enabled: true

# 默认分组
default_group: overseas

# 直接域名映射
domain_groups:
  china:
    - baidu.com
    - taobao.com
    - "*.cn"

# 远程域名列表
domains_lists:
  - name: china-list
    source: https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/accelerated-domains.china.conf
    group: china
    file: ./cache/china.yaml
    auto_update: true
    refresh_interval: 6h
    enabled: true
    format: dnsmasq

  - name: gfwlist
    source: https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt
    group: overseas
    file: ./cache/gfwlist.yaml
    auto_update: true
    refresh_interval: 24h
    enabled: true
    format: gfwlist

# 缓存配置
cache:
  enabled: true
  directory: ./cache
  ttl: 24h
  default_refresh_interval: 24h
  auto_update: true
```

## 转换后的 YAML 格式

远程列表下载后会自动转换为：

```yaml
# Generated from: https://example.com/china.conf
# Generated at: "2026-05-03T19:00:00+08:00"
# Total domains: 1000

domains:
  - baidu.com
  - taobao.com
  - qq.com
  - weixin.qq.com
  # ... 更多域名
```

## 代码使用示例

### 加载配置

```go
package main

import (
    "log"
    "log/slog"
    "os"
    
    "github.com/AdguardTeam/dnsproxy/proxy"
    "github.com/AdguardTeam/dnsproxy/upstream"
    "gopkg.in/yaml.v3"
)

func main() {
    // 读取配置文件
    data, err := os.ReadFile("config.yaml")
    if err != nil {
        log.Fatal(err)
    }

    // 解析配置
    var spec proxy.UpstreamGroupsSpec
    if err := yaml.Unmarshal(data, &spec); err != nil {
        log.Fatal(err)
    }

    // 创建 logger
    logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    }))

    // 解析上游组配置
    opts := &upstream.Options{
        Logger:  logger,
        Timeout: 5000,
    }

    ugc, err := proxy.ParseUpstreamGroups(&spec, opts)
    if err != nil {
        log.Fatal(err)
    }

    // 使用配置进行 DNS 查询
    group, err := ugc.GetGroupForDomain("baidu.com")
    if err != nil {
        log.Fatal(err)
    }

    logger.Info("domain routing", 
        "domain", "baidu.com",
        "group", group.Name)
}
```

### 手动下载和转换

```go
// 创建管理器
manager := proxy.NewDomainListManager("./cache", logger)

// 下载并转换为 YAML
err := manager.DownloadAndCache(
    "https://example.com/domains.txt",
    "./cache/domains.yaml",
)
if err != nil {
    log.Fatal(err)
}

// 加载 YAML 文件
loader := proxy.NewDomainFileLoader(logger)
domains, err := loader.LoadDomains("./cache/domains.yaml")
if err != nil {
    log.Fatal(err)
}

logger.Info("loaded domains", "count", len(domains))
```

### 自动刷新

```go
// 创建管理器
manager := proxy.NewDomainListManager("./cache", logger)

// 添加列表
list := &proxy.ManagedList{
    Name:            "china",
    Source:          "https://example.com/china.txt",
    Group:           "china",
    Enabled:         true,
    AutoUpdate:      true,
    RefreshInterval: 6 * time.Hour,
}

manager.AddList(list)

// 启动自动刷新
manager.StartAutoRefresh(
    24*time.Hour,  // 默认刷新间隔
    1*time.Hour,   // 检查间隔
)

// 程序会在后台自动刷新过期的列表
```

## 支持的源格式

系统自动检测并转换以下格式：

1. **Plain Text** - 每行一个域名
2. **Dnsmasq** - `server=/domain/`
3. **GFWList** - Base64 编码
4. **Clash** - YAML 规则
5. **Surge** - `DOMAIN-SUFFIX,domain`
6. **Hosts** - `IP domain`
7. **AdBlock** - `||domain^`
8. **JSON** - 数组或对象

所有格式都会转换为统一的 YAML 格式。

## 性能优化

### 1. 使用 Radix Tree

系统自动使用 Radix Tree 优化域名匹配：

- 精确匹配：O(1) 哈希查找
- 通配符匹配：O(log n) Radix Tree 查找
- 内存优化：压缩存储

### 2. 缓存机制

- 远程列表缓存到本地
- YAML 格式快速加载
- 自动清理过期缓存

### 3. 并发安全

- 所有操作都是并发安全的
- 使用读写锁优化性能
- 支持多 goroutine 访问

## 测试

```bash
# 测试配置解析
go test -v -run TestParseDomainLists ./proxy

# 测试自动刷新
go test -v -run TestDomainListManager_NeedsRefresh ./proxy

# 测试 YAML 转换
go test -v -run TestParseDomainLists_WithRefreshInterval ./proxy

# 运行所有测试
go test -v ./proxy
```

## 故障排查

### 1. 下载失败

```
Error: download and parse failed: http get: ...
```

**解决方案**：
- 检查网络连接
- 验证 URL 是否正确
- 检查防火墙设置

### 2. 格式解析失败

```
Error: parse domains: unsupported format
```

**解决方案**：
- 指定 `format` 字段
- 检查源文件格式
- 查看日志了解详情

### 3. 缓存目录权限

```
Error: create directory: permission denied
```

**解决方案**：
- 检查目录权限
- 使用绝对路径
- 确保目录存在

## 最佳实践

### 1. 合理设置刷新间隔

```yaml
# 频繁更新的列表
refresh_interval: 6h

# 稳定的列表
refresh_interval: 168h  # 7 天
```

### 2. 使用本地缓存

```yaml
# 总是指定 file 字段
file: ./cache/list-name.yaml
```

### 3. 启用自动更新

```yaml
cache:
  auto_update: true
  default_refresh_interval: 24h
```

### 4. 监控日志

```go
logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,  // 生产环境使用 Info
}))
```

## 相关文档

- [AUTO_REFRESH_FEATURE.md](AUTO_REFRESH_FEATURE.md) - 自动刷新功能
- [YAML_CONVERSION_FEATURE.md](YAML_CONVERSION_FEATURE.md) - YAML 转换功能
- [config-adguardhome-final.yaml](config-adguardhome-final.yaml) - 完整配置示例

## 更新日志

- **2026-05-03**: 完整功能实现
  - 支持 `domains_lists` 配置
  - 自动 YAML 转换
  - 自动刷新机制
  - 中文分组名称支持
  - 完整测试覆盖
