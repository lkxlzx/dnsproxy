# 测试循环 Bug 修复

## 问题描述

在 `cache_google_10min_test.go` 测试中发现了一个循环控制 bug，导致在测试结束时产生大量重复的日志输出。

### 症状

```
09:38 | 185.125.190.98  |  16s |    0.00ms | CACHE    | 23 (IPs: 12)
09:38 | 185.125.190.98  |  16s |    0.00ms | CACHE    | 23 (IPs: 12)
09:38 | 185.125.190.98  |  16s |    0.00ms | CACHE    | 23 (IPs: 12)
... (重复 50+ 次)
```

在 09:38 这一秒内，相同的日志被输出了 50+ 次，而测试应该每 30 秒才输出一次。

## 根本原因

### 原始代码

```go
for elapsed := time.Duration(0); elapsed < 10*time.Minute; elapsed = time.Since(startTime) {
    // ... 执行请求和日志记录 ...
    
    // Wait 30 seconds before next request (unless it's the last iteration)
    if elapsed < 10*time.Minute-30*time.Second {
        time.Sleep(30 * time.Second)
    }
}
```

### 问题分析

1. **循环条件**: `elapsed < 10*time.Minute`
2. **Sleep 条件**: `elapsed < 10*time.Minute-30*time.Second`

当测试运行到第 9 分 30 秒时：
- `elapsed = 9:30` (570 秒)
- Sleep 条件: `570 < 570` → **false**，不 sleep
- 循环继续，`elapsed = time.Since(startTime)` 仍然 < 10 分钟
- 循环立即再次执行，没有延迟
- 在最后 30 秒内，循环会快速重复执行数十次

### 时间线

```
T = 9:00  → 执行请求 → Sleep 30s
T = 9:30  → 执行请求 → 不 Sleep (elapsed >= 9:30)
T = 9:30  → 循环继续 (elapsed < 10:00)
T = 9:30  → 执行请求 → 不 Sleep
T = 9:30  → 循环继续 (elapsed < 10:00)
T = 9:30  → 执行请求 → 不 Sleep
... (快速重复)
T = 10:00 → 循环结束
```

## 修复方案

### 修复后的代码

```go
for time.Since(startTime) < 10*time.Minute {
    // ... 执行请求和日志记录 ...
    
    elapsed := time.Since(startTime)
    
    // Wait 30 seconds before next request (unless we're close to the end)
    if time.Since(startTime) < 10*time.Minute-30*time.Second {
        time.Sleep(30 * time.Second)
    } else {
        // Exit the loop if we're in the last 30 seconds
        break
    }
}
```

### 关键改进

1. **简化循环条件**: 直接使用 `time.Since(startTime) < 10*time.Minute`
2. **明确退出**: 在最后 30 秒内使用 `break` 退出循环
3. **避免重复**: 不再依赖 sleep 条件来控制循环

### 修复后的时间线

```
T = 9:00  → 执行请求 → Sleep 30s
T = 9:30  → 执行请求 → Break (elapsed >= 9:30)
T = 9:30  → 循环结束 ✅
```

## 验证测试

创建了 `TestGoogleCache_LoopFix` 来验证修复：

```go
func TestGoogleCache_LoopFix(t *testing.T) {
    startTime := time.Now()
    requestCount := 0

    // Run for 15 seconds, making requests every 3 seconds
    for time.Since(startTime) < 15*time.Second {
        requestCount++
        
        // ... 执行请求 ...
        
        if time.Since(startTime) < 15*time.Second-3*time.Second {
            time.Sleep(3 * time.Second)
        } else {
            break
        }
    }

    // Should make approximately 5 requests (0s, 3s, 6s, 9s, 12s)
    require.GreaterOrEqual(t, requestCount, 4)
    require.LessOrEqual(t, requestCount, 6)
}
```

### 测试结果

```
=== RUN   TestGoogleCache_LoopFix
    Total requests made: 5
    Expected requests: ~5 (15s / 3s)
    ✅ Loop control working correctly - no excessive iterations
--- PASS: TestGoogleCache_LoopFix (12.00s)
```

## 影响范围

### 受影响的文件

- `proxy/cache_google_10min_test.go` - 主要问题文件

### 其他类似测试

检查了其他测试文件，没有发现类似的循环控制问题：
- `cache_google_longrun_test.go` - 使用不同的循环模式
- `cache_loop_test.go` - 使用固定次数的循环
- `cache_proactive_simple_test.go` - 使用简单的 sleep

## 最佳实践

### 推荐的循环模式

对于基于时间的测试循环：

```go
// ✅ 好的模式
startTime := time.Now()
for time.Since(startTime) < duration {
    // 执行操作
    
    if time.Since(startTime) < duration-interval {
        time.Sleep(interval)
    } else {
        break  // 明确退出
    }
}
```

```go
// ❌ 避免的模式
for elapsed := time.Duration(0); elapsed < duration; elapsed = time.Since(startTime) {
    // 执行操作
    
    if elapsed < duration-interval {
        time.Sleep(interval)
    }
    // 没有明确的退出机制
}
```

### 关键原则

1. **简单的循环条件**: 直接检查时间，不要使用中间变量
2. **明确的退出**: 使用 `break` 而不是依赖循环条件
3. **避免边界问题**: 在接近结束时主动退出
4. **可预测的行为**: 确保循环次数可以计算

## 总结

这个 bug 是一个经典的循环控制边界问题：
- **原因**: 循环条件和 sleep 条件不一致
- **症状**: 在测试结束时产生大量重复输出
- **修复**: 简化循环逻辑，添加明确的退出机制
- **验证**: 创建专门的测试确保修复有效

修复后，测试行为符合预期，不再产生重复的日志输出。
