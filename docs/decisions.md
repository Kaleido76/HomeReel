# decisions.md — 契约与决策细节

> 本文件承载跨前后端的**契约**：数据模型语义、API 端点、配置示例、阶段与测试、风险对策、演进路线。
> 架构「为什么这样」（ADR）见 [AGENTS.md](../AGENTS.md) §4；实现细节以 backend / media / frontend /
> realtime / environment 各领域文档为准。

## 1. 数据模型

### 1.1 存储布局

```
<data_dir>/            # 默认 data（config 可改）
├── data.db            # SQLite（WAL 模式）
├── covers/            # <video_id>.jpg（大图，Library 卡片）
├── thumbs/            # <video_id>.thumb.jpg（小图，网格列表）
├── subtitles/         # <id>.vtt / <id>-<track>.vtt（按需提取的内封文本字幕）
├── remux/             # Remux 流拷贝缓存 MP4（<id>.mp4 / <id>-a<N>.mp4）+ 指纹 sidecar
└── hls/               # Transcode 会话分片（会话空闲约 10 分钟清理）
```

媒体源文件**不复制入库**，始终引用磁盘原始路径。视频库的入库单位是用户标记的**多媒体源**目录
（ADR-011），与文件浏览无生命周期绑定。

### 1.2 表语义

> DDL 以 `backend/internal/db/db.go` 迁移数组为准，此处只记语义与难读出的列约定。

- **media_sources** `(id, path UNIQUE, created_at, last_scan_at)`：多媒体源标记 + 扫描单位。
  「离线」是运行期状态不入库；根不可达时扫描中止不动库（ADR-014）。
- **videos**：身份三元组 `(source_id, relative_path)` 唯一，`file_id` 全局匹配（独立索引）；
  `(size, mtime)` 为变更指纹。probe 写技术列 `codec/audio_codec/container/duration/width/height/fps/
  segmented/faststart`（`segmented`=fMP4 分片布局不可直连；`faststart`=moov 前置）。
  归属列 `show_id/season_number/episode_number/episode_title/kind(movie|episode)`；
  手动列 `title/rating/studio/cast_text/tags(见 video_tags)`。
  **`title_source`**：`file`=文件派生可被扫描刷新 | `manual`=用户编辑永不覆盖。
  `search_text` 反规范化供 FTS5。`path` 冗余绝对路径便于直接访问。
- **shows**：系列显示名容器。`name` 创建时默认取文件夹名，之后仅 `PATCH /api/shows/{id}` 可改，
  扫描永不复写（与单集 `title_source` 同语义）。
- **seasons**：系列本体。`root_path UNIQUE` 是文件夹对应的唯一事实源；`sort_manual=1` 表示成员顺序
  由用户手动维护（扫描只追加新成员不重排）；`number` 恒为 1（无季号结构）；`name/kind` 为历史列不再使用。
- **link_groups + link_group_members**：系列关联 = 同组互相可见（ADR-018）；
  `series_id` 唯一索引保证每系列至多一组。
- **history** `(video_id, user, progress, updated_at)`：续播位置，属用户数据。
- **playback_prefs / series_playback_prefs**：播放选择记忆，**可重建缓存**（删行回默认），与 history 分离。
  单集级存具体值（`audio_track` 序号 / `subtitle_id` / `volume` / `muted`）；系列级存音轨/字幕**名称**
  （label，同一选择在每集解析到各自真实轨），系列有记录时优先于单集记录（ADR-006）。
  `subtitle_id` 编码：`sidecar`=侧边文件 / `e<N>`=内封文本轨序号 / `""`=明确关闭 / NULL=未选。
- **sessions / settings**：会话与键值配置（pin、口令哈希）。时间戳用秒级 RFC3339。
- **jobs**：队列行（type/target/extra JSON/status/progress/name/internal）。类型：
  probe/thumbnail（internal 隐藏）/ scan_source/mark_resource/fscopy/fsmove/convert。
  `progress<0` 表示不确定。
- **videos_fts**：FTS5 external content（title, search_text），ai/ad/au 触发器同步。
- **devlogs**：前端日志归档（entries JSON 数组逐字存储）。

> 迁移机制：`schema_migrations` 版本表 + 顺序 DDL 数组（见 [backend.md](backend.md) §3）。
> 数据层时间戳统一固定宽度纳秒 RFC3339（字典序=时间序），例外见 backend.md §1。
> 集合子系统、year/genre 列、简介 overview/description 等已由追加迁移物理删除。

