# DNSProxy 域名列表 API 文档

## 概述

提供 RESTful API 用于管理和查询域名列表统计信息，方便前端 UI 集成。

## 基础信息

- **Base URL**: `http://localhost:8080/api`
- **Content-Type**: `application/json`
- **认证**: 暂无（可根据需要添加）

## API 端点

### 1. 获取所有域名列表

获取所有配置的域名列表及其详细信息。

**请求:**
```
GET /api/domain-lists
```

**响应:**
```json
{
  "lists": [
    {
      "name": "china-domains",
      "source": "https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/accelerated-domains.china.conf",
      "group": "china-id",
      "enabled": true,
      "format": "dnsmasq",
      "auto_update": true,
      "refresh_interval": "24h",
      "domain_count": 114898,
      "last_updated": "2026-05-04T11:19:15+08:00"
    },
    {
      "name": "gfwlist",
      "source": "https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt",
      "group": "overseas-id",
      "enabled": true,
      "format": "gfwlist",
      "auto_update": true,
      "refresh_interval": "24h",
      "domain_count": 4161,
      "last_updated": "2026-05-04T11:19:15+08:00"
    }
  ],
  "timestamp": "2026-05-04T11:19:16+08:00"
}
```

**字段说明:**
- `name`: 列表名称
- `source`: 源 URL
- `group`: 所属上游组
- `enabled`: 是否启用
- `format`: 文件格式
- `auto_update`: 是否自动更新
- `refresh_interval`: 刷新间隔
- `domain_count`: 域名数量
- `last_updated`: 最后更新时间

### 2. 获取统计信息

获取所有域名列表的统计摘要。

**请求:**
```
GET /api/domain-lists/stats
```

**响应:**
```json
{
  "lists": [
    {
      "name": "china-domains",
      "domain_count": 114898,
      "last_updated": "2026-05-04T11:19:15+08:00"
    },
    {
      "name": "gfwlist",
      "domain_count": 4161,
      "last_updated": "2026-05-04T11:19:15+08:00"
    }
  ],
  "total_lists": 2,
  "total_domains": 119059,
  "timestamp": "2026-05-04T11:19:16+08:00"
}
```

**字段说明:**
- `lists`: 各列表的统计信息
- `total_lists`: 总列表数
- `total_domains`: 总域名数
- `timestamp`: 响应时间戳

### 3. 刷新所有列表

手动触发所有域名列表的刷新。

**请求:**
```
POST /api/domain-lists/refresh
```

**响应:**
```json
{
  "success": true,
  "message": "All domain lists refreshed successfully",
  "total_lists": 2,
  "total_domains": 119059,
  "timestamp": "2026-05-04T11:20:00+08:00"
}
```

**字段说明:**
- `success`: 是否成功
- `message`: 消息
- `total_lists`: 刷新的列表数
- `total_domains`: 总域名数
- `timestamp`: 刷新时间

### 4. 获取完整配置

获取完整的配置信息。

**请求:**
```
GET /api/config
```

**响应:**
```json
{
  "dns": {
    "bind_hosts": ["127.0.0.1"],
    "port": 5353,
    "upstream_dns": ["223.5.5.5"]
  },
  "upstream_groups": [...],
  "domain_groups": {...},
  "domains_lists": [...],
  "default_group": "default-id",
  "cache": {...}
}
```

### 5. 健康检查

检查服务健康状态。

**请求:**
```
GET /api/health
```

**响应:**
```json
{
  "status": "healthy",
  "groups": 3,
  "domain_mappings": 119039,
  "lists": 2,
  "timestamp": "2026-05-04T11:19:16+08:00"
}
```

**字段说明:**
- `status`: 健康状态
- `groups`: 上游组数量
- `domain_mappings`: 域名映射数量
- `lists`: 域名列表数量
- `timestamp`: 检查时间

## 使用示例

### cURL

```bash
# 获取统计信息
curl http://localhost:8080/api/domain-lists/stats

# 刷新所有列表
curl -X POST http://localhost:8080/api/domain-lists/refresh

# 健康检查
curl http://localhost:8080/api/health
```

### JavaScript (Fetch API)

```javascript
// 获取统计信息
async function getStats() {
  const response = await fetch('http://localhost:8080/api/domain-lists/stats');
  const data = await response.json();
  console.log(`Total domains: ${data.total_domains}`);
  return data;
}

// 刷新列表
async function refreshLists() {
  const response = await fetch('http://localhost:8080/api/domain-lists/refresh', {
    method: 'POST'
  });
  const data = await response.json();
  console.log(data.message);
  return data;
}

// 获取所有列表
async function getDomainLists() {
  const response = await fetch('http://localhost:8080/api/domain-lists');
  const data = await response.json();
  return data.lists;
}
```

### Python

```python
import requests

# 获取统计信息
response = requests.get('http://localhost:8080/api/domain-lists/stats')
stats = response.json()
print(f"Total domains: {stats['total_domains']}")

# 刷新列表
response = requests.post('http://localhost:8080/api/domain-lists/refresh')
result = response.json()
print(result['message'])
```

### Go

```go
package main

import (
    "encoding/json"
    "fmt"
    "net/http"
)

type StatsResponse struct {
    TotalLists   int    `json:"total_lists"`
    TotalDomains int    `json:"total_domains"`
    Timestamp    string `json:"timestamp"`
}

func main() {
    resp, _ := http.Get("http://localhost:8080/api/domain-lists/stats")
    defer resp.Body.Close()
    
    var stats StatsResponse
    json.NewDecoder(resp.Body).Decode(&stats)
    
    fmt.Printf("Total domains: %d\n", stats.TotalDomains)
}
```

## 前端集成示例

### React 组件

