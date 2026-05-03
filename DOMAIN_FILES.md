# 域名文件加载功能文档

## 概述

域名文件加载功能允许从本地文件或远程 URL 批量加载域名列表，支持多种格式，极大简化了大量域名的配置管理。

## 功能特性

- ✅ 支持本地文件和远程 URL
- ✅ 支持多种文件格式（纯文本、Clash YAML、GFWList）
- ✅ 自动格式检测
- ✅ 支持注释和空行
- ✅ 支持通配符域名
- ✅ 自动去重和清理

## 配置语法

### 基本语法

```yaml
domain_groups:
  # 方式1: 直接指定域名到组的映射
  "example.com": "group_name"
  "*.google.com": "group_name"
  
  # 方式2: 从文件加载域名列表（组名作为键，文件路径作为值）
  "group_name": "/path/to/domains.txt"
  
  # 方式3: 从 URL 加载域名列表
  "group_name": "https://example.com/domains.txt"
```

**重要说明**：
- 当值是文件路径或 URL 时，键就是组名
- 系统通过值的格式自动判断是文件引用还是直接映射
- 不需要使用 `file.` 前缀，可以使用任意组名

### 完整示例

```yaml
upstream-groups:
  default_group: "global"
  
  groups:
    - name: "global"
      upstreams: ["1.1.1.1", "8.8.8.8"]
    - name: "china"
      upstreams: ["223.5.5.5", "119.29.29.29"]
    - name: "local"
      upstreams: ["192.168.1.1"]
  
  domain_groups:
    # 直接指定域名
    "example.com": "global"
    "*.test.com": "local"
    
    # 从本地文件加载（组名: 文件路径）
    "china": "/etc/dnsproxy/domains/china.txt"
    "local": "./domains/local.yaml"
    "ads": "./domains/adblock.txt"
    
    # 从远程 URL 加载（组名: URL）
    "china": "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/ChinaMax/ChinaMax_Classical.yaml"
    "gfw": "https://raw.githubusercontent.com/gfwlist/gfwlist/refs/heads/master/gfwlist.txt"
```

## 支持的文件格式

### 1. 纯文本格式

最简单的格式，每行一个域名。

**文件示例** (`domains.txt`):
```text
# 注释行
example.com
*.google.com
test.local

! 也支持这种注释
another-domain.com
```

**特点**:
- 每行一个域名
- 支持 `#` 和 `!` 注释
- 支持通配符 `*`
- 自动忽略空行

### 2. Clash YAML 格式

Clash 规则文件格式，功能强大。

**文件示例** (`domains.yaml`):
```yaml
payload:
  - DOMAIN,example.com
  - DOMAIN-SUFFIX,google.com
  - DOMAIN-KEYWORD,youtube
```

**规则类型**:
- `DOMAIN` - 精确匹配，转换为 `example.com.`
- `DOMAIN-SUFFIX` - 后缀匹配，转换为 `*.example.com.`
- `DOMAIN-KEYWORD` - 关键词匹配，转换为 `*keyword*.`

**推荐来源**:
```yaml
# 中国域名列表
"file.china": "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/ChinaMax/ChinaMax_Classical.yaml"

# 广告域名列表
"file.ads": "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Advertising/Advertising_Classical.yaml"

# 全球 CDN
"file.cdn": "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Global/Global_Classical.yaml"
```

### 3. GFWList 格式

GFW 翻墙列表格式，base64 编码。

**文件示例**:
```text
[AutoProxy 0.2.9]
base64_encoded_content_here...
```

**特点**:
- Base64 编码
- 自动解码
- 支持多种规则语法
- 自动提取域名

**推荐来源**:
```yaml
# GFWList 官方列表
"file.gfw": "https://raw.githubusercontent.com/gfwlist/gfwlist/refs/heads/master/gfwlist.txt"
```

## 使用场景

### 场景 1: 中国域名分流

将中国域名使用国内 DNS，其他域名使用国际 DNS。

```yaml
upstream-groups:
  default_group: "global"
  
  groups:
    - name: "global"
      upstreams: ["1.1.1.1", "8.8.8.8"]
    - name: "china"
      upstreams: ["223.5.5.5", "119.29.29.29"]
  
  domain_groups:
    # 从 GitHub 加载中国域名列表（组名: URL）
    "china": "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/ChinaMax/ChinaMax_Classical.yaml"
```

### 场景 2: 广告过滤

将广告域名路由到特定组或拦截。

```yaml
upstream-groups:
  default_group: "normal"
  
  groups:
    - name: "normal"
      upstreams: ["1.1.1.1"]
    - name: "adblock"
      upstreams: ["0.0.0.0"]  # 返回无效地址
  
  domain_groups:
    # 加载广告域名列表（组名: 文件路径）
    "adblock": "./domains/adblock.txt"
```

### 场景 3: 内网域名

内网域名使用内网 DNS。

```yaml
upstream-groups:
  default_group: "public"
  
  groups:
    - name: "public"
      upstreams: ["1.1.1.1"]
    - name: "internal"
      upstreams: ["192.168.1.1"]
  
  domain_groups:
    # 加载内网域名列表（组名: 文件路径）
    "internal": "./domains/local.yaml"
```

### 场景 4: GFW 翻墙

