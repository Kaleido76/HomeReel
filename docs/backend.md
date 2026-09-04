# backend.md — 后端与数据层约定

> 改动 `backend/internal/{store,scanner,fservice,files,jobs,events,db,config,realtime,logging}` 前必读。
> 架构背景见 [AGENTS.md](../AGENTS.md) §4 ADR；媒体管线见 [media.md](media.md)；实时通道见 [realtime.md](realtime.md)。

## 1. 时间戳

数据层统一固定宽度纳秒 RFC3339（`2006-01-02T15:04:05.000000000Z07:00`），保证字典序=时间序
（recency 比较依赖）。实现：`domain.TimeLayout` / `domain.Now()`。
**例外**：jobs 与 sessions 用秒级 `time.RFC3339`（不参与 recency 比较）。

## 2. SQLite 单写者

- `SetMaxOpenConns(1)`，写操作收敛到 store 层。
- **禁止在 rows 迭代期间发起新查询（死锁）**——FTS 先收集 id 关闭 rows 再取详情，show 改名重建
  search_text 同理。

## 3. 迁移机制

- `db.go` migrations 数组尾部追加；一条迁移可含多语句。
- `splitStatements` 感知字符串字面量与 `BEGIN…END` 块（触发器内分号不会误切分）。

## 4. 单集 / 系列组织

定稿语义（ADR-015）见 AGENTS.md，表结构语义见 [decisions.md](decisions.md) §1.2。实现事实：

- **扫描（媒体源级）**：先同步文件（新增 / 未变 Touch / 变更 UpdateFingerprint+probe /
  file_id 移动改路径 / `MarkMissingBySource` 删消失行），再 `maintainSeriesMembers` 收敛成员
  （根目录直接子文件 → `BindMembers` 文件名序 1..N；移出根 → `AssignStandalone`；空系列
  `pruneEmptyShows` 清理）。扫描在主循环内**逐文件串行**「探测→缩略图」（新增单条 INSERT 全量元数据），
  且不发布 `VideoImported`——避免监听器重复生成缩略图；手动 refresh 仍走队列 + 监听器。
- **手动标记系列（`markSeries`）**：路径须在媒体源内；不得位于/包含既有系列（防嵌套）；
  `importCandidates` + `syncSeriesFolder`（`FindByRoot`/`CreateAtRoot` 幂等绑定 +
  `pruneVanishedMembers`）。
- **手动排序 / 恢复自动**：`order` 重写 `episode_number` 并置 `sort_manual=1`，此后
  `bindSeriesMembers` 只把新入库文件按文件名追加到末尾；`resort` 清 `sort_manual` 按文件名重绑，
  纯 DB 操作、不依赖根目录可达。「按显示名排序」= 前端按 `episode_title || title` 排序后走 `order`，同语义。
- **title_source 保护链**：手动编辑过（manual）的单集标题永不被扫描/探测覆盖；
  `bindSeriesMembers` 对 manual 成员保留标题、仅 file 成员随文件名刷新（批量改名
  `SeriesRenameModal` 依赖此保证）；`UpdateMetadata` 手动改标题同步 `episode_title`
  （成员列表显示 `episode_title || title`）。系列显示名 `shows.name` 同语义：创建写一次，
  之后仅 `PATCH /api/shows/{id}` 可改。
- **详情页按需检查**：单集 `CheckVideo`（ok/moved/missing）+ `SyncVideo`；系列 `CheckSeries`
  （根目录 + 成员存在性 + 新文件）+ `SyncSeries`（对根目录执行一次局部同步）。
- **系列关联**：`SetLinks` 全量替换组关系（成员脱离旧组、低于两员的组删除）；`RemoveLink` 双向生效；
  `SyncShowLinks` 把同 show 所有季并为一组（幂等）（ADR-018）。
- **单写权**：scan/mark/probe/sync 经 scanner 互斥锁串行；短写由 SQLite 单连接 + busy_timeout 保证。
- **覆盖规则总则**：路径/归属/文件名字段一律覆盖为磁盘现状；手动编辑字段保留。

## 5. FTS5

- `videos_fts` external content + ai/ad/au 触发器，bm25 排序；`search_text` 由 store 层
  `rebuildSearchText` 维护（Create/UpdateProbe/UpdateMetadata/BindMembers/SetTags/show 改名）。
- **新增影响搜索内容的写路径都要重建**。

## 6. 缓存

- `covers/thumbs/subtitles/remux` 与播放选择记忆行都是**可重建缓存**；视频删除/源变更经
  `streaming.RemoveCache` 自动清理。
- 工具页缓存管理入口：`GET /api/cache` 概览（孤儿统计）、`DELETE /api/cache?kind=subtitle`
  （唯一支持的 kind）、`DELETE /api/cache/orphans`（孤儿 = 从缓存文件名解析出的视频 ID 不在全库索引）。
- 系列级批量操作（ADR-023）：`DELETE /api/cache/series/{id}/subtitles` 一次清空系列全部成员的字幕缓存、
  `DELETE /api/cache/series/{id}/remux` 一次清空该系列全部成员的 Remux 缓存；
  `POST /api/cache/pregen`（body `{series_id}` 或 `{video_ids}`）入队 `pregen` Job 预提取内封文本字幕——
  worker 对每个视频探测文本字幕轨并写出 `<id>-<track>.vtt`（与播放按需提取同一命名），已存在跳过；
  进度按视频报告（Subtask/Progress）。payload 携带 enqueue 时解析的 `{video_id,title,path}`，worker
  不依赖视频库；job 由 `streaming.HandlePregen` 执行（`cmd/server/main.go` 分发）。
