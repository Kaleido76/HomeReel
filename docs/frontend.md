# frontend.md — 前端架构与约定

> 改动 `frontend/src/{tabs,features}` 前必读。技术栈：React 19 + TypeScript、Vite、Tailwind CSS 4、
> TanStack Router / Query、Vidstack（Range 直连，无 HLS）；**前端命令一律用 `pnpm`**。

## 1. 多 Router 页签（keep-alive）

- 顶部 5 个页签（首页/视频库/搜索/工具/文件），每页签一个独立 TanStack Router 实例
  （`createMemoryHistory`，`tabs/`），组件树常驻不卸载（`display:none` 隐藏）＝状态永不丢
  （滚动/筛选/播放器/上传状态切页签不丢）。
- 页面组件 `React.lazy` 分包懒加载；ARIA tablist 语义 + 方向键切换。
- **URL 唯一来源是活动页签**：`TabManager.activate` 切换页签用 `replaceState`（不产生历史），活动页签内
  导航由 `TabSync` 镜像 `pushState`；`popstate` 反解析 URL→页签并 `navigate({href, replace})` 对齐该页签
  memory history。
- 跨页签跳转走 `openVideo/openSeries/openLibrary`（切到 library 页签再 navigate）。
- **新增子路由必须归属到某个页签的 router**（`tabs/routers.ts`），并在 `tabFromPath` 补路径映射，
  禁止添加全局平级路由；改动此类路由结构时勿破坏 keep-alive。
- 深链/刷新按 URL 恢复对应页签视图；库的视图状态（`view/q/sort/page` + 高级筛选 `tags/desc/genre/year`）与
  搜索词 `q` 存在 URL 参数中。

## 1.1 文件页签（泛用机器文件浏览器，2026-08 增量）

- 独立第 5 页签 `/files`（标签「文件」，旧 Explorer 页签已随存储卷模型移除）。**不涉及存储卷模型**：
  直接浏览目标机器所有**本地盘符**（固定盘 + 可移动盘，排除网络映射盘）。文件细节**不入库**——每次
  进入/跳转目录后端才实时读取（HTTP 文件服务器模型），无索引/扫描/watcher。
- **布局**（全宽无留白，`TabHost` 的 `selfScrolling` + `fullBleed`）：左侧 `DriveRail` 顶天立地窄框，
  被**两个可拖动分隔条**分为三段——盘符（`/api/disks`）、pin 常用路径、【多媒体源】（`/api/files/sources`）；
  右上 `Toolbar` 为**两行**：顶部整行**面包屑**（`pathSegments` 拆段、每段可点击跳回祖先，最左「这台电脑」
  图标回初始态），底部操作按钮**全部为图标 + 即时 Tooltip**（上级、固定（已固定蓝色激活）、标记为多媒体源
  （激活紫）、剪切/拷贝/粘贴/重命名/**批量重命名（需 ≥2 勾选）**/删除、**全选/反选**（对当前目录所有可见
  条目）、**标记为单集**（所选文件/文件夹递归全部入库为单集，独立于媒体源维护）、**标记为系列**（所选
  文件夹整体打包成系列；未选文件夹且当前目录含视频时**标记当前目录**）、多媒体视图（激活蓝），文字说明经
  自定义 `Tooltip` 组件悬停即显——深色气泡、无原生 title 延迟）；右端状态文本显示项数与勾选数。排序控件
  不在工具栏。
  `FileListView` 列表视图（flex 行布局，**无「..」回上级行**——回上级统一用工具栏向上按钮，该按钮略加宽
   便于点击；**Checkbox 整格命中**切换勾选不触发行操作，行 hover 基础变色；
   **checkbox 列支持拖选**——按下左键上下拖动，把起点行与当前指针行之间的连续区间批量置为「起点行的勾选状态」
   （起点未勾选则区间全勾选、起点已勾选则区间全取消；位移小于阈值视为普通单击；v1 不做边缘自动滚动），
   文件按类型显示**不同图标与柔和颜色**（`fileType.ts`：视频蓝/音频紫/图片绿/压缩包琥珀/文档灰/代码青绿）；
   **列宽可拖拽**调整，localStorage `files.colWidths` 持久化；**排序改为点击列标题**——点当前排序列翻转
   升/降序（活动列标题旁显示 ↑/↓ 箭头），切换列时名称默认升序、大小/时间默认降序）。
