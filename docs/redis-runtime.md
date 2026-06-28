# Redis 运行态与事件链路说明

本文档说明 ZoneLease 中 Redis 的定位、环境变量、key 命名、数据类型、写入时机、过期策略和运维查看方式。

## 一、Redis 定位边界

ZoneLease 使用 PostgreSQL 作为业务事实源，Redis 只承担短期运行态能力：

- 发布刷新事件，供 SSE 长连接转发给前端。
- 保存最近刷新事件，支持 SSE 新连接回放。
- 缓存刷新任务运行中状态，减少任务列表读取运行态进度时的数据库压力。
- 缓存 Agent 最近健康检查结果，补齐仪表板服务器状态中的连续失败次数。
- 标记 Agent 正在执行同步任务，避免自动健康检查与同步任务并发访问同一 Agent。
- 缓存通知未读数，减少右上角未读红点的频繁查库。
- 提供短期分布式锁，避免多实例或重复触发时并发执行同一类后台任务。

Redis 不保存以下数据：

- DNS 区域、DNS 记录、DHCP 作用域、DHCP 租约和 DHCP 保留地址快照。
- 用户、会话、角色、审计记录、刷新任务历史和通知历史。
- API Key、密码、SMTP 密码、LDAP 密码、验证码等敏感信息。

后端启动时会连接 Redis。连接失败时，后端会终止启动，避免刷新事件、SSE 和运行态协调链路处于不完整状态。

## 二、环境变量

|           变量            | 默认值            | 说明                                                         |
| :-----------------------: | :---------------: | :----------------------------------------------------------: |
|       `REDIS_ADDR`        | `localhost:6379`  | Redis 地址                                                   |
|     `REDIS_PASSWORD`      | 空                | Redis 密码                                                   |
|        `REDIS_DB`         | `0`               | Redis 数据库编号                                             |
|   `METRIC_STREAM_MAXLEN`  | `10000`           | Redis 刷新事件 Stream 最大保留长度                           |
| `RUNTIME_DNS_DEEP_SYNC_INTERVAL` | `1d`            | DNS 定时全量同步间隔；启用后会使用 Redis 锁避免多实例重复排队    |
| `RUNTIME_DHCP_DEEP_SYNC_INTERVAL` | `1h`            | DHCP 定时全量同步间隔；启用后会使用 Redis 锁避免多实例重复排队    |

- 运行态刷新事件最近值的 TTL 由后端固定为 `2` 分钟。

- 刷新任务运行态缓存和 Agent 健康缓存 TTL 固定为 `30` 分钟。

- Agent 同步运行态标记 TTL 为 Agent 连接超时加 Agent 全量同步超时时间，再加 `1` 分钟，任务结束后会主动删除。

- 通知未读数缓存 TTL 固定为 `1` 分钟。


## 三、当前 Redis key 总览

### 3.1 刷新事件

| key / channel | 类型 | 过期策略 | 写入时机 | 读取时机 |
| :-----------: | :--: | :------: | :------: | :------: |
| `zonelease:refresh-events` | Pub/Sub channel | 无持久化 | 每次发布刷新事件时 `PUBLISH` | `/api/events` SSE 订阅后实时接收 |
| `zonelease:refresh:last` | String，JSON | 2 分钟 TTL | 每次发布刷新事件时 `SET` | 保留最近刷新事件，便于排查 |
| `zonelease:refresh:stream` | Stream | 无 TTL，按最大长度裁剪 | 每次发布刷新事件时 `XADD` | SSE 新连接建立后回放最近 20 条事件 |

- `zonelease:refresh-events` 是 Pub/Sub channel，不是普通 key，因此不会出现在 `KEYS *` 输出中。
- `zonelease:refresh:stream` 使用近似裁剪，最大长度由 `METRIC_STREAM_MAXLEN` 控制。默认保留约 `10000` 条刷新事件。

刷新事件 JSON 字段：

