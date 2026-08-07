# frontend.md — 前端架构与约定

> 改动 `frontend/src/{tabs,features}` 前必读。技术栈：React 19 + TypeScript、Vite、Tailwind CSS 4、
> TanStack Router / Query、Vidstack + 本地 hls.js；**前端命令一律用 `pnpm`**。

## 1. 多 Router 页签（keep-alive）

- 顶部 6 个页签（首页/视频库/搜索/文件/重封/文件（新）），每页签一个独立 TanStack Router 实例
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

## 1.1 文件（新）页签（泛用机器文件浏览器，2026-08 增量）

- 独立第 6 页签 `/filesnew`（标签「文件（新）」），与旧 Explorer 完全并行，**不涉及存储卷模型**：
  直接浏览目标机器所有**本地盘符**（固定盘 + 可移动盘，排除网络映射盘）。文件细节**不入库**——每次
  进入/跳转目录后端才实时读取（HTTP 文件服务器模型），无索引/扫描/watcher。
- **布局**（全宽无留白，`TabHost` 的 `selfScrolling` + `fullBleed`）：左侧 `DriveRail` 顶天立地窄框，
  被**可拖动分隔条**分为上下两栏（拖拽调整占比，localStorage `filesnew.railSplit` 持久化）——上栏自动
  枚举的盘符（`/api/disks`），下栏 pin 常用路径（存后端 `settings` 表，多终端共享）；右上 `Toolbar`
  为**两行**：顶部整行**面包屑**（`pathSegments` 拆段、每段可点击跳回祖先，最左「这台电脑」图标回初始态），
  底部操作按钮（上级、**固定/已固定**（点击即固定或取消，激活态变色）、剪切/拷贝/粘贴/重命名/删除、
  名称/大小/修改时间排序、**多媒体视图**（激活态，仅显示视频/音乐文件，目录保留可继续导航））；右下
  `FileListView` 列表视图（flex 行布局，**Checkbox 整格命中**切换勾选不触发行操作，行 hover 基础变色，
  文件按类型显示**不同图标与柔和颜色**（`fileType.ts`：视频蓝/音频紫/图片绿/压缩包琥珀/文档灰/代码青绿）；
  **列宽可拖拽**调整，localStorage `filesnew.colWidths` 持久化）。
- **剪贴板**：剪切/拷贝为前端内存态（`clipboard`，刷新即失），「粘贴」调 `/api/fs2/copy|move` 入
  **jobs 后台任务**（复用 JobsIndicator 展示进度）；删除为永久删除 + 强确认弹窗（输入「永久删除」）；
  pin 的固定/取消**仅通过工具栏按钮**进行（无 X 号），操作后即时刷新左侧面板。
- **v1 不含**上传/下载/新建文件夹；仅列表视图（无网格视图）。路径状态存 URL（`?path=`），刷新/深链可恢复。
- 目录轮询 `refetchInterval: 5000` 以感知后台复制/移动完成后的内容变化（同旧文件页 busy 轮询）。

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
- HLS 经 `useMediaProvider` 设 `provider.library = () => import('hls.js')` 指向本地包（默认 jsdelivr CDN，
  LAN 离线失败），并放宽 `provider.config.manifestLoadingTimeOut`（默认 10s 首播转码期超时→无限转圈）。
- **缓冲收紧**：`maxBufferLength/maxMaxBufferLength=10`、`maxBufferSize=0`、`backBufferLength=30`
  （增量转码期 playlist 无 ENDLIST，hls.js 按 live 追 live edge，否则会抢先下载大量分片——2026-09 修复）。
- 样式 import `@vidstack/react/player/styles/{base.css,default/theme.css,default/layouts/video.css}`，
  `DefaultVideoLayout` 从 `@vidstack/react/player/layouts/default` 导入。
- **`/api/stream/{id}` 无扩展名，`MediaPlayer` 的 `src` 必须显式带 `type`**（`VideoSrc | HLSSrc`），否则
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
- **重封页签（2026-09 新增）**：图标 `Wrench`，root `/remux`，管理分段 MP4 的手动重封（见 [media.md](media.md) §3）。
- **后台任务按钮（JobsIndicator）**：header 右侧、退出登录旁。双鱼箭头图标（`RefreshCw`）在有长时任务
  时 `animate-spin` 并显示数量徽标；点击弹出任务面板（`features/jobs/JobsIndicator.tsx`，轮询
  `GET /api/jobs`，进行中 1s / 空闲 15s）。面板为 `absolute` 覆盖自身区域，**不因点击外部消失**，仅
  再次点击按钮收起。任务行展示名称 + 进度：`progress>=0` 为确定进度条，否则 `.indeterminate-bar`
  动态滑条（`index.css`）。只显示 `internal=false` 的长时任务（扫描/重封）。

## 6. Explorer 文件页

- 存储卷改可折叠侧边栏（宽屏默认收成窄图标轨 `w-14`，展开为管理面板；窄屏为顶部横向 chips 条）。
- 宽屏（≥lg）Finder 多列浏览（`ColumnBrowser`，每级目录一列，点文件夹右侧开新列、点列头回退），
  窄屏回退单列表（`FileList`）。

## 7. 卡片 / 网格

- `VideoCard` 横版 16:9（row-span-2）、`MediaGrid` dense 混排仅用于首页/搜索。
- 库内列表不再用卡片网格（`LibraryGrid`/`SeriesCard` 已删除）。
- 浏览栏系列封面显示**总时长徽标**（`/api/series` 返回 `total_duration`，成员时长合计）。
- 系列类型文案统一为「**系列剧集**」/「电影部」（非「剧集季」）。
- 浏览栏**悬浮卡片**：每行一张 `border-neutral-200 bg-white rounded-lg shadow-sm` 卡片，行间 `gap-1.5`
  留白；hover `-translate-y-0.5` 上浮 + `border-blue-400/60` 描边变蓝 + `bg-blue-50/40` + `shadow-md`，
  `transition-all duration-200`；选中 `border-blue-500 bg-blue-50 ring-1 ring-blue-500/30`；封面无缩放动画。