- **剪贴板 + 底部工具抽屉**：剪切/拷贝为前端内存态（`clipboard`，刷新即失），工具栏状态文本不再显示
  「剪贴板 N 项」——剪切/拷贝后本次选择进入**底部工具抽屉**（通用外壳 `ToolDrawerShell` + 内容
  `ClipboardDrawer`）：与文件列表**同宽、不遮列表、滑出时把列表底边顶起**（作为文件列 flex 的末子项 +
  `max-height` 过渡）；抽屉与列表勾选**完全解耦**（勾选变化不影响抽屉），仅支持**移除单项 / 全部清除**；
  「粘贴」调 `/api/files/copy|move` 入 **jobs 后台任务**（复用 JobsIndicator 展示进度）；剪切粘贴成功后
  抽屉清空关闭、复制粘贴后保留（可继续粘到别处）。`ToolDrawerShell` 为纯外壳，接受 `heightClass` 控制
  展开高度（默认 `max-h-72`，Tailwind 类须在调用处字面出现），后续其它工具抽屉复用。
- **批量重命名抽屉**（`RenameDrawer`，工具栏按钮，勾选 ≥2 项可点）：VS Code 式替换栏（查找/替换两个
  输入框 + **无视大小写**、**正则**两个开关 + **开始替换**按钮，正则非法时红框提示并禁用按钮）；下方
  左右两栏**原文件 ↔ 新文件名**逐行对应预览（变化的行绿色高亮，行数多时可滚动）；关闭抽屉 = **撤销**
  （不执行任何重命名）；「开始替换」调 `POST /api/files/renames`（批量同步，非 jobs），成功/部分失败后
  关闭抽屉、清空勾选并刷新列表。
- **多媒体源**：标记/取消标记仅经工具栏按钮（激活态），**取消标记经强确认**（提示该源下所有单集与系列将
  从库中移除），确认后该源的全部已入库内容从库中消失（磁盘文件不受影响）；源行有「重新扫描」按钮 +
  扫描中 spinner + 离线徽标（`available=false` 表示根目录当前不可达）；源列表 5s 轮询感知扫描完成。
- **手动标记系列（2026-08 管理面定稿）**：工具栏「标记为系列」入队 mark_resource 后台任务（复用
  JobsIndicator 进度），把所选文件夹（或当前目录）打包为系列——须位于媒体源内（源外路径被拒绝并提示先
  添加为多媒体源），成员 = 根目录直接一级子文件。**「标记为单集」与离散资源概念已彻底清除**。详见
  [DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md) §6.12。
- **详情页按需同步（2026-08 管理面定稿）**：
  - 单集详情（`VideoDetailPane`）：详情响应带 `source_status`（ok | moved | missing）。moved →
    黄色警告「源文件已改名或移动」+「同步」按钮（`POST /api/videos/{id}/sync`）；missing → 红色警告
    「源文件不存在」+「移除这个单集」按钮（`DELETE /api/videos/{id}`，仅删元数据、不动文件）。
  - 系列详情（`SeriesDetailPage`）：详情响应带 `check`（根目录存在性 + 成员存在性 + 未入库新文件）。
    `out_of_sync` 时顶部黄色警告列出差异 +「同步」按钮（`POST /api/series/{id}/sync`）。
  - **手动归组 UI 已移除**（`VideoMetaPanel` 不再有系列/集号下拉）：归属由路径决定，PATCH 不再接受
    show/season/episode 字段。
- **v1 不含**上传/下载/新建文件夹；仅列表视图（无网格视图）。路径状态存 URL（`?path=`），刷新/深链可恢复。
- 目录轮询 `refetchInterval: 5000` 以感知后台复制/移动完成后的内容变化。

## 2. Library 栏位栈布局（宽屏 ≥lg）

`LibraryLayout` 是 library 页签 root 的唯一编排者——自行解析 `location.pathname` 还原成**栏位栈**
（`parseStack`，`LibraryLayout.tsx`），子路由 component 均为空（URL 仅供匹配/深链，**不渲染 `<Outlet/>`**）。

