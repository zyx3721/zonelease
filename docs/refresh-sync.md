# 刷新与同步链路说明

本文档说明 ZoneLease 中所有刷新入口的触发方式、刷新范围、数据库快照、Redis/SSE 事件和环境变量配置。

## 一、刷新概念边界

ZoneLease 当前采用“数据库快照展示 + 后台同步更新”的模式：

- 前端页面刷新只读取 PostgreSQL。
- 后端同步任务负责访问 DNS / DHCP Agent。
- Redis 负责发布刷新事件、保存最近刷新事件、回放最近刷新事件、缓存短期运行态数据和提供短期分布式锁。
- SSE 负责通知前端重新读取数据库快照。

需要注意：浏览器刷新页面不是运行态同步，不会触发 Agent 采集。

## 二、刷新入口总览

|         入口         |                   触发位置                   |           任务类型           |      刷新范围      | 是否访问 Agent |
| :------------------: | :------------------------------------------: | :--------------------------: | :----------------: | :------------: |
| 页面打开或浏览器刷新 |                 前端路由加载                 |              无              |   读取数据库快照   |       否       |
|   顶部全量刷新按钮   |                 主布局右上角                 |    `runtime.refresh.all`     |  所有可同步 Agent  |       是       |
|   DNS 定时全量刷新   |                  后端调度器                  |  `runtime.refresh.dns.all`   |  所有可同步 DNS Agent   |       是       |
|  DHCP 定时全量刷新   |                  后端调度器                  |  `runtime.refresh.dhcp.all`  |  所有可同步 DHCP Agent  |       是       |
|    服务器连接测试    |                  Agent 管理                  |          无独立任务          | 当前服务器健康状态 |       是       |
|      同步 Agent      | Agent 管理、DNS/DHCP 页面当前 Agent 刷新按钮 |   `runtime.refresh.server`   |     当前服务器     |       是       |
|    DNS 区域新增后    |              DNS 管理操作延迟合并              |  `runtime.refresh.dns.zone`  | 新增 DNS 区域记录  |       是       |
|   DNS 区域刷新按钮   |                 DNS 区域卡片                 |  `runtime.refresh.dns.zone`  | 当前 DNS 区域记录  |       是       |
| DNS 记录新增/编辑/删除后 |              DNS 管理操作延迟合并              |  `runtime.refresh.dns.zone`  | 当前 DNS 区域记录  |       是       |
| DHCP 作用域刷新按钮  |             DHCP 作用域详情卡片              | `runtime.refresh.dhcp.scope` |  当前 DHCP 作用域  |       是       |
|    DHCP 管理操作     |              DHCP 管理页面延迟合并             | `runtime.refresh.dhcp.scope` |  当前 DHCP 作用域  |       是       |

## 三、页面数据读取

前端数据读取入口：

- `frontend/src/lib/dns-dhcp-store.ts`

主要接口：

- `GET /api/state`
- `GET /api/state?includeDns=true`

当前 `includeDns=true` 是兼容参数。无论是否携带该参数，后端都只读取 PostgreSQL 快照，不会实时访问 DNS Agent。

页面收到刷新事件后，会触发浏览器内事件并重新读取状态：

- `emitZoneLeaseRefresh()`
- `onZoneLeaseRefresh(callback)`

## 四、手动全量刷新

顶部工具栏刷新按钮调用：

```text
POST /api/refresh
```

请求体为空或 `type` 为空时，后端默认创建：

```text
runtime.refresh.all
```

后端执行流程：

1. 创建 `refresh_tasks` 记录，状态为 `queued`。
2. 发布 `runtime.refresh.all` 排队事件。
3. 后台 goroutine 执行同步任务。
4. 任务状态更新为 `running`。
5. 遍历所有可同步 Agent。可同步 Agent 指已登记、配置了 Agent URL，且角色匹配本次刷新类型的服务器。
6. 每开始同步一个可同步 Agent 时更新 `refresh_tasks.payload` 并发布带 `agentEvent.status=running` 的 `progress` 事件。
7. 每完成、失败或跳过一个可同步 Agent 后更新 `refresh_tasks.payload` 中的成功 / 失败 / 跳过数量，并发布带 `agentEvent.status=completed`、`agentEvent.status=failed` 或 `agentEvent.status=skipped` 的 `progress` 事件。
8. DNS 角色服务器执行 DNS 同步。
9. DHCP 角色服务器执行 DHCP 同步。
10. 成功后任务状态更新为 `completed`，payload 保留最终进度快照，并发布 `runtime.updated` 让前端重新读取数据库快照。
11. 失败时任务状态更新为 `failed`，payload 保留最终进度快照并写入错误信息；失败路径只发布对应刷新任务的 `failed` 事件，不额外发布 `runtime.updated`。
12. 刷新任务排队、完成或失败不写入 `notifications`，仅通过任务日志、SSE 状态和 toast 提示展示。

