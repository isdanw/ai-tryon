@echo off
chcp 65001 >nul
setlocal EnableDelayedExpansion
cd /d "%~dp0"

echo ==========================================
echo   AI 换装后端 - 重新编译并重启
echo ==========================================
echo.

REM ---- 1. 关闭旧进程 ----
echo [1/4] 关闭占用 8000 端口的旧进程...
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
    echo       未发现占用 8000 端口的进程。
)
echo.

REM ---- 2. 编译 ----
echo [2/4] 正在编译 main.exe ...
if exist "main.exe~" del /f /q "main.exe~" >nul 2>&1
go build -o main.exe .
if errorlevel 1 (
    echo       [错误] 编译失败，请检查代码或 Go 环境。
    pause
    exit /b 1
)
echo       编译成功。
echo.

REM ---- 3. 启动 ----
echo [3/4] 启动服务（监听 8000 端口）...
start "AI-Tryon-Backend" cmd /k "main.exe"
echo.

REM ---- 4. 提示 ----
echo [4/4] 完成。请在弹出窗口查看服务日志。
echo ==========================================
timeout /t 2 >nul 2>&1 || ping -n 3 127.0.0.1 >nul
exit /b 0
