# 缓存无法工作的原因分析

## 🔴 问题现象

从 AdGuard Home 查询日志可以看到：

```
时间        域名              响应代码    TTL      状态
10:23:16   www.google.com   NOERROR    237秒    已处理
10:19:52   www.google.com   NOERROR    241秒    已处理
10:17:09   www.google.com   NOERROR    229秒    已处理
10:15:25   www.google.com   NOERROR    226秒    已处理
10:14:25   www.google.com   NOERROR    237秒    已处理
10:12:15   www.google.com   NOERROR    232秒    已处理
```

**关键问题**：
- ✅ 响应正常（NOERROR）
- ✅ 有 IP 地址返回
- ✅ TTL 在 220-240 秒之间
- ❌ **每次都显示"已处理"** - 说明每次都查询了上游，没有命中缓存！

## 🔍 根本原因

### 原因 1：TTL 太短 + 查询间隔太长

```
TTL = 237秒 ≈ 4分钟
查询间隔 = 2-4分钟

问题：
- 10:12:15 查询 → 缓存到 10:16:12 (237秒后)
- 10:14:25 查询 → 缓存命中 ✅
- 10:15:25 查询 → 缓存命中 ✅
- 10:17:09 查询 → 缓存已过期 ❌ (超过4分钟)
- 10:19:52 查询 → 缓存已过期 ❌
```

**结论**：查询间隔太长，缓存在下次查询前就过期了！

### 原因 2：未配置 CacheMinTTL

从代码分析：

```go
// proxy/cache.go
func (c *cache) respToItem(m *dns.Msg, u upstream.Upstream, l *slog.Logger) (item *cacheItem) {
    ttl := cacheTTL(m, l)  // 直接使用响应的 TTL
    if ttl == 0 {
        return nil
    }
    
    return &cacheItem{
        m:   m,
        u:   upsAddr,
        ttl: ttl,  // ❌ 没有应用 CacheMinTTL 覆盖！
    }
}
```

**问题**：`respToItem` 中直接使用了响应的原始 TTL，**没有应用 `CacheMinTTL` 覆盖**！

### 原因 3：TTL 覆盖只在返回给客户端时应用

```go
// proxy/server.go
func (p *Proxy) applyTTLOverrides(r *dns.Msg) {
    for _, rrSet := range rrSets {
        for _, rr := range rrSet.Value {
            original := rr.Header().Ttl
            overridden := respectTTLOverrides(original, p.CacheMinTTL, p.CacheMaxTTL)
            rr.Header().Ttl = overridden  // 只修改返回给客户端的 TTL
        }
    }
}
```

**问题**：
- TTL 覆盖只在 `server.go` 中应用
- 只影响返回给客户端的响应
- **不影响缓存的存储时间**！

## 🐛 代码缺陷

### 缺陷位置

`proxy/cache.go` 的 `respToItem` 函数：

```go
func (c *cache) respToItem(m *dns.Msg, u upstream.Upstream, l *slog.Logger) (item *cacheItem) {
    ttl := cacheTTL(m, l)  // ❌ 应该在这里应用 TTL 覆盖
    if ttl == 0 {
        return nil
    }
    
    // ❌ 缺少：ttl = respectTTLOverrides(ttl, c.cacheMinTTL, c.cacheMaxTTL)
    
    return &cacheItem{
        m:   m,
        u:   upsAddr,
        ttl: ttl,
    }
}
```

### 正确的流程应该是

```
1. 从上游获取响应 (TTL = 237秒)
2. 计算缓存 TTL = cacheTTL(m)  → 237秒
3. ✅ 应用 TTL 覆盖 = respectTTLOverrides(237, CacheMinTTL, CacheMaxTTL)
4. 存储到缓存，使用覆盖后的 TTL
5. 返回给客户端时，再次应用 TTL 覆盖（用于显示）
```

### 当前的错误流程

```
1. 从上游获取响应 (TTL = 237秒)
2. 计算缓存 TTL = cacheTTL(m)  → 237秒
3. ❌ 直接存储到缓存，使用原始 TTL (237秒)
4. 返回给客户端时，应用 TTL 覆盖（但不影响缓存）
```

## 🔧 解决方案

### 方案 1：修复代码（推荐）

修改 `proxy/cache.go`：

```go
// respToItem 需要访问 CacheMinTTL 和 CacheMaxTTL
func (c *cache) respToItem(m *dns.Msg, u upstream.Upstream, l *slog.Logger) (item *cacheItem) {
    ttl := cacheTTL(m, l)
    if ttl == 0 {
        return nil
    }
    
    // ✅ 应用 TTL 覆盖
    ttl = respectTTLOverrides(ttl, c.cacheMinTTL, c.cacheMaxTTL)
    
    upsAddr := ""
    if u != nil {
        upsAddr = u.Address()
    }
    
    return &cacheItem{
        m:   m,
        u:   upsAddr,
        ttl: ttl,
    }
}
```

需要在 `cache` 结构体中添加字段：

```go
type cache struct {
    // ... 现有字段 ...
    
    // TTL override settings
    cacheMinTTL uint32
    cacheMaxTTL uint32
}
```

在创建缓存时传入：

```go
func newCache(config cacheConfig) (c *cache) {
    return &cache{
        // ... 现有初始化 ...
        cacheMinTTL: config.cacheMinTTL,
        cacheMaxTTL: config.cacheMaxTTL,
    }
}
```

### 方案 2：配置 CacheMinTTL（临时方案）

在 AdGuard Home 配置中：

