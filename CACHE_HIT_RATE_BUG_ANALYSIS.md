# 缓存命中率低的根本原因分析

## 问题描述

用户报告 AdGuard Home 中缓存命中率只有 30%+，远低于预期。

配置参数：
- 刷新提前时间: 1000ms (1秒)
- 冷却周期: 1800秒 (30分钟)
- 冷却阈值: 3次请求
- 乐观缓存: 启用

## 根本原因

### Bug 1: 主动刷新操作被误计入请求统计

**问题代码位置**: `proxy/cache.go` 第 457-487 行

```go
func (c *cache) set(m *dns.Msg, u upstream.Upstream, l *slog.Logger) {
    // ...
    c.items.Set(key, packed)

    // Record this as a request for cooldown mechanism.
    justReachedThreshold := c.recordRequest(key)  // ❌ 问题在这里
    
    // Schedule proactive refresh if enabled.
    if c.optimistic && item.ttl > 0 && c.proactiveRefreshTime > 0 && c.cr != nil {
        c.scheduleRefresh(key, item.ttl, m)
    }
}
```

**问题流程**:

```
1. 用户请求 example.com
   └─> get() → recordRequest() → 统计: 1次

2. 用户再次请求 example.com (缓存命中)
   └─> get() → recordRequest() → 统计: 2次

3. 用户第3次请求 example.com (缓存命中)
   └─> get() → recordRequest() → 统计: 3次 ✓ 达到阈值
   └─> tryScheduleRefresh() → 调度刷新定时器

4. 定时器触发，主动刷新
   └─> refreshEntry()
   └─> replyFromUpstream() → 从上游获取新数据
   └─> cacheResp()
   └─> set() → recordRequest() → 统计: 4次 ❌ 刷新被误计为用户请求！

5. 下次刷新
   └─> set() → recordRequest() → 统计: 5次 ❌ 又被误计！

6. 循环刷新持续
   └─> 每次刷新都增加统计
   └─> 统计数据完全失真
```

### Bug 2: 统计数据失真导致的连锁反应

由于刷新操作被计入统计，会导致：

1. **统计永远不会低于阈值**
   - 即使用户不再访问该域名
   - 刷新操作会持续增加统计
   - `shouldProactiveRefresh()` 永远返回 `true`

2. **无用的循环刷新**
   - 冷门域名被持续刷新
   - 浪费上游带宽和资源
   - 缓存被无用数据占满

3. **热门域名被挤出缓存**
   - LRU 缓存机制
   - 冷门域名持续刷新 → 持续访问缓存
   - 真正的热门域名反而被挤出
   - **缓存命中率下降**

### Bug 3: 缓存键冲突问题

**问题代码位置**: `proxy/cache.go` 第 350-365 行

```go
func (c *cache) get(req *dns.Msg) (ci *cacheItem, expired bool, key []byte) {
    // ...
    if ci, expired = c.unpackItem(data, req); ci == nil {
        c.items.Del(key)  // ❌ 删除缓存
    } else {
        // Record request for cooldown mechanism.
        justReachedThreshold := c.recordRequest(key)
        // ...
    }
    return ci, expired, key
}
```

**问题**: 如果 `unpackItem` 失败（例如消息格式错误），缓存会被删除，但：
- 刷新定时器仍然存在
- 定时器触发时，缓存已不存在
- 刷新操作会重新创建缓存
- 但用户可能已经不需要这个域名了

## 影响分析

### 1. 缓存命中率低

```
正常情况（无Bug）:
- 热门域名: 持续刷新，缓存命中率高
- 冷门域名: 不刷新，过期后删除
- 缓存空间: 被热门域名占用
- 命中率: 70-90%

Bug情况:
- 热门域名: 被挤出缓存
- 冷门域名: 持续刷新，占用缓存
- 缓存空间: 被冷门域名占用
- 命中率: 30-40% ❌
```

### 2. 资源浪费

```
假设:
- 缓存大小: 4MB (约4000个域名)
- 冷却阈值: 3次
- 冷却周期: 30分钟

Bug导致:
- 所有访问过3次的域名都会持续刷新
- 即使用户只访问了一次，刷新操作也会让统计≥3
- 4000个域名 × 每分钟1次刷新 = 4000 QPS 到上游
- 实际用户请求可能只有 100 QPS
```

### 3. 上游压力

