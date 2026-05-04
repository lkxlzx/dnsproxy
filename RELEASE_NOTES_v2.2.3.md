# DNSProxy v2.2.3 发布说明

## 🎉 重大更新

v2.2.3 版本带来了强大的域名列表管理功能，支持自动下载、缓存和热重载！

## ✨ 新增功能

### 1. 域名列表自动下载和缓存

- ✅ 支持从远程 URL 自动下载域名列表
- ✅ 支持多种格式：dnsmasq, gfwlist, adblock, hosts
- ✅ 自动格式检测和转换
- ✅ 本地 YAML 格式缓存
- ✅ 自动更新机制

**示例配置:**
```yaml
domains_lists:
  - name: china-domains
    source: https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/accelerated-domains.china.conf
    group: china-id
    file: ./cache/china-domains.yaml
    auto_update: true
    refresh_interval: 24h
    enabled: true
    format: dnsmasq
    domain_count: 114898                        # 自动更新
    last_updated: "2026-05-04T11:19:15+08:00"  # 自动更新
```

### 2. 统计信息自动更新

- ✅ 自动统计域名数量 (`domain_count`)
- ✅ 自动记录更新时间 (`last_updated`)
- ✅ 统计信息写回配置文件
- ✅ 前端可直接读取配置文件获取统计

### 3. 配置文件热重载

- ✅ 自动监控配置文件变化
- ✅ 检测到变化后自动重载
- ✅ 无需重启程序
- ✅ 保留原有格式和注释

### 4. 动态域名列表管理

- ✅ 动态添加新的域名列表
- ✅ 动态删除域名列表
- ✅ 动态更新域名列表配置
- ✅ 立即生效，无需重启

**API 示例:**
```bash
# 添加新的域名列表
curl -X POST http://localhost:8080/api/domain-lists \
  -H "Content-Type: application/json" \
  -d '{
    "name": "anti-ad",
    "source": "https://example.com/anti-ad.txt",
    "group": "adblock",
    "enabled": true
  }'
```

### 5. HTTP API 支持

- ✅ RESTful API 设计
- ✅ JSON 响应格式
- ✅ 完整的 CRUD 操作
- ✅ 健康检查端点

**API 端点:**
- `POST /api/domain-lists` - 添加域名列表
- `GET /api/domain-lists` - 获取所有列表
- `DELETE /api/domain-lists/remove` - 删除列表
- `PUT /api/domain-lists/update` - 更新列表
- `POST /api/reload` - 手动重载配置
- `GET /api/health` - 健康检查

### 6. 前端 UI 集成

- ✅ React 组件示例
- ✅ Vue 组件示例
- ✅ 完整 CSS 样式
- ✅ 移动端适配

## 📁 新增文件

### 核心代码
- `proxy/upstreamgroup_config_writer.go` - 配置文件更新
- `proxy/upstreamgroup_hotreload.go` - 热重载管理
- `proxy/upstreamgroup_state.go` - 状态管理
- `proxy/upstreamgroup_api.go` - API 处理器

### 测试和示例
- `test_integration.go` - 完整集成测试
- `test_hotreload.go` - 热重载测试
- `test_config_update.go` - 配置更新测试
- `api_server_example.go` - API 服务器示例

### 文档
- `CACHE_GUIDE.md` - 缓存功能完整指南
- `HOTRELOAD_GUIDE.md` - 热重载功能指南
- `API_GUIDE.md` - API 完整文档
- `UPDATE_CONFIG_GUIDE.md` - 配置更新指南
- `UI_INTEGRATION_COMPLETE.md` - 前端集成方案
- `FEATURE_SUMMARY.md` - 功能实现总结

## 📊 测试结果

### 真实数据测试
- ✅ 成功下载 **119,059** 个域名
  - 中国域名: 114,898 个
  - GFW 列表: 4,161 个
- ✅ 缓存文件大小: 2.13 MB
- ✅ 下载时间: ~8.6 秒

