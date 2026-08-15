# backend.md — 后端与数据层约定

> 改动 `backend/internal/{store,scanner,fservice,files,jobs,events,db,config}` 等后端代码前必读。
> 架构级背景见 [AGENTS.md](../AGENTS.md) §4 ADR；媒体管线见 [media.md](media.md)。

## 1. 时间戳

统一固定宽度纳秒 RFC3339（`2006-01-02T15:04:05.000000000Z07:00`），保证字典序=时间序（recency 比较依赖）。
实现：`store.util.nowRFC3339` / `scanner.timeLayout` / `api.timeLayout`。

## 2. SQLite 单写者

- `SetMaxOpenConns(1)`，写操作收敛到 store 层。
- **禁止在 rows 迭代期间发起新查询（死锁）**——FTS 先收集 id 关闭 rows 再取详情，show 改名重建
  search_text 同理。

## 3. 迁移机制

- `db.go` migrations 数组尾部追加；一条迁移可含多语句。
- `splitStatements` 感知字符串字面量与 `BEGIN…END` 块（触发器内分号不会误切分）。

## 4. 单集 / 系列组织（2026-08 管理面定稿）

**管理面定稿**：单集是唯一基本实体（必须归属媒体源）；系列是用户显式创建的管理容器，绑定根目录
（`seasons.root_path`），成员 = 根目录**直接一级子文件**（不得嵌套），**系列显示名 = `shows.name`**（创建时
默认=文件夹名，创建后与文件夹脱钩、可编辑，扫描/同步/重新标记不覆盖；文件夹对应关系仅由 `root_path`
承载），成员标题 = 文件名。**系列只能手动创建**；自动扫描不建不删不改名系列定义，只维护既有系列成员关系。
归属判定以「路径 + FileID」为唯一事实源。

- **扫描（媒体源级）**：先同步文件（新增 / 未变 Touch / 变更 UpdateFingerprint+probe / file_id 移动改路径 /
  `MarkMissingBySource` 删消失行），再 `maintainSeriesMembers` 收敛成员关系（根目录直接子文件 → `BindMembers`
  文件名序 1..N；移出根目录 → `AssignStandalone` 脱离；空系列 `pruneEmptyShows` 清理）。
- **手动添加系列（文件夹级，`markSeries`）**：路径须在媒体源内（否则拒绝）；不得位于/包含既有系列（防
  嵌套）；对根目录直接一级子文件 `importCandidates` + `syncSeriesFolder`（`FindByRoot`/`CreateAtRoot` 幂等
  绑定 + `pruneVanishedMembers` 刷新消失成员 + 空系列清理）。
- **手动排序（2026-09，ADR-015 修订）**：`POST /api/series/{id}/order`（body `{ video_ids }` 须为该系列成员
  全集排列）重写 `episode_number` 1..N 并置 `seasons.sort_manual=1`。此后 `bindSeriesMembers`（扫描维护
  `maintainSeriesMembers` 与 `syncSeriesFolder` 共用）对 `sort_manual` 系列**保留既有成员顺序、仅把新入库
  文件按文件名追加到末尾**；普通系列仍按文件名序 1..N。**恢复自动模式（2026-09）**：`POST /api/series/{id}/resort`
  （`scanner.ResetSeriesSort`）清 `sort_manual=0` 并按文件名重绑成员 1..N，纯 DB 操作（不依赖根目录可达），
  此后扫描恢复「按文件名自动维护、新文件按文件名插入」。**按显示名排序（2026-09）**：前端按成员显示名
  （`episode_title || title`）字典序排列后走 `order` 接口（与拖拽重排同语义，置 `sort_manual=1`）。
- **详情页按需检查**：单集 `CheckVideo`（ok | moved | missing）+ `SyncVideo`（按 file_id 定位改名/移动）；
  系列 `CheckSeries`（根目录存在性 + 成员存在性 + 新文件）+ `SyncSeries`（对根目录执行一次与标记相同的局部
  同步）。
- **单写权**：scan / mark / probe / sync 经 `scanner` 互斥锁串行；同步 API 短写由 SQLite 单连接 +
  busy_timeout 保证。
