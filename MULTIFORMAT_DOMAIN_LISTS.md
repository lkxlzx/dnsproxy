# 多格式域名列表支持

dnsproxy 支持从多种格式的域名列表文件加载域名配置，并自动转换为内部格式。

## 支持的格式

### 1. Plain Text（纯文本）
每行一个域名，支持注释。

**格式示例:**
```
example.com
test.org
# 这是注释
another.com  # 行内注释
```

**特点:**
- 每行一个域名
- `#` 开头的行为注释
- 支持行内注释
- 自动忽略空行

### 2. Dnsmasq
dnsmasq 配置文件格式。

**格式示例:**
```
server=/baidu.com/114.114.114.114
server=/taobao.com/223.5.5.5
server=/qq.com/119.29.29.29
```

**特点:**
- 格式: `server=/domain/dns`
- 自动提取域名部分
- 忽略DNS服务器地址

**常见来源:**
- [dnsmasq-china-list](https://github.com/felixonmars/dnsmasq-china-list)

### 3. GFWList
GFWList 格式（Base64编码的规则列表）。

**格式示例:**
```
[AutoProxy 0.2.9]
||google.com
||youtube.com
|https://facebook.com
```

**特点:**
- Base64 编码
- 支持多种规则前缀: `||`, `|`, `@@||`, `@@|`
- 自动提取域名
- 过滤IP地址

**常见来源:**
- [gfwlist](https://github.com/gfwlist/gfwlist)

### 4. Clash
Clash 规则格式（YAML）。

**格式示例:**
```yaml
payload:
  - DOMAIN,google.com
  - DOMAIN-SUFFIX,youtube.com
  - DOMAIN-SUFFIX,facebook.com
```

**特点:**
- YAML 格式
- 支持 `DOMAIN` 和 `DOMAIN-SUFFIX` 规则
- 自动提取域名部分

**常见来源:**
- [ios_rule_script](https://github.com/blackmatrix7/ios_rule_script)

## 使用方法

### 方法1: 自动检测格式

```go
import "github.com/AdguardTeam/dnsproxy/proxy"

// 自动检测文件格式
converter := proxy.NewDomainListConverter("domains.txt", proxy.FormatAuto)
domains, err := converter.ConvertToDomains()
if err != nil {
    log.Fatal(err)
}
```

### 方法2: 指定格式

```go
// 明确指定格式
converter := proxy.NewDomainListConverter("dnsmasq.conf", proxy.FormatDnsmasq)
domains, err := converter.ConvertToDomains()
```

### 方法3: 转换为上游配置

```go
converter := proxy.NewDomainListConverter("domains.txt", proxy.FormatPlain)

// 转换为上游配置行
lines, err := converter.ConvertToUpstreamLines(
    []string{"223.5.5.5:53", "119.29.29.29:53"},
    false, // subdomainsOnly
)
// 结果: [/domain1/domain2/]223.5.5.5:53
```

### 方法4: 多格式混合配置

```go
import (
    "github.com/AdguardTeam/dnsproxy/proxy"
    "github.com/AdguardTeam/dnsproxy/upstream"
)

groups := []proxy.DomainGroupConfig{
    {
        GroupName:        "china",
        DomainFile:       "china-domains.txt",
        DomainFileFormat: proxy.FormatDnsmasq,
        Upstreams:        []string{"223.5.5.5:53"},
        SubdomainsOnly:   false,
    },
    {
        GroupName:        "gfw",
        DomainFile:       "gfwlist.txt",
        DomainFileFormat: proxy.FormatGFWList,
        Upstreams:        []string{"8.8.8.8:53"},
        SubdomainsOnly:   false,
    },
    {
        GroupName:        "clash",
        DomainFile:       "clash-rules.yaml",
        DomainFileFormat: proxy.FormatClash,
        Upstreams:        []string{"1.1.1.1:53"},
        SubdomainsOnly:   false,
    },
}

config, err := proxy.LoadUpstreamConfigFromFiles(
    groups,
    []string{"8.8.8.8:53"}, // 默认上游
    &upstream.Options{},
)
```

## 格式类型常量

```go
const (
    FormatAuto     // 自动检测
    FormatPlain    // 纯文本
    FormatDnsmasq  // Dnsmasq
    FormatGFWList  // GFWList
    FormatClash    // Clash
)
```

## API 参考

### DomainListConverter

```go
type DomainListConverter struct {
    SourceFormat DomainListFormat
    SourceFile   string
}

// 创建转换器
func NewDomainListConverter(sourceFile string, format DomainListFormat) *DomainListConverter

// 转换为域名列表
func (c *DomainListConverter) ConvertToDomains() ([]string, error)

// 转换为上游配置行
func (c *DomainListConverter) ConvertToUpstreamLines(upstreams []string, subdomainsOnly bool) ([]string, error)
```

### DomainGroupConfig

```go
type DomainGroupConfig struct {
    GroupName        string              // 分组名称
    DomainFile       string              // 域名文件路径
    DomainFileFormat DomainListFormat    // 文件格式
    Upstreams        []string            // 上游服务器列表
    SubdomainsOnly   bool                // 仅匹配子域名
}
```

### 加载函数

```go
// 从多个文件加载配置
func LoadUpstreamConfigFromFiles(
    groups []DomainGroupConfig,
    defaultUpstreams []string,
    opts *upstream.Options,
) (*UpstreamConfig, error)

// 简化版本：单文件加载
func LoadUpstreamConfigFromFileSimple(
    domainFile string,
    upstreamAddrs []string,
    defaultUpstreams []string,
    opts *upstream.Options,
) (*UpstreamConfig, error)
```

## 实际应用示例

### 示例1: 中国域名加速

```go
// 使用 dnsmasq-china-list
converter := proxy.NewDomainListConverter(
    "accelerated-domains.china.conf",
    proxy.FormatDnsmasq,
)

lines, _ := converter.ConvertToUpstreamLines(
    []string{"223.5.5.5:53", "119.29.29.29:53"},
    false,
)
```

### 示例2: GFW列表代理

```go
// 使用 gfwlist
converter := proxy.NewDomainListConverter(
    "gfwlist.txt",
    proxy.FormatGFWList,
)

lines, _ := converter.ConvertToUpstreamLines(
    []string{"8.8.8.8:53"},
    false,
)
```

### 示例3: Clash规则转换

```go
// 使用 Clash 规则
converter := proxy.NewDomainListConverter(
    "ChinaMaxNoIP_Classical.yaml",
    proxy.FormatClash,
)

domains, _ := converter.ConvertToDomains()
```

## 自动检测逻辑

转换器会根据文件内容自动检测格式:

1. **GFWList**: 检测 `[AutoProxy` 标记
2. **Clash**: 检测 `payload:` 或 `- DOMAIN,` 标记
3. **Dnsmasq**: 检测 `server=/` 前缀
4. **Plain**: 默认格式

## 注意事项

1. **编码**: 所有文件应使用 UTF-8 编码
2. **大文件**: 大文件会被完整加载到内存，注意内存使用
3. **格式混合**: 不支持在同一文件中混合多种格式
4. **IP过滤**: GFWList 格式会自动过滤IP地址
5. **子域名**: `SubdomainsOnly=true` 时，会在域名前添加 `*.` 前缀

## 性能考虑

- 自动检测只读取前10行非空非注释行
- 域名去重在上游配置解析时自动完成
- 建议对大文件使用明确的格式指定，避免自动检测开销

## 错误处理

```go
domains, err := converter.ConvertToDomains()
if err != nil {
    // 可能的错误:
    // - 文件不存在
    // - 格式检测失败
    // - Base64解码失败（GFWList）
    // - YAML解析失败（Clash）
    log.Printf("转换失败: %v", err)
}
```

## 相关文档

- [上游文件加载器](UPSTREAM_FILE_LOADER.md)
- [YAML配置格式](YAML_CONFIG_FORMAT.md)
- [快速开始指南](QUICKSTART_V1.1.md)
