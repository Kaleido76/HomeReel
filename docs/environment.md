# environment.md — 开发环境要求

> 本文件记录本机开发环境的具体版本与安装状态，以及部署形态 / 静态托管 / 启动输出的约定，属环境级知识。

## 1. 工具矩阵（当前本机状态）

| 工具 | 用途 | 本机状态 |
|---|---|---|
| Go 1.26 | 后端编译（`CGO_ENABLED=0`） | ✅ 已装（`C:\Program Files\Go`） |
| Node.js LTS | 前端运行 | ✅ 已装 |
| pnpm 11.x | 前端包管理（registry 为 npmmirror） | ✅ 已装 |
| Git | 版本管理 | ✅ 已装 |
| winget | 软件安装（备选 choco/scoop 未装） | ✅ 已装 |
| **FFmpeg / ffprobe** | 媒体探测、缩略图、字幕、格式工厂转换；integration 测试依赖 | ✅ 已装（winget 安装 Gyan.FFmpeg，已入 PATH） |

本机 `go env`：`GOOS=windows GOARCH=amd64 CGO_ENABLED=0`，符合「免 CGO 单 exe」要求。

## 2. FFmpeg 安装

方式一（推荐）：`winget install Gyan.FFmpeg --source winget`（装后写入 PATH，需新开终端生效）。
方式二：将 `ffmpeg.exe` / `ffprobe.exe` 放入项目 `bin/`，并在 `config.yaml` 的
`media.ffmpeg_path` / `media.ffprobe_path` 指定。

当前安装位置：`C:\Users\tolov\AppData\Local\Microsoft\WinGet\Packages\Gyan.FFmpeg_*`
（`bin\ffmpeg.exe` / `bin\ffprobe.exe`）。

## 3. 运行时注意（FFmpeg PATH 陷阱）

- winget 装的 FFmpeg 需新开终端 PATH 才生效。
- 服务启动时统一解析 ffmpeg/ffprobe（`media.ResolvePaths`，ADR-020）：配置的绝对路径直接用，裸名走 PATH；
  **找不到会启动即报错**。若 PATH 里缺 `ffprobe`/`ffmpeg`（报 `resolve ffmpeg/ffprobe: executable file not
  found in %PATH%`），在 `config.yaml` 的 `media.ffmpeg_path` / `media.ffprobe_path` 显式指定绝对路径
  （YAML 双引号内用 `\\` 转义），改后需重启服务。若希望显式禁用媒体能力（remux/转码/格式工厂），将对应项置空。
- 媒体管线自身的约定见 [media.md](media.md)。

## 4. 前端包管理器

**一律使用 `pnpm`**（本机 pnpm 11.x）；**禁止**使用 `npm` / `cnpm` / `yarn` 执行安装、脚本等任何前端命令。

## 5. 部署形态 / 静态托管 / 启动输出

- 单二进制：后端通过 `config.server.static_dir` 托管前端构建产物；空则自动探测 `./static`
  （部署形态）或 `../frontend/dist`（backend 目录运行 dev 形态），均不存在则仅 API。
- 静态托管为 SPA 模式：未命中 `/api` 的 GET 从静态目录出文件，无扩展名路径回退 `index.html`
  （深链刷新直达），未知 `/api/*` 保持 JSON 404（不返回 SPA）。
- `cmd/server` 显式 `net.Listen` 后再打印可达地址：绑定 `0.0.0.0` 时经 `netutil.URLs` 枚举本机
  地址（IPv4 全量 + IPv6 ULA `fc00::/7`，排除 loopback/link-local/隐私地址），无可枚举时回退
  `localhost`；端口被占用则启动即失败（退出码 1）。
