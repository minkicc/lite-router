# MKSwitch

English | [简体中文](README.zh-CN.md)

MKSwitch is a cross-platform desktop application for routing local AI model requests. Codex and other OpenAI-compatible clients connect to one stable local endpoint while MKSwitch selects an upstream channel based on groups, priorities, model mappings, and channel health. Failed requests can be retried or routed to another available channel automatically.

## Features

- Graphical management for channels, groups, priorities, model mappings, and access tokens
- Channels can use an API key or imported Codex `auth.json` authorization with automatic OAuth token refresh
- Channel health checks, retries, and automatic failover
- Client model names can be mapped to different upstream model names
- OpenAI-compatible `/v1/chat/completions`, `/v1/responses`, and `/v1/models` endpoints
- Multiple local access tokens with persistent request and token usage counters
- Local usage history with a default limit of 500 records
- Optional access from other devices on the local network
- Simplified Chinese and English interface
- Windows, macOS, and Linux support

## How It Works

```text
Codex / OpenAI-compatible client
              |
              v
    http://127.0.0.1:8787/v1
              |
              v
         MKSwitch
       /      |       \
  Channel A Channel B Channel C
```

Tauri provides the desktop interface and process management. The Go backend provides the OpenAI-compatible proxy, routing, health checks, and local usage history.

## Installation

Download the package for your platform from GitHub Releases:

- Windows: `.exe` or `.msi`
- Windows portable edition: extract `portable.zip` and run `MKSwitch.exe`
- macOS: `.dmg`
- Linux: `.AppImage`, `.deb`, or `.rpm`

Public macOS and Windows builds are not commercially code-signed by default. Your operating system may display a security warning the first time you launch the application.

The Windows portable edition does not require installation, but unsigned executables may still trigger Microsoft Defender SmartScreen. Verify the download with the `.sha256` file included in the release.

## Quick Start

1. Open MKSwitch and add at least one upstream channel on the **Channels** tab.
2. Configure group priorities, channel priorities, and model mappings as needed.
3. Copy the Base URL from the **Connect** tab. Generate an access token unless **No Token Required** is enabled.
4. Set the client Base URL to `http://127.0.0.1:8787/v1` and provide the generated access token.

A model mapping can target one channel or **All** channels. When **All** is selected, only channels whose available-model list contains the mapped upstream model are eligible.

## Development

Requirements:

- Node.js 20+
- Rust stable
- Go 1.23+
- The [Tauri v2 prerequisites](https://v2.tauri.app/start/prerequisites/) for your operating system

Install dependencies and start the development application:

```bash
npm install
npm run dev
```

Run the Go test suite:

```bash
cd backend
go test ./...
```

Build packages for the current platform:

```bash
npm run build
```

Create the Windows portable package after a full build:

```powershell
npm run portable:windows
```

The backend build script creates binaries for all supported architectures by default. CI can request one target explicitly:

```bash
npm run backend:build -- aarch64-apple-darwin
```

Supported targets:

- `x86_64-pc-windows-msvc`
- `x86_64-apple-darwin`
- `aarch64-apple-darwin`
- `x86_64-unknown-linux-gnu`
- `aarch64-unknown-linux-gnu`

## Data and Security

Desktop application data is stored in the system application configuration directory under the identifier `cc.minki.mkswitch`. On Windows, it is usually located at:

```text
%APPDATA%\cc.minki.mkswitch\
```

`config.json` contains upstream API keys, Codex OAuth authorization, and local access tokens. `usage.json` contains local usage records. Do not commit or share these files.

MKSwitch listens on `127.0.0.1` by default. When LAN access is enabled, keep token authentication enabled and only use the application on trusted networks. MKSwitch does not upload configuration or usage records by itself; proxied requests are still sent to the upstream services you configure.

## Automated Builds

GitHub Actions builds Windows, macOS, and Linux packages on pushes, pull requests, and manual runs. Pushing a version tag such as `v1.0` creates a GitHub Release and uploads all generated packages automatically.

```bash
git tag v1.0
git push origin v1.0
```

## License

[MIT](LICENSE)
