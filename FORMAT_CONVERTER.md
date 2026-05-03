# 域名格式转换器文档

## 概述

域名格式转换器提供了强大的格式自动检测和转换功能，支持多种常见的域名列表格式之间的相互转换。

## 支持的格式

### 输入格式（自动检测）

1. **Plain Text** - 纯文本格式
2. **Clash YAML** - Clash 规则格式
3. **Surge** - Surge 规则格式
4. **Dnsmasq** - Dnsmasq 配置格式
5. **Hosts** - Hosts 文件格式
6. **AdBlock** - AdBlock Plus 过滤规则
7. **GFWList** - GFW 列表（base64）
8. **JSON** - JSON 格式

### 输出格式

支持转换为以上所有格式。

## 格式说明

### 1. Plain Text（纯文本）

最简单的格式，每行一个域名。

```text
# 注释
example.com
*.google.com
test.local
```

**特点**：
- 每行一个域名
- 支持 `#` 注释
- 支持通配符 `*`

### 2. Clash YAML

Clash 代理工具的规则格式。

```yaml
payload:
  - DOMAIN,example.com
  - DOMAIN-SUFFIX,google.com
  - DOMAIN-KEYWORD,youtube
```

**规则类型**：
- `DOMAIN` - 精确匹配
- `DOMAIN-SUFFIX` - 后缀匹配
- `DOMAIN-KEYWORD` - 关键词匹配

### 3. Surge

Surge 代理工具的规则格式。

```text
DOMAIN,example.com
DOMAIN-SUFFIX,google.com
DOMAIN-KEYWORD,youtube
```

**规则类型**：同 Clash

### 4. Dnsmasq

Dnsmasq DNS 服务器配置格式。

```text
server=/example.com/8.8.8.8
address=/ads.com/0.0.0.0
```

**配置项**：
- `server=` - 指定上游服务器
- `address=` - 指定解析地址

### 5. Hosts

标准 hosts 文件格式。

```text
127.0.0.1 localhost
0.0.0.0 ads.example.com
```

**格式**：`IP地址 域名 [别名...]`

### 6. AdBlock

AdBlock Plus 过滤规则格式。

```text
! 注释
||ads.example.com^
@@||whitelist.com^
```

**规则**：
- `||domain^` - 阻止域名
- `@@||domain^` - 白名单
- `!` - 注释

### 7. GFWList

GFW 列表格式（base64 编码）。

```text
[AutoProxy 0.2.9]
base64_encoded_content...
```

**特点**：
- Base64 编码
- 自动解码
- 支持多种规则

### 8. JSON

JSON 格式域名列表。

```json
{
  "generated_at": "2024-01-01T00:00:00Z",
  "count": 100,
  "domains": [
    "example.com",
    "google.com"
  ]
}
```

或简单数组格式：

```json
["example.com", "google.com"]
```

## 自动格式检测

系统会根据以下特征自动检测格式：

### 1. 文件扩展名

- `.yaml`, `.yml` → Clash YAML
- `.json` → JSON
- `.conf` → Surge/Dnsmasq
- `.txt` → Plain Text

### 2. 内容特征

- 包含 `payload:` → Clash
- 包含 `server=/` → Dnsmasq
- 包含 `DOMAIN-SUFFIX,` → Surge
- 包含 `||` → AdBlock
- 包含 `[AutoProxy` → GFWList
- IP + 域名 → Hosts

## 使用示例

### 配置中使用

```yaml
domain_groups:
  # 自动检测格式
  "file.china": "https://example.com/domains.yaml"  # Clash
  "file.ads": "https://example.com/adblock.txt"     # AdBlock
  "file.local": "/etc/hosts"                        # Hosts
```

### 格式转换示例

系统会自动：
1. 检测源文件格式
2. 解析域名列表
3. 转换为内部格式
4. 应用到配置

## 转换规则

### 通配符处理

| 源格式 | 通配符 | 转换后 |
|--------|--------|--------|
| Plain | `*.example.com` | `*.example.com.` |
| Clash | `DOMAIN-SUFFIX,example.com` | `*.example.com.` |
| Surge | `DOMAIN-SUFFIX,example.com` | `*.example.com.` |
| AdBlock | `||example.com^` | `example.com.` |

### 精确匹配

| 源格式 | 规则 | 转换后 |
|--------|------|--------|
| Plain | `example.com` | `example.com.` |
| Clash | `DOMAIN,example.com` | `example.com.` |
| Surge | `DOMAIN,example.com` | `example.com.` |
| Hosts | `0.0.0.0 example.com` | `example.com.` |

### 关键词匹配

| 源格式 | 规则 | 转换后 |
|--------|------|--------|
| Clash | `DOMAIN-KEYWORD,youtube` | `*youtube*.` |
| Surge | `DOMAIN-KEYWORD,youtube` | `*youtube*.` |

