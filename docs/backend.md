# backend.md — 后端与数据层约定

> 改动 `backend/internal/{store,scanner,storage,files,jobs,events,db,config}` 等后端代码前必读。
> 架构级背景见 [architecture.md](architecture.md)；媒体管线见 [media.md](media.md)。

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

## 4. 剧集/系列组织

- 库 = 单集（`show_id IS NULL`）+ 系列（`seasons` 行，一季/一部一个，`kind`=tv/movie），成员按位次排序允许缺失。
- 分组唯一来源：`scanner.ParseEpisode`（`SxxEyy`/`第x集`/`Season N` 目录/中文数字）与
  `scanner.ParseMoviePart`（`Part N`/`第N部`/数字后缀）。
- **默认单集**，仅当位于 `Season N` 目录、或同目录 ≥2 个标题键相同/编辑距离 ≤2（`scanner.editDistance`）才归系列。
- `groupVideo` 在 **Scan 结束时统一对 `toGroup` 执行**（同目录兄弟可见），也用于 ImportUploaded/probe/手动归组；
  老库数据在 unchanged 分支回填。
- `series_links` 无名称、`sort_index` 排序，同 show 相邻季 `SyncShowLinks` 自动关联，
  `/api/series/{id}/links` 手动增删。
- `videos_bd` 触发器删光某 show 最后一集时删空 show；删空 season 由扫描/后续兜底。

## 5. FTS5（search_text）

- `videos_fts` external content + `ai/ad/au` 触发器，bm25 排序，`search_text` 由 store 层维护。
- 写入侧统一 `rebuildSearchText`（Create/UpdateProbe/UpdateMetadata/AssignEpisode/AssignMovie/SetTags/show 改名）。
- **新增影响搜索内容的写路径都要重建**。

## 6. 封面

- 手动上传 `POST /api/videos/{id}/cover` 落盘 `covers/<id>.<ext>`；`UpdateCovers` 空串=该列不动。
- 元数据刮削（TMDB + NFO）已移除：无 `scrape` 包、无 `scrape.*` 配置项、无 `/scrape` API。

## 7. 扫描安全

- 卷根不可达中止扫描、绝不误删元数据（ADR-014）。
- 无 probe 元数据（`duration==0 && codec==""`）强制重 probe（自愈）。

## 8. 上传清理

- 合并完成即删 staging；启动时 + 每小时清理超 24h 孤儿分片。

## 9. jobs（长时任务管理器）与事件总线

- `jobs` 是**长时任务管理器**（ADR-008）：队列 + Worker（probe/thumbnail/rescan/remux，崩溃恢复）；
  缩略图 covers 320px + thumbs 160px。
- **任务分类**：`Enqueue` 创建用户可见长时任务（scan/remux，带展示名 `name`）；`EnqueueInternal` 创建
  内部短任务（probe/thumbnail，`internal=true`，前端任务面板隐藏、不触发生命周期通知）。
- **整体进度**：`Job.Progress` 为 `[0,1]`；`<0` 表示不确定（前端显示动态条）。`Handler` 收到
  `jobs.Reporter`，`report.Progress` 节流写库（≤250ms 或 ≥1% 差值），handler 不调用即保持不确定。
  `Repo.MarkProgress`。
- **串行子任务（仅内存）**：`Reporter.Subtask(text)` / `SubtaskProgress(pct)` 就地替换当前子任务行，
  **不落库**——存入 `jobs.LiveStatus`（Service 与 Worker 共享的内存注册表），`handleJobsList` 经
  `Service.AttachLive` 合并到 job JSON 的 `subtask`/`subtask_progress`。大任务内部子任务严格串行执行，
  任务完成时从 LiveStatus 移除。
- **扫描 = 串行内联**：`Scan(ctx, st)` 保持原签名；内部 `scan(ctx, st, progress(done,total), subtask)`。
  每个待处理文件在**主循环内串行**执行「探测 → 生成缩略图」（`processInline`，逐文件上报子任务
  「探测 X」「生成 X 的缩略图」），不再 enqueue 探测 job；单文件失败不致命（下次扫描自愈）。内联路径
  不发布 `VideoImported`（避免监听器重复生成缩略图）；上传/手动刷新仍走队列 + 监听器。代价：首次
  全量扫描明显变慢（串行避免并行 ffmpeg 抢占卡顿），增量扫描仅处理变更文件。
- **生命周期通知**：`Worker.SetNotify` 在任务落库后回调 `(job, err)`，`main.go` 据此发布
  `events.JobDone` / `events.JobFailed`（ADR-010 事件总线）。
- **资源锁**：`Service.HasActive(type, target)` 查询指定资源的 queued/running 任务。rescan 的
  `target=storage_id`，扫描期间该存储卷 `Busy=true`：`/api/storages` 与 `/api/fs/list` 响应带上
  `busy` 标记；FS 变更操作（mkdir/rename/move/delete）、上传、存储卷 patch/delete/refresh、重复扫描
  均拒绝（409 `storage_busy`），直到扫描结果落库后自动解锁。
- **remux 进度**：`handleRemux` 用 `-progress pipe:1` 解析 `out_time_us`，结合 `videos.duration` 得出
  确定进度；时长未知则保持不确定。
- 事件总线（ADR-010）：`VideoImported` 等事件，AI/OCR/转写作为 Listener，事件监听逻辑与主流程解耦。
