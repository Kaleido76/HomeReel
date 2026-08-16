# frontend.md — 前端架构与约定

> 改动 `frontend/src/{tabs,features}` 前必读。技术栈：React 19 + TypeScript、Vite、Tailwind CSS 4、
> TanStack Router / Query、Vidstack（Direct/Remux Range，Transcode HLS + 本地 hls.js）；**前端命令一律用 `pnpm`**。
> UI 视觉规范（配色 / 间距 / 组件风格）见根目录 `UI.md`。

## 1. 多 Router 页签（keep-alive）

- 顶部 5 个页签（首页/视频库/搜索/工具/文件），每页签一个独立 TanStack Router 实例
  （`createMemoryHistory`，`tabs/`），组件树常驻不卸载（`display:none` 隐藏）＝状态永不丢
  （滚动/筛选/播放器状态切页签不丢）。
- 页面组件 `React.lazy` 分包懒加载；ARIA tablist 语义 + 方向键切换。
- **URL 唯一来源是活动页签**：`TabManager.activate` 切换页签用 `replaceState`（不产生历史），活动页签内
  导航由 `TabSync` 镜像 `pushState`；`popstate` 反解析 URL→页签并 `navigate({href, replace})` 对齐该页签
  memory history。
- 跨页签跳转走 `openVideo/openSeries/openLibrary`（切到 library 页签再 navigate）与
  `openFormat/openFormatVideo/openFileLocation`（分别切到 工具/文件 页签再 navigate）。
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
   条目）、**标记为系列**（所选文件夹整体打包成系列；未选文件夹且当前目录含视频时**标记当前目录**）、
   多媒体视图（激活蓝），文字说明经共享 `Tooltip` 组件（`src/components/Tooltip.tsx`）悬停即显——深色气泡、
   淡入淡出 + 轻微位移动画、无原生 title 延迟，四向 `placement` 自动翻转防溢出（见 §4）；右端状态
   文本显示项数与勾选数。排序控件不在工具栏。
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
  [backend.md](backend.md) §4。
- **详情页按需同步（2026-08 管理面定稿）**：
  - 单集详情（`VideoDetailPane`）：详情响应带 `source_status`（ok | moved | missing）。moved →
    黄色警告「源文件已改名或移动」+「同步」按钮（`POST /api/videos/{id}/sync`）；missing → 红色警告
    「源文件不存在」+「移除这个单集」按钮（`DELETE /api/videos/{id}`，仅删元数据、不动文件）。
  - 系列详情（`SeriesDetailPage`）：详情响应带 `check`（根目录存在性 + 成员存在性 + 未入库新文件）。
    `out_of_sync` 时顶部黄色警告列出差异 +「同步」按钮（`POST /api/series/{id}/sync`）。
    元数据卡片（2026-09，显示名修订）：系列**显示名**单行省略号截断（超长 hover 显示全名）；显示名
    **可内联编辑**（铅笔 → 输入框 + 保存/取消，Enter/Esc，复用 `PATCH /api/shows/{show_id}` 的 `name`
    字段——系列与 show 1:1，改名即系列改名并重建成员搜索索引；ADR-015 修订：显示名默认=文件夹名，
    创建后与文件夹脱钩，扫描/标记/同步不覆盖）；根目录路径（`root_path`，等宽字）可点击跳
     「文件」页签并定位系列根目录（`manager.openFileLocation(root_path)`）。
     关联系列卡（2026-09）：已关联列表为**垂直列表式**——每项一行「名称（截断，点击跳转）+ 移除 ×」，响应式
     网格多列（`grid-cols-1 sm:grid-cols-2 xl:grid-cols-3`：最窄单列、宽屏系列列内最多三列，窄屏单列堆叠更
     友好）；新增经「**管理关联**」按钮打开
     `SeriesPickerModal`（**多选**，排除自身，**已关联系列默认勾选**）——确定时 `PUT /api/series/{id}/links`
     提交**全量勾选集**（方案 B 分组模型）：该系列与勾选系列同组互相可见，取消勾选即不再关联；移除 ×
     走 `DELETE /api/series/{id}/links/{linkedId}`（双向生效）。
  - **系列观看进度（2026-09，`SeriesProgressCard`）**：合并卡「播放历史」内的「观看进度」小节聚合全部成员续播
    位置——已观看 X / N 集（观看超 90% 即算看完，片尾报幕不误判）+ 整体进度条（已看时长/总时长）；图标清除
    按钮（文字在 ToolTip）调 `DELETE /api/series/{id}/history` 一次清空该系列所有成员历史，成功后失效
    `['series', id]` 与 `['home']`。`VideoPlayer` 退出保存进度时若视频属某系列（`show_id` 非空）会额外失效
    `['series']`，使进度卡/成员行即时刷新。
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
  **单集详情永远带 series 上下文（2026-09）**：视频属于某系列但以独立详情打开时（首页/搜索卡片跳入、
  深链 `/library/video/:id`），`VideoDetailPane` 自动 `replace` 重定向到 `/series/:id/video/:videoId` 补齐
  series 段；返回系列的入口统一为系列详情列顶部「返回系列」条，**单集详情不再有「所属系列」跳转链接**。
