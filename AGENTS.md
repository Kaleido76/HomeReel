# AGENTS.md — HomeReel 智能体工作指南

> 本文档是任何智能体（Agent / AI 编程助手）参与本项目时的**首个引入式指导文档**。
> 参与本项目前，请先完整阅读本文档；领域细节按需阅读 `docs/` 二级文档（见 §10）。

---

## 1. 项目是什么

HomeReel 是部署在家里 PC 上的**个人视频资料管理平台**（DAM），经局域网 Web 界面供 PC / 手机 / 平板 / TV
访问：核心是「简易 JellyFin」视频管理（海报墙、元数据、剧集/季/集分组、继续观看、首页行式浏览），辅以
基础文件管理（浏览 / 上传 / 下载 / 重命名 / 移动 / 删除）。单用户、单访问口令；**支持多终端并发访问**
（各自独立会话，互不挤占）；Windows 优先部署。详见 [docs/architecture.md](docs/architecture.md)。

## 2. 文档体系（开始工作前先读这些）

| 文档 | 作用 | 备注 |
|---|---|---|
| `DEVELOPMENT_PLAN.md` | **唯一权威开发方案**，所有开发活动以此为准；关键决策记录（ADR）在 0.4 节 | 修订必须留痕 |
| `docs/README.md` | 二级文档导航（渐进披露入口） | 按领域读取 |
| `Personal_Media_DAM_Architecture.md` | 上游原始构想 | 可能滞后于 plan，仅作背景 |

**规则**：修改代码或架构前必须先读 `DEVELOPMENT_PLAN.md`。发现文档与代码不一致时，以
`DEVELOPMENT_PLAN.md` 为准，并向用户指出差异。

## 3. 核心原则（浓缩，全文见 [docs/architecture.md](docs/architecture.md)）

1. **功能正确优先，禁止过度优化**：两个操作无依赖时可并行；存在**条件竞争风险**时先用同步串行保证
   正确性，待有真实瓶颈证据再优化。
2. **一切功能围绕「元数据」扩展**（最高级原则）：文件系统只保存字节，元数据负责查找、理解与组织。
3. **单用户 + 多终端并发**：唯一所有者、单访问口令；每终端独立会话，可同时看视频 + 整理文件，互不挤占。
4. **AI 可长期维护**：模块化、低耦合、显式优先、少用复杂设计模式。
5. **Library = 简易 JellyFin**：提供媒体库体验但不引入冗余功能（无插件系统/多用户档案/直播/音乐库等）。
6. **识别类硬编码要克制（低产出投入比）**：分集/系列/标题等**名称自动识别**只做**少数稳健的规则**，
   **禁止**为覆盖各种命名变体持续堆叠硬编码（不断加正则、质量标签清单、模糊匹配、相似度算法等）。
   识别错了就错，交给用户**手动修正**（编辑标题 / 手动归组 / 手动排序）——自动识别的价值在于「大多数
   情况一次命中」，而不是「穷尽所有可能」。新增识别逻辑前先自问：这一条规则能覆盖多少真实场景？值不值得？

## 4. 技术栈（速览）

- 前端：**React 19** + TypeScript、Vite、Tailwind CSS 4、TanStack Router / Query、Vidstack（播放器，
  纯 Range 直连，无 HLS / hls.js）。**前端包管理器一律使用 `pnpm`**；**禁止**用 `npm` / `cnpm` / `yarn`。
- 后端：Go 1.22+、标准库 `net/http`、`modernc.org/sqlite`（免 CGO）、`fsnotify`、`slog`、ULID。
- 媒体：FFmpeg / ffprobe（探测、缩略图、字幕、格式工厂转换）。
- **播放策略（ADR-006，2026-08 修订）**：纯 HTTP Range 直连，**无 HLS 转码**。可播放性由前端运行期
  `canPlayType()` 核对（probe 元数据 → MIME/codecs，`lib/playability.ts`）；不可直连 → 播放按钮禁用、
  引导格式工厂转换。判定机制运行期固化，不逐格式堆硬编码布尔。
- 部署：`CGO_ENABLED=0` 单 `.exe` + `data_dir` + `config.yaml` + ffmpeg 二进制。
- 开发环境细节见 [docs/environment.md](docs/environment.md)；架构/部署详述见 [docs/architecture.md](docs/architecture.md)。

## 5. 工程目录约定

```
backend/
  cmd/server            # 装配 + 启动
  internal/
    config  auth  api  db  domain  store  netutil
    files  fservice  events  scanner  jobs
    media  streaming  search
  testdata/
frontend/
  src/
    api  components  features  lib  styles  tabs
    # features: auth home files library player series search tools
    # tabs: 多 Router 页签宿主（config/routers/manager/TabBar/TabHost/TabSync）
```

**依赖方向单向**：`api → domain → store / files / media`，禁止循环依赖；模块职责单一、目录扁平、
显式优先、少用复杂设计模式。

## 6. 工作规范