- 单视频清理：`DELETE /api/cache/remux/{videoId}`（`streaming.ClearRemux`）。概览 `GET /api/cache` 的 `remuxes`
  数组按视频报告 Remux 归属（`streaming.ListRemuxCache`，排除 `.meta` 指纹 sidecar），供前端按系列/单集展示占用
  并在归属处清理（无归属的 Remux 走孤儿清理）。
- 元数据刮削（TMDB+NFO）与封面手动上传均已移除：封面由扫描内联生成。

## 7. 扫描安全

- 多媒体源根不可达 → 扫描**中止、绝不动库**（ADR-014）。
- 无 probe 元数据（`duration==0 && codec==""`）强制重 probe（自愈）。

## 8. jobs 与事件总线

- `Enqueue` 创建用户可见长时任务（带展示名）；`EnqueueInternal` 内部短任务
  （probe/thumbnail，任务面板隐藏、不发通知）。
- 进度：`progress∈[0,1]`，`<0` 不确定；reporter 节流写库（≤250ms 或 ≥1%）。剩余时间估算与
  子任务文本只存内存 `LiveStatus`（不落库），`AttachLive` 合并进 job JSON。
- 统一进出口管线（ADR-017）：单一归一化函数 `normalizeCandidate`（scanner/ingest.go）；
  `IngestPaths`（定位所属源→建行/重定位→系列收敛→事件）与 `EvictPaths`（删行+收敛+`VideoDeleted`）
  两个入口。文件浏览器经 `SetLibraryNotifier` 挂钩：copy/convert 产物 → Ingest，
  move/rename → **先 Ingest 新路径再 Evict 旧路径**（保 file_id 身份与历史），delete → Evict；
  未来 upload/fsnotify 复用同一入口。
- 多媒体源路由表：新建即禁嵌套（`AddSource` → 400 `nested_source`）；扫描路由表跳过子源子树
  仅防御历史遗留嵌套。file_id 全局匹配：跨源移动只更新 path/source_id/relative_path。
- convert 任务（`fservice.HandleConvert`）：文件夹按完成数上报进度、逐文件收集失败为任务错误；
  完成后把实际产物路径交 Ingest 即刻入库。转换策略见 media.md §3.1。
- 生命周期：`Worker.SetNotify` 在任务落库后回调 main.go 发布 `JobDone/JobFailed`（ADR-010）。
- 实时推送：入队 / reporter 上报 / Worker 收尾三时机把用户任务完整快照经 Hub 以 `jobs.progress`
  推送（内存态不落库，internal 任务不发）；前端运行期零 `GET /api/jobs` 轮询（realtime.md §6）。

## 9. 泛用文件浏览器（fservice）

- 独立机器级文件服务：「文件」页签。自身不索引（HTTP 文件服务器模型）；每个操作完成后经
  `SetLibraryNotifier` 交给统一 ingest/evict 管线，库在操作完成瞬间即一致。仅 pin 与多媒体源持久化。
- `/api/disks`（本地盘枚举，build-tag 实现）；`/api/files/list` 实时 readdir——**Lstat 读条目自身属性
  过滤 HIDDEN/SYSTEM**（非名称过滤；junction 取自身属性）；rename/delete 同步；copy/move 入 jobs
  （字节进度，跨卷 move = copy+delete，复制跳过符号链接/junction 防环路）；pins 存 settings 键
  `files.pins`。sources 端点见 decisions.md §2.2；源路径规范化（`filepath.Clean`，盘符根保留尾分隔符）
  以便路由前缀比较。
- worker 分发：main.go 把 fscopy/fsmove/convert 交给 fservice，probe/thumbnail/scan_source 交给 scanner。

## 10. 后端日志（logging）

- 统一组件 `backend/internal/logging`（ADR-022）：`Setup` 按 `log.level/format/file` 构建 slog handler
  并经 `slog.SetDefault` 全库生效——各包继续用包级 `slog.*`，零改造。级别 `debug|info|warn|error`
  （默认 info）；格式 `text|json`（默认 text，时间戳本地毫秒级）。
- 输出：stderr + 可选 `log.file`（追加写；启动时旧文件轮转为 `<base>-<YYYYMMDD><ext>`，不做后台滚动，
  见 ADR-022）。文件句柄在服务关闭时释放。
- HTTP 访问日志（`logging.AccessLog`，路径分级，详略得当）：5xx→Error、4xx→Warn；2xx/3xx 业务 API→Info；
  `/api/stream/*` 与 SPA 静态资源等高频路径→Debug（默认 info 级别静默，播放 Range/HLS 分片不刷屏）。
  字段：`method/path/remote/status/bytes/duration`（Range 请求附加 `range`）。
- 关键业务操作以操作级粒度记 Info（非逐文件）：多媒体源增/删/重扫（api/files.go）、系列标记
  （scanner/mark.go）、转换/复制/移动任务完成（fservice，带产物数/源数）；扫描完成见 §8。
