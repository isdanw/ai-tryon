@echo off
chcp 65001 >nul
setlocal EnableDelayedExpansion
cd /d "%~dp0"

echo ==========================================
echo   AI 换装后端一键重启
echo ==========================================
echo.

REM ---- 1. 关闭占用 8000 端口的旧进程 ----
echo [1/3] 正在查找并关闭占用 8000 端口的旧进程...
set "FOUND="
for /f "tokens=5" %%a in ('netstat -ano ^| findstr ":8000" ^| findstr "LISTENING"') do (
    if not "!PID_%%a!"=="1" (
        set "PID_%%a=1"
        set "FOUND=1"
        taskkill /F /PID %%a >nul 2>&1
        echo       已结束进程 PID %%a
    )
)
if not defined FOUND (
    echo       未发现占用 8000 端口的进程，直接启动。
)
echo.

REM ---- 2. 每次都重新编译，避免源码已更新但仍启动旧 main.exe ----
echo [2/3] 正在重新编译 main.exe...
if exist "main.exe~" del /f /q "main.exe~" >nul 2>&1
go build -o main.exe .
if errorlevel 1 (
    echo       [错误] 编译失败，请检查 Go 环境和上方错误信息。
    pause
    exit /b 1
)
echo       编译完成。
echo


REM ---- 3. 启动服务 ----
echo [3/3] 正在启动后端服务（监听 8000 端口）...
echo.
echo ==========================================
start "AI-Tryon-Backend" cmd /k "main.exe"
echo   已在独立窗口启动，请查看新弹出的窗口。
echo ==========================================
echo.
timeout /t 2 >nul 2>&1 || ping -n 3 127.0.0.1 >nul
exit /b 0
