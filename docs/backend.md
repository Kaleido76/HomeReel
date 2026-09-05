# backend.md — 后端实现事实源

> 改动 `backend/internal/{store,scanner,fservice,files,jobs,events,db,config,realtime,logging}` 前必读。
> 架构决策与契约见 [decisions.md](decisions.md)；媒体管线见 [media.md](media.md)。

## 1. 时间戳

数据层统一纳秒 RFC3339（`domain.TimeLayout`），保证字典序=时间序。
例外：jobs 与 sessions 用秒级 `time.RFC3339`。

## 2. SQLite 单写者

- `SetMaxOpenConns(1)`，写操作收敛到 store 层
- **禁止在 rows 迭代期间发起新查询**——FTS 先收集 id 关闭 rows 再取详情

## 3. 迁移机制

`db.go` migrations 数组尾部追加；`splitStatements` 感知字符串字面量与 `BEGIN…END` 块。

## 4. 单集 / 系列组织

> 定稿语义见 decisions.md §0/015；表结构见 decisions.md §1.2。

- **扫描**：先同步文件（新增/未变 Touch/变更 UpdateFingerprint+probe/file_id 移动改路径/
  `MarkMissingBySource` 删消失行），再 `maintainSeriesMembers` 收敛成员
- **手动标记系列**：路径须在媒体源内；不得位于/包含既有系列
- **手动排序**：`order` 重写 `episode_number` 并置 `sort_manual=1`；`resort` 清 `sort_manual` 按文件名重绑
- **title_source 保护链**：手动编辑过的标题永不被扫描/探测覆盖
- **详情页按需检查**：单集 `CheckVideo` + `SyncVideo`；系列 `CheckSeries` + `SyncSeries`
- **系列关联**：`SetLinks` 全量替换组关系；`RemoveLink` 双向生效
- **单写权**：scan/mark/probe/sync 经 scanner 互斥锁串行

## 5. FTS5

`videos_fts` external content + 触发器，bm25 排序；`search_text` 由 store 层 `rebuildSearchText` 维护。
**新增影响搜索内容的写路径都要重建**。

## 6. 缓存

- `covers/thumbs/subtitles/remux` 与播放选择记忆都是**可重建缓存**
- 视频删除/源变更经 `streaming.RemoveCache` 自动清理
- 缓存管理 API 见 decisions.md §2.4

## 7. 扫描安全

- 多媒体源根不可达 → 扫描**中止、绝不动库**（ADR-014）
- 无 probe 元数据（`duration==0 && codec==""`）强制重 probe

## 8. jobs 与事件总线

- `Enqueue` 用户可见长时任务；`EnqueueInternal` 内部短任务（probe/thumbnail）
- 进度：`progress∈[0,1]`，`<0` 不确定；reporter 节流写库
- **统一进出口管线（ADR-017）**：`IngestPaths`（入库）与 `EvictPaths`（出库）
- 文件浏览器经 `SetLibraryNotifier` 挂钩：copy/convert → Ingest，move/rename → 先 Ingest 后 Evict，delete → Evict
- 多媒体源路由表：新建即禁嵌套；file_id 全局匹配
- convert 任务：完成后交 Ingest 即刻入库
- 实时推送：入队/reporter/Worker 收尾三时机经 Hub 推送（内部任务不发）

## 9. 文件浏览器（fservice）

> API 端点见 decisions.md §2.2。

- 独立机器级文件服务：「文件」页签；自身不索引（HTTP 文件服务器模型）
- 每个操作完成后经 `SetLibraryNotifier` 交给统一 ingest/evict 管线
- `/api/disks`（本地盘枚举）；`/api/files/list` 实时 readdir
- copy/move 入 jobs（字节进度，跨卷 move = copy+delete）
- worker 分发：main.go 把 fscopy/fsmove/convert 交给 fservice，probe/thumbnail/scan_source 交给 scanner

## 10. 后端日志

- 统一 `logging` 包：`Setup` 按 `log.level/format/file` 构建 slog handler 并经 `slog.SetDefault` 全库生效
- HTTP 访问日志路径分级：5xx→Error、4xx→Warn、业务 API→Info、高频路径→Debug
- 关键业务操作以操作级粒度记 Info