- 动手前先理解：读相关文件上下文，遵循既有代码风格、命名与模式；新用库前先确认项目是否已引入。
- **不加多余注释**，除非能解释「为什么」而非「是什么」。
- 完成后运行项目提供的 lint / typecheck / test（后端：`go test ./...`；前端：`pnpm run lint`、
  `pnpm run test` 等；**前端命令一律用 `pnpm`**，未配置则向用户确认命令）。
- **知识 / 全局要求性内容必须同步更新**：适用于全局的约定写入本文档；只影响单个领域的写入 `docs/`
  对应二级文档，以便后续会话继承。
- **需要人工验证的项交给用户**：当某个验证无法通过命令行反馈、或命令行验证的难度/效果远不如人工
  验证时（例如浏览器端视觉/交互、播放体验、局域网设备访问等），不强行用脚本模拟，应明确告知用户
  「该项需你手动验证 + 验证方法」，由用户执行。
- **禁止为验证长期占用命令行**：不得自行启动后台 webserver / 服务再用命令轮询等待；命令行链路验证
  以 httptest / 单次可返回的 curl 为准，浏览器与交互类验证一律列入「需手动验证清单」交给用户。
- 不主动 `git commit` / `push`，除非用户明确要求。
- 不擅自创建文档文件（`.md`、README），除非用户明确要求。
- 引用代码时使用 `文件路径:行号` 格式。

## 7. 强制规则（红线）

### 规则 A —— 影响环境的操作必须先经用户批准

涉及以下类别的操作，**必须先向用户说明「做什么 / 为什么 / 影响面」，得到确认后再执行**：

- 安装、卸载、升级依赖包（如 `pnpm add` / `pnpm install`、`go get`、`go mod tidy`、`pip install` 等）
- 修改全局环境变量、系统 PATH、注册表、防火墙、服务注册（NSSM）等环境级配置
- 在项目目录之外写入文件、修改用户主目录内容
- 启动/停止系统服务、部署、执行可能耗时或占用大量资源的操作
- `git` 提交、推送、打 tag、创建 PR

**例外**：纯只读操作（读文件、搜索、grep、查看状态）无需批准。

### 规则 B —— 代码核心标准：可维护、可读、架构清晰，严禁修补式代码

- **禁止修补式代码**：例如原逻辑只针对 A 情况，为新增的小概率 B 情况沿原逻辑链堆叠大量 `if` 特判
  分支。这是本项目的红线。
- 当需求需要扩展原有行为时，应把变化**归入统一的抽象**（策略 / 接口 / 配置驱动 / 数据驱动），而不是
  在调用链上打补丁。
- 判断标准：若一段逻辑因新增场景出现多份特判、重复分支，应停下来**重构**而非继续堆代码。
- 保持模块职责单一，遵守 plan 中的 ADR 与架构边界，不越界耦合。

### 规则 C —— 需求模糊时，必须提供选择并询问用户

- 当需求存在歧义、有多种可行方案、或选择会显著影响架构 / 体验 / 未来演进时：
  - 给出 2~3 个候选方案，说明各自利弊
  - 标出推荐项
  - **询问用户后再动手**，禁止自行臆断决定
- 例外：ADR / plan 已有明确结论的事项不必再问。

## 8. 质量与验证

- 开发阶段与验收标准见 plan 10 章（Phase 0~4），每个 Phase 的「验收」即完成判定，不得跳过测试。
- 测试策略见 plan 12 章：后端 `go test ./...`（`httptest` 全链路、mock 文件系统可注入、媒体相关打
  `//go:build integration`）；前端 Vitest + Testing Library。
- 阶段完成后建议打 tag 并写阶段小结（走查 ADR 与目录结构），修订 ADR 必须留痕并回填关联章节。

## 9. 沟通约定

- 回复简洁、直接、可执行；避免冗长的解释性前缀。
- 与项目文档一致，使用中文交流。

## 10. 二级文档索引（渐进披露）

> `AGENTS.md` 只承载「开发/维护任意功能必读」的信息；领域细节按需读取下表对应文档。

| 领域 | 文档 |
|---|---|
| 项目定位 / 核心原则 / ADR 摘要 / 技术栈 / 部署 / 静态托管 / 启动输出 | [docs/architecture.md](docs/architecture.md) |
| 开发环境（版本 / 安装 / FFmpeg PATH 陷阱 / go env） | [docs/environment.md](docs/environment.md) |
| 后端与数据层（时间戳 / SQLite / 迁移 / FTS5 / 剧集系列归组 / 多媒体源 / jobs / 事件总线） | [docs/backend.md](docs/backend.md) |
| 媒体管线（ffprobe / 容器判定 / 分段 MP4 / 格式工厂 / 能力判定 / 字幕 / 封面） | [docs/media.md](docs/media.md) |
| 前端（页签 keep-alive / 栏位栈 / Vidstack / 响应式 / 文件浏览器 / 卡片） | [docs/frontend.md](docs/frontend.md) |
| 现状清单 / 遗留待办 / 人工验证清单 / 未来方向 | [docs/status.md](docs/status.md) |