- **覆盖规则**：路径 / 归属 / 文件名字段同步时一律覆盖为磁盘现状；标题 / 描述 / 标签等手动编辑字段保留。
  title 经 `videos.title_source`（`file`|`manual`）区分：手动编辑过（`manual`，`UpdateMetadata` 置位）
  永不被扫描/探测覆盖（`processInline`/`handleProbe` 尊重）；**系列成员标题同样受 `title_source` 保护
  （2026-09 修订）**：`bindSeriesMembers` 对 `title_source='manual'` 的既有成员保留当前标题，仅 `file`
  成员随文件名刷新（`BindMembers` 依 `EpisodeAssign.TitleSource` 写回，不再无条件置 `file`）——批量修改
  显示名称（`SeriesRenameModal`）依赖此保证；`UpdateMetadata` 手动改标题时对 episode 同步
  `episode_title`（成员列表显示 `episode_title || title`）。**系列显示名（`shows.name`）与单集
  `title_source` 同语义**：创建时写入
  文件夹名一次，之后仅 `PATCH /api/shows/{id}` 可改，扫描/标记/同步永不复写。
- **系列间关联（2026-09 方案 B：显式分组）**：`link_groups` + `link_group_members` 分组表，关联 = 同一组内
  系列互相可见（A 关联 B、C 时三者同组，打开任一方都看到另外两方），无名称、无方向。`SetLinks`（`PUT
  /api/series/{id}/links`，body `{series_ids}`）**全量替换**组关系：该系列与勾选系列同组，取消勾选即不再
  关联；每系列至多一组（`series_id` 唯一索引），成员脱离旧组、低于两员的组删除。`RemoveLink`（`DELETE
  /api/series/{id}/links/{linkedId}`）移除单条关联、双向生效。`SyncShowLinks` 把同 show 所有季并为一组
  （幂等，替代旧「相邻季自动关联」）。
- `videos_bd` 触发器删光某 show 最后一集时删空 show；空系列由 `pruneEmptyShows` 兜底。
- 数据结构：`seasons.root_path` 唯一（NULL 除外）；`manual_resources` / `videos.resource_id` 已移除
  （离散概念清除）。

## 5. FTS5（search_text）

- `videos_fts` external content + `ai/ad/au` 触发器，bm25 排序，`search_text` 由 store 层维护。
- 写入侧统一 `rebuildSearchText`（Create/UpdateProbe/UpdateMetadata/BindMembers/SetTags/show 改名）。
- **新增影响搜索内容的写路径都要重建**。

## 6. 封面

- 封面由扫描内联生成 `covers/<id>.jpg`；`UpdateCovers` 记录路径（空串=该列不动）。手动上传封面
  （`POST /api/videos/{id}/cover`）已移除（2026-08）。
- 元数据刮削（TMDB + NFO）已移除：无 `scrape` 包、无 `scrape.*` 配置项、无 `/scrape` API。
- **缓存清理（2026-08）**：`covers/`、`thumbs/`、`subtitles/`（内封字幕按需提取的 vtt）、`remux/`
  （按需流拷贝的 MP4 + 指纹 sidecar）都是**可重建缓存**。
  视频删除/源文件变更经 `RemoveCache`（`streaming`）自动清理；工具页「缓存管理」补充手动清空入口——
  `GET /api/cache`（`streaming.CacheStats`）按类统计条目数/占用/孤儿数，`DELETE /api/cache?kind=…` 清空对应
  类，`DELETE /api/cache/orphans`（`streaming.ClearOrphans`）清理库中已无对应视频的残留文件。孤儿判定由
  缓存文件名解析视频 ID（封面 `<id>`、缩略图 `<id>.thumb`、字幕 `<id>` / `<id>-<track>`、remux `<id>.mp4`
  及 `<id>.mp4.meta`）与全库索引对比。

## 7. 扫描安全

- 多媒体源根不可达 → 扫描**中止、绝不动库**（ADR-014 保护：移动硬盘没插时不误删元数据）。
- 无 probe 元数据（`duration==0 && codec==""`）强制重 probe（自愈）。

## 8. 上传清理

