# AdGuardHome 主动缓存刷新集成指南

## 概述

本文档说明如何在 AdGuardHome 中启用和配置主动缓存刷新功能。

## 前置条件

- AdGuardHome 使用 dnsproxy v0.79.0 或更高版本
- 已启用缓存功能

## 配置参数

### 1. 核心配置

在 AdGuardHome 的配置文件中（通常是 `AdGuardHome.yaml`），添加以下 DNS 配置：

```yaml
dns:
  # 启用缓存
  cache_size: 67108864  # 64MB，单位：字节
  cache_ttl_min: 0      # 最小 TTL（秒）
  cache_ttl_max: 0      # 最大 TTL（秒），0 表示不限制
  cache_optimistic: true  # 启用乐观缓存
  
  # 主动缓存刷新配置（新增）
  cache_proactive_refresh_time: 30000  # 刷新提前时间（毫秒），默认 30000
  cache_proactive_cooldown_period: 1800  # 冷却周期（秒），默认 1800（30分钟）
  cache_proactive_cooldown_threshold: 3  # 冷却阈值（请求次数），默认 3
```

### 2. 配置参数详解

#### cache_proactive_refresh_time（毫秒）

**说明**：在 TTL 到期前多久开始主动刷新缓存

**默认值**：30000（30 秒）

**取值范围**：
- 最小值：500（0.5 秒）- 适用于极短 TTL
- 推荐值：10000-60000（10-60 秒）
- 设置为 0 或负数：禁用主动刷新

**示例**：
```yaml
# 激进刷新（高实时性）
cache_proactive_refresh_time: 5000  # 5 秒

# 平衡模式（推荐）
cache_proactive_refresh_time: 30000  # 30 秒

# 保守刷新
cache_proactive_refresh_time: 60000  # 60 秒
```

#### cache_proactive_cooldown_period（秒）

**说明**：统计请求频率的时间窗口

**默认值**：1800（30 分钟）

**取值范围**：
- 最小值：60（1 分钟）
- 推荐值：1800-3600（30-60 分钟）
- 最大值：无限制

**作用**：
- 只统计此时间窗口内的请求
- 超出窗口的请求自动被忽略
- 控制何时停止刷新冷门域名

**示例**：
```yaml
# 快速冷却（节省资源）
cache_proactive_cooldown_period: 300  # 5 分钟

# 标准冷却（推荐）
cache_proactive_cooldown_period: 1800  # 30 分钟

# 长期保持（高价值域名）
cache_proactive_cooldown_period: 7200  # 2 小时
```

#### cache_proactive_cooldown_threshold（次数）

**说明**：触发主动刷新所需的最小请求次数

**默认值**：3

**取值范围**：
- -1：禁用冷却，刷新所有域名
- 0：使用默认值 3
- 1-N：自定义阈值

**作用**：
- 过滤低频域名，只刷新热门域名
- 值越大，刷新的域名越少，越节省资源
- 值越小，刷新的域名越多，缓存越新鲜

**示例**：
```yaml
# 禁用冷却（刷新所有）
cache_proactive_cooldown_threshold: -1

# 低阈值（刷新大部分域名）
cache_proactive_cooldown_threshold: 2

# 标准阈值（推荐）
cache_proactive_cooldown_threshold: 3

# 高阈值（只刷新超热门域名）
cache_proactive_cooldown_threshold: 10
```

## 配置场景

### 场景 1：家庭用户（推荐）

**特点**：平衡性能和资源消耗

```yaml
dns:
  cache_size: 67108864
  cache_optimistic: true
  cache_proactive_refresh_time: 30000
  cache_proactive_cooldown_period: 1800
  cache_proactive_cooldown_threshold: 3
```

**效果**：
- 常访问的网站（如 Google、YouTube）保持缓存新鲜
- 偶尔访问的网站不会浪费资源刷新
- 30 分钟无访问后自动停止刷新

### 场景 2：企业/高负载环境

**特点**：优先保证缓存新鲜度

```yaml
dns:
  cache_size: 134217728  # 128MB
  cache_optimistic: true
  cache_proactive_refresh_time: 10000
  cache_proactive_cooldown_period: 3600
  cache_proactive_cooldown_threshold: 5
```

**效果**：
- 更激进的刷新策略（10 秒前刷新）
- 更长的冷却周期（1 小时）
- 更高的阈值（5 次），只刷新真正的热门域名

### 场景 3：资源受限设备（如路由器）

**特点**：最小化资源消耗

```yaml
dns:
  cache_size: 33554432  # 32MB
  cache_optimistic: true
  cache_proactive_refresh_time: 60000
  cache_proactive_cooldown_period: 900
  cache_proactive_cooldown_threshold: 10
```

**效果**：
- 保守的刷新策略（60 秒前刷新）
- 短冷却周期（15 分钟），快速停止刷新
- 高阈值（10 次），只刷新超热门域名

### 场景 4：API 服务器/关键业务

**特点**：最大化缓存新鲜度

```yaml
dns:
  cache_size: 268435456  # 256MB
  cache_optimistic: true
  cache_proactive_refresh_time: 5000
  cache_proactive_cooldown_period: 7200
  cache_proactive_cooldown_threshold: 1
```

**效果**：
- 极激进的刷新（5 秒前刷新）
- 长冷却周期（2 小时）
- 低阈值（1 次），几乎所有域名都刷新

### 场景 5：禁用主动刷新

**特点**：使用传统的被动刷新

