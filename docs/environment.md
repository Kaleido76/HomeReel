# environment.md — 开发环境与部署

> 本机开发环境配置、FFmpeg 安装、部署形态。

## 1. 工具矩阵

| 工具 | 用途 | 本机状态 |
|---|---|---|
| Go 1.26 | 后端编译（`CGO_ENABLED=0`） | ✅ |
| Node.js LTS | 前端运行 | ✅ |
| pnpm 11.x | 前端包管理 | ✅ |
| Git | 版本管理 | ✅ |
| FFmpeg / ffprobe | 媒体探测、缩略图、字幕、格式工厂 | ✅（已入 PATH） |

## 2. FFmpeg 安装

**推荐方式**：`winget install Gyan.FFmpeg --source winget`（装后需新开终端生效）

**备选方式**：将 `ffmpeg.exe` / `ffprobe.exe` 放入项目 `bin/`，在 `config.yaml` 指定路径。

**PATH 陷阱**：winget 安装后需新开终端 PATH 才生效。服务启动时统一解析，**缺失启动即报错**。
若 PATH 缺失，在 `config.yaml` 显式指定绝对路径（YAML 双引号内用 `\\` 转义）。

## 3. 包管理器

**一律使用 `pnpm`**；**禁止** `npm` / `cnpm` / `yarn`。

## 4. 部署形态

- 单二进制：`CGO_ENABLED=0 go build -o server.exe ./cmd/server`
- 产物：`server.exe` + `config.yaml` + `data/` 目录 + ffmpeg 二进制
- 前端：`pnpm run build` → `frontend/dist/` 目录
- 静态托管：`config.server.static_dir` 指定前端构建产物目录；空则自动探测 `./static` 或 `../frontend/dist`
- SPA 模式：未命中 `/api` 的 GET 从静态目录出文件，无扩展名路径回退 `index.html`
- 启动输出：`cmd/server` 显式 `net.Listen` 后打印可达地址；端口被占用则启动即失败