- `type`：事件类型，例如 `runtime.refresh.all`、`runtime.refresh.dns.all`、`runtime.refresh.dhcp.all`、`runtime.refresh.server`、`runtime.refresh.dns.zone`、`runtime.refresh.dhcp.scope` 或 `runtime.updated`。
- `taskId`：刷新任务 ID。
- `status`：任务状态，例如 `queued`、`running`、`progress`、`success` 或 `failed`。
- `message`：前端可展示的中文提示。
- `payload`：可选的任务进度详情。
- `createdAt`：事件创建时间。

### 3.2 刷新任务运行态缓存

| key | 类型 | 过期策略 | 写入时机 | 读取时机 |
| :-: | :--: | :------: | -------: | :------: |
| `zonelease:runtime:refresh-task:<taskId>` | String，JSON | 30 分钟 TTL | 刷新任务状态或 payload 更新时 | `GET /api/refresh/tasks` 读取 queued/running 任务时 |

缓存内容：

```json
{
  "status": "running",
  "payload": {
    "message": "正在同步 Agent",
    "totalAgents": 3,
    "startedAgents": 1,
    "syncedAgents": 0,
    "failedAgents": 0,
    "skippedAgents": 0,
    "warn": ""
  }
}
```

该缓存只用于补齐运行中任务的最新状态。刷新任务历史仍以 PostgreSQL `refresh_tasks` 表为准。

### 3.3 Agent 健康检查运行态缓存

| key | 类型 | 过期策略 | 写入时机 | 读取时机 |
| :-: | :--: | :------: | :------: | :------: |
| `zonelease:runtime:agent-health:<serverId>` | String，JSON | 30 分钟 TTL | 自动健康检查、手动测试连接、同步任务健康检查后；删除 Agent 时清理 | `GET /api/state` 读取服务器状态时 |
| `zonelease:runtime:agent-sync:<serverId>` | String，JSON | Agent 连接超时加 Agent 全量同步超时时间，再加 1 分钟，任务结束后主动删除 | 指定 Agent 同步任务开始时写入，结束时删除 | 自动健康检查、DNS / DHCP 管理操作、手动 DNS 区域 / DHCP 作用域刷新和单 Agent 同步执行前读取 |

缓存内容：

```json
{
  "serverId": "037d0f4f-3015-ae50-a740-198442d7b9e9",
  "serverName": "DNS-01",
  "status": "Online",
  "error": "",
  "failureCount": 0,
  "durationMillis": 35,
  "checkedAt": "2026-06-24T10:00:00Z"
}
```

仪表板服务器状态仍从 PostgreSQL 读取服务器基础信息。Redis 只补齐最近健康检查产生的连续失败次数等运行态字段。

Agent 同步运行态标记只表示该 Agent 当前有同步任务正在执行，不作为任务历史。自动健康检查会跳过正在同步的 Agent；DNS / DHCP 管理操作、手动 DNS 区域 / DHCP 作用域刷新和单 Agent 同步会读取该标记并提示“当前 Agent 正在同步，请稍后再操作”。任务详情仍以 PostgreSQL `refresh_tasks` 和 `zonelease:runtime:refresh-task:<taskId>` 为准。

### 3.4 通知未读数缓存

| key | 类型 | 过期策略 | 写入时机 | 读取时机 |
| :-: | :--: | :------: | :------: | :------: |
| `zonelease:runtime:notifications:unread-count` | String，整数 | 1 分钟 TTL | 未读数接口查库后 | `GET /api/notifications/unread-count` |

以下行为会主动删除该缓存：

- 创建 Agent 离线通知。
- Agent 恢复在线并清理离线通知。
- 创建 PostgreSQL / Redis 平台服务异常通知。
- PostgreSQL / Redis 恢复在线并清理平台服务异常通知。
- 标记单条通知已读。
- 标记全部通知已读。
- 清空通知中心。

缓存删除后，下一次读取未读数会重新查询 PostgreSQL 并写回 Redis。

### 3.5 短期分布式锁