全量刷新进度 payload 字段：

- `message`：当前任务提示文案；完成后格式为 `刷新已完成 syncedAgents/totalAgents，异常 failedAgents`；如果存在跳过 Agent，追加 `，跳过 skippedAgents`；无可同步 Agent 时为 `暂无可同步的 Agent`。
- `warn`：全量刷新中被跳过 Agent 的警告信息汇总；没有跳过时不写入。
- `startedAt`：任务开始时间，使用后端服务本地时间，格式为 `YYYY-MM-DD HH:mm:ss`。
- `finishedAt`：任务结束时间，使用后端服务本地时间，格式为 `YYYY-MM-DD HH:mm:ss`。
- `totalAgents`：本次刷新范围内的可同步 Agent 数量；可同步 Agent 指已登记、配置了 Agent URL，且角色匹配本次刷新类型的服务器。
- `startedAgents`：已开始同步的服务器数量，前端用它显示 `[当前/总数]` 进度。
- `syncedAgents`：已同步成功的服务器数量。
- `skippedAgents`：因目标 Agent 已在其他同步任务中而跳过的服务器数量。
- `failedAgents`：已同步失败的服务器数量。
- `resourceType`：全量刷新固定为 `runtime`，用于任务详情和导出时保持目标语义稳定。
- `agentResults`：每个 Agent 的 `id`、`name`、`status`，失败时写入 `error`，跳过时写入 `warn`。
- `currentAgent`：当前开始同步的服务器名称，仅在 Agent 开始运行的 `progress` 事件中保留兼容展示。
- `agentEvent`：当前发生状态变化的 Agent，包含 `id`、`name`、`status`，失败时写入 `error`，跳过时写入 `warn`。`status` 可为 `running`、`completed`、`failed` 或 `skipped`，前端用它维护单 Agent 独立 toast。

全量刷新完成或失败后仍会保留上述聚合字段，任务审计页面可继续显示 `syncedAgents/totalAgents` 和失败数量。

局部刷新任务在运行、完成或失败状态下会保留目标字段：

- Agent 同步任务保留 `message`、`resourceType=server`、`resourceId=<serverId>`、`resourceName=<agentName>`、`serverId` 和 `serverName`。
- DNS 区域刷新任务按顺序展示 `message`、`resourceType=dns.zone`、`resourceId=<zoneId>`、`resourceName=<zoneName>`、`serverId` 和 `serverName`。
- DHCP 作用域刷新任务按顺序展示 `message`、`resourceType=dhcp.scope`、`resourceId=<scopeId>`、`resourceName=<scopeName>`、`serverId` 和 `serverName`。
- 局部刷新任务的 `resourceId` 保存数据库资源 ID，前端任务目标列、搜索和导出优先使用 `resourceName` 展示可读名称。

前端显示规则：

