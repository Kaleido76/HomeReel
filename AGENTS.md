# AGENTS.md — HomeReel 智能体工作指南

> 本文档是任何智能体（Agent / AI 编程助手）参与本项目时的**首个引入式指导文档**。
> 参与本项目前，请先完整阅读本文档。

---

## 1. 项目是什么

HomeReel 是一个**个人视频资料管理平台**（DAM，Digital Asset Management），部署在家里的一台
PC 上，通过局域网 Web 界面供所有设备（PC / 手机 / 平板 / TV）访问。

- 核心能力：**视频管理**——目标是达到「简易 JellyFin」体验（海报墙、元数据、剧集/季/集分组、
  继续观看、首页行式浏览），但**不做** JellyFin 的冗余加法功能（插件系统、多用户档案、直播、
  音乐库等）。
- 辅助能力：基础文件管理（浏览 / 上传 / 下载 / 重命名 / 移动 / 删除）。
- 定位：单用户、单访问口令；**支持多终端并发访问**（卧室/书房 PC、同机多标签页可同时在线，
  各自独立会话，互不挤占）；Windows 优先部署。

## 2. 文档优先级（开始工作前先读这些）

| 文档 | 作用 | 备注 |
|---|---|---|
| `DEVELOPMENT_PLAN.md` | **唯一权威开发方案**，所有开发活动以此为准；关键决策记录（ADR）在 0.4 节 | 如有修订必须留痕 |
| `Personal_Media_DAM_Architecture.md` | 上游原始构想 | 可能滞后于 plan，仅作背景 |

**规则**：修改代码或架构前必须先读 `DEVELOPMENT_PLAN.md`。发现文档与代码不一致时，以
`DEVELOPMENT_PLAN.md` 为准，并向用户指出差异。

## 3. 核心原则（来自 plan 0.2）

1. 视频体验优先
2. 保留任意文件管理能力
3. AI 可长期维护（模块化、低耦合、显式优先）
4. UI 现代化
5. Windows 优先部署
6. 单用户：唯一所有者，通过一个访问口令保护；**支持多终端并发访问**（每终端独立会话，
   可同时看视频 + 整理文件，互不挤占）
7. **最高级原则**：一切功能围绕「元数据」扩展，而非「文件系统」——文件系统只保存字节，
   元数据负责查找、理解与组织
8. Library 提供简易 JellyFin 媒体库体验，但不引入冗余功能
9. **功能正确优先，禁止过度优化**（用户新增原则）：先保证功能正确，再谈性能与并发。两个操作
   彼此无依赖时可异步并行；但凡存在**条件竞争风险**时，一律先用同步串行手段保证正确性，
   待有真实瓶颈证据后再优化

## 4. 关键架构决策（ADR 摘要，详见 plan 0.4）

- **单 Go 服务**：v1.0 全程不拆服务；内部按包拆分，预留拆分边界（ADR-001）
- **认证（多终端并发会话）**：单口令 + 会话 Cookie；每终端独立登录/独立会话，登出只清自身，
  互不挤占（ADR-002）
- **数据库**：SQLite（纯 Go 驱动 + WAL），演进路线 WAL → FTS5 → 队列 → 缓存，**不默认迁移
  PostgreSQL**（ADR-005）
- **播放策略**：能力探测三层——可直连 → HTTP Range；不可直连 → HLS 转码；仍不可 → 转码兜底；
  HLS 默认 `auto`（ADR-006）
- **文件身份**：`(storage_id, file_id, relative_path)` 三元组 + `(file_id, size, mtime)` 指纹
  （ADR-007）
- **多数据源**：`storages` 抽象（type: internal/external/network），外接卷热插拔、盘符重映射、
  离线不删元数据（ADR-011 / ADR-014）
