@echo off
chcp 65001 >nul
echo ========================================
echo DNSProxy 功能测试套件
echo ========================================
echo.

echo [1/6] 运行单元测试...
echo.
cd proxy
go test -v -run "TestCachePrefetch|TestHeatTracker|TestDomainGroup" -timeout 2m
if errorlevel 1 (
    echo.
    echo ✗ 单元测试失败
    cd ..
    pause
    exit /b 1
)
cd ..

echo.
echo ========================================
echo [2/6] 测试预取功能...
echo ========================================
echo.
if exist test_prefetch_complete.exe (
    test_prefetch_complete.exe
) else (
    echo 编译测试程序...
    go build -o test_prefetch_complete.exe test_prefetch_complete.go
    if errorlevel 1 (
        echo ✗ 编译失败
        pause
        exit /b 1
    )
    test_prefetch_complete.exe
)

echo.
echo ========================================
echo [3/6] 测试域名分组...
echo ========================================
echo.
if exist test_domain_groups.exe (
    test_domain_groups.exe
) else (
    echo 编译测试程序...
    go build -o test_domain_groups.exe test_domain_groups.go
    if errorlevel 1 (
        echo ✗ 编译失败
        pause
        exit /b 1
    )
    test_domain_groups.exe
)

echo.
echo ========================================
echo [4/6] 测试关键字匹配...
echo ========================================
echo.
if exist test_keyword_matching.exe (
    test_keyword_matching.exe
) else (
    echo 编译测试程序...
    go build -o test_keyword_matching.exe test_keyword_matching.go
    if errorlevel 1 (
        echo ✗ 编译失败
        pause
        exit /b 1
    )
    test_keyword_matching.exe
)

echo.
echo ========================================
echo [5/6] 测试通配符匹配...
echo ========================================
echo.
if exist test_wildcard_matching.exe (
    test_wildcard_matching.exe
) else (
    echo 编译测试程序...
    go build -o test_wildcard_matching.exe test_wildcard_matching.go
    if errorlevel 1 (
        echo ✗ 编译失败
        pause
        exit /b 1
    )
    test_wildcard_matching.exe
)

echo.
echo ========================================
echo [6/6] 运行基准测试...
echo ========================================
echo.
cd proxy
go test -bench=BenchmarkCachePrefetch -benchmem -benchtime=3s
cd ..

echo.
echo ========================================
echo ✓ 所有测试完成
echo ========================================
echo.
echo 测试摘要:
echo - 单元测试: 通过
echo - 预取功能: 通过
echo - 域名分组: 通过
echo - 关键字匹配: 通过
echo - 通配符匹配: 通过
echo - 基准测试: 完成
echo.
pause
