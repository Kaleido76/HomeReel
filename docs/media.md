# media.md — 媒体管线约定

> 改动 `backend/internal/{media,streaming}` 或前端播放器媒体相关代码前必读。
> FFmpeg 安装与环境 PATH 见 [environment.md](environment.md)。

## 1. ffprobe / ffmpeg

- 探测、缩略图、字幕、格式工厂转换均依赖 FFmpeg / ffprobe。
- 进程 PATH 找不到时在 `config.yaml` 显式配置 `media.ffmpeg_path` / `media.ffprobe_path`
  （YAML `\\` 转义），改后重启。

## 2. 容器判定

- ffprobe `format_name` 是逗号分隔列表（如 `mov,mp4,m4a,...`）。`media.Probe` 归一化首个 token 入库。
- `streaming.DirectPlayable`（后端保守 fallback）按逗号分词匹配，**禁止整串查映射**。
- **`contentType` 优先按文件扩展名**判定（`.mp4/.m4v→video/mp4`、`.mov→video/quicktime` 等），
  demuxer 分词仅作扩展名未识别时的回退——因为 MOV demuxer 覆盖整个 MP4 家族，先按 token 会把真实
  .mp4 误标成 `video/quicktime`，桌面浏览器不认该 MIME 会退化为整文件下载/卡顿（2026-09 修复）。

## 3. 分段 MP4（2026-09）与播放元信息

- hls.js 拼接文件/分片式 MP4 有多个顶层 `mdat` 盒子或 `moof` 分片，Chrome `<video src>` 会整文件
  顺序下载后才播放。
- `media.Probe` 在探测时轻量走顶层 box（读 8/16 字节头逐盒跳转）识别并写入 `videos.segmented`
  （迁移新增列），`mdat>1` 或发现 `moof` 即判真，仅对 mp4/mov 家族扫描。
- **分段文件判不可直连**（`segmented` 前端直接否决播放）。动态流兜底：h264 流可走 Remux（§4.1，流拷贝成
  常规 Faststart MP4 后 Range 播放），无法 Remux 的引导「格式工厂」（§3.1）转标准 Faststart MP4 副本。
- `media.Probe` 同时写入：
  - `audio_codec`：首个音频流的编码名（供前端 canPlayType 核对音频是否可解）；
  - `faststart`：mp4 家族文件 **moov 是否位于首个 mdat 之前**（`isFastStart` 复用 box 遍历），
    非 faststart 在详情页展示「非快速启动」提示（拖动需缓冲，建议转换以获得流畅体验），不阻止播放。

## 3.1 格式工厂（2026-09，替代原「重封」页签）

把任意本地视频/文件夹转换为**浏览器可直接 Range 播放的 Faststart MP4 副本**（`fservice/convert.go`）。
2026-08 起播放走**三层动态流**（§4）：Direct / Remux / Transcode 三者皆不可的资源（编码与容器都不兼容、
且 ffmpeg 未配置等）才播放按钮禁用并引导格式工厂转换，转换产物即可直连播放、可拖动进度条。

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
   **逐比特拷贝**，码率/画质/所有音轨不变；文本字幕转 mov_text 保留。唯一例外：**按首个音轨编码**判定——
   仅当首轨是**全设备通用**的 aac/mp3（`universalMp4Audio` 白名单）才拷贝，否则（**AC3/EAC3**、dts、vorbis、opus、
   flac、truehd、pcm…）一律转 AAC 192k——AC3/EAC3 浏览器通常无 Dolby 解码器会无声、Windows 播放器也报
   「不支持的 AC3 编码」，拷贝会产出「换个设备就无声」的文件。视频永远无损。**注意**：判定只看首个音轨，
   混装「aac 首轨 + AC3 次轨」时 `-map 0` 会把非通用轨一并拷贝（罕见场景，见 §6 已知限制）。
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
- 前端：文件页签工具栏「格式工厂」按钮把勾选（或含可转换文件的当前目录）移交到「工具」页签的格式工厂工具
  （`features/tools/format/FormatFactoryPage.tsx`）。面板自上而下：**操作面板**（预设工具 + 可微调参数表单）、
  **待转换队列**（勾选的文件/文件夹 + 「开始转换」；再次从文件页移交会**替换**当前待转换批次，非追加）、
  **转换队列**（所有 convert 任务：进行中在前带进度条与 ETA，历史在后分成功/失败，每行标注所用预设）。
