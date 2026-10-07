@echo off
rem Multi2FA TOTP Windows build.
rem   build.bat           build\windows\Multi2FA-TOTP-<ver>-windows-amd64\ (+ .zip), exe with icon and version info
rem   build.bat cli       build\multi2fa-cli.exe only (no C compiler needed)
rem   build.bat test      run unit tests
rem   build.bat clean     remove build output
setlocal

set NAME=Multi2FA TOTP
set EXE=Multi2FA-TOTP.exe
set /p VERSION=<VERSION
if "%BUILD%"=="" set BUILD=1
set PKG=.\cmd\multi2fa
set DIR=build\windows\Multi2FA-TOTP-%VERSION%-windows-amd64

where go >nul 2>nul
if errorlevel 1 (
    echo [!] Go is not installed or not in PATH: https://go.dev/dl/
    exit /b 1
)

if /i "%1"=="clean" (
    if exist build rmdir /s /q build
    del /q %PKG%\rsrc_windows_*.syso 2>nul
    echo cleaned
    exit /b 0
)

if /i "%1"=="cli" (
    set CGO_ENABLED=0
    go build -trimpath -ldflags "-s -w -X main.version=%VERSION%" -o build\multi2fa-cli.exe .\cmd\multi2fa-cli
    if errorlevel 1 exit /b 1
    echo Done: build\multi2fa-cli.exe
    exit /b 0
)

if /i "%1"=="test" (
    go test ./...
    exit /b %errorlevel%
)

where gcc >nul 2>nul
if errorlevel 1 (
    echo [!] gcc not found. Fyne needs a C compiler for OpenGL:
    echo     install MSYS2 ^(https://www.msys2.org^) and run: pacman -S mingw-w64-ucrt-x86_64-gcc
    echo     then add C:\msys64\ucrt64\bin to PATH.
    exit /b 1
)

echo Building %NAME% %VERSION%...
go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64 --out %PKG%\rsrc --manifest gui --icon assets\icon.ico --product-name "%NAME%" --file-description "%NAME%" --product-version %VERSION%.%BUILD% --file-version %VERSION%.%BUILD% --original-filename %EXE% --copyright "Copyright (c) 2026 keklick1337"
if errorlevel 1 exit /b 1

if exist "%DIR%" rmdir /s /q "%DIR%"
mkdir "%DIR%"
set CGO_ENABLED=1
go build -trimpath -ldflags "-s -w -H=windowsgui -X main.version=%VERSION%" -o "%DIR%\%EXE%" %PKG%
if errorlevel 1 exit /b 1
set CGO_ENABLED=0
go build -trimpath -ldflags "-s -w -X main.version=%VERSION%" -o "%DIR%\multi2fa-cli.exe" .\cmd\multi2fa-cli
if errorlevel 1 exit /b 1

copy /y assets\icon.ico "%DIR%\" >nul
copy /y assets\icon.png "%DIR%\" >nul
copy /y packaging\windows\README.txt "%DIR%\" >nul
powershell -NoProfile -Command "Compress-Archive -Force -Path '%DIR%' -DestinationPath '%DIR%.zip'"

echo.
echo Done: %DIR%\%EXE%
echo Tip: put ffmpeg.exe next to %EXE% to enable camera scanning.
endlocal
