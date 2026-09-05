# decisions.md — 架构决策与契约

> 本文件承载跨前后端的**骨架决策与契约**：架构取舍、数据模型、API 端点、配置。
> 实现细节以各领域文档为准；开发阶段、风险、演进见 [status.md](status.md)。

## 0. 架构决策笔记

> 记录「为什么这样」的骨架决策与踩坑结论。**只记两类内容**：
> 1. **不可逆的骨架决策**（单向门）：如存储模型、通信协议、部署形态
> 2. **踩坑后的负向结论**（墓碑记录）：如"为什么放弃了 X"
>
> **绝对不记**：具体 API 路由、前端 UI 交互、内部数据结构字段名。这些直接看 Git Commit 和代码。

| # | 类型 | 决策 | 结论 | 理由 |
|---|---|---|---|---|
| 001 | 骨架 | 后端形态 | 单个 Go 服务，**v1.0 全程不拆服务** | 家庭部署单进程足够；拆服务带来成倍的运维成本 |
| 002 | 骨架 | 认证模型 | 单口令 + 会话 Cookie；多终端并发独立；无用户概念，`user` 固定 `local` | 个人使用；多终端同时在线互不挤占 |
| 003 | 骨架 | 部署形态 | 纯 Web 应用，监听 `0.0.0.0`，浏览器访问 | 局域网任意设备免安装即用 |
| 005 | 骨架 | 数据库 | SQLite（纯 Go 驱动 + WAL），真遇瓶颈才考虑 PostgreSQL | Windows 免 CGO、零依赖单文件 |
| 006 | 骨架 | 播放策略 | 三层动态流：Direct → Remux → Transcode → 格式工厂兜底；前端运行期 `canPlayType()` 判定 | Remux 成本低且全片可拖；Transcode 只转观看部分 |
| 007 | 骨架 | 文件身份 | file_id **全局匹配**（跨源移动仍是同一视频）；`(size, mtime)` 为变更指纹 | 目录移动甚至跨源移动不丢身份与历史 |
| 008 | 骨架 | 任务队列 | SQLite 持久化 `jobs` 表 + 进程内 worker 池 | 进度可见、崩溃可恢复、实现简单 |
| 013 | 骨架 | 前端形态 | 文件浏览与视频库共享 API/播放器/元数据，顶部页签切换 | 共享层避免重复实现 |
| 021 | 骨架 | 实时通道 | 单条 WebSocket 承载 S→C 推送 + C→S RPC；事件桥复用域事件总线 | 轮询有延迟；复用 events 总线前端零额外依赖 |
| 016 | 墓碑 | ~~元数据刮削~~ | TMDB 在线 + NFO 子系统已于 2026-08 整体移除，仅保留手动编辑 | 防止重新引入 |

**规则**：实现细节下沉到 `docs/` 下的领域文档。本文档只记骨架。

## 1. 数据模型

### 1.1 存储布局

```
<data_dir>/
├── data.db            # SQLite（WAL 模式）
├── covers/            # <video_id>.jpg（大图，Library 卡片）
├── thumbs/            # <video_id>.thumb.jpg（小图，网格列表）
├── subtitles/         # <id>.vtt / <id>-<track>.vtt（内封文本字幕提取缓存）
├── remux/             # Remux 流拷贝缓存 MP4 + 指纹 sidecar
└── hls/               # Transcode 会话分片（空闲 ~10min 清理）
```

媒体源文件**不复制入库**，始终引用磁盘原始路径。

### 1.2 表语义

> DDL 以 `backend/internal/db/db.go` 迁移数组为准，此处只记语义。

- **media_sources** `(id, path UNIQUE, created_at, last_scan_at)`：多媒体源标记 + 扫描单位。
- **videos**：身份三元组 `(source_id, relative_path)` 唯一，`file_id` 全局匹配；`(size, mtime)` 为变更指纹。
  归属列 `show_id/season_number/episode_number/episode_title/kind(movie|episode)`；
  手动列 `title/rating/studio/cast_text/tags`。
  **`title_source`**：`file`=文件派生可被扫描刷新 | `manual`=用户编辑永不覆盖。
- **shows**：系列显示名容器。`name` 创建时默认取文件夹名，之后仅 `PATCH /api/shows/{id}` 可改。
- **seasons**：系列本体。`root_path UNIQUE` 是文件夹对应的唯一事实源；`sort_manual=1` 表示成员顺序由用户手动维护。
- **link_groups + link_group_members**：系列关联 = 同组互相可见；`series_id` 唯一索引保证每系列至多一组。
- **history** `(video_id, user, progress, updated_at)`：续播位置。
- **playback_prefs / series_playback_prefs**：播放选择记忆，**可重建缓存**。系列记录优先于单集记录（ADR-006）。
- **sessions / settings**：会话与键值配置（pin、口令哈希）。时间戳用秒级 RFC3339。
- **jobs**：队列行。类型：probe/thumbnail（internal）/ scan_source/mark_resource/fscopy/fsmove/convert。
- **videos_fts**：FTS5 external content（title, search_text），触发器同步。
- **devlogs**：前端日志归档。

## 2. 后端 API 契约

统一前缀 `/api`；除 auth 外均需会话 Cookie。JSON；错误统一 `{ error: { code, message } }`。