- **过滤器是浏览栏的一部分**（`全部/单集/系列` tab + 搜索框 + 高级筛选按钮，位于浏览栏顶部，随浏览栏一起被挤出屏幕；
  窄屏仅浏览视图显示）。
- **高级筛选面板**（`AdvancedFilter.tsx`，替换了原排序下拉）：JobsIndicator 同款开关——首次点击展开、再次点击
   **应用并收起**；内含 标签（多选 chips，来自 `/api/tags`）/ 类型 / 年份 过滤器 + 排序下拉；面板保留
  本地草稿，应用前不生效；有激活筛选时按钮显示蓝色徽标计数，面板内「重置」仅清空草稿。
  `tags` 在 URL 中逗号连接存储，`parseGridSearch` 还原为数组。
- 系列详情成员行有**播放/续播 + 详情**两按钮：播放直达播放栏（左侧保留系列详情，不再插入单集详情）；
  详情打开单集详情栏（顶部有「返回系列」条）。
- **系列成员列表（2026-09，`SeriesMemberList`）**：头部一行 = 左侧标题「剧集列表 N 集」+ 右侧**可展开的
   按钮组容器**（同一 `rounded-md border p-1` 容器，右对齐）：平时只显示**六个点**的拖拽把手按钮
   （`GripVertical`，hover 提示「编辑模式」），点击后进入**编辑模式**并在容器内展开显示 4 个图标按钮——
   **「按当前显示名称排序」**（`ArrowUpNarrowWide`，前端按 `episode_title || title` 排序后走
   `POST /api/series/{id}/order`，手动改过的标题生效）、**「恢复文件名序」**（`ArrowDownAZ`，
   `POST /api/series/{id}/resort`，手动改过的标题保留）、「批量修改显示名称」（`Pencil`，打开
   `SeriesRenameModal`）、「退出编辑模式」（`X`）。每行 = 左侧**独立编号槽**（右侧有分隔
   竖线，视觉隔离）+ 内容（只显示单集显示名，**不显示源文件名** + 续播进度条 + 时长 + 播放/详情按钮），
   padding 较大（`px-4 py-3.5`）；行有 `border` + 白底 + `rounded-md` 及 `space-y-1.5` 留白，视觉上独立成行。
   **播放/详情按钮为统一「灰框描边按钮」风格（2026-09）**：中性灰描边 + 白底 + 灰字，hover 时浅灰填充
   （低饱和 outline 风格）；播放为 `Play` 图标（续播/播放）、详情为 `Info` 图标，**图标 + 文字**，窄屏（`<sm`）
   时文字 `hidden` 仅留图标以省横向空间。
   **编辑模式**（`sortMode`）：**关闭**（默认）→ 行不可拖拽、文字可选中复制；**开启** → 列表整体
   `select-none` 防文本选择，按住行拖动超过 6px 进入拖拽——**原行留在原位变半透明**，**列表内 `absolute`
   定位一个与行同宽的 ghost（无编号，垂直居中跟随鼠标；因宽屏栏位栈有 transform，不用 `fixed` 以免坐标系
   偏移）**，因此不会遮挡/误判对其它行的 hover；目标位置实时高亮：**落到某行中间带 → 与该行交换**（整行蓝圈
   高亮），**落到两行之间的上下边缘带 → 插入两行之间**（蓝色插入线）；松手后 `POST /api/series/{id}/order`
   持久化并失效系列缓存。编号槽（`data-no-drag`）与按钮/链接**不是拖拽把手**。
