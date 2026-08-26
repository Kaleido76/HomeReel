# frontend.md — 前端架构与约定

> 改动 `frontend/src/{tabs,features}` 前必读。技术栈与 pnpm 要求见 AGENTS.md §1；
> UI 设计语言见 [UI.md](../UI.md)；实时通道见 [realtime.md](realtime.md)。

## 1. 多 Router 页签（keep-alive）

- 顶部 5 个页签（首页/视频库/搜索/工具/文件），每页签一个独立 TanStack Router 实例
  （`createMemoryHistory`，`tabs/`），组件树常驻不卸载（`display:none` 隐藏）＝状态永不丢。
  页面组件 `React.lazy` 分包懒加载；ARIA tablist + 方向键切换。
- **URL 唯一来源是活动页签**：切页签 `replaceState`，页签内导航由 `TabSync` 镜像 `pushState`；
  `popstate` 反解析 URL → 页签并 `navigate({replace})` 对齐该页签 memory history。
  深链/刷新按 URL 恢复对应页签视图。
- 跨页签跳转走 `manager` 的 openVideo/openSeries/openLibrary/openFormat/openFileLocation
  （先切页签再 navigate）。
- **新增子路由必须归属某页签的 router 并在 `tabFromPath` 补映射**，禁止全局平级路由；勿破坏 keep-alive。

## 1.1 文件页签

机器级文件浏览器（HTTP 文件服务器模型）：每次进入目录后端实时读取，无索引/watcher；
本地盘枚举排除网络盘。布局 = 左侧三段侧栏（盘符/pin/多媒体源，可拖动分隔条）+ 右上两行工具栏
（面包屑 + 图标按钮组带 Tooltip）+ 列表。关键行为：

- 无「..」行——回上级统一用工具栏向上按钮；checkbox 整格命中切换勾选不触发行操作，
  checkbox 列支持拖选批量勾选/取消；列宽 localStorage 持久化；点列标题排序（活动列翻转升降序）。
- 剪贴板为前端内存态 + 底部通用抽屉外壳 `ToolDrawerShell`（可复用）；粘贴调 copy/move 入 jobs
  后台任务；剪切粘贴成功清空抽屉、复制粘贴保留。
- 批量重命名 `RenameDrawer`：查找/替换预览（大小写/正则开关），关闭即撤销不执行，
  提交走同步接口 `/api/files/renames`。
- 多媒体源：标记 / 取消标记经强确认（整源从库移除）/ 重扫；扫描完成监听
  `events.jobs.done|failed(type=scan_source)` 即时失效 `['files-sources']`（ADR-021）；
  目录列表仍 5s 轮询兜底。
- 标记为系列：所选文件夹（未选文件夹且当前目录含视频时为当前目录）入队 mark_resource；
  约束见 backend.md §4。「标记为单集」等离散资源概念已彻底清除。
- 详情页按需同步：单集 `source_status`（moved→「同步」按钮；missing→「移除」按钮）；
  系列 `check`（out_of_sync→差异警告+「同步」）。系列显示名可内联编辑
  （复用 `PATCH /api/shows/{id}` 的 name）；root_path 可点击跳文件页定位。
- v1 不含上传/下载/新建文件夹；仅列表视图；路径状态存 URL `?path=`。

## 2. Library 栏位栈

`LibraryLayout` 是 library 页签 root 的唯一编排者：自行解析 `location.pathname` 还原成**栏位栈**
（子路由 component 为空，不渲染 `<Outlet/>`，URL 仅供匹配/深链）。

- **URL 即栈**：`/library`=浏览、`/library/video/:id`=+[单集详情]、`…/play`=+[播放]、
  `/series/:id`=+[系列详情]、`/series/:id/video/:videoId`、`/series/:id/play/:videoId`。
  **单集详情永远带 series 上下文**：属系列的单集以独立路径打开时自动 replace 重定向补齐 series 段，
  返回入口统一为系列详情顶部「返回系列」条。
- 栈模型：每栏 50vw，视口只显示栈顶两栏（translateX 位移）；更早栏位被挤出屏外但 keep-alive 在 DOM。
  过滤器是浏览栏的一部分（随浏览栏一起被挤出）。窄屏退化为单栏全页（NarrowBack 返回条），复用同批组件。
