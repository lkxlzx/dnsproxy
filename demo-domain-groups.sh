#!/bin/bash
# 域名分组功能演示脚本 (Linux/macOS)

echo "========================================"
echo "DNSProxy 域名分组功能演示"
echo "========================================"
echo ""

echo "1. 运行集成测试"
echo "----------------------------------------"
go test -v ./proxy -run TestUpstreamGroupIntegration
echo ""

echo "2. 测试结果总结"
echo "----------------------------------------"
echo "如果所有测试都显示 PASS，说明功能正常工作："
echo "  - 海外域名 (google.com) 使用 8.8.8.8"
echo "  - 国内域名 (baidu.com) 使用 223.5.5.5"
echo "  - 通配符匹配正常工作"
echo "  - 默认组回退正常工作"
echo ""

echo "3. 查看测试报告"
echo "----------------------------------------"
echo "详细测试报告请查看："
echo "  - DOMAIN_GROUPS_TEST_REPORT.md"
echo "  - FINAL_TEST_SUMMARY.md"
echo ""

echo "4. 启动测试服务 (可选)"
echo "----------------------------------------"
echo "如果要启动测试服务，请运行："
echo "  ./dnsproxy --config-path=config-test-domain-groups.yaml"
echo ""
echo "然后可以使用 dig 测试："
echo "  dig @127.0.0.1 -p 5301 baidu.com"
echo "  dig @127.0.0.1 -p 5301 google.com"
echo ""
