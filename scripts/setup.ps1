$ErrorActionPreference = "Stop"

$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Push-Location $RepoRoot

try {
    Write-Host "=== NetProbe // Windows Setup & Build ===" -ForegroundColor Cyan

    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Error "Go toolchain not found in PATH. Install Go from https://go.dev/dl/ and retry."
        exit 1
    }

    Write-Host "Go version: $(go version)" -ForegroundColor Gray

    if (-not (Test-Path "bin")) {
        New-Item -ItemType Directory -Path "bin" | Out-Null
    }

    Write-Host "[1/3] Syncing Go dependencies..." -ForegroundColor Yellow
    go mod tidy

    Write-Host "[2/3] Building Windows CLI (bin/speedtest-cli-windows.exe)..." -ForegroundColor Yellow
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    go build -ldflags="-s -w" -o bin/speedtest-cli-windows.exe ./cmd/cli

    Write-Host "[3/3] Building Windows Desktop GUI (bin/NetworkToolkit.exe)..." -ForegroundColor Yellow
    if (Get-Command gcc -ErrorAction SilentlyContinue) {
        $env:CGO_ENABLED = "1"
        go build -ldflags="-s -w -H=windowsgui" -o bin/NetworkToolkit.exe ./cmd/gui
        Write-Host "GUI built successfully: bin/NetworkToolkit.exe" -ForegroundColor Green
    } else {
        Write-Warning "MinGW-w64 gcc not found in PATH. Skipped GUI build (CLI binary is ready)."
    }

    Write-Host "Setup & build complete. Binaries are in bin/." -ForegroundColor Green
} finally {
    Pop-Location
}
