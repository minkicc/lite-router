# MKRouter Backend

MKRouter 的 Go 核心进程，负责 OpenAI 兼容代理、路由、健康检查和本地使用记录。

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

构建产物输出到 `../src-tauri/binaries/`，并按 Tauri external binary 的 target triple 命名。

## 环境变量

- `MKROUTER_CONFIG_PATH`：配置文件路径
- `MKROUTER_NO_BROWSER`：设为 `1` 时不自动打开管理页
- `MKROUTER_LISTEN_ADDR`：覆盖监听地址

旧的 `MKSWITCH_*` 变量在 v1.1.2 中仍可作为兼容别名使用。
