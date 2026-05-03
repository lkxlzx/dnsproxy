#!/bin/bash
# Upstream Groups 功能演示脚本

echo "=================================="
echo "Upstream Groups 功能演示"
echo "=================================="
echo ""

# 检查 dnsproxy 是否存在
if [ ! -f "./dnsproxy" ]; then
    echo "❌ 错误: 找不到 dnsproxy 可执行文件"
    echo "请先运行: make build"
    exit 1
fi

echo "✅ 找到 dnsproxy 可执行文件"
echo ""

# 演示 1: 基础配置
echo "📝 演示 1: 基础配置"
echo "-----------------------------------"
cat > demo-basic.yaml << 'EOF'
listen-addrs:
  - "127.0.0.1"
listen-ports:
  - 5353
cache: true

upstream-groups:
  default_group: "cloudflare"
  groups:
    - name: "cloudflare"
      mode: "load_balance"
      timeout: "5s"
      enabled: true
      upstreams:
        - "1.1.1.1"
        - "1.0.0.1"
EOF

echo "配置文件内容:"
cat demo-basic.yaml
echo ""
echo "启动命令: ./dnsproxy --config-path=demo-basic.yaml"
echo ""

# 演示 2: 内外网分离
echo "📝 演示 2: 内外网分离配置"
echo "-----------------------------------"
cat > demo-split.yaml << 'EOF'
listen-addrs:
  - "127.0.0.1"
listen-ports:
  - 5353
cache: true

upstream-groups:
  default_group: "public"
  
  groups:
    - name: "public"
      mode: "load_balance"
      timeout: "5s"
      enabled: true
      upstreams:
        - "1.1.1.1"
        - "8.8.8.8"
    
    - name: "internal"
      mode: "load_balance"
      timeout: "3s"
      enabled: true
      upstreams:
        - "192.168.1.1"
  
  domain_groups:
    "internal.local": "internal"
    "corp.local": "internal"
EOF

echo "配置文件内容:"
cat demo-split.yaml
echo ""
echo "说明: 内网域名 (*.internal.local, *.corp.local) 使用内网 DNS"
echo "      其他域名使用公网 DNS (Cloudflare, Google)"
echo ""

# 演示 3: 加密 DNS
echo "📝 演示 3: 加密 DNS 配置"
echo "-----------------------------------"
cat > demo-encrypted.yaml << 'EOF'
listen-addrs:
  - "127.0.0.1"
listen-ports:
  - 5353
cache: true

upstream-groups:
  default_group: "standard"
  
  groups:
    - name: "standard"
      mode: "load_balance"
      timeout: "5s"
      enabled: true
      upstreams:
        - "1.1.1.1"
        - "8.8.8.8"
    
    - name: "encrypted"
      mode: "parallel"
      timeout: "10s"
      enabled: true
      upstreams:
        - "tls://dns.adguard.com"
        - "https://dns.google/dns-query"
  
  domain_groups:
    "banking.com": "encrypted"
    "secure.example.com": "encrypted"
EOF

echo "配置文件内容:"
cat demo-encrypted.yaml
echo ""
echo "说明: 敏感域名使用加密 DNS (DoT/DoH)"
echo "      其他域名使用标准 DNS"
echo ""

# 演示 4: 文本格式配置
echo "📝 演示 4: 文本格式配置"
echo "-----------------------------------"
cat > demo-groups.txt << 'EOF'
# 主要组 - 快速公共 DNS
[group:primary:load_balance:5s]
1.1.1.1
8.8.8.8

# 本地组 - 内网 DNS
[group:local:load_balance:3s]
192.168.1.1

# 域名映射
[/internal.local/corp.local/]local

# 默认组
[default]primary
EOF

echo "配置文件内容:"
cat demo-groups.txt
echo ""
echo "启动命令: ./dnsproxy --upstream-groups-file=demo-groups.txt -l 127.0.0.1 -p 5353"
echo ""

# 测试说明
echo "🧪 测试方法"
echo "-----------------------------------"
echo "1. 启动服务:"
echo "   ./dnsproxy --config-path=demo-basic.yaml --verbose"
echo ""
echo "2. 在另一个终端测试查询:"
echo "   # Linux/Mac"
echo "   dig @127.0.0.1 -p 5353 google.com"
echo ""
echo "   # Windows"
echo "   nslookup google.com 127.0.0.1"
echo ""

# 性能测试
echo "⚡ 性能测试"
echo "-----------------------------------"
echo "使用 dnsperf 进行性能测试:"
echo "  dnsperf -s 127.0.0.1 -p 5353 -d queries.txt"
echo ""

# 清理说明
echo "🧹 清理"
echo "-----------------------------------"
echo "删除演示配置文件:"
echo "  rm demo-*.yaml demo-groups.txt"
echo ""

echo "=================================="
echo "演示准备完成！"
echo "=================================="
echo ""
echo "📚 更多信息:"
echo "  - 快速入门: UPSTREAM_GROUPS_QUICKSTART.md"
echo "  - 完整文档: UPSTREAM_GROUPS.md"
echo "  - 配置示例: config-groups.yaml.example"
echo ""
echo "🚀 开始使用:"
echo "  ./dnsproxy --config-path=demo-basic.yaml --verbose"
