@echo off
REM Upstream Groups 功能演示脚本 (Windows)

echo ==================================
echo Upstream Groups 功能演示
echo ==================================
echo.

REM 检查 dnsproxy 是否存在
if not exist "dnsproxy.exe" (
    echo ❌ 错误: 找不到 dnsproxy.exe 可执行文件
    echo 请先运行: make build
    exit /b 1
)

echo ✅ 找到 dnsproxy.exe 可执行文件
echo.

REM 演示 1: 基础配置
echo 📝 演示 1: 基础配置
echo -----------------------------------
(
echo listen-addrs:
echo   - "127.0.0.1"
echo listen-ports:
echo   - 5353
echo cache: true
echo.
echo upstream-groups:
echo   default_group: "cloudflare"
echo   groups:
echo     - name: "cloudflare"
echo       mode: "load_balance"
echo       timeout: "5s"
echo       enabled: true
echo       upstreams:
echo         - "1.1.1.1"
echo         - "1.0.0.1"
) > demo-basic.yaml

echo 配置文件已创建: demo-basic.yaml
echo.
echo 启动命令: dnsproxy.exe --config-path=demo-basic.yaml
echo.

REM 演示 2: 内外网分离
echo 📝 演示 2: 内外网分离配置
echo -----------------------------------
(
echo listen-addrs:
echo   - "127.0.0.1"
echo listen-ports:
echo   - 5353
echo cache: true
echo.
echo upstream-groups:
echo   default_group: "public"
echo   groups:
echo     - name: "public"
echo       upstreams:
echo         - "1.1.1.1"
echo         - "8.8.8.8"
echo     - name: "internal"
echo       upstreams:
echo         - "192.168.1.1"
echo   domain_groups:
echo     "internal.local": "internal"
echo     "corp.local": "internal"
) > demo-split.yaml

echo 配置文件已创建: demo-split.yaml
echo 说明: 内网域名使用内网 DNS，其他域名使用公网 DNS
echo.

REM 演示 3: 加密 DNS
echo 📝 演示 3: 加密 DNS 配置
echo -----------------------------------
(
echo listen-addrs:
echo   - "127.0.0.1"
echo listen-ports:
echo   - 5353
echo cache: true
echo.
echo upstream-groups:
echo   default_group: "standard"
echo   groups:
echo     - name: "standard"
echo       upstreams:
echo         - "1.1.1.1"
echo         - "8.8.8.8"
echo     - name: "encrypted"
echo       mode: "parallel"
echo       upstreams:
echo         - "tls://dns.adguard.com"
echo         - "https://dns.google/dns-query"
echo   domain_groups:
echo     "banking.com": "encrypted"
) > demo-encrypted.yaml

echo 配置文件已创建: demo-encrypted.yaml
echo 说明: 敏感域名使用加密 DNS
echo.

REM 演示 4: 文本格式
echo 📝 演示 4: 文本格式配置
echo -----------------------------------
(
echo # 主要组
echo [group:primary:load_balance:5s]
echo 1.1.1.1
echo 8.8.8.8
echo.
echo # 本地组
echo [group:local:load_balance:3s]
echo 192.168.1.1
echo.
echo # 域名映射
echo [/internal.local/]local
echo [default]primary
) > demo-groups.txt

echo 配置文件已创建: demo-groups.txt
echo.

REM 测试说明
echo 🧪 测试方法
echo -----------------------------------
echo 1. 启动服务:
echo    dnsproxy.exe --config-path=demo-basic.yaml --verbose
echo.
echo 2. 在另一个命令提示符测试:
echo    nslookup google.com 127.0.0.1
echo.

REM 清理说明
echo 🧹 清理
echo -----------------------------------
echo 删除演示配置文件:
echo    del demo-*.yaml demo-groups.txt
echo.

echo ==================================
echo 演示准备完成！
echo ==================================
echo.
echo 📚 更多信息:
echo   - 快速入门: UPSTREAM_GROUPS_QUICKSTART.md
echo   - 完整文档: UPSTREAM_GROUPS.md
echo   - 配置示例: config-groups.yaml.example
echo.
echo 🚀 开始使用:
echo   dnsproxy.exe --config-path=demo-basic.yaml --verbose
echo.

pause
