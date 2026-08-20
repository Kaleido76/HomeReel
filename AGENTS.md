# AGENTS.md — HomeReel 项目指南

> 本文档是参与本项目的**必读指导文档**。
> 参与本项目前，必须首先完整阅读本文档。

---

## 1. 项目是什么

HomeReel 是部署在家庭局域网的**个人视频资料管理与播放平台**，经 WebUI 供 PC / 手机 / 平板 / TV 等终端设备访问。
包括以下功能：
+ （类似 JellyFin）视频平台（元数据、剧集/季分组、继续观看 等）
+ 基础文件管理（浏览 / 上传 / 下载 / 重命名 / 移动 / 删除 等）
+ 单用户、单访问口令，但 **支持多终端并发访问**（各自独立会话，互不挤占）
+ 优先部署在 Windows 平台（跨平台能力需要注意，但不是高优先级）

### 1.1 技术栈

**前端**
+ 语言 / 框架：React 19 + TypeScript、Vite、Tailwind CSS 4
+ 数据层：TanStack Router / Query
+ 播放器：Vidstack；Direct/Remux 走 HTTP Range，Transcode 走 HLS + 本地 hls.js（不走 CDN）
+ 代码检查：oxlint（`pnpm run lint`）
+ 包管理器：一律使用 `pnpm`，**禁止** `npm` / `cnpm` / `yarn`

**后端**
+ 语言：Go（`go.mod` 指定 1.26）+ 标准库 `net/http`（1.22+ `PathValue` 路由）与 `slog`
+ 数据库：SQLite（`modernc.org/sqlite`，纯 Go 免 CGO，WAL 模式）
+ 其他依赖：`oklog/ulid/v2`（主键）、`fsnotify`（文件监听）、`gopkg.in/yaml.v3`（配置）、`golang.org/x/crypto`（口令哈希）

**媒体**
+ FFmpeg / ffprobe：探测、缩略图、字幕、格式工厂转换、Remux 流拷贝、Transcode 按需转码

**播放策略（ADR-006）**
+ 三层动态流（Direct / Remux / Transcode / 格式工厂兜底），判定与实现详见 [media.md](docs/media.md) §4 与 `frontend/src/lib/playability.ts`

**部署**
+ `CGO_ENABLED=0` 单 `.exe` + `data_dir` + `config.yaml` + ffmpeg 二进制
+ 开发环境 / 部署细节见 [docs/environment.md](docs/environment.md)；UI 视觉规范见 `UI.md`

## 2. 文档体系

> 当前文档只承载必读的通用信息；开始工作前按需阅读下表中的文档。

| 文档 | 作用 | 何时读 |
|---|---|---|
| 本文档 §4 | **关键决策记录（ADR）**：架构「为什么这样」的依据，决策修订必须留痕 | 理解设计取舍、评估架构影响 |
| `docs/decisions.md` | 契约：数据模型（SQL）/ API 契约 / 配置示例 / AI 扩展预留 / 演进路线 / 风险对策 | 查表结构、API、config.yaml、未来方向 |
| `docs/environment.md` | 本机开发环境、FFmpeg PATH 陷阱、部署形态/静态托管/启动输出 | 搭建环境、部署、排障 |
| `docs/backend.md` | 后端与数据层实现事实源（时间戳/SQLite/迁移/FTS5/系列归组/多媒体源/jobs/事件总线） | 改动 `backend/internal/{store,scanner,fservice,files,jobs,events,db,config}` |
| `docs/media.md` | 媒体管线实现事实源（ffprobe/容器判定/分段 MP4/格式工厂/能力判定/字幕/封面） | 改动 `backend/internal/{media,streaming}` 或前端播放器媒体相关 |
| `docs/frontend.md` | 前端实现事实源（页签 keep-alive/栏位栈/Vidstack/响应式/文件浏览器/卡片） | 改动 `frontend/src/{tabs,features}` |
| `docs/status.md` | 现状快照/遗留待办/人工验证清单/未来方向 | 规划新功能、验收、会话交接 |
| `UI.md` | UI 设计语言（视觉规范；与实现冲突处以 frontend.md §5/§6 为准） | 改动前端样式/布局前 |
| `Note.md` | 用户草稿 | 永不阅读 |

