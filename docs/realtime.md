# realtime.md — WebSocket 实时通道实现事实源

> 改动 `backend/internal/realtime` 或 `src/api/realtime.ts` 前必读。
> 架构决策见 [decisions.md](decisions.md) §0/021；API 契约见 decisions.md §2.7。

## 1. 目标与形态

- **单条 WebSocket 连接**承载双向通讯（ADR-021）
- **S→C 推送**：域事件（经事件桥）、任务进度
- **C→S RPC**：任意后端模块注册处理器，前端 `request()` 得到 Promise 响应
- **解除轮询**：推送替代「定时拉取」，业务编码只需声明「收到 X → 失效 Y」
- 多终端并发：每个会话一条独立连接；广播默认全量

## 2. 消息协议

统一信封（前后端一致）：

```jsonc
// 客户端 → 服务端 RPC 请求（带 id）
{ "id": "req-1-1720000000000", "type": "jobs.list", "data": { "limit": 50 } }
// 服务端 → 客户端 RPC 响应
{ "id": "req-1-1720000000000", "type": "jobs.list.result", "data": { "jobs": [] } }
// 服务端 → 客户端推送（无 id）
{ "type": "events.jobs.done", "data": { "job_id": "...", "type": "scan_source" } }
```

- `type` 使用 `域.动作` 命名空间
- 错误体 `{ code, message }` 与 REST 错误契约一致
- 内置：`ping/pong` 探活；`events.<域事件>` 事件桥转发

## 3. 后端实现

### 3.1 包结构（`backend/internal/realtime`）

```
realtime/
  protocol.go  # Envelope / 消息类型常量
  hub.go       # Hub：连接注册表、Broadcast、Handle(type,fn)、HandleConnection
  client.go    # Client：读泵/写泵、心跳、慢客户端丢弃
```

- **`Hub.Handle(typ, fn)`**：模块注册处理器；返回值自动组装为 `<type>.result`
- **`Hub.Broadcast(typ, payload)`**：全量推送，不阻塞调用方
- **慢客户端**：写队列满即断开该连接
- **心跳**：服务端每 30s 协议层 ping，60s 无 pong 判死
- **鉴权**：`GET /api/ws` 挂在 `requireAuth` 之后

### 3.2 装配（`cmd/server/main.go`）

- **事件桥**：`events.Bus.SubscribeAll()` 转发全部域事件为 `events.<type>` 推送
- 中间件兼容：`statusRecorder` 需实现 `http.Hijacker`/`http.Flusher`

## 4. 前端实现

### 4.1 `src/api/realtime.ts` — `RealtimeClient` 单例

- 生命周期：`connect()`（登录后）/ `disconnect()`（登出）
- **自动重连**：指数退避 1s→30s；`visibilitychange` 回到前台时立即重连
- API：`on(type, handler)` / `send(type, data)` / `request<T>(type, data, timeoutMs?)`

### 4.2 React 挂接（`src/components/RealtimeProvider.tsx`）

- `<RealtimeProvider>` 挂在 `AuthProvider` 内层
- 钩子：`useRealtime()`、`useRealtimeMessage(type, handler)`
- **`invalidateOnMessage(client, queryClient, mapping)`**：收到推送即 `invalidateQueries`

### 4.3 开发代理

`vite.config.ts` 的 `/api` proxy 已加 `ws: true`。

## 5. 消息类型注册表

| type | 方向 | 说明 |
|---|---|---|
| `ping` / `pong` | 双向 | 应用层探活 |
| `events.<域事件>` | S→C | 事件桥转发全部域事件 |

## 6. 迁移路线

> 进度明细见 [status.md](status.md) §2。

- **jobs** ✅ 已迁移：后端三时机推送用户任务快照；前端零轮询
- **文件页** 部分迁移：扫描完成已实时化；目录列表仍 5s 轮询
- 未来 fsnotify / upload 复用同一 Hub 推送通道
