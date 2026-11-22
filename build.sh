#!/usr/bin/env bash
set -euo pipefail

if ! command -v go >/dev/null 2>&1; then
  echo "Error: Go toolchain not found in PATH." >&2
  exit 1
fi

mkdir -p bin

echo "Syncing Go modules..."
go mod tidy

echo "Building CLI for Windows (amd64)..."
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o bin/speedtest-cli-windows.exe ./main.go

echo "Building CLI for Linux (amd64)..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/speedtest-cli-linux ./main.go

echo "Building CLI for macOS (arm64)..."
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o bin/speedtest-cli-mac ./main.go

HOST_OS="$(go env GOOS)"
if command -v gcc >/dev/null 2>&1; then
  echo "Building Desktop GUI for host OS (${HOST_OS})..."
  case "${HOST_OS}" in
    windows)
      CGO_ENABLED=1 go build -ldflags="-s -w -H=windowsgui" -o bin/NetworkToolkit.exe ./cmd/gui
      cp -f bin/NetworkToolkit.exe ./NetworkToolkit.exe
      ;;
    linux)
      CGO_ENABLED=1 go build -ldflags="-s -w" -o bin/network-toolkit-linux ./cmd/gui
      ;;
    darwin)
      CGO_ENABLED=1 go build -ldflags="-s -w" -o bin/network-toolkit-mac ./cmd/gui
      ;;
    *)
      echo "Skipping GUI build for unsupported host OS: ${HOST_OS}"
      ;;
  esac
else
  echo "Warning: gcc not found. Skipping Desktop GUI build (CGO is required for Fyne)."
fi

echo "Build complete. Output files are in bin/."