```yaml
dns:
  cache_size: 67108864
  cache_optimistic: true
  cache_proactive_refresh_time: 0  # 禁用
  # 或者不设置这些参数
```

## 编程接口（Go API）

如果你在代码中直接使用 dnsproxy，可以这样配置：

### 基础配置

```go
package main

import (
    "github.com/AdguardTeam/dnsproxy/proxy"
    "github.com/AdguardTeam/dnsproxy/upstream"
)

func main() {
    // 创建上游配置
    upstreamConfig, _ := proxy.ParseUpstreamsConfig(
        []string{"8.8.8.8:53", "1.1.1.1:53"},
        &upstream.Options{},
    )

    // 创建代理配置
    dnsProxy, err := proxy.New(&proxy.Config{
        // 基础配置
        UDPListenAddr: []*net.UDPAddr{
            {IP: net.ParseIP("0.0.0.0"), Port: 53},
        },
        
        // 缓存配置
        CacheEnabled:    true,
        CacheSizeBytes:  64 * 1024 * 1024,  // 64MB
        CacheOptimistic: true,
        
        // 主动刷新配置
        CacheProactiveRefreshTime:       30000,  // 30秒（毫秒）
        CacheProactiveCooldownPeriod:    1800,   // 30分钟（秒）
        CacheProactiveCooldownThreshold: 3,      // 3次请求
        
        // 上游服务器
        UpstreamConfig: upstreamConfig,
    })
    
    if err != nil {
        panic(err)
    }
    
    // 启动代理
    err = dnsProxy.Start(context.Background())
    if err != nil {
        panic(err)
    }
    defer dnsProxy.Shutdown(context.Background())
    
    // 保持运行
    select {}
}
```

### 动态配置

```go
// 在运行时无法动态修改配置
// 需要重启代理才能应用新配置

// 1. 关闭现有代理
dnsProxy.Shutdown(context.Background())

// 2. 创建新配置
newProxy, err := proxy.New(&proxy.Config{
    // 新的配置参数
    CacheProactiveRefreshTime: 60000,  // 修改为 60 秒
    // ... 其他配置
})

// 3. 启动新代理
newProxy.Start(context.Background())
```

## 监控和调试

### 日志输出

启用主动刷新后，你会在日志中看到类似信息：

```
[INFO] cache enabled size=67108864 proactive_refresh_ms=30000 cooldown_period_sec=1800 cooldown_threshold=3
[DEBUG] proactively refreshed cache entry domain=example.com.
[DEBUG] dynamically scheduled proactive refresh after reaching threshold domain=trending.com. delay=25s
[DEBUG] skipping proactive refresh due to low request frequency domain=rare-site.com.
```

### 性能指标

主动刷新的性能开销：

- **内存开销**：每个缓存条目约 24-48 字节（请求统计）+ 200 字节（定时器）
- **CPU 开销**：~810 ns/op（基准测试结果）
- **网络开销**：取决于刷新的域名数量和 TTL

### 调试建议

1. **查看刷新日志**：
   ```yaml
   log:
     verbose: true  # 启用详细日志
   ```

2. **监控上游请求数**：
   - 观察上游 DNS 服务器的请求量
   - 如果请求量过大，提高冷却阈值

3. **调整参数**：
   - 从保守配置开始
   - 逐步调整到最佳状态

## 常见问题

### Q1: 主动刷新会增加多少上游请求？

**A**: 取决于配置：
- 冷却阈值 = 3，约增加 10-30% 的请求（只刷新热门域名）
- 冷却阈值 = -1（禁用），约增加 100-200% 的请求（刷新所有域名）

### Q2: 如何知道主动刷新是否生效？

**A**: 查看日志：
```
[DEBUG] proactively refreshed cache entry domain=example.com.
```
如果看到这条日志，说明主动刷新正在工作。

### Q3: 主动刷新会一直循环吗？

**A**: 是的，这是设计特性：
- 热门域名会持续循环刷新，保持缓存新鲜
- 当用户停止访问后，统计过期，自动停止刷新

### Q4: 如何完全禁用主动刷新？

**A**: 三种方法：
1. 设置 `cache_proactive_refresh_time: 0`
2. 设置 `cache_optimistic: false`
3. 不设置这些参数（使用默认行为）

### Q5: 配置错误会怎样？

**A**: 系统会使用默认值：
- 无效的刷新时间 → 使用 30000（30 秒）
- 无效的冷却周期 → 使用 1800（30 分钟）
- 无效的阈值 → 使用 3

## 版本兼容性

| dnsproxy 版本 | 主动刷新支持 | 说明 |
|--------------|------------|------|
| < v0.79.0 | ❌ 不支持 | 需要升级 |
| >= v0.79.0 | ✅ 完全支持 | 推荐版本 |

## 相关文档

- [功能说明](PROACTIVE_CACHE_REFRESH.md) - 详细的功能介绍
- [实现总结](IMPLEMENTATION_SUMMARY.md) - 技术实现细节
- [循环刷新设计](CONTINUOUS_REFRESH_DESIGN.md) - 循环刷新机制说明

## 技术支持

如有问题，请：
1. 查看日志输出
2. 参考本文档的常见问题
3. 提交 Issue 到 GitHub 仓库

## 更新日志

### v0.79.0 (2024-12-03)
- ✅ 首次发布主动缓存刷新功能
- ✅ 支持毫秒级刷新时间控制
- ✅ 支持智能冷却机制
- ✅ 支持动态调度
- ✅ 支持循环刷新
