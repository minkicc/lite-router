# Lite Router Sidecar

Lite Router 的 Go 核心进程，负责 OpenAI 兼容代理、路由、健康检查和本地使用记录。

## 接口

- `/v1/chat/completions`
- `/v1/responses`
- `/v1/models`
- `/health`
- `/api/state`
- `/api/config`
- `/api/reload`
- `/api/check`
- `/api/usage`

## 构建

```powershell
./build.ps1
```

或：

```bash
./build.sh
```

构建产物输出到 `../src-tauri/binaries/`，并按 Tauri sidecar 的 target triple 命名。

## 环境变量

- `LITE_ROUTER_CONFIG_PATH`：配置文件路径
- `LITE_ROUTER_NO_BROWSER`：设为 `1` 时不自动打开管理页
- `LITE_ROUTER_LISTEN_ADDR`：覆盖监听地址
