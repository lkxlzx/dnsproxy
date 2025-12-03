# IPv6 (AAAA记录) 缓存和主动刷新测试报告

## 测试目的

验证主动缓存刷新机制对 IPv6 (AAAA记录) 的支持是否正确，包括：
1. AAAA 记录的缓存功能
2. AAAA 记录的主动刷新机制
3. AAAA 记录的循环刷新
4. A 和 AAAA 记录分别独立缓存和刷新

## 测试环境

- **测试文件**: `proxy/cache_ipv6_test.go`
- **TTL**: 3秒
- **主动刷新时间**: 500ms (TTL - 500ms)
- **冷却机制**: 禁用 (threshold = -1)

## 测试1: IPv6 缓存和主动刷新 (TestIPv6CacheAndRefresh)

### 测试流程

```
阶段1: 首次查询 IPv6 地址
  └─> 查询 ipv6.google.com AAAA
  └─> 返回: 2001:4860:4860::8888
  └─> 上游调用: 1次 ✓

阶段2: 立即再次查询（缓存命中）
  └─> 查询 ipv6.google.com AAAA
  └─> 上游调用: 仍为1次 ✓

阶段3: 等待主动刷新 (2.7秒)
  └─> 等待 TTL - 500ms = 2.5秒
  └─> 上游调用: 2次 ✓ (发生了主动刷新)

阶段4: 验证刷新后的缓存
  └─> 查询 ipv6.google.com AAAA
  └─> 返回刷新后的记录 ✓

阶段5: 验证循环刷新 (再等2.7秒)
  └─> 上游调用: 3次 ✓ (发生了循环刷新)
```

### 测试结果

```
=== RUN   TestIPv6CacheAndRefresh
    cache_ipv6_test.go:76: === 阶段1: 首次查询 IPv6 地址 ===
    cache_ipv6_test.go:92: 首次查询结果: 2001:4860:4860::8888
    cache_ipv6_test.go:98: === 阶段2: 立即再次查询（应该命中缓存）===
    cache_ipv6_test.go:111: === 阶段3: 等待主动刷新执行 ===
    cache_ipv6_test.go:117: 主动刷新后，上游调用次数: 2
    cache_ipv6_test.go:119: === 阶段4: 验证刷新后的缓存 ===
    cache_ipv6_test.go:132: === 阶段5: 验证循环刷新 ===
    cache_ipv6_test.go:138: 循环刷新后，上游调用次数: 3
    cache_ipv6_test.go:140: === 测试总结 ===
    cache_ipv6_test.go:141: ✓ IPv6 (AAAA) 记录缓存正常
    cache_ipv6_test.go:142: ✓ 主动刷新机制工作正常
    cache_ipv6_test.go:143: ✓ 循环刷新机制工作正常
    cache_ipv6_test.go:144: ✓ 总上游调用次数: 3
--- PASS: TestIPv6CacheAndRefresh (5.40s)
```

**结论**: ✅ **通过** - IPv6 记录的缓存、主动刷新和循环刷新全部正常工作

## 测试2: A 和 AAAA 记录分别缓存 (TestIPv6AndIPv4Separate)

### 测试目的

验证同一域名的 A 记录和 AAAA 记录是否：
- 分别独立缓存
- 分别独立刷新
- 互不干扰

### 测试流程

```
1. 查询 dual.google.com A 记录
   └─> A记录上游调用: 1次
   └─> AAAA记录上游调用: 0次 ✓

2. 查询 dual.google.com AAAA 记录
   └─> A记录上游调用: 仍为1次 ✓
   └─> AAAA记录上游调用: 1次 ✓

3. 再次查询 A 记录（缓存命中）
   └─> A记录上游调用: 仍为1次 ✓

4. 再次查询 AAAA 记录（缓存命中）
   └─> AAAA记录上游调用: 仍为1次 ✓

5. 等待主动刷新 (2.7秒)
   └─> A记录上游调用: 2次 ✓
   └─> AAAA记录上游调用: 2次 ✓
```

### 测试结果

```
=== RUN   TestIPv6AndIPv4Separate
    cache_ipv6_test.go:219: === 测试 A 和 AAAA 记录分别缓存 ===
    cache_ipv6_test.go:269: === 等待主动刷新 ===
    cache_ipv6_test.go:276: === 测试总结 ===
    cache_ipv6_test.go:277: ✓ A记录上游调用: 2次
    cache_ipv6_test.go:278: ✓ AAAA记录上游调用: 2次
    cache_ipv6_test.go:279: ✓ A和AAAA记录分别缓存和刷新
--- PASS: TestIPv6AndIPv4Separate (2.70s)
```

**结论**: ✅ **通过** - A 和 AAAA 记录完全独立缓存和刷新，互不干扰

## 关键发现

### 1. 缓存键的正确性

缓存系统正确地为不同查询类型创建了独立的缓存键：
- `dual.google.com. A` → 独立缓存项
- `dual.google.com. AAAA` → 独立缓存项

这意味着缓存键包含了查询类型 (Qtype)，确保了不同类型的查询不会互相覆盖。

### 2. 主动刷新的独立性

每个缓存项都有自己的：
- 刷新定时器
- 请求统计
- 冷却状态

这确保了：
- A 记录的刷新不会触发 AAAA 记录的刷新
- AAAA 记录的刷新不会触发 A 记录的刷新
- 两者可以有不同的刷新频率（如果 TTL 不同）

### 3. IPv6 地址处理

测试使用了真实的 IPv6 地址格式：
- `2001:4860:4860::8888` (Google Public DNS IPv6)
- 正确解析和显示
- 缓存和刷新机制完全兼容

## 总体结论

✅ **所有测试通过**

主动缓存刷新机制对 IPv6 (AAAA记录) 的支持完全正确：

1. ✅ AAAA 记录可以正常缓存
2. ✅ AAAA 记录可以主动刷新
3. ✅ AAAA 记录可以循环刷新
4. ✅ A 和 AAAA 记录完全独立，互不干扰
5. ✅ 缓存键正确包含查询类型
6. ✅ 每个记录类型有独立的刷新机制

## 实际应用场景

这个功能对于双栈 (Dual Stack) 网络环境特别重要：

```yaml
# 用户配置
dns:
  cache_enabled: true
  cache_optimistic: true
  cache_proactive_refresh_time: 5000  # 5秒
  upstream_dns:
    - https://dns.google/dns-query
```

当客户端查询 `www.google.com` 时：
- 首次查询 A 记录 → 缓存 + 调度刷新
- 首次查询 AAAA 记录 → 独立缓存 + 独立调度刷新
- 两者互不干扰，各自保持新鲜

这确保了 IPv4 和 IPv6 双栈环境下的最佳性能。
