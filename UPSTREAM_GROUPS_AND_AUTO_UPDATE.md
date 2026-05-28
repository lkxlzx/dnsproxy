# 上游分组管理和域名列表自动更新功能

版本: v2.2
日期: 2026-05-28

## 概述

本文档介绍DNSProxy v2.2的重要功能：

1. **上游分组管理** - 通过分组ID引用上游DNS服务器，简化配置管理
2. **主备模式** - 每个上游分组支持主要和备用DNS服务器
3. **多层故障转移** - 完整的三层故障转移机制，确保高可用性
4. **域名列表自动更新** - 从远程URL自动下载和更新域名列表

这些功能大大提升了配置的可维护性、可靠性和自动化程度。

## 功能1: 上游分组管理

### 问题背景

在v2.0版本中，每个域名分组都需要单独配置上游DNS服务器列表：

```yaml
domain-groups:
  - name: china1
    upstream:
      - 223.5.5.5
      - 119.29.29.29
  
  - name: china2
    upstream:
      - 223.5.5.5      # 重复配置
      - 119.29.29.29   # 重复配置
```

这种方式存在以下问题：
- 配置重复，难以维护
- 修改上游服务器需要更新多处
- 配置文件冗长
- 缺乏备用服务器机制

### 解决方案

引入**上游分组**概念，支持主备模式：

```yaml
# 定义上游分组（支持主备模式）
upstream-groups:
  - id: china-dns
    description: 中国DNS服务器
    upstreams:
      - 223.5.5.5      # 主要DNS
      - 119.29.29.29   # 主要DNS
    fallback-upstreams:
      - 114.114.114.114  # 备用DNS（仅在主要全部失败时使用）

# 域名分组引用上游分组ID
domain-groups:
  - name: china1
    upstream-group-id: china-dns  # 引用分组ID
  
  - name: china2
    upstream-group-id: china-dns  # 引用同一个分组
```

### 核心组件

#### 1. UpstreamGroup 结构

```go
type UpstreamGroup struct {
    ID          string   // 唯一标识符
    Upstreams   []string // 上游DNS服务器列表
    Description string   // 描述信息（可选）
}
```

#### 2. UpstreamGroupManager 管理器

```go
type UpstreamGroupManager struct {
    // 管理所有上游分组
}

// 主要方法
func (m *UpstreamGroupManager) AddGroup(group *UpstreamGroup) error
func (m *UpstreamGroupManager) GetGroup(id string) (*UpstreamGroup, bool)
func (m *UpstreamGroupManager) GetUpstreams(id string) ([]string, error)
func (m *UpstreamGroupManager) ResolveUpstreams(groupID string, directUpstreams []string) ([]string, error)
```

### 配置示例

```yaml
# 上游分组定义
upstream-groups:
  - id: china-dns
    description: 中国大陆DNS服务器
    upstreams:
      - 223.5.5.5      # 阿里DNS
      - 223.6.6.6
      - 119.29.29.29   # 腾讯DNS
      - 119.28.28.28

  - id: global-dns
    description: 国际DNS服务器
    upstreams:
      - 8.8.8.8        # Google DNS
      - 8.8.4.4
      - 1.1.1.1        # Cloudflare DNS
      - 1.0.0.1

  - id: adblock-dns
    description: 广告拦截DNS
    upstreams:
      - 94.140.14.14   # AdGuard DNS
      - 94.140.15.15

  - id: privacy-dns
    description: 隐私保护DNS
    upstreams:
      - 9.9.9.9        # Quad9
      - 149.112.112.112

  - id: internal-dns
    description: 内网DNS
    upstreams:
      - 192.168.1.1
      - 10.0.0.1

# 域名分组使用上游分组ID
domain-groups:
  - name: china
    domain-file: china_domains.txt
    upstream-group-id: china-dns  # 引用上游分组
  
  - name: ads
    domain-file: ad_domains.txt
    upstream-group-id: adblock-dns
  
  - name: development
    domain-file: custom_domains.txt
    upstream-group-id: internal-dns
```

### 优势

