@echo off
chcp 65001 >nul
title WhatsApp Operations Platform

echo.
echo ==========================================
echo  WhatsApp Operations Platform (whatsapp-ops)
echo ==========================================
echo.

REM 检查 Go 环境
where go >nul 2>nul
if %errorlevel% neq 0 (
    echo [错误] 未检测到 Go 环境，请先安装 Go 1.22+
    echo 下载地址: https://go.dev/dl/
    pause
    exit /b 1
)

echo [信息] 检测到 Go 环境:
go version

echo.
echo [信息] 正在构建单文件二进制...
go run build.go

if %errorlevel% neq 0 (
    echo.
    echo [错误] 构建失败，请检查错误信息
    pause
    exit /b 1
)

echo.
echo [信息] 构建成功！输出文件: bin\whatsapp-ops.exe
echo.
echo ==========================================
echo  启动方式:
echo  1. 直接双击 bin\whatsapp-ops.exe 运行
echo  2. 或在浏览器打开 http://localhost:8080
echo  3. 或直接双击 web/index.html (纯前端模式，需先启动后端)
echo ==========================================
echo.
echo [信息] 启动服务中... (按 Ctrl+C 停止)
echo.

cd /d "%~dp0"
bin\whatsapp-ops.exe