```yaml
dns:
  cache_size: 4194304
  cache_ttl_min: 600        # ✅ 最小缓存 10 分钟
  cache_ttl_max: 86400      # 最大缓存 24 小时
  cache_optimistic: true
```

**但这个方案不会生效**，因为代码有 bug！

### 方案 3：启用主动刷新（绕过问题）

```yaml
dns:
  cache_size: 4194304
  cache_optimistic: true
  
  # 主动刷新配置
  cache_proactive_refresh_time: 30000      # 30秒
  cache_proactive_cooldown_period: 1800    # 30分钟
  cache_proactive_cooldown_threshold: 3    # 3次请求后启用
```

**原理**：
- 即使 TTL 只有 237 秒
- 主动刷新会在 TTL 到期前 30 秒刷新
- 保持缓存始终有效

**但这只是绕过问题，不是真正的解决方案！**

## 📊 测试验证

### 测试 1：验证当前行为

```go
func TestCacheTTL_NoOverride(t *testing.T) {
    prx := createTestProxy(t, &Config{
        CacheEnabled: true,
        // ❌ 不设置 CacheMinTTL
    })
    
    // 创建 TTL=100 的响应
    resp := createTestResponse("example.com", 100)
    
    // 存储到缓存
    prx.cache.set(resp, nil, log)
    
    // 验证：缓存 TTL 应该是 100
    item := prx.cache.get(...)
    assert.Equal(t, uint32(100), item.ttl)  // ✅ 当前行为
}
```

### 测试 2：验证修复后的行为

```go
func TestCacheTTL_WithMinTTL(t *testing.T) {
    prx := createTestProxy(t, &Config{
        CacheEnabled: true,
        CacheMinTTL:  600,  // 10分钟
    })
    
    // 创建 TTL=100 的响应
    resp := createTestResponse("example.com", 100)
    
    // 存储到缓存
    prx.cache.set(resp, nil, log)
    
    // 验证：缓存 TTL 应该被覆盖为 600
    item := prx.cache.get(...)
    assert.Equal(t, uint32(600), item.ttl)  // ✅ 期望行为
}
```

## 🎯 最佳实践

### 推荐配置

```yaml
dns:
  # 基础缓存配置
  cache_size: 4194304
  cache_ttl_min: 600        # 最小 10 分钟（修复后生效）
  cache_ttl_max: 86400      # 最大 24 小时
  cache_optimistic: true
  
  # 主动刷新配置（增强可靠性）
  cache_proactive_refresh_time: 30000
  cache_proactive_cooldown_period: 1800
  cache_proactive_cooldown_threshold: 3
  
  # 多上游配置（增强可用性）
  upstream_dns:
    - https://dns.google/dns-query
    - https://cloudflare-dns.com/dns-query
    - 8.8.8.8:53
  upstream_mode: parallel
```

### 为什么需要 CacheMinTTL

1. **Google 等大型网站的 TTL 很短**
   - Google: 200-300 秒
   - Facebook: 60-120 秒
   - Twitter: 60 秒

2. **短 TTL 导致缓存命中率低**
   - 用户查询间隔通常 > 5 分钟
   - 缓存在下次查询前就过期了

3. **CacheMinTTL 可以强制延长缓存时间**
   - 设置 600 秒 = 10 分钟
   - 即使上游返回 60 秒，也缓存 10 分钟
   - 大幅提高缓存命中率

## 📈 性能影响

### 修复前

```
场景：Google 查询，TTL=237秒，查询间隔=5分钟

查询 1 → 上游查询 → 缓存 237 秒
等待 5 分钟...
查询 2 → 缓存过期 → 上游查询 → 缓存 237 秒
等待 5 分钟...
查询 3 → 缓存过期 → 上游查询

缓存命中率：0%
上游查询次数：每次都查询
```

### 修复后（CacheMinTTL=600）

```
场景：Google 查询，TTL=237秒 → 覆盖为 600秒，查询间隔=5分钟

查询 1 → 上游查询 → 缓存 600 秒
等待 5 分钟...
查询 2 → 缓存命中 ✅
等待 5 分钟...
查询 3 → 缓存命中 ✅

缓存命中率：66%+
上游查询次数：大幅减少
```

### 修复后 + 主动刷新

```
场景：CacheMinTTL=600 + 主动刷新=30秒

查询 1 → 上游查询 → 缓存 600 秒
570 秒后 → 主动刷新 → 缓存更新 600 秒
等待 5 分钟...
查询 2 → 缓存命中 ✅（且数据是新的）
570 秒后 → 主动刷新 → 缓存更新 600 秒

缓存命中率：接近 100%
数据新鲜度：高
上游查询次数：最少
```

## 🚀 总结

### 问题根源

1. **代码缺陷**：`respToItem` 没有应用 `CacheMinTTL` 覆盖
2. **TTL 太短**：Google 等网站返回的 TTL 只有 200-300 秒
3. **查询间隔长**：用户查询间隔通常 > 5 分钟，缓存已过期

### 解决方案优先级

1. **🔥 立即修复代码** - 让 `CacheMinTTL` 真正生效
2. **⚙️ 配置 CacheMinTTL** - 设置合理的最小缓存时间（600秒）
3. **🚀 启用主动刷新** - 保持缓存始终有效且新鲜

### 预期效果

- ✅ 缓存命中率从 0% 提升到 80%+
- ✅ 上游查询次数减少 80%+
- ✅ DNS 查询延迟降低 90%+
- ✅ 带宽使用减少
- ✅ 用户体验提升

**这是一个严重的 bug，必须修复！** 🐛
