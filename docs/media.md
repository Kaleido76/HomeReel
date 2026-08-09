# media.md — 媒体管线约定

> 改动 `backend/internal/{media,streaming}` 或前端播放器媒体相关代码前必读。
> FFmpeg 安装与环境 PATH 见 [environment.md](environment.md)。

## 1. ffprobe / ffmpeg

- 探测、缩略图、HLS 转码、字幕均依赖 FFmpeg / ffprobe。
- 进程 PATH 找不到时在 `config.yaml` 显式配置 `media.ffmpeg_path` / `media.ffprobe_path`
  （YAML `\\` 转义），改后重启。

## 2. 容器判定

- ffprobe `format_name` 是逗号分隔列表（如 `mov,mp4,m4a,...`）。`media.Probe` 归一化首个 token 入库。
- `DirectPlayable` 按逗号分词匹配，**禁止整串查映射**（否则 mp4/h264 被误判为不可直连而误走 HLS）。
- **`contentType` 优先按文件扩展名**判定（`.mp4/.m4v→video/mp4`、`.mov→video/quicktime` 等），
  demuxer 分词仅作扩展名未识别时的回退——因为 MOV demuxer 覆盖整个 MP4 家族，先按 token 会把真实
  .mp4 误标成 `video/quicktime`，桌面浏览器不认该 MIME 会退化为整文件下载/卡顿（2026-09 修复）。

## 3. 分段 MP4（2026-09）

- hls.js 拼接文件/分片式 MP4 有多个顶层 `mdat` 盒子或 `moof` 分片，Chrome `<video src>` 会整文件
  顺序下载后才播放。
- `media.Probe` 在探测时轻量走顶层 box（读 8/16 字节头逐盒跳转）识别并写入 `videos.segmented`
  （迁移新增列），`mdat>1` 或发现 `moof` 即判真，仅对 mp4/mov 家族扫描。
- **分段文件不回退 HLS**（增量 HLS 期间按 live 处理、长片无进度条，体验差）；而是**保持直连**，
  由「格式工厂」（§3.1）把源文件转成标准 Faststart MP4 副本根治。
- `streaming.Direct` 对分段文件优先服务 `data/remux/<id>.mp4` 遗留缓存（旧「重封」页签的产物，
  现已无管理入口）否则回退原文件；删除/文件变更时 `RemoveCache` 清理 HLS 与 remux 缓存。

## 3.1 格式工厂（2026-09，替代原「重封」页签）

把任意本地视频/文件夹转换为**浏览器可直接播放的 Faststart MP4 副本**（`fservice/convert.go`），
转换后播放器可直连播放、可拖动进度条，不再触发按需 HLS 转码——这是 MKV/HEVC 播放器一直处于
「Live 直播模式」的根治手段。

