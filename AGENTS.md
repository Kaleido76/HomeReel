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
| `docs/backend.md` | 后端与数据层实现事实源（时间戳/SQLite/迁移/FTS5/系列归组/多媒体源/jobs/事件总线/后端日志） | 改动 `backend/internal/{store,scanner,fservice,files,jobs,events,db,config,realtime,logging}` |
| `docs/media.md` | 媒体管线实现事实源（ffprobe/容器判定/分段 MP4/格式工厂/能力判定/字幕/封面） | 改动 `backend/internal/{media,streaming}` 或前端播放器媒体相关 |
| `docs/frontend.md` | 前端实现事实源（页签 keep-alive/栏位栈/Vidstack/响应式/文件浏览器/卡片） | 改动 `frontend/src/{tabs,features}` |
| `docs/realtime.md` | 实时双向通道实现事实源（WS 协议/Hub/RealtimeClient） | 改动 `backend/internal/realtime` 或 `src/api/realtime.ts` |
| `docs/status.md` | 现状快照/遗留待办/人工验证清单/未来方向 | 规划新功能、验收、会话交接 |
| `UI.md` | UI 设计语言（设计原则总纲；具体规格以 frontend.md 为准） | 改动前端样式/布局前 |
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
> 每条 ADR 只写「结论 + 理由」；实现细节一律下沉领域文档（下表括号内为细节所在处）。