- **栏位栈**：浏览永远是栈底，选择视频/系列 push 详情栏、播放 push 播放栏；每栏占 `50vw`，视口一次只见
  **栈顶两栏**（位移 `translateX = -(栈深-2)*50vw`，栈深≤2 为 0），更早的栏位被挤到屏左外
  （keep-alive 仍在 DOM，选中高亮随栈第 2 栏保留）。
- **URL 即栈**：`/library`=`[浏览]`、`/library/video/:id`=`[浏览][单集详情]`、
  `/library/video/:id/play`=`[浏览][单集详情][播放]`、`/series/:id`=`[浏览][系列详情]`、
  `/series/:id/video/:videoId`=`[浏览][系列详情][单集详情]`、`/series/:id/play/:videoId`=`[浏览][系列详情][播放]`。
- **过滤器是浏览栏的一部分**（`全部/单集/系列` tab + 搜索框 + 高级筛选按钮，位于浏览栏顶部，随浏览栏一起被挤出屏幕；
  窄屏仅浏览视图显示）。
- **高级筛选面板**（`AdvancedFilter.tsx`，替换了原排序下拉）：JobsIndicator 同款开关——首次点击展开、再次点击
  **应用并收起**；内含 标签（多选 chips，来自 `/api/tags`）/ 简介 / 类型 / 年份 过滤器 + 排序下拉；面板保留
  本地草稿，应用前不生效；有激活筛选时按钮显示蓝色徽标计数，面板内「重置」仅清空草稿。
  `tags` 在 URL 中逗号连接存储，`parseGridSearch` 还原为数组。
- 系列详情成员行有**播放/续播 + 详情**两按钮：播放直达播放栏（左侧保留系列详情，不再插入单集详情）；
  详情打开单集详情栏（顶部有「返回系列」条）。
- **替换逻辑（仅系列剧集）**：从系列内单集详情点播放时，单集详情栏被替换成播放栏
  （`/series/:id/video/:v` → `/series/:id/play/:v`）。单集从浏览栏打开的行为不变
  （`[浏览][单集详情]` → 播放 → `[单集详情][播放]`）。
- **导航目标由栏位栈父栏决定**（`LibraryLayout` 依栈计算 `playHref`/`exitHref`/`goHref` 传入
  `VideoDetailPane`/`PlayerPane`，**不依赖视频的 `series_id` 属性**）：单集详情栏在系列内 → 播放跳
  `/series/:id/play/:videoId`，在浏览栏上 → `/library/video/:id/play`；播放栏退出 → 父栏路径，上下集/
  接下来播放 → 系列上下文走 `/series/:id/play/:v`、否则 `/library/video/:v/play`。
- **单集详情技术卡片**（`VideoTechCard`，2026-08）：详情栏内专门卡片集中展示 probe 技术信息——视频/音频
  编码、分辨率/帧率、容器、时长/大小、faststart 标记；并给出**逐流解码能力说明**（容器/视频/音频分别指出
  哪一部分「不支持前端解码」，基于 `playability.reportFor`，与 `canPlay` 同一判定/缓存）。字幕轨道清单
  （侧边文件 + 内封文本轨）随卡片展示，与播放器共用 `['subtitles', id]` 查询缓存。
- 播放栏=`PlayerPane`：**顶部退出播放条** + 播放器 + 简略元信息 + 系列内上下集/自动连播 +
  **「接下来播放」列表**（当前集后的至多 3 个成员，小缩略图低调展示）。
- **窄屏（<lg）**退化为单栏全页（浏览→详情→播放，`NarrowBack` 返回条），复用同一批组件。
- 网格筛选状态（`view/q/sort/page` + `tags/desc/genre/year`）存于 `LibraryLayout` 组件 state
  （`features/library/types.ts`），仅 `/library` 页写回 URL。子页面组件（`VideoDetailPane`/`PlayerPane`/
  `SeriesDetailPage`）以 props 接收 id，不用 `useParams`。
- 播放器切走自动暂停：`VideoPlayer` 订阅页签 store，非 library 页签即 `remote.pause()`，切回保持暂停由
  用户手动播放。
- 视频卡片任意页签点击 → 切到视频库页签打开播放器。

## 3. 播放器（Vidstack）

