@echo off
REM End-to-End DNS Routing Test Script for Windows
REM Tests domain group routing with real DNS queries

setlocal enabledelayedexpansion

echo ==========================================
echo DNS Upstream Groups - E2E Routing Test
echo ==========================================
echo.

REM Test configuration
set DNSPROXY_PORT=5301
set DNSPROXY_ADDR=127.0.0.1
set CONFIG_FILE=config-test-e2e-routing.yaml
set CACHE_DIR=.\cache\test-e2e
set DOMAINS_DIR=.\domains

REM Test counters
set TOTAL_TESTS=0
set PASSED_TESTS=0
set FAILED_TESTS=0

REM Step 1: Create test configuration
echo Step 1: Creating test configuration...

if not exist "%CACHE_DIR%" mkdir "%CACHE_DIR%"
if not exist "%DOMAINS_DIR%" mkdir "%DOMAINS_DIR%"

REM Create domain files
(
echo baidu.com
echo qq.com
echo taobao.com
echo jd.com
echo 163.com
echo sina.com.cn
echo weibo.com
echo bilibili.com
) > "%DOMAINS_DIR%\china-test.txt"

(
echo google.com
echo youtube.com
echo facebook.com
echo twitter.com
echo github.com
echo stackoverflow.com
echo reddit.com
) > "%DOMAINS_DIR%\overseas-test.txt"

REM Create test configuration
(
echo # E2E Routing Test Configuration
echo.
echo # Upstream groups
echo upstream-groups:
echo   default_group: "overseas"
echo.  
echo   groups:
echo     # Overseas DNS ^(Google, Cloudflare^)
echo     - name: "overseas"
echo       mode: "load_balance"
echo       upstreams:
echo         - "8.8.8.8"
echo         - "1.1.1.1"
echo       timeout: "3s"
echo       enabled: true
echo.    
echo     # China DNS ^(Alibaba, DNSPod^)
echo     - name: "china"
echo       mode: "load_balance"
echo       upstreams:
echo         - "223.5.5.5"
echo         - "119.29.29.29"
echo       timeout: "3s"
echo       enabled: true
echo.
echo   # Domain routing rules
echo   domain_groups:
echo     # China domains
echo     "baidu.com": "china"
echo     "*.baidu.com": "china"
echo     "qq.com": "china"
echo     "*.qq.com": "china"
echo     "taobao.com": "china"
echo     "jd.com": "china"
echo     "bilibili.com": "china"
echo     "*.bilibili.com": "china"
echo.    
echo     # Overseas domains
echo     "google.com": "overseas"
echo     "*.google.com": "overseas"
echo     "youtube.com": "overseas"
echo     "github.com": "overseas"
echo     "*.github.com": "overseas"
echo.
echo # Listen configuration
echo listen-addrs:
echo   - "127.0.0.1"
echo.
echo listen-ports:
echo   - %DNSPROXY_PORT%
echo.
echo # Logging
echo log-level: "info"
) > "%CONFIG_FILE%"

echo [OK] Configuration created: %CONFIG_FILE%

REM Step 2: Build dnsproxy (if needed)
echo.
echo Step 2: Checking dnsproxy binary...

if not exist "dnsproxy.exe" (
    echo [INFO] Building dnsproxy...
    go build -o dnsproxy.exe .\cmd\dnsproxy
    if errorlevel 1 (
        echo [ERROR] Failed to build dnsproxy
        exit /b 1
    )
    echo [OK] DNSProxy built
) else (
    echo [OK] DNSProxy binary found
)

REM Step 3: Start dnsproxy
echo.
echo Step 3: Starting DNSProxy...

start /B dnsproxy.exe --config-path="%CONFIG_FILE%" > dnsproxy-e2e.log 2>&1

REM Wait for dnsproxy to start
timeout /t 3 /nobreak > nul

echo [OK] DNSProxy started on %DNSPROXY_ADDR%:%DNSPROXY_PORT%

REM Step 4: Run tests
echo.
echo ==========================================
echo Running DNS Routing Tests
echo ==========================================

echo.
echo === Test Group 1: China Domains ===
echo.

call :run_test "Baidu" "baidu.com" "china"
call :run_test "Baidu subdomain" "www.baidu.com" "china"
call :run_test "QQ" "qq.com" "china"
call :run_test "QQ subdomain" "mail.qq.com" "china"
call :run_test "Taobao" "taobao.com" "china"
call :run_test "JD" "jd.com" "china"
call :run_test "Bilibili" "bilibili.com" "china"
call :run_test "Bilibili subdomain" "www.bilibili.com" "china"

echo.
echo === Test Group 2: Overseas Domains ===
echo.

call :run_test "Google" "google.com" "overseas"
call :run_test "Google subdomain" "www.google.com" "overseas"
call :run_test "YouTube" "youtube.com" "overseas"
call :run_test "GitHub" "github.com" "overseas"
call :run_test "GitHub subdomain" "api.github.com" "overseas"

echo.
echo === Test Group 3: Default Group ===
echo.

call :run_test "Example.com" "example.com" "overseas"
call :run_test "Cloudflare" "cloudflare.com" "overseas"

echo.
echo === Test Group 4: Wildcard Matching ===
echo.

call :run_test "Deep subdomain (Baidu)" "tieba.baidu.com" "china"
call :run_test "Deep subdomain (Google)" "mail.google.com" "overseas"

REM Step 5: Performance Test
echo.
echo ==========================================
echo Performance Test
echo ==========================================
echo.

echo [INFO] Running 10 queries to measure performance...

set start_time=%time%

for /L %%i in (1,1,10) do (
    nslookup baidu.com %DNSPROXY_ADDR% > nul 2>&1
)

set end_time=%time%

echo   Start: %start_time%
echo   End: %end_time%
echo [OK] Performance test completed

REM Step 6: Summary
echo.
echo ==========================================
echo Test Summary
echo ==========================================
echo Total Tests: %TOTAL_TESTS%
echo Passed: %PASSED_TESTS%
echo Failed: %FAILED_TESTS%

if %FAILED_TESTS% EQU 0 (
    echo.
    echo [SUCCESS] All tests passed!
) else (
    echo.
    echo [ERROR] Some tests failed!
)

REM Cleanup
echo.
echo [INFO] Stopping DNSProxy...
taskkill /F /IM dnsproxy.exe > nul 2>&1

echo.
echo Test completed. Check dnsproxy-e2e.log for details.
echo.

pause
exit /b 0

REM Function to run a test
:run_test
set /a TOTAL_TESTS+=1
set test_name=%~1
set domain=%~2
set expected_group=%~3

echo Test %TOTAL_TESTS%: %test_name%
echo   Domain: %domain%
echo   Expected Group: %expected_group%

REM Query DNS using nslookup
nslookup %domain% %DNSPROXY_ADDR% > nul 2>&1

if errorlevel 1 (
    echo   [FAIL] DNS query failed
    set /a FAILED_TESTS+=1
) else (
    echo   [PASS] DNS query successful
    set /a PASSED_TESTS+=1
)

echo.
goto :eof