- 手动点击顶部全量刷新按钮时，接口排队成功后显示 toast。
- 手动全量刷新期间，每个开始同步的 Agent 会显示独立 loading toast。该 Agent 完成、失败或跳过后，同一 toast 更新为成功、失败或警告提示，并在 3 秒后自动隐藏。
- 如果全量刷新中的某个 Agent 已处于其他同步任务中，该 Agent 会标记为 `skipped`，前端提示 `<Agent> 已处于正在同步中，跳过同步`，不按失败处理。
- 全部 Agent 同步结束后，前端会额外显示独立总结 toast；成功时显示 `[总数/总数] 所有 Agent 已同步完成`，存在失败时显示 `[已完成/总数] 全量同步完成，异常 N`，仅存在跳过时显示 `[已完成/总数] 全量同步完成，跳过 N`。
- 手动点击 DNS / DHCP 页面标题行右侧的当前 Agent 刷新按钮时，前端会轮询对应刷新任务状态。任务执行期间刷新图标持续旋转，并显示固定 toast；任务完成后 toast 更新为完成提示并在 3 秒后自动隐藏。
- 顶部全量刷新按钮在任务未结束前保持旋转，任务完成或失败后恢复静态。
- 全量刷新不再显示顶部同步状态卡片，只通过排队 toast、单 Agent toast 和最终总结 toast 展示进度与收尾状态。
- 前端收到 SSE 后只更新页面数据和同步状态，不再弹出“刷新事件已同步”类 toast。
- 任务详情和导出会按任务类型归一 payload 字段顺序；由于 PostgreSQL `JSONB` 不承诺保留对象字段原始顺序，界面不直接依赖数据库返回的 key 顺序。

全量同步以 Agent 返回快照为准：

- DNS 同步会删除同服务器下 Agent 不再返回的旧区域。
- DNS 区域记录同步会整区替换旧记录。
- DHCP 同步通过 `GET /dhcp/scopes` 读取作用域列表。
- legacy DHCP Agent 下，作用域基础信息、保留地址、排除范围、地址范围、状态和租期来自一次全局 `netsh dhcp server dump` 缓存。
- legacy DHCP Agent 下，只有租约列表继续按 DHCP 作用域并发逐作用域执行 `show clients 1`。
- legacy DHCP Agent 下，后端仅在全量 DHCP 同步结束后调用 `/dhcp/cache/clear` 清理本次全局 dump 缓存。
- DHCP 同步会删除同服务器下 Agent 不再返回的旧作用域。
- DHCP 同步会删除同作用域下 Agent 不再返回的旧排除范围、租约和保留地址。

主要实现位置：

- 后端刷新接口：`backend/api/router/refresh.go`
- 后端同步服务：`backend/internal/service/sync/service.go`
- 刷新任务仓储：`backend/internal/repository/refresh.go`
- 前端全量刷新按钮：`frontend/src/components/app-layout.tsx`
- 任务 / 审计页面：`frontend/src/routes/_authenticated/audit.tsx`

任务 / 审计页面读取规则：

- 页面默认通过 `GET /api/refresh/tasks?limit=all` 读取刷新任务列表，界面初始显示 50 条。
- 显示数量可切换为 30、50、100、200 或全部，只控制前端当前列表分页，不会创建新的刷新任务。
- 点击导出任务时，会先通过 `GET /api/refresh/tasks?limit=all` 读取可导出的任务数据，再按当前筛选条件生成文件。
- 任务导出字段包含任务 ID、类型、状态、目标、进度、错误信息、警告信息、创建时间、完成时间和载荷。

## 五、定时全量刷新

后端启动后会根据环境变量启动定时全量刷新：

```env
RUNTIME_DNS_DEEP_SYNC_INTERVAL=1d
RUNTIME_DHCP_DEEP_SYNC_INTERVAL=1h
```

规则：

- `RUNTIME_DNS_DEEP_SYNC_INTERVAL` 默认 `1d`，按本地自然日边界创建 `runtime.refresh.dns.all` 任务。
- `RUNTIME_DHCP_DEEP_SYNC_INTERVAL` 默认 `1h`，按本地整点边界创建 `runtime.refresh.dhcp.all` 任务。
- 支持 `m`、`h`、`d` 单位，例如 `30m`、`2h`、`1d`。
- 能整除 24 小时的短间隔会按本地当天零点对齐，例如 `1h` 对齐整点、`30m` 对齐半小时。
- 按天配置的间隔会按本地自然日零点对齐，例如 `1d` 对齐次日零点，`2d` 对齐从当前本地日期零点起算的下一个两天边界。
- 不能整除 24 小时且不是整天数的间隔按固定周期对齐；固定周期以 Unix epoch `1970-01-01 00:00:00 UTC` 为起点，不以服务启动时间为起点。
- 值为 `0` 时关闭对应角色的定时全量同步。
- DNS / DHCP 定时任务分别只同步对应角色 Agent。
- 不会在服务启动瞬间立即执行。
- 定时任务会写入任务日志，不写入操作审计。
- 定时刷新不会触发前端 toast；页面收到 SSE 后只重新读取数据库快照。

