# docs/ — HomeReel 二级文档

> 本目录是 `AGENTS.md` 的**渐进式披露**内容层：`AGENTS.md` 只保留开发/维护**任意**功能都必读的通用信息，
> 领域化、环境化、状态化的知识按需放在这里，按「要动的领域」定向读取。

## 1. 文档树

| 文档 | 覆盖领域 | 何时需要读 |
|---|---|---|
| [architecture.md](architecture.md) | 项目定位、核心原则、ADR 摘要、技术栈、部署形态、config.yaml、静态托管、启动输出 | 新成员上手、跨模块改动、评估架构影响 |
| [environment.md](environment.md) | 本机开发环境（版本、安装、FFmpeg PATH 陷阱、go env） | 首次搭建/排障环境、安装依赖 |
| [backend.md](backend.md) | 后端与数据层约定：时间戳、SQLite、迁移、FTS5、剧集/系列归组、多媒体源、jobs、事件总线 | 改动 `backend/internal/{store,scanner,fservice,files,jobs,events,db}` |
| [media.md](media.md) | 媒体管线：ffprobe/ffmpeg、容器判定、分段 MP4、格式工厂、HLS 转码、能力探测、字幕、封面 | 改动 `backend/internal/{media,streaming}` 或前端播放器媒体相关 |
| [frontend.md](frontend.md) | 前端架构：多 Router 页签 keep-alive、栏位栈、Vidstack、响应式、文件浏览器、卡片 | 改动 `frontend/src/{tabs,features}` |
| [status.md](status.md) | 已实现能力摘要、遗留待办、人工验证清单、未来方向 | 规划新功能、验收、会话交接 |

## 2. 读取策略

- 进项目先读 `AGENTS.md`（通用必读）。
- 再按本次任务触达的领域，读上表对应的一篇（或几篇）。
- 权威开发方案永远是 `DEVELOPMENT_PLAN.md`：发现本文档与代码或 plan 不一致时，以
  `DEVELOPMENT_PLAN.md` 为准，并向用户指出差异。

## 3. 维护约定

- 会话中发现任何适用于**项目全局**的约定（工具链、命令、命名、工作流程），回填到 `AGENTS.md`；
  只影响**单个领域**的，写进对应二级文档。
- 新增领域时在此表登记；删除领域时同步清理。
- 本目录文件由会话按需维护，禁止引入与 `DEVELOPMENT_PLAN.md` 冲突的内容。
