# 自动格式检测功能

## 功能说明

DNSProxy 支持自动检测域名列表的格式，无需手动指定 `format` 字段。程序会根据文件内容和 URL 特征自动识别格式类型。

## 支持的格式

程序可以自动检测以下格式：

1. **clash** - Clash YAML 格式
2. **gfwlist** - GFWList Base64 编码格式
3. **surge** - Surge 规则格式
4. **dnsmasq** - Dnsmasq 配置格式
5. **hosts** - Hosts 文件格式
6. **adblock** - AdBlock Plus 过滤规则
7. **json** - JSON 格式
8. **plain** - 纯文本格式（每行一个域名）

## 使用方法

### 方式 1：省略 format 字段

```yaml
domains_lists:
  - name: china-domains
    source: https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/accelerated-domains.china.conf
    group: china
    file: ./cache/china-domains.yaml
    enabled: true
    # format 字段省略，程序会自动检测
```

### 方式 2：使用 format: auto

```yaml
domains_lists:
  - name: gfwlist
    source: https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt
    group: overseas
    file: ./cache/gfwlist.yaml
    enabled: true
    format: auto  # 明确指定自动检测
```

### 方式 3：手动指定格式（推荐用于已知格式）

```yaml
domains_lists:
  - name: adblock-list
    source: https://example.com/adblock.txt
    group: adblock
    file: ./cache/adblock.yaml
    enabled: true
    format: adblock  # 手动指定，跳过检测，提高性能
```

## 检测逻辑

### 1. 文件扩展名检测

```
.yaml/.yml → 检查是否为 Clash 格式
.txt       → 进一步内容检测
.conf      → 检查是否为 dnsmasq 格式
.json      → JSON 格式
```

### 2. URL 特征检测

```
gfwlist.txt        → gfwlist 格式
china.conf         → dnsmasq 格式
adblock/filter.txt → adblock 格式
```

### 3. 内容特征检测

```
payload:           → Clash 格式
server=/           → dnsmasq 格式
0.0.0.0           → hosts 格式
||domain^         → adblock 格式
DOMAIN,           → Surge 格式
base64 编码       → gfwlist 格式
```

## 检测结果

### 自动更新配置文件

程序检测到格式后，会自动更新配置文件中的 `format` 字段：

**初始配置：**
```yaml
domains_lists:
  - name: gfwlist
    source: https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt
    group: overseas
    file: ./cache/gfwlist.yaml
    enabled: true
    format: auto  # 或省略
```

**运行后自动更新为：**
```yaml
domains_lists:
  - name: gfwlist
    source: https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt
    group: overseas
    file: ./cache/gfwlist.yaml
    enabled: true
    format: gfwlist  # ✅ 自动检测并更新
    domain_count: 4161
    last_updated: "2026-05-04T10:55:52+08:00"
```

### 日志输出

程序会在日志中显示检测过程：

```
time=2026-05-04T10:55:48.579+08:00 level=INFO msg="format will be auto-detected" name=china-domains
time=2026-05-04T10:55:52.101+08:00 level=INFO msg="format detected" name=china-domains format=dnsmasq
```

## 测试示例

### 测试程序

```bash
# 运行自动格式检测测试
go run test_auto_format.go
```

### 测试输出

```
=== 测试自动格式检测功能 ===

📄 初始配置中的 format 字段:
   china-domains: format="(未指定)"
   gfwlist: format="auto"

=== 开始解析和下载域名列表 ===
✓ 正在下载和检测格式...

=== 格式检测完成 ===

📊 检测到的格式:
   china-domains: format="dnsmasq"
   gfwlist: format="gfwlist"

=== 成功! ===
✅ 格式自动检测功能正常工作
✅ 配置文件已更新为检测到的格式
```

## 性能考虑

### 自动检测 vs 手动指定

| 方式 | 优点 | 缺点 | 适用场景 |
|------|------|------|----------|
| **自动检测** | 无需了解格式<br>配置简单<br>自动适应 | 需要下载内容<br>略微增加处理时间 | 不确定格式<br>多种来源<br>快速配置 |
| **手动指定** | 跳过检测步骤<br>性能最优<br>明确可控 | 需要了解格式<br>配置复杂 | 已知格式<br>性能敏感<br>生产环境 |

### 性能影响

- 自动检测增加的时间：< 1ms（仅内容分析）
- 对于大文件（100k+ 域名）：影响可忽略
- 建议：首次使用自动检测，确认后可改为手动指定

## 格式检测规则详解

### Clash 格式

**特征：**
```yaml
payload:
  - DOMAIN,example.com
  - DOMAIN-SUFFIX,google.com
```