1. **集中管理** - 所有上游服务器在一处定义
2. **配置复用** - 多个域名分组可以共享同一个上游分组
3. **易于维护** - 修改上游分组会自动影响所有引用它的域名分组
4. **配置简洁** - 减少重复配置，提高可读性
5. **向后兼容** - 仍然支持直接在域名分组中指定upstreams

### 使用示例

参见 `example_upstream_groups.go`

```bash
cd dnsproxy
go run example_upstream_groups.go
```

---

## 功能2: 域名列表自动更新

### 问题背景

在v2.0版本中，域名列表只能从本地文件加载：

```yaml
domain-groups:
  - name: china
    domain-file: china_domains.txt  # 只支持本地文件
```

这种方式存在以下问题：
- 需要手动下载和更新域名列表
- 无法自动获取最新的域名列表
- 维护成本高

### 解决方案

支持从远程URL自动下载和更新域名列表：

```yaml
domain-groups:
  - name: china
    domain-file: china_domains.txt           # 本地缓存文件
    domain-file-url: https://example.com/china-list.txt  # 远程URL
    update-interval: 24h                     # 自动更新间隔
```

### 核心组件

#### 1. DomainListSource 结构

```go
type DomainListSource struct {
    FilePath       string        // 本地文件路径
    URL            string        // 远程URL
    Format         string        // 格式（plain/dnsmasq/gfwlist/clash）
    UpdateInterval time.Duration // 更新间隔
    LastUpdate     time.Time     // 最后更新时间
    LastChecksum   string        // SHA256校验和
    DomainCount    int           // 域名数量
    LastError      error         // 最后错误
}
```

#### 2. DomainListUpdater 更新器

```go
type DomainListUpdater struct {
    // 管理所有域名列表的自动更新
}

// 主要方法
func (u *DomainListUpdater) AddSource(groupName string, source *DomainListSource) error
func (u *DomainListUpdater) UpdateNow(groupName string) error
func (u *DomainListUpdater) UpdateAll() map[string]error
func (u *DomainListUpdater) GetStatus() map[string]DomainListSourceStatus
func (u *DomainListUpdater) Stop()
```


### 配置示例

```yaml
domain-groups:
  # 示例1: 中国域名列表（每24小时自动更新）
  - name: china
    enabled: true
    domain-file: china_domains.txt
    domain-file-url: https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/accelerated-domains.china.conf
    domain-file-format: dnsmasq
    update-interval: 24h
    upstream-group-id: china-dns

  # 示例2: 广告域名列表（每12小时自动更新）
  - name: ads
    enabled: true
    domain-file: ad_domains.txt
    domain-file-url: https://raw.githubusercontent.com/privacy-protection-tools/anti-AD/master/anti-ad-domains.txt
    domain-file-format: plain
    update-interval: 12h
    upstream-group-id: adblock-dns

  # 示例3: GFWList（每7天自动更新）
  - name: gfwlist
    enabled: true
    domain-file: gfw_domains.txt
    domain-file-url: https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt
    domain-file-format: gfwlist
    update-interval: 168h  # 7天
    upstream-group-id: global-dns

  # 示例4: 本地文件（不自动更新）
  - name: custom
    enabled: true
    domain-file: custom_domains.txt
    # 不设置URL和update-interval，仅使用本地文件
    upstream-group-id: internal-dns
```

### 更新间隔格式

支持Go的time.Duration格式：

- `30m` - 30分钟
- `1h` - 1小时
- `24h` - 24小时
- `168h` - 7天
- `720h` - 30天

### 工作流程

1. **启动时**
   - 检查本地文件是否存在
   - 如果不存在且配置了URL，立即下载
   - 加载域名列表到内存

2. **自动更新**
   - 根据`update-interval`定时触发
   - 从URL下载最新内容
   - 计算SHA256校验和
   - 如果内容变化，保存到本地文件
   - 更新域名数量和时间戳

3. **手动更新**
   - 通过API触发立即更新
   - 支持单个分组或全部分组

4. **三层故障转移**
   - **第1层（主要上游）**: 优先使用的DNS服务器
   - **第2层（备用上游）**: 主要全部失败时使用
   - **第3层（默认上游）**: 备用也失败时使用，通过`default-upstream-group-id`引用分组

