# media.md — 媒体管线约定

> 改动 `backend/internal/{media,streaming}` 或前端播放器媒体相关代码前必读。
> FFmpeg 安装与环境 PATH 见 [environment.md](environment.md)。

## 1. ffprobe / ffmpeg 收口（ADR-020）

- 所有命令构建统一收口 `media` 包（探测/缩略图/字幕提取/Remux/HLS 分片/格式工厂），
  调用方只传结构化参数不拼命令行；基础头 `-nostdin -hide_banner -loglevel error -y`、
  映射模式 `-map 0:v:0 -map 0:a:N?`、音频白名单 `UniversalAudioCodecs`（aac/mp3）、
  可流拷贝视频编码 `RemuxVideoCodecs`（h264/avc1/avc3）、位图字幕 `BitmapSubtitleCodecs`
  （PGS/VobSub/DVB…）均只此一处定义。
- 二进制路径启动时 `ResolvePaths` 统一解析：绝对路径直用，裸名走 PATH，**缺失启动即报错**；
  显式空值 = 禁用对应能力（remux/transcode/字幕提取/格式工厂）。PATH 找不到时在 config.yaml
  显式配置（YAML `\\` 转义），改后重启。

## 2. 容器判定

- ffprobe `format_name` 是逗号分隔列表（如 `mov,mp4,m4a,...`）；`Probe` 归一化首个 token 入库；
  `DirectPlayable` 按逗号分词匹配，禁止整串查映射。
- **contentType 优先按扩展名判定**：MOV demuxer 覆盖整个 MP4 家族，先按 demuxer token 会把真实
  .mp4 误标成 `video/quicktime`，桌面浏览器不认该 MIME 会退化为整文件下载。

## 3. 分段 MP4 与播放元信息

- hls.js 拼接的文件式 MP4 有多个顶层 `mdat` 或 `moof` 分片，Chrome `<video src>` 会整文件顺序下载后才播。
  `Probe` 走顶层 box（逐盒跳转）识别写入 `videos.segmented`；**分段文件判不可直连**，
  兜底 = Remux 流拷贝成常规 Faststart MP4，否则格式工厂。
- 同一遍 box 遍历顺带写：`audio_codec`（供 canPlayType 核对）、`faststart`（moov 是否在首个 mdat 前；
  非 faststart 能播但拖动需缓冲，详情页提示「非快速启动」，不阻止播放）。

## 3.1 格式工厂

把任意视频/文件夹转为**浏览器可直接 Range 播放的 Faststart MP4 副本**
（`fservice/convert.go` + `media.ConvertToMp4`）。三层动态流皆不可的资源才需要它（ADR-006）。

- **输入**：媒体库视频集 `IsVideo` 外加 ffmpeg 可读常见容器（rmvb/rm、vob、asf、dat 等
  `convertibleExts`）；列表 API `is_convertible` 驱动文件页按钮可用态。媒体库扫描仍只认 `IsVideo`。
