# 🛡️ Anti-Censorship Speedtest Toolkit

A robust, cross-platform network diagnostic and speed test toolkit built with Go. Designed to bypass network restrictions using Cloudflare's edge network (TCP 443), providing both a terminal-based CLI for power users and a clean Desktop GUI for standard users.

## 🚀 Features
* **Dual Interface:** Comes with a fully functional TUI (Terminal User Interface) and a desktop GUI built with Fyne.
* **Anti-Censorship Core:** Uses HTTPS/TCP port 443 to bypass standard DPI (Deep Packet Inspection) and network throttling.
* **Homelab Ready:** (Coming Soon) Direct integration with local WireGuard VPN and DNS-level ad/tracker blocking.
* **Zero Dependencies:** Compiled as a single standalone `.exe` binary.

## 🛠️ Usage
Go to the **Releases** tab and download the latest version for your system.
* **GUI Version:** Double-click `NetworkToolkit.exe` to launch the desktop app.
* **CLI Version:** Run `speedtest-cli-windows.exe` via terminal for automated scripting.

## ⚙️ Build from Source
```bash
git clone [https://github.com/mustafainci067-cell/Anti-Censorship-Speedtest.git](https://github.com/mustafainci067-cell/Anti-Censorship-Speedtest.git)
cd Anti-Censorship-Speedtest
go mod tidy
go build -ldflags="-H windowsgui -s -w" -o NetworkToolkit.exe ./cmd/gui
