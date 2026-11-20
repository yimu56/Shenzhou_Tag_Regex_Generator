@echo off
chcp 65001 >nul
title 标签提取规则生成器 - 编译脚本

echo ============================================
echo   标签提取规则生成器 - 一键编译
echo ============================================
echo.

REM 版本号：发布新版本时修改此处
set "VERSION=2.3.1"
set "OUTPUT=shenzhou-tag-regex.exe"

echo [1/5] 检查 Go 环境...
where go >nul 2>nul
if errorlevel 1 (
    echo 错误：未检测到 Go，请先安装 Go 1.26 及以上版本。
    pause
    exit /b 1
)
go version

echo.
echo [2/5] 校验版本号一致性...
REM 从 app.manifest 读取主程序集版本，与脚本 VERSION 比对（manifest 为四段，末段固定 0）
findstr /i /r "version=.%VERSION%.0." app.manifest >nul 2>nul
if errorlevel 1 (
    echo 警告：app.manifest 的版本号与脚本 VERSION（%VERSION%）不一致！
    echo 请将 app.manifest 中 assemblyIdentity version 改为 %VERSION%.0 后再发布。
) else (
    echo 版本号一致：v%VERSION%
)

echo.
echo [3/5] 检查 manifest 资源文件...
if not exist "rsrc.syso" (
    echo 未找到 rsrc.syso，尝试从 app.manifest 生成...
    where rsrc >nul 2>nul
    if errorlevel 1 (
        echo 错误：缺少 rsrc.syso，且未检测到 rsrc 工具。
        echo 请先执行：go install github.com/akavel/rsrc@latest
        pause
        exit /b 1
    )
    rsrc -manifest app.manifest -arch amd64 -o rsrc.syso
    if errorlevel 1 (
        echo 错误：rsrc.syso 生成失败。
        pause
        exit /b 1
    )
)
echo rsrc.syso 已就绪。

echo.
echo [4/5] 运行单元测试...
set "CGO_ENABLED=0"
go test ./...
if errorlevel 1 (
    echo 错误：单元测试未通过，已停止编译。
    pause
    exit /b 1
)

echo.
echo [5/5] 编译程序（版本 %VERSION%）...
go build -trimpath -ldflags="-H windowsgui -s -w -X main.Version=%VERSION%" -o %OUTPUT% .
if errorlevel 1 (
    echo 错误：编译失败。
    pause
    exit /b 1
)

echo.
echo ============================================
echo 编译完成！
echo 产物：%OUTPUT%
echo 版本：v%VERSION%
echo ============================================
echo.
pause
