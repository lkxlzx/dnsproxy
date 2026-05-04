#!/bin/bash

# 测试动态添加域名列表

API_BASE="http://localhost:8080/api"

echo "================================"
echo "动态添加域名列表测试"
echo "================================"
echo ""

# 1. 查看当前列表
echo "1. 查看当前域名列表"
echo "GET $API_BASE/domain-lists"
curl -s "$API_BASE/domain-lists" | python -m json.tool
echo ""
echo ""

# 2. 添加新的域名列表
echo "2. 添加新的域名列表 (Anti-AD)"
echo "POST $API_BASE/domain-lists"
curl -s -X POST "$API_BASE/domain-lists" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "anti-ad",
    "source": "https://raw.githubusercontent.com/privacy-protection-tools/anti-AD/master/anti-ad-domains.txt",
    "group": "overseas-id",
    "enabled": true,
    "auto_update": true,
    "refresh_interval": "12h",
    "format": "hosts"
  }' | python -m json.tool
echo ""
echo ""

# 3. 等待几秒让系统下载和处理
echo "3. 等待 5 秒让系统下载域名列表..."
sleep 5
echo ""

# 4. 再次查看列表（应该包含新添加的）
echo "4. 查看更新后的域名列表"
echo "GET $API_BASE/domain-lists"
curl -s "$API_BASE/domain-lists" | python -m json.tool
echo ""
echo ""

# 5. 查看健康状态
echo "5. 查看健康状态"
echo "GET $API_BASE/health"
curl -s "$API_BASE/health" | python -m json.tool
echo ""
echo ""

# 6. 更新域名列表
echo "6. 更新域名列表 (禁用 anti-ad)"
echo "PUT $API_BASE/domain-lists/update?name=anti-ad"
curl -s -X PUT "$API_BASE/domain-lists/update?name=anti-ad" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": false
  }' | python -m json.tool
echo ""
echo ""

# 7. 删除域名列表
echo "7. 删除域名列表 (anti-ad)"
echo "DELETE $API_BASE/domain-lists/remove?name=anti-ad"
curl -s -X DELETE "$API_BASE/domain-lists/remove?name=anti-ad" | python -m json.tool
echo ""
echo ""

# 8. 最终查看列表
echo "8. 最终查看域名列表"
echo "GET $API_BASE/domain-lists"
curl -s "$API_BASE/domain-lists" | python -m json.tool
echo ""
echo ""

echo "================================"
echo "测试完成"
echo "================================"