- **系列组织 + 刮削**：视频库统一为「单集 + 系列」（一季/一部一个系列，`seasons.kind`=tv/movie，
  成员按位次排序允许缺失）；系列间弱关联 `series_links`；目录/文件名规则识别；元数据离线优先
  （NFO → 手动 → 可选 TMDB）（ADR-015 / ADR-016）
- **AI 解耦**：事件总线（`VideoImported` 等），AI/OCR/转写作为 Listener（ADR-010）
- **搜索隔离**：`SearchProvider` 接口，当前 FTS5，Meilisearch 后续替换（ADR-009）
- **前端形态**：Explorer 与 Library 是两个共享 API/播放器/元数据的 App（ADR-013）

## 5. 技术栈（详见 plan 1 章）

- 前端：**React 19** + TypeScript、Vite、Tailwind CSS 4、TanStack Router / Query、
  Vidstack（播放器，HLS 用本地 hls.js）；Vitest + Testing Library（尚未配置 test script）；
  **前端包管理器：一律使用 `pnpm`**（本机已装 pnpm 11.x）；**禁止**使用 `npm` / `cnpm` / `yarn`
  执行安装、脚本等任何前端命令
- 后端：Go 1.22+、标准库 `net/http`、`modernc.org/sqlite`（免 CGO）、`fsnotify`、`slog`、ULID
- 媒体：FFmpeg / ffprobe（探测、缩略图、HLS、字幕）
- 部署：`CGO_ENABLED=0` 单 `.exe` + `data_dir` + `config.yaml` + ffmpeg 二进制

### 5.1 开发环境要求（当前本机状态）

| 工具 | 用途 | 本机状态 |
|---|---|---|
| Go 1.22+ | 后端编译（当前 1.26.5，`CGO_ENABLED=0`） | ✅ 已装（`C:\Program Files\Go`） |
| Node.js 18+ | 前端运行（当前 v24.18.0） | ✅ 已装 |
| pnpm | 前端包管理（当前 11.9.0，registry 为 npmmirror） | ✅ 已装 |
| Git | 版本管理（当前 2.55.0） | ✅ 已装 |
| winget | 软件安装（备选 choco/scoop 未装） | ✅ 已装 |
| **FFmpeg / ffprobe** | 媒体探测、缩略图、HLS 转码、字幕；integration 测试依赖 | ✅ 已装（8.1.2，`winget install Gyan.FFmpeg`，已入 PATH） |

- FFmpeg 安装方式（选一）：`winget install Gyan.FFmpeg --source winget`（装后写入 PATH，
  需新开终端生效）；或将 `ffmpeg.exe` / `ffprobe.exe` 放入项目 `bin/` 并在 `config.yaml` 的
  `media.ffmpeg_path` / `media.ffprobe_path` 指定。
- 当前安装位置：`C:\Users\tolov\AppData\Local\Microsoft\WinGet\Packages\Gyan.FFmpeg_*`
  （`bin\ffmpeg.exe` / `bin\ffprobe.exe`）。
- **运行时注意**：winget 装的 FFmpeg 需新开终端 PATH 才生效；若服务进程 PATH 里找不到
  `ffprobe`（probe 任务报 `executable file not found in %PATH%`），在 `config.yaml` 的
  `media.ffmpeg_path` / `media.ffprobe_path` 显式指定绝对路径（YAML 双引号内用 `\\` 转义），
  改后需重启服务。
- 本机 `go env`：`GOOS=windows GOARCH=amd64 CGO_ENABLED=0`，符合「免 CGO 单 exe」要求。

## 6. 工程目录约定（详见 plan 3 章）

```
backend/
  cmd/server            # 装配 + 启动
  internal/
    config  auth  api  db  domain  store
    files  storage  events  scanner  scrape
    jobs  media  streaming  search
  testdata/
frontend/
  src/
    api  components  features  lib  styles  tabs
    # features: auth home explorer library player series search
    # tabs: 多 Router 页签宿主（config/routers/manager/TabBar/TabHost/TabSync）
```

