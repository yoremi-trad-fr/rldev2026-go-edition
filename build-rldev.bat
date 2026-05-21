@echo off
setlocal EnableExtensions

REM RLdev2026-Go command line tools build script.
REM Native Windows builds produce bin\kprl16.exe, bin\rlc2026.exe, bin\rlxml.exe and bin\vaconv.exe.

set "ROOT=%~dp0"
set "ROOT=%ROOT:~0,-1%"
set "OUTDIR=%OUTDIR%"
if "%OUTDIR%"=="" set "OUTDIR=%ROOT%\bin"
set "GOCACHE=%GOCACHE%"
if "%GOCACHE%"=="" set "GOCACHE=%ROOT%\.gocache"
set "GOTMPDIR=%GOTMPDIR%"
if "%GOTMPDIR%"=="" set "GOTMPDIR=%ROOT%\.gotmp"
set "GOOS=%GOOS%"
if "%GOOS%"=="" for /f "delims=" %%i in ('go env GOOS') do set "GOOS=%%i"
set "GOARCH=%GOARCH%"
if "%GOARCH%"=="" for /f "delims=" %%i in ('go env GOARCH') do set "GOARCH=%%i"
set "EXT="
if /I "%GOOS%"=="windows" set "EXT=.exe"
set "BUILD_STATUS=0"

where go >nul 2>nul
if errorlevel 1 (
    echo Go is required. Install Go 1.22 or newer, then run this script again.
    exit /b 1
)

if not exist "%OUTDIR%" mkdir "%OUTDIR%"
if not exist "%GOCACHE%" mkdir "%GOCACHE%"
if not exist "%GOTMPDIR%" mkdir "%GOTMPDIR%"

echo.
echo  RLdev2026-Go Build Script
echo  Target : %GOOS%/%GOARCH%
echo  Output : %OUTDIR%
for /f "delims=" %%i in ('go env GOVERSION') do echo  Go     : %%i
echo.

if /I "%GOOS%"=="windows" if /I "%GOARCH%"=="amd64" (
    echo [prep] Embedding Windows version resources...
    pushd "%ROOT%"
    go run .\kprl\internal\winresgen -root "%ROOT%"
    if errorlevel 1 (
        popd
        goto :error
    )
    popd
) else if /I "%GOOS%"=="windows" (
    echo [prep] Windows resources are currently generated for amd64 only; continuing without them for %GOARCH%.
)

call :build 1 kprl16 ".\kprl\cmd\kprl" || goto :error
call :build 2 rlc2026 ".\rlc\cmd\rlc" || goto :error
call :build 3 rlxml ".\rlxml\cmd\rlxml" || goto :error
call :build 4 vaconv ".\vaconv\cmd\vaconv" || goto :error

echo.
echo  All tools built successfully in: %OUTDIR%
echo.
dir "%OUTDIR%\*%EXT%"
echo.
goto :end

:build
echo [%~1/4] Building %~2%EXT%...
pushd "%ROOT%"
go build -trimpath -o "%OUTDIR%\%~2%EXT%" %~3
set "ERR=%ERRORLEVEL%"
popd
if not "%ERR%"=="0" (
    echo FAILED
    exit /b %ERR%
)
echo       OK
exit /b 0

:error
set "BUILD_STATUS=1"
echo.
echo  Build FAILED.
echo.

:end
exit /b %BUILD_STATUS%