### 特性

#### 1. 智能更新检测

使用SHA256校验和检测内容变化：

```go
// 只有内容真正变化时才更新
if checksum == source.LastChecksum {
    // 内容未变化，跳过更新
    return nil
}
```

#### 2. 原子文件写入

使用临时文件+重命名确保原子性：

```go
// 先写入临时文件
tmpFile := filePath + ".tmp"
os.WriteFile(tmpFile, content, 0644)

// 原子重命名
os.Rename(tmpFile, filePath)
```

#### 3. 多格式支持

自动检测或手动指定格式：

- **plain** - 纯文本，每行一个域名
- **dnsmasq** - Dnsmasq格式 (`server=/domain/dns`)
- **gfwlist** - GFWList格式（Base64编码）
- **clash** - Clash YAML格式

#### 4. 错误处理

- HTTP错误自动记录
- 解析错误不影响现有配置
- 保留最后一次成功的内容

#### 5. 状态监控

提供详细的状态信息：

```go
type DomainListSourceStatus struct {
    FilePath       string        // 本地文件路径
    URL            string        // 远程URL
    Format         string        // 格式
    UpdateInterval time.Duration // 更新间隔
    LastUpdate     time.Time     // 最后更新时间
    LastChecksum   string        // 校验和
    DomainCount    int           // 域名数量
    LastError      string        // 最后错误
}
```

### DomainGroupConfig 更新

```go
type DomainGroupConfig struct {
    GroupName       string // 分组名称
    DomainFile      string // 本地文件路径
    DomainFileURL   string // 远程URL（新增）
    DomainFileFormat string // 格式
    UpdateInterval  string // 更新间隔（新增）
    UpstreamGroupID string // 上游分组ID（新增）
    Upstreams       []string // 直接上游（向后兼容）
    SubdomainsOnly  bool   // 仅匹配子域名
    Enabled         bool   // 是否启用
    
    // 内部字段
    domains      []string // 缓存的域名列表
    domainCount  int      // 域名数量（新增）
    lastUpdate   string   // 最后更新时间（新增）
}
```

### API接口

#### 1. 获取分组状态

```go
status := domainGroupManager.GetGroups()
// 返回所有分组的状态，包括域名数量、更新时间等
```

#### 2. 手动触发更新

```go
err := domainGroupManager.UpdateGroup("china")
// 立即从URL下载并更新指定分组
```

#### 3. 获取更新状态

```go
updateStatus := domainGroupManager.GetUpdateStatus()
// 返回所有域名列表的更新状态
```

### 使用示例

参见 `example_auto_update.go`

```bash
cd dnsproxy
go run example_auto_update.go
```

---

## 完整配置示例

结合两个功能的完整配置：

```yaml
# ============================================================================
# DNSProxy 完整配置示例 (v2.1)
# ============================================================================

# 基础配置
listen-addrs:
  - 0.0.0.0
listen-ports:
  - 53

# ----------------------------------------------------------------------------
# 上游分组配置（新功能）
# ----------------------------------------------------------------------------
upstream-groups:
  - id: china-dns
    description: 中国大陆DNS服务器
    upstreams:
      - 223.5.5.5
      - 223.6.6.6
      - 119.29.29.29
      - 119.28.28.28

  - id: global-dns
    description: 国际DNS服务器
    upstreams:
      - 8.8.8.8
      - 8.8.4.4
      - 1.1.1.1
      - 1.0.0.1

  - id: adblock-dns
    description: 广告拦截DNS
    upstreams:
      - 94.140.14.14
      - 94.140.15.15

# ----------------------------------------------------------------------------
# 域名分组配置（增强功能）
# ----------------------------------------------------------------------------
domain-groups:
  # 中国域名 - 使用上游分组 + 自动更新
  - name: china
    enabled: true
    domain-file: china_domains.txt
    domain-file-url: https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/accelerated-domains.china.conf
    domain-file-format: dnsmasq
    update-interval: 24h
    upstream-group-id: china-dns  # 引用上游分组

  # 广告域名 - 使用上游分组 + 自动更新
  - name: ads
    enabled: true
    domain-file: ad_domains.txt
    domain-file-url: https://raw.githubusercontent.com/privacy-protection-tools/anti-AD/master/anti-ad-domains.txt
    update-interval: 12h
    upstream-group-id: adblock-dns

  # 自定义域名 - 仅本地文件
  - name: custom
    enabled: true
    domain-file: custom_domains.txt
    upstream-group-id: global-dns

# 默认上游
upstream:
  - 8.8.8.8
  - 1.1.1.1

# 缓存配置
cache: true
cache-size: 10000

# 智能预取配置
cache-prefetch-enabled: true
cache-prefetch-threshold-seconds: 5
cache-prefetch-threshold-percent: 80
```

