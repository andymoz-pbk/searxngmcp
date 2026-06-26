@echo off
REM ============================================================
REM searxngmcp - Windows Service installer using NSSM
REM ============================================================
REM
REM This script:
REM   1. Checks for administrator privileges
REM   2. Downloads NSSM if not present
REM   3. Installs binary + run.bat to %ProgramFiles%\searxngmcp\
REM   4. Creates config at %ProgramData%\searxngmcp\config.json
REM   5. Registers Windows Service via NSSM
REM   6. Starts the service
REM
REM The service uses run.bat as its entry point. Edit env vars
REM in %ProgramFiles%\searxngmcp\run.bat and restart the service
REM to apply changes.
REM
REM Usage:
REM   install_service.bat           # install and start
REM   install_service.bat --remove  # remove the service
REM
REM ============================================================

setlocal enabledelayedexpansion

REM --- Check for admin privileges ---
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo Error: This script requires administrator privileges.
    echo Right-click and select "Run as administrator".
    exit /b 1
)

set "SCRIPT_DIR=%~dp0"
set "SCRIPT_DIR=%SCRIPT_DIR:~0,-1%"
set "INSTALL_DIR=%ProgramFiles%\searxngmcp"
set "CONFIG_DIR=%ProgramData%\searxngmcp"
set "SERVICE_NAME=searxngmcp"

REM --- Handle --remove flag ---
if "%~1"=="--remove" (
    echo Removing service %SERVICE_NAME%...
    nssm stop %SERVICE_NAME% >nul 2>&1
    nssm remove %SERVICE_NAME% confirm
    echo Service removed.
    echo Binary still at: %INSTALL_DIR%\searxngmcp.exe
    echo Config still at: %CONFIG_DIR%\config.json
    exit /b 0
)

REM --- Check for required files ---
if not exist "%SCRIPT_DIR%\searxngmcp.exe" (
    echo Error: searxngmcp.exe not found in %SCRIPT_DIR%
    echo Build it first:  go build -o searxngmcp.exe .
    exit /b 1
)
if not exist "%SCRIPT_DIR%\run.bat" (
    echo Error: run.bat not found in %SCRIPT_DIR%
    exit /b 1
)

REM --- Check for NSSM ---
set "NSSM_EXE="
where nssm >nul 2>&1
if %errorLevel% equ 0 (
    set "NSSM_EXE=nssm"
    echo NSSM found in PATH.
) else if exist "%SCRIPT_DIR%\nssm.exe" (
    set "NSSM_EXE=%SCRIPT_DIR%\nssm.exe"
    echo NSSM found in script directory.
) else (
    echo NSSM not found. Downloading...
    powershell -Command "try { Invoke-WebRequest -Uri 'https://nssm.cc/release/nssm-2.24.zip' -OutFile '%SCRIPT_DIR%\nssm.zip' } catch { exit 1 }"
    if %errorLevel% neq 0 (
        echo Error: Failed to download NSSM.
        echo Please download manually from https://nssm.cc/ and place nssm.exe in %SCRIPT_DIR%
        exit /b 1
    )
    powershell -Command "Expand-Archive -Path '%SCRIPT_DIR%\nssm.zip' -DestinationPath '%SCRIPT_DIR%\nssm-extract' -Force"
    copy /y "%SCRIPT_DIR%\nssm-extract\nssm-2.24\win64\nssm.exe" "%SCRIPT_DIR%\nssm.exe" >nul
    if not exist "%SCRIPT_DIR%\nssm.exe" (
        echo Error: Failed to extract NSSM.
        exit /b 1
    )
    set "NSSM_EXE=%SCRIPT_DIR%\nssm.exe"
    del /q "%SCRIPT_DIR%\nssm.zip" 2>nul
    rmdir /s /q "%SCRIPT_DIR%\nssm-extract" 2>nul
    echo NSSM downloaded to %SCRIPT_DIR%\nssm.exe
)

REM --- Create installation directories ---
echo.
echo Installing to %INSTALL_DIR%...
mkdir "%INSTALL_DIR%" 2>nul
mkdir "%CONFIG_DIR%" 2>nul

REM --- Copy files ---
copy /y "%SCRIPT_DIR%\searxngmcp.exe" "%INSTALL_DIR%\" >nul
copy /y "%SCRIPT_DIR%\run.bat" "%INSTALL_DIR%\" >nul
if exist "%SCRIPT_DIR%\config.example.json" (
    if not exist "%CONFIG_DIR%\config.json" (
        echo Creating config at %CONFIG_DIR%\config.json...
        copy /y "%SCRIPT_DIR%\config.example.json" "%CONFIG_DIR%\config.json" >nul
    ) else (
        echo Config already exists at %CONFIG_DIR%\config.json ^(not overwritten^).
    )
)

REM --- Stop existing service if running ---
"%NSSM_EXE%" stop %SERVICE_NAME% >nul 2>&1

REM --- Remove existing service if present ---
"%NSSM_EXE%" remove %SERVICE_NAME% confirm >nul 2>&1

REM --- Install service ---
echo Installing service...
"%NSSM_EXE%" install %SERVICE_NAME% "%INSTALL_DIR%\run.bat"
"%NSSM_EXE%" set %SERVICE_NAME% AppDirectory "%INSTALL_DIR%"
"%NSSM_EXE%" set %SERVICE_NAME% DisplayName "SearXNG MCP Server"
"%NSSM_EXE%" set %SERVICE_NAME% Description "MCP server wrapping SearXNG metasearch"
"%NSSM_EXE%" set %SERVICE_NAME% Start SERVICE_AUTO_START
"%NSSM_EXE%" set %SERVICE_NAME% AppStdout "%CONFIG_DIR%\searxngmcp.log"
"%NSSM_EXE%" set %SERVICE_NAME% AppStderr "%CONFIG_DIR%\searxngmcp.log"
"%NSSM_EXE%" set %SERVICE_NAME% AppRotateFiles 1
"%NSSM_EXE%" set %SERVICE_NAME% AppRotateBytes 10485760

REM --- Start service ---
echo Starting service...
"%NSSM_EXE%" start %SERVICE_NAME%

echo.
echo ===============================================================
echo  Installation complete.
echo ===============================================================
echo.
echo  Service name:  %SERVICE_NAME%
echo  Binary:        %INSTALL_DIR%\searxngmcp.exe
echo  Runner:        %INSTALL_DIR%\run.bat
echo  Config:        %CONFIG_DIR%\config.json
echo  Log file:      %CONFIG_DIR%\searxngmcp.log
echo.
echo  To manage the service:
echo    nssm edit %SERVICE_NAME%       (edit settings)
echo    nssm restart %SERVICE_NAME%     (restart)
echo    nssm stop %SERVICE_NAME%       (stop)
echo    sc query %SERVICE_NAME%         (check status)
echo.
echo  To edit environment variables:
echo    Edit %INSTALL_DIR%\run.bat
echo    Then:  nssm restart %SERVICE_NAME%
echo.
echo  To remove the service:
echo    install_service.bat --remove
echo.