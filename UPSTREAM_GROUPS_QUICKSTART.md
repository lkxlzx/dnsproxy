# Upstream Groups 快速入门

## 什么是 Upstream Groups？

Upstream Groups（上游服务器分组）允许你将 DNS 服务器组织成不同的组，每个组可以有不同的配置和用途。

## 5 分钟快速开始

### 1. 创建配置文件

创建 `my-groups.yaml`：

```yaml
listen-addrs:
  - "127.0.0.1"
listen-ports:
  - 5353
cache: true

upstream-groups:
  default_group: "cloudflare"
  
  groups:
    - name: "cloudflare"
      upstreams:
        - "1.1.1.1"
        - "1.0.0.1"
    
    - name: "google"
      upstreams:
        - "8.8.8.8"
        - "8.8.4.4"
```

### 2. 启动 dnsproxy

```bash
./dnsproxy --config-path=my-groups.yaml
```

### 3. 测试

```bash
# Linux/Mac
dig @127.0.0.1 -p 5353 google.com

# Windows
nslookup google.com 127.0.0.1:5353
```

## 常见使用场景

### 场景 1：内网和外网分离

```yaml
upstream-groups:
  default_group: "internet"
  
  groups:
    - name: "internet"
      upstreams:
        - "1.1.1.1"
        - "8.8.8.8"
    
    - name: "intranet"
      upstreams:
        - "192.168.1.1"
  
  domain_groups:
    "company.local": "intranet"
    "internal.local": "intranet"
```

### 场景 2：使用加密 DNS

```yaml
upstream-groups:
  default_group: "encrypted"
  
  groups:
    - name: "encrypted"
      mode: "parallel"
      upstreams:
        - "tls://dns.adguard.com"
        - "https://dns.google/dns-query"
```

### 场景 3：性能优化

```yaml
upstream-groups:
  default_group: "fast"
  
  groups:
    - name: "fast"
      mode: "fastest_addr"
      timeout: "3s"
      upstreams:
        - "1.1.1.1"
        - "8.8.8.8"
        - "9.9.9.9"
```

## 配置参数说明

### 组参数

- `name`: 组名（必需）
- `upstreams`: 服务器列表（必需）
- `mode`: 模式（可选）
  - `load_balance`: 负载均衡（默认）
  - `parallel`: 并行查询
  - `fastest_addr`: 最快地址
- `timeout`: 超时时间（可选，默认 10s）
- `enabled`: 是否启用（可选，默认 true）

### 全局参数

- `default_group`: 默认组名（必需）
- `domain_groups`: 域名到组的映射（可选）

## 文本格式配置

如果你喜欢简单的文本格式，创建 `groups.txt`：

```text
[group:primary:load_balance:5s]
1.1.1.1
8.8.8.8

[group:local:load_balance:3s]
192.168.1.1

[/company.local/]local
[default]primary
```

然后使用：

```bash
./dnsproxy --upstream-groups-file=groups.txt -l 127.0.0.1 -p 5353
```

## 完整示例

### 企业级配置

```yaml
listen-addrs:
  - "0.0.0.0"
listen-ports:
  - 53
cache: true
cache-min-ttl: 300
verbose: true

upstream-groups:
  default_group: "public"
  
  groups:
    # 公网 DNS - 快速可靠
    - name: "public"
      mode: "load_balance"
      timeout: "5s"
      max_retries: 2
      enabled: true
      upstreams:
        - "1.1.1.1"
        - "8.8.8.8"
    
    # 内网 DNS - 公司内部
    - name: "corporate"
      mode: "load_balance"
      timeout: "3s"
      enabled: true
      upstreams:
        - "192.168.1.1"
        - "192.168.1.2"
    
    # 安全 DNS - 加密连接
    - name: "secure"
      mode: "parallel"
      timeout: "10s"
      max_retries: 3
      enabled: true
      upstreams:
        - "tls://dns.adguard.com"
        - "https://dns.google/dns-query"
    
    # 备用 DNS - 故障转移
    - name: "backup"
      mode: "load_balance"
      timeout: "15s"
      priority: 10
      enabled: true
      upstreams:
        - "9.9.9.9"
        - "149.112.112.112"
  
  domain_groups:
    # 内网域名
    "corp.local": "corporate"
    "internal.company.com": "corporate"
    "*.internal.company.com": "corporate"
    
    # 敏感域名使用加密
    "banking.example.com": "secure"
    "*.banking.example.com": "secure"
    "secure.company.com": "secure"
```

## 验证配置

启动时查看日志：

```
INFO upstream groups configured count=4 default=public
INFO upstream group name=public upstreams=2 mode=load_balance enabled=true
INFO upstream group name=corporate upstreams=2 mode=load_balance enabled=true
INFO upstream group name=secure upstreams=2 mode=parallel enabled=true
INFO upstream group name=backup upstreams=2 mode=load_balance enabled=true
INFO domain-specific groups count=6
```

## 故障排查

### 配置不生效？

1. 检查 YAML 格式是否正确
2. 确认组名拼写正确
3. 确保设置了 `default_group`
4. 使用 `--verbose` 查看详细日志

### 性能问题？

1. 避免过度使用 `parallel` 模式
2. 调整 `timeout` 参数
3. 启用缓存：`cache: true`
4. 使用 `fastest_addr` 模式

## 下一步

- 阅读完整文档：[UPSTREAM_GROUPS.md](UPSTREAM_GROUPS.md)
- 查看示例配置：[config-groups.yaml.example](config-groups.yaml.example)
- 尝试文本格式：[groups.txt.example](groups.txt.example)

## 获取帮助

如有问题，请：
1. 查看详细文档
2. 使用 `--verbose` 启用详细日志
3. 在 GitHub 提交 Issue