| key | 类型 | 过期策略 | 用途 |
| :-: | :--: | :------: | :--: |
| `zonelease:lock:agent-health-check` | String，随机 token | Agent 连接超时乘以检查目标数量，再加 1 分钟 | 避免多实例同时执行自动健康检查 |
| `zonelease:lock:refresh:scheduled-dns` | String，随机 token | Agent 连接超时加 Agent 全量同步超时，再加 1 分钟 | 避免多实例同时创建 DNS 定时全量刷新任务 |
| `zonelease:lock:refresh:scheduled-dhcp` | String，随机 token | Agent 连接超时加 Agent 全量同步超时，再加 1 分钟 | 避免多实例同时创建 DHCP 定时全量刷新任务 |
| `zonelease:lock:refresh:all` | String，随机 token | Agent 连接超时加 Agent 全量同步超时，再加 1 分钟 | 避免全量刷新任务并发执行 |
| `zonelease:lock:refresh:dns-all` | String，随机 token | Agent 连接超时加 Agent 全量同步超时，再加 1 分钟 | 避免 DNS 全量刷新任务并发执行 |
| `zonelease:lock:refresh:dhcp-all` | String，随机 token | Agent 连接超时加 Agent 全量同步超时，再加 1 分钟 | 避免 DHCP 全量刷新任务并发执行 |
| `zonelease:lock:agent-sync:<serverId>` | String，随机 token | Agent 连接超时加 Agent 全量同步超时时间，再加 1 分钟 | 避免全量同步和单 Agent 同步并发访问同一 Agent |
| `zonelease:lock:refresh:server:<serverId>` | String，随机 token | Agent 连接超时加 Agent 全量同步超时，再加 1 分钟 | 避免同一 Agent 同步任务并发执行 |
| `zonelease:lock:refresh:dns-zone:<serverId>:<zoneName>` | String，随机 token | Agent 连接超时加 Agent 全量同步超时，再加 1 分钟 | 避免同一 DNS 区域刷新任务并发执行 |
| `zonelease:lock:refresh:dhcp-scope:<serverId>:<scopeExternalId>` | String，随机 token | Agent 连接超时加 Agent 全量同步超时，再加 1 分钟 | 避免同一 DHCP 作用域刷新任务并发执行 |
| `zonelease:lock:operation-refresh:<target>` | String，随机 token | 操作后刷新等待时间加 1 分钟 | 避免 DNS / DHCP 操作后的延迟刷新被多实例重复排队 |

锁释放使用 Lua 脚本校验 token 后删除，避免误删其他实例持有的锁。

锁获取失败时：

- 自动健康检查会跳过本轮执行。
- 定时全量刷新会跳过本轮排队。
- 重复刷新任务会删除本次重复任务记录，不写入任务日志；全量刷新、单 Agent 同步、DNS 区域刷新和 DHCP 作用域刷新常规重复请求会优先在创建任务前被拒绝。
- DNS / DHCP 管理操作会先检查目标 Agent 是否正在同步，正在同步时返回当前 Agent 正在同步，请稍后再操作。
- 操作后延迟刷新会跳过重复排队；如果同目标手动刷新正在运行，或目标 Agent 正在同步，也会跳过本次延迟刷新排队。

## 四、截图中 key 的来源说明

截图里执行的是：

```text
keys '*'
```

这些 key 的显示含义如下：

| 示例 key | 含义 | 为什么会出现 |
| :------: | :--: | :----------: |
| `zonelease:runtime:agent-health:037d0f4f-3015-ae50-a740-198442d7b9e9` | 某个 Agent 的最近健康检查缓存 | 自动健康检查、手动测试连接或同步任务访问 `/health` 后写入；删除 Agent 后清理 |
| `zonelease:runtime:agent-sync:037d0f4f-3015-ae50-a740-198442d7b9e9` | 某个 Agent 正在同步的运行态标记 | 单 Agent 同步或全量刷新同步到该 Agent 时写入；任务结束后删除，异常残留时等待 TTL 到期 |
| `zonelease:runtime:refresh-task:0cee9290-06b6-4f18-92b4-c53e90823a42` | 某个刷新任务的运行态缓存 | 刷新任务进入 queued/running/completed/failed 或 payload 更新后写入 |
| `zonelease:refresh:stream` | 刷新事件 Stream | 后端发布刷新事件时写入，用于 SSE 断线回放 |
| `zonelease:runtime:notifications:unread-count` | 通知中心未读数缓存 | 前端读取未读数接口后写入 |
| `zonelease:refresh:last` | 最近一次刷新事件 | 后端发布刷新事件时写入，2 分钟后自动过期 |