```jsx
import React, { useState, useEffect } from 'react';

function DomainListsStats() {
  const [stats, setStats] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchStats();
  }, []);

  const fetchStats = async () => {
    try {
      const response = await fetch('http://localhost:8080/api/domain-lists/stats');
      const data = await response.json();
      setStats(data);
    } catch (error) {
      console.error('Failed to fetch stats:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleRefresh = async () => {
    setLoading(true);
    try {
      await fetch('http://localhost:8080/api/domain-lists/refresh', {
        method: 'POST'
      });
      await fetchStats();
    } catch (error) {
      console.error('Failed to refresh:', error);
    }
  };

  if (loading) return <div>Loading...</div>;

  return (
    <div className="stats-container">
      <h2>域名列表统计</h2>
      <div className="summary">
        <div className="stat-card">
          <h3>总列表数</h3>
          <p>{stats.total_lists}</p>
        </div>
        <div className="stat-card">
          <h3>总域名数</h3>
          <p>{stats.total_domains.toLocaleString()}</p>
        </div>
      </div>
      
      <div className="lists">
        {stats.lists.map(list => (
          <div key={list.name} className="list-card">
            <h4>{list.name}</h4>
            <p>域名数量: {list.domain_count.toLocaleString()}</p>
            <p>最后更新: {new Date(list.last_updated).toLocaleString()}</p>
          </div>
        ))}
      </div>
      
      <button onClick={handleRefresh}>刷新所有列表</button>
    </div>
  );
}

export default DomainListsStats;
```

### Vue 组件

```vue
<template>
  <div class="stats-container">
    <h2>域名列表统计</h2>
    
    <div v-if="loading">加载中...</div>
    
    <div v-else>
      <div class="summary">
        <div class="stat-card">
          <h3>总列表数</h3>
          <p>{{ stats.total_lists }}</p>
        </div>
        <div class="stat-card">
          <h3>总域名数</h3>
          <p>{{ stats.total_domains.toLocaleString() }}</p>
        </div>
      </div>
      
      <div class="lists">
        <div v-for="list in stats.lists" :key="list.name" class="list-card">
          <h4>{{ list.name }}</h4>
          <p>域名数量: {{ list.domain_count.toLocaleString() }}</p>
          <p>最后更新: {{ formatDate(list.last_updated) }}</p>
        </div>
      </div>
      
      <button @click="refreshLists">刷新所有列表</button>
    </div>
  </div>
</template>

<script>
export default {
  data() {
    return {
      stats: null,
      loading: true
    };
  },
  
  mounted() {
    this.fetchStats();
  },
  
  methods: {
    async fetchStats() {
      try {
        const response = await fetch('http://localhost:8080/api/domain-lists/stats');
        this.stats = await response.json();
      } catch (error) {
        console.error('Failed to fetch stats:', error);
      } finally {
        this.loading = false;
      }
    },
    
    async refreshLists() {
      this.loading = true;
      try {
        await fetch('http://localhost:8080/api/domain-lists/refresh', {
          method: 'POST'
        });
        await this.fetchStats();
      } catch (error) {
        console.error('Failed to refresh:', error);
      }
    },
    
    formatDate(dateString) {
      return new Date(dateString).toLocaleString();
    }
  }
};
</script>
```

## 错误处理

### 错误响应格式

```json
{
  "error": "Error message",
  "code": "ERROR_CODE",
  "timestamp": "2026-05-04T11:19:16+08:00"
}
```

### HTTP 状态码

- `200 OK`: 请求成功
- `400 Bad Request`: 请求参数错误
- `404 Not Found`: 资源不存在
- `405 Method Not Allowed`: 方法不允许
- `500 Internal Server Error`: 服务器内部错误

## 性能优化

### 缓存

API 响应可以添加缓存头：

```go
w.Header().Set("Cache-Control", "public, max-age=60")
```

### 压缩

启用 gzip 压缩：

```go
import "github.com/NYTimes/gziphandler"

http.Handle("/api/", gziphandler.GzipHandler(http.HandlerFunc(handler)))
```

### CORS

如果需要跨域访问：

```go
w.Header().Set("Access-Control-Allow-Origin", "*")
w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
```

## 安全建议

1. **认证**: 添加 API Key 或 JWT 认证
2. **限流**: 防止 API 滥用
3. **HTTPS**: 生产环境使用 HTTPS
4. **输入验证**: 验证所有输入参数
5. **日志**: 记录所有 API 请求

## 部署

### 开发环境

```bash
# 启动 API 服务器
go run api_server_example.go
```

### 生产环境

```bash
# 编译
go build -o dnsproxy-api api_server_example.go

# 运行
./dnsproxy-api
```

### Docker

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o dnsproxy-api api_server_example.go

FROM alpine:latest
COPY --from=builder /app/dnsproxy-api /usr/local/bin/
COPY test-cache-config.yaml /etc/dnsproxy/config.yaml
EXPOSE 8080
CMD ["dnsproxy-api"]
```

## 监控

### Prometheus 指标

可以添加 Prometheus 指标：

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    requestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "api_requests_total",
            Help: "Total number of API requests",
        },
        []string{"endpoint", "method"},
    )
)
```

### 健康检查

使用 `/api/health` 端点进行健康检查：

```bash
# Kubernetes liveness probe
livenessProbe:
  httpGet:
    path: /api/health
    port: 8080
  initialDelaySeconds: 30
  periodSeconds: 10
```

## 总结

这个 API 提供了完整的域名列表管理功能，方便前端 UI 集成。主要特性：

- ✅ RESTful 设计
- ✅ JSON 响应格式
- ✅ 实时统计信息
- ✅ 手动刷新功能
- ✅ 健康检查
- ✅ 易于集成

可以根据实际需求扩展更多功能，如单个列表的刷新、列表的启用/禁用等。
