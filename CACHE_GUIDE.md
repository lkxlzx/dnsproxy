# 域名列表缓存功能指南

## 概述

DNSProxy 支持从远程 URL 下载域名列表并缓存到本地，支持自动更新和离线使用。

## 配置说明

### 基本配置

```yaml
# 缓存配置
cache:
  enabled: true                      # 启用缓存功能
  directory: ./cache                 # 默认缓存目录
  ttl: 24h                          # 缓存有效期
  default_refresh_interval: 24h     # 默认刷新间隔
  auto_update: true                 # 启用自动更新
  max_size: 100MB                   # 最大缓存大小
  cleanup_interval: 1h              # 清理检查间隔

# 域名列表配置
domains_lists:
  - name: gfwlist
    source: https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt
    group: overseas
    file: ./cache/gfwlist.yaml      # 缓存文件路径（可自定义）
    auto_update: true
    refresh_interval: 24h
    enabled: true
    format: gfwlist
    domain_count: 0                 # 域名数量（自动更新）
    last_updated: ""                # 最后更新时间（自动更新）
```

## 缓存文件路径自定义

### 1. 使用默认缓存目录

```yaml
domains_lists:
  - name: china-domains
    source: https://example.com/china.txt
    group: china
    file: ./cache/china-domains.yaml    # 相对于工作目录
    enabled: true
```

### 2. 使用绝对路径

```yaml
domains_lists:
  - name: gfwlist
    source: https://example.com/gfw.txt
    group: overseas
    file: /var/cache/dnsproxy/gfwlist.yaml    # Linux 绝对路径
    # file: C:\cache\dnsproxy\gfwlist.yaml    # Windows 绝对路径
    enabled: true
```

### 3. 使用自定义目录

```yaml
domains_lists:
  - name: adblock
    source: https://example.com/adblock.txt
    group: adblock
    file: ./data/lists/adblock.yaml         # 自定义子目录
    enabled: true
```

### 4. 按分组组织

```yaml
domains_lists:
  - name: china-domains
    source: https://example.com/china.txt
    group: china
    file: ./cache/china/domains.yaml        # 按分组分目录
    enabled: true
  
  - name: overseas-domains
    source: https://example.com/overseas.txt
    group: overseas
    file: ./cache/overseas/domains.yaml
    enabled: true
```

## 缓存文件格式

缓存文件使用 YAML 格式，包含元数据和域名列表：

```yaml
# Generated from: https://example.com/list.txt
# Generated at: 2026-05-04T09:26:19+08:00
# Total domains: 1234

domains:
    - google.com
    - youtube.com
    - facebook.com
    - "*.google.com"
    - "*.cn"
```

## 工作流程

### 首次运行

1. 读取配置文件中的 `domains_lists`
2. 检查 `file` 指定的缓存文件是否存在
3. 如果不存在，从 `source` 下载域名列表
4. 解析并转换为标准 YAML 格式
5. 保存到 `file` 指定的路径
6. 更新 `domain_count` 和 `last_updated` 字段

### 后续运行

1. 检查缓存文件是否存在
2. 如果存在且未过期，直接使用缓存
3. 如果启用 `auto_update` 且超过 `refresh_interval`，自动更新
4. 更新后重新保存到缓存文件

## 自动更新机制

### 配置自动更新

```yaml
domains_lists:
  - name: gfwlist
    source: https://example.com/gfw.txt
    group: overseas
    file: ./cache/gfwlist.yaml
    auto_update: true              # 启用自动更新
    refresh_interval: 24h          # 每 24 小时更新一次
    enabled: true
```

### 更新时间间隔

支持的时间单位：
- `h` - 小时（例如：`6h`, `24h`）
- `m` - 分钟（例如：`30m`, `60m`）
- `s` - 秒（例如：`3600s`）

常用配置：
- `6h` - 每 6 小时更新（适合频繁变化的列表）
- `12h` - 每 12 小时更新
- `24h` - 每天更新（推荐）
- `168h` - 每周更新（适合稳定的列表）

## 统计信息

### domain_count 和 last_updated

这两个字段会在域名列表加载后自动更新：

```yaml
domains_lists:
  - name: gfwlist
    source: https://example.com/gfw.txt
    group: overseas
    file: ./cache/gfwlist.yaml
    auto_update: true
    refresh_interval: 24h
    enabled: true
    format: gfwlist
    domain_count: 1234              # 自动更新：域名数量
    last_updated: "2026-05-04T09:26:19+08:00"  # 自动更新：最后更新时间
```

### 前端集成

可以通过 API 获取这些统计信息：