**依赖方向单向**：`api → domain → store / files / media`，禁止循环依赖；模块职责单一、目录扁平、
显式优先、少用复杂设计模式。

## 7. 工作规范

- 动手前先理解：读相关文件上下文，遵循既有代码风格、命名与模式；新用库前先确认项目是否已引入。
- **不加多余注释**，除非能解释「为什么」而非「是什么」。
- 完成后运行项目提供的 lint / typecheck / test（后端：`go test ./...`；前端：`pnpm run lint`、
  `pnpm run test` 等；**前端命令一律用 `pnpm`**，未配置则向用户确认命令）。
- **知识 / 全局要求性内容必须同步更新到 AGENTS.md**：当会话中发现任何适用于项目全局的约定
  （工具链、命令、命名、工作流程等）时，立即将其写入本文档，以便后续会话继承。
- **需要人工验证的项交给用户**：当某个验证无法通过命令行反馈、或命令行验证的难度/效果远不如
  人工验证时（例如浏览器端视觉/交互、播放体验、局域网设备访问等），不强行用脚本模拟，应明确
  告知用户「该项需你手动验证 + 验证方法」，由用户执行。
- **禁止为验证长期占用命令行**：不得自行启动后台 webserver / 服务再用命令轮询等待；不得执行
  不会立即返回的命令去「跑通」交互流程。命令行链路验证以 httptest / 单次可返回的 curl 为准，
  浏览器与交互类验证一律直接列入「需手动验证清单」交给用户。
- 不主动 `git commit` / `push`，除非用户明确要求。
- 不擅自创建文档文件（`.md`、README），除非用户明确要求。
- 引用代码时使用 `文件路径:行号` 格式。

## 8. 强制规则

以下三条是用户明确要求、必须遵守的硬性规则：

### 规则 A —— 影响环境的操作必须先经用户批准

涉及以下类别的操作，**必须先向用户说明「做什么 / 为什么 / 影响面」，得到确认后再执行**：

- 安装、卸载、升级依赖包（如 `pnpm add` / `pnpm install`、`go get`、`go mod tidy`、`pip install` 等）
- 修改全局环境变量、系统 PATH、注册表、防火墙、服务注册（NSSM）等环境级配置
- 在项目目录之外写入文件、修改用户主目录内容
- 启动/停止系统服务、部署、执行可能耗时或占用大量资源的操作
- `git` 提交、推送、打 tag、创建 PR

**例外**：纯只读操作（读文件、搜索、grep、查看状态）无需批准。

### 规则 B —— 代码核心标准：可维护、可读、架构清晰，严禁修补式代码

- **禁止修补式代码**：例如原逻辑只针对 A 情况，为新增的小概率 B 情况沿原逻辑链堆叠大量
  `if` 特判分支。这是本项目的红线。
- 当需求需要扩展原有行为时，应把变化**归入统一的抽象**（策略 / 接口 / 配置驱动 / 数据驱动），
  而不是在调用链上打补丁。
- 判断标准：若一段逻辑因新增场景出现多份特判、重复分支，应停下来**重构**而非继续堆代码。
- 保持模块职责单一，遵守 plan 中的 ADR 与架构边界，不越界耦合。

### 规则 C —— 需求模糊时，必须提供选择并询问用户

- 当需求存在歧义、有多种可行方案、或选择会显著影响架构 / 体验 / 未来演进时：
  - 给出 2~3 个候选方案，说明各自利弊
  - 标出推荐项
  - **询问用户后再动手**，禁止自行臆断决定
- 例外：ADR / plan 已有明确结论的事项不必再问。

## 9. 质量与验证

- 开发阶段与验收标准见 plan 10 章（Phase 0~4），每个 Phase 的「验收」即完成判定，不得跳过测试。
- 测试策略见 plan 12 章：后端 `go test ./...`（`httptest` 全链路、mock 文件系统可注入、
  媒体相关打 `//go:build integration`）；前端 Vitest + Testing Library。