## 六、DNS 区域刷新

DNS 区域卡片右侧刷新按钮调用：

```text
POST /api/dns/zones/{id}/refresh
```

后端执行流程：

1. 根据 `{id}` 查询 `dns_zones`。
2. 检查目标 Agent 是否正在同步，以及同一 DNS 区域是否已有刷新任务正在运行。
3. 检查通过后创建 `runtime.refresh.dns.zone` 任务；如果目标 Agent 正在同步或同目标刷新正在运行，则返回冲突提示且不创建任务。
4. 发布区域刷新排队事件。
5. 后台调用对应 DNS Agent：

```text
POST /dns/records/query
```

6. 用 Agent 返回结果整区替换 `dns_records` 中该区域记录。
7. 发布 `runtime.updated`。

后端会把区域名放在 JSON body 的 `zone` 字段中，避免特殊区域名出现在 HTTP URL 路径里被网络安全设备误拦截。以下情况会回退到 `GET /dns/zones/{zone}/records` 兼容旧版本：

- 目标 Agent 不支持 body 版接口并返回 `404`。
- Windows Server 2008/2008 R2 legacy Agent 缺少 `.NET System.Web.Extensions`，导致 JSON body 解析返回 `500`。

该刷新不会遍历所有服务器，也不会刷新其他区域。

## 七、Agent 管理测试与同步

Agent 管理中点击「测试连接」会访问 Agent `/health`，并按角色继续探测 DNS `/dns/zones` 或 DHCP `/dhcp/probe`：

```text
POST /api/servers/{id}/ping
```

后端执行流程：

1. 调用服务器 Agent `/health`。
2. 成功则更新服务器状态为 `Online`。
3. 手动测试失败则更新服务器状态为 `Offline`，响应 `detail` 包含失败原因。
4. 写入 `Checked server health` 审计记录。

该入口只测试连接，不执行资源同步。

后端后台自动健康检查按「系统配置 / 基础配置 / Agent 判定」中的连通性检查间隔执行，不依赖仪表板页面是否打开。该检查只针对已配置 Agent URL 且当前未处于同步中的 Agent 调用 `/health`，并按自动检查并发配置同时检查多个 Agent，更新服务器状态和最近检查时间；只有连续失败达到离线判定次数、服务器状态实际写为 `Offline` 时，才创建 Agent 离线通知。

自动健康检查不写入 `Checked server health` 审计记录，也不执行 DNS / DHCP 资源同步。

Agent 管理中点击「同步 Agent」：

```text
POST /api/servers/{id}/sync
```

后端执行流程：

1. 查询服务器登记信息。
2. 创建 `runtime.refresh.server` 任务。
3. 发布服务器同步排队事件。
4. 后台同步当前服务器角色对应的数据。
5. 同步完成后发布 `runtime.updated`。
6. 写入 `Queued server sync` 审计记录。

DNS / DHCP 管理页标题行右侧的当前 Agent 刷新按钮复用同一接口，只刷新当前选择的 Agent。前端会等待 `refresh_tasks` 中对应任务进入 `completed` 或 `failed` 后再结束按钮旋转和固定 toast。

如果指定 Agent 已处于同步任务中，后端会在创建任务前返回 `409 agent_sync_running`，提示 `当前 Agent 正在同步，请稍后再操作`；该预检查失败不创建 `runtime.refresh.server` 任务，也不写入 `Queued server sync` 审计。

手动单 Agent 同步包含 Agent 管理中的「同步 Agent」和 DNS / DHCP 管理页右上角当前 Agent 刷新按钮。同步前 `/health` 预检查失败时，后端会使用 Agent 连接超时的用户友好错误写入任务失败原因，并立即将该 Agent 标记为 `Offline`、刷新健康缓存和创建离线通知。

Agent 管理中新增 Agent 时按以下流程处理：