- 用 `@vidstack/react`（v1.15+，**勿装废弃的 `@vidstack/player`**）。
- **纯 Range 直连（2026-08，无 HLS / hls.js）**：可播放性在**进入播放器之前**由 `lib/playability.ts` 的
  `canPlay()` 决定——probe 元数据（容器/视频编码/音频编码/segmented）→ MIME + codecs 串 →
  `HTMLMediaElement.canPlayType()`。不可播放时播放按钮禁用，详情/播放栏显示「格式工厂转换」引导，播放器
  组件本身**永远不会拿到不可直连的文件**。
- 样式 import `@vidstack/react/player/styles/{base.css,default/theme.css,default/layouts/video.css}`，
  `DefaultVideoLayout` 从 `@vidstack/react/player/layouts/default` 导入。
- **`/api/stream/{id}` 无扩展名，`MediaPlayer` 的 `src` 必须显式带 `type`**（`VideoSrc`），否则
  回退 HEAD 探测失败即报 `could not find a loader`。
- **播放器填满播放栏高度**：`MediaPlayer` 加 `className="h-full w-full bg-black"` +
  `style={{ aspectRatio: 'auto' }}` 覆盖 Vidstack 默认 16:9（`aspect-ratio: inherit` 沿用到
  provider/video，`object-fit: contain` 居中），底部黑边留给控制条，进度条不压 WebVTT 字幕
  （WebVTT 字幕按 overlay 90% 定位在画面内）。

## 4. 响应式 UI 与布局

- 横跨手机/平板/笔记本/电视(2K/4K)。`index.css` 增加 `3xl(1920px)` 断点。
- 容器宽度自适应：页签宿主 `max-w-[1600px]`、文件页 `max-w-[1920px]`，大屏高密度。
- `TabHost` 对 library 页签**全宽无 max-w/无 padding**（vw 单位布局，留白会破坏面板感）；文件页仍
  `max-w-[1920px]`。
- `useMediaQuery`（`lib/useMediaQuery.ts`）选择窄屏/宽屏布局。
- **播放器/系列详情**：`PlayerPane`/`SeriesDetailPage` 只渲染内容（返回由三栏动画 / 窄屏 `NarrowBack` 承担）。

## 5. header 与页签 UI 风格（2026-08 重设计）

- 两侧顶格（无 `max-w` 居中约束，`px-3 sm:px-6` 响应式内边距）。
- 页签为独立圆角胶囊按钮（`rounded-md px-3 py-1.5`），激活态 `bg-neutral-900 text-white` 深色胶囊、
  非激活 `text-neutral-600 hover:bg-neutral-100`，**不使用蓝色下划线**。
- 页签文字标签 `<sm` 断点隐藏仅留图标（`hidden sm:inline`），窄屏页签列表 `overflow-x-auto`。
- Logo 与「退出登录」保持原样。
- **工具页签（2026-09，替代原「重封」/「格式工厂」独立页签）**：图标 `Wrench`，root `/tools`。布局与文件页签
  类似：**左侧垂直工具栏 + 右侧工具面板**（`features/tools/ToolsPage.tsx`，`?tool=id` 存于 URL；工具注册表
   `features/tools/tools.ts`，访问过的工具面板保持挂载，切换不丢状态）。左栏为**硬朗/方角/等宽风格**：
   按钮无圆角、与栏等宽（无内边距），激活态 `bg-neutral-900 text-white`（呼应 TabBar）。当前含两个工具——
   **格式工厂**与**缓存管理**（`CacheManagerPage`，见下）。格式工厂：把任意视频/文件夹转为浏览器可播放的
   Faststart MP4 副本（见 [media.md](media.md) §3.1）。
  文件页签工具栏「格式工厂」按钮把勾选（或含视频的当前目录）经 `manager.openFormat` 移交到工具的
  **待转换队列**（模块 store `features/tools/format/queue.ts`，用 `usePending` 订阅）。面板自上而下：
  - **操作面板**（`FormatFactoryPage` 内 `OperationsPanel`）：未选择文件时整体**遮罩禁用**；预设工具
    **快速 MP4 / H.264 / H.265**（`features/tools/format/presets.ts`）仅作**快速填充**（点击填参数），
    下方**参数表单持续存在**（视频编码、清晰度 CRF、音频模式、AAC 码率、烧录字幕），改动后标记
    「已自定义」；`POST /api/convert` 携带当前 `params`。表单字段按上下文**禁用**（无损拷贝→禁用 CRF、
    保留原音→禁用码率、无字幕轨→禁用烧录、检测到非通用音频→禁用「保留原音」）。
  - **检测信息**：`POST /api/convert/probe` 逐个探测所选文件（目录展开为直接一级视频），操作面板顶部
    以引导性文字提示（位图字幕将自动降级、AC3 将转 AAC 等）；**待转换清单每行显示探测徽标**
    （位图字幕 / 文本字幕 / AC3 等），目录行显示汇总（N 个视频、位图字幕 ×N）。
   - **待转换清单**（含目标输出路径）+「开始转换」：可随时继续追加批次入队，即使有任务运行中。
   - **转换队列**（复用 `GET /api/jobs`，按 `type==='convert'` 过滤）：进行中任务在前（进度条 + ETA +
     子任务），历史在后（成功/失败），每行标注所用预设（解析 `job.extra` 的 params 反查）。
