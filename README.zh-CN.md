# Lite Router

[English](README.md) | 简体中文

Lite Router 是一个跨平台的本地 AI 模型路由桌面应用。Codex 或其他 OpenAI 兼容客户端只需连接一个固定的本地地址，Lite Router 会根据分组、优先级、模型映射和渠道健康状态选择上游渠道，并在请求失败时自动重试或切换渠道。

## 功能

- 图形化管理渠道、分组、优先级、模型映射和访问 Token
- 支持渠道健康检查、失败重试和自动切换
- 支持将客户端模型名映射为不同渠道的上游模型名
- 支持 `/v1/chat/completions`、`/v1/responses` 和 `/v1/models`
- 支持多个本地访问 Token，并持久化请求数与 Token 用量
- 本地保存使用记录，默认最多保留 500 条
- 可选择允许局域网设备访问
- 支持简体中文和英文
- 支持 Windows、macOS 和 Linux

## 工作方式

```text
Codex / OpenAI-compatible client
              |
              v
    http://127.0.0.1:8787/v1
              |
              v
         Lite Router
       /      |       \
  Channel A Channel B Channel C
```

Tauri 提供桌面界面和进程管理，Go 后端提供 OpenAI 兼容代理、路由、健康检查和本地使用记录。

## 安装

从 GitHub Releases 下载对应平台的安装包：

- Windows：`.exe` 或 `.msi`
- Windows 绿色版：`portable.zip`，解压后运行 `Lite Router.exe`
- macOS：`.dmg`
- Linux：`.AppImage`、`.deb` 或 `.rpm`

macOS 和 Windows 的公开构建默认不包含商业代码签名证书。系统可能在首次启动时显示安全提示。

绿色版不需要安装，也不会写入程序目录以外的安装信息，但未签名的可执行文件仍可能触发 Windows SmartScreen。下载后可使用 Release 中的 `.sha256` 文件校验完整性。

## 快速使用

1. 打开 Lite Router，在「渠道」页添加至少一个上游渠道。
2. 按需配置分组优先级、渠道优先级和模型映射。
3. 在「接入」页复制 Base URL；若未开启「无需 Token」，请生成一个访问 Token。
4. 将客户端的 Base URL 设置为 `http://127.0.0.1:8787/v1`，并填入访问 Token。

模型映射可以限定到某个渠道，也可以选择「所有」。选择「所有」时，只会匹配可用模型列表中包含该上游模型的渠道。

## 本地开发

环境要求：

- Node.js 20+
- Rust stable
- Go 1.23+
- 当前系统所需的 [Tauri v2 系统依赖](https://v2.tauri.app/start/prerequisites/)

安装依赖并启动开发版：

```bash
npm install
npm run dev
```

运行 Go 测试：

```bash
cd backend
go test ./...
```

构建当前平台安装包：

```bash
npm run build
```

完整构建后生成 Windows 绿色包：

```powershell
npm run portable:windows
```

构建脚本默认生成所有支持架构的 Go 后端。CI 也可以只生成指定架构：

```bash
npm run backend:build -- aarch64-apple-darwin
```

支持的 target：

- `x86_64-pc-windows-msvc`
- `x86_64-apple-darwin`
- `aarch64-apple-darwin`
- `x86_64-unknown-linux-gnu`
- `aarch64-unknown-linux-gnu`

## 数据与安全

桌面应用的数据保存在系统应用配置目录中。应用标识为 `cc.minki.literouter`；Windows 下通常位于：

```text
%APPDATA%\cc.minki.literouter\
```

`config.json` 包含渠道 API Key 和本地访问 Token，`usage.json` 包含本地使用记录。不要将这些文件提交到 Git 仓库或发送给他人。

从旧构建升级时，Lite Router 会自动将 `com.literouter.desktop` 中的现有数据迁移到新的应用目录。

Lite Router 默认仅监听 `127.0.0.1`。开启「局域网访问」后，请务必启用 Token 验证，并只在可信网络中使用。Lite Router 不会主动上传配置或使用记录；代理请求仍会发送到你配置的上游服务。

## 自动构建

GitHub Actions 会在推送、Pull Request 和手动触发时构建 Windows、macOS 与 Linux 安装包。推送形如 `v1.0` 的版本标签时，会自动创建 GitHub Release 并上传所有安装包。

```bash
git tag v1.0
git push origin v1.0
```

## License

[MIT](LICENSE)
