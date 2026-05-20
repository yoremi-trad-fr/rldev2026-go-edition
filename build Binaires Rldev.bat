@echo off
setlocal EnableExtensions
REM ============================================================
REM  RLdev2026-Go — Build Script (Windows)
REM  Compile les 4 outils depuis les sources Go
REM ============================================================

echo.
echo  RLdev2026-Go Build Script
echo  =========================
echo.

set "ROOT=%~dp0"
set "OUTDIR=%ROOT%bin"
set "GOCACHE=%ROOT%.gocache"
set "GOTMPDIR=%ROOT%.gotmp"
set "BUILD_STATUS=0"

if not exist "%OUTDIR%" mkdir "%OUTDIR%"
if not exist "%GOCACHE%" mkdir "%GOCACHE%"
if not exist "%GOTMPDIR%" mkdir "%GOTMPDIR%"

echo  Cache Go : %GOCACHE%
echo  Temp Go  : %GOTMPDIR%
echo.

cd /d "%ROOT%"

call :build 1 kprl16.exe ".\kprl\cmd\kprl" || goto :error
call :build 2 rlc2026.exe ".\rlc\cmd\rlc" || goto :error
call :build 3 rlxml.exe ".\rlxml\cmd\rlxml" || goto :error
call :build 4 vaconv.exe ".\vaconv\cmd\vaconv" || goto :error

echo.
echo  All tools built successfully in: %OUTDIR%
echo.
dir "%OUTDIR%\*.exe"
echo.
goto :end

:build
echo [%~1/4] Building %~2...
go build -trimpath -ldflags "-buildid=" -o "%OUTDIR%\%~2" %~3
if errorlevel 1 (
    echo FAILED
    exit /b 1
)
echo       OK
exit /b 0

:error
set "BUILD_STATUS=1"
echo.
echo  Build FAILED. Make sure Go is installed (go.dev)
echo.

:end
cd /d "%ROOT%"
if /I not "%~1"=="--no-pause" pause
exit /b %BUILD_STATUS%