**检测条件：**
- 文件扩展名为 `.yaml` 或 `.yml`
- 内容包含 `payload:`

### GFWList 格式

**特征：**
```
W0F1dG9Qcm94eSAwLjIuOV0KISBDaGVja3N1bTogOEJBQzh...
```

**检测条件：**
- URL 包含 `gfwlist`
- 内容为 Base64 编码
- 解码后包含 `[AutoProxy` 标记

### Dnsmasq 格式

**特征：**
```
server=/example.com/114.114.114.114
server=/google.com/8.8.8.8
```

**检测条件：**
- 文件扩展名为 `.conf`
- 内容包含 `server=/`

### Hosts 格式

**特征：**
```
0.0.0.0 example.com
127.0.0.1 localhost
```

**检测条件：**
- 内容包含 `0.0.0.0` 或 `127.0.0.1`
- 格式为 `IP 域名`

### AdBlock 格式

**特征：**
```
||example.com^
||ads.google.com^$third-party
```

**检测条件：**
- 内容包含 `||` 和 `^`
- URL 包含 `adblock` 或 `filter`

### Surge 格式

**特征：**
```
DOMAIN,example.com
DOMAIN-SUFFIX,google.com
DOMAIN-KEYWORD,ads
```

**检测条件：**
- 内容包含 `DOMAIN,` 或 `DOMAIN-SUFFIX,`
- 不包含 `payload:`（区别于 Clash）

### JSON 格式

**特征：**
```json
{
  "domains": ["example.com", "google.com"]
}
```

**检测条件：**
- 文件扩展名为 `.json`
- 内容为有效的 JSON

### 纯文本格式

**特征：**
```
example.com
google.com
*.youtube.com
```

**检测条件：**
- 不匹配其他任何格式
- 每行一个域名

## API 接口

### GetLastDetectedFormat

获取最后检测到的格式：

```go
loader := proxy.NewDomainFileLoader(logger)
domains, _ := loader.LoadDomains(source)
format := loader.GetLastDetectedFormat()
fmt.Printf("Detected format: %s\n", format)
```

### 在配置解析中使用

```go
// 解析配置时自动检测格式
spec := &proxy.UpstreamGroupsSpec{
    DomainLists: []proxy.DomainListSpec{
        {
            Name:    "test-list",
            Source:  "https://example.com/list.txt",
            Format:  "auto", // 或省略
            Enabled: true,
        },
    },
}

// 解析后，Format 字段会被更新为检测到的格式
ugc, _ := proxy.ParseUpstreamGroups(spec, opts)
fmt.Printf("Detected format: %s\n", spec.DomainLists[0].Format)
```

## 最佳实践

### 1. 开发阶段

使用自动检测，快速配置：

```yaml
domains_lists:
  - name: test-list
    source: https://example.com/list.txt
    group: test
    file: ./cache/test.yaml
    enabled: true
    # 省略 format，让程序自动检测
```

### 2. 生产环境

确认格式后，手动指定以提高性能：

```yaml
domains_lists:
  - name: china-domains
    source: https://example.com/china.conf
    group: china
    file: ./cache/china.yaml
    enabled: true
    format: dnsmasq  # 明确指定，跳过检测
```

### 3. 混合使用

已知格式手动指定，未知格式自动检测：

```yaml
domains_lists:
  # 已知格式
  - name: gfwlist
    source: https://example.com/gfwlist.txt
    format: gfwlist
    enabled: true
  
  # 未知格式
  - name: custom-list
    source: https://example.com/custom.txt
    format: auto  # 自动检测
    enabled: true
```

## 故障排查

### 格式检测失败

**问题：** 程序无法正确检测格式

**解决方案：**
1. 检查文件内容是否符合标准格式
2. 手动指定 `format` 字段
3. 查看日志中的检测信息
4. 使用 `plain` 格式作为后备

### 检测到错误的格式

**问题：** 检测到的格式不正确

**解决方案：**
1. 手动指定正确的 `format`
2. 检查文件内容是否混合了多种格式
3. 提交 issue 报告检测问题

### 性能问题

**问题：** 自动检测导致启动变慢

**解决方案：**
1. 改为手动指定 `format`
2. 使用缓存文件（首次检测后不再重复）
3. 减少域名列表数量

## 总结

✅ **自动检测**：支持 8 种常见格式
✅ **智能识别**：基于扩展名、URL 和内容特征
✅ **自动更新**：检测结果写回配置文件
✅ **灵活配置**：支持自动检测和手动指定
✅ **性能优化**：检测开销可忽略

自动格式检测让配置更简单，无需了解各种域名列表的格式细节！