## 2. 后端 API 契约

统一前缀 `/api`；除 auth 外均需会话 Cookie。JSON；错误统一 `{ error: { code, message } }`。
以下为端点清单与请求语义；响应字段以各 handler 为准。

### 2.1 认证

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/auth/login` | `{ password }` → 校验口令，签发会话 Cookie |
| POST | `/api/auth/logout` | 注销当前会话 |
| GET | `/api/auth/status` | 是否已登录（前端路由守卫用） |
| GET | `/api/me` | 当前身份（恒为 `{ user: "local" }`） |

### 2.2 文件浏览与多媒体源

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/disks` | Windows 本地盘符枚举（fixed/removable，排除网络盘） |
| GET | `/api/files/list?path=` | 按绝对路径实时列目录（后端按 HIDDEN/SYSTEM 属性过滤） |
| POST | `/api/files/copy\|move` | 剪贴板式复制/移动（入 jobs 后台任务，字节进度） |
| POST | `/api/files/rename` / `renames` | 单个 / 批量重命名（批量同步，OpResult 收集逐项错误） |
| POST | `/api/files/delete` | 永久删除（批量） |
| GET/POST/DELETE | `/api/files/pins` | 常用路径固定（settings 表） |
| GET | `/api/files/sources` | 多媒体源列表（含 `available` 离线态 / `scanning` 扫描中） |
| POST | `/api/files/sources` | 标记当前目录为多媒体源并入队全量扫描；禁止嵌套 → 400 `nested_source` |
| DELETE | `/api/files/sources?path=` | 取消标记（先删该源全部视频与系列再删标记，磁盘不受影响） |
| POST | `/api/files/sources/scan` | 手动重新扫描 |
| POST | `/api/files/resources` | 标记所选文件夹为系列（入队 mark_resource 任务） |
| POST | `/api/convert` | 格式工厂：把所选文件/文件夹转为 Faststart MP4 副本（入队 convert 任务，策略见 media.md §3.1） |
| POST | `/api/convert/probe` | 格式工厂探测：逐个 ffprobe 返回流事实，供操作面板指引/禁用 |

### 2.3 视频库与系列

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/videos` | 列表；`q`（仅匹配显示标题）/ `tag`（可重复 AND）/ `kind` / `showId` / `ungrouped=1` / 分页参数 |
| GET | `/api/videos/:id` | 详情（标签、所属系列、能力标志 direct/remux/transcode_playable、source_status） |
| PATCH | `/api/videos/:id` | 更新 `title / kind / rating / studio / cast_text / tags`（归属由路径决定，不接受 show/episode 字段） |
| DELETE | `/api/videos/:id` | 删除元数据记录（不动源文件） |
| POST | `/api/videos/:id/refresh` | 重新 ffprobe 并更新元数据 |
| POST | `/api/videos/:id/sync` | 按 file_id 定位改名/移动的源文件并收敛系列（找不到 → 404） |
| GET | `/api/series` | 系列列表（kind/成员数/关联系数/总时长），支持 `q`（仅显示名）/ `tag` |
| GET | `/api/series/:id` | 系列详情（members / links / check 同步状态） |
| POST | `/api/series/:id/sync` | 对根目录执行一次局部同步（与标记同逻辑） |
| DELETE | `/api/series/:id/history` | 清除该系列全部成员观看进度 |
| POST | `/api/series/:id/order` | 手动排序成员 `{ video_ids }`（须为成员全集排列；置 sort_manual） |
| POST | `/api/series/:id/resort` | 恢复自动模式：清 sort_manual 按文件名序重绑 1..N（纯 DB 操作） |
| GET | `/api/series/:id/members` | 成员列表（位次排序、允许缺失、含进度） |
| GET | `/api/series/:id/links` | 关联列表（同组其他系列，互相可见） |
| PUT | `/api/series/:id/links` | 全量替换关联勾选集 `{ series_ids }`（ADR-018 分组模型） |
| DELETE | `/api/series/:id/links/:linkedId` | 移除单条关联（双向生效） |
| GET | `/api/series/:id/poster` | 系列海报（fallback 成员封面） |
| GET | `/api/tags` | 全部标签及出现次数 |

> `PATCH /api/shows/:id` 的 `name` 是 `/api/shows*` 中唯一仍被前端使用的端点（系列显示名，ADR-015）。

### 2.4 播放 / 流媒体 / 缓存

> 三层动态流的判定与各层语义见 [media.md](media.md) §4；此处只列端点。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/stream/:id` | Direct：HTTP Range 直连源文件 |
| GET | `/api/stream/:id/remux?audio=N` | Remux：整片流拷贝成缓存 MP4 后 Range 输出；按音轨独立缓存 |
| GET | `/api/stream/:id/hls/playlist.m3u8?session=&audio=N` | Transcode：VOD 全量播放列表（分片 URL 内嵌 session；换轨须换会话） |
| GET | `/api/stream/:id/hls/{file}` | 按需转码分片 seg-{n}.ts（首次请求当场生成并缓存） |
| GET | `/api/stream/:id/cover` / `subtitle?track=` | 封面图 / 字幕（侧边优先，内封文本轨按需提取 vtt） |
| GET | `/api/videos/:id/subtitles` / `audio` | 字幕轨清单 / 音轨清单（供播放器菜单选轨） |
| GET | `/api/cache` | 缓存概览（孤儿统计 + 字幕分组 + 播放选择记忆列表） |
| DELETE | `/api/cache?kind=subtitle` | 清空全部字幕缓存（唯一支持的 kind，其余走孤儿清理） |
| DELETE | `/api/cache/orphans` | 清空孤儿缓存（库中已无对应视频的残留文件） |
| DELETE | `/api/cache/subtitles/{videoId}[/{track}]` | 按视频 / 按轨删除字幕缓存 |
| DELETE | `/api/cache/prefs[/{videoId}]` | 清空全部 / 单视频播放选择记忆 |