```go
// 获取所有列表的统计信息
stats := manager.GetAllStats()

// 返回格式：
// [
//   {
//     "name": "gfwlist",
//     "source": "https://example.com/gfw.txt",
//     "group": "overseas",
//     "enabled": true,
//     "domain_count": 1234,
//     "last_updated": "2026-05-04T09:26:19+08:00",
//     "auto_update": true,
//     "format": "gfwlist"
//   }
// ]
```

## 离线使用

### 预下载缓存文件

1. 在有网络的环境运行一次 DNSProxy
2. 缓存文件会自动下载到指定路径
3. 将缓存文件复制到离线环境
4. 在离线环境中，DNSProxy 会使用缓存文件

### 禁用自动更新

如果在离线环境使用，建议禁用自动更新：

```yaml
domains_lists:
  - name: gfwlist
    source: https://example.com/gfw.txt
    group: overseas
    file: ./cache/gfwlist.yaml
    auto_update: false              # 禁用自动更新
    enabled: true
```

## 缓存管理

### 手动清理缓存

```bash
# 删除所有缓存文件
rm -rf ./cache/*.yaml

# 删除特定缓存文件
rm -f ./cache/gfwlist.yaml

# 重启 DNSProxy 会自动重新下载
```

### 自动清理

配置自动清理过期缓存：

```yaml
cache:
  enabled: true
  directory: ./cache
  ttl: 24h                    # 缓存有效期
  cleanup_interval: 1h        # 每小时检查一次
  auto_update: true
```

### 查看缓存状态

```bash
# 查看缓存文件
ls -lh ./cache/

# 查看缓存文件内容
cat ./cache/gfwlist.yaml

# 查看域名数量
grep -c "^    -" ./cache/gfwlist.yaml
```

## 最佳实践

### 1. 目录结构建议

```
project/
├── config.yaml
├── cache/                      # 缓存目录
│   ├── china/                  # 按分组组织
│   │   └── domains.yaml
│   ├── overseas/
│   │   └── domains.yaml
│   └── adblock/
│       └── filters.yaml
└── logs/
```

### 2. 配置示例

```yaml
cache:
  enabled: true
  directory: ./cache
  ttl: 24h
  default_refresh_interval: 24h
  auto_update: true

domains_lists:
  # 中国域名 - 每 6 小时更新
  - name: china-domains
    source: https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/accelerated-domains.china.conf
    group: china
    file: ./cache/china/domains.yaml
    auto_update: true
    refresh_interval: 6h
    enabled: true
    format: dnsmasq
    domain_count: 0
    last_updated: ""
  
  # GFW 列表 - 每天更新
  - name: gfwlist
    source: https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt
    group: overseas
    file: ./cache/overseas/gfwlist.yaml
    auto_update: true
    refresh_interval: 24h
    enabled: true
    format: gfwlist
    domain_count: 0
    last_updated: ""
  
  # 广告过滤 - 每 12 小时更新
  - name: adguard-filter
    source: https://adguardteam.github.io/AdGuardSDNSFilter/Filters/filter.txt
    group: adblock
    file: ./cache/adblock/adguard.yaml
    auto_update: true
    refresh_interval: 12h
    enabled: true
    format: adblock
    domain_count: 0
    last_updated: ""
```

### 3. 权限设置

确保缓存目录有正确的权限：

```bash
# Linux/macOS
mkdir -p ./cache
chmod 755 ./cache

# 确保 DNSProxy 进程有写权限
chown dnsproxy:dnsproxy ./cache
```

### 4. 备份策略

```bash
# 定期备份缓存文件
tar -czf cache-backup-$(date +%Y%m%d).tar.gz ./cache/

# 恢复备份
tar -xzf cache-backup-20260504.tar.gz
```

## 故障排查

### 缓存文件未创建

1. 检查 `cache.enabled` 是否为 `true`
2. 检查 `file` 路径是否正确
3. 检查目录权限
4. 查看日志输出

### 自动更新不工作

1. 检查 `auto_update` 是否为 `true`
2. 检查 `refresh_interval` 格式是否正确
3. 检查网络连接
4. 查看日志中的错误信息

### 域名数量为 0

1. 检查源文件格式是否正确
2. 检查 `format` 字段是否匹配
3. 手动下载源文件验证内容
4. 查看解析日志

## 示例文件

查看 `cache/` 目录中的示例文件：
- `china-domains-example.yaml` - 中国域名列表示例
- `gfwlist-example.yaml` - GFW 列表示例
- `example-converted.yaml` - 通用格式示例
