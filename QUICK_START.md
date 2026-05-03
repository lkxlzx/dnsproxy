# 快速开始指南

## 5 分钟快速上手

### 1. 创建配置文件

创建 `config.yaml`：

```yaml
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

default_group: overseas

domain_groups:
  china:
    - baidu.com
    - taobao.com
    - "*.cn"

domains_lists:
  - name: china-list
    source: https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/accelerated-domains.china.conf
    group: china
    file: ./cache/china.yaml
    auto_update: true
    refresh_interval: 6h
    enabled: true

cache:
  enabled: true
  directory: ./cache
  auto_update: true
```

### 2. 使用代码

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
    // 读取配置
    data, _ := os.ReadFile("config.yaml")
    
    var spec proxy.UpstreamGroupsSpec
    yaml.Unmarshal(data, &spec)
    
    // 创建 logger
    logger := slog.Default()
    
    // 解析配置
    opts := &upstream.Options{Logger: logger}
    ugc, err := proxy.ParseUpstreamGroups(&spec, opts)
    if err != nil {
        log.Fatal(err)
    }
    
    // 查询域名
    group, _ := ugc.GetGroupForDomain("baidu.com")
    logger.Info("routing", "domain", "baidu.com", "group", group.Name)
    // 输出: routing domain=baidu.com group=china
}
```

### 3. 运行

```bash
go run main.go
```

## 常见场景

### 场景 1: 国内外分流

```yaml
upstream_groups:
  - name: china
    upstreams: [223.5.5.5, 119.29.29.29]
    mode: load_balance
    enabled: true

  - name: overseas
    upstreams: [8.8.8.8, 1.1.1.1]
    mode: parallel
    enabled: true

default_group: overseas

domain_groups:
  china:
    - "*.cn"
    - "*.com.cn"
    - baidu.com
    - taobao.com
```

### 场景 2: 广告拦截

```yaml
upstream_groups:
  - name: normal
    upstreams: [8.8.8.8]
    enabled: true

  - name: adblock
    upstreams: [127.0.0.1:5353]  # 本地拦截服务
    enabled: true

domains_lists:
  - name: adblock-list
    source: https://adguardteam.github.io/AdGuardSDNSFilter/Filters/filter.txt
    group: adblock
    file: ./cache/adblock.yaml
    auto_update: true
    refresh_interval: 12h
    enabled: true
```

### 场景 3: 安全 DNS

```yaml
upstream_groups:
  - name: secure
    upstreams:
      - tls://dns.adguard.com
      - https://dns.google/dns-query
    mode: load_balance
    enabled: true

default_group: secure
```

## 测试

```bash
# 运行测试
go test -v ./proxy

# 测试特定功能
go test -v -run TestParseDomainLists ./proxy
```

## 下一步

- 阅读 [COMPLETE_USAGE_GUIDE.md](COMPLETE_USAGE_GUIDE.md) 了解详细用法
- 查看 [config-adguardhome-final.yaml](config-adguardhome-final.yaml) 完整配置示例
- 了解 [AUTO_REFRESH_FEATURE.md](AUTO_REFRESH_FEATURE.md) 自动刷新功能
- 学习 [YAML_CONVERSION_FEATURE.md](YAML_CONVERSION_FEATURE.md) YAML 转换

## 常见问题

### Q: 如何添加新的域名列表？

A: 在 `domains_lists` 中添加：

```yaml
domains_lists:
  - name: my-list
    source: https://example.com/list.txt
    group: my-group
    file: ./cache/my-list.yaml
    enabled: true
```

### Q: 如何禁用自动刷新？

A: 设置 `auto_update: false`：

```yaml
cache:
  auto_update: false
```

### Q: 支持哪些域名格式？

A: 支持 8 种格式，会自动检测和转换：
- Plain Text
- Dnsmasq
- GFWList
- Clash
- Surge
- Hosts
- AdBlock
- JSON

### Q: 如何查看日志？

A: 使用 slog：

```go
logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,  // Debug/Info/Warn/Error
}))
```

## 获取帮助

- 查看文档目录
- 运行测试了解用法
- 查看示例配置文件

---

**开始使用吧！** 🚀