---

## 迁移指南

### 从v2.0迁移到v2.1

#### 步骤1: 定义上游分组

将重复的上游配置提取为分组：

**v2.0配置:**
```yaml
domain-groups:
  - name: china1
    upstream:
      - 223.5.5.5
      - 119.29.29.29
  - name: china2
    upstream:
      - 223.5.5.5
      - 119.29.29.29
```

**v2.1配置:**
```yaml
upstream-groups:
  - id: china-dns
    upstreams:
      - 223.5.5.5
      - 119.29.29.29

domain-groups:
  - name: china1
    upstream-group-id: china-dns
  - name: china2
    upstream-group-id: china-dns
```

#### 步骤2: 添加自动更新（可选）

为需要自动更新的分组添加URL：

```yaml
domain-groups:
  - name: china
    domain-file: china_domains.txt
    domain-file-url: https://example.com/china-list.txt
    update-interval: 24h
    upstream-group-id: china-dns
```

#### 步骤3: 测试配置

```bash
# 检查配置
./dnsproxy -c config.yaml --check-config

# 启动服务
./dnsproxy -c config.yaml
```

### 向后兼容性

v2.1完全向后兼容v2.0：

- 仍然支持直接在域名分组中指定`upstreams`
- 仍然支持仅使用本地文件（不配置URL）
- 配置格式保持一致

---

## 性能影响

### 内存占用

- 上游分组管理器：< 1MB
- 域名列表更新器：< 2MB
- 总体增加：< 3MB

### CPU占用

- 自动更新检查：极低（仅在更新间隔触发）
- 下载和解析：短暂峰值（通常< 1秒）
- 日常运行：无影响

### 网络流量

- 取决于域名列表大小和更新频率
- 典型场景：
  - 中国域名列表：~500KB，每24小时
  - 广告域名列表：~1MB，每12小时
  - GFWList：~200KB，每7天

---

## 故障排查

### 问题1: 上游分组未找到

**错误信息:**
```
upstream group not found: china-dns
```

**解决方案:**
- 检查`upstream-groups`中是否定义了该ID
- 确认ID拼写正确（区分大小写）

### 问题2: 域名列表下载失败

**错误信息:**
```
failed to update domain list: http status: 404
```

**解决方案:**
- 检查URL是否正确
- 确认网络连接正常
- 查看日志获取详细错误信息

### 问题3: 格式解析失败

**错误信息:**
```
parse failed: unsupported format
```

**解决方案:**
- 手动指定`domain-file-format`
- 检查文件内容格式是否正确
- 参考格式说明文档

---

## 最佳实践

### 1. 上游分组命名

使用描述性的ID：
- ✅ `china-dns`, `global-dns`, `adblock-dns`
- ❌ `group1`, `g1`, `dns`

### 2. 更新间隔设置

根据列表更新频率设置：
- 频繁变化的列表（广告）：6-12小时
- 稳定的列表（中国域名）：24小时
- 很少变化的列表（GFWList）：7天

### 3. 本地文件路径

使用绝对路径或相对于配置文件的路径：
- ✅ `/etc/dnsproxy/china_domains.txt`
- ✅ `./lists/china_domains.txt`
- ❌ `china_domains.txt`（可能找不到）

### 4. 错误监控

定期检查更新状态：
```bash
# 通过API获取状态
curl http://localhost:8080/api/domain-groups/status
```

