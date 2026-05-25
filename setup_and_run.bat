@echo off
echo ===================================================
echo   LeGaJ Automatic Setup ^& Launch Wizard
echo ===================================================
echo.

:: Detect Python executable
set PYTHON_EXE=python
where python >nul 2>nul
if %errorlevel% neq 0 (
    echo [INFO] Python not found in system PATH. Checking standard local directories...
    if exist "%LOCALAPPDATA%\Programs\Python\Python312\python.exe" (
        set PYTHON_EXE="%LOCALAPPDATA%\Programs\Python\Python312\python.exe"
    ) else if exist "%LOCALAPPDATA%\Programs\Python\Python311\python.exe" (
        set PYTHON_EXE="%LOCALAPPDATA%\Programs\Python\Python311\python.exe"
    ) else if exist "%LOCALAPPDATA%\Programs\Python\Python310\python.exe" (
        set PYTHON_EXE="%LOCALAPPDATA%\Programs\Python\Python310\python.exe"
    ) else (
        echo [ERROR] Python installation not detected!
        echo Please ensure Python 3.10+ is installed on your computer.
        pause
        exit /b 1
    )
)

echo Using Python executable: %PYTHON_EXE%
echo.
echo Step 1: Installing Python library requirements...
%PYTHON_EXE% -m pip install --upgrade pip
%PYTHON_EXE% -m pip install -r requirements.txt
if %errorlevel% neq 0 (
    echo.
    echo [ERROR] Failed to install Python requirements.
    echo Please make sure Python 3.10+ is installed and pip is functioning.
    pause
    exit /b %errorlevel%
)
echo.
echo Step 2: Requirements installed successfully!
echo.
echo Step 3: Launching LeGaJ GUI...
start "" legaj.exe
echo.
echo Done! LeGaJ is running. You can close this window.
timeout /t 5