- **系列选择器（2026-09，`SeriesPickerModal`，`src/components/`）**：系统级复用模态框，用于「任意时刻选择
  库中系列」的场景，交互类似 Windows 文件选择器——顶部**筛选输入框**（不区分大小写子串匹配系列名）+ 中部
  **可滚动系列列表**（每行 = 选择指示器 + 名称 + 成员数）+ 底部确认栏（已选计数 + 取消/确定，无选择时确定
  禁用）。**单/多选由调用方决定**：`multiple` 为 true 渲染复选方框可逐项勾选，为 false 渲染单选圆点、点选
  即替换之前选择（交互上保证单选）；`initialSelectedIds` 在打开时**默认勾选**指定系列（用于管理类弹窗）；
  确定后 `onConfirm(selected: Series[])` 把选中系列（id/name 等）交给调用方，模态框自关闭。`excludeIds`
  排除不可选系列（如当前系列本身）。数据经 `['series']` queryKey 复用列表缓存，不重复拉取。
- **批量修改显示名称（2026-09，`SeriesRenameModal`）**：系列详情编辑模式下的模态框（复用共享 `Modal`，
  `lg` 带标题栏），界面与文件页签批量重命名一致（查找/替换 + 无视大小写 + 正则开关 + 原/新两列预览），
  但它修改的是**单集显示名**而非文件名：对每个变化的成员 `PATCH /api/videos/{id}` 的 `title`
  （`title_source` 变 `manual`，扫描/同步**不再覆盖**，见 backend.md 覆盖规则；后端 `UpdateMetadata` 同步
  `episode_title` 使成员列表即时显示新名）。
- **替换逻辑（仅系列剧集）**：从系列内单集详情点播放时，单集详情栏被替换成播放栏
  （`/series/:id/video/:v` → `/series/:id/play/:v`）。单集从浏览栏打开的行为不变
  （`[浏览][单集详情]` → 播放 → `[单集详情][播放]`）。
- **导航目标由栏位栈父栏决定**（`LibraryLayout` 依栈计算 `playHref`/`exitHref`/`goHref` 传入
  `VideoDetailPane`/`PlayerPane`，**不依赖视频的 `series_id` 属性**）：单集详情栏在系列内 → 播放跳
  `/series/:id/play/:videoId`，在浏览栏上 → `/library/video/:id/play`；播放栏退出 → 父栏路径，上下集/
  接下来播放 → 系列上下文走 `/series/:id/play/:v`、否则 `/library/video/:v/play`。