- 后端先检查 Agent 名称和接口地址是否已存在；重复时返回 `409`，提示 `Agent 名称已存在` 或 `Agent 接口地址已存在`。
- 保存接口不重复访问 Agent；前端会要求当前 Agent 地址、角色、API Key 和 TLS 设置先通过 `POST /api/servers/probe` 测试。
- 名称和接口地址唯一性通过后，后端写入服务器登记，并以 `Online` 状态返回。
- 前端保存成功后使用固定 toast 显示 `<Agent> 已保存，开始同步`。
- 随后排队单 Agent 同步任务，并通过 `skipHealthCheck=1` 跳过首次自动同步前 `/health` 预检查。
- 单 Agent 同步任务排队时会立即写入 Agent 同步运行态标记，避免后台自动健康检查在任务尚未真正运行前抢先检查同一 Agent。
- 任务完成后同一 toast 更新为 `<Agent> 同步完成`。
- 前端重新读取服务器与 DNS/DHCP 快照。

DNS 正向区域局部刷新时，后端会按 A 记录 IP 推导可能关联的反向区域；只有该反向区域已存在于数据库快照中时，才会同步其记录，用于反推 A 记录的 `create_ptr` 标记并更新反向区域快照。

## 八、DNS / DHCP 管理操作后同步

DNS / DHCP 管理操作会先访问对应 Agent 执行真实 Windows DNS / DHCP 变更。Agent 调用成功后，后端不会立即为每次操作都创建刷新任务，而是按目标做延迟合并：

- DNS 按 `serverId + zoneName` 合并。
- DHCP 按 `serverId + scopeExternalId` 合并。
- 默认等待窗口为 `10` 秒。
- 等待窗口由「系统配置 / 基础配置 / 同步参数 / 操作后刷新等待」控制，可配置 `1` 到 `60` 秒。
- 等待窗口期间尚未创建刷新任务；同一目标在等待窗口内又发生新操作时，会取消原计时并重新等待。
- 如果同一目标仍有操作正在执行，后端会等操作结束后再开始等待窗口。
- 手动点击 DNS 区域刷新按钮或 DHCP 作用域刷新按钮不走等待窗口；目标 Agent 未同步且同目标刷新未运行时会立即创建刷新任务。
- 手动点击同一 DNS 区域刷新按钮或 DHCP 作用域刷新按钮时，如果同目标刷新任务正在执行，后端会返回 `409 refresh_target_running`，不创建重复任务。
- 延迟刷新到点后，如果目标 Agent 正在执行全量同步、自动全量同步或单 Agent 同步，后端会跳过本次延迟局部刷新，不创建 `runtime.refresh.dns.zone` 或 `runtime.refresh.dhcp.scope` 任务。

这样可以让多用户连续修改同一区域或作用域时只产生一批局部同步任务，降低 Agent 和数据库压力。

DHCP 页面上的创建作用域、编辑作用域、启停作用域、删除作用域、创建排除范围、删除排除范围、释放租约、从租约行添加到保留地址、编辑保留地址和删除保留地址都会先访问对应 DHCP Agent。

成功后处理规则：

- 作用域刷新按钮在目标 Agent 未同步且同目标刷新未运行时立即创建 `runtime.refresh.dhcp.scope` 任务；如果目标 Agent 正在同步或同目标刷新正在运行，则返回冲突提示且不创建任务。
- 作用域创建、编辑、启停、排除范围创建和删除、释放租约、保留地址创建、编辑和删除会在常规路径下按作用域延迟合并创建 `runtime.refresh.dhcp.scope` 任务。
- `runtime.refresh.dhcp.scope` 任务只同步当前作用域，不会遍历其他服务器或其他作用域。
- 作用域删除会删除数据库中对应作用域及其租约、保留地址快照。
- 后端发布 `runtime.updated`，前端收到事件后重新读取数据库快照。

失败处理规则：

- Agent 调用失败时不修改数据库。
- 已存在数据库快照的 DHCP 作用域相关操作在 Agent 调用失败时，也会标记当前作用域需要延迟局部刷新，用于收敛 legacy `netsh -f` 批处理可能已经部分生效的真实状态。
- Agent 成功但局部同步失败时，会尽量按操作结果清理对应本地快照，并依赖下一次全量刷新最终收敛。

## 九、Redis 与 SSE

Redis 当前用途：

