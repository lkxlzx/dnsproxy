# Smart Cache Prefetch Feature

## 概述

智能缓存预获取机制是对dnsproxy缓存系统的增强，它将被动的缓存过期机制替换为主动的缓存刷新机制。该功能基于域名访问热度，在缓存即将过期前主动刷新热门域名的DNS记录，从而避免缓存过期导致的查询延迟。

## 核心特性

### 1. 双时钟系统
- **全局时钟T**: 用于TTL管理，每秒递增，预获取成功后重置为0
- **真实时间**: 用于热度管理，不受T重置影响

### 2. 热度跟踪
- **冷启动阶段**: 域名在180秒内访问6次后加入主动刷新队列
- **队列管理**: 只对队列中的域名进行预获取
- **不活跃剔除**: 超过180秒未访问的域名从队列中移除

### 3. 双阈值触发
- **固定阈值**: 剩余TTL < 5秒时触发（处理长TTL）
- **百分比阈值**: 剩余TTL < 原始TTL * 80%时触发（处理短TTL）
- 取两者中较小值

## 配置参数

### 基础配置
```yaml
# 启用预获取功能（默认: false，保持向后兼容）
cache-prefetch-enabled: true
```

### 触发条件配置
```yaml
# 固定阈值（秒）
cache-prefetch-threshold-seconds: 5

# 百分比阈值（0-100）
cache-prefetch-threshold-percent: 80
```

### 并发控制
```yaml
# 最大并发预获取任务数
cache-prefetch-max-concurrent: 10

# 扫描间隔
cache-prefetch-scan-interval: 1s
```

### 热度管理
```yaml
# 最小热度阈值（冷启动阶段需达到的访问次数）
cache-prefetch-min-heat-threshold: 6

# 时间窗口（用于冷启动判断和队列剔除）
cache-prefetch-time-window: 180s

# 不活跃检查间隔
cache-prefetch-inactivity-check-interval: 10s
```

## 工作流程

### 冷启动阶段
```
T=0     首次访问google.com
        FirstAccessTime = 2024-01-01 10:00:00
        AccessCount = 1

T=30    第2次访问，AccessCount = 2
T=60    第3次访问，AccessCount = 3
T=90    第4次访问，AccessCount = 4
T=120   第5次访问，AccessCount = 5

T=150   第6次访问，AccessCount = 6
        判断：now - FirstAccessTime = 150秒 < 180秒 ✓
        且 AccessCount >= 6 ✓
        → 加入主动刷新队列
        InPrefetchQueue = true
```

### 主动刷新阶段
```
T=295   剩余TTL = 300 - 295 = 5秒
        触发预获取

T=296   预获取完成，新TTL=305
        T重置为0
        ExpiresAt = 305
        LastAccessTime更新（预获取算活动）
```

### 队列剔除
```
如果超过180秒未访问（包括预获取）：
        → 从队列中剔除
        InPrefetchQueue = false
        重置冷启动状态
```

## 向后兼容性

### 默认行为
- 预获取功能默认**禁用**
- 禁用时完全回退到原有的被动缓存过期机制
- 保持与原版dnsproxy的完全兼容

### 配置兼容
- 支持原有的缓存配置参数：
  - `cache-size`
  - `cache-min-ttl`
  - `cache-max-ttl`
- 新增的预获取参数为可选配置

## 实现状态

### 已完成
- [x] 配置结构定义 (`prefetch_config.go`)
- [x] 全局时钟系统 (`global_clock.go`)
- [x] 扩展缓存条目 (`cache_entry_ext.go`)
- [x] 热度跟踪器 (`heat_tracker.go`)
- [x] 预获取调度器 (`prefetch_scheduler.go`)
- [x] 配置集成 (`config.go`)
- [x] 编译验证

### 待完成
- [ ] 与现有缓存系统集成
- [ ] 实现实际的DNS查询逻辑
- [ ] 预获取成功/失败处理
- [ ] 统计指标收集
- [ ] 单元测试
- [ ] 集成测试
- [ ] 性能测试

## 使用示例

### 启动dnsproxy with prefetch
```bash
./dnsproxy-prefetch.exe -c test-prefetch-config.yaml
```

### 命令行参数（待实现）
```bash
./dnsproxy-prefetch.exe \
  --cache-size=10000 \
  --cache-prefetch-enabled=true \
  --cache-prefetch-threshold=5 \
  --cache-prefetch-percent=80
```

## 性能考虑

### 内存开销
- 每个缓存条目增加约64字节（热度跟踪数据）
- 预获取队列使用map存储，内存占用与热门域名数量成正比

### CPU开销
- 扫描循环每秒执行一次
- 不活跃检查每10秒执行一次
- 后台查询使用goroutine池，限制并发数

### 网络开销
- 只对热门域名进行预获取
- 预获取频率受TTL和阈值控制
- 失败的预获取不会重试

## 调试

### 日志级别
```yaml
verbose: true
```

### 关键日志
- `prefetch scheduler started`: 调度器启动
- `prefetch triggered`: 触发预获取
- `prefetch completed`: 预获取完成
- `removed inactive domains`: 剔除不活跃域名

## 下一步

1. 集成到现有缓存系统
2. 实现完整的DNS查询逻辑
3. 添加统计和监控
4. 编写测试用例
5. 性能优化和调优
