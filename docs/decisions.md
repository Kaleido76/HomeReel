# decisions.md — 契约与决策细节

> 本文件承载原 `DEVELOPMENT_PLAN.md`（2026-09 删除）中的**契约**部分：数据模型、API 契约、配置示例、
> AI 扩展预留、开发阶段与测试、风险对策、演进路线。**架构「为什么这样」的决策依据（ADR）在
> [AGENTS.md](../AGENTS.md) §4**。实现细节以 backend / media / frontend / environment 各领域文档为准。

## 1. 数据模型与存储布局

### 1.1 存储布局

```
<data_dir>/            # 默认 data（config 可改）
├── data.db            # SQLite（WAL 模式）
├── covers/            # <video_id>.jpg  （大图，供 Library 卡片）
├── thumbs/            # <video_id>.thumb.jpg （小图，供网格列表）
├── subtitles/         # <id>.vtt / <id>-<track>.vtt 抽取的字幕（可选）
├── remux/             # Remux 整片流拷贝的缓存 MP4（<id>.mp4 / <id>-a<N>.mp4）+ 指纹 sidecar（可重建）
└── hls/               # Transcode 会话分片（会话空闲约 10 分钟清理）
```

媒体源文件**不复制入库**，始终引用磁盘上的原始路径。视频库的入库单位是用户标记的**多媒体源**目录
（见 AGENTS §4 ADR-011），与文件浏览系统无生命周期绑定。

### 1.2 表结构