| # | 决策 | 结论 | 理由 |
|---|---|---|---|
| ADR-001 | 后端形态 | 单个 Go 服务，内部按包拆分（api/auth/config/db/domain/store/files/fservice/events/scanner/jobs/media/netutil/realtime/streaming/search）；**v1.0 全程不拆服务** | 家庭部署单进程 + 单 sqlite 足够；拆服务带来成倍的日志/配置/部署成本，AI 等真正变重再拆 |
| ADR-002 | 认证模型 | 单口令 + 会话 Cookie；多终端并发会话各自独立、登出/过期互不影响；无用户概念，数据 `user` 固定 `local` | 个人使用；多终端同时在线互不挤占；`user` 字段为未来多用户留位 |
| ADR-003 | 部署形态 | 纯 Web 应用，监听 `0.0.0.0`，浏览器访问 | 局域网任意设备免安装即用 |
| ADR-004 | 文件入库 | 多媒体源标记驱动视频库入库；分块上传已随旧 Explorer 移除，upload 待后续补 | 入库只依赖「用户标记的目录」，与文件浏览完全解耦 |
| ADR-005 | 数据库 | SQLite（纯 Go 驱动 + WAL）；演进按需推进（WAL → FTS5 → 队列/缓存），真遇瓶颈才考虑 PostgreSQL | Windows 免 CGO、零依赖单文件；个人量级瓶颈是 ffmpeg 不是 DB |
| ADR-006 | 播放策略 | **三层动态流**：Direct（Range 直连）→ Remux（整片流拷贝成缓存 MP4）→ Transcode（按需 HLS）→ 格式工厂兜底；判定源 = 前端运行期 `canPlayType()` 核对 probe 元数据，后端能力标志仅兜底。附随能力：多音轨选轨、播放选择记忆（单集+系列双层）、移动端伪全屏（细节见 media.md §4/§6、frontend.md §3） | Remux 纯拷贝成本极低且结果全片可拖；Transcode 只转实际观看部分；判定运行期固化，不逐格式堆硬编码布尔。实测依据（Matroska 流拷贝 seek 不可靠、音频重编码 ~70× 实时）见 media.md §4 |
| ADR-007 | 文件身份 | `(source_id, file_id, relative_path)` 唯一；file_id **全局匹配**（跨源移动仍是同一视频）；`(size, mtime)` 为变更指纹；hash 未实现，仅留扩展空间 | 目录移动甚至跨源移动不丢身份与历史；大文件哈希代价高，首次索引要快 |
| ADR-008 | 任务队列 | SQLite 持久化 `jobs` 表 + 进程内 worker 池 | 进度可见、崩溃可恢复、实现简单 |
| ADR-009 | 搜索 | `SearchProvider` 接口；当前实现 SQLite FTS5（反规范化 `search_text`），后续可加 Meilisearch/AI 实现 | 控制器不直接写 SQL；替换引擎不动上层 |
| ADR-010 | AI 模块 | 事件解耦：AI/OCR/缩略图/转写作为 Listener 订阅 `VideoImported` 等域事件 | AI 接入不修改主流程 |
| ADR-011 | 存储模型 | storages 卷模型已删除，改为**多媒体源** `media_sources`（仅 id/path/时间戳）：持久化标记 + 扫描单位，不参与文件浏览生命周期 | 入库与文件浏览彻底解耦；多媒体源像 pin 一样轻易增删，取消标记不删库 |
| ADR-012 | 目录扫描 | 扫描/缩略图/导入统一收敛到 `scanner` 包；扫描单位是多媒体源而非存储卷；fsnotify 监视未实施 | Explorer/Library 不负责扫描，职责单一；fsnotify 将来复用 ADR-017 统一入口 |
| ADR-013 | 前端形态 | 文件浏览与视频库是共享 API/播放器/元数据的两个应用面，顶部页签切换 | 共享层避免重复实现；将来可裁出 TV Mode |
| ADR-014 | 可用性保护 | 无卷序列号/盘符映射等热插拔管理；源根不可达 → 扫描中止且绝不动库，根存在 → 正常增删改 | 移动硬盘没插时不误判「文件全部删除」；多媒体源只是普通路径 |
| ADR-015 | 系列组织 | 系列是用户显式创建的管理容器：绑定根目录、成员 = 直接一级子文件（文件名序）；显示名/成员顺序/成员标题均可手动修正且扫描永不覆盖；扫描只维护既有系列成员、绝不自动建系列；单集必须归属媒体源；无电影/tv 结构类型（区分走标签）。实现见 backend.md §4 | 严格「路径 + FileID」对应避免误组；系列是纯 DB 概念不随磁盘漂移；识别错了由用户手动修正而不动磁盘 |
| ADR-016 | （废止） | ~~元数据刮削~~：TMDB 在线 + NFO 子系统已于 2026-08 整体移除，仅保留手动编辑 | 墓碑记录，防止重新引入 |
| ADR-017 | 统一进出口管线 | 一切入库/出库收敛到 scanner 的 `IngestPaths` / `EvictPaths`（共用单一 `normalizeCandidate`）；文件浏览器操作经回调挂钩——copy/convert 产物 → Ingest，move/rename → 先 Ingest 新路径再 Evict 旧路径，delete → Evict；操作完成瞬间即归一化，不等下次扫描。多媒体源新建即禁止嵌套；手动 title 经 `title_source` 保护 | 消除「哑操作滞后到下次扫描」；扫描/标记/同步三份并存实现收敛为一；file_id 全局匹配使移动不丢历史 |
| ADR-018 | 系列关联 | 显式分组模型：`link_groups` + 成员表，同组系列互相可见、无方向；维护 = `PUT /api/series/{id}/links` 全量替换勾选集，每系列至多一组 | 「A 关联 B、C 后打开 B 也看到 A、C」的互相可见需求；全量替换比读时遍历直观、增量维护简单 |
| ADR-019 | 开发者工具日志 | 前端内存环形缓冲（2000 条上限，开关控制）劫持 `console.*` + `devLog()` 采集；归档经 `POST /api/devlogs` 存 SQLite `devlogs` 表，PC 端查看，另提供 `/raw` 纯文本端点 | 移动端打不开 devtools，需把日志带回 PC 排错；采集在内存态避免长期运行拖慢页面 |
| ADR-020 | ffmpeg 收口 | 所有 ffmpeg/ffprobe 命令构建统一收口 `media` 包，调用方只传结构化参数不拼命令行；音频白名单（`UniversalAudioCodecs`）与可流拷贝视频编码（`RemuxVideoCodecs`)单点定义；二进制路径启动时统一解析，缺失启动即报错 | 改参数/升级/换二进制只需改一处；命令分散三处的重复码表消除；缺失早暴露而非运行中途 |
| ADR-021 | 实时通道 | 单条 WebSocket（`GET /api/ws`，cookie 鉴权）承载 S→C 推送 + C→S RPC，信封 `{id?, type, data?}`；事件桥把全部域事件转发为 `events.*` 推送（发布方零改动）；轮询迁移模式 = 初始 REST 快照 + 推送失效。协议见 realtime.md | 轮询有延迟与无谓请求；复用 events 总线前端零额外依赖，业务编码只需声明「收到 X → 失效 Y」 |
| ADR-022 | 后端日志 | 统一 `logging` 包：`log.level/format/file` 单点配置，`Setup` 经 `slog.SetDefault` 全库生效（各包继续包级 `slog.*` 零改造）；HTTP 访问日志路径分级——业务 API 成功 Info、`/api/stream/*` 与静态资源成功 Debug、4xx→Warn、5xx→Error；可选文件输出启动时按日期轮转一次；关键业务操作以操作级粒度记 Info。实现见 backend.md §10 | 播放流量（Range/HLS 分片/封面）会淹没日志，分级后默认 info 只剩业务操作与错误（详略得当）；`slog.SetDefault` 覆盖全部调用点；家庭服务按天轮转一次足够，不做后台滚动机制（禁止过度优化） |

## 5. 工程目录约定

```
backend/
  cmd/server            # 装配 + 启动
  internal/
    api  auth  config  db  domain  events
    files  fservice  jobs  logging  media  netutil  realtime
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
