# DNSProxy 配置指南 v2.2.1

本指南详细说明了 DNSProxy 的配置选项和使用方法。

## 📋 目录

- [快速开始](#快速开始)
- [配置文件示例](#配置文件示例)
- [核心配置](#核心配置)
- [上游分组](#上游分组)
- [域名路由](#域名路由)
- [AdGuard Home 集成](#adguard-home-集成)
- [高级功能](#高级功能)

## 🚀 快速开始

### 1. 选择配置模板

根据你的需求选择合适的配置模板：

- **`config-simple-example.yaml`** - 最小化配置，适合快速开始
- **`config-adguard-example.yaml`** - AdGuard Home 兼容格式
- **`config-complete-example.yaml`** - 完整配置，包含所有功能

### 2. 复制并修改配置

```bash
# 复制配置模板
cp config-simple-example.yaml config.yaml

# 编辑配置
vim config.yaml
```

### 3. 启动服务

```bash
./dnsproxy -c config.yaml
```

**首次运行说明**：
- `cache` 目录会自动创建（如果不存在）
- 域名列表会自动下载并缓存到 `cache` 目录
- 如果网络不可用，会使用已缓存的文件（如果存在）

## 📝 配置文件示例

### 最小配置

```yaml
dns:
  bind_hosts: [0.0.0.0]
  port: 53
  upstream_dns: [114.114.114.114]

upstream_groups:
  - id: default-id
    name: 默认
    upstreams: [223.5.5.5]
    mode: load_balance
    timeout: 5s

default_group: default-id
```

## ⚙️ 核心配置

### DNS 服务器配置

```yaml
dns:
  # 监听地址
  bind_hosts:
    - 0.0.0.0      # IPv4
    - "::"         # IPv6
  
  # 监听端口
  port: 53
  
  # 速率限制（每秒请求数）
  ratelimit: 20
  
  # 是否拒绝 ANY 查询
  refuse_any: true
  
  # 全局上游 DNS
  upstream_dns:
    - 114.114.114.114
```

### 速率限制配置

```yaml
dns:
  ratelimit: 20                      # 每秒最大请求数
  ratelimit_subnet_len_ipv4: 24      # IPv4 子网掩码
  ratelimit_subnet_len_ipv6: 56      # IPv6 子网掩码
  ratelimit_whitelist:               # 白名单
    - 192.168.1.0/24
    - 10.0.0.0/8
```

## 🔀 上游分组

### 基本配置

```yaml
upstream_groups:
  - id: dd1c17fb-9196-409e-b350-82c829c39d7a  # UUID 格式（推荐）
    name: 国内                                 # 分组名称
    enabled: true                              # 是否启用
    upstreams:                                 # 上游服务器列表
      - 223.5.5.5
      - 119.29.29.29
    mode: load_balance                         # 负载均衡模式
    timeout: 5s                                # 超时时间
    priority: 0                                # 优先级（保留字段）
```

### ID 字段说明

- **格式**: 推荐使用 UUID 格式（如 `dd1c17fb-9196-409e-b350-82c829c39d7a`）
- **用途**: 用于与外部系统（如 AdGuard Home）集成
- **可选**: ID 字段是可选的，不影响向后兼容性
- **唯一性**: 每个分组的 ID 应该是唯一的

### 负载均衡模式

#### 1. load_balance（负载均衡）

```yaml
mode: load_balance
```

- 按顺序轮询所有上游服务器
- 适合多个可靠的上游服务器

#### 2. parallel（并行查询）

```yaml
mode: parallel
```

- 同时查询所有上游服务器
- 使用最快的响应
- 适合需要低延迟的场景

#### 3. fastest_addr（最快地址）

```yaml
mode: fastest_addr
```

- 查询所有上游服务器
- 返回响应最快的 IP 地址
- 适合优化访问速度

### 上游服务器格式

支持多种协议：

```yaml
upstreams:
  # UDP/TCP（标准 DNS）
  - 8.8.8.8
  - 8.8.8.8:53
  
  # DNS over HTTPS (DoH)
  - https://dns.google/dns-query
  - https://dns.cloudflare.com/dns-query
  
  # DNS over TLS (DoT)
  - tls://dns.adguard.com
  - tls://dns.quad9.net
  
  # DNS over QUIC (DoQ)
  - quic://dns.adguard.com
```

## 🌐 域名路由

### 方式 1: 使用 domain_groups

#### 使用 ID 作为键（推荐）

```yaml
domain_groups:
  dd1c17fb-9196-409e-b350-82c829c39d7a:
    domains:
      - baidu.com
      - "*.cn"
```

#### 使用名称作为键

```yaml
domain_groups:
  国内:
    - baidu.com
    - "*.cn"
```

### 方式 2: 使用 domains_lists

从远程或本地文件加载域名列表：

```yaml
domains_lists:
  - name: china-domains
    source: https://example.com/china-domains.txt
    group: dd1c17fb-9196-409e-b350-82c829c39d7a  # 使用 ID
    file: ./cache/china-domains.yaml              # 本地缓存路径（自动创建）
    auto_update: true
    refresh_interval: 6h
    enabled: true
    format: dnsmasq
```

**工作流程**：
1. 首次运行时，从 `source` 下载域名列表
2. 转换为 YAML 格式并保存到 `file` 指定的路径
3. 后续启动时，如果缓存文件存在且未过期，直接使用缓存
4. 如果启用 `auto_update`，会在后台定期更新

#### 支持的格式

- **dnsmasq** - Dnsmasq 格式
- **gfwlist** - GFWList 格式
- **adblock** - AdBlock 格式
- **hosts** - Hosts 格式
- **yaml** - YAML 格式（默认）

### 域名匹配规则

```yaml
domains:
  - example.com          # 精确匹配
  - "*.example.com"      # 通配符匹配（所有子域名）
  - "*.cn"               # 所有 .cn 域名
```

### 优先级规则

1. **精确匹配** - 最高优先级
2. **通配符匹配** - 中等优先级
3. **默认分组** - 最低优先级

示例：

```yaml
domain_groups:
  group1:
    - example.com        # 精确匹配，优先级最高
  
  group2:
    - "*.example.com"    # 通配符匹配，优先级中等

default_group: group3    # 默认分组，优先级最低
```

查询 `www.example.com` 的匹配顺序：
1. 检查是否有 `www.example.com` 的精确匹配 ❌
2. 检查是否有 `*.example.com` 的通配符匹配 ✅ → 使用 group2
3. 如果都没有匹配，使用默认分组 group3

## 🔗 AdGuard Home 集成

### 从 AdGuard Home 导出配置

1. 在 AdGuard Home 中配置上游分组
2. 导出配置（YAML 格式）
3. 直接使用导出的配置

### AdGuard Home 格式示例

```yaml
upstream_groups:
  - id: baf42657-598c-4f35-9185-36b9706785d0
    name: overseas
    upstreams:
      - https://dns.google/dns-query
    mode: load_balance
    timeout: 10s

domain_groups:
  baf42657-598c-4f35-9185-36b9706785d0:
    domains:
      - google.com
      - "*.google.com"

domains_lists:
  - name: gfwlist
    group: baf42657-598c-4f35-9185-36b9706785d0
    source: https://example.com/gfwlist.txt

default_group: baf42657-598c-4f35-9185-36b9706785d0
```

### 兼容性说明

- ✅ 支持使用 ID 引用分组
- ✅ 支持 `domains:` 嵌套字段
- ✅ 支持 `domain_groups` 使用 ID 作为键
- ✅ 支持 `domains_lists` 中的 `group` 字段使用 ID
- ✅ 支持 `default_group` 使用 ID

## 🔧 高级功能

### 缓存配置

```yaml
cache:
  enabled: true                      # 启用缓存
  directory: ./cache                 # 缓存目录（自动创建）
  ttl: 24h                          # 缓存有效期
  default_refresh_interval: 24h     # 默认刷新间隔
  auto_update: true                 # 自动更新
  max_size: 100MB                   # 最大缓存大小
  cleanup_interval: 1h              # 清理间隔
```

**注意**：
- `cache` 目录会在首次运行时自动创建
- 域名列表文件会自动下载并缓存到指定的 `file` 路径
- 如果源文件无法访问，会使用缓存的文件（如果存在）

### 日志配置

```yaml
log:
  level: info                        # 日志级别
  file: ./logs/dnsproxy.log         # 日志文件
  max_size: 100                     # 最大大小（MB）
  max_backups: 3                    # 保留文件数
  max_age: 28                       # 保留天数
  compress: true                    # 压缩旧日志
```

### EDNS 客户端子网

```yaml
advanced:
  edns_client_subnet:
    enabled: true
    ipv4_mask: 24
    ipv6_mask: 56
```

### DNS64

```yaml
advanced:
  dns64:
    enabled: true
    prefix: 64:ff9b::/96
```

### 响应过滤

```yaml
advanced:
  filtering:
    blocked_response_ttl: 3600       # 被拦截域名的 TTL
    filter_aaaa: false               # 过滤 AAAA 记录
    filter_a: false                  # 过滤 A 记录
```

## 📚 常见配置场景

### 场景 1: 国内外分流

```yaml
upstream_groups:
  - id: china-id
    name: 国内
    upstreams: [223.5.5.5, 119.29.29.29]
    mode: load_balance
  
  - id: overseas-id
    name: overseas
    upstreams: [8.8.8.8, 1.1.1.1]
    mode: parallel

domain_groups:
  china-id:
    domains: ["*.cn", "baidu.com"]
  
  overseas-id:
    domains: ["google.com", "*.google.com"]

default_group: china-id
```

### 场景 2: 广告拦截

```yaml
upstream_groups:
  - id: adblock-id
    name: adblock
    upstreams: [127.0.0.1:5353]
    mode: load_balance
  
  - id: normal-id
    name: normal
    upstreams: [223.5.5.5]
    mode: load_balance

domains_lists:
  - name: ad-filter
    source: https://example.com/ad-domains.txt
    group: adblock-id
    file: ./cache/ad-filter.yaml

default_group: normal-id
```

### 场景 3: 安全 DNS

```yaml
upstream_groups:
  - id: secure-id
    name: secure
    upstreams:
      - tls://dns.adguard.com
      - https://dns.quad9.net/dns-query
    mode: parallel
    timeout: 10s

default_group: secure-id
```

## 🔍 故障排查

### 缓存文件问题

**问题**：找不到 cache 文件夹或缓存文件

**解决方案**：
```bash
# cache 目录会在首次运行时自动创建
# 如果需要手动创建：
mkdir -p cache

# 检查缓存文件
ls -la cache/

# 清理缓存（如果需要重新下载）
rm -rf cache/*.yaml
```

### 检查配置文件

```bash
# 验证 YAML 语法
./dnsproxy -c config.yaml --check-config
```

### 查看日志

```bash
# 启用调试日志
./dnsproxy -c config.yaml --log-level debug
```

### 测试域名解析

```bash
# 使用 dig 测试
dig @127.0.0.1 -p 53 example.com

# 使用 nslookup 测试
nslookup example.com 127.0.0.1
```

## 📖 参考资源

- [完整配置示例](config-complete-example.yaml)
- [简化配置示例](config-simple-example.yaml)
- [AdGuard Home 格式示例](config-adguard-example.yaml)
- [项目状态文档](PROJECT_STATUS.md)
- [快速开始指南](QUICK_START.md)

## 🆘 获取帮助

如果遇到问题：

1. 查看日志文件
2. 检查配置文件语法
3. 参考配置示例
4. 提交 Issue 到 GitHub