**规则**：改代码或架构前先读本文档 §4 的 ADR 与 `docs/decisions.md` 契约；实现细节以各领域文档为准。
发现文档与代码不一致时，以 ADR 与契约为准，并向用户指出差异。

## 3. 核心设计原则

> 整个系统在任何时候都必须考虑的原则性内容

**一切功能围绕「元数据」扩展**（最高级原则）
+ 文件存储功能全权由文件系统承载
+ 数据库中的元数据全权承载查找、理解与组织功能

**功能正确优先，禁止过度优化**
+ 保守演进，未见真实性能瓶颈、缺陷时，禁止预先设计复杂的并发优化、抽象架构等

**AI 友好与长期维护**
+ 模块化、低耦合、显式优先、避免深层继承与过度抽象的设计模式
+ 保持逻辑流畅，可阅读性强

## 4. 关键决策记录（ADR）

> 架构「为什么这样」的依据。**新增/修订决策必须在此留痕**（追加行或修订结论），并同步回填受影响的关联章节。

| # | 决策 | 结论 | 理由 |
|---|---|---|---|
| ADR-001 | 后端形态 | 单个 Go 服务，内部按包拆分（api/auth/config/db/domain/store/files/fservice/events/scanner/jobs/media/streaming/search）；**v1.0 全程不拆服务** | 家庭部署单个进程 + 单个 sqlite 完全够用；拆服务带来四套日志/配置/部署成本，等 AI/OCR/Whisper 真正变重再拆 |
| ADR-002 | 认证模型 | 单口令 + 会话 Cookie，**支持多终端并发会话**（各终端独立会话，登出/过期互不影响），无用户概念，数据中 `user` 固定为 `local` | 个人使用；多终端同时在线互不挤占；登录页实现局域网访问保护 |
| ADR-003 | 部署形态 | 纯 Web 应用，监听 `0.0.0.0`，浏览器访问 | 任何局域网设备（PC/手机/平板/TV）无需安装即可用 |
| ADR-004 | 文件入库 | 目录扫描 + **多媒体源标记**（`media_sources`）驱动视频库；手动分块上传与统一上传接口已随旧 Explorer 移除（2026-08，files 上传能力待后续补） | 入库只依赖「用户标记的目录」，与文件浏览完全解耦 |
| ADR-005 | 数据库 | SQLite（纯 Go 驱动 + WAL 模式）；演进路线：WAL → FTS5 → Background Queue → Read Cache → 仅真正遇瓶颈才考虑 PostgreSQL | Windows 免 CGO、零依赖、单文件；个人量级瓶颈是 ffmpeg 而非数据库 |
| ADR-006 | 播放策略 | **三层动态流（2026-08 修订，替代纯 Range 直连）**：前端把 probe 元数据映射成 MIME/codecs 串，用 `canPlayType()` 核对目标机能力决定播放层——Direct（浏览器原生可解码）→ HTTP Range 直连；Remux（编码可解但容器不兼容，如 MKV h264+aac）→ 后端**整片流拷贝成缓存 MP4** 走 Range（全片可拖）；Transcode（编码不兼容 HEVC/rmvb/DTS 等，**或音频不可拷贝 AC3/EAC3/DTS/PCM**——浏览器无 Dolby 解码器且整条音频重编码过慢会卡死 Remux 整片生成）→ **按需转码 HLS**（VOD 全量列表 + 关键帧对齐 + 每会话独立）。三者皆不可才引导格式工厂（保留为可选预转码工具）。**多音轨选轨（2026-09）**：播放器菜单选轨后 Remux 按轨缓存、Transcode 以新会话重建流、Direct 暂用默认轨。**播放选择记忆（2026-09 修订）**：单集级 + 系列级两层缓存音轨/字幕/音量——系列剧集共享同一记忆（音轨/字幕按**轨道名称**匹配，系列有记录时优先于单集记录、单集记录作关系重组兜底），详情页可查看并清除。实现细节（命令/缓存命名/音轨号传递）见 [media.md](docs/media.md) §4/§6。**伪全屏（2026-09 修订）**：触摸/移动端原生全屏不可用（iOS `<div>` 不支持 `requestFullscreen`）或会盖住自定义控制条/WebVTT 字幕，故移动端全屏按钮走**纯 CSS 伪全屏**（`position:fixed` 铺满 `dvh/dvw` 盖住应用 + 竖屏设备对横屏内容 `rotate(90deg)`），桌面端保持原生全屏；由 `PlayerFullscreenButton` 覆盖默认 `fullscreenButton` slot，按 `pointer: coarse` 判定设备分发。实现细节见 [frontend.md](docs/frontend.md) §3。**实测依据**：ffmpeg 对 Matroska 流拷贝按需 seek 不可靠，故 Remux 整片流拷贝不切片；Transcode 用重编码精确 seek，内容与 PTS 均精确 | 原「纯 Range + 引导转换」要用户手动操作；Remux 一次流拷贝成本极低且结果全片可拖；Transcode 只转用户实际看的部分。判定机制运行期固化（前后端协同），不逐格式堆硬编码布尔 |
| ADR-007 | 文件身份 | `(source_id, file_id, relative_path)` 三元组唯一；**file_id 全局匹配**（跨源移动仍识别为同一视频）；`(file_id, size, mtime)` 为变更指纹；`hash`（SHA-256）为可选后台任务（**2026-09 修订**：videos 表无 hash 列、无 hash 任务类型，当前未实现，仅保留扩展空间） | 文件移动目录（甚至移动出原多媒体源、进入另一源）后仍能识别为同一视频；大文件全量哈希代价高，首次索引快速 |
| ADR-008 | 任务队列 | SQLite 持久化 `jobs` 表 + 进程内 worker 池 | 可展示进度、崩溃可恢复、实现简单 |
| ADR-009 | 搜索 | 定义 `SearchProvider` 接口；当前实现 SQLite FTS5（反规范化 `search_text`），Meilisearch/AI 检索后续新增实现 | 控制器不直接写 SQL；替换搜索引擎无需改动 Controller |
| ADR-010 | AI 模块 | 通过**事件（Event）**解耦：`VideoImported` 等事件广播，AI/OCR/缩略图/转写作为 Listener 监听 | 各能力独立演进，AI 接入无需修改 Upload/主流程 |
| ADR-011 | 存储抽象 | **已移除（2026-08）**：storages 模型整体删除，改为**多媒体源** `media_sources`（仅 `(id, path, created_at, last_scan_at)`，纯持久化标记 + 扫描单位），不参与文件浏览生命周期 | 视频库入库与文件浏览彻底解耦；多媒体源是「用户承诺这里主要放多媒体」的目录，可如 pin 一样轻易增删，取消标记不删库 |
| ADR-012 | 目录扫描 | 扫描、缩略图、元数据、哈希、导入统一收敛到 `scanner` 模块；扫描单位是**多媒体源**而非存储卷；fsnotify 监视为下阶段能力 | Explorer/Library 均不负责扫描，职责单一；未来 AI 也以事件接入 scanner |
| ADR-013 | 前端形态 | 文件浏览器（files）与视频库（library）是共享 API / 播放器 / 元数据的两个应用面，以顶部页签切换 | 以后可做 TV Mode（只保留 Library）；共享层避免重复实现 |
| ADR-014 | 热插拔与可用性 | **已移除（2026-08）**：无卷序列号/盘符重映射/可用性探测。多媒体源根不可达 → 扫描**中止且不动库**（移动硬盘没插时保护元数据）；根存在 → 正常增删改 | 多媒体源是普通路径，不维护虚拟盘生命周期；「不可达即暂停扫描」沿用旧保护语义 |
| ADR-015 | 系列组织 | **2026-08 管理面定稿 + 2026-09 显示名/手动排序修订**：系列是**用户显式创建**的管理容器，绑定根目录（`seasons.root_path`），成员 = 根目录**直接一级子文件**默认按文件名序 1..N（无季号结构），**可手动拖拽重排**（`POST /api/series/{id}/order`，置 `seasons.sort_manual=1`，此后扫描/同步只追加新成员、不重排），**可恢复自动模式**（`POST /api/series/{id}/resort`，清 `sort_manual` 按文件名序重绑，纯 DB 操作）；**扫描只维护既有系列成员、绝不自动建系列**；单集是唯一基本实体、必须归属媒体源；**无电影/tv 结构类型**（区分走标签）；只增不拆、识别错了由用户手动归组。**系列显示名（2026-09）**：系列名 = `shows.name`，**创建时默认取文件夹名**，创建后是**独立于文件夹的显示名**——可经 `PATCH /api/shows/{id}` 手动编辑，扫描/标记/同步**永不覆盖**（与单集 `title_source` 同语义）；文件夹对应关系只由 `root_path` 承载。**成员标题受 `title_source` 保护（2026-09 修订）**：手动编辑过的成员标题（批量改名/单集详情编辑）扫描/同步**不再还原为文件名**，`bindSeriesMembers` 依 `EpisodeAssign.TitleSource` 保留 | 剧集/番剧是常见形态；严格「路径 + FileID」对应避免误组；系列是纯 DB 概念不随磁盘自动漂移，避免自动规则误判后难以纠错；显示名/成员顺序允许用户修正识别结果而不动磁盘 |
| ADR-016 | ~~元数据刮削~~（已移除） | ~~离线优先、手动触发：本地 NFO → 手动编辑 → 可选在线刮削（TMDB，需 API Key）~~ | 2026-08 移除：刮削子系统（TMDB 在线 + NFO 侧边文件）整体删除，仅保留手动编辑 |
| ADR-017 | 统一资源进口/出口管线 | 所有资源「入库/入维护范围」收敛到**同一套后端函数**：scanner 暴露 `IngestPaths`（进口：probe + file_id 指纹 + 建行/重定位 + 系列收敛 + 事件）与 `EvictPaths`（出口：删行 + 系列收敛 + `VideoDeleted`），内部共用单一 `normalizeCandidate`。扫描/标记/同步重构为复用同一函数；文件浏览器经 `fservice.SetLibraryNotifier` 注入回调挂钩——copy/convert 产物 → Ingest，move/rename → **Ingest(新路径) 先、Evict(旧路径) 后**（保住 file_id 身份与历史），delete → Evict。任何经 HomeReel 的文件变更在**操作完成瞬间**即归一化，不再等下次扫描；未来 upload / fsnotify 复用同一入口。**多媒体源禁止嵌套**（`AddSource` 校验拒绝，路由表仅防御历史遗留）；手动 title 经 `title_source` 保护（`manual` 永不被扫描覆盖）。顺带修复：`VideoUpdated` 只清字幕缓存（封面/缩略图由 probe 重建，纯改名/移动不动缓存） | 消除「文件操作是哑操作、数据滞后到下次扫描」；三份并存实现（scan/mark/sync）收敛为一；文件身份（file_id）全局匹配使移动不丢历史 |
| ADR-018 | 系列关联模型 | **2026-09 方案 B：显式分组**。`link_groups` + `link_group_members` 两张表，关联 = 同一组内系列**互相可见**（A 关联 B、C 时三者同组，打开任一方都看到另外两方）；无名称、无方向。维护为**全量替换**：`PUT /api/series/{id}/links` 提交勾选集，该系列与勾选系列同组、取消勾选即不再关联（前端「管理关联」多选弹窗 + `SeriesPickerModal`）；每系列至多一组（`series_id` 唯一索引），`SyncShowLinks` 把同 show 所有季自动并入一组。替代旧 `series_links` 无向边模型（读时直接邻居、不相邻不可见） | 解决「A 关联 B、C 后打开 B 也看到 A、C」的互相可见需求；显式分组比读时遍历/传递闭包实现直观、可增量维护；开发期直接重建无需迁移旧数据 |
| ADR-019 | 开发者工具日志 | **2026-09**：工具页新增「开发者工具」工具——在移动端等无开发者工具的终端上记录/查看/归档前端日志。**采集**：App 启动即按持久化开关安装 `console.*` 劫持（覆盖第三方库输出）+ 提供带模块标记的 `devLog()` logger，环形缓冲上限 2000 条，仅开关开启时采集，采集状态与缓冲为**前端内存态**（localStorage 持久化开关）。**归档**：把当前采集日志 `POST /api/devlogs` 存 SQLite `devlogs` 表（entries 为 JSON 数组，逐字存储以便取回还原），PC 端开发者工具选择归档查看/复制，另提供 `GET /api/devlogs/{id}/raw` 纯文本端点供开发时按 ID 非 GUI 抓取日志 | 移动端打不开开发者工具，需把前端日志带回 PC 排错；SQLite 存储与项目数据层一致、可查询/可删除；采集放内存避免长期运行拖慢页面 |

