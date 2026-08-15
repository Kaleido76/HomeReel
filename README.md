# HomeReel

**Self-hosted personal video library manager (DAM) — a lightweight Jellyfin for your home.**
部署在家用 PC 的个人视频资料管理平台：局域网内所有设备经 Web 界面访问，视频管理与文件管理一体。

## Features

- **视频库（简易 Jellyfin）**：海报墙、单集 / 系列分组、元数据编辑、续播、首页行式浏览
- **三层动态播放**：Direct 直连秒开（HTTP Range）；容器不兼容但编码兼容（如 MKV h264+aac）自动流拷贝成
  MP4（Remux）；编码不兼容（HEVC/rmvb 等）按需转码 HLS（Transcode）；三者皆不可才引导「格式工厂」
- **多终端并发**：单口令 + 独立会话，PC / 手机 / 平板 / TV 同时在线互不挤占
- **文件管理**：浏览 / 复制 / 移动 / 重命名 / 删除，多媒体源标记即扫描入库
- **一切围绕元数据**：SQLite + FTS5 搜索，AI 等扩展能力以事件总线解耦接入

## Tech Stack

Go · React 19 + TypeScript · SQLite (modernc.org/sqlite) · FFmpeg · Tailwind CSS 4 · Vidstack

## Documentation

- [AGENTS.md](AGENTS.md) — 项目指南：定位、原则、ADR、工作规范与红线
- [docs/decisions.md](docs/decisions.md) — 数据模型 / API 契约 / 配置 / 演进路线
- [docs/](docs/) — 后端 / 媒体管线 / 前端 / 环境 / 现状
