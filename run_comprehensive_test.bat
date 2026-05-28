@echo off
echo ========================================
echo DNSProxy 综合功能测试
echo ========================================
echo.

echo 编译测试程序...
go build -o test_comprehensive.exe test_comprehensive_features.go
if errorlevel 1 (
    echo 编译失败！
    pause
    exit /b 1
)

echo 编译配置文件测试程序...
go build -o test_with_config.exe test_with_config.go
if errorlevel 1 (
    echo 编译失败！
    pause
    exit /b 1
)

echo.
echo ========================================
echo 测试1: 使用代码配置
echo ========================================
echo.
test_comprehensive.exe

echo.
echo ========================================
echo 测试2: 使用YAML配置文件
echo ========================================
echo.
test_with_config.exe test_comprehensive_config.yaml

echo.
echo ========================================
echo 所有测试完成
echo ========================================
pause
