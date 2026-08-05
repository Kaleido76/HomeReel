# frontend.md — 前端架构与约定

> 改动 `frontend/src/{tabs,features}` 前必读。技术栈：React 19 + TypeScript、Vite、Tailwind CSS 4、
> TanStack Router / Query、Vidstack + 本地 hls.js；**前端命令一律用 `pnpm`**。

## 1. 多 Router 页签（keep-alive）

- 顶部 5 个页签（首页/视频库/搜索/文件/重封），每页签一个独立 TanStack Router 实例
  （`createMemoryHistory`，`tabs/`），组件树常驻不卸载（`display:none` 隐藏）＝状态永不丢
  （滚动/筛选/播放器/上传状态切页签不丢）。
- 页面组件 `React.lazy` 分包懒加载；ARIA tablist 语义 + 方向键切换。
- **URL 唯一来源是活动页签**：`TabManager.activate` 切换页签用 `replaceState`（不产生历史），活动页签内
  导航由 `TabSync` 镜像 `pushState`；`popstate` 反解析 URL→页签并 `navigate({href, replace})` 对齐该页签
  memory history。
- 跨页签跳转走 `openVideo/openSeries/openLibrary`（切到 library 页签再 navigate）。
- **新增子路由必须归属到某个页签的 router**（`tabs/routers.ts`），并在 `tabFromPath` 补路径映射，
  禁止添加全局平级路由；改动此类路由结构时勿破坏 keep-alive。
- 深链/刷新按 URL 恢复对应页签视图；库的视图状态（`view/q/sort/page`）与搜索词 `q` 存在 URL 参数中。

## 2. Library 栏位栈布局（宽屏 ≥lg）

`LibraryLayout` 是 library 页签 root 的唯一编排者——自行解析 `location.pathname` 还原成**栏位栈**
（`parseStack`，`LibraryLayout.tsx`），子路由 component 均为空（URL 仅供匹配/深链，**不渲染 `<Outlet/>`**）。

- **栏位栈**：浏览永远是栈底，选择视频/系列 push 详情栏、播放 push 播放栏；每栏占 `50vw`，视口一次只见
  **栈顶两栏**（位移 `translateX = -(栈深-2)*50vw`，栈深≤2 为 0），更早的栏位被挤到屏左外
  （keep-alive 仍在 DOM，选中高亮随栈第 2 栏保留）。
- **URL 即栈**：`/library`=`[浏览]`、`/library/video/:id`=`[浏览][单集详情]`、
  `/library/video/:id/play`=`[浏览][单集详情][播放]`、`/series/:id`=`[浏览][系列详情]`、
  `/series/:id/video/:videoId`=`[浏览][系列详情][单集详情]`、`/series/:id/play/:videoId`=`[浏览][系列详情][播放]`。
- **过滤器是浏览栏的一部分**（`全部/单集/系列` tab + 搜索 + 排序，位于浏览栏顶部，随浏览栏一起被挤出屏幕；
  窄屏仅浏览视图显示）。
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
- 网格筛选状态（`view/q/sort/page`）存于 `LibraryLayout` 组件 state（`features/library/types.ts`），
  仅 `/library` 页写回 URL。子页面组件（`VideoDetailPane`/`PlayerPane`/`SeriesDetailPage`）以 props 接收
  id，不用 `useParams`。
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
- 过滤器排序下拉在「系列」视图**禁用而非隐藏**（高度一致）。