- **缓存管理**（`CacheManagerPage`，2026-08）：管理可重建缓存，粒度贴合实际用途——
  - **字幕缓存**（重点）：按视频列出提取出的 vtt（同一剧集系列的视频合并在一组），每条字幕可单删
    （`DELETE /api/cache/subtitles/{videoId}/{track}`）、按视频清空（`DELETE /api/cache/subtitles/{videoId}`）、
    或一键清空全部字幕（`DELETE /api/cache?kind=subtitle`）。源文件更换/提取乱码时逐条删除，重播即重建。
  - **封面/缩略图**：正常使用中的**不提供删除**（扫描时重建，删无收益），仅当它们成为孤儿时经「清理孤儿」清掉。
  - **孤儿缓存**（`DELETE /api/cache/orphans`）：库中已无对应视频的残留文件，打包显示总量并按类给明细，
    一键整体清理。孤儿判定由后端按缓存文件名解析视频 ID 并与全部视频索引对比。所有清理不影响源文件。
- **后台任务按钮（JobsIndicator）**：header 右侧、退出登录旁。双鱼箭头图标（`RefreshCw`）在有长时任务
  时 `animate-spin` 并显示数量徽标；点击弹出任务面板（`features/jobs/JobsIndicator.tsx`，轮询
  `GET /api/jobs`，进行中 1s / 空闲 15s）。面板为 `absolute` 覆盖自身区域，**不因点击外部消失**，仅
  再次点击按钮收起。任务行展示名称 + 进度：`progress>=0` 为确定进度条，否则 `.indeterminate-bar`
  动态滑条（`index.css`）。进行中的确定进度任务展示**剩余时间估算**（`job.eta_seconds`，后端按
  进度×耗时推算，`formatEta` 格式化）。只显示 `internal=false` 的长时任务（扫描/转换/复制/移动）。

## 6. Explorer 文件页

> 已移除（2026-08）：旧 Explorer（存储卷模型）随后端 storages 一并删除，由「文件」页签
> （files，见 1.1）取代。本节保留历史说明供参考。
>
> - 原实现为存储卷可折叠侧边栏（宽屏默认收成窄图标轨 `w-14`，展开为管理面板；窄屏为顶部横向 chips 条）。
> - 宽屏（≥lg）Finder 多列浏览（`ColumnBrowser`，每级目录一列，点文件夹右侧开新列、点列头回退），
>   窄屏回退单列表（`FileList`）。

## 7. 卡片 / 网格

- `VideoCard` 横版 16:9（row-span-2）、`MediaGrid` dense 混排仅用于首页/搜索。
- 库内列表不再用卡片网格（`LibraryGrid`/`SeriesCard` 已删除）。
- 浏览栏系列封面显示**总时长徽标**（`/api/series` 返回 `total_duration`，成员时长合计）。
- 系列类型文案统一为「**系列剧集**」/「电影部」（非「剧集季」）。
- 浏览栏**悬浮卡片**：每行一张 `border-neutral-200 bg-white rounded-lg shadow-sm` 卡片，行间 `gap-1.5`
  留白；hover `-translate-y-0.5` 上浮 + `border-blue-400/60` 描边变蓝 + `bg-blue-50/40` + `shadow-md`，
  `transition-all duration-200`；选中 `border-blue-500 bg-blue-50 ring-1 ring-blue-500/30`；封面无缩放动画。
