# 综合功能测试指南

本文档说明如何测试 DNSProxy 的所有高级功能。

## 测试内容

### 1. 智能预取功能
- 冷启动阶段（访问次数统计）
- 热度队列管理
- TTL阈值触发
- 自动缓存刷新
- 事件驱动架构验证

### 2. 域名分组功能
- 精确域名匹配
- 子域名匹配
- 通配符匹配（`*.example.com`）
- 关键字匹配（`keyword:ad`）
- 分组优先级

### 3. DNS上游分组
- 不同域名使用不同DNS服务器
- 中国域名使用国内DNS
- 广告域名返回0.0.0.0
- 自定义域名使用自定义DNS

### 4. 动态启用/禁用
- 运行时启用分组
- 运行时禁用分组
- 分组状态查询
- 配置热重载

### 5. 域名列表管理
- 域名列表加载
- 多格式支持（plain, dnsmasq, gfwlist, clash）
- 列表热重载
- 内存管理（LRU）

### 6. 功能组合测试
- 预取 + 域名分组
- 预取 + 关键字匹配
- 预取 + 通配符匹配

## 测试文件

### 测试程序
1. **test_comprehensive_features.go** - 使用代码配置的综合测试
2. **test_with_config.go** - 使用YAML配置文件的测试

### 配置文件
- **test_comprehensive_config.yaml** - 综合测试配置

### 运行脚本
- **run_comprehensive_test.bat** - Windows批处理脚本

## 快速开始

### 方法1: 使用批处理脚本（推荐）

```bash
cd dnsproxy
run_comprehensive_test.bat
```

### 方法2: 手动运行

#### 测试1: 代码配置测试
```bash
cd dnsproxy
go build -o test_comprehensive.exe test_comprehensive_features.go
test_comprehensive.exe
```

#### 测试2: YAML配置测试
```bash
cd dnsproxy
go build -o test_with_config.exe test_with_config.go
test_with_config.exe test_comprehensive_config.yaml
```

## 测试输出说明

### 成功标记
- `✓` - 测试通过
- `⚠` - 警告（功能可能正常但有异常情况）
- `✗` - 测试失败

### 示例输出

```
=== DNSProxy 综合功能测试 ===

✓ 代理启动成功

【测试1】预取功能测试
  测试域名: google.com.
  步骤1: 冷启动阶段 - 访问3次触发预取队列
  ✓ 查询1完成 (TTL: 300秒)
  ✓ 查询2完成 (TTL: 299秒)
  ✓ 查询3完成 (TTL: 298秒)
  步骤2: 等待TTL接近阈值...
  步骤3: 再次查询，应触发预取
  ✓ 查询完成 (TTL: 295秒)
  步骤4: 验证缓存已刷新
  ✓ 缓存TTL: 300秒
  ✓ 预取功能正常 - TTL已刷新

【测试2】域名分组匹配测试
  测试: baidu.com. -> 期望使用 中国DNS
  ✓ 查询成功，返回 3 条记录
  测试: taobao.com. -> 期望使用 中国DNS
  ✓ 查询成功，返回 2 条记录
  测试: google.com. -> 期望使用 默认DNS
  ✓ 查询成功，返回 1 条记录

【测试3】关键字匹配测试
  测试: ad-server.example.com. -> 应被广告组拦截
  ✓ 已拦截 (返回 0.0.0.0)
  测试: tracker.analytics.com. -> 应被广告组拦截
  ✓ 已拦截 (返回 0.0.0.0)
  测试: normal-site.com. -> 应使用默认DNS
  ✓ 正常解析 (返回 93.184.216.34)

【测试4】动态启用/禁用分组测试
  测试域名: example.com. (分组: custom)
  步骤1: 分组禁用状态 - 应使用默认DNS
  ✓ 查询成功 (禁用状态)
  步骤2: 启用分组
  ✓ 分组已启用
  步骤3: 启用状态 - 应使用自定义DNS
  ✓ 查询成功 (启用状态)
  步骤4: 禁用分组
  ✓ 分组已禁用
  步骤5: 再次禁用状态 - 应使用默认DNS
  ✓ 查询成功 (禁用状态)
  ✓ 动态启用/禁用功能正常

【测试5】分组优先级测试
  测试: 域名匹配优先级
  查询: ad-baidu.com. (同时匹配 'ad' 关键字和 'baidu' 域名)
  ✓ 查询成功，返回 1 条记录
  注: 优先级由配置顺序决定

【测试6】预取与分组结合测试
  测试域名: baidu.com. (使用中国DNS分组)
  步骤1: 访问3次触发预取队列
  ✓ 查询1完成 (TTL: 300秒)
  ✓ 查询2完成 (TTL: 299秒)
  ✓ 查询3完成 (TTL: 298秒)
  步骤2: 等待TTL接近阈值...
  步骤3: 再次查询，应触发预取（使用分组DNS）
  ✓ 查询完成 (TTL: 295秒)
  ✓ 预取与分组结合功能正常

=== 所有测试完成 ===
```