## 格式转换 API

### 编程接口

```go
// 创建转换器
converter := NewFormatConverter(logger)

// 转换为不同格式
plainText, _ := converter.Convert(domains, "plain")
clashYAML, _ := converter.Convert(domains, "clash")
surgeRules, _ := converter.Convert(domains, "surge")
dnsmasqConf, _ := converter.Convert(domains, "dnsmasq")
hostsFile, _ := converter.Convert(domains, "hosts")
adblockRules, _ := converter.Convert(domains, "adblock")
jsonData, _ := converter.Convert(domains, "json")
```

## 实际应用场景

### 场景 1：统一格式管理

从不同来源收集域名，统一转换为内部格式。

```yaml
domain_groups:
  # Clash 格式
  "file.china": "https://github.com/user/repo/clash-rules.yaml"
  
  # AdBlock 格式
  "file.ads": "https://easylist.to/easylist/easylist.txt"
  
  # Hosts 格式
  "file.block": "/etc/hosts.block"
  
  # 自动转换并合并
```

### 场景 2：格式转换工具

将一种格式转换为另一种格式。

```bash
# 加载 Clash 格式
# 自动转换为内部格式
# 导出为 Dnsmasq 格式
```

### 场景 3：多源聚合

从多个不同格式的源聚合域名列表。

```yaml
domain_groups:
  "file.combined": [
    "https://source1.com/clash.yaml",
    "https://source2.com/adblock.txt",
    "/local/hosts.txt"
  ]
```

## 性能优化

### 1. 缓存机制

```yaml
# 建议启用缓存
cache:
  enabled: true
  ttl: 3600  # 1小时
```

### 2. 增量更新

```yaml
# 只更新变化的部分
reload:
  enabled: true
  check_interval: "3600s"
```

### 3. 并行处理

系统自动并行处理多个文件。

## 错误处理

### 格式检测失败

```
WARN unknown format, trying plain text source=file.txt
```

**解决方案**：
- 检查文件内容
- 手动指定格式
- 使用纯文本格式

### 解析失败

```
ERROR failed to parse format=clash error="invalid yaml"
```

**解决方案**：
- 验证文件格式
- 检查语法错误
- 查看详细日志

### 转换失败

```
ERROR conversion failed target=dnsmasq error="unsupported rule"
```

**解决方案**：
- 检查源数据
- 使用支持的格式
- 查看转换规则

## 最佳实践

### 1. 格式选择

- **简单场景**：使用 Plain Text
- **代理工具**：使用 Clash/Surge
- **DNS 服务器**：使用 Dnsmasq
- **广告过滤**：使用 AdBlock
- **程序处理**：使用 JSON

### 2. 文件组织

```
domains/
├── plain/
│   ├── china.txt
│   └── local.txt
├── clash/
│   └── rules.yaml
├── adblock/
│   └── filters.txt
└── converted/
    ├── dnsmasq.conf
    └── hosts
```

### 3. 版本控制

```bash
# 使用 Git 管理
git add domains/
git commit -m "Update domain lists"
```

### 4. 自动化

```bash
#!/bin/bash
# 自动下载和转换

# 下载 Clash 格式
curl -o clash.yaml https://example.com/rules.yaml

# 下载 AdBlock 格式
curl -o adblock.txt https://example.com/filters.txt

# 触发重载（自动转换）
curl -X POST http://127.0.0.1:8080/api/v1/reload
```

## 支持的转换路径

```
Plain Text ←→ Clash YAML
Plain Text ←→ Surge
Plain Text ←→ Dnsmasq
Plain Text ←→ Hosts
Plain Text ←→ AdBlock
Plain Text ←→ JSON
Plain Text ←→ GFWList

Clash YAML ←→ Surge
Clash YAML ←→ JSON

AdBlock ←→ Hosts
AdBlock ←→ Dnsmasq

... (所有格式之间都可以转换)
```

## 常见问题

### Q: 支持哪些格式？

A: 支持 8 种常见格式：Plain Text、Clash、Surge、Dnsmasq、Hosts、AdBlock、GFWList、JSON。

### Q: 如何指定格式？

A: 系统会自动检测，无需手动指定。

### Q: 转换会丢失信息吗？

A: 基本信息（域名）不会丢失，但某些格式特有的元数据可能会丢失。

### Q: 性能如何？

A: 非常快，10万个域名的转换通常在 1 秒内完成。

### Q: 支持自定义格式吗？

A: 目前不支持，但可以扩展代码添加新格式。

## 总结

格式转换器提供了：

- ✅ 8 种格式支持
- ✅ 自动格式检测
- ✅ 双向转换
- ✅ 高性能处理
- ✅ 错误处理
- ✅ 易于使用

让域名列表管理变得简单高效！