```sql
-- 多媒体源（ADR-011 替换 storages）：轻量持久化标记 + 扫描单位，不参与文件浏览生命周期
CREATE TABLE media_sources (
  id           TEXT PRIMARY KEY,            -- ULID
  path         TEXT NOT NULL UNIQUE,        -- 规范化绝对路径，如 D:\Videos
  created_at   TEXT NOT NULL,
  last_scan_at TEXT                         -- 最近一次成功扫描的开始时间
);

-- 视频主表（身份：source_id + file_id + relative_path，ADR-007；file_id 全局匹配）
CREATE TABLE videos (
  id               TEXT PRIMARY KEY,            -- ULID
  source_id        TEXT REFERENCES media_sources(id) ON DELETE SET NULL, -- 归属媒体源（2026-08 管理面定稿：所有单集必须归属媒体源；NULL 仅历史兼容）
  file_id          TEXT NOT NULL,               -- inode / Windows FileID（NTFS 文件 ID）
  relative_path    TEXT NOT NULL,               -- 相对所属多媒体源根的路径
  path             TEXT NOT NULL,               -- 绝对路径（冗余，便于流式/直接访问）
  size             INTEGER NOT NULL DEFAULT 0,  -- 文件字节数，与 mtime 一起作为变更指纹
  mtime            INTEGER NOT NULL DEFAULT 0,  -- 修改时间（毫秒），与 size 一起作为变更指纹
  title            TEXT NOT NULL DEFAULT '',    -- 电影标题 / 剧集默认为 Show 名
  kind             TEXT NOT NULL DEFAULT 'movie', -- movie（单集）| episode（系列成员）
  duration         REAL,                        -- 秒
  codec            TEXT,                        -- 视频编码（如 h264 / hevc）
  audio_codec      TEXT,
  container        TEXT,                        -- mkv / mp4 / mov ...
  segmented        INTEGER NOT NULL DEFAULT 0,  -- 分段 MP4（多 mdat/moof），探测标记，不可直连
  faststart        INTEGER NOT NULL DEFAULT 0,  -- mp4 家族 moov 前置（可立即 seek）
  width            INTEGER,
  height           INTEGER,
  fps              REAL,
  file_size        INTEGER,
  cover_path       TEXT,                        -- 相对 data_dir 的封面路径
  thumb_path       TEXT,
  backdrop_path    TEXT,                        -- 详情页背景大图（刮削/手动）
  show_id          TEXT REFERENCES shows(id),   -- 归属剧集（episode 必填）
  season_number    INTEGER,                     -- 第几季
  episode_number   INTEGER,                     -- 第几集
  episode_title    TEXT,                        -- 单集名（默认为文件名）
  rating           REAL,                        -- 评分 0~10（刮削或用户）
  studio           TEXT,
  cast_text        TEXT,                        -- 演职员（逗号分隔）
  metadata_source  TEXT NOT NULL DEFAULT 'manual', -- manual（刮削源已移除）
  title_source     TEXT NOT NULL DEFAULT 'file', -- file（文件派生，扫描/探测刷新）| manual（用户编辑，永不覆盖）
  search_text      TEXT,                        -- 反规范化文本（title+tags+show），供 FTS5
  created_at       TEXT NOT NULL,
  updated_at       TEXT NOT NULL,
  last_scanned_at  TEXT NOT NULL,               -- 最近一次被扫描确认的时间
  UNIQUE (source_id, relative_path)
);
CREATE INDEX idx_videos_source ON videos(source_id);
CREATE INDEX idx_videos_file   ON videos(file_id);
CREATE INDEX idx_videos_show    ON videos(show_id, season_number, episode_number);
CREATE INDEX idx_videos_kind    ON videos(kind);

-- 多媒体源的「离线」是运行期状态（其根目录当前不可达），不入库；扫描遇不可达源直接中止、
-- 不更新元数据（ADR-014）。若将来同一视频可存在于多源（去重/镜像）再增加冗余可用性列。

-- 剧集（Show，ADR-015，JellyFin 式剧集分组）
CREATE TABLE shows (
  id              TEXT PRIMARY KEY,             -- ULID
  name            TEXT NOT NULL,                -- 系列显示名（创建时默认=文件夹名，可手动编辑，扫描不覆盖）
  overview        TEXT,
  rating          REAL,
  poster_path     TEXT,                         -- 相对 data_dir
  backdrop_path   TEXT,
  metadata_source TEXT DEFAULT 'manual',        -- manual（刮削源已移除）
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL
);
CREATE INDEX idx_shows_name ON shows(name);

-- 系列（2026-08 管理面定稿 + 2026-09 显示名/手动排序修订，ADR-015）：用户显式创建的管理容器，绑定一个根目录
-- （seasons.root_path）；成员 = 根目录**直接一级子文件**，默认文件名序 1..N；可手动拖拽重排（sort_manual=1，
-- 扫描/同步只追加新成员、不重排）。无季号结构。
CREATE TABLE seasons (
  id          TEXT PRIMARY KEY,                 -- ULID（即系列 id）
  show_id     TEXT NOT NULL REFERENCES shows(id) ON DELETE CASCADE,
  number      INTEGER NOT NULL,                 -- 恒为 1（系列无季号/部号）
  root_path   TEXT UNIQUE,                      -- 系列根目录（成员=其直接一级子文件；NULL 除外唯一），文件对应唯一事实源
  name        TEXT,                             -- 历史列（创建时=文件夹名；展示用 shows.name，本列不再使用）
  overview    TEXT,
  poster_path TEXT,
  kind        TEXT NOT NULL DEFAULT 'tv',       -- tv=剧集季 | movie=电影部（历史列，无电影/tv 结构类型）
  sort_manual INTEGER NOT NULL DEFAULT 0,       -- 1=成员顺序由用户手动维护（拖拽重排，ADR-015 修订）
  UNIQUE (show_id, number)
);

-- 系列间关联（2026-09 方案 B：显式分组）。关联 = 同一组内系列互相可见
-- （A 关联 B、C 时三者同组，打开任一方都看到另外两方）；无名称、无方向。
-- 组由「管理关联」全量替换维护（每系列至多一组），同 show 所有季自动并入一组。
CREATE TABLE link_groups (
  id         TEXT PRIMARY KEY,
  created_at TEXT NOT NULL
);
CREATE TABLE link_group_members (
  group_id   TEXT NOT NULL REFERENCES link_groups(id) ON DELETE CASCADE,
  series_id  TEXT NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
  sort_index INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  PRIMARY KEY (group_id, series_id)
);
CREATE UNIQUE INDEX idx_link_group_members_series ON link_group_members(series_id);

-- 标签（多值）
CREATE TABLE video_tags (
  video_id  TEXT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
  tag       TEXT NOT NULL,
  PRIMARY KEY (video_id, tag)
);

-- 播放历史 / 续播（单用户固定 user='local'，字段保留以兼容未来多用户）
CREATE TABLE history (
  video_id   TEXT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
  user       TEXT NOT NULL DEFAULT 'local',
  progress   REAL NOT NULL DEFAULT 0,           -- 秒
  updated_at TEXT NOT NULL,
  PRIMARY KEY (video_id, user)
);

-- 播放选择记忆（2026-09，ADR-006 player prefs）：per-video 的音轨/字幕/音量
-- 偏好缓存。与续播 history 分离——这是「可重建缓存」，可在工具页缓存管理删除
-- （删行即回到默认轨/默认字幕/默认音量），仅用户手动切换时刷新。
CREATE TABLE playback_prefs (
  video_id    TEXT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
  user        TEXT NOT NULL DEFAULT 'local',
  audio_track INTEGER,             -- 0 起音轨序号；NULL=未选（默认轨）
  subtitle_id TEXT,                -- "sidecar"=侧边文件 / "e<N>"=内封文本轨流序号 / ""=明确关闭字幕；NULL=未选
  volume      REAL,                -- 0~1；NULL=未记录
  muted       INTEGER,             -- NULL=未记录
  updated_at  TEXT NOT NULL,
  PRIMARY KEY (video_id, user)
);

-- 系列级播放选择记忆（2026-09 修订，ADR-006 player prefs）：系列剧集共享同一
-- 音轨/字幕/音量记忆。音轨/字幕按**轨道名称**（label）存储与匹配——同一选择在
-- 每集解析到各自的真实轨道（如「简体中文」在每集都选中对应轨）。**系列有记录时
-- 优先于单集记录**（单集记录作为关系重组后的兜底），删除走系列详情页/缓存管理。
CREATE TABLE series_playback_prefs (
  series_id        TEXT NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
  user             TEXT NOT NULL DEFAULT 'local',
  audio_track_name TEXT,           -- 音轨名称（label），NULL=未选
  subtitle_name    TEXT,           -- 字幕名称（label）；""=明确关闭字幕；NULL=未选
  volume           REAL,           -- 0~1；NULL=未记录
  muted            INTEGER,        -- NULL=未记录
  updated_at       TEXT NOT NULL,
  PRIMARY KEY (series_id, user)
);

-- 开发者工具日志归档（2026-09）：前端「开发者工具」提交的日志快照（移动端等无
-- 开发者工具处采集），供 PC 端选择查看/复制。entries 为 JSON 数组（每条含
-- timestamp/level/module/message），原样存储以便取回逐字还原。
CREATE TABLE devlogs (
  id         TEXT PRIMARY KEY,      -- ULID
  source     TEXT NOT NULL DEFAULT '',  -- 提交设备/来源描述（如「Android（移动端）」）
  note       TEXT NOT NULL DEFAULT '',  -- 用户备注（可选）
  entries    TEXT NOT NULL,         -- JSON 数组，日志条目
  created_at TEXT NOT NULL
);
CREATE INDEX idx_devlogs_created ON devlogs(created_at);

-- 会话（登录后签发）
CREATE TABLE sessions (
  token      TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL
);

-- 键值配置（固定 pin、口令哈希 等）
CREATE TABLE settings (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

-- 任务队列（ADR-008）
CREATE TABLE jobs (
  id         TEXT PRIMARY KEY,
  type       TEXT NOT NULL,        -- probe | thumbnail | scan_source | mark_resource | fscopy | fsmove | convert
  target     TEXT NOT NULL,        -- 目标文件/目录路径（probe/thumbnail 用 extra.video_id 定位）
  extra      TEXT NOT NULL DEFAULT '', -- JSON：{video_id, source_id} 等
  status     TEXT NOT NULL,        -- queued | running | done | failed
  progress   REAL NOT NULL DEFAULT 0,   -- 0~1；<0 表示不确定
  error      TEXT NOT NULL DEFAULT '',
  name       TEXT NOT NULL DEFAULT '',   -- 任务展示名（internal=false 时用于任务面板）
  internal   INTEGER NOT NULL DEFAULT 0, -- 1=内部短任务（probe/thumbnail），对用户隐藏
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

-- FTS5（external content，指向 videos，ADR-009）
CREATE VIRTUAL TABLE videos_fts USING fts5(
  content='videos', content_rowid='rowid',
  title, search_text
);
-- ai/ad/au 触发器同步；search_text = title + tags + show 名（store 层维护），
-- 剧集单集并入 Show 名称，便于按剧名命中。单集简介（description/overview）已于 2026-09 移除。
```