- 阶段完成后建议打 tag 并写阶段小结（走查 ADR 与目录结构），修订 ADR 必须留痕并回填关联章节。

## 10. 沟通约定

- 回复简洁、直接、可执行；避免冗长的解释性前缀。
- 与项目文档一致，使用中文交流。

## 11. 当前状态与后续方向

> 本节是会话间交接快照：已实现能力清单（现状）、后续开发必须遵守的约定、遗留与验证事项。

### 11.1 已实现能力（现状）

- **认证与会话**（ADR-002）：单口令 + 会话 Cookie；多终端独立会话，登出只清自身。
- **存储卷与文件管理**：`storages` CRUD + 可用性探测；Explorer 浏览、分块可续传上传、Range 下载、新建/重命名/移动/删除（只读卷 403）。
- **扫描与索引**：指纹 `(file_id,size,mtime)` 增量扫描、移动识别、删除标记、fsnotify 5s 去抖监视、NTFS FileID；`jobs` 队列 + Worker（probe/thumbnail/rescan，崩溃恢复）；缩略图 covers 320px + thumbs 160px。
- **播放**：能力探测三层（直连 Range / HLS 按需转码 / 兜底）；live 型增量 HLS 转码（单飞、内存快照、`.done` 标记、no-store）；封面；侧边 `.srt/.vtt/.ass` 字幕；Vidstack 播放器 + 本地 hls.js。
- **历史续播**：`history` 表（user=`local`）；前端 10s 节流保存、进入 seek（距片尾 20s 内不续播、播完归零）。
- **媒体库（单集/系列）**：统一「单集（`show_id IS NULL`）+ 系列（`seasons` 行，一季/一部一个系列，`kind`=tv/movie）」；成员位次排序允许缺失；`series_links` 弱关联（同 show 相邻季自动关联 + 手动增删）；首页行（继续观看/最近添加）。集合子系统已整体移除（2026-08）。
- **搜索**：FTS5（`videos_fts` external content + 触发器，bm25 排序），`search_text` 由 store 层维护。
- **刮削**（ADR-016）：NFO（Kodi 新旧 rating）/ 手动 PATCH / 可选 TMDB；封面落盘 `covers|posters|backdrops`。
- **前端（多 Router 页签架构）**：顶部 4 个页签（首页/视频库/搜索/文件），每页签一个独立 TanStack Router 实例（`tabs/`），组件树常驻不卸载 = 类浏览器 keep-alive（滚动/筛选/播放器/上传状态切页签不丢）；URL 与活动页签双向同步（`TabManager` + `TabSync`）；页面组件 `React.lazy` 分包懒加载；ARIA tablist 语义 + 方向键切换；深链/刷新按 URL 恢复对应页签视图。库的视图状态（`view/q/sort/page`）与搜索词 `q` 存在 URL 参数中。视频卡片任意页签点击 → 切到视频库页签打开播放器。**播放器切走自动暂停**（`VideoPlayer` 订阅页签 store，非 library 页签即 `remote.pause()`，切回保持暂停由用户手动播放）。

### 11.2 关键约定（后续开发必须遵守）