### 5. 备份配置

在修改配置前备份：
```bash
cp config.yaml config.yaml.backup
```

---

## 测试

### 单元测试

```bash
cd dnsproxy/proxy
go test -v -run TestUpstreamGroup
go test -v -run TestDomainListUpdater
```

### 集成测试

```bash
cd dnsproxy
go run example_upstream_groups.go
go run example_auto_update.go
```

### 功能测试

1. 测试上游分组引用
2. 测试域名列表自动更新
3. 测试手动触发更新
4. 测试错误处理

---

## 相关文档

- [PREFETCH_FEATURE.md](PREFETCH_FEATURE.md) - 智能预取功能
- [DYNAMIC_DOMAIN_GROUPS.md](DYNAMIC_DOMAIN_GROUPS.md) - 动态域名分组
- [MULTIFORMAT_DOMAIN_LISTS.md](MULTIFORMAT_DOMAIN_LISTS.md) - 多格式支持
- [config.yaml.example](config.yaml.example) - 完整配置示例

---

## 总结

v2.1版本新增的上游分组管理和域名列表自动更新功能，显著提升了DNSProxy的可维护性和自动化程度：

### 主要优势

1. **配置简化** - 通过上游分组减少重复配置
2. **自动化** - 域名列表自动更新，无需手动维护
3. **灵活性** - 支持多种格式和更新策略
4. **可靠性** - 原子更新、错误处理、状态监控
5. **向后兼容** - 完全兼容v2.0配置

### 适用场景

- 需要管理多个域名分组的场景
- 需要自动更新域名列表的场景
- 需要集中管理上游DNS服务器的场景
- 需要监控域名列表状态的场景

### 下一步

- 查看完整配置示例：`config.yaml.example`
- 运行示例程序：`example_upstream_groups.go`, `example_auto_update.go`
- 阅读相关文档了解更多功能


---

## 功能3: 三层故障转移机制

### 问题背景

在实际使用中，单一的主备模式可能不够：
- 主要DNS失败后，备用DNS也可能失败
- 需要一个最终的兜底方案确保所有域名都能解析
- 未匹配任何分组的域名也需要可靠的解析路径

### 解决方案

实现**三层故障转移机制**：

```yaml
# 未匹配域名的默认分组（仅对未匹配域名生效）
default-upstream-group-id: global-dns

# 全局兜底（对所有域名生效，包括匹配和未匹配的）
upstream:
  - 8.8.8.8
  - 1.1.1.1

domain-groups:
  - name: china
    upstream-group-id: china-dns  # 包含主要和备用DNS
    # 故障转移：china-dns主要 → china-dns备用 → 全局upstream
```

### 核心概念

- **upstream (全局兜底)**: 对所有域名生效的最终兜底DNS，无论是否匹配域名分组
- **default-upstream-group-id**: 仅对未匹配任何域名分组的域名生效
- **upstream-group-id**: 域名分组使用的上游分组（包含主要和备用DNS）

### 三层结构

#### 对于匹配到域名分组的查询

```
第1层：分组主要上游 (upstreams)
  └─ 该分组优先使用的DNS服务器
  └─ 全部失败 ↓

第2层：分组备用上游 (fallback-upstreams)
  └─ 只有当第1层全部失败时才使用
  └─ 全部失败 ↓

第3层：全局upstream（全局兜底）
  └─ 只有当第1层和第2层都失败时才使用
  └─ 对所有域名生效，无论是否匹配分组
  └─ 全部失败 ↓

❌ 返回DNS查询失败
```

#### 对于未匹配任何域名分组的查询

```
第1层：全局默认上游分组 (default-upstream-group-id的upstreams)
  └─ 使用指定分组的主要上游
  └─ 全部失败 ↓

第2层：全局默认上游分组的备用 (default-upstream-group-id的fallback-upstreams)
  └─ 使用指定分组的备用上游
  └─ 全部失败 ↓

第3层：全局upstream（全局兜底）
  └─ 只有当第1层和第2层都失败时才使用
  └─ 全部失败 ↓

❌ 返回DNS查询失败
```

### 查询流程示例

