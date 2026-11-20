@echo off
chcp 65001 >nul
rem ==========================================================================
rem  轻码 IDE（lightcode）Windows 编译脚本
rem
rem    build.bat            发布构建：无控制台、去符号，输出 bin\lightcode.exe
rem    build.bat --debug    调试构建：保留控制台与符号，方便看日志
rem    build.bat --test     构建前先跑 go vet 与 go test
rem    build.bat --run      构建成功后直接启动
rem
rem  依赖：Go 1.27+（在 PATH 里）、本地 MyGo 检出（默认 ..\mygo，
rem  也可以用环境变量 MYGO_HOME 指过去）。
rem
rem  lightcode\go.mod 目前指向上游 mygo v0.7.4，而应用用到的
rem  ui.Theme.InactiveAccent / editor.State.PopoverMenu 只在本地框架里，
rem  所以第一次运行会在 lightcode\go.work 里把本地 mygo 挂上（不会覆盖已有的）。
rem
rem  终端需要 ghostty-vt.dll：脚本会把本机 mygo 缓存里的那份复制到 exe 旁边；
rem  找不到时不影响编译，程序首次使用终端会自己下载。
rem ==========================================================================
setlocal EnableExtensions EnableDelayedExpansion

set "ROOT=%~dp0"
set "APP=%ROOT%lightcode"
set "OUT=%ROOT%bin"
set "EXE=%OUT%\lightcode.exe"
set "MYGO_DIR=%ROOT%..\mygo"
set "DEBUG="
set "TEST="
set "RUN="

:args
if "%~1"=="" goto build
if /i "%~1"=="--debug" goto argDebug
if /i "%~1"=="-d" goto argDebug
if /i "%~1"=="--test" goto argTest
if /i "%~1"=="-t" goto argTest
if /i "%~1"=="--run" goto argRun
if /i "%~1"=="-r" goto argRun
if /i "%~1"=="--help" goto usage
if /i "%~1"=="-h" goto usage
echo [错误] 不认识的参数：%~1
echo.
call :usage
exit /b 1

:argDebug
set "DEBUG=1"
shift
goto args

:argTest
set "TEST=1"
shift
goto args

:argRun
set "RUN=1"
shift
goto args

:usage
echo 用法：build.bat [--debug] [--test] [--run]
echo.
echo   --debug, -d   保留控制台与符号（调试用，会多一个黑色控制台窗口）
echo   --test,  -t   构建前跑 go vet 与 go test
echo   --run,   -r   构建成功后启动 bin\lightcode.exe
echo.
echo 产物：bin\lightcode.exe（发布版），bin\ghostty-vt.dll（终端用原生库）
exit /b 0

:build
where go >nul 2>nul
if errorlevel 1 (
  echo [错误] 找不到 go：请先安装 Go 1.27+ 并加入 PATH
  exit /b 1
)
if not exist "%APP%\go.mod" (
  echo [错误] %APP% 下没有 go.mod
  exit /b 1
)

rem ---- 本地框架：没有 go.work 时把它挂上 -------------------------------------
if not exist "%MYGO_DIR%\go.mod" (
  if defined MYGO_HOME set "MYGO_DIR=%MYGO_HOME%"
)
if not exist "%APP%\go.work" (
  if not exist "%MYGO_DIR%\go.mod" (
    echo [错误] 找不到本地 MyGo 检出：%MYGO_DIR%
    echo        把它放在 ..\mygo，或者设置 MYGO_HOME 指向它
    exit /b 1
  )
  for %%I in ("%MYGO_DIR%") do set "MYGO_ABS=%%~fI"
  rem 默认的同级检出记相对路径，仓库搬动或重新克隆后仍然有效
  set "MYGO_USE=!MYGO_ABS!"
  if /i "%MYGO_DIR%"=="%ROOT%..\mygo" set "MYGO_USE=..\..\mygo"
  pushd "%APP%"
  go work init . "!MYGO_USE!" >nul
  if errorlevel 1 (
    popd
    echo [错误] go work init 失败，请检查 !MYGO_ABS!
    exit /b 1
  )
  popd
  echo [信息] 已创建 lightcode\go.work，指向 !MYGO_ABS!
)

pushd "%APP%"

rem ---- 可选：静态检查与测试 ------------------------------------------------
if defined TEST (
  echo [步骤] go vet ./...
  go vet ./...
  if errorlevel 1 (
    popd
    echo [错误] go vet 没通过
    exit /b 1
  )
  echo [步骤] go test ./...
  go test ./...
  if errorlevel 1 (
    popd
    echo [错误] go test 没通过
    exit /b 1
  )
)

rem ---- 编译 ------------------------------------------------------------------
if not exist "%OUT%" mkdir "%OUT%"
rem 调试版保留符号；发布版去符号，并且不要控制台窗口
set "LDFLAGS="
if not defined DEBUG set "LDFLAGS=-s -w -H=windowsgui"
echo [步骤] go build ./cmd/lightcode
go build -trimpath -ldflags "!LDFLAGS!" -o "%EXE%" ./cmd/lightcode
if errorlevel 1 (
  popd
  echo [错误] 编译失败：上面是 go build 的报错（如果 lightcode 正在运行，先关掉再编译）
  exit /b 1
)
popd

rem ---- 终端原生库 ------------------------------------------------------------
set "DLL="
if defined MYGO_GHOSTTY_VT if exist "%MYGO_GHOSTTY_VT%" set "DLL=%MYGO_GHOSTTY_VT%"
if not defined DLL (
  for /f "delims=" %%F in ('where /r "%LOCALAPPDATA%\mygo\natives" ghostty-vt.dll 2^>nul') do (
    if not defined DLL set "DLL=%%F"
  )
)
if defined DLL (
  copy /y "!DLL!" "%OUT%\ghostty-vt.dll" >nul
  if errorlevel 1 (
    echo [警告] 复制 ghostty-vt.dll 失败；终端首次使用时会自行下载
  ) else (
    echo [信息] 已复制 ghostty-vt.dll，终端开箱可用
  )
) else (
  echo [警告] 本机没有缓存的 ghostty-vt.dll；终端首次使用时会自行下载
)

rem ---- 结果 ------------------------------------------------------------------
for %%F in ("%EXE%") do set "SIZE=%%~zF"
set /a SIZEKB=!SIZE! / 1024
echo [完成] %EXE%
echo         !SIZE! 字节（约 !SIZEKB! KB）
if defined RUN (
  echo [步骤] 启动 %EXE%
  start "" "%EXE%"
)
exit /b 0
