# 域名列表统计功能说明

## 📊 功能概述

v2.2.3 版本实现了域名列表统计信息的自动收集和更新功能，统计信息会自动保存到配置文件中，方便前端 UI 读取和显示。

---

## 🔧 核心功能

### 1. 自动统计收集

系统会自动收集以下统计信息：
- **domain_count**: 域名数量
- **last_updated**: 最后更新时间 (RFC3339 格式)
- **format**: 检测到的格式 (plain, hosts, dnsmasq, adblock, gfwlist)

### 2. 配置文件自动更新

统计信息会自动写入配置文件，保持与实际缓存同步：

```yaml
domains_lists:
  - name: china-domains
    source: https://example.com/china-list.txt
    group: china-id
    file: ./cache/china-domains.yaml
    enabled: true
    format: dnsmasq
    domain_count: 114898              # 自动更新
    last_updated: "2026-05-04T13:27:38+08:00"  # 自动更新
```

### 3. 前端集成支持

前端可以直接读取配置文件获取统计信息，无需额外 API 调用：

```javascript
// 读取配置文件
const config = await fetch('/config.yaml').then(r => r.text());
const parsed = YAML.parse(config);

// 获取统计信息
parsed.domains_lists.forEach(list => {
  console.log(`${list.name}: ${list.domain_count} 个域名`);
  console.log(`最后更新: ${list.last_updated}`);
});
```

---

## 📝 配置示例

### 基础配置

```yaml
domains_lists:
  - name: china-domains
    source: https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/accelerated-domains.china.conf
    group: china-id
    file: ./cache/china-domains.yaml
    enabled: true
    auto_update: true
    refresh_interval: 24h
    format: dnsmasq
    domain_count: 0              # 初始值，会自动更新
    last_updated: ""             # 初始值，会自动更新
```

### 完整配置

```yaml
default_group: default-id

cache:
  enabled: true
  directory: ./cache

groups:
  - name: china
    id: china-id
    upstreams:
      - https://dns.alidns.com/dns-query
      - https://doh.pub/dns-query
  
  - name: overseas
    id: overseas-id
    upstreams:
      - https://dns.google/dns-query
      - https://cloudflare-dns.com/dns-query

domains_lists:
  - name: china-domains
    source: https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/accelerated-domains.china.conf
    group: china-id
    file: ./cache/china-domains.yaml
    enabled: true
    auto_update: true
    refresh_interval: 24h
    format: dnsmasq
    domain_count: 114898
    last_updated: "2026-05-04T10:30:00+08:00"
  
  - name: gfwlist
    source: https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt
    group: overseas-id
    file: ./cache/gfwlist.yaml
    enabled: true
    auto_update: true
    refresh_interval: 24h
    format: gfwlist
    domain_count: 4161
    last_updated: "2026-05-04T10:30:00+08:00"
```

---

## 🔄 更新时机

统计信息会在以下情况自动更新：

### 1. 域名列表加载时
```go
// 解析配置时自动更新
ugc, err := proxy.ParseUpstreamGroups(spec, opts)
// 统计信息已更新到配置文件
```

### 2. 热重载时
```go
// 配置文件变化时自动重载
hrm := proxy.NewHotReloadManager(configPath, spec, opts, onChange)
hrm.Start(5 * time.Second)
// 每次重载都会更新统计信息
```

### 3. API 操作时
```go
// 添加新域名列表
hrm.AddDomainList(newList)
// 统计信息自动更新

// 更新域名列表
hrm.UpdateDomainList("list-name", updates)
// 统计信息自动更新
```

---

## 🛠️ API 使用

### 手动收集统计信息

```go
// 收集统计信息
stats, err := proxy.CollectDomainListStats(spec)
if err != nil {
    log.Fatal(err)
}

// 查看统计
for name, stat := range stats {
    fmt.Printf("%s: %d domains, updated at %s\n",
        name, stat.DomainCount, stat.LastUpdated)
}
```

### 手动更新配置文件

```go
// 更新配置文件
err := proxy.UpdateConfigFileStats(configPath, stats)
if err != nil {
    log.Fatal(err)
}
```