#### 示例1：匹配到china域名分组的查询（如baidu.com）

```
1️⃣ 第1层：主要上游
   └─ china-dns的upstreams: [223.5.5.5, 119.29.29.29]
   └─ 全部失败 ↓

2️⃣ 第2层：备用上游
   └─ china-dns的fallback-upstreams: [114.114.114.114]
   └─ 全部失败 ↓

3️⃣ 第3层：全局upstream（最终兜底）
   └─ 全局upstream: [8.8.8.8, 1.1.1.1]
   └─ 全部失败 ↓

❌ 返回DNS查询失败
```

#### 示例2：未匹配任何域名分组的查询（如example.com）

```
1️⃣ 第1层：默认分组主要上游
   └─ global-dns的upstreams: [8.8.8.8, 8.8.4.4]
   └─ 全部失败 ↓

2️⃣ 第2层：默认分组备用上游
   └─ global-dns的fallback-upstreams: [1.1.1.1, 1.0.0.1]
   └─ 全部失败 ↓

3️⃣ 第3层：全局upstream（最终兜底）
   └─ 全局upstream: [8.8.8.8, 1.1.1.1]
   └─ 全部失败 ↓

❌ 返回DNS查询失败
```

### 配置示例

#### 示例1: 完整三层配置

```yaml
upstream-groups:
  - id: china-dns
    upstreams: [223.5.5.5, 119.29.29.29]
    fallback-upstreams: [114.114.114.114]
  
  - id: global-dns
    upstreams: [8.8.8.8, 8.8.4.4]
    fallback-upstreams: [1.1.1.1, 1.0.0.1]

# 未匹配域名的默认分组
default-upstream-group-id: global-dns

# 全局兜底（对所有域名生效）
upstream:
  - 8.8.8.8
  - 1.1.1.1

domain-groups:
  # 中国域名：使用中国DNS，最终兜底用全局upstream
  - name: china
    domain-file: china_domains.txt
    upstream-group-id: china-dns
    # 故障转移：china-dns主要 → china-dns备用 → 全局upstream
```

#### 示例2: 直接配置（不使用分组ID）

```yaml
# 全局兜底
upstream:
  - 8.8.8.8
  - 1.1.1.1

domain-groups:
  - name: cdn
    domain-file: cdn_domains.txt
    upstream: [8.8.4.4, 8.8.8.8]           # 第1层
    fallback-upstream: [1.1.1.1]           # 第2层
    # 第3层使用全局upstream
```

### DomainGroupConfig 结构

```go
type DomainGroupConfig struct {
    GroupName              string   // 分组名称
    UpstreamGroupID        string   // 引用上游分组ID（包含主要+备用）
    Upstreams              []string // 主要上游（第1层，直接配置）
    FallbackUpstreams      []string // 备用上游（第2层，直接配置）
    // 注意：没有DefaultUpstreamGroupID字段
    // 全局upstream在Config级别配置，对所有域名生效
    // ...
}
```

### 实现细节

#### 核心方法

```go
// MatchDomainWithFallback 匹配域名并返回主要和备用上游服务器
func (m *DomainGroupManager) MatchDomainWithFallback(domain string) ([]upstream.Upstream, []upstream.Upstream, error)

// GetDefaultUpstreams 返回未匹配域名的默认上游
func (m *DomainGroupManager) GetDefaultUpstreams(defaultUpstreamGroupID string) ([]upstream.Upstream, []upstream.Upstream, error)

// tryMultiLayerFailover 实现完整的多层故障转移逻辑
func (p *Proxy) tryMultiLayerFailover(req *dns.Msg, host string, isPrivate bool) (*dns.Msg, upstream.Upstream, error)
```

### 配置建议

1. **推荐配置方式**：
   - 使用upstream-groups定义可重用的DNS服务器分组
   - 在domain-groups中通过upstream-group-id引用分组
   - 设置default-upstream-group-id作为未匹配域名的默认分组
   - 设置upstream作为所有域名的最终兜底

2. **简化配置方式**：
   - 直接在domain-groups中配置upstream和fallback-upstream
   - 设置upstream作为全局兜底

