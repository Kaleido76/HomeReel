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

## 9. jobs 队列与事件总线

- `jobs`：队列 + Worker（probe/thumbnail/rescan，崩溃恢复）；缩略图 covers 320px + thumbs 160px。
- 事件总线（ADR-010）：`VideoImported` 等事件，AI/OCR/转写作为 Listener，事件监听逻辑与主流程解耦。