> 迁移说明：`db/` 包内置顺序迁移（`schema_migrations` 版本表 + 顺序执行 DDL），不引入重型迁移框架。
> 将来迁 PostgreSQL 时按 `domain/repo.go` 接口换实现即可。集合子系统已于 2026-08 移除：新库不再建集合表，
> 老库经追加的 `DROP TABLE` 迁移物理删除。
>
> 落地说明：**数据层时间戳统一为固定宽度纳秒 RFC3339（`2006-01-02T15:04:05.000000000Z07:00`）**，
> 保证字符串字典序即时间序（扫描 recency 比较依赖此约定）。例外：jobs 与 sessions 用秒级
> `time.RFC3339`（与排序语义无关，不参与 recency 比较）。

## 2. 后端 API 契约

统一前缀 `/api`；除 `auth` 外均需会话 Cookie。JSON 请求/响应；错误统一为
`{ "error": { "code": "...", "message": "..." } }`。

### 2.1 认证

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/auth/login` | `{ password }` → 校验口令，签发会话 Cookie |
| POST | `/api/auth/logout` | 注销会话 |
| GET | `/api/auth/status` | 是否已登录（前端路由守卫用） |
| GET | `/api/me` | 当前身份（恒为 `{ user: "local" }`，受保护路由示例） |

### 2.2 文件浏览与多媒体源（「文件」页签，2026-08 重构）

泛用机器级文件浏览器按**绝对路径**浏览，与视频库解耦。实现细节见 [backend.md](backend.md) §10。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/disks` | Windows 本地盘符枚举（fixed/removable，排除网络盘） |
| GET | `/api/files/list?path=` | 按绝对路径实时列目录（过滤 HIDDEN/SYSTEM；junction 取自身属性） |
| POST | `/api/files/copy\|move` | 剪贴板式复制/移动（入 jobs 后台任务，带字节进度） |
| POST | `/api/files/rename` | 重命名 |
| POST | `/api/files/renames` | 批量重命名（同步，OpResult 收集逐项错误） |
| POST | `/api/files/delete` | 永久删除（批量） |
| GET/POST/DELETE | `/api/files/pins` | 常用路径固定（settings 表） |
| GET | `/api/files/sources` | 多媒体源列表（含 `available` 离线状态 / `scanning` 扫描中） |
| POST | `/api/files/sources` | 标记当前目录为多媒体源并**入队全量扫描**；**禁止嵌套**（路径位于/包含既有源 → 400 `nested_source`） |
| DELETE | `/api/files/sources?path=` | 取消标记（**其下所有已入库单集与系列从库中移除**，磁盘文件不受影响；先删库再删标记，逐视频发布 `VideoDeleted` 清理缓存） |
| POST | `/api/files/sources/scan` | 手动重新扫描 |
| POST | `/api/files/resources` | 标记所选文件夹为系列 `{ paths, kind: series }` → 入队 mark_resource 任务（离散「单集」概念已清除，仅支持 series） |
| POST | `/api/convert` | 格式工厂：`{ paths }` 把所选文件/文件夹转换为**浏览器可播放的 Faststart MP4 副本**，逐个入队 `convert` 后台任务（单集→同目录 `X.mp4`；系列/文件夹→同级 `X (MP4)\`）。首选无损流拷贝（`-c copy`，音频浏览器不支持的编码自动转 AAC），无损失败自动降级为烧录字幕的高质量重编码（h264 CRF19，详见 [media.md](media.md) §3.1） |
| POST | `/api/convert/probe` | 格式工厂探测：`{ paths }` 逐个 ffprobe 返回流事实（视频/音频/字幕编码、位图字幕标记），供操作面板指引/禁用 |

> 旧存储卷 API（`/api/storages*`、`/api/fs/*`、`/api/upload`）已随旧 Explorer 一并移除（2026-08）。

### 2.3 视频库（Library，简易 JellyFin 媒体库）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/videos` | 列表，支持 `q`（仅匹配单集显示标题 `title`，不含文件名/路径）/ `tag（可重复，多标签 AND）/ kind / showId / ungrouped / sort / order / page / pageSize`；`ungrouped=1` 查单集 |
| GET | `/api/videos/:id` | 详情（含标签、所属系列 `series_id`、能力标志 `direct/remux/transcode_playable`、`source_status`/`new_path`） |
| PATCH | `/api/videos/:id` | 更新 `title / kind / rating / studio / cast_text / tags`（2026-09 起不含简介 description/overview、年份 year 与类型 genre） |
| DELETE | `/api/videos/:id` | 删除元数据记录（仅删库，不动源文件） |
| POST | `/api/videos/:id/refresh` | 重新 ffprobe 并更新元数据 |
| POST | `/api/videos/:id/sync` | 按 file_id 定位改名/移动的源文件并收敛系列成员（找不到 → 404，前端引导移除） |
| GET | `/api/series` | 系列列表（一季/一部一个系列，含 kind / 成员数 / 关联系数 / 总时长 `total_duration`），支持 `q`（仅系列显示名）/ `tag（可重复，成员标签 AND）` |
| GET | `/api/series/:id` | 系列详情（含 members / links / check） |
| POST | `/api/series/:id/sync` | 对根目录执行一次与标记相同的局部同步（按需检查的「同步」按钮） |
| DELETE | `/api/series/:id/history` | 清除该系列全部成员观看进度（只删历史，不动视频/文件） |
| POST | `/api/series/:id/order` | 手动排序成员 `{ video_ids: [...] }`（须为该系列成员全集的排列，重排 episode_number 并置 sort_manual） |
| POST | `/api/series/:id/resort` | 恢复自动模式（2026-09）：清 sort_manual 按文件名序重绑成员 1..N，纯 DB 操作；手动改过的标题保留 |
| GET | `/api/series/:id/members` | 系列成员（位次排序、允许缺失，含进度） |
| GET | `/api/series/:id/links` | 系列关联列表（方案 B 分组模型：返回与该系列同组的其他系列，互相可见） |
| PUT | `/api/series/:id/links` | 全量替换关联 `{ series_ids: [...] }`（勾选集即期望关联集合；该系列与勾选系列同组互相可见，取消勾选即不再关联） |
| DELETE | `/api/series/:id/links/:linkedId` | 移除单条关联（双向生效） |
| GET | `/api/series/:id/poster` | 系列海报（fallback 成员封面） |
| GET | `/api/tags` | 全部标签及出现次数（用于筛选器） |

> `/api/shows*` 多为历史遗留（Phase 3 旧接口），仅保留兼容；**唯一仍被前端使用的**是
> `PATCH /api/shows/:id` 的 `name` 字段——系列显示名（ADR-015 修订，系列与 show 1:1）。

### 2.4 播放 / 流媒体

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/stream/:id` | 直连播放（Direct）：HTTP Range 输出源文件（原生 `Content-Type`） |
| GET | `/api/stream/:id/remux` | 容器重封装（Remux）：浏览器可解码但容器不兼容（如 MKV h264+aac）→ 后端流拷贝成缓存 MP4（`-c:v copy -c:a copy`，faststart），HTTP Range 输出，全片可拖。**仅限音频可流拷贝（aac/mp3/无声）**；音频不可拷贝（AC3/EAC3/DTS/PCM）走 Transcode。`?audio=N` 选音轨（默认 0），按轨独立缓存 `<id>.mp4` / `<id>-a<N>.mp4` |
| GET | `/api/stream/:id/hls/playlist.m3u8?session=<uuid>` | 转码流（Transcode）：VOD 全量播放列表（关键帧对齐、`#EXT-X-ENDLIST`），hls.js 进度条即全片可拖；分片 URL 由播放列表内嵌并携带 `session`。`&audio=N` 选音轨（会话创建时固化，换轨须换新会话） |
| GET | `/api/stream/:id/hls/{file}` | 按需转码分片 `seg-{n}.ts`（libx264 + AAC，`?session=` 定位会话，首次请求当场生成并缓存） |
| GET | `/api/stream/:id/cover` | 封面图 |
| GET | `/api/stream/:id/subtitle` | 字幕（侧边 `.srt/.vtt/.ass` 优先；`?track=<index>` 指定内封文本轨，按需提取为 vtt） |
| GET | `/api/videos/:id/subtitles` | 字幕轨清单（侧边 + 内封文本轨，供播放器字幕菜单） |
| GET | `/api/videos/:id/audio` | 音轨清单（多音轨容器，index/codec/声道/label，供播放器音轨菜单选轨） |
| GET | `/api/cache` | 缓存概览：孤儿统计（封面/缩略图/字幕/remux）+ 字幕缓存按视频分组列表（含剧集系列标题）+ **播放选择记忆**列表（`prefs` 按视频、`series_prefs` 系列级共享） |
| DELETE | `/api/cache?kind=subtitle` | 清空全部字幕缓存（可重建，不影响源文件） |
| DELETE | `/api/cache/orphans` | 清空孤儿缓存（库中已无对应视频的残留文件） |
| DELETE | `/api/cache/subtitles/{videoId}` | 清空该视频全部字幕缓存 |
| DELETE | `/api/cache/subtitles/{videoId}/{track}` | 删除该视频指定轨的字幕缓存（track=-1 指旧式 `<id>.vtt`） |
| DELETE | `/api/cache/prefs` | 清空全部播放选择记忆（单集 + 系列级，删行后播放器回默认音轨/字幕/音量） |
| DELETE | `/api/cache/prefs/{videoId}` | 删除该视频的播放选择记忆 |

> 2026-08 修订（ADR-006）：**三层动态流**。可播放性由前端运行期 `canPlayType()` 核对（probe 元数据 →
> MIME/codecs），据此选择：
> - **Direct** — 浏览器原生可解码 → Range 直连 `/api/stream/:id`；
> - **Remux** — 编码可解但容器不兼容（MKV h264+aac 等）→ `/api/stream/:id/remux` 流拷贝成缓存 MP4 走
>   Range（纯拷贝，秒级完成）；仅限音频可流拷贝（aac/mp3/无声）；
> - **Transcode** — 编码不兼容（HEVC/rmvb/MPEG2 等）**或音频不可拷贝**（h264+AC3/EAC3/DTS/PCM）→
>   `/api/stream/:id/hls/*` 按需转码 HLS（VOD 全量列表 + 关键帧对齐分片 + `-mpegts_copyts 1 -output_ts_offset`
>   保持源时间轴）。音频不可拷贝时视频也会重编码（Matroska 流拷贝 seek 不可靠），但分片转码几秒即起播，
>   而整条音频重编码 ~70× 实时会卡死 Remux 整片生成。
>
> 视频详情响应与系列成员携带后端能力标志：`direct_playable`（保守 fallback）/ `remux_playable` /
> `transcode_playable`。三者皆不可时才引导格式工厂（保留为可选预转码工具）。

### 2.5 历史 / 搜索 / 设置

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/videos/:id/history` | 读取续播位置 |
| PUT | `/api/videos/:id/history` | `{ progress }` 保存（节流由前端控制） |
| DELETE | `/api/videos/:id/history` | 清空续播位置（单集详情页「清除历史」） |
| GET | `/api/videos/:id/prefs` | 读取**有效**播放选择记忆：`{ prefs, series_id }`。`prefs.scope` 区分 `series`（系列记录优先，含音轨/字幕**名称**，前端按当前集实际轨道解析）与 `video`（单集自身具体值）；无记录 `null`。`series_id` 在该视频归属系列时始终给出（播放器据此写系列级） |
| PUT | `/api/videos/:id/prefs` | 部分更新。归属系列的单集写**系列级**：`{ audio_track_name?, subtitle_name?, volume?, muted? }`（按名称共享）；独立单集写自身：`{ audio_track?, subtitle_id?, volume?, muted? }`。只写携带字段（播放器仅在用户手动切换时调用） |
| GET | `/api/series/:id/prefs` | 读取系列的共享播放选择记忆（音轨/字幕**名称** + 音量，无则 `null`） |
| DELETE | `/api/series/:id/prefs` | 删除系列的共享播放选择记忆（系列详情页「清除缓存」/ 缓存管理） |
| GET | `/api/home` | 首页行：继续观看 / 最近添加（单次拉取） |
| GET | `/api/search?q=` | 统一搜索（文件名 / 标签 / 剧名，FTS5；单集简介已移除 2026-09） |
| GET | `/api/jobs` | 任务队列状态（索引进度） |
| GET | `/api/health` | 健康检查 |

### 2.6 开发者工具日志归档

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/devlogs` | 提交一份前端日志归档 `{ source?, note?, entries: [{timestamp, level, module, message}] }` → `{ id }`（移动端把当前采集的日志传到后端保存） |
| GET | `/api/devlogs` | 归档记录列表（`{ items: [{id, source, note, count, created_at}] }`，不含 entries） |
| GET | `/api/devlogs/:id` | 取回一份归档（含 `entries` 逐字还原） |
| GET | `/api/devlogs/:id/raw` | 归档日志的纯文本（`text/plain`，每行一条）——非 GUI 方式，开发时按 ID 快速抓取日志 |
| DELETE | `/api/devlogs/:id` | 删除一份归档 |

> 数据模型：`devlogs` 表（见 §1.2）。前端采集采用「全局劫持 `console.*` + 带模块标记的
> `devLog()` logger」，环形缓冲上限 2000 条，仅在开关开启时采集（localStorage 持久化开关，App
> 启动即安装 hook）；归档接口只负责保存/取回，采集状态与缓冲为前端内存态。

## 3. 配置（config.yaml 示例）

```yaml
server:
  host: "0.0.0.0"          # 局域网访问
  port: 8080
  data_dir: "data"         # 默认 data（相对路径）
  static_dir: ""           # 前端构建产物（SPA）目录；空=自动探测 ./static 或 ../frontend/dist，均不存在则仅 API

auth:
  password: ""             # 空 → 首启自动生成并打印
  session_days: 30

# 多媒体源（media_sources）由前端在「文件」页签标记，不在 config.yaml 中声明；
# 首个全量扫描经 jobs 队列异步执行。

media:
  ffmpeg_path: "ffmpeg"    # 默认取 PATH 中的 ffmpeg/ffprobe
  ffprobe_path: "ffprobe"
  probe_concurrency: 2     # 后台 probe/thumbnail 任务并发数
```

## 4. AI 模块扩展契约（仅预留，默认不实现）

**目标**：保持主服务干净，AI 以独立进程/服务接入，通过**事件（Event）** + REST + 任务队列解耦；
AI 侧只需订阅 `VideoImported` 等事件，**不修改 Upload / 主流程**（ADR-010）。

| 能力 | 预留点 | 接入方式 |
|---|---|---|
| Whisper 转写 | `videos.transcript` 预留字段（见下表）+ jobs 增加 `whisper` 类型 | Listener 监听 `VideoImported` → 拉取源文件 → 回写 transcript + 关键词 |
| OCR | `frames` 任务：从关键帧 OCR | Listener 消费封面/抽帧结果 |
| AI 标签 / 摘要 | `video_tags`（AI 生成带 `source=ai` 标记）、`videos.summary` | Listener 回写元数据 API |
| Embedding | `videos.embedding_id` 预留 + 独立向量表 | Listener 维护向量库 |
| 智能搜索 | `search` 包 `SearchProvider` 新增 `ai` 实现即可 | 前端搜索框并列入口 |

```sql
-- Phase 4 启用时新增（初期不建表，避免空表）
ALTER TABLE videos ADD COLUMN transcript TEXT;
ALTER TABLE videos ADD COLUMN summary   TEXT;
ALTER TABLE videos ADD COLUMN embedding_id TEXT;
```

AI 服务侧建议：Python/Go 独立服务 + 队列（复用现有 jobs 表或独立消息），通过 API Key 与主服务通信。

## 5. 开发阶段计划

> **v1.0 的目标**：把「视频管理」做到每天愿意用——文件导入稳定可靠、缩略图生成快、播放流畅
> （续播 / 字幕 / 倍速 / 历史记录）、搜索与标签好用、视频库与文件页签切换自然；Library
> 达到**简易 JellyFin** 体验（海报墙、元数据、单集/系列分组、首页行）。基础层稳定之前**不引入 AI**，
> 避免基础层反复调整导致 AI 反复返工。

- **Phase 0 —— 骨架与认证（已完成）**：Go 后端（config/db/auth/api）+ React 前端脚手架 + 单口令会话认证闭环。
- **Phase 1 —— 文件管理与索引（已完成）**：泛用文件浏览器（files）盘符/pin/剪贴板式操作；**多媒体源**
  标记 + 全量扫描入库（jobs 队列 + ffprobe 探测 + 缩略图）。分块上传已随旧 Explorer 移除（2026-08）。
- **Phase 2 —— 播放与媒体体验（已完成）**：视频库 API + 网格页；三层动态播放（Direct Range / Remux 缓存
  MP4 / Transcode 按需 HLS，2026-08 修订）+ 封面；历史与续播；能力判定（前端 canPlayType + 后端
  direct/remux/transcode 标志）+ 侧边字幕；Vidstack 播放器。
- **Phase 3 —— 媒体库体验（已完成）**：数据迁移（videos 补元数据/分组列 + shows/seasons/video_tags + FTS5）；
  单集/系列模型与扫描归组（2026-08 管理面定稿，含系列间关联；2026-09 关联改显式分组方案 B）；FTS5 搜索；
  首页行、搜索、播放页元数据面板等前端页面。（2026-08：元数据刮削子系统已移除，仅留手动编辑）
- **Phase 4 —— AI 扩展（预留，默认不实现）**：仅在接口/数据上预留（见 §4），按需以独立 Go 服务或子模块接入，
  不耦合主服务。

> 每阶段完成后建议打 tag 并写简短的阶段小结（走查 ADR 与目录结构）。

### 5.1 测试策略

**后端**：`go test ./...`——domain 逻辑、store（`file::memory:` SQLite 或临时目录）、
api（`net/http/httptest` 全链路、mock 文件系统可注入）、scanner 指纹算法、events 总线。
媒体相关（ffprobe/ffmpeg）：打 `//go:build integration` 标签，CI/本机有 FFmpeg 时运行，否则跳过
（当前尚未编写 integration 测试）。关键路径必须覆盖：增量扫描去重、删除同步、FTS 同步、会话过期、
外接卷拔出→离线→插回→重扫（mock 卷枚举后端）、系列归组规则。

**前端**：Vitest + Testing Library（规划方向，**当前未配置**，见 status.md §2）——工具函数、组件
（列表、筛选、历史节流）。播放器相关只做轻量冒烟（真实媒体验证依赖手动/后续 Playwright e2e）。

**验收红线**：每个 Phase 的「验收」条目即该阶段完成判定；不得跳过测试进入下一阶段。

## 6. 风险与对策

| 风险 | 对策 |
|---|---|
| 大目录首次扫描耗时长/占资源 | 增量指纹 + 并发限制 + 进度可见 + 可中断重扫 |
| NTFS 硬链接/符号链接/大小写歧义 | 明确「只跟踪普通文件+目录」，符号链接默认跳过（可配置） |
| 源文件被外部程序改名/移动 | 以 file_id 校正（全局匹配，path 变化仍识别为同一视频）；last_scanned_at 校正删除 |
| 文件移动后数据库失效（仅按 path 识别时） | 以 (source_id, file_id, relative_path) 为身份，移动只更新路径/所属源 |
| 多媒体源根不可达被误判为「文件全部删除」 | 扫描前置可达性探测：不可达 → 中止、不动库；仅根可达且遍历完成才 MarkMissing |
| 嵌套/重叠多媒体源 | **新建源禁止嵌套**（`AddSource` 校验拒绝，400 `nested_source`）；历史遗留嵌套源由扫描路由表防御（子源拥有其子树，父源跳过子源子树，MarkMissing 不越界） |
| 剧集命名不规范导致归组错误 | 系列由用户手动创建（路径 + FileID 对应），识别错误由用户手动归组兜底 |
| 浏览器编码兼容（HEVC/MKV/AC3 等） | 前端运行期 canPlayType 核对（probe → MIME/codecs）：可直连 → Range；不可直连 → 动态流（视频可解**且音频可流拷贝 aac/mp3** 走 Remux MP4：容器重封装纯拷贝；视频不可解**或音频不可拷贝 AC3/EAC3/DTS/PCM** 走 HLS Transcode 按需转码，均后端生成）；三者皆不可 → 引导格式工厂转换 |
| 播放源 ffmpeg 未配置/源文件不可达 | 详情页 `remux_playable` / `transcode_playable` 为 false → 播放按钮禁用并引导格式工厂；Direct/Remux 端点返回 409 `storage_unavailable`，HLS playlist 返回 409 `stream_unavailable` |
| MKV 流拷贝按需 seek 不可靠（ffmpeg 对 Matroska cluster seek 依赖布局/版本） | Remux 层不切片：整片流拷贝成缓存 MP4（`-c:v copy -c:a copy`，仅 aac/mp3 音频）再走 Range；Transcode 层用重编码精确 seek（`-ss 关键帧`），实测内容与 PTS 均精确对齐 |
| AC3/EAC3/DTS/PCM 音轨（h264 视频本可解，但浏览器无 Dolby 解码器且整条音频重编码太慢） | h264+AC3/EAC3/DTS/PCM **不归入 Remux 层**（整片阻塞生成 + ~70× 实时音频重编码会卡死首播），改走 **HLS Transcode**：分片按需重编码视频+音频转 AAC，几秒即起播（见 media.md §4.1/§4.2）。原「Remux 仅音频转 AAC」方案实测首播 ~80 秒转圈被否决 |
| 转码分片 B 帧重排尾部与相邻分片 1 帧重叠 | MPEG-TS 固有现象，hls.js 按 EXTINF 累积时间轴 + timestampOffset 对齐，可容忍 |
| HLS 转码音频多声道不可播（x265 5.1 类文件卡 0:00:00） | ffmpeg 原生 AAC 对非标准布局（5.1(side)）输出 `chanCfg=0`+PCE，hls.js 推导 0 声道 esds 被 Chromium MSE 拒绝 → 无限重取分片。修复：>2 声道源重映射 `-channel_layout 5.1`（`chanCfg=6`）+ `-b:a 192k`，声道数经 `media.ProbeAudioChannels` 会话缓存（见 media.md §4.2） |
| SQLite 并发写冲突 | WAL + 单写者模式（写操作收敛到 store 层串行） |
| 多终端并发操作（两终端同时改名同一视频或文件） | 会话相互独立互不挤占；文件/DB 写操作收敛到 store/文件层串行；历史进度后写者胜；sessions 定期清理过期行 |
| 局域网弱设备（手机/TV）播放卡顿 | 缩略图 + 格式工厂转换为低码率 MP4；带宽提示 |
| 口令弱导致局域网内泄露 | 默认随机口令、登录限速（简单失败计数）、可选局域网 IP 白名单 |

## 7. 未来演进路线（不在本期）

1. PostgreSQL 迁移（repo 接口已隔离；**仅当 SQLite 真正遇到瓶颈才考虑**）。
2. Meilisearch / OpenSearch 替换 FTS5（`SearchProvider` 新增实现）。
3. AI 模块按 §4 落地（事件驱动，AI 作为 Listener 订阅 `VideoImported`）。
4. 多用户与权限（数据层已留 user 字段）。
5. 音频/图片资产支持（模型可扩展 `asset` 基类）。
6. 移动端 / PWA 离线壳。
7. 媒体转码队列化（批量预转码到统一格式）。
8. TV Mode（仅保留 Library 的电视端界面，ADR-013）。
9. 多媒体源变更监控：fsnotify 监视源目录，文件增/删/迁移自动反映到视频库（ADR-012 下阶段，复用统一
   `IngestPaths`/`EvictPaths` 入口，与 HomeReel 自身操作同一套归一化）。
10. 基于元数据与播放历史的轻量推荐（「你可能想看」行）——在 AI 落地前即可用规则实现。
