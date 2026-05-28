# 简单测试指南

由于配置结构较复杂，建议使用以下现有测试程序进行功能验证：

## 1. 预取功能测试

### 使用现有测试程序
```bash
cd dnsproxy
go run test_prefetch_complete.go
```

这个测试会验证：
- 冷启动阶段（访问3次触发预取队列）
- TTL阈值触发
- 自动缓存刷新
- 事件驱动架构

## 2. 域名分组测试

### 使用现有测试程序
```bash
cd dnsproxy
go run test_domain_groups.go
```

这个测试会验证：
- 精确域名匹配
- 子域名匹配
- 动态启用/禁用分组

## 3. 关键字匹配测试

```bash
cd dnsproxy
go run test_keyword_matching.go
```

这个测试会验证：
- 关键字匹配（`keyword:ad`）
- 广告拦截功能

## 4. 通配符匹配测试

```bash
cd dnsproxy
go run test_wildcard_matching.go
```

这个测试会验证：
- 通配符匹配（`*.example.com`）
- 子域名匹配

## 5. 运行所有单元测试

```bash
cd dnsproxy/proxy
go test -v -run TestCachePrefetch
go test -v -run TestDomainGroup
go test -v -run TestHeatTracker
```

## 6. 使用配置文件启动完整代理

### 步骤1: 编译dnsproxy
```bash
cd dnsproxy
go build -o dnsproxy.exe
```

### 步骤2: 使用测试配置启动
```bash
dnsproxy.exe -c test-prefetch-config.yaml
```

### 步骤3: 使用dig或nslookup测试
```bash
# 测试预取
dig @127.0.0.1 -p 53 google.com
dig @127.0.0.1 -p 53 google.com
dig @127.0.0.1 -p 53 google.com

# 测试中国域名分组
dig @127.0.0.1 -p 53 baidu.com
dig @127.0.0.1 -p 53 taobao.com

# 测试广告拦截
dig @127.0.0.1 -p 53 ad-server.example.com
```

## 测试配置文件

### test-prefetch-config.yaml
```yaml
listen-addrs:
  - 127.0.0.1
listen-ports:
  - 53

upstream:
  - 8.8.8.8
  - 1.1.1.1

cache: true
cache-size: 10000

# 预取配置
cache-prefetch-enabled: true
cache-prefetch-threshold-seconds: 5
cache-prefetch-threshold-percent: 80
cache-prefetch-max-concurrent: 10
cache-prefetch-min-heat-threshold: 6
cache-prefetch-time-window: 180s

verbose: true
```

## 验证功能

### 1. 验证预取功能
查看日志中的关键信息：
- `prefetch scheduler started (on-demand mode)` - 预取启动
- `cache prefetch enabled (on-demand mode)` - 预取启用
- `domain joined prefetch queue` - 域名加入预取队列
- `prefetch triggered` - 预取触发

### 2. 验证域名分组
查看日志中的关键信息：
- `domain group matched` - 域名分组匹配
- `using group upstream` - 使用分组上游

### 3. 验证缓存命中
查看日志中的关键信息：
- `replying from cache` - 从缓存回复
- `cache hit` - 缓存命中

## 性能测试

### 基准测试
```bash
cd dnsproxy/proxy
go test -bench=BenchmarkCachePrefetch -benchmem
go test -bench=BenchmarkCache -benchmem
```

### 并发测试
```bash
cd dnsproxy/proxy
go test -run TestHeatTracker_ConcurrentAccess -race
go test -run TestCachePrefetch_ConcurrentRecordAccess -race
```

## 故障排查

### 问题1: 预取未触发
- 检查访问次数是否达到阈值（默认6次）
- 检查时间窗口是否过期（默认180秒）
- 检查TTL是否达到触发阈值

### 问题2: 域名分组不生效
- 检查域名文件是否存在
- 检查域名格式是否正确（需要尾部点号）
- 检查分组是否启用

### 问题3: 缓存未命中
- 检查缓存是否启用
- 检查缓存大小配置
- 检查TTL配置

## 相关文档

- [PREFETCH_FEATURE.md](PREFETCH_FEATURE.md) - 预取功能详细说明
- [DYNAMIC_DOMAIN_GROUPS.md](DYNAMIC_DOMAIN_GROUPS.md) - 域名分组详细说明
- [COMPREHENSIVE_TEST_GUIDE.md](COMPREHENSIVE_TEST_GUIDE.md) - 综合测试指南
