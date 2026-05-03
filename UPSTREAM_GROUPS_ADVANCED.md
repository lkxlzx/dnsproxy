# Upstream Groups 高级功能文档

## 概述

本文档介绍 Upstream Groups 的所有高级功能，包括健康检查、统计信息、动态重载和 HTTP API 管理接口。

## 目录

- [健康检查](#健康检查)
- [统计信息](#统计信息)
- [动态重载](#动态重载)
- [HTTP API](#http-api)
- [权重负载均衡](#权重负载均衡)
- [完整示例](#完整示例)

---

## 健康检查

### 功能说明

健康检查功能自动监控所有上游服务器的健康状态，自动排除不健康的服务器，确保查询始终发送到可用的服务器。

### 配置

```yaml
health-check:
  enabled: true
  interval: "30s"           # 检查间隔
  timeout: "5s"             # 检查超时
  failure_threshold: 3      # 失败阈值
  success_threshold: 2      # 成功阈值
  test_domain: "dns.google." # 测试域名
```

### 参数说明

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | false | 是否启用健康检查 |
| `interval` | duration | 30s | 检查间隔时间 |
| `timeout` | duration | 5s | 单次检查超时 |
| `failure_threshold` | int | 3 | 连续失败多少次标记为不健康 |
| `success_threshold` | int | 2 | 连续成功多少次标记为健康 |
| `test_domain` | string | dns.google. | 用于健康检查的测试域名 |

### 工作原理

1. **定期检查**：按配置的间隔定期向每个上游服务器发送 DNS 查询
2. **状态跟踪**：记录每次检查的成功/失败状态
3. **阈值判断**：根据连续失败/成功次数判断健康状态
4. **自动排除**：不健康的服务器自动从查询列表中排除
5. **自动恢复**：服务器恢复后自动重新加入

### 健康统计

每个上游服务器维护以下统计信息：

- 健康状态（健康/不健康）
- 最后检查时间
- 最后成功时间
- 最后失败时间
- 连续失败次数
- 连续成功次数
- 总检查次数
- 总失败次数
- 平均延迟

### API 查询

```bash
# 查询所有上游健康状态
curl http://127.0.0.1:8080/api/v1/health

# 查询特定上游健康状态
curl http://127.0.0.1:8080/api/v1/health/1.1.1.1:53
```

### 示例响应

```json
{
  "upstreams": [
    {
      "Address": "1.1.1.1:53",
      "Healthy": true,
      "LastCheck": "2024-01-01T12:00:00Z",
      "LastSuccess": "2024-01-01T12:00:00Z",
      "ConsecutiveFailures": 0,
      "ConsecutiveSuccesses": 5,
      "TotalChecks": 100,
      "TotalFailures": 2,
      "AverageLatency": 15.5
    }
  ]
}
```

---

## 统计信息

### 功能说明

统计信息功能自动收集每个组和每个上游服务器的查询统计，帮助分析性能和优化配置。

### 配置

```yaml
statistics:
  enabled: true
```

### 收集的统计信息

#### 组级统计

- 总查询数
- 成功查询数
- 失败查询数
- 平均延迟
- 最小延迟
- 最大延迟
- 最后查询时间
- 成功率
- 失败率

#### 上游级统计

- 总查询数
- 成功查询数
- 失败查询数
- 平均延迟
- 最后使用时间

### API 查询

```bash
# 查询所有组统计
curl http://127.0.0.1:8080/api/v1/stats

# 查询特定组统计
curl http://127.0.0.1:8080/api/v1/stats/primary
```

### 示例响应

```json
{
  "primary": {
    "Name": "primary",
    "TotalQueries": 1000,
    "SuccessfulQueries": 980,
    "FailedQueries": 20,
    "AverageLatency": 25.5,
    "MinLatency": 10,
    "MaxLatency": 150,
    "LastQueryTime": "2024-01-01T12:00:00Z",
    "UpstreamStats": {
      "1.1.1.1:53": {
        "Address": "1.1.1.1:53",
        "TotalQueries": 500,
        "SuccessfulQueries": 495,
        "FailedQueries": 5,
        "AverageLatency": 20.3,
        "LastUsed": "2024-01-01T12:00:00Z"
      }
    }
  }
}
```

### 使用场景

1. **性能分析**：识别慢速上游服务器
2. **容量规划**：根据查询量规划资源
3. **故障诊断**：分析失败率找出问题
4. **优化配置**：根据统计数据调整配置

---

## 动态重载

### 功能说明

动态重载功能允许在不重启服务的情况下更新配置，实现零停机时间的配置更新。

### 配置

```yaml
reload:
  enabled: true
  watch_file: "config.yaml"
  check_interval: "30s"
```

### 参数说明

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | false | 是否启用自动重载 |
| `watch_file` | string | - | 监控的配置文件路径 |
| `check_interval` | duration | 30s | 检查文件变化的间隔 |

### 工作原理

1. **文件监控**：定期检查配置文件的修改时间
2. **变化检测**：发现文件变化时触发重载
3. **配置解析**：解析新的配置文件
4. **配置验证**：验证新配置的正确性
5. **平滑切换**：验证通过后切换到新配置
6. **资源清理**：关闭旧配置的资源

### 手动重载

除了自动重载，还可以通过 API 手动触发重载：

```bash
curl -X POST http://127.0.0.1:8080/api/v1/reload
```

### 重载流程

```
文件变化 → 检测变化 → 加载配置 → 验证配置 → 应用配置 → 清理旧配置
```

### 错误处理

- 如果新配置解析失败，保持使用旧配置
- 如果新配置验证失败，保持使用旧配置
- 所有错误都会记录到日志
- 可以通过回调函数处理错误

### 最佳实践

1. **测试配置**：在重载前先验证配置文件
2. **备份配置**：保留旧配置文件的备份
3. **监控日志**：关注重载过程的日志输出
4. **渐进更新**：先在测试环境验证再应用到生产

---

## HTTP API

### 功能说明

HTTP API 提供 RESTful 接口用于管理和监控上游组，支持查询状态、统计信息和触发操作。

### 配置

```yaml
api:
  enabled: true
  listen_addr: "127.0.0.1:8080"
  read_timeout: "10s"
  write_timeout: "10s"
  auth_token: "your-secret-token"
```

### 参数说明

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `enabled` | bool | false | 是否启用 API |
| `listen_addr` | string | 127.0.0.1:8080 | 监听地址 |
| `read_timeout` | duration | 10s | 读取超时 |
| `write_timeout` | duration | 10s | 写入超时 |
| `auth_token` | string | - | 认证令牌（可选） |

### API 端点

#### 1. 服务器状态

```
GET /api/v1/status
```

返回服务器基本状态信息。

**示例**：
```bash
curl http://127.0.0.1:8080/api/v1/status
```

**响应**：
```json
{
  "status": "ok",
  "version": "1.0.0",
  "groups_count": 5,
  "default_group": "primary"
}
```

#### 2. 列出所有组

```
GET /api/v1/groups
```

返回所有上游组的信息。

**示例**：
```bash
curl http://127.0.0.1:8080/api/v1/groups
```

**响应**：
```json
{
  "groups": [
    {
      "name": "primary",
      "upstreams": ["1.1.1.1:53", "8.8.8.8:53"],
      "mode": "load_balance",
      "timeout": "5s",
      "max_retries": 2,
      "enabled": true,
      "priority": 1
    }
  ],
  "default_group": "primary",
  "total": 1
}
```

#### 3. 获取特定组信息

```
GET /api/v1/groups/{name}
```

返回指定组的详细信息。

**示例**：
```bash
curl http://127.0.0.1:8080/api/v1/groups/primary
```

#### 4. 获取所有统计信息

```
GET /api/v1/stats
```

返回所有组的统计信息。

**示例**：
```bash
curl http://127.0.0.1:8080/api/v1/stats
```

#### 5. 获取特定组统计

```
GET /api/v1/stats/{group}
```

返回指定组的统计信息。

**示例**：
```bash
curl http://127.0.0.1:8080/api/v1/stats/primary
```

#### 6. 获取所有健康状态

```
GET /api/v1/health
```

返回所有上游的健康状态。

**示例**：
```bash
curl http://127.0.0.1:8080/api/v1/health
```

#### 7. 获取特定上游健康状态

```
GET /api/v1/health/{address}
```

返回指定上游的健康状态。

**示例**：
```bash
curl http://127.0.0.1:8080/api/v1/health/1.1.1.1:53
```

#### 8. 触发配置重载

```
POST /api/v1/reload
```

手动触发配置重载。

**示例**：
```bash
curl -X POST http://127.0.0.1:8080/api/v1/reload
```

**响应**：
```json
{
  "message": "configuration reloaded successfully"
}
```

### 认证

如果配置了 `auth_token`，所有请求（除了 `/api/v1/status`）都需要提供认证：

```bash
curl -H "Authorization: Bearer your-secret-token" \
     http://127.0.0.1:8080/api/v1/groups
```

### 错误响应

所有错误响应格式统一：

```json
{
  "error": "error message"
}
```

常见 HTTP 状态码：
- `200 OK` - 成功
- `400 Bad Request` - 请求参数错误
- `401 Unauthorized` - 认证失败
- `404 Not Found` - 资源不存在
- `405 Method Not Allowed` - 方法不允许
- `500 Internal Server Error` - 服务器内部错误

---

## 权重负载均衡

### 功能说明

权重负载均衡允许为每个上游服务器分配不同的权重，实现更智能的流量分配。

### 配置（未来功能）

```yaml
upstream-groups:
  groups:
    - name: "weighted"
      mode: "weighted_load_balance"
      upstreams:
        - address: "1.1.1.1:53"
          weight: 3
        - address: "8.8.8.8:53"
          weight: 2
        - address: "9.9.9.9:53"
          weight: 1
```

### 权重分配策略

1. **固定权重**：手动配置每个服务器的权重
2. **动态权重**：根据性能指标自动调整权重
3. **基于延迟**：延迟低的服务器获得更高权重
4. **基于成功率**：成功率高的服务器获得更高权重

### 使用场景

- 服务器性能不同时的负载均衡
- 逐步迁移到新服务器
- A/B 测试不同的 DNS 服务器
- 根据地理位置分配流量

---

## 完整示例

### 生产环境配置

```yaml
# 完整的生产环境配置示例
listen-addrs: ["0.0.0.0"]
listen-ports: [53]
cache: true
cache-min-ttl: 300

upstream-groups:
  default_group: "primary"
  
  groups:
    - name: "primary"
      enabled: true
      mode: "load_balance"
      timeout: "5s"
      max_retries: 2
      priority: 1
      upstreams:
        - "1.1.1.1:53"
        - "8.8.8.8:53"
    
    - name: "backup"
      enabled: true
      mode: "load_balance"
      timeout: "10s"
      max_retries: 5
      priority: 10
      upstreams:
        - "9.9.9.9:53"
        - "149.112.112.112:53"
  
  domain_groups:
    "internal.local": "local"

health-check:
  enabled: true
  interval: "30s"
  timeout: "5s"
  failure_threshold: 3
  success_threshold: 2

statistics:
  enabled: true

reload:
  enabled: true
  watch_file: "config.yaml"
  check_interval: "30s"

api:
  enabled: true
  listen_addr: "127.0.0.1:8080"
  auth_token: "production-secret-token"
```

### 监控脚本

```bash
#!/bin/bash
# 监控脚本示例

API_URL="http://127.0.0.1:8080/api/v1"
TOKEN="your-secret-token"

# 检查服务器状态
check_status() {
    curl -s "$API_URL/status"
}

# 检查健康状态
check_health() {
    curl -s -H "Authorization: Bearer $TOKEN" "$API_URL/health"
}

# 检查统计信息
check_stats() {
    curl -s -H "Authorization: Bearer $TOKEN" "$API_URL/stats"
}

# 触发重载
trigger_reload() {
    curl -s -X POST -H "Authorization: Bearer $TOKEN" "$API_URL/reload"
}

# 主循环
while true; do
    echo "=== $(date) ==="
    
    echo "Status:"
    check_status | jq .
    
    echo "Health:"
    check_health | jq '.upstreams[] | select(.Healthy == false)'
    
    echo "Stats:"
    check_stats | jq 'to_entries[] | {group: .key, queries: .value.TotalQueries, success_rate: (.value.SuccessfulQueries / .value.TotalQueries * 100)}'
    
    sleep 60
done
```

### Python 客户端示例

```python
import requests
import json

class DNSProxyAPI:
    def __init__(self, base_url, token=None):
        self.base_url = base_url
        self.headers = {}
        if token:
            self.headers['Authorization'] = f'Bearer {token}'
    
    def get_status(self):
        return requests.get(f'{self.base_url}/status').json()
    
    def get_groups(self):
        return requests.get(
            f'{self.base_url}/groups',
            headers=self.headers
        ).json()
    
    def get_stats(self, group=None):
        url = f'{self.base_url}/stats'
        if group:
            url += f'/{group}'
        return requests.get(url, headers=self.headers).json()
    
    def get_health(self, address=None):
        url = f'{self.base_url}/health'
        if address:
            url += f'/{address}'
        return requests.get(url, headers=self.headers).json()
    
    def reload(self):
        return requests.post(
            f'{self.base_url}/reload',
            headers=self.headers
        ).json()

# 使用示例
api = DNSProxyAPI('http://127.0.0.1:8080/api/v1', 'your-token')

# 获取状态
status = api.get_status()
print(f"Status: {status}")

# 获取统计
stats = api.get_stats('primary')
print(f"Primary group stats: {stats}")

# 检查健康
health = api.get_health()
unhealthy = [u for u in health['upstreams'] if not u['Healthy']]
if unhealthy:
    print(f"Unhealthy upstreams: {unhealthy}")

# 触发重载
result = api.reload()
print(f"Reload result: {result}")
```

---

## 总结

高级功能提供了：

1. **健康检查** - 自动监控和故障转移
2. **统计信息** - 性能分析和优化
3. **动态重载** - 零停机配置更新
4. **HTTP API** - 便捷的管理接口
5. **权重负载** - 智能流量分配

这些功能使 dnsproxy 成为一个功能完整、易于管理的企业级 DNS 代理解决方案。
