@echo off
REM ============================================================
REM  RLdev2026-Go — Build Script (Windows)
REM  Compile les 4 outils depuis les sources Go
REM ============================================================

echo.
echo  RLdev2026-Go Build Script
echo  =========================
echo.

set OUTDIR=%~dp0bin
if not exist "%OUTDIR%" mkdir "%OUTDIR%"

echo [1/4] Building kprl16.exe...
cd /d "%~dp0kprl"
go build -o "%OUTDIR%\kprl16.exe" ./cmd/kprl/
if errorlevel 1 (echo FAILED & goto :error)
echo       OK

echo [2/4] Building rlc2026.exe...
cd /d "%~dp0rlc"
go build -o "%OUTDIR%\rlc2026.exe" ./cmd/rlc/
if errorlevel 1 (echo FAILED & goto :error)
echo       OK

echo [3/4] Building rlxml.exe...
cd /d "%~dp0rlxml"
go build -o "%OUTDIR%\rlxml.exe" ./cmd/rlxml/
if errorlevel 1 (echo FAILED & goto :error)
echo       OK

echo [4/4] Building vaconv.exe...
cd /d "%~dp0vaconv"
go build -o "%OUTDIR%\vaconv.exe" ./cmd/vaconv/
if errorlevel 1 (echo FAILED & goto :error)
echo       OK

echo.
echo  All tools built successfully in: %OUTDIR%
echo.
dir "%OUTDIR%\*.exe"
echo.
goto :end

:error
echo.
echo  Build FAILED. Make sure Go is installed (go.dev)
echo.

:end
cd /d "%~dp0"
pause