- **播放器挂 Layout 根部绝对定位覆盖层**（keyed by videoId）：宽屏占右 50vw、窄屏整列。原因——
  若在宽/窄两棵分支里渲染，跨 1024px 断点会卸载重挂导致播放中断、音量丢失。
- 浏览栏：视图选择宽屏三项并排、窄屏折叠单下拉；搜索框即时筛选（匹配范围=卡片标题：
  系列按显示名、单集按 title，不含文件名/路径）；高级筛选仅标签 chips（多选 AND，草稿制 +
  激活徽标计数）；列表排序固定最近添加。网格状态存组件 state，仅浏览页写回 URL。
- 系列详情：成员列表头部编辑模式（把手展开四按钮：按当前显示名排序 / 恢复文件名序 /
  批量修改显示名称 / 退出）。编辑模式支持拖拽重排——落到行中间带=交换、落到行间边缘带=插入，
  松手持久化 `order` 后置 sort_manual（此后扫描只追加新成员）。ghost 用 absolute 不用 fixed
  （宽屏栏位栈有 transform，fixed 坐标系会偏移）。
- 共享组件：`SeriesPickerModal`（系统级单/多选系列模态框，initialSelectedIds 默认勾选）、
  `SeriesRenameModal`（批量改成员显示名，PATCH title → `title_source='manual'`，扫描不再覆盖）。
- 单集详情卡片序：元信息 → 播放历史合并卡（观看进度 + 配置缓存两小节，清除按钮纯图标）→
  技术卡（播放方式徽标 + 技术信息 + 逐流处理自然语言键值对 + 有损/无损徽标 + 音轨/字幕清单，
  与播放器共用同一判定缓存）。**播完存完整时长**（progress==duration 表示已播完，区别于从未播放的 0，
  各处显示 100%、续播从片头、不计入继续观看）。
- 导航目标由栈父栏决定：Layout 依栈计算 playHref/exitHref 注入详情/播放栏，**不依赖视频 series_id 属性**；
  系列内播放替换单集详情栏（`[系列详情][单集详情]` → `[系列详情][播放]`）。
- 播放器切走页签自动暂停；视频卡片任意页签点击跳 library 打开。

## 3. 播放器（Vidstack）

- `@vidstack/react` v1.15+（勿装废弃的 `@vidstack/player`）；样式 import 三件套 +
  `DefaultVideoLayout`。
- **三层动态流**：进入播放器前由 `lib/playability.ts` 的 `playMode()` 决定（判定链见 media.md §4）。
  src 映射：direct → `/api/stream/{id}`；remux → `/api/stream/{id}/remux?audio=N`；
  transcode → `/api/stream/{id}/hls/playlist.m3u8?session=<uuid>`；none → 按钮禁用 + 格式工厂引导。
  **src 必须显式带 `type`**（端点无扩展名，否则回退 HEAD 探测失败报 loader 错误）。
- **hls.js 本地注入不走 CDN**：HLS provider 的 `provider.library = Hls`。
- 多音轨菜单注入播放器自带设置菜单（多轨且非 Direct 层时显示）；选轨重建流后
  `ensurePendingSeek` 反复 seek 回原进度直到追上（HLS 新流未就绪时单次 seek 会被丢弃，自愈重试）。
- **播放选择记忆**（语义见 media.md §6.1）：播放时自动应用、仅用户手动切换时保存。
  - 自动应用每字段至多一次；音量/静音经**受控 props** 应用——Vidstack can-play 会把音量重置为 100%，
    不传 volume prop 记忆会被覆盖。
  - 保存触发点：音轨菜单选择、字幕切换事件（含关闭→空串）、音量变化（拖动防抖 400ms 存最终值，
    卸载兜底补存）；成功后 setQueryData 立即更新本地缓存再 invalidate 兜底。
  - prefs 查询 `refetchOnMount: 'always'` 覆盖全局 staleTime，保证每次进播放读到最新记忆。
  - 已知边界：Direct 层无法选音轨，音轨记忆只在 remux/transcode 生效；字幕与音量全层生效。
