# NetProbe // Anti-Censorship Speedtest Toolkit

A local-first, cross-platform network diagnostic and speedtest toolkit built in Go. Engineered to operate reliably behind restrictive firewalls, campus/corporate NATs, and DNS-filtered networks using encrypted DNS-over-HTTPS (DoH), direct Cloudflare Anycast edge routing, and TCP/443 probing.

## Project Structure

```text
├── cmd/
│   ├── cli/                     # Terminal CLI application (Cobra + Bubble Tea)
│   │   ├── main.go
│   │   └── root.go
│   └── gui/                     # Desktop GUI application (Fyne)
│       ├── fyne_metadata_init.go
│       ├── icon.png
│       ├── main.go
│       └── nvidia_windows.go    # Windows NVIDIA overlay DLL injection mitigation
├── internal/
│   └── network/                 # Anti-censorship transport & benchmark engine
│       ├── bandwidth.go         # Multi-stream time-boxed TLS throughput engine
│       ├── bypass.go            # Encrypted DoH resolver & direct edge IP fallback
│       └── ping.go              # Concurrent 4-node TCP/443 latency & jitter prober
├── scripts/
│   ├── build.sh                 # Cross-platform Bash build script
│   └── setup.ps1                # Windows PowerShell setup & build script
├── bin/                         # Compiled output binaries (gitignored)
├── go.mod
├── go.sum
└── README.md
```

## Key Features
* **Anti-Censorship Transport Engine:** Resolves endpoints over encrypted DoH (`1.1.1.1` / `1.0.0.1` / `8.8.8.8`) directly by IP with automatic fallback to verified Cloudflare Anycast edge IPs over `TLS 1.3` (`TCP/443`).
* **Multi-Stream Bandwidth Benchmark:** Time-boxed parallel TLS streams (`4x` download, `3x` upload) measuring sustained Mbps, peak Mbps, and transferred volume without timing out on slow links.
* **Backbone Reachability Matrix:** Concurrent `TCP/443` (with `TCP/53` fallback) latency, jitter, and packet-loss probing across Cloudflare (`1.1.1.1`), Google (`8.8.8.8`), Quad9 (`9.9.9.9`), and Cisco OpenDNS (`208.67.222.222`) to bypass ICMP blocks.
* **Dual Interface (GUI + CLI):** Native dark-themed Desktop GUI (`bin/NetworkToolkit.exe`) with Windows process mitigation policies to prevent game overlay (e.g., NVIDIA ShadowPlay) injection, plus an interactive terminal CLI with JSON automation support.

## Usage

### Desktop GUI
Run `bin\NetworkToolkit.exe` to launch the desktop diagnostic dashboard. Select your preferred routing mode (`Auto Bypass`, `Force DoH`, `Direct Edge IP`, or `System DNS`) and click **Start Diagnostic**.

### Terminal CLI
```powershell
# Interactive TUI
.\bin\speedtest-cli-windows.exe

# Force DoH bypass and output JSON for scripting
.\bin\speedtest-cli-windows.exe --mode doh --json

# Single run with custom stream count and phase duration
.\bin\speedtest-cli-windows.exe --once --streams 4 --duration 6
```

## Build & Setup from Source

```bash
git clone https://github.com/mustafainci067-cell/Anti-Censorship-Speedtest.git
cd Anti-Censorship-Speedtest
```

### Windows (PowerShell)
```powershell
.\scripts\setup.ps1
```

### Linux / macOS (Bash)
```bash
chmod +x scripts/build.sh
./scripts/build.sh
```
