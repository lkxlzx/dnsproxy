# Upstream Groups 功能文档

## 概述

Upstream Groups（上游服务器分组）功能允许你将 DNS 上游服务器组织成不同的组，每个组可以有独立的配置和行为。这使得你可以：

- 为不同类型的域名使用不同的 DNS 服务器
- 为每个组配置独立的超时、重试策略和负载均衡模式
- 实现更灵活的 DNS 解析策略
- 提高 DNS 查询的可靠性和性能

## 功能特性

### 1. 多组管理
- 创建多个上游服务器组
- 每个组有独立的名称和配置
- 支持启用/禁用特定组

### 2. 组级配置
每个组可以配置：
- **Mode（模式）**：负载均衡策略
  - `load_balance`：负载均衡（默认）
  - `parallel`：并行查询所有服务器
  - `fastest_addr`：返回最快响应的 IP
- **Timeout（超时）**：查询超时时间
- **MaxRetries（最大重试）**：失败重试次数
- **Priority（优先级）**：用于故障转移排序
- **Enabled（启用状态）**：是否启用该组

### 3. 域名路由
- 为特定域名指定使用哪个组
- 支持通配符域名匹配
- 配置默认组处理未匹配的域名

## 配置方式

### 方式一：YAML 配置文件

在 `config.yaml` 中添加 `upstream-groups` 配置：

```yaml
upstream-groups:
  # 默认组
  default_group: "primary"
  
  # 定义组
  groups:
    - name: "primary"
      enabled: true
      mode: "load_balance"
      timeout: "5s"
      max_retries: 2
      priority: 1
      upstreams:
        - "1.1.1.1:53"
        - "8.8.8.8:53"
    
    - name: "secure"
      enabled: true
      mode: "parallel"
      timeout: "10s"
      max_retries: 3
      priority: 2
      upstreams:
        - "tls://dns.adguard.com"
        - "https://dns.google/dns-query"
  
  # 域名到组的映射
  domain_groups:
    "internal.local": "local"
    "*.banking.com": "secure"
```

### 方式二：文本配置文件

创建 `groups.txt` 文件：

```text
# 定义组：[group:名称:模式:超时]
[group:primary:load_balance:5s]
1.1.1.1
8.8.8.8

[group:secure:parallel:10s]
tls://dns.adguard.com
https://dns.google/dns-query

# 域名映射：[/域名1/域名2/]组名
[/internal.local/corp.local/]local
[/banking.com/]secure

# 设置默认组
[default]primary
```

然后在配置文件中引用：

```yaml
upstream-groups-file: "groups.txt"
```

## 使用场景

### 场景 1：内外网分离

```yaml
upstream-groups:
  default_group: "public"
  
  groups:
    - name: "public"
      upstreams:
        - "1.1.1.1"
        - "8.8.8.8"
    
    - name: "internal"
      upstreams:
        - "192.168.1.1"
        - "192.168.1.2"
  
  domain_groups:
    "internal.local": "internal"
    "corp.local": "internal"
    "*.internal.local": "internal"
```

### 场景 2：安全敏感域名加密

```yaml
upstream-groups:
  default_group: "standard"
  
  groups:
    - name: "standard"
      mode: "load_balance"
      upstreams:
        - "1.1.1.1"
        - "8.8.8.8"
    
    - name: "encrypted"
      mode: "parallel"
      upstreams:
        - "tls://dns.adguard.com"
        - "https://dns.google/dns-query"
        - "quic://dns.adguard.com"
  
  domain_groups:
    "banking.com": "encrypted"
    "*.banking.com": "encrypted"
    "secure.example.com": "encrypted"
```

### 场景 3：性能优化

```yaml
upstream-groups:
  default_group: "balanced"
  
  groups:
    - name: "balanced"
      mode: "load_balance"
      timeout: "5s"
      upstreams:
        - "1.1.1.1"
        - "8.8.8.8"
    
    - name: "fastest"
      mode: "fastest_addr"
      timeout: "3s"
      upstreams:
        - "1.1.1.1"
        - "1.0.0.1"
        - "8.8.8.8"
        - "8.8.4.4"
    
    - name: "reliable"
      mode: "parallel"
      timeout: "10s"
      max_retries: 5
      upstreams:
        - "1.1.1.1"
        - "8.8.8.8"
        - "9.9.9.9"
  
  domain_groups:
    "cdn.example.com": "fastest"
    "critical.example.com": "reliable"
```

### 场景 4：多级故障转移

