# realtime.md — 实时双向通道（WebSocket，ADR-021）

> 前后端实时通讯基础设施的实现事实源。架构「为什么这样」见 [AGENTS.md](../AGENTS.md) §4 ADR-021，
> API 契约见 [decisions.md](decisions.md) §2.7。
>
> 本文件只描述**基础设施**与迁移路线；jobs 轮询已迁移（见 §6），文件页轮询待迁移。

## 1. 目标与形态

- **单条 WebSocket 连接**承载双向通讯（ADR-021）：
  - **服务端 → 客户端推送**：域事件（经事件桥）、任务进度、任何模块主动下发的通知。
  - **客户端 → 服务端 RPC**：任意后端模块注册处理器，前端 `request()` 得到 Promise 响应。
- **解除轮询**：推送替代「定时拉取」，前端收到即失效 TanStack Query 对应 queryKey 重新拉取，
  业务编码只需声明「收到 X → 失效 Y」。
- 多终端并发（ADR-002）：每个会话一条独立连接；广播默认全量，`SendTo` 预留定向。

## 2. 消息协议

统一信封（前后端一致）：

```jsonc
// 客户端 → 服务端 RPC 请求（带 id）
{ "id": "req-1-1720000000000", "type": "jobs.list", "data": { "limit": 50 } }
// 服务端 → 客户端 RPC 响应（id 关联，type 后缀）
{ "id": "req-1-1720000000000", "type": "jobs.list.result", "data": { "jobs": [] } }
{ "id": "req-1-1720000000000", "type": "jobs.list.error", "data": { "code": "internal", "message": "..." } }
// 服务端 → 客户端推送（无 id，即发即忘，不回复）
{ "type": "events.jobs.done", "data": { "job_id": "...", "type": "scan_source" } }
// 客户端 → 服务端即发即忘通知（无 id）
{ "type": "some.notify", "data": { ... } }
```

- `type` 使用 `域.动作` 命名空间（`events.*`、`jobs.*`…），前后端按同一注册表编码。
- 错误体 `{ code, message }` 与 REST 错误契约一致，前端 `RealtimeError` 与 `ApiError` 同构。
- 内置系统消息：`ping` → `pong`、`hello` → `hello.result`。心跳另有 WebSocket 协议层 ping/pong
  （浏览器自动应答，无需前端代码）。

## 3. 后端实现

### 3.1 包结构（`backend/internal/realtime`）

```
realtime/
  protocol.go  # Envelope / 消息类型常量 / request 应答封装
  hub.go       # Hub：连接注册表、Broadcast、Handle(type,fn)、HandleConnection（upgrade）
  client.go    # Client：单连接读泵/写泵、心跳、慢客户端丢弃
  hub_test.go
```

- **`Hub.Handle(typ, fn)`**：模块注册处理器 `fn(ctx, payload json.RawMessage) (any, error)`；
  返回值自动组装为 `<type>.result`，错误为 `<type>.error`。未注册类型应答 `<type>.error`
  （`unknown_type`）。即发即忘（无 id）不产生响应。
- **`Hub.Broadcast(typ, payload)`**：全量推送，可被任意 goroutine 调用，不阻塞调用方。
- **慢客户端**：每连接写队列（64 帧）满即把新帧交给分离 goroutine，仍阻塞则断开该连接——
  一个卡死终端不影响 Hub 或其它客户端。
- **心跳**：服务端每 30s 协议层 ping，60s 无 pong（读超时）判死。
- **鉴权**：`GET /api/ws` 挂在 `requireAuth` 之后——同源握手携带 `homereel_session` Cookie，
  未登录返回 401（升级前即拒绝）。`Upgrader.CheckOrigin` 保持宽松：Cookie 为 `SameSite=Lax`，
  跨站握手不带 Cookie 会被 `requireAuth` 拦下，无需再按 Origin 防御。

### 3.2 装配（`cmd/server/main.go` + `internal/api/server.go`）