### 2.1 认证

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/auth/login` | `{ password }` → 校验口令，签发会话 Cookie |
| POST | `/api/auth/logout` | 注销当前会话 |
| GET | `/api/auth/status` | 是否已登录 |
| GET | `/api/me` | 当前身份（恒为 `{ user: "local" }`） |

### 2.2 文件浏览与多媒体源

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/disks` | Windows 本地盘符枚举 |
| GET | `/api/files/list?path=` | 按绝对路径实时列目录 |
| POST | `/api/files/copy\|move` | 剪贴板式复制/移动（入 jobs） |
| POST | `/api/files/rename` / `renames` | 单个 / 批量重命名 |
| POST | `/api/files/delete` | 永久删除（批量） |
| GET/POST/DELETE | `/api/files/pins` | 常用路径固定 |
| GET | `/api/files/sources` | 多媒体源列表（含离线态） |
| POST | `/api/files/sources` | 标记为多媒体源并入队扫描；禁止嵌套 |
| DELETE | `/api/files/sources?path=` | 取消标记（磁盘不受影响） |
| POST | `/api/files/sources/scan` | 手动重新扫描 |
| POST | `/api/files/resources` | 标记为系列 |
| POST | `/api/convert` | 格式工厂：转为 Faststart MP4 副本 |
| POST | `/api/convert/probe` | 格式工厂探测 |

### 2.3 视频库与系列

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/videos` | 列表；支持 `q`/`tag`/`kind`/`showId`/`ungrouped`/分页 |
| GET | `/api/videos/:id` | 详情（含能力标志） |
| PATCH | `/api/videos/:id` | 更新元数据 |
| DELETE | `/api/videos/:id` | 删除元数据记录（不动源文件） |
| POST | `/api/videos/:id/refresh` | 重新 ffprobe |
| POST | `/api/videos/:id/sync` | 按 file_id 定位改名/移动的源文件 |
| GET | `/api/series` | 系列列表 |
| GET | `/api/series/:id` | 系列详情 |
| POST | `/api/series/:id/sync` | 局部同步 |
| DELETE | `/api/series/:id/history` | 清除全部成员观看进度 |
| POST | `/api/series/:id/order` | 手动排序成员 |
| POST | `/api/series/:id/resort` | 恢复自动模式 |
| GET | `/api/series/:id/members` | 成员列表 |
| GET/PUT/DELETE | `/api/series/:id/links` | 关联管理（同组互相可见） |
| GET | `/api/series/:id/poster` | 系列海报 |
| GET | `/api/tags` | 全部标签及出现次数 |
| PATCH | `/api/shows/:id` | 修改系列显示名 |

### 2.4 播放 / 流媒体 / 缓存

> 三层动态流判定见 [media.md](media.md) §4。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/stream/:id` | Direct：HTTP Range 直连 |
| GET | `/api/stream/:id/remux?audio=N` | Remux：整片流拷贝后 Range 输出 |
| GET | `/api/stream/:id/hls/playlist.m3u8` | Transcode：VOD 播放列表 |
| GET | `/api/stream/:id/hls/{file}` | 按需转码分片 |
| GET | `/api/stream/:id/cover` / `subtitle` | 封面图 / 字幕 |
| GET | `/api/videos/:id/subtitles` / `audio` | 字幕轨 / 音轨清单 |
| GET | `/api/cache` | 缓存概览 |
| DELETE | `/api/cache` / `/api/cache/orphans` | 清空字幕缓存 / 孤儿缓存 |
| DELETE | `/api/cache/remux/{videoId}` | 清空单视频 Remux 缓存 |
| DELETE | `/api/cache/series/{id}/subtitles\|remux` | 系列级批量清理 |
| DELETE | `/api/cache/prefs[/{videoId}]` | 清空播放选择记忆 |
| POST | `/api/cache/pregen` | 入队预生成 Job |

### 2.5 历史 / 搜索 / 设置

| 方法 | 路径 | 说明 |
|---|---|---|
| GET/PUT/DELETE | `/api/videos/:id/history` | 续播位置 |
| GET/PUT | `/api/videos/:id/prefs` | 播放选择记忆（单集级） |
| GET/DELETE | `/api/series/:id/prefs` | 播放选择记忆（系列级） |
| GET | `/api/home` | 首页行 |
| GET | `/api/search?q=` | 统一搜索 |
| GET | `/api/jobs` | 任务队列快照 |
| GET | `/api/health` | 健康检查 |

### 2.6 开发者工具日志

| 方法 | 路径 | 说明 |
|---|---|---|
| POST/GET | `/api/devlogs` | 归档提交 / 列表 |
| GET/DELETE | `/api/devlogs/:id` | 取回归档 / 删除 |

### 2.7 实时通道（WebSocket）

`GET /api/ws`（cookie 鉴权）。信封 `{ id?, type, data? }`：`id` 非空 = RPC，无 `id` = 推送。
协议细节见 [realtime.md](realtime.md)。

## 3. 配置（config.yaml）

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  data_dir: "data"
  static_dir: ""           # 空=自动探测 ./static 或 ../frontend/dist

auth:
  password: ""             # 空 → 首启自动生成并打印
  session_days: 30

media:
  ffmpeg_path: "ffmpeg"    # 默认取 PATH；缺失启动即报错
  ffprobe_path: "ffprobe"
  probe_concurrency: 2

log:
  level: "info"            # debug | info | warn | error
  format: "text"           # text | json
  file: ""                 # 空=仅控制台
```

## 4. AI 模块扩展契约（仅预留）

AI 以独立进程接入，通过事件 + REST + 任务队列解耦。预留点：videos 增加 transcript/summary/embedding_id 列、
jobs 新增类型、SearchProvider 新增 ai 实现、video_tags 带 source=ai 标记。