- **输出命名**：单集 → 同目录 `X.mp4`（同名 `(N)` 递增，绝不覆盖源）；系列/文件夹 → 同级
  `X (MP4)\` 只转直接一级视频。
- **预设**（`ConvertParams{video,vcrf,audio,akbps,burn}`，`norm()` 钳制非法值）：

  | 预设 | video | audio | 说明 |
  |---|---|---|---|
  | 快速 MP4 | copy | smart | 两级自动策略（见下） |
  | H.264 MP4 | h264 CRF19 | smart / aac | 重编码；文本字幕转 mov_text 保留，位图字幕需勾选烧录 |
  | H.265 MP4 | h265 CRF23 | smart / aac | 同上，产物 HEVC |

  `audio=smart`：仅通用编码（aac/mp3 白名单）拷贝，其余一律 AAC。
- **两级策略（快速 MP4 自动选择）**：
  1. 无损流拷贝首选（`-map 0 -c copy -c:s mov_text +faststart`），但**按首个音轨编码**判定——
     非通用编码（AC3/EAC3、dts、vorbis、opus、flac、pcm…）一律转 AAC 192k：浏览器通常无 Dolby
     解码器会无声、Windows 播放器也报「不支持 AC3」。混装「aac 首轨 + AC3 次轨」时次轨会一并拷贝
     （罕见场景，v1 接受）。
  2. 失败自动降级（典型 PGS/VobSub 位图字幕）：libx264 CRF19 高质量重编码 + 烧录首选字幕进画面
     （libass，滤镜路径正斜杠 + `\:` 转义盘符冒号）；再失败报错跳过。
- **陷阱**：输出为无扩展名 `.tmp`，必须显式 `-f mp4`；烧录路径只映射首选音轨。
- **进度**：解析 ffmpeg `-progress` 的 `out_time_us`，主路径 = out_time/ffprobe 真实时长；
  时长缺失的流拷贝用 total_size/源大小（≈1:1）；最后按大小估算时长兜底。ETA 由 jobs reporter 推算。
- 操作面板交互（预设填充、探测徽标、字段禁用规则）见 frontend.md §5 与 `fservice/convert_probe.go`。

## 4. 能力判定：三层动态流（ADR-006）

- **唯一决策源是前端运行期 `canPlayType()`**（`lib/playability.ts`）：probe 元数据（容器/视频编码/
  音频编码/segmented）→ MIME + RFC 6381 codecs → `HTMLMediaElement.canPlayType()`。
  据 `playMode()` 选择 Direct / Remux / Transcode，三者皆不可 → 格式工厂兜底。
  **`mov/qt` 必须映射到 `video/mp4`**：Chromium 对 `video/quicktime` 返回空，会把每个 mp4 误判为不可播。
- 后端三标志仅兜底/门槛：
  - `direct_playable`：保守 fallback（Chromium 确定支持集，不含 MKV/HEVC/fMP4/AC3/EAC3——
    Chromium 与 Firefox 均无 Dolby 解码器，AC3 会无声），仅在 canPlayType 不可用时兜底；
  - `remux_playable`：ffmpeg 已配置 且 视频∈RemuxVideoCodecs 且 音频无声或∈UniversalAudioCodecs；
  - `transcode_playable`：ffmpeg 已配置 且 时长已知。
- **音频不可拷贝（AC3/EAC3/DTS/PCM）不走 Remux 层**：整条音频重编码 ~70× 实时（100 分钟约 80 秒），
  而 Remux 是整片生成完才起播的阻塞模型，会卡死首播；HLS 分片转码几秒即起播。可流拷贝时 Remux
  优先于 Transcode（纯拷贝成本极低）。
- **MKV**：`canPlayType('video/x-matroska')` 天然区分浏览器版本差异（含原生 demuxer 的新版 Chromium
  可直连）。**MKV 且 `audio_codec` 为空时保守判不可直连**（空值无法确认音轨能否解码，DTS/PCM 会无声；
  前端判不可播、后端 RemuxPlayable 当无声 → 一律走 Remux），重扫源填充后按真实编码判定。
- **HEVC** 由目标机能力决定：装「HEVC 视频扩展」或硬件解码可直连，否则 Transcode。
- 判定结果在页面生命周期内缓存（解码能力不会中途变化）；详情/成员响应携带三标志与 probe 元信息。

### 4.1 Remux（容器重封装）

- 适用：视频编码浏览器可解但容器不兼容（典型 MKV h264+aac），音频须可流拷贝。
- 首次请求把源**整片流拷贝**成缓存 MP4（`-c:v copy -c:a copy +faststart`，`?audio=N` 选轨默认 0），
  落盘 `remux/<id>.mp4` / `<id>-a<N>.mp4` + 同名指纹 sidecar（源变化即重封装），随后 Range 输出全片可拖。
  多音轨各自独立缓存，换轨命中即秒开。
- **为什么整片流拷贝而非按需 seek 切片**：实测 ffmpeg 对 Matroska 的流拷贝按需 seek 不可靠
  （cluster 边界 + B 帧重排，落到前一关键帧/错位 GOP，且与布局/版本相关），MP4 索引 seek 才精确；
  一次纯拷贝成本极低（数秒），结果天然可 Range 播放。
- 每视频进程内互斥锁串行生成；`VideoDeleted` → RemoveCache 清除。

### 4.2 Transcode（按需转码 HLS）

- 适用：编码不兼容（HEVC/rmvb/MPEG2/DTS 等）或音频不可拷贝。
- VOD 全量播放列表（关键帧对齐分片、`#EXT-X-ENDLIST`）+ 按需生成 `seg-{n}.ts`；播放列表内嵌
  **携带 `?session=` 的绝对分片 URL**（hls.js 相对解析会丢 query）。
- 单分片命令：`-ss <关键帧> -i src … libx264 veryfast CRF23 yuv420p + 音频参数
  -mpegts_copyts 1 -output_ts_offset -t <len> -f mpegts`。重编码做**精确 seek**（解码到目标丢弃），
  内容与 PTS 精确对齐，与容器 seek 布局无关。
- **多音轨**：换轨必须换新 session（会话创建时固化音轨号）；声道数按所选轨探测。
- **音频声道布局陷阱（实测）**：ffmpeg 原生 AAC 编码器对非标准布局（5.1(side)）输出 ADTS
  `chanCfg=0`+PCE，hls.js 由此推导出 **0 声道** esds 被 Chromium MSE 拒绝（卡 0:00:00 无限重取分片）。
  修复：>2 声道源重映射 `-channel_layout 5.1`（chanCfg=6）+ `-b:a 192k`，≤2 声道 AAC 128k。
  原生 `<video src>`（Direct/Remux）无此问题——esds 含完整 PCE。
