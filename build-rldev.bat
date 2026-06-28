@echo off
setlocal EnableExtensions

REM RLdev2026-Go command line tools build script.
REM Native Windows builds produce bin\kprl16.exe, bin\rlc2026.exe, bin\rlxml.exe, bin\vaconv.exe and bin\rlsave.exe.
REM Windows builds also produce bin\reallive-debug-launcher-v2.exe as a 32-bit helper for RealLive.

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
call :build 5 rlsave ".\kprl\cmd\rlsave" || goto :error
if /I "%GOOS%"=="windows" if exist "%ROOT%\kprl\cmd\reallive-debug-launcher" (
    call :build_debug 6 reallive-debug-launcher-v2 ".\kprl\cmd\reallive-debug-launcher" || goto :error
) else if /I "%GOOS%"=="windows" (
    echo [6/6] Skipping reallive-debug-launcher-v2; source directory not present.
) else (
    echo [6/6] Skipping reallive-debug-launcher-v2; Windows-only helper.
)

echo.
echo  All tools built successfully in: %OUTDIR%
echo.
dir "%OUTDIR%\*%EXT%"
echo.
goto :end

:build
echo [%~1/6] Building %~2%EXT%...
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

:build_debug
echo [%~1/6] Building %~2.exe for windows/386...
pushd "%ROOT%"
set "OLD_GOOS=%GOOS%"
set "OLD_GOARCH=%GOARCH%"
set "GOOS=windows"
set "GOARCH=386"
go build -trimpath -o "%OUTDIR%\%~2.exe" %~3
set "ERR=%ERRORLEVEL%"
set "GOOS=%OLD_GOOS%"
set "GOARCH=%OLD_GOARCH%"
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