被墙域名使用代理 DNS。

```yaml
upstream-groups:
  default_group: "direct"
  
  groups:
    - name: "direct"
      upstreams: ["223.5.5.5"]
    - name: "proxy"
      upstreams: ["8.8.8.8"]  # 通过代理访问
  
  domain_groups:
    # 加载 GFWList（组名: URL）
    "proxy": "https://raw.githubusercontent.com/gfwlist/gfwlist/refs/heads/master/gfwlist.txt"
```

## 文件路径

### 本地文件

```yaml
# 绝对路径
"file.china": "/etc/dnsproxy/domains/china.txt"

# 相对路径（相对于配置文件）
"file.china": "./domains/china.txt"
"file.china": "../shared/domains/china.txt"

# Windows 路径
"file.china": "C:\\dnsproxy\\domains\\china.txt"
```

### 远程 URL

```yaml
# HTTP
"file.china": "http://example.com/domains.txt"

# HTTPS（推荐）
"file.china": "https://example.com/domains.txt"

# GitHub Raw
"file.china": "https://raw.githubusercontent.com/user/repo/master/domains.txt"
```

## 性能优化

### 1. 启动时间

大文件会增加启动时间：

```yaml
# 建议
- 使用本地缓存
- 定期更新而非每次启动
- 考虑文件大小（建议 < 10MB）
```

### 2. 内存使用

域名列表会占用内存：

```yaml
# 估算
- 1万个域名 ≈ 500KB
- 10万个域名 ≈ 5MB
- 100万个域名 ≈ 50MB
```

### 3. 查询性能

使用 map 存储，O(1) 查找：

```yaml
# 不影响查询性能
- 1万个域名：< 1μs
- 100万个域名：< 1μs
```

## 自动更新

### 使用动态重载

```yaml
reload:
  enabled: true
  watch_file: "config.yaml"
  check_interval: "3600s"  # 每小时检查
```

### 使用 Cron 任务

```bash
#!/bin/bash
# update-domains.sh

# 下载最新域名列表
curl -o /etc/dnsproxy/domains/china.txt \
  https://example.com/china-domains.txt

# 触发重载
curl -X POST http://127.0.0.1:8080/api/v1/reload
```

```cron
# 每天凌晨 3 点更新
0 3 * * * /usr/local/bin/update-domains.sh
```

## 错误处理

### 文件加载失败

```yaml
# 日志示例
ERROR failed to load domains source=/path/to/file.txt error="file not found"
```

**解决方案**:
1. 检查文件路径
2. 检查文件权限
3. 检查网络连接（URL）
4. 查看详细日志

### 格式解析失败

```yaml
# 日志示例
WARN failed to parse domain line=10 content="invalid domain"
```

**解决方案**:
1. 检查文件格式
2. 移除无效行
3. 使用正确的格式

### URL 下载失败

```yaml
# 日志示例
ERROR http get failed url=https://example.com/domains.txt error="timeout"
```

**解决方案**:
1. 检查网络连接
2. 检查 URL 有效性
3. 增加超时时间
4. 使用本地缓存

## 最佳实践

### 1. 文件组织

```
/etc/dnsproxy/
├── config.yaml
└── domains/
    ├── china.txt
    ├── local.yaml
    ├── adblock.txt
    └── gfw.txt
```

### 2. 版本控制

```bash
# 使用 Git 管理域名文件
cd /etc/dnsproxy/domains
git init
git add *.txt *.yaml
git commit -m "Initial domain lists"
```

### 3. 备份策略

```bash
# 定期备份
tar -czf domains-backup-$(date +%Y%m%d).tar.gz domains/
```

### 4. 监控和告警

```bash
# 监控域名数量
curl http://127.0.0.1:8080/api/v1/groups | \
  jq '.groups[] | {name: .name, domains: (.upstreams | length)}'
```

### 5. 测试验证

```bash
# 测试域名解析
dig @127.0.0.1 baidu.com
dig @127.0.0.1 google.com

# 验证分流
dig @127.0.0.1 baidu.com +short
dig @127.0.0.1 google.com +short
```

## 示例文件

项目提供了示例文件：

- `domains/china.txt` - 中国常用域名
- `domains/local.yaml` - 内网域名（Clash 格式）
- `domains/adblock.txt` - 广告域名

## 常见问题

### Q: 支持多少个域名？

A: 理论上无限制，实际建议：
- 小型部署：< 1万个域名
- 中型部署：1-10万个域名
- 大型部署：10-100万个域名

### Q: 文件更新后如何生效？

A: 两种方式：
1. 启用动态重载（自动）
2. 手动触发重载：`curl -X POST http://127.0.0.1:8080/api/v1/reload`

### Q: 支持正则表达式吗？

A: 不直接支持，但支持通配符：
- `*.example.com` - 匹配所有子域名
- `example.*` - 匹配所有 TLD

### Q: 如何调试域名加载？

A: 启用详细日志：
```bash
./dnsproxy --config-path=config.yaml --verbose
```

## 总结

域名文件加载功能提供了：

- ✅ 灵活的配置方式
- ✅ 多种格式支持
- ✅ 本地和远程加载
- ✅ 自动格式检测
- ✅ 高性能查询
- ✅ 易于维护

非常适合需要管理大量域名的场景！