- **单集详情页头 + 播放历史**（`VideoDetailPane`，2026-09）：详情页顶部为**标题式页头**「详情」
  （`text-xl font-semibold`，无图标，右侧为播放按钮，无法在线播放时改由下方格式工厂引导）；主体卡片顺序为
  **元信息（含单集名）→ 播放历史 → 技术信息**，源状态警告（moved/missing/无法在线）为顶部的条件告警条。
  「**播放历史**」为**合并卡**（`PlaybackHistoryCard` 外壳 + 两个小节）：小节一「观看进度」
  （`PlaybackProgressSection`，自带 `['history',id]` 查询与清除 mutation）——有记录时展示上次播放进度条
  （上次播放到 X / 总时长 · 日期，蓝色进度条），无记录时显示「尚未播放过。」；小节二「配置缓存」
  （`PlaybackPrefsCard`）。两个小节的清除按钮均为**纯图标**（文字在 ToolTip），靠按钮所在小节区分清除对象：
  进度小节清空续播位置（`DELETE /api/videos/{id}/history`，无记录时禁用，清除成功后 `setQueryData` 即时切为
  「尚未播放过。」）、配置小节清除播放选择记忆。`VideoPlayer` 退出播放时保存进度后 invalidate `['history', id]`
  查询，详情页返回后进度即时刷新。
  元信息卡最末的**文件路径**（`font-mono` 相对路径）可点击——hover 变蓝加下划线，经 `manager.openFileLocation`
  切到「文件」页签并定位到该文件的**所在目录**（`parentPath(video.path)`）。
- **单集详情技术卡片**（`VideoTechCard`，2026-08 / 2026-09 扩展）：详情栏内专门卡片集中展示 probe 技术信息并
  解释本片在前端如何播放——顶部**播放方式徽标**（直接播放 / 重封装播放 / 转码播放 / 无法在线播放，基于
  `playability.playMode` 与后端三能力标志）；**技术信息**（容器 + MIME、视频编码/分辨率/帧率、音频摘要、
  时长/大小、segmented、faststart）；**逐流播放处理**（容器/视频/音频在所选播放层下分别如何处理，全部为
  自然语言键值对 + 语义色，如「本机浏览器不支持此容器，后端重封装为 MP4 后播放」「重编码为 H.264」，基于
   `playability.reportFor` 与 `mode`，与 `playMode` 同一判定/缓存）——**「有损/无损」以彩色徽标呈现**
  （`Handling.lossless` 布尔：无损=emerald 徽标、有损=amber 徽标，不做质量转换的层不显示），不再以括号字符串
  表达；「不支持前端解码」不再是死路，各层会接手，故不再展示旧式「XX 不支持前端解码」警告与原始探测串。
  **排版（2026-09）**：卡片为 `@container`，正文统一 `text-sm`，仅用颜色/字重区分层级（小节标题最深、
  键最浅、值居中）；键值对在容器窄于 `@sm`（24rem）时**纵向堆叠**（键在上靠左、值在下靠右），避免移动端
  单行显示不全被省略。字幕说明用大白话：外部字幕文件直接显示 / 内封字幕播放时自动提取显示 / 内封位图字幕
  仅可经格式工厂烧录。
  **音频轨道清单**（`['audio', id]` 查询缓存，与播放器共用）：每轨 codec/声道/语言 + 在所选层下如何处理 +
  有损/无损徽标，默认轨标注。**字幕轨道清单**（`['subtitles', id]` 缓存共享）：侧边文件直接提供、内封文本轨
  提取 WebVTT、位图字幕（PGS/VobSub，`playable=false`）标注「仅可格式工厂烧录」。
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
- **三层动态流（2026-08 修订，ADR-006）**：可播放性在**进入播放器之前**由 `lib/playability.ts` 的
  `playMode()` 决定——probe 元数据（容器/视频编码/音频编码/segmented）→ MIME + codecs 串 →
  `HTMLMediaElement.canPlayType()`，结合后端 `direct_playable / remux_playable / transcode_playable`
  返回 `'direct' | 'remux' | 'transcode' | 'none'`。`VideoPlayer` 按模式取 src：
  - direct → `/api/stream/{id}`（Range MP4）
  - remux → `/api/stream/{id}/remux`（缓存 MP4，Range）
  - transcode → `/api/stream/{id}/hls/playlist.m3u8?session=<uuid>`（`type: 'application/x-mpegurl'`）
  - none → 播放按钮禁用，详情/播放栏显示「格式工厂转换」引导