> 2026-08 已随旧 Explorer 移除（分块上传接口删除），无 staging 目录需清理。

## 9. jobs（长时任务管理器）与事件总线

- `jobs` 是**长时任务管理器**（ADR-008）：队列 + Worker（probe/thumbnail/scan_source/convert，崩溃恢复）；
  缩略图 covers 320px + thumbs 160px。
- **任务分类**：`Enqueue` 创建用户可见长时任务（scan_source/convert，带展示名 `name`）；`EnqueueInternal` 创建
  内部短任务（probe/thumbnail，`internal=true`，前端任务面板隐藏、不触发生命周期通知）。
- **整体进度**：`Job.Progress` 为 `[0,1]`；`<0` 表示不确定（前端显示动态条）。`Handler` 收到
  `jobs.Reporter`，`report.Progress` 节流写库（≤250ms 或 ≥1% 差值），handler 不调用即保持不确定。
  `Repo.MarkProgress`。
- **串行子任务（仅内存）**：`Reporter.Subtask(text)` / `SubtaskProgress(pct)` 就地替换当前子任务行，
  **不落库**——存入 `jobs.LiveStatus`（Service 与 Worker 共享的内存注册表），`handleJobsList` 经
  `Service.AttachLive` 合并到 job JSON 的 `subtask`/`subtask_progress`。大任务内部子任务严格串行执行，
  任务完成时从 LiveStatus 移除。
- **剩余时间估算（仅内存）**：`Reporter.Progress` 每次上报时按「已耗时 × (1−fraction)/fraction」推算剩余秒数，
  经 `LiveStatus.SetEta` 合并到 job JSON 的 `eta_seconds`（nil=未知）；只在任务报告确定进度时有意义。
  转换（`-progress pipe:1` + ffprobe 时长/文件大小估算）、扫描（文件数）、复制/移动（字节数）均受益。
- **扫描 = 串行内联**：`ScanSource(ctx, src)`；内部 `scan(ctx, src, progress(done,total), subtask)`。
  每个待处理文件在**主循环内串行**执行「探测 → 生成缩略图」（新增文件在探测后**单条 INSERT 全量元数据**，
  原子；变更文件走 UpdateProbe 单条 UPDATE），并逐文件上报子任务「探测 X」「生成 X 的缩略图」。
  扫描结束执行 `maintainSeriesMembers`（对既有系列维护成员关系，不创建系列）；内联路径不发布
  `VideoImported`（避免监听器重复生成缩略图）。手动 refresh 仍走队列 + 监听器。代价：首次全量扫描明显
  变慢（串行避免并行 ffmpeg 抢占卡顿），增量扫描仅处理变更文件。库写长任务（scan/mark/probe/sync）经
  `scanner` 互斥锁串行（单写权）。
- **生命周期通知**：`Worker.SetNotify` 在任务落库后回调 `(job, err)`，`main.go` 据此发布
  `events.JobDone` / `events.JobFailed`（ADR-010 事件总线）。
- **统一进口/出口管线（ADR-017）**：扫描、标记、同步与文件浏览器操作共用**同一套归一化函数**
  `normalizeCandidate`（probe + file_id 指纹 + 建行/重定位；`scanner/ingest.go`）。scanner 暴露两个统一
  入口：`IngestPaths(paths)`（文件进入维护范围：定位所属媒体源 → 建行/重定位 → 系列收敛 →
  `VideoImported`/`VideoUpdated`）与 `EvictPaths(paths)`（文件离开维护范围：删行 + 系列收敛 +
  `VideoDeleted`）。文件浏览器经 `fservice.SetLibraryNotifier` 注入回调挂钩：copy/convert 产物 → Ingest，
  move/rename → **Ingest(新路径) 先、Evict(旧路径) 后**（保住 file_id 身份与历史/手动元数据），delete →
  Evict。任何经 HomeReel 的文件变更在**操作完成瞬间**即归一化，不再等下次扫描；未来 upload / fsnotify
  复用同一入口。单文件 Ingest 经 `VideoImported` 走异步缩略图任务，bulk 扫描内联生成（不重复发布事件）。