- `api.New` 新增 `rt *realtime.Hub` 参数，路由 `GET /api/ws` → `s.requireAuth(s.rt.HandleConnection)`。
- **事件桥**：`events.Bus.SubscribeAll()`（叶子包新增，纯追加）订阅全部域事件，main.go 起
  goroutine 转发为 `events.<type>` 推送。**事件发布方零改动**，`VideoImported`/`JobDone`/
  `JobFailed` 等立即可达前端。
- 中间件兼容：`statusRecorder` 需实现 `http.Hijacker`/`http.Flusher`/`Unwrap`（WebSocket 升级
  依赖 Hijacker），否则 upgrade 会报 `response does not implement http.Hijacker`。

## 4. 前端实现

### 4.1 `src/api/realtime.ts` — `RealtimeClient` 单例

- 生命周期：`connect()`（登录后）/ `disconnect()`（登出）；幂等。
- **自动重连**：指数退避 1s→30s（含 ±500ms 抖动）；`visibilitychange` 回到前台时立即重连；
  断线期间 `request()` 全部 reject（`disconnected`），`send()` 入队（上限 50 条）连上后补发。
- API：
  - `on(type, handler)` → 返回退订；`onStatus(listener)` → 订阅连接状态
    （`disconnected | connecting | connected`）。
  - `send(type, data)`：即发即忘。
  - `request<T>(type, data, timeoutMs?)`：RPC，超时默认 15s，`<type>.error` → reject `RealtimeError`。
- RPC 响应按 `id` 关联（独立 pending 表），推送按 `type` 分发到订阅者——两类流量互不干扰。

### 4.2 React 挂接（`src/components/RealtimeProvider.tsx`）

- `<RealtimeProvider>` 挂在 `AuthProvider` 内层：`authenticated` 为真 `connect()`、登出
  `disconnect()`；暴露 context `{ client, status }`。
- 钩子：`useRealtime()`、`useRealtimeMessage(type, handler)`（挂载时订阅、卸载退订）。
- **`src/lib/realtimeQuery.ts` 的 `invalidateOnMessage(client, queryClient, mapping)`**：
  `{ 'events.jobs.done': [['jobs']] }` ——收到推送即 `invalidateQueries`。**迁移轮询的主要工具**。

### 4.3 开发代理

`vite.config.ts` 的 `/api` proxy 已加 `ws: true`，dev 下浏览器连 `ws://localhost:5173/api/ws` 即被
转发到 `ws://localhost:8080/api/ws`。

## 5. 消息类型注册表（现状）

| type | 方向 | 说明 |
|---|---|---|
| `ping` / `pong` | 双向 | 应用层探活（协议层心跳另有） |
| `hello` | C→S | 连接信息（预留） |
| `events.<域事件>` | S→C | 事件桥转发全部 `events.Bus` 事件（`events.video.imported`、`events.jobs.done`…） |

## 6. 迁移路线

目标：轮询点改为「初始 REST 快照 + 实时推送失效」，逐个迁移。进度明细见 [status.md](status.md) §2：

- **jobs** ✅ 已迁移——后端在入队 / reporter 上报 / Worker 收尾三个时机经 Hub 推送**用户任务**
  完整快照（内存态不落库，internal 任务不发）；前端初始 REST 快照 + 就地合并缓存，
  运行期零 `GET /api/jobs` 轮询，仅（重）连成功时补拉一次恢复断连期间遗漏。
- **文件页** 部分迁移：扫描完成已实时化（监听 `events.jobs.done|failed(type=scan_source)`
  即时失效 `['files-sources']`）；目录列表与离线徽标仍走 5s 轮询，待改为 fservice 操作完成后
  经 Hub 发布变更推送。
- 未来 fsnotify / upload 复用同一 Hub 推送通道。

## 7. 验证

- 后端：`go test ./...`（含 `internal/realtime` 单测：广播、处理器分发、错误应答、未知类型、
  ping/pong、慢客户端不阻塞；`internal/api` 握手鉴权与 RPC 全链路）。
- 前端：`pnpm run lint`、`pnpm run build`。
- 人工：浏览器 devtools → Network → WS 帧（登录后建连、登出断开、扫描多媒体源时收到
  `events.*` 推送）；双终端并发连接互不影响。