- **hls.js 本地注入（不走 CDN）**：transcode 模式在 `MediaPlayer` 的 `onProviderChange` 里
  `isHLSProvider(provider) && (provider.library = Hls)`。session 每视频 `crypto.randomUUID()` 生成，
  播放列表内嵌携带 `?session=` 的绝对分片 URL（hls.js 相对解析会丢 query）。Direct/Remux 不需 hls.js。
- **多音轨菜单（2026-09）**：`VideoPlayer` 经 `fetchVideoAudioTracks` 拉 `GET /api/videos/{id}/audio`，多轨
  （且非 Direct 层）时把「音轨」`DefaultMenuSection` + `DefaultMenuRadioGroup` 注入**播放器自带「设置」菜单**
  （`DefaultVideoLayout slots.settingsMenuItemsEnd`），与倍速/画质/字幕同一 UI。选轨：
  - remux → `src=remuxUrl(id, N)`（`?audio=N`，命中各自缓存）；
  - transcode → **新 session token** + `&audio=N`（会话创建时固化音轨号，换轨必须换会话）；
  - 选轨后 `ensurePendingSeek` 在 `onCanPlay`/`onLoadedMetadata`/`onTimeUpdate` 反复 seek 到 `posRef` 记录
    的进度，直到真正追上（HLS 新流未就绪时单次 seek 会被丢弃，故自愈重试），追上后若切换前在播放则自动
    `remote.play()` 续播。
  Direct 层不注入菜单（浏览器默认轨）。
- **播放选择记忆（2026-09，ADR-006 player prefs；系列共享 2026-09 修订）**：缓存「上次手动选择的音轨/字幕/
  音量」，播放时自动应用、**仅用户手动切换时更新**（`PUT /api/videos/{id}/prefs` 部分更新）。属于可重建缓存，
  由工具页「缓存管理」删除（`DELETE /api/cache/prefs…`）或详情页「清除缓存」删除，删除后回默认轨/默认字幕/
  默认音量，与续播 `history` 完全分离。**系列剧集共享同一记忆**：`GET /api/videos/{id}/prefs` 返回**有效**
  记录（`scope: series | video` + 归属 `series_id`）。`series` 记录存音轨/字幕的**名称**（label），前端按当前
  集的实际轨道清单解析；`video` 记录存具体值（音轨序号 / `subtitle_id`）。系列有记录时忽略单集记录；单集记录
  是关系重组后的兜底。
  - **自动应用**（`applyPrefs`，按媒体流经 `appliedPrefsRef` 每字段至多一次）：音轨仅在动态层（remux/
    transcode）生效——`series` 按 `audio_track_name` 在当前集音轨清单里 `findIndex(label)`、`video` 按序号，
    找到才 `setAudio`（换轨即重建流）；字幕按名称/`subtitle_id` 匹配 `<Track>` 的 `id`
    （`sidecar`=侧边文件 / `e<N>`=内封文本轨流序号，空串=关闭字幕）经 `remote.changeTextTrackMode` 应用；
    音量经 `remote.changeVolume`/`mute`/`unmute`（两层共用）。音量是播放器级属性、跨 src 变化保持，故只应用
    一次；音轨/字幕随 src 变化（选轨重载）重新应用以保持选择。`playerReadyRef`（`onLoadedMetadata` 置位）保证
    provider 就绪后才下发音量/字幕请求，避免请求被丢弃而标记已应用。系列级名称匹配不到当前集轨道时该字段不
    应用（回默认）。
  - **仅手动刷新**：`applyingPrefsRef` 在自动应用期间置位，抑制一切保存。保存触发点——音轨 = 菜单
    `onTrackSelect`（系列成员发 `audio_track_name`、独立单集发 `audio_track`）；字幕 = 监听
    `media-text-track-change-request`（内置字幕菜单/字幕按钮的唯一出口，含「关闭字幕」→ 空串），按
    `detail.index` 反查 `textTracks` 取回 `id` 再反查 label（系列成员发 `subtitle_name`）；音量 =
    `onVolumeChange`（1s 节流 + 退出时若用户调过则补存一次）。`savePrefs` 成功后 invalidate `['prefs', id]`
    （及系列 `['series-prefs', seriesId]`）重取，同一页面会话内再次进入播放立即应用新选择。
  - **详情页展示与清除（2026-09）**：`PlaybackPrefsCard`（`features/player/PlaybackPrefsCard.tsx`）是合并卡
    「播放历史」内的「配置缓存」小节——三行信息（音轨/字幕/音量）+ 纯图标清除按钮（文字在 ToolTip）。
    系列详情页经 `GET /api/series/{id}/prefs` 展示共享记录（注「整部系列共享」），清除删系列记录
    （`DELETE /api/series/{id}/prefs`）；单集详情页展示**有效**记录（`series` 时注「来自系列」并清除系列记录、
    `video` 时清除自身记录），清除后回退到兜底记录/默认值。
  - **已知边界**：Direct 层无法选音轨（浏览器默认轨），故音轨记忆只在 remux/transcode 生效；字幕与音量
    全层生效。
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
- **按钮 hover 手指光标（2026-09）**：Tailwind v4 预置样式不再给 `button` 加 `cursor:pointer`（v3 曾内置），
  `index.css` 在 `@layer base` 恢复 `button:not(:disabled){cursor:pointer}`（禁用态保持默认光标）。
