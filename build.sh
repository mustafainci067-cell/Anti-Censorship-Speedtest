#!/bin/bash

# Create bin directory if it doesn't exist
mkdir -p bin

echo "Building CLI for Windows (amd64)..."
GOOS=windows GOARCH=amd64 go build -o bin/speedtest-cli-windows.exe ./main.go

echo "Building CLI for Linux (amd64)..."
GOOS=linux GOARCH=amd64 go build -o bin/speedtest-cli-linux ./main.go

echo "Building CLI for macOS (arm64)..."
GOOS=darwin GOARCH=arm64 go build -o bin/speedtest-cli-mac ./main.go

echo "Building GUI for Windows (amd64)..."
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 go build -o bin/network-toolkit-windows.exe ./cmd/gui/main.go || echo "Warning: GUI build for Windows failed. CGO (e.g., MinGW-w64 gcc) is required."

echo "Building GUI for Linux (amd64)..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -o bin/network-toolkit-linux ./cmd/gui/main.go || echo "Warning: GUI build for Linux failed. CGO and cross-compilers are required."

echo "Building GUI for macOS (arm64)..."
GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -o bin/network-toolkit-mac ./cmd/gui/main.go || echo "Warning: GUI build for macOS failed. CGO and cross-compilers are required."

echo "Build complete! Binaries are in the bin/ directory."
