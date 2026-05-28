# DNS上游分组 - 从文件加载域名

## 功能说明

这个功能允许你从配置文件中读取域名列表，并为这些域名指定专用的DNS上游服务器。支持两种格式：

- **YAML格式** - 兼容AdGuard Home (ADGH)的配置格式 ✨
- **文本格式** - 简单的域名列表文件

这对于以下场景非常有用：

- 🇨🇳 中国域名使用国内DNS（加速访问）
- 🏢 公司内网域名使用内网DNS
- 🔒 特定域名使用特定DNS（隐私、安全）
- 📋 大量域名管理（从文件批量导入）

## 快速开始

### 方法1: YAML配置文件（推荐，ADGH兼容）

#### 1.1 创建YAML配置文件

创建 `upstream_config.yaml`：

```yaml
# 默认上游DNS服务器（用于未匹配的域名）
upstream_dns:
  - 8.8.8.8:53
  - 1.1.1.1:53

# 域名分组配置
upstream_dns_groups:
  # 中国域名使用国内DNS
  - name: china
    domains:
      - baidu.com
      - taobao.com
      - qq.com
    upstreams:
      - 223.5.5.5:53      # 阿里DNS
      - 119.29.29.29:53   # 腾讯DNS

  # 公司内网域名
  - name: company
    domains:
      - company.local
    upstreams:
      - 192.168.1.1:53
    subdomains_only: false
```

#### 1.2 加载YAML配置

```go
package main

import (
    "time"
    "github.com/AdguardTeam/dnsproxy/proxy"
    "github.com/AdguardTeam/dnsproxy/upstream"
)

func main() {
    // 从YAML文件加载配置
    upstreamConfig, err := proxy.LoadUpstreamConfigFromYAML(
        "upstream_config.yaml",
        &upstream.Options{
            Timeout: 5 * time.Second,
        },
    )
    
    if err != nil {
        panic(err)
    }
    
    // 创建代理
    config := &proxy.Config{
        UpstreamConfig: upstreamConfig,
        CacheEnabled:   true,
    }
    
    dnsProxy, _ := proxy.New(config)
    // 使用 dnsProxy...
}
```

### 方法2: YAML + 外部域名文件

如果域名列表很长，可以将域名存储在单独的文件中：

#### 2.1 创建域名文件

`china_domains.txt`:
```text
baidu.com
taobao.com
qq.com
weibo.com
```

#### 2.2 创建YAML配置

`upstream_config_with_files.yaml`:
```yaml
upstream_dns:
  - 8.8.8.8:53

upstream_dns_groups:
  # 从文件加载域名
  - name: china
    domain_file: china_domains.txt
    upstreams:
      - 223.5.5.5:53

  # 混合使用：既有直接配置，也有文件
  - name: mixed
    domains:
      - example.com
    domain_file: additional_domains.txt
    upstreams:
      - 1.2.3.4:53
```

#### 2.3 加载配置

```go
upstreamConfig, err := proxy.LoadUpstreamConfigFromYAMLWithFiles(
    "upstream_config_with_files.yaml",
    &upstream.Options{
        Timeout: 5 * time.Second,
    },
)
```

### 方法3: 纯文本文件（简单场景）

### 方法3: 纯文本文件（简单场景）

#### 3.1 准备域名列表文件

创建一个文本文件（如 `china_domains.txt`），每行一个域名：

```text
# 中国常用域名
baidu.com
taobao.com
qq.com
weibo.com

# 支持注释和空行
bilibili.com  # 行内注释也可以
```

### 2. 使用简化API

```go
package main

import (
    "time"
    "github.com/AdguardTeam/dnsproxy/proxy"
    "github.com/AdguardTeam/dnsproxy/upstream"
)

func main() {
    // 从文件加载配置
    upstreamConfig, err := proxy.LoadUpstreamConfigFromFileSimple(
        "china_domains.txt",                    // 域名列表文件
        []string{"223.5.5.5:53", "119.29.29.29:53"}, // 这些域名使用的DNS
        []string{"8.8.8.8:53", "1.1.1.1:53"},   // 其他域名使用的DNS
        &upstream.Options{
            Timeout: 5 * time.Second,
        },
    )
    
    if err != nil {
        panic(err)
    }
    
    // 创建代理
    config := &proxy.Config{
        UpstreamConfig: upstreamConfig,
        CacheEnabled:   true,
    }
    
    dnsProxy, _ := proxy.New(config)
    // 使用 dnsProxy...
}
```

### 3. 使用完整API（多分组）

```go
groups := []proxy.DomainGroupConfig{
    {
        GroupName:      "中国域名",
        DomainFile:     "china_domains.txt",
        Upstreams:      []string{"223.5.5.5:53", "119.29.29.29:53"},
        SubdomainsOnly: false, // 包括域名本身和子域名
    },
    {
        GroupName:      "公司内网",
        DomainFile:     "company_domains.txt",
        Upstreams:      []string{"192.168.1.1:53"},
        SubdomainsOnly: false,
    },
    {
        GroupName:      "广告域名",
        DomainFile:     "ad_domains.txt",
        Upstreams:      []string{"0.0.0.0:53"}, // 屏蔽广告
        SubdomainsOnly: true,  // 只匹配子域名
    },
}

upstreamConfig, err := proxy.LoadUpstreamConfigFromFiles(
    groups,
    []string{"8.8.8.8:53", "1.1.1.1:53"}, // 默认DNS
    &upstream.Options{
        Timeout: 5 * time.Second,
    },
)
```