- 容器宽度自适应：页签宿主 `max-w-[1600px]`、文件页 `max-w-[1920px]`，大屏高密度。
- `TabHost` 对 library 页签**全宽无 max-w/无 padding**（vw 单位布局，留白会破坏面板感）；文件页仍
  `max-w-[1920px]`。
- `useMediaQuery`（`lib/useMediaQuery.ts`）选择窄屏/宽屏布局。
- **播放器/系列详情**：`PlayerPane`/`SeriesDetailPage` 只渲染内容（返回由三栏动画 / 窄屏 `NarrowBack` 承担）。
- **共享 Modal 组件（2026-09，`src/components/Modal.tsx`）**：全系统统一模态框外壳——暗色遮罩
  （`fixed inset-0 z-50 bg-black/40 p-4`）+ 居中白色圆角面板 + 可选标题栏（图标 + 标题 + 关闭按钮）。
  内容完全由调用方提供，仅内容不同；`size`（`sm`/`md`/`lg`）控制宽度，面板无内边距（由内容自持）。
  关闭语义归调用方（`onClose`），Escape/点遮罩关闭不做内置。现有消费者：文件页签永久删除强确认
  （`ConfirmDelete`，`sm`，无标题栏）、系列详情批量修改显示名（`SeriesRenameModal`，`lg`，带标题栏）。
   新增模态框一律复用此组件，禁止再手写遮罩。
- **共享 Tooltip 组件（2026-09，`src/components/Tooltip.tsx`）**：全系统统一悬停提示——深色气泡
  （`bg-neutral-900 text-neutral-50 border-neutral-700 shadow-lg`，无箭头）、即时显示（无原生 title 延迟）、
  淡入淡出 + 轻微位移（150ms，`prefers-reduced-motion` 自动抑制）。定位四向 `placement`
  （`top`/`bottom`/`left`/`right`，默认 `top`），空间不足自动翻到对侧并钳制在视口内；气泡经 portal 渲染、
  先量尺寸再定位防首帧跳动；`content` 为空/`undefined` 时不渲染气泡。**用法**：`<Tooltip content="…">` 包裹
  任意触发元素（按钮/图标/文本均可），属性名用 `content`（旧 `tip` 已废弃）。**迁移约定**：按钮/图标等「动作
  说明」类提示一律用此组件；文本截断全文提示（`title` 展示完整标题/路径）、拖拽把手、禁用 `<select>` 说明仍
  保留原生 `title`。新增任何悬停提示一律复用此组件，禁止再写原生 `title` 或私有气泡。