截图中没有看到 `zonelease:lock:*`，通常是因为锁只在任务执行期间短暂存在，任务结束后会释放，或者 TTL 到期后自动消失。

截图中没有看到 `zonelease:refresh-events`，因为它是 Pub/Sub channel，不是可枚举的普通 key。

## 五、运维查看命令

查看 key 类型：

```bash
TYPE zonelease:refresh:stream
TYPE zonelease:refresh:last
TYPE zonelease:runtime:notifications:unread-count
```

查看 key 剩余过期时间：

```bash
TTL zonelease:refresh:last
TTL zonelease:runtime:notifications:unread-count
TTL zonelease:runtime:agent-health:<serverId>
TTL zonelease:runtime:agent-sync:<serverId>
TTL zonelease:runtime:refresh-task:<taskId>
```

查看普通 String 内容：

```bash
GET zonelease:refresh:last
GET zonelease:runtime:notifications:unread-count
GET zonelease:runtime:agent-health:<serverId>
GET zonelease:runtime:agent-sync:<serverId>
GET zonelease:runtime:refresh-task:<taskId>
```

查看 Stream 长度和最近事件：

```bash
XLEN zonelease:refresh:stream
XREVRANGE zonelease:refresh:stream + - COUNT 5
```

查看 Pub/Sub channel：

```bash
PUBSUB CHANNELS zonelease:*
```

生产环境排查时优先使用 `SCAN`，避免 `KEYS *` 在 key 很多时阻塞 Redis：

```bash
SCAN 0 MATCH zonelease:* COUNT 100
```

## 六、清理与恢复

Redis 中的运行态 key 可以被清理，清理后系统会按下一次事件或接口请求重新生成：

- 删除 `zonelease:runtime:notifications:unread-count` 后，下一次未读数接口会重新查库并缓存。
- 删除 `zonelease:runtime:agent-health:<serverId>` 后，下一次状态接口只使用 PostgreSQL 中的服务器状态；后续健康检查会重新写入缓存。
- 删除 `zonelease:runtime:agent-sync:<serverId>` 后，自动健康检查可能在同步任务尚未结束时检查同一 Agent；一般不建议手动删除，异常残留时优先等待 TTL 到期。
- 删除 `zonelease:runtime:refresh-task:<taskId>` 后，任务列表仍能读取 PostgreSQL 中的任务记录，只是运行中 payload 可能不再有 Redis 补偿。
- 删除 `zonelease:refresh:stream` 后，不影响后续新事件发布，但 SSE 新连接无法回放删除前的事件。
- 删除 `zonelease:refresh:last` 后，不影响实时刷新事件推送。

不建议在任务运行中手动删除 `zonelease:lock:*`。如果确认锁因异常残留，优先等待 TTL 到期；只有确认没有实例仍在执行对应任务时，才手动清理。

## 七、后续变更同步要求

涉及以下变更时，必须同步更新本文档：

- 新增、删除或重命名 Redis key。
- 修改 Redis key 的数据类型、TTL、payload 字段或写入时机。
- 修改 Redis Stream 最大长度、回放条数或 SSE 事件发送顺序。
- 修改 Pub/Sub channel 名称或事件格式。
- 修改 Redis 锁 key、锁 TTL、重复任务处理语义或释放逻辑。
- 新增将业务事实源迁移到 Redis 的设计。