## 5. 工程目录约定

```
backend/
  cmd/server            # 装配 + 启动
  internal/
    api  auth  config  db  domain  events
    files  fservice  jobs  media  netutil
    scanner  search  store  streaming
  data/                 # 运行期数据（data_dir）：SQLite + covers/thumbs/remux/hls/subtitles/uploads
  config.yaml           # 服务配置（示例见 docs/decisions.md）
frontend/
  src/
    api  components  features  lib  tabs  index.css  main.tsx
    # features: auth files home jobs library player search series tools
    # tabs: 多 Router 页签宿主（config.ts / routers.ts / manager.ts / TabBar / TabHost / TabSync）
```

**依赖方向单向**：`api → domain → store / files / media`，禁止循环依赖；模块职责单一、目录扁平、
显式优先、少用复杂设计模式。

## 6. 工作规范

- **先理解，再开发**
  - 制定方案、开发代码时，必须先阅读相关文件上下文，理解既有代码风格、命名与模式
  - 用户提供的要求、信息如果存在模糊或不合理之处时，需要询问用户，双方没有理解Gap后再执行
- **保持注释简洁**
  - 优先使用代码自注释（基于变量名、函数名等）
  - 仅在“没有该注释会影响理解”的情况下，使用注释，且着重描述“为什么”而非“是什么”
  - 代码文件中的所有注释使用ascii字符，禁止使用中文和emoji
