# environment.md — 开发环境要求

> 本文件记录本机开发环境的具体版本与安装状态，属环境级知识（`AGENTS.md` §5.1 展开层）。

## 1. 工具矩阵（当前本机状态）

| 工具 | 用途 | 本机状态 |
|---|---|---|
| Go 1.22+ | 后端编译（当前 1.26.5，`CGO_ENABLED=0`） | ✅ 已装（`C:\Program Files\Go`） |
| Node.js 18+ | 前端运行（当前 v24.18.0） | ✅ 已装 |
| pnpm | 前端包管理（当前 11.9.0，registry 为 npmmirror） | ✅ 已装 |
| Git | 版本管理（当前 2.55.0） | ✅ 已装 |
| winget | 软件安装（备选 choco/scoop 未装） | ✅ 已装 |
| **FFmpeg / ffprobe** | 媒体探测、缩略图、HLS 转码、字幕；integration 测试依赖 | ✅ 已装（8.1.2，`winget install Gyan.FFmpeg`，已入 PATH） |

本机 `go env`：`GOOS=windows GOARCH=amd64 CGO_ENABLED=0`，符合「免 CGO 单 exe」要求。

## 2. FFmpeg 安装

方式一（推荐）：`winget install Gyan.FFmpeg --source winget`（装后写入 PATH，需新开终端生效）。
方式二：将 `ffmpeg.exe` / `ffprobe.exe` 放入项目 `bin/`，并在 `config.yaml` 的
`media.ffmpeg_path` / `media.ffprobe_path` 指定。

当前安装位置：`C:\Users\tolov\AppData\Local\Microsoft\WinGet\Packages\Gyan.FFmpeg_*`
（`bin\ffmpeg.exe` / `bin\ffprobe.exe`）。

## 3. 运行时注意（FFmpeg PATH 陷阱）

- winget 装的 FFmpeg 需新开终端 PATH 才生效。
- 若服务进程 PATH 里找不到 `ffprobe`（probe 任务报 `executable file not found in %PATH%`），在
  `config.yaml` 的 `media.ffmpeg_path` / `media.ffprobe_path` 显式指定绝对路径（YAML 双引号内用
  `\\` 转义），改后需重启服务。
- 媒体管线自身的约定见 [media.md](media.md)。

## 4. 前端包管理器

**一律使用 `pnpm`**（本机 pnpm 11.x）；**禁止**使用 `npm` / `cnpm` / `yarn` 执行安装、脚本等任何前端命令。
