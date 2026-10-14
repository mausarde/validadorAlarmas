@echo off
setlocal EnableExtensions

:: ---------- Configuración ----------
set PROJECT_NAME=ValidadorAlarmas
set BIN_NAME=ValidadorAlarmas
set DIST_DIR=dist
set BUILD_DIR=%DIST_DIR%\%PROJECT_NAME%
set MAIN_PKG=.\cmd\validadoralarmas

:: Trabajar siempre desde la carpeta del script (raíz del proyecto)
cd /d "%~dp0"

:: ---------- Metadatos de versión (con valores por defecto si no hay Git) ----------
set VERSION=dev
set COMMIT=desconocido
set DATE=
set BUILT_BY=%USERNAME%
for /f "delims=" %%i in ('git describe --tags --always --dirty 2^>nul') do set VERSION=%%i
for /f "delims=" %%i in ('git rev-parse --short HEAD 2^>nul') do set COMMIT=%%i
for /f "delims=" %%i in ('powershell -NoProfile -Command "Get-Date -Format yyyy-MM-ddTHH:mm:ss"') do set DATE=%%i
for /f "delims=" %%i in ('whoami') do set BUILT_BY=%%i
set BUILT_BY=%BUILT_BY: =_%:

:: ---------- Dependencias y pruebas (se ejecutan con el SO local, antes de fijar GOOS=linux) ----------
echo [1/4] Resolviendo dependencias...
go mod tidy || goto :error

echo [2/4] Ejecutando go vet y pruebas...
go vet ./... || goto :error
go test ./... || goto :error

:: ---------- Preparar directorio de salida ----------
if exist "%BUILD_DIR%" rmdir /s /q "%BUILD_DIR%"
mkdir "%BUILD_DIR%\bin"
mkdir "%BUILD_DIR%\etc"
mkdir "%BUILD_DIR%\log"
mkdir "%BUILD_DIR%\reports"

:: ---------- Compilar para Linux amd64 (binario estático) ----------
echo [3/4] Compilando %BIN_NAME% %VERSION% para linux/amd64...
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=amd64
go clean
go build -trimpath -ldflags "-s -w -X 'main.version=%VERSION%' -X 'main.commit=%COMMIT%' -X 'main.date=%DATE%' -X 'main.builtBy=%BUILT_BY%'" -o "%BUILD_DIR%\bin\%BIN_NAME%" %MAIN_PKG% || goto :error
go build -trimpath -ldflags "-s -w -X main.version=%VERSION% -X main.commit=%COMMIT% -X main.date=%DATE% -X main.builtBy=%BUILT_BY%" -o "%BUILD_DIR%\bin\%BIN_NAME%" %MAIN_PKG% || goto :error

:: ---------- Archivos adicionales ----------
copy /y etc\validador.json "%BUILD_DIR%\etc\" >nul
copy /y etc\loader.example.json "%BUILD_DIR%\etc\" >nul
if exist README.md copy /y README.md "%BUILD_DIR%\" >nul

:: ---------- Empaquetar ----------
echo [4/4] Empaquetando...
if exist "%DIST_DIR%\%PROJECT_NAME%.zip" del "%DIST_DIR%\%PROJECT_NAME%.zip"
powershell -NoProfile -Command "Compress-Archive -Path '%BUILD_DIR%\*' -DestinationPath '%DIST_DIR%\%PROJECT_NAME%.zip'" || goto :error

echo.
echo Build completado con la version: %VERSION% (commit %COMMIT%)
echo Binario: %BUILD_DIR%\bin\%BIN_NAME%
echo Paquete: %DIST_DIR%\%PROJECT_NAME%.zip
endlocal
exit /b 0

:error
echo.
echo ERROR: el build fallo (codigo %errorlevel%).
endlocal
exit /b 1