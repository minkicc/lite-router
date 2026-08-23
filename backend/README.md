# MKSwitch Backend

MKSwitch 的 Go 核心进程，负责 OpenAI 兼容代理、路由、健康检查和本地使用记录。

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

- `MKSWITCH_CONFIG_PATH`：配置文件路径
- `MKSWITCH_NO_BROWSER`：设为 `1` 时不自动打开管理页
- `MKSWITCH_LISTEN_ADDR`：覆盖监听地址
