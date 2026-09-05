# frontend.md — 前端实现事实源

> 改动 `frontend/src/{tabs,features}` 前必读。
> 技术栈见 AGENTS.md §1；UI 设计语言见 [UI.md](../UI.md)；实时通道见 [realtime.md](realtime.md)。

## 1. 多 Router 页签（keep-alive）

- 顶部 5 个页签（首页/视频库/搜索/工具/文件），每页签一个独立 TanStack Router 实例
  （`createMemoryHistory`，`tabs/`），组件树常驻不卸载（`display:none` 隐藏）
- **URL 唯一来源是活动页签**：切页签 `replaceState`，页签内导航由 `TabSync` 镜像 `pushState`
- 跨页签跳转走 `manager` 的 openVideo/openSeries/openLibrary/openFormat/openFileLocation
- **新增子路由必须归属某页签的 router 并在 `tabFromPath` 补映射**，禁止全局平级路由

### 1.1 文件页签

> API 端点见 decisions.md §2.2；后端实现见 backend.md §9。

机器级文件浏览器（HTTP 文件服务器模型）：每次进入目录后端实时读取，无索引/watcher。
布局 = 左侧三段侧栏（盘符/pin/多媒体源）+ 右上工具栏（面包屑 + 按钮组）+ 列表。

- 无「..」行——回上级统一用工具栏向上按钮
- 剪贴板为前端内存态；粘贴调 copy/move 入 jobs 后台任务
- 批量重命名 `RenameDrawer`：查找/替换预览，关闭即撤销，提交走同步接口
- 多媒体源：标记/取消/重扫；扫描完成监听 `events.jobs.done|failed(type=scan_source)` 即时失效
- 标记为系列：所选文件夹入队 mark_resource
- 详情页按需同步：单集 `source_status`；系列 `check`

## 2. Library 栏位栈

`LibraryLayout` 是 library 页签 root 的唯一编排者。

- **URL 即栈**：`/library`=浏览、`/library/video/:id`=+[单集详情]、`…/play`=+[播放]、
  `/series/:id`=+[系列详情]、`/series/:id/video/:videoId`、`/series/:id/play/:videoId`
- **单集详情永远带 series 上下文**：属系列的单集以独立路径打开时自动补齐 series 段
- 栈模型：每栏 50vw，视口只显示栈顶两栏；窄屏退化为单栏全页
- **播放器挂 Layout 根部绝对定位覆盖层**（keyed by videoId）：宽屏占右 50vw、窄屏整列
- 浏览栏：视图选择宽屏三项并排、窄屏折叠单下拉；搜索框即时筛选；高级筛选仅标签 chips
- 系列详情：成员列表头部编辑模式（排序/恢复/批量改名）；支持拖拽重排
- 共享组件：`SeriesPickerModal`、`SeriesRenameModal`
- 单集详情卡片序：元信息 → 播放历史合并卡 → 技术卡

## 3. 播放器（Vidstack）

> 三层动态流判定与语义见 [media.md](media.md) §4；播放选择记忆语义见 media.md §6。

- `@vidstack/react` v1.15+；样式 import 三件套 + `DefaultVideoLayout`
- **src 映射**：direct → `/api/stream/{id}`；remux → `/api/stream/{id}/remux?audio=N`；
  transcode → `/api/stream/{id}/hls/playlist.m3u8?session=<uuid>`；none → 按钮禁用
- **src 必须显式带 `type`**（端点无扩展名，否则回退 HEAD 探测失败）
- **hls.js 本地注入不走 CDN**：`provider.library = Hls`
- 多音轨菜单注入播放器自带设置菜单；选轨重建流后 seek 回原进度
- **播放选择记忆应用/保存**：
  - 自动应用每字段至多一次；音量/静音经受控 props 应用
  - 保存触发点：音轨菜单选择、字幕切换、音量变化（防抖 400ms）
  - prefs 查询 `refetchOnMount: 'always'`
- **伪全屏**：触摸端走纯 CSS 伪全屏（`useFakeFullscreen`），桌面保持原生

## 4. 工具页签

### 4.1 格式工厂

> 后端实现与策略见 [media.md](media.md) §3.1。

- 位置：`features/tools/convert/`
- 入口：`/tools/convert`；来源：`/files` 页 checkbox 菜单「格式工厂」
- 核心组件：`ConvertProbe`（探测结果 + 预设选择 + 操作面板）
- 预设快速填充：探测结果 → 自动选择「快速 MP4」；位图字幕 → 自动勾选「烧录」
- 字段禁用规则：copy 时禁用 video/audio/burn；重编码时启用

### 4.2 缓存管理

> API 见 decisions.md §2.4。

- 位置：`features/tools/cache/`
- 层级化主从视图：概览条 + 可搜索系列列表 + 孤儿缓存 + 未归组单集
- 默认只显示有缓存的系列/单集；底部一次性开关「显示无缓存的...」
- Remux 缓存按视频归属显示，仅在归属处清理

### 4.3 开发者工具

- 位置：`features/tools/devtools/`
- console 劫持 + devLog 采集（环形缓冲 2000 条，开关 localStorage 持久化）
- 归档经 `POST /api/devlogs`；查看/下载在 PC 端

## 5. 响应式

- 断点：1024px（宽/窄屏切换）
- 宽屏：Library 栏位栈并排、文件页三段侧栏
- 窄屏：单栏全页、NarrowBack 返回条、播放器全屏
