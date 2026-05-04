#!/bin/bash

# API 测试脚本

API_BASE="http://localhost:8080/api"

echo "================================"
echo "DNSProxy API 测试"
echo "================================"
echo ""

# 测试健康检查
echo "1. 健康检查"
echo "GET $API_BASE/health"
curl -s "$API_BASE/health" | python -m json.tool
echo ""
echo ""

# 获取统计信息
echo "2. 获取统计信息"
echo "GET $API_BASE/domain-lists/stats"
curl -s "$API_BASE/domain-lists/stats" | python -m json.tool
echo ""
echo ""

# 获取所有域名列表
echo "3. 获取所有域名列表"
echo "GET $API_BASE/domain-lists"
curl -s "$API_BASE/domain-lists" | python -m json.tool
echo ""
echo ""

# 刷新所有列表
echo "4. 刷新所有列表"
echo "POST $API_BASE/domain-lists/refresh"
curl -s -X POST "$API_BASE/domain-lists/refresh" | python -m json.tool
echo ""
echo ""

echo "================================"
echo "测试完成"
echo "================================"