- 会话按 token 分离、按 videoID 缓存关键帧扫描，空闲 ~10min Sweep 清理；per-segment 锁防重复生成。
- 分片间 B 帧重排尾 1 帧重叠是 MPEG-TS 固有现象，hls.js 按 EXTINF 累积时间轴对齐，可容忍。

### 4.3 常见格式处理路径速查

> 判定是运行期的：同一文件在不同浏览器路径可能不同。下表为典型路径；决策链见 §4。

| 容器 | 典型路径 |
|---|---|
| mp4/mov/m4v/3gp（box 家族，MIME video/mp4） | 编码兼容（h264/vp8/vp9/av1/theora + aac/mp3/opus/vorbis/flac/无声）→ Direct；HEVC 视目标机；h264+AC3/EAC3/DTS/PCM → Transcode；fMP4(segmented) 不可直连 → h264 走 Remux 否则 Transcode；非 faststart 仍 Direct |
| webm / ogg/ogv | vp8/vp9/av1+opus/vorbis → Direct；theora 视目标机（现代 Chrome 已移除解码）；其余 → Transcode |
| mkv/matroska | 支持 Matroska 的 Chromium 且 h264+aac/mp3 → Direct；其余浏览器 h264+aac/mp3 → Remux；AC3/EAC3/DTS/PCM 或 mpeg2/mpeg4(asp)/rv40/wmv3/vc1 → Transcode |
| avi/wmv/flv/ts/m2ts/mpeg（无原生播放路径） | h264+aac/mp3/无声 → Remux（流拷贝）；其余组合（wmv3/vc1、mpeg2、h263/vp6、AC3 类音频）→ Transcode |
| rm/rmvb 等杂项 | RealVideo 无法无损封装 → Transcode 或格式工厂重编码 |
| 兜底 | ffmpeg 未配置或源不可达 → 播放禁用引导格式工厂（Direct/Remux 409 `storage_unavailable`、HLS playlist 409 `stream_unavailable`） |

注：内封**文本**字幕轨不影响播放路径（可提取 vtt）；**位图字幕**（PGS/VobSub）只能经格式工厂烧录。

### 4.4 架构取舍备注（已否决方案）

> 经评估**不采纳**，记录理由以防反复推翻（实现以三层动态流为准）。

- **单文件 fMP4 直连**：canPlayType 不区分 moof 分片布局，而 Chromium `<video src>` 对 fMP4 需整片
  下载后才播；判不可直连、h264 走 Remux 是可靠路径。
- **MediaCapabilities 判定 HEVC**：canPlayType 已隐含平台解码器检查，增量价值低且需把同步的
  playMode 改异步（改动面大），不引入。
- **前端 demux TS（mux.js）**：后端 Remux 对 TS 流拷贝简单可靠；个人量级 TS 占比低，不引入。

## 5. 字幕

- 侧边 `.srt/.vtt/.ass`（与视频同名）优先提供。
- 内封文本轨：`GET /api/videos/{id}/subtitles` 枚举全部字幕源（侧边+内封，带语言/标题标签）；
  `GET /api/stream/{id}/subtitle?track=<index>` 按需提取为 WebVTT 并缓存
  （`subtitles/<id>-<index>.vtt`）。前端渲染多 `<track>` 可切换多轨。
- 位图字幕无法转文本：清单中列出但 `playable=false`，只能经格式工厂「烧录进画面」。

## 6. 音轨

- 枚举：`GET /api/videos/{id}/audio` 返回全部音轨 `{index, codec, language, label, channels}`；
  **index 是 0 起音轨序号**，直接对应 `-map 0:a:<index>` / `?audio=<index>`；
  label = title > 语言中文名 > 「音轨 N」。
- 选轨重建流：Remux → `?audio=N` 按轨独立缓存；Transcode → 新 session + `&audio=N`；
  Direct 暂不处理（浏览器默认轨）。切换语义 = 「选轨 → 重建流 → seek 回原进度」（短暂重载，非无缝；
  真正无缝需 HLS 多 rendition master playlist，改动大且仅惠及 Transcode，暂缓）。
- **已知限制（v1 接受）**：混装「可拷贝 aac/mp3 + 不可拷贝 ac3」音轨时 Remux 选到不可拷贝轨可能无声。

### 6.1 播放选择记忆

- 单集级 `playback_prefs` 存具体值；系列级 `series_playback_prefs` 按轨道**名称**存储
  （同一选择在每集解析到各自真实轨），**系列记录优先于单集记录**（单集记录作关系重组后的兜底；
  单集归属系列后手动切换写系列级）。表结构见 decisions.md §1.2。
- API：`GET/PUT /api/videos/{id}/prefs`（GET 返回有效记录 + scope + series_id；PUT 部分更新）、
  `GET/DELETE /api/series/{id}/prefs`、删除入口在缓存管理。与续播 history 分离：记忆是可重建缓存，
  history 是用户数据。
- 播放器应用/保存机制见 [frontend.md](frontend.md) §3。