- Pub/Sub channel：`zonelease:refresh-events`
- 最近刷新事件 key：`zonelease:refresh:last`
- Redis Stream：`zonelease:refresh:stream`
  - 每次发布刷新事件时同步写入。
  - 新 SSE 连接建立后会回放最近 20 条刷新事件，用于浏览器断线重连后补齐短时间内错过的刷新状态。
  - Stream 最大保留长度由 `METRIC_STREAM_MAXLEN` 控制，默认 `10000`。
- 短期运行态缓存 key：
  - `zonelease:runtime:refresh-task:<taskId>`：缓存刷新任务运行中状态和 payload，TTL 为 30 分钟；`GET /api/refresh/tasks` 读取 queued/running 任务时会用该缓存补齐最新进度。
  - `zonelease:runtime:agent-health:<serverId>`：缓存最近一次 Agent 健康检查状态、错误、连续失败次数、耗时和检查时间，TTL 为 30 分钟；`GET /api/state` 读取服务器状态时会用该缓存补齐失败计数。
  - `zonelease:runtime:agent-sync:<serverId>`：标记指定 Agent 正在执行全量同步、自动全量同步或单 Agent 同步，TTL 为 Agent 连接超时加 Agent 全量同步超时时间，再加 1 分钟；自动健康检查读取该 key 后会跳过正在同步的 Agent，DNS / DHCP 管理操作、手动 DNS 区域 / DHCP 作用域刷新或单 Agent 同步读取该 key 后会提示当前 Agent 正在同步并拒绝本次操作。
  - `zonelease:runtime:notifications:unread-count`：缓存通知中心未读数量，TTL 为 1 分钟；通知创建、恢复清理、标记已读和清空时会主动失效。
- 短期锁 key：
  - `zonelease:lock:agent-health-check`：避免多实例同时执行自动健康检查。
  - `zonelease:lock:refresh:scheduled-dns`：避免多实例同时创建 DNS 定时全量刷新任务。
  - `zonelease:lock:refresh:scheduled-dhcp`：避免多实例同时创建 DHCP 定时全量刷新任务。
  - `zonelease:lock:refresh:all`：避免全量刷新任务并发执行。
  - `zonelease:lock:refresh:dns-all`：避免 DNS 全量刷新任务并发执行。
  - `zonelease:lock:refresh:dhcp-all`：避免 DHCP 全量刷新任务并发执行。
  - `zonelease:lock:agent-sync:<serverId>`：避免手动全量同步、自动全量同步和单 Agent 同步并发访问同一 Agent。
  - `zonelease:lock:refresh:server:<serverId>`：避免同一 Agent 同步任务并发执行。
  - `zonelease:lock:refresh:dns-zone:<serverId>:<zoneName>`：避免同一 DNS 区域刷新任务并发执行。
  - `zonelease:lock:refresh:dhcp-scope:<serverId>:<scopeExternalId>`：避免同一 DHCP 作用域刷新任务并发执行。
  - `zonelease:lock:operation-refresh:<target>`：避免操作后延迟刷新被多实例重复排队。

Redis 仅用于短期运行态协调、最近事件回放和读性能优化，不承载 DNS / DHCP 快照、审计日志或通知历史。全量刷新、单 Agent 同步、DNS 区域刷新和 DHCP 作用域刷新会优先在创建任务前做运行中检查，重复请求不创建任务日志；如果极端并发下仍在后台拿锁失败，后端会删除本次重复任务记录，不写入任务日志。DNS / DHCP 管理操作会先检查目标 Agent 是否正在同步，正在同步时返回当前 Agent 正在同步，请稍后再操作。操作后延迟刷新触发时，如果同目标手动刷新正在运行，或目标 Agent 正在同步，会跳过本次延迟刷新排队。

SSE 接口：

```text
GET /api/events
```

前端监听事件：

- `runtime.refresh.all`
- `runtime.refresh.dns.all`
- `runtime.refresh.dhcp.all`
- `runtime.refresh.server`
- `runtime.refresh.dns.zone`
- `runtime.refresh.dhcp.scope`
- `runtime.updated`

收到事件后，前端不会再发起刷新任务，只会重新读取数据库快照。

SSE 建立连接后会先返回 `connected` 事件，再从 Redis Stream 回放最近刷新事件。回放事件和实时 Pub/Sub 事件使用同一事件 `type`，缺省类型为 `runtime.updated`。