### 2.5 历史 / 搜索 / 设置 / 播放记忆

| 方法 | 路径 | 说明 |
|---|---|---|
| GET/PUT/DELETE | `/api/videos/:id/history` | 读 / 存（节流由前端控制）/ 清续播位置 |
| GET | `/api/videos/:id/prefs` | 读**有效**播放记忆：`{ prefs, series_id }`，`prefs.scope` 区分 `series`（存名称）/ `video`（存具体值）；无记录 null |
| PUT | `/api/videos/:id/prefs` | 部分更新；系列成员写系列级（发名称）、独立单集写自身（发序号/id）；仅用户手动切换时调用 |
| GET/DELETE | `/api/series/:id/prefs` | 系列共享记忆读取 / 删除 |
| GET | `/api/home` | 首页行：继续观看 / 最近添加 |
| GET | `/api/search?q=` | 统一搜索（FTS5：标题 / 标签 / 剧名） |
| GET | `/api/jobs` | 任务队列快照（运行期靠 WS 推送，见 §2.7） |
| GET | `/api/health` | 健康检查 |

### 2.6 开发者工具日志归档

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/devlogs` | 提交日志归档 `{ source?, note?, entries[] }` → `{ id }` |
| GET | `/api/devlogs` | 归档列表（不含 entries） |
| GET | `/api/devlogs/:id`[/raw] | 取回归档（逐字还原）/ 纯文本（text/plain，非 GUI 抓取用） |
| DELETE | `/api/devlogs/:id` | 删除归档 |

### 2.7 实时通道（WebSocket，ADR-021）

`GET /api/ws`（cookie 鉴权，未登录 401）。信封 `{ id?, type, data? }`：

- `id` 非空 = RPC 请求，响应 `<type>.result` / `<type>.error`（错误体与 REST 一致）；
- 无 `id` = 即发即忘推送，不回复。

内置消息：`ping/pong` 探活；`events.<域事件>` 由事件桥转发全部域事件
（`video.imported`、`jobs.done`…）。协议细节与前后端实现见 [realtime.md](realtime.md)。

## 3. 配置（config.yaml 示例）

```yaml
server:
  host: "0.0.0.0"          # 局域网访问
  port: 8080
  data_dir: "data"         # 默认 data（相对路径）
  static_dir: ""           # 前端构建产物目录；空=自动探测 ./static 或 ../frontend/dist，均不存在则仅 API

auth:
  password: ""             # 空 → 首启自动生成并打印
  session_days: 30

