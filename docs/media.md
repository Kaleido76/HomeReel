# media.md — 媒体管线实现事实源

> 改动 `backend/internal/{media,streaming}` 或前端播放器媒体相关代码前必读。
> FFmpeg 安装见 [environment.md](environment.md)；架构决策见 [decisions.md](decisions.md) §0/006。

## 1. ffprobe / ffmpeg 收口（ADR-020）

所有命令构建统一收口 `media` 包，调用方只传结构化参数不拼命令行。
二进制路径启动时 `ResolvePaths` 统一解析：绝对路径直用，裸名走 PATH，**缺失启动即报错**。

## 2. 容器判定

- ffprobe `format_name` 归一化首个 token 入库；`DirectPlayable` 按逗号分词匹配
- **contentType 优先按扩展名判定**：避免真实 .mp4 误标成 `video/quicktime`

## 3. 分段 MP4 与播放元信息

- `Probe` 识别 `videos.segmented`（分段文件判不可直连）
- 同一遍 box 遍历顺带写：`audio_codec`、`faststart`

### 3.1 格式工厂

> 前端操作面板交互见 [frontend.md](frontend.md) §5。

把任意视频/文件夹转为**浏览器可直接 Range 播放的 Faststart MP4 副本**。
三层动态流皆不可的资源才需要它。

- **输出命名**：单集 → 同目录 `X.mp4`（同名 `(N)` 递增）；系列/文件夹 → 同级 `X (MP4)\`
- **预设**：

  | 预设 | video | audio | 说明 |
  |---|---|---|---|
  | 快速 MP4 | copy | smart | 两级自动策略 |
  | H.264 MP4 | h264 CRF19 | smart / aac | 重编码 |
  | H.265 MP4 | h265 CRF23 | smart / aac | 同上，产物 HEVC |

  `audio=smart`：仅通用编码（aac/mp3）拷贝，其余一律 AAC。
- **两级策略（快速 MP4 自动选择）**：
  1. 无损流拷贝首选，但非通用编码一律转 AAC 192k
  2. 失败自动降级（位图字幕）：libx264 CRF19 重编码 + 烧录首选字幕
- **进度**：解析 ffmpeg `-progress` 的 `out_time_us`

## 4. 能力判定：三层动态流（ADR-006）

- **唯一决策源是前端运行期 `canPlayType()`**（`lib/playability.ts`）
- 后端三标志仅兜底/门槛：`direct_playable` / `remux_playable` / `transcode_playable`
- **音频不可拷贝（AC3/EAC3/DTS/PCM）不走 Remux 层**（整条音频重编码 ~70× 实时）
- **MKV 且 `audio_codec` 为空时保守判不可直连**
- **HEVC** 由目标机能力决定

### 4.1 Remux（容器重封装）

- 适用：视频编码浏览器可解但容器不兼容（典型 MKV h264+aac），音频须可流拷贝
- 首次请求把源**整片流拷贝**成缓存 MP4，随后 Range 输出全片可拖
- 多音轨各自独立缓存，换轨命中即秒开

### 4.2 Transcode（按需转码 HLS）

- 适用：编码不兼容或音频不可拷贝
- VOD 全量播放列表 + 按需生成 `seg-{n}.ts`
- **多音轨**：换轨必须换新 session
- **音频声道布局陷阱**：>2 声道源重映射 `-channel_layout 5.1`
- 会话按 token 分离、按 videoID 缓存关键帧扫描，空闲 ~10min 清理

### 4.3 常见格式处理路径速查

> 判定是运行期的：同一文件在不同浏览器路径可能不同。

| 容器 | 典型路径 |
|---|---|
| mp4/mov/m4v/3gp | 编码兼容 → Direct；HEVC 视目标机；fMP4 → Remux 或 Transcode |
| webm / ogg/ogv | vp8/vp9/av1+opus/vorbis → Direct；其余 → Transcode |
| mkv/matroska | h264+aac → Chromium Direct / 其余浏览器 Remux；AC3/DTS → Transcode |
| avi/wmv/flv/ts/m2ts/mpeg | h264+aac → Remux；其余 → Transcode |
| rm/rmvb 等杂项 | RealVideo → Transcode 或格式工厂 |

注：内封**文本**字幕轨不影响播放路径；**位图字幕**只能经格式工厂烧录。

### 4.4 架构取舍备注（已否决方案）

- **单文件 fMP4 直连**：Chromium 需整片下载后才播，不采纳
- **MediaCapabilities 判定 HEVC**：增量价值低，不引入
- **前端 demux TS（mux.js）**：后端 Remux 对 TS 流拷贝简单可靠，不引入

## 5. 字幕

- 侧边 `.srt/.vtt/.ass`（与视频同名）优先提供
- 内封文本轨：按需提取为 WebVTT 并缓存
- 位图字幕无法转文本：只能经格式工厂「烧录进画面」

## 6. 音轨

- 枚举：`GET /api/videos/{id}/audio` 返回全部音轨
- 选轨重建流：Remux → `?audio=N` 按轨独立缓存；Transcode → 新 session
- **播放选择记忆**：单集级存具体值；系列级按轨道名称存储，系列记录优先于单集记录
  （语义见 decisions.md §1.2；前端应用/保存机制见 [frontend.md](frontend.md) §3）
