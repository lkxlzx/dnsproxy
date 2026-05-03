# Upstream Groups 高级功能使用示例

本文档提供所有高级功能的实际使用示例和最佳实践。

## 目录

- [健康检查示例](#健康检查示例)
- [统计信息示例](#统计信息示例)
- [动态重载示例](#动态重载示例)
- [HTTP API 示例](#http-api-示例)
- [完整集成示例](#完整集成示例)

---

## 健康检查示例

### 基础配置

```yaml
health-check:
  enabled: true
  interval: "30s"
  timeout: "5s"
  failure_threshold: 3
  success_threshold: 2
  test_domain: "dns.google."
```

### 使用场景 1：自动故障转移

当主服务器不可用时，自动切换到备用服务器。

```yaml
upstream-groups:
  default_group: "primary"
  groups:
    - name: "primary"
      upstreams: ["1.1.1.1", "8.8.8.8"]
    - name: "backup"
      upstreams: ["9.9.9.9", "149.112.112.112"]

health-check:
  enabled: true
  interval: "10s"
  failure_threshold: 2
```

### 使用场景 2：监控服务器健康

通过 API 实时监控所有服务器的健康状态。

```bash
# 查询所有服务器健康状态
curl http://127.0.0.1:8080/api/v1/health | jq

# 查询特定服务器
curl http://127.0.0.1:8080/api/v1/health/1.1.1.1:53 | jq
```

---

## 统计信息示例

### 基础配置

```yaml
statistics:
  enabled: true
```

### 使用场景 1：性能分析

分析每个组的性能指标。

```bash
# 获取所有组的统计
curl http://127.0.0.1:8080/api/v1/stats | jq

# 获取特定组的统计
curl http://127.0.0.1:8080/api/v1/stats/primary | jq
```

### 使用场景 2：识别慢速服务器

```bash
# 查找平均延迟最高的服务器
curl http://127.0.0.1:8080/api/v1/stats | \
  jq '.[] | .UpstreamStats | to_entries[] | 
  {address: .key, latency: .value.AverageLatency} | 
  select(.latency > 50)'
```

### 使用场景 3：计算成功率

```bash
# 计算每个组的成功率
curl http://127.0.0.1:8080/api/v1/stats | \
  jq 'to_entries[] | {
    group: .key,
    success_rate: (.value.SuccessfulQueries / .value.TotalQueries * 100)
  }'
```

---

## 动态重载示例

### 基础配置

```yaml
reload:
  enabled: true
  watch_file: "config.yaml"
  check_interval: "30s"
```


### 使用场景 1：自动重载

配置文件修改后自动重载，无需重启服务。

```bash
# 1. 启动服务
./dnsproxy --config-path=config.yaml --verbose

# 2. 修改配置文件
vim config.yaml

# 3. 保存后自动重载（30秒内）
# 查看日志确认重载成功
```

### 使用场景 2：手动触发重载

通过 API 手动触发配置重载。

```bash
# 触发重载
curl -X POST http://127.0.0.1:8080/api/v1/reload

# 响应示例
{
  "message": "configuration reloaded successfully"
}
```

### 使用场景 3：验证配置后重载

```bash
#!/bin/bash
# 安全重载脚本

CONFIG_FILE="config.yaml"
BACKUP_FILE="config.yaml.backup"

# 备份当前配置
cp $CONFIG_FILE $BACKUP_FILE

# 修改配置
vim $CONFIG_FILE

# 验证配置（可选：使用配置验证工具）
# ./dnsproxy --config-path=$CONFIG_FILE --validate

# 触发重载
if curl -X POST http://127.0.0.1:8080/api/v1/reload; then
    echo "重载成功"
    rm $BACKUP_FILE
else
    echo "重载失败，恢复配置"
    cp $BACKUP_FILE $CONFIG_FILE
fi
```

---

## HTTP API 示例

### 基础配置

```yaml
api:
  enabled: true
  listen_addr: "127.0.0.1:8080"
  auth_token: "your-secret-token"
```

### 使用场景 1：监控仪表板

创建简单的监控脚本。

```bash
#!/bin/bash
# monitor.sh - 简单的监控脚本

API_URL="http://127.0.0.1:8080/api/v1"
TOKEN="your-secret-token"

while true; do
    clear
    echo "=== DNS Proxy 监控 ==="
    echo "时间: $(date)"
    echo ""
    
    # 服务器状态
    echo "服务器状态:"
    curl -s "$API_URL/status" | jq
    echo ""
    
    # 不健康的服务器
    echo "不健康的服务器:"
    curl -s -H "Authorization: Bearer $TOKEN" "$API_URL/health" | \
        jq '.upstreams[] | select(.Healthy == false)'
    echo ""
    
    # 统计摘要
    echo "查询统计:"
    curl -s -H "Authorization: Bearer $TOKEN" "$API_URL/stats" | \
        jq 'to_entries[] | {
            group: .key,
            queries: .value.TotalQueries,
            success_rate: (.value.SuccessfulQueries / .value.TotalQueries * 100 | round)
        }'
    
    sleep 5
done
```

### 使用场景 2：Python 监控客户端

```python
#!/usr/bin/env python3
# monitor.py - Python 监控客户端

import requests
import time
import json
from datetime import datetime

class DNSProxyMonitor:
    def __init__(self, base_url, token=None):
        self.base_url = base_url
        self.headers = {}
        if token:
            self.headers['Authorization'] = f'Bearer {token}'
    
    def get_status(self):
        return requests.get(f'{self.base_url}/status').json()
    
    def get_health(self):
        return requests.get(
            f'{self.base_url}/health',
            headers=self.headers
        ).json()
    
    def get_stats(self):
        return requests.get(
            f'{self.base_url}/stats',
            headers=self.headers
        ).json()
    
    def check_unhealthy(self):
        health = self.get_health()
        unhealthy = [
            u for u in health['upstreams'] 
            if not u['Healthy']
        ]
        return unhealthy
    
    def get_slow_upstreams(self, threshold=50):
        stats = self.get_stats()
        slow = []
        for group_name, group_stats in stats.items():
            for addr, upstream_stats in group_stats['UpstreamStats'].items():
                if upstream_stats['AverageLatency'] > threshold:
                    slow.append({
                        'group': group_name,
                        'address': addr,
                        'latency': upstream_stats['AverageLatency']
                    })
        return slow
    
    def monitor(self, interval=10):
        while True:
            print(f"\n=== {datetime.now()} ===")
            
            # 检查不健康的服务器
            unhealthy = self.check_unhealthy()
            if unhealthy:
                print(f"⚠️  不健康的服务器: {len(unhealthy)}")
                for u in unhealthy:
                    print(f"  - {u['Address']}")
            else:
                print("✅ 所有服务器健康")
            
            # 检查慢速服务器
            slow = self.get_slow_upstreams(50)
            if slow:
                print(f"🐌 慢速服务器 (>50ms): {len(slow)}")
                for s in slow:
                    print(f"  - {s['address']}: {s['latency']:.1f}ms")
            
            # 显示统计
            stats = self.get_stats()
            print("\n📊 查询统计:")
            for group, data in stats.items():
                total = data['TotalQueries']
                success = data['SuccessfulQueries']
                rate = (success / total * 100) if total > 0 else 0
                print(f"  {group}: {total} 查询, {rate:.1f}% 成功")
            
            time.sleep(interval)

if __name__ == '__main__':
    monitor = DNSProxyMonitor(
        'http://127.0.0.1:8080/api/v1',
        'your-secret-token'
    )
    monitor.monitor()
```


---

## 完整集成示例

### 生产环境完整配置

```yaml
# production-config.yaml
# 生产环境完整配置示例

listen-addrs:
  - "0.0.0.0"
listen-ports:
  - 53
cache: true
cache-min-ttl: 300
cache-max-ttl: 3600
verbose: false

# 上游组配置
upstream-groups:
  default_group: "primary"
  
  groups:
    # 主要组 - Cloudflare + Google
    - name: "primary"
      enabled: true
      mode: "load_balance"
      timeout: "5s"
      max_retries: 2
      priority: 1
      upstreams:
        - "1.1.1.1:53"
        - "1.0.0.1:53"
        - "8.8.8.8:53"
        - "8.8.4.4:53"
    
    # 安全组 - 加密 DNS
    - name: "secure"
      enabled: true
      mode: "parallel"
      timeout: "10s"
      max_retries: 3
      priority: 2
      upstreams:
        - "tls://dns.adguard.com"
        - "https://dns.google/dns-query"
        - "https://cloudflare-dns.com/dns-query"
    
    # 本地组 - 内网 DNS
    - name: "local"
      enabled: true
      mode: "load_balance"
      timeout: "3s"
      max_retries: 1
      priority: 1
      upstreams:
        - "192.168.1.1:53"
        - "192.168.1.2:53"
    
    # 备用组 - Quad9
    - name: "backup"
      enabled: true
      mode: "load_balance"
      timeout: "15s"
      max_retries: 5
      priority: 10
      upstreams:
        - "9.9.9.9:53"
        - "149.112.112.112:53"
  
  domain_groups:
    # 内网域名
    "internal.company.com": "local"
    "corp.local": "local"
    "*.internal.company.com": "local"
    
    # 敏感域名使用加密
    "banking.company.com": "secure"
    "*.banking.company.com": "secure"
    "secure.company.com": "secure"

# 健康检查
health-check:
  enabled: true
  interval: "30s"
  timeout: "5s"
  failure_threshold: 3
  success_threshold: 2
  test_domain: "dns.google."

# 统计信息
statistics:
  enabled: true

# 动态重载
reload:
  enabled: true
  watch_file: "production-config.yaml"
  check_interval: "60s"

# HTTP API
api:
  enabled: true
  listen_addr: "127.0.0.1:8080"
  read_timeout: "10s"
  write_timeout: "10s"
  auth_token: "production-secret-token-change-me"
```

### 启动脚本

```bash
#!/bin/bash
# start-dnsproxy.sh - 生产环境启动脚本

set -e

CONFIG_FILE="production-config.yaml"
LOG_FILE="/var/log/dnsproxy/dnsproxy.log"
PID_FILE="/var/run/dnsproxy.pid"

# 检查配置文件
if [ ! -f "$CONFIG_FILE" ]; then
    echo "错误: 配置文件不存在: $CONFIG_FILE"
    exit 1
fi

# 创建日志目录
mkdir -p "$(dirname $LOG_FILE)"

# 启动服务
echo "启动 dnsproxy..."
./dnsproxy \
    --config-path="$CONFIG_FILE" \
    --output="$LOG_FILE" \
    --verbose &

# 保存 PID
echo $! > "$PID_FILE"

echo "dnsproxy 已启动, PID: $(cat $PID_FILE)"
echo "日志文件: $LOG_FILE"
echo "API 地址: http://127.0.0.1:8080"
```

### 监控和告警脚本

```bash
#!/bin/bash
# alert.sh - 监控和告警脚本

API_URL="http://127.0.0.1:8080/api/v1"
TOKEN="production-secret-token-change-me"
ALERT_EMAIL="admin@company.com"

# 检查不健康的服务器
check_health() {
    unhealthy=$(curl -s -H "Authorization: Bearer $TOKEN" "$API_URL/health" | \
        jq -r '.upstreams[] | select(.Healthy == false) | .Address')
    
    if [ -n "$unhealthy" ]; then
        echo "⚠️  发现不健康的服务器:"
        echo "$unhealthy"
        
        # 发送告警邮件
        echo "不健康的 DNS 服务器: $unhealthy" | \
            mail -s "DNS Proxy 告警" "$ALERT_EMAIL"
        
        return 1
    fi
    
    return 0
}

# 检查失败率
check_failure_rate() {
    high_failure=$(curl -s -H "Authorization: Bearer $TOKEN" "$API_URL/stats" | \
        jq -r 'to_entries[] | 
        select((.value.TotalQueries > 100) and 
               ((.value.FailedQueries / .value.TotalQueries) > 0.1)) | 
        .key')
    
    if [ -n "$high_failure" ]; then
        echo "⚠️  发现高失败率的组:"
        echo "$high_failure"
        
        # 发送告警邮件
        echo "DNS 组失败率过高: $high_failure" | \
            mail -s "DNS Proxy 告警" "$ALERT_EMAIL"
        
        return 1
    fi
    
    return 0
}

# 主循环
while true; do
    echo "$(date): 执行健康检查..."
    
    check_health
    check_failure_rate
    
    sleep 300  # 每5分钟检查一次
done
```

### Systemd 服务配置

```ini
# /etc/systemd/system/dnsproxy.service

[Unit]
Description=DNS Proxy with Upstream Groups
After=network.target

[Service]
Type=simple
User=dnsproxy
Group=dnsproxy
WorkingDirectory=/opt/dnsproxy
ExecStart=/opt/dnsproxy/dnsproxy --config-path=/opt/dnsproxy/production-config.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5s

# 安全设置
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/log/dnsproxy

[Install]
WantedBy=multi-user.target
```

### 使用 Systemd 管理

```bash
# 安装服务
sudo cp dnsproxy.service /etc/systemd/system/
sudo systemctl daemon-reload

# 启动服务
sudo systemctl start dnsproxy

# 设置开机自启
sudo systemctl enable dnsproxy

# 查看状态
sudo systemctl status dnsproxy

# 查看日志
sudo journalctl -u dnsproxy -f

# 重载配置
sudo systemctl reload dnsproxy

# 停止服务
sudo systemctl stop dnsproxy
```

---

## 最佳实践总结

### 1. 健康检查配置

- ✅ 生产环境必须启用健康检查
- ✅ 设置合理的检查间隔（30-60秒）
- ✅ 根据网络情况调整阈值
- ✅ 监控健康检查日志

### 2. 统计信息使用

- ✅ 定期查看统计信息
- ✅ 识别性能瓶颈
- ✅ 根据统计优化配置
- ✅ 设置告警阈值

### 3. 动态重载

- ✅ 启用自动重载
- ✅ 修改前备份配置
- ✅ 验证配置正确性
- ✅ 监控重载日志

### 4. API 安全

- ✅ 使用强密码作为 token
- ✅ 只监听本地地址
- ✅ 使用 HTTPS（如需外部访问）
- ✅ 定期更换 token

### 5. 监控和告警

- ✅ 实施持续监控
- ✅ 设置多级告警
- ✅ 记录所有异常
- ✅ 定期审查日志

---

## 故障排查

### 问题 1：健康检查失败

**症状**：所有服务器被标记为不健康

**解决方案**：
```bash
# 1. 检查网络连接
ping 1.1.1.1

# 2. 检查 DNS 查询
dig @1.1.1.1 dns.google

# 3. 调整超时设置
# 在配置中增加 timeout
health-check:
  timeout: "10s"
```

### 问题 2：统计信息不准确

**症状**：统计数据异常

**解决方案**：
```bash
# 重置统计信息（通过 API）
curl -X POST http://127.0.0.1:8080/api/v1/stats/reset
```

### 问题 3：重载失败

**症状**：配置重载后服务异常

**解决方案**：
```bash
# 1. 检查配置文件语法
./dnsproxy --config-path=config.yaml --validate

# 2. 查看错误日志
tail -f /var/log/dnsproxy/dnsproxy.log

# 3. 恢复备份配置
cp config.yaml.backup config.yaml
curl -X POST http://127.0.0.1:8080/api/v1/reload
```

---

## 总结

本文档提供了所有高级功能的实际使用示例，包括：

- ✅ 健康检查的配置和使用
- ✅ 统计信息的收集和分析
- ✅ 动态重载的实现方法
- ✅ HTTP API 的完整示例
- ✅ 生产环境的完整配置
- ✅ 监控和告警的实现
- ✅ 最佳实践和故障排查

通过这些示例，你可以充分利用 Upstream Groups 的所有高级功能，构建一个强大、可靠、易于管理的 DNS 代理服务。
