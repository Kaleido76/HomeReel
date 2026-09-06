# frontend.md — 前端实现事实源

> 改动 `frontend/src/{tabs,features}` 前必读。
> 技术栈见 AGENTS.md §1；UI 设计语言见 [UI.md](../UI.md)；实时通道见 [realtime.md](realtime.md)。

## 1. 多 Router 页签（keep-alive）

- 顶部 5 个页签（首页/视频库/播放/工具/文件），每页签一个独立 TanStack Router 实例
  （`createMemoryHistory`，`tabs/`），组件树常驻不卸载（`display:none` 隐藏）
- **URL 唯一来源是活动页签**：切页签 `replaceState`，页签内导航由 `TabSync` 镜像 `pushState`
- 跨页签跳转走 `manager` 的 openVideo/openSeries/openPlay/openLibrary/openFormat/openFileLocation
- **新增子路由必须归属某页签的 router 并在 `tabFromPath` 补映射**，禁止全局平级路由

### 1.1 播放页签

> API 端点见 decisions.md §2.4；媒体层见 media.md §4。

- 位置：`features/player/`；入口：`/player`（空态，尚未播放时）与 `/player/:videoId`（播放中）
- 系列上下文用 `?series=` 携带：驱动侧栏剧集列表与上一集/下一集
- 播放跨页签进入走 `manager.playVideo(videoId, seriesId?)`；顶部无退出按钮——点击页签栏「视频库」即可回到浏览
- 自动暂停：切出播放页签即 `remote.pause()`（keep-alive 防后台出声），见 `VideoPlayer.tsx`
- **PC 端布局**（大屏三区）：
  - 左侧：视频播放区（保持影视自然宽高比，通常 16:9），溢出裁剪
  - 右侧窄栏（`320px`）：系列海报+元信息（shrink-0）；下方剧集列表（flex-1 滚动，当前集高亮）
  - 底部控制栏：上一集/下一集/自动连播开关；右侧预留扩展空间
- 响应式：窄屏（`<1024px`）侧栏堆叠到视频下方（`flex-col → lg:flex-row`），后续可进一步适配

### 1.1 文件页签

> API 端点见 decisions.md §2.2；后端实现见 backend.md §9。

机器级文件浏览器（HTTP 文件服务器模型）：每次进入目录后端实时读取，无索引/watcher。
布局 = 根视图入口（`HomePane`）或全宽文件列表 + 工具栏（面包屑 + 按钮组）。

- **根视图**（无路径）：`HomePane` 铺开三组入口块（盘符/常用目录/多媒体源），替代旧侧栏常驻导航
- 非根视图：全宽列表，无侧栏；面包屑「这台电脑」图标回根视图
- 无「..」行——回上级统一用工具栏向上按钮
- 剪贴板为前端内存态；粘贴调 copy/move 入 jobs 后台任务
- 批量重命名 `RenameDrawer`：查找/替换预览，关闭即撤销，提交走同步接口
- 多媒体源：标记/取消/重扫；扫描完成监听 `events.jobs.done|failed(type=scan_source)` 即时失效
- 标记为系列：所选文件夹入队 mark_resource
- 详情页按需同步：单集 `source_status`；系列 `check`
- **移动端适配**（`<1024px`）：PC/移动端统一布局；工具栏精简为高频按钮 + 「更多」溢出菜单；列表行触控优化（checkbox 加宽、行高加大）

## 2. Library 栏位栈

`LibraryLayout` 是 library 页签 root 的唯一编排者。

- **URL 即栈**：`/library`=浏览、`/library/video/:id`=+[单集详情]、
  `/series/:id`=+[系列详情]、`/series/:id/video/:videoId`=+[系列详情][单集详情]
- **单集详情永远带 series 上下文**：属系列的单集以独立路径打开时自动补齐 series 段
- 栈模型：每栏 50vw，视口只显示栈顶两栏；窄屏退化为单栏全页
- **播放不在本页签**：播放由播放页签承接（见 §1.1），库只负责浏览与详情
- 浏览栏：视图选择宽屏三项并排、窄屏折叠单下拉；搜索框即时筛选；高级筛选仅标签 chips
- 系列详情：成员列表头部编辑模式（排序/恢复/批量改名）；支持拖拽重排
- 共享组件：`SeriesPickerModal`、`SeriesRenameModal`
- 单集详情卡片序：元信息 → 播放历史合并卡 → 技术卡

## 3. 播放器（Vidstack）

> 三层动态流判定与语义见 [media.md](media.md) §4；播放选择记忆语义见 media.md §6。
> 播放页签宿主与 URL 见 §1.1。

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
- **伪全屏**：触摸端走纯 CSS 伪全屏（`useFakeFullscreen`），桌面保持原生；播放页签
  全出血布局（`TabHost` fullBleed），伪全屏固定定位覆盖含 Header 的整个视口

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

- 断点：1024px（宽/窄屏切换），统一由 `lib/breakpoints.ts` 常量 `WIDE` 和 `useIsWide()` hook 管理
- 宽屏：Library 栏位栈并排、缓存页 master-detail 双栏；文件页全宽无侧栏
- 窄屏：单栏全页、NarrowBack 返回条（`components/NarrowBack.tsx`）、播放器全屏；文件页与 PC 统一布局
- 拖拽手势统一使用 Pointer Events（`lib/drag.ts`），触摸/鼠标/笔输入均可工作