```yaml
upstream-groups:
  default_group: "tier1"
  
  groups:
    - name: "tier1"
      priority: 1
      timeout: "3s"
      upstreams:
        - "1.1.1.1"
        - "8.8.8.8"
    
    - name: "tier2"
      priority: 2
      timeout: "5s"
      upstreams:
        - "9.9.9.9"
        - "149.112.112.112"
    
    - name: "tier3"
      priority: 3
      timeout: "10s"
      upstreams:
        - "208.67.222.222"
        - "208.67.220.220"
```

## 命令行使用

### 启动时指定配置文件

```bash
./dnsproxy --config-path=config-groups.yaml
```

### 使用文本格式的组配置

```bash
./dnsproxy --upstream-groups-file=groups.txt
```

### 组合使用

```bash
./dnsproxy \
  --config-path=config.yaml \
  --upstream-groups-file=groups.txt \
  --cache \
  --verbose
```

## 配置参数详解

### 组配置参数

| 参数 | 类型 | 必需 | 默认值 | 说明 |
|------|------|------|--------|------|
| `name` | string | 是 | - | 组的唯一标识符 |
| `upstreams` | []string | 是 | - | 上游服务器地址列表 |
| `mode` | string | 否 | load_balance | 负载均衡模式 |
| `timeout` | string | 否 | 10s | 查询超时时间 |
| `max_retries` | int | 否 | 0 | 最大重试次数 |
| `enabled` | bool | 否 | true | 是否启用该组 |
| `priority` | int | 否 | 0 | 优先级（越小越高） |

### 模式说明

#### load_balance（负载均衡）
- 在组内的服务器之间分配请求
- 适合大多数场景
- 提供良好的性能和可靠性

#### parallel（并行查询）
- 同时向组内所有服务器发送查询
- 返回最快的响应
- 适合对可靠性要求高的场景
- 会增加网络流量

#### fastest_addr（最快地址）
- 检测最快响应的 IP 地址
- 只返回最快的 IP
- 适合 CDN 和性能敏感的场景
- 需要配合缓存使用

## 监控和调试

### 启用详细日志

```bash
./dnsproxy --config-path=config-groups.yaml --verbose
```

### 查看组信息

启动时会输出组配置信息：

```
INFO upstream groups configured count=3 default=primary
INFO upstream group name=primary upstreams=2 mode=load_balance enabled=true
INFO upstream group name=secure upstreams=2 mode=parallel enabled=true
INFO domain-specific groups count=5
```

## 最佳实践

### 1. 合理设置超时时间
- 本地 DNS：1-3 秒
- 公共 DNS：3-5 秒
- 加密 DNS：5-10 秒

### 2. 选择合适的模式
- 日常使用：`load_balance`
- 关键业务：`parallel`
- CDN 优化：`fastest_addr`

### 3. 配置故障转移
- 使用 `priority` 设置多级故障转移
- 为备用组设置更长的超时时间
- 为备用组配置更多重试次数

### 4. 域名分组策略
- 内网域名使用内网 DNS
- 敏感域名使用加密 DNS
- CDN 域名使用快速模式
- 其他域名使用默认组

### 5. 性能优化
- 启用缓存减少查询次数
- 使用 `fastest_addr` 模式优化 CDN
- 合理设置 `cache-min-ttl`

## 故障排查

### 问题：组配置不生效

检查：
1. 配置文件路径是否正确
2. YAML 格式是否正确
3. 组名是否唯一
4. 默认组是否存在

### 问题：域名路由不工作

检查：
1. 域名格式是否正确
2. 组名是否存在
3. 组是否启用（`enabled: true`）
4. 是否设置了默认组

### 问题：性能不佳

优化：
1. 减少 `parallel` 模式的使用
2. 调整超时时间
3. 启用缓存
4. 使用 `fastest_addr` 模式

## 示例配置文件

完整的示例配置文件请参考：
- `config-groups.yaml.example` - YAML 格式示例
- `groups.txt.example` - 文本格式示例

## API 集成

如果你需要在代码中使用组功能：

```go
import "github.com/AdguardTeam/dnsproxy/proxy"

// 创建组配置
ugc := proxy.NewUpstreamGroupConfig()

// 添加组
group := &proxy.UpstreamGroup{
    Name:    "primary",
    Mode:    proxy.UpstreamModeLoadBalance,
    Timeout: 5 * time.Second,
    Enabled: true,
}
ugc.AddGroup(group)

// 设置域名映射
ugc.SetDomainGroup("example.com", "primary")
ugc.DefaultGroup = "primary"

// 验证配置
if err := ugc.Validate(); err != nil {
    log.Fatal(err)
}
```

## 贡献

欢迎提交问题和改进建议到项目的 GitHub 仓库。
