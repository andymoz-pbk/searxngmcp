@echo off
REM ============================================================
REM searxngmcp - Windows runner script (manual or service)
REM ============================================================
REM
REM Config file is OPTIONAL. If env vars are set below, no
REM config.json is needed. Env vars override all config file
REM values.
REM
REM Config search order:
REM   1. --config flag (if provided)
REM   2. .\config.json (current directory)
REM   3. %ProgramData%\searxngmcp\config.json (system-wide)
REM
REM ----------------------------------------------------------------
REM Environment variables (uncomment and edit as needed):
REM ----------------------------------------------------------------

REM set SEARXNGMCP_SEARXNG_BASE_URL=http://localhost:8080
REM set SEARXNGMCP_SEARXNG_TIMEOUT=30
REM set SEARXNGMCP_SERVER_HOST=0.0.0.0
REM set SEARXNGMCP_SERVER_PORT=8000
REM set SEARXNGMCP_SEARCH_DEFAULT_MAX_RESULTS=10
REM set SEARXNGMCP_SEARCH_MAX_MAX_RESULTS=50
REM set SEARXNGMCP_SEARCH_DEFAULT_CATEGORIES=general
REM set SEARXNGMCP_SEARCH_DEFAULT_LANGUAGE=
REM set SEARXNGMCP_SEARCH_DEFAULT_SAFESEARCH=0
REM set SEARXNGMCP_FETCH_MAX_CONTENT_LENGTH=1048576
REM set SEARXNGMCP_FETCH_TIMEOUT=30
REM set SEARXNGMCP_FETCH_USER_AGENT=Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36
REM set SEARXNGMCP_FETCH_MAX_CONCURRENT=5
REM set SEARXNGMCP_LOGGING_LEVEL=info

REM ----------------------------------------------------------------

if not exist searxngmcp.exe (
    echo Error: searxngmcp.exe not found in current directory.
    echo Build it:  go build -o searxngmcp.exe .
    echo Or download a release from the project page.
    exit /b 1
)

REM Parse --config flag (pass through to binary)
set CONFIG_FLAG=
if "%~1"=="--config" (
    set CONFIG_FLAG=--config %~2
)

searxngmcp.exe %CONFIG_FLAG%