3. **最小配置方式**：
   - 只配置upstream（所有域名都使用同一组DNS）

### 优先级规则

1. **主要上游来源**:
   - 如果设置了`upstream-group-id`，使用分组的`upstreams`
   - 否则使用`upstreams`字段

2. **备用上游来源**:
   - 如果设置了`upstream-group-id`，使用分组的`fallback-upstreams`
   - 否则使用`fallback-upstreams`字段

3. **默认上游来源**:
   - 如果设置了`default-upstream-group-id`，使用该分组
   - 否则使用全局`default-upstream-group-id`
   - 如果都没设置，使用全局`upstream`配置

### 使用场景

#### 场景1: 地域优化 + 全局兜底

```yaml
domain-groups:
  - name: china
    upstream: [223.5.5.5]              # 国内DNS（快）
    fallback-upstream: [119.29.29.29]  # 国内备用
    default-upstream-group-id: global-dns  # 国际DNS兜底
```

#### 场景2: 专用DNS + 通用兜底

```yaml
domain-groups:
  - name: internal
    upstream: [192.168.1.1]            # 内网DNS
    fallback-upstream: [10.0.0.1]      # 内网备用
    default-upstream-group-id: global-dns  # 公网DNS兜底
```

#### 场景3: 多级容错

```yaml
domain-groups:
  - name: critical
    upstream: [8.8.8.8, 8.8.4.4]       # Google DNS（主）
    fallback-upstream: [1.1.1.1, 1.0.0.1]  # Cloudflare（备）
    default-upstream-group-id: china-dns   # 国内DNS（兜底）
```

### 性能影响

- **正常情况**: 无影响，只使用第1层
- **第1层失败**: 增加第2层查询延迟
- **第1+2层失败**: 增加第3层查询延迟
- **内存占用**: 每个分组额外 < 1KB

### 监控建议

建议监控以下指标：
- 第1层成功率
- 第2层触发次数
- 第3层触发次数
- 完全失败次数

---

## 完整配置示例（包含三层故障转移）

```yaml
# ============================================================================
# DNSProxy 完整配置示例 (v2.2 - 三层故障转移)
# ============================================================================

# 上游分组
upstream-groups:
  - id: china-dns
    upstreams: [223.5.5.5, 119.29.29.29]
    fallback-upstreams: [114.114.114.114]
  
  - id: global-dns
    upstreams: [8.8.8.8, 1.1.1.1]
    fallback-upstreams: [9.9.9.9]

# 全局默认上游（最终兜底）
default-upstream-group-id: global-dns

# 域名分组
domain-groups:
  # 中国域名：三层完整配置
  - name: china
    domain-file: china_domains.txt
    domain-file-url: https://example.com/china-list.txt
    update-interval: 24h
    upstream-group-id: china-dns           # 第1层+第2层
    default-upstream-group-id: global-dns  # 第3层

  # 广告域名：使用全局默认
  - name: ads
    domain-file: ad_domains.txt
    upstream-group-id: adblock-dns
    # 使用全局default-upstream-group-id作为第3层

  # CDN域名：自定义三层
  - name: cdn
    domain-file: cdn_domains.txt
    upstream: [8.8.4.4, 8.8.8.8]           # 第1层
    fallback-upstream: [1.1.1.1]           # 第2层
    default-upstream-group-id: china-dns   # 第3层

# 缓存和预取
cache: true
cache-size: 10000
cache-prefetch-enabled: true
```

---

## 总结

v2.2版本新增的三层故障转移机制，进一步提升了DNSProxy的可靠性：

### 主要特性

1. **三层保护** - 主要、备用、默认三层故障转移
2. **灵活配置** - 每个分组可独立配置或使用全局默认
3. **分组引用** - 默认层可引用任何上游分组
4. **向后兼容** - 不影响现有配置

### 适用场景

- 需要极高可用性的生产环境
- 跨地域DNS部署
- 内网+公网混合环境
- 关键业务DNS查询

### 下一步

- 查看完整配置示例：`config.yaml.example`
- 阅读相关文档了解更多功能
- 根据实际需求调整故障转移策略