- **即时更新文档**
  - 新增的核心知识、全局要求性内容等必须更新进本项目的文档
  - 必读的核心内容写入本文档；只影响单个领域的写入 `docs/` 下的对应二级文档
- **委托人工操作**
  - 当某个操作无法通过命令行执行，或使用命令行的难度、效果远不如人工执行时（例如验证浏览器UI显示效果、播放体验、使用局域网设备访问等），不强行用脚本模拟，应明确告知用户需手动执行的内容以及执行方法，由用户执行
- 不主动 `git commit` / `push`，除非用户明确要求
- 不擅自创建文档文件（`.md`），除非用户明确要求
- 引用代码时使用 `文件路径:行号` 格式

## 7. 必须遵守的强制规则（红线）

### 7.1 影响环境的操作必须先经用户批准

涉及以下类别的操作，**必须先向用户说明「做什么 / 为什么 / 影响面」，得到确认后再执行**：

- 安装、卸载、升级任何依赖包或系统组件（如 `pnpm add` / `pnpm install`、`go get`、`go mod tidy`、`pip install` 等）
- 修改全局环境变量、系统 PATH、注册表、防火墙、服务注册（NSSM）等环境级配置
- 操作当前项目目录之外的文件
- 启动/停止系统服务
- 部署、执行可能具有上述影响的程序
- `git` 操作如提交、推送、打 tag、创建 PR 等