- **时间戳**：统一固定宽度纳秒 RFC3339（`2006-01-02T15:04:05.000000000Z07:00`），保证字典序=时间序（recency 比较依赖）。`store.util.nowRFC3339` / `scanner.timeLayout` / `api.timeLayout`。
- **SQLite 单写者**：`SetMaxOpenConns(1)`，写操作收敛到 store 层；**禁止在 rows 迭代期间发起新查询（死锁）**——FTS 先收集 id 关闭 rows 再取详情，show 改名重建 search_text 同理。
- **迁移机制**：`db.go` migrations 数组尾部追加；一条迁移可含多语句，`splitStatements` 感知字符串字面量与 `BEGIN…END` 块（触发器内分号不会误切分）。
- **剧集/系列组织**：库 = 单集（`show_id IS NULL`）+ 系列（`seasons` 行，一季/一部一个，`kind`=tv/movie）。分组唯一来源 `scanner.ParseEpisode`（`SxxEyy`/`第x集`/`Season N` 目录/中文数字）与 `scanner.ParseMoviePart`（`Part N`/`第N部`/数字后缀）；**默认单集**，仅当位于 `Season N` 目录、或同目录 ≥2 个标题键相同/编辑距离 ≤2（`scanner.editDistance`）才归系列；`groupVideo` 在 **Scan 结束时统一对 `toGroup` 执行**（同目录兄弟可见），也用于 ImportUploaded/probe/手动归组；老库数据在 unchanged 分支回填。`series_links` 无名称、`sort_index` 排序，同 show 相邻季 `SyncShowLinks` 自动关联，`/api/series/{id}/links` 手动增删。`videos_bd` 触发器删光某 show 最后一集时删空 show；删空 season 由扫描/后续兜底。
- **FTS5（search_text）**：`videos_fts` external content + `ai/ad/au` 触发器；写入侧统一 `rebuildSearchText`（Create/UpdateProbe/UpdateMetadata/AssignEpisode/AssignMovie/SetTags/show 改名）；新增影响搜索内容的写路径都要重建。
- **刮削**：`scrape.ParseNFO`（Kodi 新旧 rating）；`VideoImported` listener 自动应用侧边 `.nfo`/`tvshow.nfo`；TMDB 需 `scrape.tmdb_api_key`（空则 400 `no_provider`），改后重启；手动封面 `POST /api/videos/{id}/cover` 落盘 `covers/<id>.<ext>`；`UpdateCovers` 空串=该列不动。
- **扫描安全**：卷根不可达中止扫描、绝不误删元数据（ADR-014）；无 probe 元数据（`duration==0 && codec==""`）强制重 probe（自愈）。
- **上传清理**：合并完成即删 staging；启动时 + 每小时清理超 24h 孤儿分片。
- **ffprobe/ffmpeg**：进程 PATH 找不到时在 `config.yaml` 显式配置 `media.ffmpeg_path`/`media.ffprobe_path`（YAML `\\` 转义），改后重启。
- **容器判定**：ffprobe `format_name` 是逗号分隔列表（如 `mov,mp4,m4a,...`）。`media.Probe` 归一化首个 token 入库；`DirectPlayable`/`contentType` 按逗号分词匹配，**禁止整串查映射**（否则 mp4/h264 被误判为不可直连而误走 HLS）。
- **HLS 转码**：单飞（in-flight 去重）+ 后台；命令见 `streaming/transcode.go`（`-hls_list_size 0`、`-hls_flags temp_file+independent_segments` 原子写、`-map 0:a:0?`）；`master.m3u8` **读入内存快照**服务（ffmpeg 原地重写，直接 ServeContent 会读到截断），仅含 `#EXTINF` 才对外服务、`Cache-Control: no-store`（避免 304 卡死 hls.js）；`.done` 标记区分完成缓存/崩溃残留，残留整目录重建；转码结束从 `s.active` 移除；缓存随 `VideoDeleted`/`VideoUpdated` 失效。
- **能力探测唯一来源**：直连/HLS 判定由后端 `streaming.DirectPlayable`/`HLSEnabled` 计算，`GET /api/videos/{id}` 返回 `direct_playable`/`hls_enabled`，前端不重复实现。
- **Vidstack**：用 `@vidstack/react`（v1.15+，**勿装废弃的 `@vidstack/player`**）；HLS 经 `useMediaProvider` 设 `provider.library = () => import('hls.js')` 指向本地包（默认 jsdelivr CDN，LAN 离线失败），并放宽 `provider.config.manifestLoadingTimeOut`（默认 10s 首播转码期超时→无限转圈）；样式 import `@vidstack/react/player/styles/{base.css,default/theme.css,default/layouts/video.css}`，`DefaultVideoLayout` 从 `@vidstack/react/player/layouts/default` 导入；**`/api/stream/{id}` 无扩展名，`MediaPlayer` 的 `src` 必须显式带 `type`**（`VideoSrc | HLSSrc`），否则回退 HEAD 探测失败即报 `could not find a loader`。
- **多 Router 页签（keep-alive）**：每页签一个独立 Router（`createMemoryHistory`），组件树常驻（`display:none` 隐藏）＝状态永不丢。**URL 唯一来源是活动页签**：`TabManager.activate` 切换页签用 `replaceState`（不产生历史），活动页签内导航由 `TabSync` 镜像 `pushState`；`popstate` 反解析 URL→页签并 `navigate({href, replace})` 对齐该页签 memory history。跨页签跳转走 `openVideo/openSeries/openLibrary`（切到 library 页签再 navigate）。**新增子路由必须归属到某个页签的 router**（`tabs/routers.ts`），并在 `tabFromPath` 补路径映射，禁止添加全局平级路由。
- **新增依赖需用户批准**（规则 A），含 `go get` / `pnpm add`。

