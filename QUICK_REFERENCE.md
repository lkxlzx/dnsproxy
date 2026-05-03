# 上游分组功能 - 快速参考

## 📋 目录

- [基础配置](#基础配置)
- [域名文件](#域名文件)
- [混合模式](#混合模式)
- [常用命令](#常用命令)
- [API 端点](#api-端点)

## 基础配置

### 最简配置
```yaml
upstream-groups:
  default_group: "default"
  groups:
    - name: "default"
      upstreams: ["8.8.8.8", "1.1.1.1"]
```

### 国内外分流
```yaml
upstream-groups:
  default_group: "overseas"
  groups:
    - name: "overseas"
      upstreams: ["8.8.8.8", "1.1.1.1"]
    - name: "china"
      upstreams: ["223.5.5.5", "119.29.29.29"]
  
  domain_groups:
    "baidu.com": "china"
    "*.baidu.com": "china"
    "qq.com": "china"
    "*.qq.com": "china"
```

### 负载均衡模式
```yaml
groups:
  - name: "load-balance"
    mode: "load_balance"      # 轮询
    upstreams: ["8.8.8.8", "1.1.1.1"]
  
  - name: "parallel"
    mode: "parallel"          # 并行查询
    upstreams: ["8.8.8.8", "1.1.1.1"]
  
  - name: "fastest"
    mode: "fastest_addr"      # 最快响应
    upstreams: ["8.8.8.8", "1.1.1.1", "9.9.9.9"]
```

## 域名文件

### 支持的格式
| 格式 | 示例 |
|------|------|
| Plain Text | `example.com` |
| Clash YAML | `payload: [example.com]` |
| Surge | `DOMAIN,example.com` |
| Dnsmasq | `server=/example.com/` |
| Hosts | `127.0.0.1 example.com` |
| AdBlock | `\|\|example.com^` |
| GFWList | Base64 编码 |
| JSON | `{"domains":["example.com"]}` |

### 本地文件
```yaml
domain_groups:
  "china": "./domains/china.txt"
  "local": "./domains/local.yaml"
  "ads": "./domains/adblock.txt"
```

### 远程 URL
```yaml
domain_groups:
  "china": "https://raw.githubusercontent.com/.../china.yaml"
  "gfw": "https://raw.githubusercontent.com/.../gfwlist.txt"
```

### 混合使用
```yaml
domain_groups:
  # 本地文件（快速）
  "local": "./domains/local.txt"
  
  # 远程 URL（自动下载、转换、缓存）
  "china": "https://example.com/china.yaml"
  "ads": "https://example.com/adblock.txt"
```

## 混合模式

### dnsproxy 核心功能
```go
// 创建管理器
manager := proxy.NewDomainListManager(cacheDir, logger)

// 下载并缓存（自动格式转换）
err := manager.DownloadAndCache(url, localPath)

// 添加列表
list := &proxy.ManagedList{
    Name:      "china",
    Source:    url,
    LocalPath: localPath,
    Group:     "china",
    Enabled:   true,
}
manager.AddList(list)

// 构建配置
config := manager.BuildDomainGroupsConfig()
```

### AdGuard Home 集成
```go
// 简单的 API 转发
func handleAddDomainList(w http.ResponseWriter, r *http.Request) {
    // 1. 调用 dnsproxy（自动处理一切）
    manager.DownloadAndCache(req.URL, localPath)
    
    // 2. 添加到管理器
    manager.AddList(list)
    
    // 3. 保存配置
    saveConfig()
    
    // 4. 重载 DNS
    reloadDNS()
}
```

## 常用命令

### 启动服务
```bash
# 基础配置
./dnsproxy --config-path=config-groups.yaml.example

# 域名文件配置
./dnsproxy --config-path=config-groups-domains.yaml.example

# 高级配置
./dnsproxy --config-path=config-groups-advanced.yaml.example
```

### 测试查询
```bash
# 测试国内域名
dig @127.0.0.1 -p 5301 baidu.com

# 测试海外域名
dig @127.0.0.1 -p 5301 google.com

# 测试子域名
dig @127.0.0.1 -p 5301 www.baidu.com
```

### 运行测试
```bash
# 所有测试
go test -v ./proxy

# 上游分组测试
go test -v ./proxy -run UpstreamGroup

# 域名加载测试
go test -v ./proxy -run DomainFileLoader

# 管理器测试
go test -v ./proxy -run DomainListManager
```

## API 端点

### 基础端点
```bash
# 服务器状态
curl http://localhost:8080/api/v1/status

# 列出所有组
curl http://localhost:8080/api/v1/groups

# 获取组信息
curl http://localhost:8080/api/v1/groups/china
```

### 统计信息
```bash
# 所有统计
curl http://localhost:8080/api/v1/stats

# 组统计
curl http://localhost:8080/api/v1/stats/china
```

### 健康检查
```bash
# 所有健康状态
curl http://localhost:8080/api/v1/health

# 上游健康状态
curl http://localhost:8080/api/v1/health/8.8.8.8
```

### 重载配置
```bash
# 触发重载
curl -X POST http://localhost:8080/api/v1/reload \
  -H "Authorization: Bearer your-token"
```

## 配置文件

### 文件位置
- 基础：`config-groups.yaml.example`
- 高级：`config-groups-advanced.yaml.example`
- 域名：`config-groups-domains.yaml.example`
- 测试：`config-test-domain-groups.yaml`
- 文本：`groups.txt.example`

### 域名文件位置
- `domains/china.txt` - 中国域名
- `domains/local.yaml` - 内网域名
- `domains/adblock.txt` - 广告域名

## 文档

### 入门文档
- `UPSTREAM_GROUPS_README.md` - 功能总览
- `UPSTREAM_GROUPS_QUICKSTART.md` - 5分钟入门
- `UPSTREAM_GROUPS.md` - 完整文档

### 高级文档
- `UPSTREAM_GROUPS_ADVANCED.md` - 高级功能
- `DOMAIN_FILES.md` - 域名文件
- `FORMAT_CONVERTER.md` - 格式转换
- `ADGUARD_HOME_INTEGRATION.md` - AdGuard Home 集成

### 参考文档
- `PROJECT_COMPLETE.md` - 项目完成报告
- `FINAL_TEST_SUMMARY.md` - 测试总结
- `FEATURES_CHECKLIST.md` - 功能清单

## 性能参考

| 操作 | 性能 |
|------|------|
| 域名查找 | < 1μs |
| 本地文件加载 | < 1ms |
| 远程文件下载 | 取决于网络 |
| 格式转换 | < 10ms (10万域名) |
| 内存占用 | ~5MB (10万域名) |

## 故障排查

### 域名不匹配
1. 检查域名格式（是否有尾部点）
2. 检查通配符语法（`*.example.com`）
3. 查看日志确认加载成功

### 文件加载失败
1. 检查文件路径
2. 检查文件格式
3. 查看错误日志

### 远程下载失败
1. 检查网络连接
2. 检查 URL 是否正确
3. 使用本地缓存

## 最佳实践

1. ✅ 使用本地缓存提高性能
2. ✅ 启用健康检查保证可用性
3. ✅ 启用统计信息监控性能
4. ✅ 使用通配符简化配置
5. ✅ 定期更新域名列表
6. ✅ 配置备用上游组
7. ✅ 使用 API 进行管理

## 获取帮助

- 查看文档：`docs/`
- 查看示例：`config-*.yaml.example`
- 运行演示：`demo-*.sh` 或 `demo-*.bat`
- 查看测试：`proxy/*_test.go`

---

**快速开始**: 复制 `config-groups.yaml.example`，修改上游地址，启动服务！