- **单集**：`Movie.mkv` → 同目录 `Movie.mp4`（同名已存在则 `Movie (1).mp4`，**绝不覆盖源文件**）。
- **系列/文件夹**：`MyShow\` → 同级 `MyShow (MP4)\`，把该文件夹**直接一级视频文件**逐一转换
  （不递归子文件夹；非视频文件忽略；目录无可转换文件则任务报错）。
- **可接受的输入类型**：入口按 `files.IsConvertible` 判定 = 媒体库视频集（`IsVideo`：mp4/mkv/avi/mov/
  wmv/flv/webm/m4v/mpg/mpeg/ts/m2ts/3gp/ogv）**外加** ffmpeg 可读的常见容器（`convertibleExts`：
  **rmvb/rm**、vob、asf、mts/trp/tp、dat、divx、f4v、swf、ogm）。媒体库扫描与「多媒体视图」仍只认
  `IsVideo`（其他逻辑不变）；rmvb 等老格式通常无法无损封装进 mp4（如 RealVideo 流），会走「无损 → 烧录
  重编码」自动降级产出 h264 mp4。文件列表 API 的 `is_convertible` 字段驱动文件页工具栏「格式工厂」按钮可用态。

**操作面板预设（`POST /api/convert` 携带 `params`，前端 `features/tools/format/presets.ts`）**：三个工具
预设，点击即填充参数表单、可再微调（改动后标记「已自定义」）。`fservice.ConvertParams` 字段：
`video`（`copy`|`h264`|`h265`）、`vcrf`（重编码清晰度，copy 忽略）、`audio`（`smart`|`copy`|`aac`）、
`akbps`（AAC 码率）、`burn`（是否烧录首选字幕）。服务端 `norm()` 归一化非法值，空参等价于快速 MP4。

| 预设 | video | vcrf | audio | burn | 行为 |
|---|---|---|---|---|---|
| **快速 MP4** | copy | — | smart | 自动降级 | 下方两级策略 |
| **H.264 MP4** | h264 | 19 | smart | 可勾选 | 重编码 libx264，文本字幕转 mov_text 保留，位图字幕无法承载时 `-sn` 丢弃；勾选烧录则首选字幕烧进画面 |
| **H.265 MP4** | h265 | 23 | smart | 可勾选 | 同上，`libx265`（产物为 HEVC），同等画质更小、需较新设备支持 |

`audio=smart`：仅**全设备通用**的 aac/mp3（`universalMp4Audio` 白名单）走 `-c:a copy`，其余一律 AAC；
`copy` 强制保留原音轨（可能产出 Windows 播放器不认的 AC3）；`aac` 强制按 `akbps` 重编码。
`vcrf`/`akbps` 越界值在 `norm()` 中被钳制到安全区间。

**两级策略（快速 MP4，ffprobe 探测流 → 自动选择）**：
1. **无损流拷贝（首选）**：`-map 0 -c copy -c:s mov_text -f mp4 -movflags +faststart`——视频/音频帧
   **逐比特拷贝**，码率/画质/所有音轨不变；文本字幕转 mov_text 保留。唯一例外：音频仅当编码是
   **全设备通用**的 aac/mp3（`universalMp4Audio` 白名单）才拷贝，其余（**AC3/EAC3**、dts、vorbis、opus、
   flac、truehd、pcm…）一律转 AAC 192k——AC3 等 Chrome 能解但 Windows 播放器报「不支持的 AC3 编码」，
   拷贝会产出「换个设备就无声」的文件。视频永远无损。
2. **烧录字幕重编码（自动降级）**：无损拷贝失败（典型：内封 **PGS/VobSub 位图字幕**——mp4 无法承载、
   而选择 MKV 往往正是因为它们）时，自动改为高质量重编码：视频 `libx264 -crf 19 -pix_fmt yuv420p`
   （画质接近原片）、音频尽量 `-c:a copy`（仅通用编码，否则 AAC 192k）、把**首选字幕轨烧录进画面**
   （`-vf subtitles='C\:/…':si=0`，需 ffmpeg 带 libass；`si` 是字幕流序号，首个字幕恒为 0）。
   结果字幕以「始终显示、不可关闭」的形式保留，视频为有损重编码。仍失败则该文件报错跳过（不做其它兜底）。

**细节陷阱**：输出为 `*.tmp` 无扩展名，**必须显式 `-f mp4`**（否则无法推断 muxer）；字幕滤镜路径用
正斜杠 + `\:` 转义盘符冒号（`escapeFilterPath`）；烧录路径只映射首选音轨（`-map 0:a?`）。

- 后台任务：`jobs` 的 `convert` 类型（`fservice` 包）。**进度与剩余时间**：解析 ffmpeg `-progress pipe:1` 的
  `out_time_us` 与 `total_size`，按优先级取进度——① **`out_time/ffprobe 真实时长`（精确，主路径）**；
  ② 时长缺失且是流拷贝时用 **`total_size/源大小`**（产物≈源大小，1:1 无任意系数）；③ 最后才用按文件大小估算的
  时长（假设约 8 Mbps 平均码率）兜底。剩余时间由 `jobs.Reporter` 按「进度 × 已耗时」推算写入
  `job.eta_seconds`，前端任务面板与格式工厂面板以「预计还需 X」展示（扫描/复制/移动等确定进度任务同样受益）。
- 前端：文件页签工具栏「格式工厂」按钮把勾选（或含视频的当前目录）移交到「工具」页签的格式工厂工具
  （`features/tools/format/FormatFactoryPage.tsx`）。面板自上而下：**操作面板**（预设工具 + 可微调参数表单）、
  **待转换队列**（勾选的文件/文件夹 + 「开始转换」，可随时继续追加批次入队）、**转换队列**（所有 convert
  任务：进行中在前带进度条与 ETA，历史在后分成功/失败，每行标注所用预设）。
- **探测信息（`POST /api/convert/probe`，`fservice/convert_probe.go`）**：为所选文件逐个 ffprobe
  （`probeStreams` 复用转换引擎的探测），目录展开为直接一级视频，返回 `video_codec` / `audio_codecs` /
  `subtitle_codecs` / `duration` / `has_bitmap_subtitle`（`bitmapSubtitleCodecs`：PGS/VobSub/DVB 等位图字幕）。
  前端据此在操作面板给出**检测信息**与禁用规则：含位图字幕 → 提示「快速 MP4 将自动降级为烧录」；检测到
  非通用音频（AC3 等）→ **禁用「保留原音」选项**（否则产出 Windows 播放器无声文件）、提示将自动转 AAC；
  均无字幕轨 → 禁用烧录字幕复选框；无损流拷贝 → 禁用清晰度 CRF；保留原音 → 禁用音频码率。待转换清单每行
  展示该文件的探测徽标（位图字幕/文本字幕/AC3 等）。未选择文件时整个操作面板被遮罩禁用。

## 4. HLS 转码

- 单飞（in-flight 去重）+ 后台；命令见 `streaming/transcode.go`（`-hls_list_size 0`、
  `-hls_flags temp_file+independent_segments` 原子写、`-map 0:a:0?`）。
- `master.m3u8` **读入内存快照**服务（ffmpeg 原地重写，直接 ServeContent 会读到截断），仅含
  `#EXTINF` 才对外服务、`Cache-Control: no-store`（避免 304 卡死 hls.js）。
- `.done` 标记区分完成缓存/崩溃残留，残留整目录重建；转码结束从 `s.active` 移除。
- 缓存随 `VideoDeleted`/`VideoUpdated` 失效。
- HLS 仅单一路码率（`-hls_time 10`，无自适应多码率）；嵌入字幕轨道抽取未做（仅侧边字幕文件）。

## 5. 能力探测唯一来源

- 直连/HLS 判定由后端 `streaming.DirectPlayable`/`HLSEnabled` 计算，`GET /api/videos/{id}` 返回
  `direct_playable`/`hls_enabled`，前端不重复实现。

## 6. 字幕

- 侧边 `.srt/.vtt/.ass` 字幕文件（与视频同名）会被识别并提供给播放器；嵌入字幕轨道抽取未做。