- **多媒体源路由表**：媒体源**新建即禁止嵌套**（`fservice.AddSource` 校验，路径位于/包含既有源 →
  400 `nested_source`）；路由表（`files.UnderRoot(o.Path, src.Path)` 且非自身 → `collect` 整棵 `SkipDir`；
  `MarkMissingBySource` 跳过子源根下各行）仅防御**历史遗留**嵌套源。file_id **全局匹配**：跨源移动仅更新
  path/source_id/relative_path，不新建记录。
- **convert 任务**：`fservice.HandleConvert`（`fservice/convert.go`）——先 ffprobe 探测流（音频是否浏览器
  可播、有无字幕），文件夹按「已完成文件数/总数」上报确定进度 + 子任务「转换 <名>（n/N）」，逐文件收集失败
  为任务错误；`convertMeta` 携带操作面板的 `ConvertParams`（video/crf/audio/akbps/burn，`norm()` 归一化）。
  预设、两级尝试链与进度算法详见 [media.md](media.md) §3.1。转换完成后把**实际产物路径**交给统一
  `Ingest`，产物立即入库。
- 事件总线（ADR-010）：`VideoImported` 等事件，AI/OCR/转写作为 Listener，事件监听逻辑与主流程解耦。

## 10. 泛用文件浏览器（fservice，2026-08 增量）

- 独立于视频库的**机器级文件服务**（「文件」页签）：绝对路径列目录 + 剪贴板式 copy/move/rename/delete。
  自身不索引；每个操作完成后经 `SetLibraryNotifier` 把受影响路径交给 scanner 的统一 ingest/evict 管线
  （ADR-017），使库在操作完成瞬间即一致。仅 pin 与多媒体源标记持久化。
- `fservice` 包（`internal/fservice`）：`/api/disks`（Windows 本地盘枚举，fixed/removable，排除网络盘，
  build-tag 实现）；`/api/files/list?path=`（实时 readdir，**Lstat 读条目自身属性过滤 Windows
  HIDDEN/SYSTEM 条目**——$RECYCLE.BIN、System Volume Information、desktop.ini 等特殊目录/文件不出现在
  列表，非名称过滤；junction 取其自身属性而非目标；再 Stat 取目录/尺寸/时间）；`/api/files/rename|delete`
   （同步）；`/api/files/copy|move`（**入 jobs** `fscopy`/`fsmove`，后台任务带字节进度，跨卷 move 自动
   copy+delete，复制跳过符号链接/junction 防环路）；`/api/files/pins`（增删查，settings 键 `files.pins`）。
  - **多媒体源**（`media_sources` 表，见 [decisions.md](decisions.md) §2.2）：`/api/files/sources`（GET 列表，附每源
    `available`=根可达性、`scanning`=有无进行中 scan_source 任务）、`POST`（标记 + 入队 `scan_source`
    Job；**禁止嵌套**——路径位于/包含既有源时返回 400 `nested_source`）、`DELETE`（取消标记：**先按源删除其下全部视频** `videos.DeleteBySource`，逐视频发布
    `VideoDeleted` 清缓存，再删标记——该源的单集与系列随之全部从库中消失；`videos_bd` 触发器随最后成员
    清空系列；磁盘文件不受影响）、`/api/files/sources/scan`（手动重扫）。源路径在
    `fservice.sources.go` 做规范化（`filepath.Clean`，盘符根保留尾分隔符）以便路由前缀比较。
- worker 分发：main.go 把 `fscopy`/`fsmove`/`convert` 交给 `fsvc.HandleJob`，其余（probe/thumbnail/
  scan_source）交给 `scannerSvc.HandleJob`。
- **格式工厂（2026-09，替代原「重封」页签）**：`POST /api/convert`（body 含 `paths` + 可选 `params`）→
  `fservice.EnqueueConvert` 逐个入队 `convert` 任务；`POST /api/convert/probe` → `fservice.ProbeConvert`
  逐个 ffprobe 返回流事实供操作面板指引/禁用。转换路径（单集→同目录 `X.mp4`、系列/文件夹→同级
  `X (MP4)\`，collision 时 ` (N)` 递增）、预设与两级策略详见 [media.md](media.md) §3.1。