**例外**：纯只读操作（例如读文件、grep、查看 git 状态等）无需批准

### 7.2 严禁修补式代码

例如原逻辑只针对 A 情况，若为新增的小概率 B 情况，需要沿原逻辑链堆叠大量 `if` 特判分支，即属于修补式代码。

- 当需求需要扩展原有行为时，应整体考虑是否存在 **统一的、抽象的**方法，例如策略类 / 接口类 / 配置驱动 / 数据驱动
- 如果实在无法解决，则应当考虑是否应该重构整体实现，依然**严禁**在调用链上打补丁

### 7.3 关键决策禁止主观臆测

关键需求模糊时，必须提供选择并询问用户。

- 当用户需求存在歧义、有多种可行方案、或选择会显著影响架构 / 体验 / 未来演进时：
  - 给出 2~3 个候选方案，说明各自利弊
  - 标出推荐项
  - **经用户确认后再执行**，禁止自行臆断决定

例外：
- ADR / decisions.md 已有明确结论的事项不必再问
- 选择不会造成可察觉的影响时，不必询问（例如两个依赖包用途相同，只是接口不同，这种情况由你自行决定）

### 7.4 禁止执行永久阻塞命令

永久阻塞命令包括但不限于：
+ 使用命令行启动一个后台 server，而该 server 除非手动 Ctrl-C 不会自动关闭
+ 某项在合理时间内无法执行完毕的命令，例如某项计算需要10分钟，直接对某目录下的数百个文件批量执行该计算

