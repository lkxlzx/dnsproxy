# AdGuard Home 集成快速开始

## 🚀 5 分钟快速部署

### 前置要求

- Go 1.21+
- AdGuard Home（可选）
- Docker（可选）

---

## 方式 1: 独立运行（最简单）

### 步骤 1: 准备配置文件

创建 `config.yaml`:

```yaml
upstream_groups:
  - name: china
    upstreams:
      - 223.5.5.5
      - 119.29.29.29
    mode: load_balance
    enabled: true

  - name: overseas
    upstreams:
      - 8.8.8.8
      - 1.1.1.1
    mode: parallel
    enabled: true

domain_groups:
  china:
    - baidu.com
    - *.cn
  
default_group: overseas
```

### 步骤 2: 编译运行

```bash
# 克隆仓库
git clone https://github.com/AdguardTeam/dnsproxy.git
cd dnsproxy

# 编译
go build -o dnsproxy

# 运行
./dnsproxy -c config.yaml -l 0.0.0.0 -p 5353
```

### 步骤 3: 测试

```bash
# 测试中国域名（应该使用 223.5.5.5）
dig @127.0.0.1 -p 5353 baidu.com

# 测试海外域名（应该使用 8.8.8.8）
dig @127.0.0.1 -p 5353 google.com
```

---

## 方式 2: 与 AdGuard Home 集成

### 步骤 1: 安装 AdGuard Home

```bash
# Linux/Mac
curl -s -S -L https://raw.githubusercontent.com/AdguardTeam/AdGuardHome/master/scripts/install.sh | sh -s -- -v

# 或使用 Docker
docker run -d --name adguardhome \
  -p 53:53/tcp -p 53:53/udp \
  -p 80:80/tcp \
  adguard/adguardhome
```

### 步骤 2: 启动 DNSProxy

```bash
# 使用上面的配置文件
./dnsproxy -c config.yaml -l 127.0.0.1 -p 5353
```

### 步骤 3: 配置 AdGuard Home

1. 打开 AdGuard Home Web UI: `http://localhost:80`
2. 进入 **设置** → **DNS 设置**
3. 在 **上游 DNS 服务器** 中添加:
   ```
   127.0.0.1:5353
   ```
4. 保存设置

### 步骤 4: 验证

在 AdGuard Home 的 **查询日志** 中查看 DNS 查询，应该能看到请求被转发到 DNSProxy。

---

## 方式 3: Docker Compose 一键部署

### 步骤 1: 创建 docker-compose.yml

```yaml
version: '3'

services:
  dnsproxy:
    image: golang:1.21
    working_dir: /app
    volumes:
      - .:/app
    command: >
      sh -c "
        go build -o dnsproxy &&
        ./dnsproxy -c config.yaml -l 0.0.0.0 -p 5353
      "
    ports:
      - "5353:5353/udp"
    networks:
      - dns_network

  adguardhome:
    image: adguard/adguardhome:latest
    ports:
      - "53:53/tcp"
      - "53:53/udp"
      - "80:80/tcp"
    volumes:
      - ./adguard/work:/opt/adguardhome/work
      - ./adguard/conf:/opt/adguardhome/conf
    environment:
      - UPSTREAM_DNS=dnsproxy:5353
    networks:
      - dns_network
    depends_on:
      - dnsproxy

networks:
  dns_network:
    driver: bridge
```

### 步骤 2: 启动

```bash
docker-compose up -d
```

### 步骤 3: 访问

- AdGuard Home UI: `http://localhost:80`
- DNSProxy: `127.0.0.1:5353`

---

## 📋 完整配置示例

### 基础配置

```yaml
# config.yaml
upstream_groups:
  - name: china
    upstreams:
      - 223.5.5.5
      - 119.29.29.29
    mode: load_balance
    timeout: 5s
    enabled: true

  - name: overseas
    upstreams:
      - 8.8.8.8
      - 1.1.1.1
    mode: parallel
    timeout: 10s
    enabled: true

domain_groups:
  china:
    - baidu.com
    - taobao.com
    - *.cn

default_group: overseas
```