---

## 📊 统计数据结构

### DomainListStats

```go
type DomainListStats struct {
    DomainCount int       // 域名数量
    LastUpdated time.Time // 最后更新时间
    Format      string    // 检测到的格式
}
```

### 配置文件字段

| 字段 | 类型 | 说明 | 示例 |
|------|------|------|------|
| domain_count | int | 域名数量 | 114898 |
| last_updated | string | 最后更新时间 (RFC3339) | "2026-05-04T10:30:00+08:00" |
| format | string | 域名列表格式 | "dnsmasq", "plain", "hosts", "adblock", "gfwlist" |

---

## 🎨 前端集成示例

### React 组件

```jsx
import React, { useState, useEffect } from 'react';
import YAML from 'yaml';

function DomainListStats() {
  const [lists, setLists] = useState([]);

  useEffect(() => {
    fetch('/config.yaml')
      .then(r => r.text())
      .then(text => {
        const config = YAML.parse(text);
        setLists(config.domains_lists || []);
      });
  }, []);

  return (
    <div>
      <h2>域名列表统计</h2>
      {lists.map(list => (
        <div key={list.name}>
          <h3>{list.name}</h3>
          <p>域名数量: {list.domain_count?.toLocaleString() || 0}</p>
          <p>最后更新: {list.last_updated || '未更新'}</p>
          <p>格式: {list.format || '未知'}</p>
          <p>状态: {list.enabled ? '启用' : '禁用'}</p>
        </div>
      ))}
    </div>
  );
}
```

### Vue 组件

```vue
<template>
  <div>
    <h2>域名列表统计</h2>
    <div v-for="list in lists" :key="list.name">
      <h3>{{ list.name }}</h3>
      <p>域名数量: {{ (list.domain_count || 0).toLocaleString() }}</p>
      <p>最后更新: {{ list.last_updated || '未更新' }}</p>
      <p>格式: {{ list.format || '未知' }}</p>
      <p>状态: {{ list.enabled ? '启用' : '禁用' }}</p>
    </div>
  </div>
</template>

<script>
import YAML from 'yaml';

export default {
  data() {
    return {
      lists: []
    };
  },
  async mounted() {
    const response = await fetch('/config.yaml');
    const text = await response.text();
    const config = YAML.parse(text);
    this.lists = config.domains_lists || [];
  }
};
</script>
```

---

## 🔍 实际测试数据

### 测试配置

```yaml
domains_lists:
  - name: china-domains
    source: https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/accelerated-domains.china.conf
    group: china-id
    file: ./cache/china-domains.yaml
    enabled: true
    format: dnsmasq
    domain_count: 114898
    last_updated: "2026-05-04T09:25:07+08:00"
  
  - name: gfwlist
    source: https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt
    group: overseas-id
    file: ./cache/gfwlist.yaml
    enabled: true
    format: gfwlist
    domain_count: 4161
    last_updated: "2026-05-04T09:25:07+08:00"
```

### 测试结果

- ✅ 中国域名列表: 114,898 个域名
- ✅ GFW 列表: 4,161 个域名
- ✅ 总计: 119,059 个域名
- ✅ 统计信息自动更新到配置文件
- ✅ 前端可以直接读取

---

## 📖 相关文档

- [缓存功能指南](CACHE_GUIDE.md)
- [热重载指南](HOTRELOAD_GUIDE.md)
- [API 文档](API_GUIDE.md)
- [配置更新指南](UPDATE_CONFIG_GUIDE.md)
- [完整测试报告](COMPLETE_E2E_TEST_REPORT.md)

---

## ✅ 功能验证

所有功能已通过完整的端到端测试：

- ✅ 统计信息收集
- ✅ 配置文件更新
- ✅ 格式自动检测
- ✅ 热重载支持
- ✅ API 集成
- ✅ 前端读取

详见 [完整测试报告](COMPLETE_E2E_TEST_REPORT.md)

---

**版本:** v2.2.3  
**状态:** ✅ 生产就绪  
**最后更新:** 2026-05-04
