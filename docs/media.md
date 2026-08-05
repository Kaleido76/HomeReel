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
- **分段文件不回退 HLS**（增量 HLS 期间按 live 处理、长片无进度条，体验差）；而是**保持直连** +
  **用户手动重封**：`POST /api/videos/{id}/remux`、`POST /api/fs/remux`（`{storageId,path}` 批量）、
  `GET /api/remux/status`。
- `remux` 作业用 `-c copy -movflags +faststart` 流拷贝重封到 `data/remux/<id>.mp4`（1GB 约 3s，不重编码），
  `streaming.Direct` 优先服务重封产物否则回退原文件；前端「重封」页签（`features/remux/RemuxPage.tsx`）管理。
- 删除/文件变更时 `RemoveCache` 清理 HLS 与 remux 缓存。

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