- **探测信息（`POST /api/convert/probe`，`fservice/convert_probe.go`）**：为所选文件逐个 ffprobe
  （`probeStreams` 复用转换引擎的探测），目录展开为直接一级视频，返回 `video_codec` / `audio_codecs` /
  `subtitle_codecs` / `duration` / `has_bitmap_subtitle`（`bitmapSubtitleCodecs`：PGS/VobSub/DVB 等位图字幕）。
  前端据此在操作面板给出**检测信息**与禁用规则：含位图字幕 → 提示「快速 MP4 将自动降级为烧录」；检测到
  非通用音频（AC3 等）→ **禁用「保留原音」选项**（否则产出 Windows 播放器无声文件）、提示将自动转 AAC；
  均无字幕轨 → 禁用烧录字幕复选框；无损流拷贝 → 禁用清晰度 CRF；保留原音 → 禁用音频码率。待转换清单每行
  展示该文件的探测徽标（位图字幕/文本字幕/AC3 等）。未选择文件时整个操作面板被遮罩禁用。

## 4. 能力判定（2026-08 修订：三层动态流，ADR-006）

- **唯一决策源是前端运行期 `canPlayType()`**（`frontend/src/lib/playability.ts` 的 `reportFor`/`playMode`）：
  probe 元数据（容器/视频编码/音频编码/segmented）→ MIME + RFC 6381 codecs 串 →
  `HTMLMediaElement.canPlayType()`。据结果选择播放层：
  - **Direct** → Range 直连 `/api/stream/{id}`；
  - **Remux**（编码兼容、容器不兼容，如 MKV h264+aac）→ `/api/stream/{id}/remux`；
  - **Transcode**（编码不兼容，HEVC/rmvb/DTS 等）→ `/api/stream/{id}/hls/*`；
  - 三者皆不可 → 播放按钮禁用 + 引导格式工厂转换。
  **`mov`/`qt` 映射到 `video/mp4` 而非 `video/quicktime`**——ffprobe 把整个 MP4 家族报为
  `mov,mp4,m4a,3gp,3g2,mj2`（首个 token `mov`），而 Chromium 的 `canPlayType('video/quicktime; …')`
  返回空，会把每个 mp4 误判为不可播；`video/mp4` 对该 box 家族始终可解析。