Redis key、Stream、Pub/Sub、运行态缓存、分布式锁和运维查看命令的完整说明见 [redis-runtime.md](redis-runtime.md)。

`runtime.refresh.all`、`runtime.refresh.dns.all` 和 `runtime.refresh.dhcp.all` 状态说明：

- `queued`：全量刷新任务已排队。
- `running`：全量刷新任务开始运行。
- `progress`：全量刷新任务服务器级进度发生变化，包括单 Agent 开始、完成或失败。
- `success`：全量刷新任务完成。
- `failed`：全量刷新任务失败。

## 十、环境变量

|             变量             | 默认值  |                               说明                                |
| :--------------------------: | :-----: | :---------------------------------------------------------------: |
| `RUNTIME_DNS_DEEP_SYNC_INTERVAL` |  `1d`   | DNS 定时全量同步间隔，支持 `m`、`h`、`d`，设为 `0` 关闭 |
| `RUNTIME_DHCP_DEEP_SYNC_INTERVAL` |  `1h`   | DHCP 定时全量同步间隔，支持 `m`、`h`、`d`，设为 `0` 关闭 |
|   `METRIC_RETENTION_DAYS`    |  `30`   |                         预留指标保留天数                          |
|     `LOG_RETENTION_DAYS`     |  `30`   | 任务、审计和通知中心日志保留天数；设为小于等于 `0` 时关闭自动清理 |
|    `METRIC_STREAM_MAXLEN`    | `10000` |        Redis 刷新事件 Stream 最大保留长度，用于 SSE 断线回放       |

全量同步服务器级并发、DNS 区域并发、DHCP 作用域详情并发和操作后刷新等待由「系统配置 / 基础配置 / 同步参数」保存到 PostgreSQL 后读取。

- 全量同步并发控制一次全量刷新中同时同步的 Agent 数量。
- DNS 区域并发控制单个 DNS Agent 内同时采集记录的区域数量，可配置 `1` 到 `50` 个。
- DHCP 作用域并发控制单个 DHCP Agent 内同时采集作用域详情的数量；legacy Agent 下主要影响逐作用域 `show clients 1` 租约读取，可配置 `1` 到 `50` 个，默认 `5` 个。

操作后刷新等待用于 DNS / DHCP 管理操作成功后的局部同步防抖：

- 默认值为 `10` 秒。
- 可配置范围为 `1` 到 `60` 秒。
- 同一区域或作用域在等待窗口内没有新的操作时，后端才创建局部刷新任务。
- 如果等待到点时同目标手动刷新正在运行，或目标 Agent 正在执行手动全量同步、自动全量同步或单 Agent 同步，后端会跳过本次延迟刷新，不创建局部刷新任务。

Agent 保存前探测、手动测试、自动健康检查和同步前 `/health` 检查由「系统配置 / 基础配置 / Agent 判定」中的 Agent 连接超时控制。

- 默认值为 `5` 秒。
- 可配置范围为 `1` 到 `20` 秒。

DNS / DHCP 管理写入类操作的整体超时时间由「系统配置 / 基础配置 / Agent 判定」中的 Agent 操作超时控制。

- 默认值为 `20` 秒。
- 可配置范围为 `1` 到 `60` 秒。

全量同步、单 Agent 同步、DNS 区域同步和 DHCP 作用域同步中的资源采集阶段由「系统配置 / 基础配置 / Agent 判定」中的 Agent 全量同步超时控制。

- 默认值为 `300` 秒。
- 可配置范围为 `60` 到 `600` 秒。
- 全量同步和单 Agent 同步执行时会先访问 Agent `/health`；该预检查使用 Agent 连接超时，失败时不会进入资源采集阶段。
- `/health` 通过后，后端会为实际 DNS / DHCP 资源采集重新开启 Agent 全量同步超时窗口。
- 前端等待刷新任务完成的 toast 轮询超时会读取该配置，并额外预留短暂轮询缓冲，避免任务仍在运行时按旧的固定 `120` 秒提前提示等待超时。

Go DNS Agent 内部单次 PowerShell 命令由 `DNS_AGENT_POWERSHELL_TIMEOUT_SECONDS` 控制：