## 5. header 与页签 UI 风格（2026-08 重设计）

- 两侧顶格（无 `max-w` 居中约束，`px-3 sm:px-6` 响应式内边距）。
- 页签为独立圆角胶囊按钮（`rounded-md px-3 py-1.5`），激活态 `bg-neutral-900 text-white` 深色胶囊、
  非激活 `text-neutral-600 hover:bg-neutral-100`，**不使用蓝色下划线**。
- 页签文字标签 `<sm` 断点隐藏仅留图标（`hidden sm:inline`），窄屏页签列表 `overflow-x-auto`。
- Logo 与「退出登录」保持原样。
- **工具页签（2026-09，替代原「重封」/「格式工厂」独立页签）**：图标 `Wrench`，root `/tools`。布局与文件页签
  类似：**左侧垂直工具栏 + 右侧工具面板**（`features/tools/ToolsPage.tsx`，`?tool=id` 存于 URL；工具注册表
   `features/tools/tools.ts`，访问过的工具面板保持挂载，切换不丢状态）。左栏为**硬朗/方角/等宽风格**：
   按钮无圆角、与栏等宽，激活态 `bg-neutral-900 text-white`（呼应 TabBar）。当前含两个工具——
   **格式工厂**与**缓存管理**（`CacheManagerPage`，见下）。格式工厂：把任意视频/文件夹转为浏览器可播放的
   Faststart MP4 副本（见 [media.md](media.md) §3.1）。
  文件页签工具栏「格式工厂」按钮把勾选（或含可转换文件的当前目录）经 `manager.openFormat` 移交到工具的
  **待转换队列**（模块 store `features/tools/format/queue.ts`，用 `usePending` 订阅；再次移交**替换**当前批次）。
  面板自上而下：
  - **操作面板**（`FormatFactoryPage` 内 `OperationsPanel`）：未选择文件时整体**遮罩禁用**；预设工具
    **快速 MP4 / H.264 / H.265**（`features/tools/format/presets.ts`）仅作**快速填充**（点击填参数），
    下方**参数表单持续存在**（视频编码、清晰度 CRF、音频模式、AAC 码率、烧录字幕），改动后标记
    「已自定义」；`POST /api/convert` 携带当前 `params`。表单字段按上下文**禁用**（无损拷贝→禁用 CRF、
    保留原音→禁用码率、无字幕轨→禁用烧录、检测到非通用音频→禁用「保留原音」）。
  - **检测信息**：`POST /api/convert/probe` 逐个探测所选文件（目录展开为直接一级视频），操作面板顶部
    以引导性文字提示（位图字幕将自动降级、AC3 将转 AAC 等）；**待转换清单每行显示探测徽标**
    （位图字幕 / 文本字幕 / AC3 等），目录行显示汇总（N 个视频、位图字幕 ×N）。
   - **待转换清单**（含目标输出路径）+「开始转换」：可随时继续移交新批次（**整体替换**当前待转换清单），
      即使有任务运行中。
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

## 6. 卡片 / 网格

- `VideoCard` 横版 16:9（row-span-2）、`MediaGrid` dense 混排仅用于首页/搜索。
- 库内列表不再用卡片网格（`LibraryGrid`/`SeriesCard` 已删除）。
- 浏览栏系列封面显示**总时长徽标**（`/api/series` 返回 `total_duration`，成员时长合计）。
- 系列类型文案统一为「**系列剧集**」/「电影部」（非「剧集季」）。
- 浏览栏**悬浮卡片**：每行一张 `border-neutral-200 bg-white rounded-lg shadow-sm` 卡片，行间 `gap-1.5`
  留白；hover `-translate-y-0.5` 上浮 + `border-blue-400/60` 描边变蓝 + `bg-blue-50/40` + `shadow-md`，
  `transition-all duration-200`；选中 `border-blue-500 bg-blue-50 ring-1 ring-blue-500/30`；封面无缩放动画。
