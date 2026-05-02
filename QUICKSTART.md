# DNSProxy Smart Prefetch - 快速启动指南

## 当前状态

✅ **阶段1完成**: 核心基础设施已实现并编译成功

⚠️ **注意**: 当前版本包含预获取功能的基础框架，但尚未完全集成到DNS查询流程中。可以用于：
- 代码审查
- 架构验证
- 单元测试开发
- 进一步开发的基础

## 文件结构

```
dnsproxy/
├── dnsproxy-prefetch.exe           # 编译后的可执行文件（17MB）
├── PREFETCH_FEATURE.md             # 功能详细说明
├── IMPLEMENTATION_STATUS.md        # 实现状态和待办事项
├── QUICKSTART.md                   # 本文件
├── test-prefetch-config.yaml       # 测试配置文件
├── test_prefetch.go                # 测试程序
└── proxy/
    ├── prefetch_config.go          # 配置结构
    ├── global_clock.go             # 全局时钟
    ├── cache_entry_ext.go          # 扩展缓存条目
    ├── heat_tracker.go             # 热度跟踪器
    └── prefetch_scheduler.go       # 预获取调度器
```

## 已实现的组件

### 1. 配置系统 ✅
```go
type PrefetchConfig struct {
    Enabled                 bool          // 启用开关
    ThresholdSeconds        uint32        // 固定阈值（5秒）
    ThresholdPercent        uint32        // 百分比阈值（80%）
    MaxConcurrent           int           // 最大并发数（10）
    ScanInterval            time.Duration // 扫描间隔（1秒）
    MinHeatThreshold        int           // 热度阈值（6次）
    TimeWindow              time.Duration // 时间窗口（180秒）
    InactivityCheckInterval time.Duration // 不活跃检查（10秒）
}
```

### 2. 全局时钟 ✅
- 每秒递增的时钟T
- 用于TTL管理
- 支持重置（预获取后）
- 原子操作保证并发安全

### 3. 热度跟踪 ✅
- 冷启动阶段：180秒内6次访问加入队列
- 队列管理：只对队列中的域名预获取
- 不活跃剔除：超过180秒未访问移除
- 并发安全的map操作

### 4. 预获取调度 ✅
- 定期扫描（每秒）
- 双阈值判断
- 热度排序
- 并发控制
- 不活跃检查（每10秒）

## 编译验证

### 编译命令
```bash
cd dnsproxy
go build -o dnsproxy-prefetch.exe
```

### 编译结果
```
✅ 编译成功
✅ 无错误
✅ 无警告
✅ 生成文件: dnsproxy-prefetch.exe (17,339,904 字节)
```

## 配置示例

### 完整配置 (test-prefetch-config.yaml)
```yaml
# 监听地址
listen-addrs:
  - 127.0.0.1

listen-ports:
  - 53

# 上游DNS服务器
upstream:
  - 8.8.8.8
  - 1.1.1.1

# 基础缓存配置
cache: true
cache-size: 10000
cache-min-ttl: 0
cache-max-ttl: 0

# 智能预获取配置（新增）
cache-prefetch-enabled: true
cache-prefetch-threshold-seconds: 5
cache-prefetch-threshold-percent: 80
cache-prefetch-max-concurrent: 10
cache-prefetch-scan-interval: 1s
cache-prefetch-min-heat-threshold: 6
cache-prefetch-time-window: 180s
cache-prefetch-inactivity-check-interval: 10s

# 日志
verbose: true
```

## 下一步开发

### 优先级1: 缓存系统集成
需要修改 `proxy/cache.go`:
1. 添加globalClock实例
2. 添加heatTracker实例
3. 添加prefetchScheduler实例
4. 修改get()方法记录访问
5. 修改set()方法创建扩展条目
6. 启动调度器

### 优先级2: DNS查询实现
需要修改 `proxy/prefetch_scheduler.go`:
1. 在executePrefetch中实现实际查询
2. 调用proxy的upstream resolver
3. 处理查询结果
4. 更新缓存
5. 重置全局时钟

### 优先级3: 测试
1. 单元测试每个组件
2. 集成测试完整流程
3. 性能基准测试

## 调试建议

### 查看组件结构
```bash
# 查看配置定义
cat proxy/prefetch_config.go

# 查看全局时钟实现
cat proxy/global_clock.go

# 查看热度跟踪器
cat proxy/heat_tracker.go

# 查看调度器
cat proxy/prefetch_scheduler.go
```

### 代码审查要点
1. **并发安全**: 所有共享数据都有锁保护
2. **双时钟系统**: TTL用全局时钟，热度用真实时间
3. **向后兼容**: 默认禁用，不影响现有功能
4. **配置灵活**: 所有参数可配置

## 设计亮点

### 1. 双时钟系统
```
全局时钟T（TTL管理）:
T=0 → T=1 → T=2 → ... → T=295 → [预获取] → T=0（重置）

真实时间（热度管理）:
10:00:00 → 10:00:01 → ... → 10:05:00（不受T重置影响）
```

### 2. 冷启动机制
```
首次访问 → 记录时间
2-5次访问 → 累加计数
第6次访问 → 检查时间窗口 → 加入队列
```

### 3. 双阈值触发
```
固定阈值: 剩余TTL < 5秒
百分比阈值: 剩余TTL < 原始TTL * 80%
取较小值 → 处理各种TTL场景
```

## 性能特性

### 内存占用
- 每个缓存条目增加约64字节
- 预获取队列使用map，O(1)查找
- 热度跟踪数据按需分配

### CPU占用
- 扫描循环：每秒1次
- 不活跃检查：每10秒1次
- 后台查询：goroutine池限制并发

### 网络占用
- 只对热门域名预获取
- 失败不重试
- 并发数可控（默认10）

## 常见问题

### Q: 为什么默认禁用？
A: 保持向后兼容性，用户需要显式启用新功能。

### Q: 如何验证功能是否工作？
A: 当前版本需要完成缓存集成后才能完整测试。可以通过单元测试验证各组件。

### Q: 性能影响如何？
A: 设计目标是内存增长<10%，CPU增长<5%。需要性能测试验证。

### Q: 如何调试？
A: 设置 `verbose: true` 查看详细日志。

## 贡献指南

### 代码风格
- 遵循Go标准代码风格
- 使用gofmt格式化
- 添加必要的注释
- 编写单元测试

### 提交流程
1. 创建功能分支
2. 实现功能
3. 编写测试
4. 提交PR
5. 代码审查

## 参考文档

- [功能详细说明](PREFETCH_FEATURE.md)
- [实现状态](IMPLEMENTATION_STATUS.md)
- [设计文档](../.kiro/specs/dnsproxy-advanced-features/design.md)
- [需求文档](../.kiro/specs/dnsproxy-advanced-features/requirements.md)

## 联系方式

如有问题或建议，请查看规格文档或提交issue。

---

**最后更新**: 2026-05-01
**版本**: v0.1.0-alpha (阶段1完成)
**状态**: 开发中 🚧