### 7.5 主动清理执行垃圾

执行垃圾包括但不限于：
+ 执行某项任务过程中，产生的中间脚本、中间输出文件
+ 残留的进程，例如某些工具会产生后台进程，在工具退出时这些进程又没有退出

## 8. 交付、质量与验证

### 8.1 验收标准

- 开发阶段与验收标准见 [docs/decisions.md](docs/decisions.md) §5（Phase 0~4），每个 Phase 的「验收」即完成判定，不得跳过测试

### 8.2 测试策略

- 整体策略见 [docs/decisions.md](docs/decisions.md) §5.1：
  - 后端：`go test ./...`（`httptest` 全链路、mock 文件系统可注入、媒体相关打 `//go:build integration`）
  - 前端：**尚未配置测试**（`package.json` 无 test script，见 [docs/status.md](docs/status.md)）；预期 Vitest + Testing Library

### 8.3 改动完成后的自检

- 运行项目提供的 lint / typecheck / test：
  - 后端：`go test ./...`
  - 前端：`pnpm run lint`（**前端命令一律用 `pnpm`**；其余未配置命令向用户确认）

### 8.4 阶段收尾

- 阶段完成后建议打 tag 并写阶段小结（走查 ADR 与目录结构）；修订 ADR 必须留痕并回填关联章节

### 8.5 Git 交付方式

当用户提出“提交本次会话的修改”时，使用Git打包提交到远程仓库。

- git add .
- git commit -m "commit messgae"
- git push

Commit Message应当是一个简单的英文短语，用词准确、凝练地描述当前会话做了哪些修改。请你提供2~3个message选项，然后向用户询问使用哪一个更加恰当。

## 9. 沟通约定

- 回复简洁、直接、可执行
- 避免冗长的解释性前缀
- 与项目文档一致，使用中文交流