## API 参考

### YAML格式API

#### LoadUpstreamConfigFromYAML

从YAML文件加载配置（域名直接写在YAML中）。

```go
func LoadUpstreamConfigFromYAML(
    yamlFile string,          // YAML配置文件路径
    opts *upstream.Options,   // 上游选项
) (*UpstreamConfig, error)
```

#### LoadUpstreamConfigFromYAMLWithFiles

从YAML文件加载配置（支持引用外部域名文件）。

```go
func LoadUpstreamConfigFromYAMLWithFiles(
    yamlFile string,          // YAML配置文件路径
    opts *upstream.Options,   // 上游选项
) (*UpstreamConfig, error)
```

### 文本格式API

#### LoadUpstreamConfigFromFileSimple

简化版本，适合单个域名列表文件。

```go
func LoadUpstreamConfigFromFileSimple(
    domainFile string,        // 域名列表文件路径
    upstreamAddrs []string,   // 这些域名使用的上游服务器
    defaultUpstreams []string, // 默认上游服务器
    opts *upstream.Options,   // 上游选项
) (*UpstreamConfig, error)
```

### LoadUpstreamConfigFromFiles

完整版本，支持多个分组。

```go
func LoadUpstreamConfigFromFiles(
    groups []DomainGroupConfig,  // 域名分组配置
    defaultUpstreams []string,   // 默认上游服务器
    opts *upstream.Options,      // 上游选项
) (*UpstreamConfig, error)
```

### DomainGroupConfig

域名分组配置结构：

```go
type DomainGroupConfig struct {
    GroupName      string   // 分组名称（用于日志）
    DomainFile     string   // 域名列表文件路径
    Upstreams      []string // 该分组使用的上游服务器
    SubdomainsOnly bool     // 是否仅匹配子域名
}
```

## 域名文件格式

### 基本格式

```text
# 注释行以 # 开头
example.com
test.org

# 支持行内注释
another.com  # 这是注释

# 空行会被忽略
```

### 注意事项

- ✅ 每行一个域名
- ✅ 支持 `#` 注释（整行或行内）
- ✅ 自动忽略空行
- ✅ 自动去除首尾空格
- ❌ 不支持通配符（如 `*.example.com`）
- ❌ 域名中不能包含空格

## 使用场景

### 场景1: 中国域名加速

```go
// china_domains.txt 包含常用中国网站
upstreamConfig, _ := proxy.LoadUpstreamConfigFromFileSimple(
    "china_domains.txt",
    []string{"223.5.5.5:53", "119.29.29.29:53"}, // 阿里DNS、腾讯DNS
    []string{"8.8.8.8:53"},                      // Google DNS
    opts,
)
```

### 场景2: 公司内网域名

```go
groups := []proxy.DomainGroupConfig{
    {
        GroupName:  "内网域名",
        DomainFile: "internal_domains.txt",
        Upstreams:  []string{"192.168.1.1:53"}, // 内网DNS
    },
}
```

### 场景3: 广告屏蔽

```go
groups := []proxy.DomainGroupConfig{
    {
        GroupName:      "广告域名",
        DomainFile:     "ad_domains.txt",
        Upstreams:      []string{"0.0.0.0:53"}, // 返回0.0.0.0
        SubdomainsOnly: true,  // 只屏蔽子域名
    },
}
```

### 场景4: 多地域DNS优化

```go
groups := []proxy.DomainGroupConfig{
    {
        GroupName:  "中国域名",
        DomainFile: "china_domains.txt",
        Upstreams:  []string{"223.5.5.5:53"},
    },
    {
        GroupName:  "美国域名",
        DomainFile: "us_domains.txt",
        Upstreams:  []string{"8.8.8.8:53"},
    },
    {
        GroupName:  "欧洲域名",
        DomainFile: "eu_domains.txt",
        Upstreams:  []string{"9.9.9.9:53"},
    },
}
```

## 运行示例

```bash
# 编译示例程序
go build -o example_upstream_file_loader.exe example_upstream_file_loader.go

# 运行示例
./example_upstream_file_loader.exe
```

## 性能考虑

- 📁 文件只在启动时读取一次
- 🚀 域名查找使用高效的map结构
- 💾 大量域名（10000+）也能快速查找
- 🔄 如需更新域名列表，需要重启程序

## 错误处理

```go
upstreamConfig, err := proxy.LoadUpstreamConfigFromFileSimple(...)
if err != nil {
    // 可能的错误：
    // - 文件不存在
    // - 文件格式错误
    // - 域名格式无效
    // - 上游服务器地址无效
    log.Fatalf("加载配置失败: %v", err)
}
```

## 与现有功能集成

这个功能完全兼容dnsproxy的现有功能：

- ✅ 缓存预取（Cache Prefetch）
- ✅ DNSSEC验证
- ✅ DNS64
- ✅ ECS（EDNS Client Subnet）
- ✅ 速率限制
- ✅ 私有DNS反向解析

## 测试

运行单元测试：

```bash
go test -v ./proxy -run TestLoadUpstreamConfig
```

## 相关文档

- [dnsproxy README](README.md)
- [上游配置文档](proxy/upstreams.go)
- [缓存预取功能](PREFETCH_FEATURE.md)
