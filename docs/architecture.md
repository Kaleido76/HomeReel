# architecture.md — 项目定位、原则、ADR 与技术栈

> 本文件是 `AGENTS.md` 第 4/5 节的展开层。权威细节以 `DEVELOPMENT_PLAN.md` 为准（ADR 全文见 plan §0.4）。

## 1. 项目定位

HomeReel 是**个人视频资料管理平台**（DAM，Digital Asset Management），部署在家里的一台 PC 上，
通过局域网 Web 界面供所有设备（PC / 手机 / 平板 / TV）访问。

- **核心能力：视频管理**——目标是「简易 JellyFin」体验（海报墙、元数据、剧集/季/集分组、继续观看、
  首页行式浏览），但**不做** JellyFin 的冗余加法功能（插件系统、多用户档案、直播、音乐库等）。
- **辅助能力**：基础文件管理（浏览 / 上传 / 下载 / 重命名 / 移动 / 删除）。
- **定位**：单用户、单访问口令；**支持多终端并发访问**（卧室/书房 PC、同机多标签页可同时在线，
  各自独立会话，互不挤占）；Windows 优先部署。

## 2. 核心原则（plan §0.2）

1. 视频体验优先
2. 保留任意文件管理能力
3. AI 可长期维护（模块化、低耦合、显式优先）
4. UI 现代化
5. Windows 优先部署
6. 单用户：唯一所有者，通过一个访问口令保护；**支持多终端并发访问**（每终端独立会话，可同时看视频 + 整理文件，互不挤占）
7. **最高级原则**：一切功能围绕「元数据」扩展，而非「文件系统」——文件系统只保存字节，元数据负责查找、理解与组织
8. Library 提供简易 JellyFin 媒体库体验，但不引入冗余功能
9. **功能正确优先，禁止过度优化**（用户新增原则）：先保证功能正确，再谈性能与并发。两个操作彼此无依赖时可异步并行；但凡存在**条件竞争风险**时，一律先用同步串行手段保证正确性，待有真实瓶颈证据后再优化

## 3. 关键架构决策（ADR 摘要，全文见 plan §0.4）

- **单 Go 服务**：v1.0 全程不拆服务；内部按包拆分，预留拆分边界（ADR-001）
- **认证（多终端并发会话）**：单口令 + 会话 Cookie；每终端独立登录/独立会话，登出只清自身，互不挤占（ADR-002）
- **数据库**：SQLite（纯 Go 驱动 + WAL），演进路线 WAL → FTS5 → 队列 → 缓存，**不默认迁移 PostgreSQL**（ADR-005）
- **播放策略**：能力探测三层——可直连 → HTTP Range；不可直连 → HLS 转码；仍不可 → 转码兜底；HLS 默认 `auto`（ADR-006）
- **文件身份**：`(source_id, file_id, relative_path)` 三元组 + `(file_id, size, mtime)` 指纹；**file_id 全局匹配**（跨源移动保持同一视频）（ADR-007）
- **多媒体源**：视频库入库单位是用户标记的**多媒体源**目录（`media_sources`，轻量持久化标记 + 扫描单位，不参与文件浏览生命周期）；嵌套源按路由表由子源优先；源根不可达时扫描中止、不动库（ADR-011 / ADR-014，2026-08 替换原 storages 多数据源/热插拔模型）
- **系列组织**：视频库统一为「单集 + 系列」（一季/一部一个系列，`seasons.kind`=tv/movie，成员按位次排序允许缺失）；系列间弱关联 `series_links`；目录/文件名规则识别（ADR-015）。**元数据刮削（ADR-016，TMDB 在线 + NFO 侧边文件）已整体移除（2026-08）**，仅剩手动编辑。
- **AI 解耦**：事件总线（`VideoImported` 等），AI/OCR/转写作为 Listener（ADR-010）
- **搜索隔离**：`SearchProvider` 接口，当前 FTS5，Meilisearch 后续替换（ADR-009）
- **前端形态**：文件浏览器（files）与 Library 是两个共享 API/播放器/元数据的 App（ADR-013）

## 4. 技术栈

- 前端：**React 19** + TypeScript、Vite、Tailwind CSS 4、TanStack Router / Query、
  Vidstack（播放器，HLS 用本地 hls.js）；Vitest + Testing Library（尚未配置 test script）。
- 后端：Go 1.22+、标准库 `net/http`、`modernc.org/sqlite`（免 CGO）、`fsnotify`、`slog`、ULID。
- 媒体：FFmpeg / ffprobe（探测、缩略图、HLS、字幕）。
- 部署：`CGO_ENABLED=0` 单 `.exe` + `data_dir` + `config.yaml` + ffmpeg 二进制。

## 5. 部署形态与配置

- 单二进制：后端通过 `config.server.static_dir` 托管前端构建产物；空则自动探测 `./static`（部署形态）
  或 `../frontend/dist`（backend 目录运行 dev 形态），均不存在则仅 API。
- 运行形态：`data_dir`（数据库、缩略图、转码缓存等）+ `config.yaml`（`media.ffmpeg_path` /
  `media.ffprobe_path` 等）+ 系统 PATH 或 config 中的 ffmpeg 二进制。
- 配置细节（config.yaml 各键的完整说明）见 plan §8。

## 6. 静态托管（SPA）

- `api.New` 注册 `GET /` 兜底：未命中 `/api` 的 GET 从静态目录出文件，无扩展名路径回退 `index.html`
  （深链刷新直达），未知 `/api/*` 保持 JSON 404（不返回 SPA）。
- 开发时前端仍走 Vite `:5173`（`/api` 代理到 8080），两者互不干扰。

## 7. 启动输出

- `cmd/server` 显式 `net.Listen` 后再启动日志打印可达地址。
- 绑定 `0.0.0.0`（wildcard，config 默认）时经 `netutil.URLs` 枚举本机地址（IPv4 全量 + IPv6 ULA
  `fc00::/7`，排除 loopback/link-local/全局 IPv6 隐私地址；无可枚举时回退 `localhost`）。
- 端口被占用则启动即失败（退出码 1）。