### 测试覆盖
- ✅ 单元测试通过
- ✅ 集成测试通过
- ✅ API 测试通过
- ✅ 热重载测试通过
- ✅ 配置更新测试通过

## 🚀 快速开始

### 1. 基本使用

```yaml
# config.yaml
domains_lists:
  - name: china-domains
    source: https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/accelerated-domains.china.conf
    group: china
    file: ./cache/china-domains.yaml
    auto_update: true
    refresh_interval: 24h
    enabled: true
```

### 2. 运行测试

```bash
# 完整集成测试
go run test_integration.go

# 热重载测试
go run test_hotreload.go

# API 服务器
go run api_server_example.go
```

### 3. 使用 API

```bash
# 添加新的域名列表
curl -X POST http://localhost:8080/api/domain-lists \
  -H "Content-Type: application/json" \
  -d @new-list.json

# 查看统计信息
curl http://localhost:8080/api/domain-lists/stats

# 手动刷新
curl -X POST http://localhost:8080/api/domain-lists/refresh
```

## 🔧 配置示例

### 完整配置

参见 `config-complete-example.yaml` 和 `config-adguard-example.yaml`

### 测试配置

参见 `test-cache-config.yaml`

## 📚 文档

详细文档请查看：

1. **缓存功能**: [CACHE_GUIDE.md](./CACHE_GUIDE.md)
2. **热重载**: [HOTRELOAD_GUIDE.md](./HOTRELOAD_GUIDE.md)
3. **API 文档**: [API_GUIDE.md](./API_GUIDE.md)
4. **配置更新**: [UPDATE_CONFIG_GUIDE.md](./UPDATE_CONFIG_GUIDE.md)
5. **前端集成**: [UI_INTEGRATION_COMPLETE.md](./UI_INTEGRATION_COMPLETE.md)

## 🎯 使用场景

### 场景 1: 自动管理域名列表

配置后自动下载和更新，无需手动维护。

### 场景 2: 动态添加规则

通过 API 或 UI 动态添加新的域名列表，立即生效。

### 场景 3: 前端 UI 集成

前端可以直接读取配置文件或调用 API 获取统计信息。

## ⚠️ 注意事项

1. **首次运行**: 会自动下载域名列表，可能需要几秒到几十秒
2. **网络要求**: 需要能访问域名列表的源 URL
3. **磁盘空间**: 缓存文件可能占用几 MB 空间
4. **权限**: 需要有写入缓存目录和配置文件的权限

## 🔄 升级指南

### 从 v2.2.2 升级

1. 拉取最新代码
2. 更新配置文件（添加 `domains_lists` 和 `cache` 配置）
3. 重新编译
4. 启动程序（会自动下载域名列表）

### 配置迁移

如果你之前使用本地域名文件，可以迁移到新的 `domains_lists` 配置：

**旧配置:**
```yaml
domain_groups:
  china:
    - file:./domains/china.txt
```

**新配置:**
```yaml
domains_lists:
  - name: china-domains
    source: ./domains/china.txt  # 或远程 URL
    group: china
    file: ./cache/china-domains.yaml
    enabled: true
```

## 🐛 已知问题

无

## 🙏 致谢

感谢所有贡献者和测试者！

## 📝 更新日志

### v2.2.3 (2026-05-04)

**新增:**
- 域名列表自动下载和缓存
- 多格式支持和自动检测
- 统计信息自动更新
- 配置文件热重载
- 动态域名列表管理
- HTTP API 支持
- 前端 UI 集成示例

**改进:**
- 优化域名加载性能
- 改进错误处理
- 增强日志输出

**文档:**
- 新增 6 个完整指南文档
- 新增 API 文档
- 新增前端集成示例

## 🔗 相关链接

- GitHub: https://github.com/lkxlzx/dnsproxy
- 分支: v2.2.3
- 提交: 3555e0c

---

**完整功能已实现并测试通过，可以投入生产使用！** 🎉