- **伪全屏**（ADR-006）：触摸端原生全屏要么不可用（iOS `<div>`）要么接管浏览器盖住自定义控制条/
  字幕，故移动端全屏按钮走纯 CSS 伪全屏（`useFakeFullscreen`），桌面保持原生。
  实现：`.fake-fullscreen` fixed 铺满动态视口盖住应用；竖屏设备+横屏内容加 rotate(90deg)
  强制 CSS 旋转。**前提：播放器祖先不得带 CSS transform**（否则成为 fixed 包含块）——
  当前 overlay 挂 LibraryLayout 根部满足该前提。`PlayerFullscreenButton` 覆盖默认 slot 分发
  移动/桌面行为；若伪全屏期间浏览器仍进了原生全屏则先退出再进入，避免叠加。
- 全屏时钟：原生或伪全屏时右上角显示 HH:mm，30s 刷新，不挡控制条。
- 播放器填满播放栏高度：覆盖 Vidstack 默认 16:9（aspectRatio auto + h-full），底部黑边留给控制条，
  进度条不压 WebVTT 字幕。

## 4. 响应式与共享组件

- 断点：Tailwind 预置 + `3xl(1920px)`；`useMediaQuery` 选择宽/窄布局。TabHost 对 library 页签
  全宽无留白（vw 栏位栈布局），其余页签 max-w 容器高密度。
- 圆角 tokens 全局钳制 0–4px（见 index.css 与 UI.md）；`button:not(:disabled)` 恢复 hover
  手指光标（Tailwind v4 已不带）。
- **共享 Modal**（`components/Modal.tsx`）：全系统统一模态框外壳（遮罩+居中面板+可选标题栏），
  内容由调用方提供，size 控制宽度。新增模态框一律复用，禁止手写遮罩。
- **共享 Tooltip**（`components/Tooltip.tsx`）：即时深色气泡、四向 placement 自动翻转防溢出、portal
  定位防首帧跳动。动作说明类提示一律用它；文本截断全文提示、拖拽把手、禁用 select 说明仍用原生 title。

## 5. header 与工具页签

- header：两侧顶格；页签为圆角胶囊按钮（激活深色胶囊，窄屏隐藏文字仅图标）；右侧 JobsIndicator +
  退出登录。
- 工具页签：工具注册表 `features/tools/tools.ts`，访问过的面板保持挂载，`?tool=id` 存 URL。
  导航响应式：桌面左侧垂直工具栏（激活蓝底），窄屏顶栏下全宽上下文行 + 向下展开抽屉
  （遮罩只盖工具面板区）。现有工具：格式工厂、缓存管理、开发者工具。
- 格式工厂面板：操作面板（预设快速填充 + 参数表单持续存在 + 按探测结果禁用字段）→ 待转换队列
  （文件页移交整体替换批次）→ 转换队列（进行中带进度+ETA，历史分成功失败并标注预设）。
- 缓存管理：字幕缓存按视频分组列出（单删/按视频清空/清空全部）；封面缩略图正常使用中不可删
  （删无收益），仅孤儿清理；孤儿打包统计一键清理。
- 开发者工具（ADR-019）：console 劫持 + devLog 采集（环形缓冲 2000 条，开关 localStorage 持久化）、
  级别/模块过滤、归档到后端供 PC 查看（raw 端点见 decisions.md §2.6）。
- JobsIndicator：realtime 驱动零轮询（初始快照 + `jobs.progress` 就地合并）；确定进度条或
  indeterminate 动画条 + ETA；仅显示 internal=false 任务；面板点击外部不消失。

## 6. 卡片 / 网格

- 库内为悬浮行卡（hover 上浮描边变蓝、选中蓝底 ring），窄屏封面收紧变矮；首页/搜索用 16:9 混排网格。
- 浏览栏系列封面带总时长徽标；系列类型文案统一「系列剧集」/「电影部」。

## 7. 实时通道前端约定

> 协议与后端实现见 realtime.md。前端使用约定：

- `RealtimeClient` 单例（`api/realtime.ts`）：生命周期随登录态由 `RealtimeProvider` 管理
  （登录 connect、登出 disconnect）；自动重连（指数退避+页面可见唤醒）、`request()` RPC、
  `on()` 订阅、`send()` 即发即忘。
- 迁移轮询的推荐姿势：初始一次 REST 快照 + `invalidateOnMessage(client, queryClient, mapping)`
  把推送映射到 queryKey 失效；组件内按需订阅用 `useRealtimeMessage`。
- 迁移进度见 status.md §2（jobs 已迁移；文件页部分迁移）。
