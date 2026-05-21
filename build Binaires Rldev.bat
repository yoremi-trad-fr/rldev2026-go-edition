@echo off
setlocal EnableExtensions

set "ROOT=%~dp0"
call "%ROOT%build-rldev.bat"
set "BUILD_STATUS=%ERRORLEVEL%"

if /I not "%~1"=="--no-pause" pause
exit /b %BUILD_STATUS%