# 多媒体源由前端在「文件」页签标记，不在 config.yaml 声明。

media:
  ffmpeg_path: "ffmpeg"    # 默认取 PATH；缺失启动即报错（见 environment.md）
  ffprobe_path: "ffprobe"
  probe_concurrency: 2     # 后台 probe/thumbnail 并发数

log:
  level: "info"            # debug | info | warn | error（默认 info；详见 backend.md §10）
  format: "text"           # text | json（默认 text）
  file: ""                 # 可选日志文件路径；空=仅控制台。启动时把已有日志按日期轮转一次
```

## 4. AI 模块扩展契约（仅预留，默认不实现）

AI 以独立进程接入，通过**事件 + REST + 任务队列**解耦（ADR-010）：订阅 `VideoImported` 等 Listener，
不修改主流程。预留点：`videos` 增加 `transcript/summary/embedding_id` 列（Phase 4 启用时 ALTER）、
jobs 新增类型、`SearchProvider` 新增 `ai` 实现、`video_tags` 带 `source=ai` 标记。

## 5. 开发阶段与测试

v1.0 目标：「视频管理」做到每天愿意用——导入稳定、缩略图快、播放流畅（续播/字幕/倍速）、搜索标签好用，
Library 达到简易 JellyFin 体验；基础层稳定前不引入 AI。

- Phase 0 骨架与认证 ✅ / Phase 1 文件管理与索引 ✅ / Phase 2 播放与媒体体验 ✅ /
  Phase 3 媒体库体验（单集/系列、FTS5、首页行）✅ / Phase 4 AI 扩展（仅接口预留，不实现）。

### 5.1 测试策略

- 后端：`go test ./...`——domain/store/api（httptest 全链路、mock FS 注入）/scanner 指纹/events 总线；
  媒体相关打 `//go:build integration`（有 FFmpeg 才跑，尚未编写）。关键路径：增量扫描去重、删除同步、
  FTS 同步、会话过期、外接卷拔出→插回、系列归组规则。
- 前端：Vitest + Testing Library（规划方向，**当前未配置**，见 status.md §2）。
- 验收红线：不得跳过测试进入下一阶段。

## 6. 风险与对策

| 风险 | 对策 |
|---|---|
| 大目录首次扫描耗时长 | 增量指纹 + 进度可见 + 可中断重扫；扫描串行避免并行 ffmpeg 抢占卡顿 |
| NTFS 符号链接/junction 歧义 | 只跟踪普通文件+目录，链接默认跳过；junction 取自身属性 |
| 源文件被外部改名/移动 | file_id 全局匹配校正路径；last_scanned_at 校正删除 |
| 多媒体源根不可达被误判「全部删除」 | 扫描前置可达性探测：不可达即中止、不动库（ADR-014） |
| 嵌套/重叠多媒体源 | 新建源禁止嵌套（400 `nested_source`）；路由表防御历史遗留嵌套 |
| 剧集命名不规范归组错误 | 系列只由用户显式创建，识别错误手动兜底（ADR-015） |
| 浏览器编码兼容（HEVC/MKV/AC3 等） | 三层动态流运行期判定，详见 media.md §4 |
| SQLite 并发写冲突 | WAL + SetMaxOpenConns(1)，写操作收敛 store 层串行 |
| 多终端并发操作互踩 | 会话相互独立；文件/DB 写收敛串行；历史进度后写者胜 |
| 局域网弱设备播放卡顿 | 缩略图降载 + 格式工厂转低码率 MP4 |
| 口令弱导致泄露 | 默认随机口令、登录失败限速 |

> 播放相关的格式级风险（MKV seek、AC3 无声、HLS 声道布局等）及实测依据见 media.md §4。

## 7. 未来演进路线（不在本期）

1. PostgreSQL 迁移（repo 接口已隔离；真遇瓶颈才考虑）。
2. Meilisearch 替换 FTS5（SearchProvider 新增实现）。
3. AI 模块按 §4 落地（事件驱动）。
4. 多用户与权限（user 字段已留位）。
5. 上传 / fsnotify 变更监控（复用 IngestPaths/EvictPaths 统一入口，ADR-017）。
6. 移动端 PWA 离线壳；TV Mode（仅 Library 界面，ADR-013）。
7. 媒体转码队列化（批量预转码）；基于元数据与历史的轻量推荐。