### 11.3 待办 / 遗留

- **人工验证清单（需用户执行，浏览器体验，尚未验证）**：① 直连播放（mp4/h264 秒开、拖动流畅、续播、播完从头）；② 真 MKV/HEVC 触发 HLS 首播（60s 内首屏）并播放；③ 手机/平板局域网访问播放；④ 同名 `.srt/.vtt` 字幕出现在字幕菜单；⑤ 重启后老库自动迁移 + 触发一次「存储卷刷新/重扫」让已有视频自动归组；⑥ 单集/系列两视图、系列详情成员按位次展示（含缺失占位）；⑦ 同标题季/部自动关联、可跳转与手动增删；⑧ 首页「继续观看」随历史变化；⑨ 标签增删后可被搜索命中；⑩ 未配置 TMDB Key 时在线刮削给友好提示；⑪ 上传封面后播放页/海报墙更新。
- **页签保活验证（2026-08 新架构）**：① 各页签切换后返回保持原视图（筛选/排序/分页/滚动位置/搜索词）——已由用户验证通过；② 播放中切走页签**自动暂停**，切回保持暂停（手动播放）；③ 文件管理页进行中的上传切走再回来不中断；④ 浏览器前进/后退在页签内有效、跨页签返回会切回对应页签；⑤ 刷新/深链直接恢复对应页签视图（如 `/library/video/x`）——已由用户验证通过；⑥ 点视频卡片从首页/搜索跳转到视频库页签并打开播放器——已由用户验证通过；⑦ 顶部页签 Active 高亮清晰、键盘方向键可切换页签、无 JS 错误（控制台）——已由用户验证通过。
- 单集/系列**手动归组与编辑 UI** 未做（`PATCH /api/videos/:id` 支持 show_id/season_number/episode_number，API 已就绪）。
- 前端无 Vitest 测试（`package.json` 无 test script）；TMDB 需配置 `scrape.tmdb_api_key`（改后重启）。
- 设备热插拔**盘符重映射未实现**：监视仅在启动时对 enabled 卷建立，拔出/插入不会自动重扫/重映射（ADR-014 后半部分）。
- `delete_mode: trash` 未实现（当前永久删除）；前端断点续传 UI 未做。
- HLS 仅单一路码率（`-hls_time 10`，无自适应多码率）；嵌入字幕轨道抽取未做（仅侧边字幕文件）。
- `docs/decisions.md` 尚未创建（plan 附录 A）。

### 11.4 未来方向

见 `DEVELOPMENT_PLAN.md` §14 演进路线（PostgreSQL 迁移、Meilisearch、AI 模块、多用户、TV Mode、更多刮削源等均仅预留，不在近期实施）。