- 默认值为 `180` 秒。
- 可配置范围为 `1` 到 `3600` 秒。
- legacy DNS Agent 不读取该变量，记录枚举继续使用原有 `dnscmd.exe` 直接调用方式。
- 该值小于后端 Agent 全量同步超时时，大区域记录枚举可能先在 Go Agent 内部超时，后端任务会记录读取失败、连接关闭或 Agent 返回错误。

Go DHCP Agent 内部 PowerShell 命令由 `DHCP_AGENT_POWERSHELL_TIMEOUT_SECONDS` 控制：

- 默认值为 `180` 秒。
- 可配置范围为 `1` 到 `3600` 秒。
- 该值小于后端 Agent 全量同步超时时，大作用域租约或保留地址枚举可能先在 Agent 内部超时。

仪表板服务器状态会按「系统配置 / 基础配置 / Agent 判定」中的连通性检查间隔静默检查已配置 Agent URL 且当前未处于同步中的 Agent，并按自动检查并发限制同时检查的 Agent 数量。

- 默认值为 `1` 分钟。
- 检查间隔可配置范围为 `1` 到 `60` 分钟。
- 自动检查并发默认值为 `1` 个，可配置范围为 `1` 到 `20` 个；默认 `1` 个时保持串行检查。

后端未读取到入库配置时使用以下默认值：

- 全量同步并发和 DNS 区域并发：`3`。
- DHCP 作用域并发：`5`。
- 操作后刷新等待：`10` 秒。
- Agent 连接超时：`5` 秒。
- Agent 操作超时：`20` 秒。
- Agent 全量同步超时：`300` 秒。
- 自动连通性检查间隔：`1` 分钟。
- 自动检查并发：`1` 个。

Agent 状态更新规则如下：

- 后台同步和自动连通性检查会先访问 Agent `/health`。
- `/health` 失败会累计 `servers.failure_count`。
- 连续失败次数达到「系统配置 / 基础配置 / Agent 判定」中的 Agent 离线失败次数后，服务器状态才会写为 `Offline`，并在状态从非 `Offline` 进入 `Offline` 时创建一条同源去重的 Agent 离线通知。
- 仪表板或设置页手动测试连接失败会立即写为 `Offline`。
- 健康检查或同步成功会把状态写为 `Online` 并清零失败计数，同时更新 `servers.last_checked`，并自动清理对应 Agent 的未关闭离线通知。
- 删除 Agent 会清理该 Agent 的未关闭离线通知、未读数缓存和最近健康检查运行态缓存。
- 如果 `/health` 成功但后续 DNS / DHCP 资源同步失败，刷新任务会记录失败，不按 Agent 离线失败次数累计。

## 十一、任务与审计

刷新任务写入：

- `refresh_tasks`

任务状态：

- `queued`
- `running`
- `completed`
- `failed`

当前刷新相关审计：

- `Queued refresh`
- `Queued server sync`
- `Queued DNS zone refresh`
- `Queued DHCP scope refresh`
- DNS / DHCP 管理操作成功后会写入对应业务审计，并按目标延迟合并创建局部刷新任务

如果后续新增单服务器刷新、DNS 服务器刷新等入口，应同步设计：

- 任务类型。
- 任务 payload。
- SSE 事件。
- 前端 loading 和禁用状态。
- 审计 action。

## 十二、后续变更同步要求

涉及以下变更时，必须同步更新本文档：

- 新增、删除或修改任何刷新入口。
- 调整 `runtime.refresh.all`、`runtime.refresh.dns.all`、`runtime.refresh.dhcp.all`、`runtime.refresh.dns.zone`、`runtime.refresh.dhcp.scope` 或新增任务类型。
- 修改 Redis key、Pub/Sub channel、SSE 事件名称或发送时机。
- 修改 Redis 锁 key、锁 TTL、重复任务处理语义或操作后延迟刷新防重逻辑。
- 修改前端 `emitZoneLeaseRefresh` / `onZoneLeaseRefresh` 订阅逻辑。
- 修改刷新任务 payload、状态流转或审计 action。
- 修改环境变量语义、默认值或是否启用定时刷新。
- 修改页面刷新是否访问 Agent 的边界。