### 高级配置（带域名文件）

```yaml
# config-advanced.yaml
upstream_groups:
  - name: china
    upstreams:
      - 223.5.5.5
      - 119.29.29.29
      - 114.114.114.114
    mode: load_balance
    timeout: 5s
    enabled: true

  - name: overseas
    upstreams:
      - 8.8.8.8
      - 1.1.1.1
      - https://dns.google/dns-query
    mode: parallel
    timeout: 10s
    enabled: true

  - name: adblock
    upstreams:
      - 127.0.0.1:5354
    mode: load_balance
    timeout: 1s
    enabled: true

domain_groups:
  # 从本地文件加载
  china:
    ./domains/china.txt
  
  # 从远程 URL 加载
  china_remote:
    https://raw.githubusercontent.com/felixonmars/dnsmasq-china-list/master/accelerated-domains.china.conf
  
  # 广告拦截列表
  adblock:
    - https://adguardteam.github.io/AdGuardSDNSFilter/Filters/filter.txt

default_group: overseas

cache:
  enabled: true
  directory: ./cache
  ttl: 24h
```

---

## 🧪 测试和验证

### 1. 测试域名匹配

```bash
# 测试中国域名
dig @127.0.0.1 -p 5353 baidu.com +short

# 测试海外域名
dig @127.0.0.1 -p 5353 google.com +short

# 测试通配符匹配
dig @127.0.0.1 -p 5353 www.example.cn +short
```

### 2. 查看日志

```bash
# 启动时添加详细日志
./dnsproxy -c config.yaml -l 0.0.0.0 -p 5353 -v
```

### 3. 性能测试

```bash
# 使用 dnsperf 测试
dnsperf -s 127.0.0.1 -p 5353 -d domains.txt -c 100 -l 30
```

---

## 📊 监控和统计

### 查看统计信息

如果启用了 API 服务器：

```bash
# 获取上游组统计
curl http://localhost:3000/api/stats/upstream_groups

# 测试域名匹配
curl -X POST http://localhost:3000/api/test/domain_match \
  -H "Content-Type: application/json" \
  -d '{"domain": "www.baidu.com"}'
```

---

## 🔧 常见问题

### Q1: 如何添加更多域名？

**方法 1**: 直接在配置文件中添加
```yaml
domain_groups:
  china:
    - newdomain.com
    - *.newdomain.com
```

**方法 2**: 使用域名文件
```yaml
domain_groups:
  china:
    ./domains/my-domains.txt
```

### Q2: 如何更新远程域名列表？

重启 DNSProxy 会自动重新下载远程列表。或者使用 API：

```bash
curl -X POST http://localhost:3000/api/domain_lists/china/update
```

### Q3: 如何查看哪个域名使用了哪个上游？

启用详细日志：
```bash
./dnsproxy -c config.yaml -l 0.0.0.0 -p 5353 -v
```

或使用 API 测试：
```bash
curl -X POST http://localhost:3000/api/test/domain_match \
  -d '{"domain": "example.com"}'
```

### Q4: 性能如何？

- 精确匹配: **13.84 ns/op**
- 通配符匹配: **46.49 ns/op**
- 支持 **10 万+** 域名无压力

### Q5: 如何备份配置？

```bash
# 备份配置文件
cp config.yaml config.yaml.backup

# 备份缓存（可选）
tar -czf cache-backup.tar.gz ./cache
```

---

## 📚 下一步

1. 阅读 [完整配置示例](config-adguardhome-integration.yaml)
2. 查看 [API 文档](ADGUARDHOME_UI_INTEGRATION.md)
3. 了解 [性能优化](RADIX_TREE_PERFORMANCE.md)
4. 参考 [集成指南](INTEGRATION_GUIDE.md)

---

## 💡 提示

- 使用 `load_balance` 模式可以分散负载
- 使用 `parallel` 模式可以获得最快响应
- 定期更新远程域名列表以保持最新
- 使用缓存可以提高性能

---

**快速开始完成！** 🎉

如有问题，请查看完整文档或提交 Issue。