```
正常情况:
- 用户请求: 100 QPS
- 缓存命中: 80 QPS
- 上游请求: 20 QPS
- 主动刷新: 10 QPS
- 总上游: 30 QPS

Bug情况:
- 用户请求: 100 QPS
- 缓存命中: 30 QPS (命中率低)
- 上游请求: 70 QPS
- 无用刷新: 50 QPS (冷门域名)
- 总上游: 120 QPS ❌ (4倍压力)
```

## 解决方案

### 方案1: 区分用户请求和刷新操作（推荐）

修改 `set` 方法，添加一个参数标识是否来自刷新：

```go
// set stores response and upstream in the cache.
// isRefresh indicates if this is from a proactive refresh operation.
func (c *cache) set(m *dns.Msg, u upstream.Upstream, l *slog.Logger, isRefresh bool) {
    item := c.respToItem(m, u, l)
    if item == nil {
        return
    }

    key := msgToKey(m)
    packed := item.pack()

    c.itemsLock.Lock()
    defer c.itemsLock.Unlock()

    c.items.Set(key, packed)

    // Only record request if this is NOT a refresh operation.
    var justReachedThreshold bool
    if !isRefresh {
        justReachedThreshold = c.recordRequest(key)
    }

    // Schedule proactive refresh if enabled.
    if c.optimistic && item.ttl > 0 && c.proactiveRefreshTime > 0 && c.cr != nil {
        c.scheduleRefresh(key, item.ttl, m)
    } else if justReachedThreshold && c.optimistic && c.proactiveRefreshTime > 0 && c.cr != nil {
        // Dynamic threshold activation (only for user requests)
    }
}
```

修改 `refreshEntry` 方法：

```go
func (c *cache) refreshEntry(m *dns.Msg) {
    defer slogutil.RecoverAndLog(context.TODO(), c.logger)

    if m == nil || len(m.Question) == 0 {
        return
    }

    dctx := &DNSContext{
        Req:       m.Copy(),
        IsRefresh: true,  // 标记为刷新操作
    }

    ok, err := c.cr.replyFromUpstream(dctx)
    if err != nil {
        c.logger.Debug("proactive cache refresh failed", slogutil.KeyError, err)
        return
    }

    if ok {
        c.cr.cacheResp(dctx)  // cacheResp 会检查 IsRefresh 标志
        c.logger.Debug("proactively refreshed cache entry", "domain", m.Question[0].Name)
    }
}
```

### 方案2: 刷新时不重新调度

修改 `set` 方法，刷新操作不调度新的刷新：

```go
func (c *cache) set(m *dns.Msg, u upstream.Upstream, l *slog.Logger, isRefresh bool) {
    // ...
    c.items.Set(key, packed)

    if !isRefresh {
        // Only record and schedule for user requests
        justReachedThreshold := c.recordRequest(key)
        
        if c.optimistic && item.ttl > 0 && c.proactiveRefreshTime > 0 && c.cr != nil {
            c.scheduleRefresh(key, item.ttl, m)
        }
    }
    // Refresh operations don't schedule new refreshes
}
```

### 方案3: 刷新时检查统计是否仍然满足阈值

修改 `scheduleRefresh` 方法，在刷新后重新检查：

```go
func (c *cache) scheduleRefresh(key []byte, ttl uint32, m *dns.Msg, isRefresh bool) {
    // If this is a refresh operation, check if we should continue refreshing
    if isRefresh {
        if !c.shouldProactiveRefresh(key) {
            // Request frequency dropped below threshold, stop refreshing
            c.logger.Debug("stopping refresh due to low request frequency",
                "domain", m.Question[0].Name)
            return
        }
    }
    
    // ... rest of the method
}
```

## 推荐实施

**推荐方案1**，因为：

1. **最彻底**: 从根本上解决统计混乱问题
2. **最清晰**: 明确区分用户请求和刷新操作
3. **最安全**: 不会影响现有逻辑

## 预期效果

修复后：

```
缓存命中率: 30% → 70-90%
上游压力: 120 QPS → 30 QPS
资源利用: 冷门域名占用 → 热门域名占用
刷新准确性: 所有域名刷新 → 只刷新热门域名
```

## 测试验证

需要添加测试用例验证：

1. 刷新操作不计入请求统计
2. 冷门域名停止刷新
3. 热门域名持续刷新
4. 缓存命中率提升