## 配置说明

### 预取配置
```yaml
cache-prefetch-enabled: true              # 启用预取
cache-prefetch-threshold-seconds: 3       # 固定阈值（秒）
cache-prefetch-threshold-percent: 80      # 百分比阈值
cache-prefetch-max-concurrent: 5          # 最大并发数
cache-prefetch-min-heat-threshold: 3      # 最小热度阈值
cache-prefetch-time-window: 30s           # 时间窗口
cache-prefetch-max-retries: 2             # 最大重试次数
cache-prefetch-initial-retry-delay: 1s    # 初始重试延迟
```

### 域名分组配置
```yaml
domain-groups:
  - name: china                           # 分组名称
    enabled: true                         # 是否启用
    domains:                              # 域名列表
      - baidu.com                         # 精确匹配
      - "*.cdn.example.com"               # 通配符匹配
      - keyword:ad                        # 关键字匹配
    upstream:                             # 上游DNS
      - 223.5.5.5
      - 119.29.29.29
```

## 故障排查

### 问题1: 预取未触发
**症状**: TTL未刷新
**原因**: 
- 访问次数未达到阈值（默认3次）
- 时间窗口过期（默认30秒）
- TTL未达到触发阈值

**解决**: 
- 增加访问次数
- 缩短时间窗口
- 调整阈值参数

### 问题2: 域名分组不生效
**症状**: 域名未使用指定DNS
**原因**:
- 分组被禁用
- 域名匹配规则错误
- 配置优先级问题

**解决**:
- 检查分组启用状态
- 验证域名格式（需要尾部点号）
- 调整配置顺序

### 问题3: 关键字匹配失败
**症状**: 包含关键字的域名未被匹配
**原因**:
- 关键字格式错误（需要 `keyword:` 前缀）
- 关键字大小写敏感

**解决**:
- 使用正确格式：`keyword:ad`
- 检查关键字拼写

## 性能测试

### 基准测试
```bash
cd dnsproxy/proxy
go test -bench=BenchmarkCachePrefetch -benchmem
```

### 并发测试
```bash
cd dnsproxy/proxy
go test -run TestHeatTracker_ConcurrentAccess -race
```

### 长时间运行测试
```bash
cd dnsproxy
go run test_longrun.go
```

## 高级用法

### 自定义测试域名
编辑 `test_comprehensive_features.go`，修改测试域名：

```go
testCases := []struct {
    domain   string
    group    string
    expected string
}{
    {"your-domain.com.", "your-group", "描述"},
}
```

### 自定义配置
编辑 `test_comprehensive_config.yaml`，添加自定义分组：

```yaml
domain-groups:
  - name: my-group
    enabled: true
    domains:
      - my-domain.com
    upstream:
      - 8.8.8.8
```

### 调试模式
启用详细日志：

```yaml
verbose: true
```

查看日志输出：
- `prefetch triggered` - 预取触发
- `domain group matched` - 域名分组匹配
- `cache hit` - 缓存命中

## 相关文档

- [PREFETCH_FEATURE.md](PREFETCH_FEATURE.md) - 预取功能详细说明
- [DYNAMIC_DOMAIN_GROUPS.md](DYNAMIC_DOMAIN_GROUPS.md) - 域名分组详细说明
- [MULTIFORMAT_DOMAIN_LISTS.md](MULTIFORMAT_DOMAIN_LISTS.md) - 多格式域名列表支持
- [ENHANCED_DNSPROXY_COMPARISON.md](../ENHANCED_DNSPROXY_COMPARISON.md) - 功能对比

## 贡献

如果发现问题或有改进建议，请提交Issue或Pull Request。

## 许可证

与主项目相同