- 后端 `streaming.DirectPlayable` / 系列成员 `direct_playable` 是**保守 fallback**（Chromium 确定支持集：
  原生容器 + h264/vp8/vp9/av1/theora + aac/mp3/opus/vorbis/flac，不含 MKV/HEVC/fMP4/**AC3/EAC3**——Chromium
  与 Firefox 均无 Dolby 解码器，AC3 音轨会无声），仅在 `canPlayType` 探测不可用时兜底，不作为主判据。
- 后端另提供两个**动态流门槛**：`remux_playable`（`streaming.RemuxPlayable`：ffmpeg 已配置 且 视频
  ∈{h264,avc1,avc3} 且 音频为空 或 ∈{aac,mp3}（可流拷贝））与 `transcode_playable`（`streaming.TranscodePlayable`：
  ffmpeg 已配置 且 时长已知）。音频不可拷贝（AC3/EAC3/DTS/PCM——浏览器无 Dolby 解码器/无损 PCM）的文件
  **不判 remux_playable**，落入 Transcode：整条音频重编码要 ~70× 实时（一部 100 分钟影片约 80 秒），
  Remux 整片阻塞生成会卡死首播，而 HLS 按分片转码几秒即起播。Remux 优先于 Transcode（对可流拷贝的文件成本低）。
  三者皆不可 → 格式工厂兜底。
- **MKV** 由 `canPlayType('video/x-matroska; codecs=…')` 天然区分：Chromium 对 Matroska 的原生支持因
  版本/平台/构建而异（含原生 Matroska demuxer 的桌面新版本可直连），其余浏览器返回空（走 Remux/HLS）。**MKV 且 `audio_codec` 为空（未探测/源未重扫）时保守判不可播放**
  ——空值无法确认音轨能否解码（DTS/PCM 会无声），重扫源填充 `audio_codec` 后有声轨的按编码正确判定。
- **HEVC** 由目标机能力决定：装「HEVC 视频扩展」或硬件解码支持时 `canPlayType` 返回非空（可直连），
  否则走 Transcode HLS（仅转码实际观看的分片）。
- `GET /api/videos/{id}` 返回 `direct_playable / remux_playable / transcode_playable` 与 video 的
  `container/codec/audio_codec/segmented/faststart` 元信息；`GET /api/series/{id}` 成员带同样字段。

### 4.1 Remux（容器重封装）

- 适用：视频编码浏览器可解但容器不兼容（典型 MKV h264+aac），音频亦须可流拷贝（aac/mp3 或无声）。
  音频不可拷贝（AC3/EAC3/DTS/PCM）的文件**不走此层**，见 §4.2 Transcode。
- 实现：`streaming.Remux` → `remuxPath`。首次请求把源**整片流拷贝**成缓存 MP4
  （`-i src -map 0:v:0 -map 0:a:N? -c:v copy -c:a copy`；`-movflags +faststart -f mp4`；`?audio=N` 选音轨，
  默认 0），落盘 `data_dir/remux/<id>.mp4`（默认轨）或 `<id>-a<N>.mp4`（其它轨）+ 同名 `.meta`
  （`size mtime` 指纹，源变化即重封装），随后经 `http.ServeContent` 走 HTTP Range，浏览器原生全片可拖。
- **多音轨**：`GET /api/videos/{id}/audio` 枚举全部音轨（index/codec/声道/label），前端菜单选轨后以
  `?audio=N` 重建流；每个音轨独立缓存（`<id>.mp4` / `<id>-a1.mp4` …），换轨即时起播（若已缓存）。
- **为什么整片流拷贝而非按需 seek 切片**：实测 ffmpeg 8.1.2 对 Matroska 的流拷贝按需 seek 不可靠
  （cluster 边界判断 + B 帧重排，落到前一个关键帧/错位 GOP，且与布局/版本相关），MP4 索引 seek 才精确。
  一次流拷贝成本极低（纯拷贝约数秒），结果天然可直接 Range 播放，故 Remux 层不切片。
- **为什么音频不可拷贝的文件不在此层**：整条音频重编码 ~70× 实时（100 分钟影片约 80 秒），而 Remux
  是「整片生成完才起播」的阻塞模型，首播会长时间转圈；AC3/EAC3/DTS/PCM 改走 Transcode 按分片重编码，
  几秒即起播。可流拷贝的 aac/mp3 仍是 Remux（纯拷贝，秒级完成）。
- 每视频一个进程内互斥锁串行生成；并发请求复用缓存；`VideoDeleted` → `RemoveCache` 顺带清除 remux。

### 4.2 Transcode（按需转码 HLS）

- 适用：编码不兼容（HEVC/rmvb/MPEG2/DTS 等）**或音频不可拷贝**（AC3/EAC3/DTS/PCM——浏览器无 Dolby 解码器，
  且整条音频重编码太慢不宜在 Remux 整片生成），`streaming.TranscodePlayable`。
- 实现：`streaming.Playlist` / `Segment`（`hls.go`）。VOD 全量播放列表（关键帧对齐分片、`#EXT-X-ENDLIST`）
  + 按需生成分片 `seg-{n}.ts`。播放列表内嵌**携带 `?session=` 的绝对分片 URL**（hls.js 相对解析会丢 query）。
- 单分片命令：`-ss <kf[n]> -i src -map 0:v:0 -map 0:a:N? -c:v libx264 -preset veryfast -crf 23
  -pix_fmt yuv420p` + 音频参数（见下）+ `-mpegts_copyts 1 -output_ts_offset <kf[n]> -t <len> -f mpegts`
  （`?audio=N` 选音轨，默认 0）。重编码做**精确 seek**（解码前一个关键帧并丢弃到目标），内容与 PTS 均精确
  落在 `kf[n]`，与容器 seek 布局无关。
- **多音轨**：前端选轨时生成**新会话 token** + `&audio=N`（换轨必须换会话，因会话创建时固化音轨号），
  `generateSegment` 按 `hs.audio` 映射 `0:a:N`；声道数也按所选轨探测（不同音轨声道可能不同）。
- **音频声道布局（2026-08 修复）**：会话按**所选音轨**探测 `ProbeAudioChannels`（不再按 videoID 共享——不同
  音轨声道可能不同）。源 ≤2 声道 →
  `-c:a aac -b:a 128k`；源 >2 声道 → `-c:a aac -b:a 192k -channel_layout 5.1`。原因：ffmpeg 原生 AAC
  编码器对非标准布局（如 5.1(side)）输出 ADTS `chanCfg=0`（声道配置内嵌 PCE），hls.js 由此推导出
  **0 声道**的 esds，Chromium MSE 直接拒绝（`bufferAppendError`，播放器卡 0:00:00 且无限重取分片）。
  重映射为标准 5.1 布局使编码器输出 `chanCfg=6`，浏览器正常解码；6 声道全部保留（环绕→后环的
  重排，听感一致）。原生 `<video src>`（Remux/直连）无此问题——ffmpeg 写的 esds 含完整 PCE。
- 会话：前端每视频生成一个 `session` UUID，后端 `hlsManager` 按 token 分会话、按 videoID 缓存关键帧扫描，
  空闲 10min 由 `Sweep` 清理；`per-segment` 锁防并发重复生成。
- 分片间 B 帧重排尾 1 帧重叠为 MPEG-TS 固有现象，hls.js 按 EXTINF 累积时间轴 + timestampOffset 对齐。

### 4.3 常见格式处理路径速查

> 判定是**运行期**的：同一文件在不同浏览器上路径可能不同（如 MKV 在 Chromium 直连、在 Firefox/Safari 走
> Remux）。下表「路径」列按目标机能力分两栏说明。决策链见 §4；最终路径 = 前端 `canPlayType` 结果 +
> 后端 `remux_playable / transcode_playable` 标志（`playMode` 顺序：Direct → Remux → Transcode → 格式工厂）。

#### 4.3.1 MP4 / M4V / MOV（box 家族，MIME `video/mp4`）

| 视频编码 | 音频编码 | 路径 |
|---|---|---|
| h264 / avc1 / avc3 | aac / mp3 / opus / vorbis / flac | **Direct** |
| h264 / avc1 / avc3 | 无声 | **Direct** |
| h264 / avc1 / avc3 | ac3 / eac3 / dts / pcm / truehd 等（浏览器无法解码） | **Transcode**（音频转 AAC，分片起播快） |
| hevc / hvc1 | 兼容音频 | 目标机可解 HEVC（扩展/硬件）→ **Direct**；否则 **Transcode** |
| vp8 / vp9 / av1 | opus / vorbis | **Direct**（目标机支持该编码时） |
| theora | vorbis | 目标机支持 → **Direct**；否则 **Transcode**（现代 Chrome 已移除 theora 解码） |
| **任意**（fMP4，`segmented=true`） | 任意 | 不可直连 → h264 → **Remux**（流拷贝成常规 MP4）；否则 **Transcode** / 格式工厂 |
| **任意**（非 faststart） | 任意 | 仍 **Direct**（moov 在尾，拖动需缓冲，详情页提示「非快速启动」） |

#### 4.3.2 MKV / Matroska（MIME `video/x-matroska`）

| 视频编码 | 音频编码 | 路径 |
|---|---|---|
| h264 / avc1 / avc3 | aac / mp3 | 支持 Matroska 的 Chromium → **Direct**；其余浏览器 → **Remux**（流拷贝 MP4，Range 全片可拖） |
| h264 / avc1 / avc3 | 无声（`audio_codec` 空） | **Remux**（前端对 MKV 空音频保守判不可直连，见下方「MKV 无声保护」） |
| h264 / avc1 / avc3 | ac3 / eac3 / dts / pcm | **Transcode**（音频转 AAC——AC3/EAC3 浏览器无 Dolby 解码器会无声；整条音频重编码太慢不宜 Remux） |
| hevc / hvc1 | aac 等 | 目标机可解 HEVC → **Direct**；否则 **Transcode** |
| vp9 / av1 | opus / vorbis | 目标机支持 → **Direct**；否则 **Transcode** |
| mpeg2 / mpeg4(asp) / rv40 / wmv3 / vc1 | 任意 | **Transcode** |
| 内封**文本**字幕轨 | — | 不参与播放路径；可经 `/api/stream/{id}/subtitle` 提取 vtt 供字幕菜单 |
| 内封**位图**字幕（PGS/VobSub） | — | 无法转文本；只能经格式工厂「烧录进画面」 |

> **MKV 无声保护**：`audio_codec` 为空（真无声 **或** 未重扫导致音频未填充）时，前端对 MKV 保守判不可直连
> （`decoderSupported=null`），后端 `RemuxPlayable` 把空音频当无声 → **一律走 Remux**（与浏览器无关）。
> 真无声 → 产物无声正常；未重扫的有声文件 → 产物可能无声。**重扫源填充 `audio_codec` 后**按真实编码判定
> （有声的按编码进 Direct/Remux/Transcode）。

#### 4.3.3 WebM / OGG / OGV

| 容器 | 视频编码 | 音频编码 | 路径 |
|---|---|---|---|
| webm | vp8 / vp9 / av1 | opus / vorbis | **Direct** |
| ogg / ogv | theora | vorbis | 目标机支持 → **Direct**；否则 **Transcode** |
| 以上容器 | 其他编码 | 任意 | **Transcode** / 格式工厂 |

#### 4.3.4 AVI / WMV / FLV / TS（浏览器无原生容器，一律不直连）

| 容器 | 视频编码 | 音频编码 | 路径 |
|---|---|---|---|
| avi | h264 / avc1 | aac / mp3 / 无声 | **Remux** |
| avi | h264 / avc1 | ac3 / eac3 / dts / pcm | **Transcode**（音频转 AAC） |
| avi | mpeg4(asp)/xvid / mpeg2 / rv40 | mp3 / 任意 | **Transcode** |
| wmv / asf | wmv3 / vc1 | wmav2 | **Transcode** |
| flv | h264 / avc1 | aac / mp3 | **Remux** |
| flv | h263 / vp6 等 | 任意 | **Transcode** |
| ts / m2ts / mpeg | h264 / avc1 | aac / 无声 | **Remux**（流拷贝） |
| ts / m2ts / mpeg | h264 / avc1 | ac3 / eac3 / dts / pcm | **Transcode**（音频转 AAC） |
| ts / m2ts / mpeg | mpeg2 | mp2 / ac3 | **Transcode** |

> **3gp 例外**：`.3gp` 虽非浏览器原生容器扩展名，但 ffprobe 用 MOV demuxer 解析，容器入库为 `mov`，
> 前端映射到 `video/mp4` → 按 box 家族正常判定（h264+aac → **Direct**），与 mp4/mov 同规则。

#### 4.3.5 兜底（none → 格式工厂）

| 情况 | 说明 |
|---|---|
| ffmpeg 未配置 | `remux_playable` / `transcode_playable` 均为 false → 播放按钮禁用，引导格式工厂 |
| 源文件不可达（存储离线/已删除） | 详情页标 `missing`；Direct/Remux 端点返回 409 `storage_unavailable`，HLS playlist 返回 409 `stream_unavailable` |
| 编码 ffmpeg 也无法解码（罕见） | Transcode 亦失败 → 播放报错，格式工厂可尝试重新编码 |
| 需要永久离线/跨播放器观看 | 格式工厂产出 Faststart MP4 副本（`-c copy` 优先，失败自动降级重编码） |

### 4.4 架构取舍备注（2026-08 评估记录）

> 下列方案经评估后**不采纳**，记录理由以防反复推翻（ADR-006 边界，实现以三层动态流为准）。

- **单文件 fMP4 直连**：`canPlayType` 对 `video/mp4` 不区分普通布局与 moof 分片布局，而 Chromium 的
  `<video src>` demuxer 对 fMP4 需整片下载后才播（实测注释，见 `media.Info.Segmented`）。故 fMP4 判
  不可直连、h264 走 Remux（流拷贝成常规 MP4）是可靠路径，不冒直连播放质量风险。
- **MediaCapabilities 判定 HEVC**：Chromium 对 HEVC 的 `canPlayType` 已隐含平台解码器检查（无硬件/
  扩展返回空），Safari 有硬件解码。MediaCapabilities 的 `smooth/powerEfficient` 增量价值低，且需把
  同步的 `playMode` 改造成异步（改动面大），故不引入。
- **前端 demux TS（mux.js）**：后端 Remux 对 TS 流拷贝简单可靠；前端 demux 增加复杂度和浏览器兼容
  不确定性，个人量级 TS 文件占比低、后端成本可忽略，故不引入。

## 5. 字幕

- 侧边 `.srt/.vtt/.ass` 字幕文件（与视频同名）优先提供给播放器。
- **内封文本字幕轨**（MKV 的 ass/subrip 等）：`GET /api/videos/{id}/subtitles` 枚举全部字幕源（侧边 +
  内封文本轨，带语言/标题标签）；`GET /api/stream/{id}/subtitle?track=<index>` 按需用 ffmpeg 提取指定轨为
  WebVTT（`media.ExtractTextSubtitle`），缓存到 `data_dir/subtitles/<id>-<index>.vtt`。前端按清单渲染多个
  `<track>`，**播放器内置字幕菜单可切换多轨**（无需重封）。
- **位图字幕（PGS/VobSub）无法转文本**：`ListSubtitles` 仍会列出（`playable=false`，带 codec），详情页技术
  卡片标注「仅可格式工厂烧录」；播放器 `<track>` 只渲染 `playable` 的轨（`VideoPlayer` 过滤
  `t.playable !== false`）。只能经格式工厂「烧录进画面」。

## 6. 音轨（多音轨容器选轨，2026-09）

- **枚举**：`media.ProbeAudioStreams` 列出全部音轨（index/codec/声道/language/title）；
  `GET /api/videos/{id}/audio` 返回 `AudioTrack{index, codec, language, label, channels}`（**index 是 0 起
  音轨序号**，直接对应 `-map 0:a:<index>` / `?audio=<index>`；label = title > 语言中文名 > 「音轨 N」）。
- **选轨播放**：前端播放器「设置」菜单注入「音轨」项（`DefaultMenuSection`+`DefaultMenuRadioGroup`，
  与倍速/画质/字幕同一 UI，仅多轨且非 Direct 层显示）。选轨后按所选轨重建流：
  - **Remux** → `?audio=N`，后端按轨缓存 `<id>.mp4` / `<id>-a<N>.mp4`，`-map 0:a:N`；
  - **Transcode HLS** → **新会话 token** + `&audio=N`（会话创建时固化音轨号，换轨必须换会话），
    `generateSegment` 用 `-map 0:a:N` 且声道按所选轨探测；选轨后前端自愈 seek 回原进度（`ensurePendingSeek`）；
  - **Direct** → 暂不处理（浏览器默认轨；Chromium 原生 `video.audioTracks` 切换为后续可选增强）。
- **无缝切换说明**：本方案为「选轨 → 重建流 → seek 回原进度」，切换时有短暂重载（非无缝）。真正零中断的
  无缝切换需 HLS 多音轨 rendition（master playlist `EXT-X-MEDIA` + 每轨独立音频分片），改动较大、且仅惠及
  Transcode 层，暂缓（可作为后续增强）。
- **已知限制（v1 接受）**：同一文件若混装「可拷贝 aac/mp3 + 不可拷贝 ac3」音轨，Remux 层选到不可拷贝轨可能
  无声——罕见场景，统一编码的多音轨（如双 AC3 / 双 AAC）不受影响。

### 6.1 播放选择记忆（2026-09，per-video + 系列级 音轨/字幕/音量偏好缓存）

- **单集级**：`playback_prefs` 表（video_id,user 主键，FK 级联删除），列 `audio_track`（0 起音轨序号，NULL=未选）、
  `subtitle_id`（`sidecar`=侧边文件 / `e<N>`=内封文本轨流序号 / `""`=明确关闭字幕，NULL=未选）、`volume`（0~1）、
  `muted`。与续播 `history` 分离：前者是**可重建缓存**（工具页「缓存管理」可删，删行回默认），后者是用户数据。
- **系列级（2026-09 修订，ADR-006 player prefs）**：`series_playback_prefs` 表（series_id,user 主键，FK
  级联删除），列 `audio_track_name`/`subtitle_name`——音轨/字幕按**轨道名称（label）**存储与匹配，使同一选择
  在每集解析到各自真实轨道（如「简体中文」在每集都选中对应轨）；`volume`/`muted` 直接共享。**系列有记录时
  优先于单集记录**（「忽略单集记录」），单集记录作为关系重组后的兜底：单集曾独立播放过、或系列记录被清除后
  生效；单集归属系列后，手动切换**写系列级**（按名称），不再写自身。
- 读写：`GET/PUT /api/videos/{id}/prefs`（GET 返回**有效**记录 + `scope` + `series_id`；PUT 部分更新，只写
  携带字段，未携带字段保留——系列成员发名称、独立单集发序号/字幕 id）；系列记录单独走
  `GET/DELETE /api/series/{id}/prefs`；删除走 `DELETE /api/cache/prefs`（全部，含系列级）/
  `DELETE /api/cache/prefs/{videoId}`（单视频）。
- 播放器行为：进入播放自动应用记忆（音轨仅在 remux/transcode 生效——Direct 无法选轨；字幕/音量全层生效），
  仅用户手动切换音轨/字幕/音量时刷新。系列级按名称匹配不到当前集的轨道时该字段不应用（回默认）。系列详情页 /
  单集详情页的「播放历史」合并卡内「配置缓存」小节显示记忆内容并提供清除图标。实现细节见
  [frontend.md](frontend.md) §3「播放选择记忆」。
