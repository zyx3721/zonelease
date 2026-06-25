# DHCP 管理与同步说明

本文档说明 ZoneLease 中 DHCP 作用域、租约和保留地址的展示来源、Agent 交互接口、数据库快照规则、操作链路和后续开发时的文档同步要求。

## 一、整体链路

DHCP 页面默认读取 PostgreSQL 中的 DHCP 快照。后端在手动全量刷新、定时全量刷新、Agent 管理中的同步 Agent，以及 DHCP 管理操作中访问 DHCP Agent。

当前链路如下：

1. 前端 DHCP 页面调用 `/api/state`。
2. 后端从 PostgreSQL 读取 `dhcp_scopes`、`dhcp_leases` 和 `dhcp_reservations`。
3. 手动全量刷新、定时全量刷新、同步当前 Agent 或作用域刷新按钮会访问 DHCP Agent。
4. Go DHCP Agent 通过 Windows PowerShell `DhcpServer` 模块读取作用域、租约和保留地址；Windows Server 2008/2008 R2 legacy Agent 通过 `netsh dhcp server` 提供同一组 HTTP 接口。
5. 后端把 Agent 返回结果写入 PostgreSQL 快照表。
6. 后端通过 Redis Pub/Sub 发布 SSE 事件，前端收到事件后重新读取数据库快照。
7. DHCP 创建、启停、删除、释放租约、保留地址变更和作用域刷新按钮会创建 `runtime.refresh.dhcp.scope` 局部刷新任务。

主要实现位置：

- 后端路由：`backend/api/router/resources.go`
- 后端同步服务：`backend/internal/service/sync/service.go`
- 后端 DHCP 仓储：`backend/internal/repository/resources.go`、`backend/internal/repository/dhcp_sync.go`
- DHCP Agent 路由：`dhcp-agent/internal/server/server.go`
- DHCP Agent PowerShell 实现：`dhcp-agent/internal/dhcp/powershell.go`
- DHCP legacy Agent：`dhcp-agent/legacy/source-agent.ps1`
- DHCP Agent 模型：`dhcp-agent/internal/dhcp/model.go`
- 前端 DHCP 页面：`frontend/src/routes/_authenticated/dhcp.tsx`
- 前端数据层：`frontend/src/lib/dns-dhcp-store.ts`

## 二、数据库快照

DHCP 作用域写入 `dhcp_scopes`：

| 字段 | 含义 | 来源 |
| :-: | :-: | :-: |
| `id` | 平台 UUID | PostgreSQL |
| `name` | 作用域名称 | DHCP Agent 或平台输入 |
| `description` | 作用域描述 / 注释 | DHCP Agent |
| `subnet` | 子网信息 | DHCP Agent 或平台输入 |
| `start_range` | 起始地址 | DHCP Agent 或平台输入 |
| `end_range` | 结束地址 | DHCP Agent 或平台输入 |
| `lease_duration_hours` | 租约小时数 | DHCP Agent 或平台输入 |
| `lease_duration_seconds` | 租约秒数，`-1` 表示无限制，前端按天 / 时 / 分动态展示 | DHCP Agent 或平台输入 |
| `state` | 作用域状态 | DHCP Agent 或平台操作 |
| `server_id` | 所属服务器 | 平台服务器登记 |
| `external_id` | Windows DHCP 作用域 ID | DHCP Agent |
| `last_synced_at` | 最近同步时间 | 后端同步服务 |
| `sync_status` | 同步状态 | 后端同步服务 |
| `last_error` | 最近同步错误 | 后端同步服务 |

DHCP 排除范围写入 `dhcp_exclusions`：

| 字段 | 含义 | 来源 |
| :-: | :-: | :-: |
| `id` | 平台 UUID | PostgreSQL |
| `scope_id` | 所属平台作用域 ID | 后端同步服务映射 |
| `start_ip` | 排除范围起始 IP | DHCP Agent 或平台输入 |
| `end_ip` | 排除范围结束 IP | DHCP Agent 或平台输入 |
| `external_id` | 外部排除范围标识 | DHCP Agent |
| `last_synced_at` | 最近同步时间 | 后端同步服务 |

DHCP 租约写入 `dhcp_leases`：

| 字段 | 含义 | 来源 |
| :-: | :-: | :-: |
| `id` | 平台 UUID | PostgreSQL |
| `scope_id` | 所属平台作用域 ID | 后端同步服务映射 |
| `ip` | 租约 IP | DHCP Agent |
| `mac` | 客户端 MAC/ClientId | DHCP Agent |
| `hostname` | 租约名称 | DHCP Agent |
| `state` | 租约类型，通常为 `DHCP` 或 `BOOTP` | DHCP Agent |
| `expires_at` | 到期时间 | DHCP Agent |
| `external_id` | 外部租约标识 | DHCP Agent |
| `last_synced_at` | 最近同步时间 | 后端同步服务 |

DHCP 保留地址写入 `dhcp_reservations`：

| 字段 | 含义 | 来源 |
| :-: | :-: | :-: |
| `id` | 平台 UUID | PostgreSQL |
| `scope_id` | 所属平台作用域 ID | 后端同步服务映射 |
| `ip` | 保留地址 IP | DHCP Agent 或平台输入 |
| `mac` | 客户端 MAC/ClientId | DHCP Agent 或平台输入 |
| `name` | 保留名称 | DHCP Agent 或平台输入 |
| `description` | 描述 | DHCP Agent 或平台输入 |
| `external_id` | 外部保留地址标识 | DHCP Agent |
| `last_synced_at` | 最近同步时间 | 后端同步服务 |

同步时使用 `server_id + external_id` 识别同一个作用域。

同步时使用以下组合避免重复写入：

- 租约：`scope_id + ip`
- 保留地址：`scope_id + ip`
- 排除范围：`scope_id + start_ip + end_ip`

全量同步采用“以 Agent 快照为准”的收敛策略：

- Agent 返回的新作用域、排除范围、租约和保留地址会写入数据库。
- Agent 返回的已有作用域、排除范围、租约和保留地址会更新数据库。
- 同一服务器下 Agent 不再返回的旧作用域会从数据库删除。
- 同一作用域下 Agent 不再返回的旧排除范围、租约和保留地址会从数据库删除。

局部同步只收敛当前作用域，不会删除同服务器其他作用域。

全量 DHCP 同步中，单服务器下作用域详情采集并发由「系统配置 / 基础配置 / 同步参数」中的 DHCP 作用域并发控制，可配置 `1` 到 `50` 个，并保存到 PostgreSQL。

DHCP 操作后的二次同步等待时间由「系统配置 / 基础配置 / 同步参数」中的操作后刷新等待控制，默认 `10` 秒，可配置 `1` 到 `60` 秒。同一作用域在等待窗口内继续发生 DHCP 操作时，计时会重新开始；同一作用域仍有操作正在执行时，后端会等操作结束后再开始等待窗口。

DHCP 作用域创建、作用域删除、作用域启停、租约释放、保留地址创建和保留地址删除的整体超时时间由「系统配置 / 基础配置 / Agent 判定」中的 Agent 操作超时控制，默认 `20` 秒，可配置 `1` 到 `60` 秒。

DHCP 作用域同步和全量同步中的 DHCP 详情采集由「系统配置 / 基础配置 / Agent 判定」中的 Agent 全量同步超时控制，默认 `300` 秒，可配置 `60` 到 `600` 秒。

后端未读取到入库配置时，DHCP 作用域并发使用默认值 `5`，操作后刷新等待使用默认值 `10` 秒，Agent 操作超时使用默认值 `20` 秒，Agent 全量同步超时使用默认值 `300` 秒。

## 三、刷新入口

### 3.1 页面打开

页面打开或浏览器刷新只读取数据库：

```text
frontend DHCP page
  -> GET /api/state
  -> PostgreSQL dhcp_scopes / dhcp_leases / dhcp_reservations
```

不会触发 DHCP Agent 采集。

### 3.2 手动全量刷新

顶部工具栏全量刷新调用：

```text
POST /api/refresh
```

后端遍历 `DHCP` 角色服务器。Go DHCP Agent 和 legacy DHCP Agent 都使用逐作用域采集：

Windows Server 2008/2008 R2 legacy Agent 会在 `/health` 中返回 `mode=legacy`。后端识别后的采集规则如下：

- 优先通过 `GET /dhcp/scopes/{scopeId}/details` 一次读取该作用域租约和保留地址。
- 旧 legacy 脚本返回 `404` 时，后端会回退到分别请求租约和保留地址接口。
- 每个作用域详情采集成功后会立即写入该作用域快照。
- 如果全量同步总超时，已完成作用域的数据仍会保留在页面和数据库中。
- Go Agent 和 legacy Agent 的作用域详情采集并发都由「系统配置 / 基础配置 / 同步参数」控制。
- legacy Agent 本地再通过 `DHCP_AGENT_LEGACY_REQUEST_CONCURRENCY` 控制 runspace worker 池可同时处理的 HTTP 请求数。
- legacy Agent 依赖 `netsh` 枚举租约和保留地址，作用域或租约较多时应适当调大全量同步超时。
- 应根据目标服务器性能把 DHCP 作用域并发和 legacy 本地请求并发从较低值逐步上调。

- `GET /dhcp/scopes`
- `GET /dhcp/scopes/{scopeId}/details`
- `GET /dhcp/scopes/{scopeId}/leases`
- `GET /dhcp/scopes/{scopeId}/reservations`

同步完成后写入 PostgreSQL 快照。

Go DHCP Agent 内部单次 PowerShell 命令由 `DHCP_AGENT_POWERSHELL_TIMEOUT_SECONDS` 控制：

- 默认值为 `180` 秒。
- 可配置范围为 `1` 到 `3600` 秒。
- 覆盖作用域、租约、保留地址读取，以及作用域、租约和保留地址操作。
- legacy DHCP Agent 不读取该变量，继续使用 `netsh.exe` 直接调用方式。
- Go DHCP Agent 和 legacy DHCP Agent 都会将运行日志写入 `DHCP_AGENT_LOG_PATH` 指定的文件。
- legacy DHCP Agent 通过 `DHCP_AGENT_LEGACY_REQUEST_CONCURRENCY` 控制本地 HTTP 请求并发，范围 `1` 到 `50`，默认 `5`。
- legacy DHCP Agent 会在同步期间复用一次全局 `netsh dhcp server dump` 解析出的保留地址缓存；后端在 DHCP 同步结束后会调用 Agent 清理本次缓存。
- 大作用域租约或保留地址枚举耗时较长时，如果 Go Agent 内部 PowerShell 超时小于后端全量同步超时，后端可能收到连接被远端关闭或 Agent 返回错误。

Go DHCP Agent 读取作用域、租约和保留地址时会对单条 PowerShell 对象做容错处理：

- 作用域、租约或保留地址单条对象字段异常时，会写入 PowerShell stderr 并跳过该条对象，不中断同一批次其他对象枚举。
- `LeaseDuration`、`LeaseExpiryTime`、`ClientId`、`HostName`、`Name` 和 `Description` 等字段允许为空，并会转换为 `0` 或空字符串。
- 保留地址列表对象缺少名称或描述时，Go Agent 会按 IP 尝试读取单条保留地址详情补齐字段。
- 租约和保留地址缺少 IP 时会跳过该条记录，避免写入无法映射到作用域的快照。

### 3.3 定时全量刷新

后端通过 `RUNTIME_DEEP_SYNC_INTERVAL` 周期执行全量同步：

- 默认 `0`，即关闭。
- 设置为大于 `0` 的 Go duration 可启用。
- 创建 `runtime.refresh.all` 任务。
- 不会因为前端页面刷新而触发。

DHCP Agent 状态更新规则如下：

- 后台同步和后端自动连通性检查会先访问 Agent `/health`。
- 后端自动连通性检查的检查间隔和并发数量由「系统配置 / 基础配置 / Agent 判定」控制，默认每 `1` 分钟串行检查。
- `/health` 失败会累计 `servers.failure_count`。
- 连续失败次数达到「系统配置 / 基础配置 / Agent 判定」中的离线失败次数后，服务器状态才会标记为 `Offline`，并在状态从非 `Offline` 进入 `Offline` 时创建 Agent 离线通知。
- 仪表板或设置页手动测试连接失败会立即标记为 `Offline`。
- 健康检查或同步成功后会恢复为 `Online`，清零失败计数、更新最近检查时间并自动清理对应 Agent 的离线通知。
- 如果 `/health` 成功但后续 DHCP 资源同步失败，刷新任务会记录失败，不按 Agent 离线失败次数累计。

### 3.4 同步 Agent

DHCP 管理页标题行右侧的当前 Agent 选择框只过滤 PostgreSQL 中已同步的 DHCP 快照，不会因为切换 Agent 直接访问 Windows DHCP。

左侧作用域列表支持搜索作用域名称、网段、起止地址范围和状态；没有作用域或搜索无匹配结果时，会显示“未找到匹配作用域”。

选择当前 DHCP Agent 后点击「刷新」会调用：

```text
POST /api/servers/{id}/sync
```

后端创建 `runtime.refresh.server` 任务，只同步该 Agent 对应角色的数据，并在完成后发布 `runtime.updated` 让页面重新读取数据库快照。

前端会在任务执行期间让刷新图标保持旋转，并显示固定 toast，toast 文本可复制且保留关闭按钮。同步完成后会更新为完成提示，并在 3 秒后自动隐藏。

新建 DHCP 作用域时，前端不再单独选择服务器。后端会使用 DHCP 管理页标题行右侧当前选择的 DHCP Agent 作为目标服务器。

### 3.5 作用域级刷新

DHCP 作用域详情卡片中的刷新按钮调用：

```text
POST /api/dhcp/scopes/{id}/refresh
```

后端创建任务：

```text
runtime.refresh.dhcp.scope
```

该任务只同步当前作用域的排除范围、租约和保留地址，不会刷新其他作用域，也不会遍历其他 DHCP 服务器。

前端会显示固定 toast：

- 任务执行期间为 `<作用域> 正在刷新`
- 任务完成后更新为 `<作用域> 刷新完成`
- 完成后强制重新读取数据库快照
- 当前右侧卡片会立即显示最新排除范围、租约和保留地址

Agent 管理中对服务器执行同步 Agent：

1. 后端创建 `runtime.refresh.server` 任务。
2. 后台调用 Agent `/health`。
3. 成功后更新服务器状态为 `Online`。
4. 后端同步该服务器角色对应的数据。
5. 同步完成后发布 `runtime.updated`。

## 四、Agent 接口

DHCP Agent 提供以下接口：

| 接口 | 用途 |
| :-: | :-: |
| `GET /health` | 健康检查 |
| `GET /dhcp/probe` | DHCP Agent 轻量连通性测试，不枚举作用域 |
| `GET /dhcp/scopes` | 读取 DHCP 作用域列表 |
| `POST /dhcp/scopes` | 创建 DHCP 作用域 |
| `PUT /dhcp/scopes/{scopeId}` | 更新 DHCP 作用域名称、租期、状态和地址范围 |
| `POST /dhcp/scopes/state` | 通过 JSON body 启用或停用 DHCP 作用域 |
| `DELETE /dhcp/scopes/{scopeId}` | 删除 DHCP 作用域 |
| `POST /dhcp/scopes/{scopeId}/activate` | 启用 DHCP 作用域 |
| `POST /dhcp/scopes/{scopeId}/deactivate` | 停用 DHCP 作用域 |
| `GET /dhcp/scopes/{scopeId}/details` | legacy Agent 一次读取指定作用域排除范围、租约和保留地址详情 |
| `GET /dhcp/scopes/{scopeId}/exclusions` | 读取指定作用域排除范围 |
| `POST /dhcp/exclusions` | 创建 DHCP 排除范围 |
| `POST /dhcp/exclusions/delete` | 通过 JSON body 删除 DHCP 排除范围 |
| `GET /dhcp/scopes/{scopeId}/leases` | 读取指定作用域租约 |
| `POST /dhcp/leases/release` | 通过 JSON body 释放指定租约 |
| `DELETE /dhcp/scopes/{scopeId}/leases/{ip}` | 释放指定租约 |
| `GET /dhcp/scopes/{scopeId}/reservations` | 读取指定作用域保留地址 |
| `POST /dhcp/reservations` | 创建 DHCP 保留地址 |
| `POST /dhcp/reservations/update` | 通过 JSON body 更新 DHCP 保留地址 |
| `POST /dhcp/reservations/delete` | 通过 JSON body 删除 DHCP 保留地址 |
| `DELETE /dhcp/reservations/{scopeId}/{ip}` | 删除 DHCP 保留地址 |
| `POST /dhcp/cache/clear` | legacy Agent 清理本次 DHCP 同步使用的全局 dump 缓存 |

Windows Server 2008/2008 R2 legacy DHCP Agent 也支持以上接口。legacy 模式说明如下：

- 不依赖新版 `DhcpServer` PowerShell 模块。
- 使用 `netsh dhcp server` 执行读取和写入操作。
- 作用域列表读取 `netsh dhcp server show scope`，并从行内识别作用域地址、子网掩码、状态、名称和注释。
- 作用域注释写入 `dhcp_scopes.description`，前端在作用域标题下方显示，空值显示为 `-`。
- legacy Agent 会从全局 `netsh dhcp server dump` 的当前作用域上下文和 `add iprange` 行解析地址范围。
- legacy Agent 会从全局 `netsh dhcp server dump` 的当前作用域上下文和 `add excluderange` 行解析排除范围。
- legacy Agent 会从全局 `netsh dhcp server dump` 的 `optionvalue 51 DWORD` 按作用域解析租期秒数；值为 `-1` 时表示无限制。
- 后端保存 `lease_duration_seconds`，并兼容写入向上取整后的 `lease_duration_hours`。
- 前端优先使用秒级租期动态展示为天 / 时 / 分；无限制时显示为“无限制”。
- 脚本源码保持 ASCII 以降低 Windows Server 2008/2008 R2 PowerShell 2.0 编码解析风险。
- `GET /dhcp/probe` 用于后端测试 DHCP Agent 连接，不枚举作用域，也不会触发全局 `dump`。
- `GET /dhcp/scopes` 用于同步作用域列表，会解析租期并预热本次同步的全局 `dump` 缓存。
- legacy Agent 的逐作用域详情采集有两层并发控制：后端按系统配置中的 DHCP 作用域并发同时请求多个作用域，legacy Agent 本地再按 `DHCP_AGENT_LEGACY_REQUEST_CONCURRENCY` 使用 runspace worker 池处理这些 HTTP 请求。
- Windows Server 2008/2008 R2 上建议先从 `2` 到 `3` 观察，若终端或 Agent 响应明显变慢，可继续调低并发；若仅因总耗时超过限制失败，可优先调大 Agent 全量同步超时。
- legacy Agent 支持 `GET /dhcp/scopes/{scopeId}/details` 后，每个作用域详情从排除范围、租约和保留地址多个 HTTP 往返降为一个 HTTP 往返；该接口内部会并行读取排除范围、租约和保留地址。
- 旧脚本不支持 `details` 时，后端会自动回退到排除范围、租约和保留地址接口；旧脚本不支持排除范围接口时，后端按空列表处理。
- legacy 粒度同步对齐 DNS 区域记录同步的入库方式：作用域列表先入库，每个作用域详情成功后单独入库，避免全量任务超时时丢弃已完成的作用域数据。
- 如果后端全量同步超时后关闭 HTTP 连接，legacy Agent 写响应时可能检测到客户端断开，并记录 `Client disconnected` 日志；这表示调用方已取消请求，不代表 `netsh` DHCP 枚举本身失败。
- 完整作用域详情采集时，地址范围优先从全局 `dump` 的 `add iprange` 读取，缺失时回退到 `netsh dhcp server scope <scopeId> show iprange`。
- 同一作用域下排除范围、租约和保留地址读取会并行执行。
- 租约读取使用 `show clients 1`，从 IP 行识别 IPv4、MAC、租用截止日期和 `-U-` / `-D-` 等租约类型标记，并从同一行类型标记后提取租约名称；租用截止日期为 `永不过期` 时，前端按 DHCP 管理控制台显示为 `保留 (活动的)`。
- 保留地址读取使用同步期间保留的全局 `netsh dhcp server dump` 缓存，并从 `reservedip` 命令行按作用域分组提取保留 IP、MAC、名称和描述。
- 排除范围读取使用同步期间保留的全局 `netsh dhcp server dump` 缓存，并从当前作用域上下文和 `add excluderange` 命令行按作用域分组提取；未匹配到时按空排除范围返回，不再回退执行逐作用域 `scope <scopeId> dump`。
- 同步结束后后端会通知 legacy Agent 清理本次同步使用的全局 `dump` 缓存。
- 任一作用域的租约或保留地址枚举失败时，后端刷新任务会记录失败；已完成作用域会保留已写入的最新快照。
- legacy Agent 使用端口级单实例互斥，并在日志中记录每个 HTTP 请求的 `requestId` 和耗时，便于排查同步链路。
- legacy Agent 会记录 DHCP 采集阶段日志，包括 `dhcp scopes start/done`、`dhcp leases start/done scope=<scopeId>` 和 `dhcp reservations start/done scope=<scopeId>`。如果服务器终端看似卡住，可通过最后一条 `start` 日志定位卡在具体作用域的租约或保留地址枚举。
- 支持 `.env` 中的 `DHCP_AGENT_PORT`、`DHCP_AGENT_API_KEY`、`DHCP_AGENT_ALLOW_ANONYMOUS`、`DHCP_AGENT_LOG_PATH` 和 `DHCP_AGENT_LEGACY_REQUEST_CONCURRENCY`。

业务接口需要携带：

```text
X-API-Key: <DHCP_AGENT_API_KEY>
```

除非 Agent 显式开启匿名访问。

## 五、操作链路

DHCP 管理操作遵循“Agent 成功后按作用域延迟合并局部刷新任务”的规则：

- 创建 DHCP 作用域：后端转发到 `POST /dhcp/scopes`，成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 任务。
- 更新 DHCP 作用域：后端转发到 `PUT /dhcp/scopes/{scopeId}`，当前更新作用域名称、租期、状态和地址范围；子网仍以 Agent 同步快照为准。
- 启用或停用作用域：后端根据当前数据库状态转发到 `activate` 或 `deactivate`，成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 任务。
- 刷新 DHCP 作用域：后端立即创建 `runtime.refresh.dhcp.scope` 任务，只同步当前作用域排除范围、租约和保留地址。
- 删除 DHCP 作用域：后端转发到 `DELETE /dhcp/scopes/{scopeId}`，成功后删除数据库中该作用域及其排除范围、租约、保留地址快照。
- 创建 DHCP 排除范围：后端转发到 `POST /dhcp/exclusions`，成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 任务。
- 删除 DHCP 排除范围：后端转发到 `POST /dhcp/exclusions/delete`，成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 任务。
- 释放 DHCP 租约：后端优先转发到 `POST /dhcp/leases/release`，旧 Agent 返回 `404` 时回退到 `DELETE /dhcp/scopes/{scopeId}/leases/{ip}`，成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 任务。
- 创建 DHCP 保留地址：后端转发到 `POST /dhcp/reservations`，成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 任务。
- 更新 DHCP 保留地址：后端转发到 `POST /dhcp/reservations/update`，成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 任务。
- 删除 DHCP 保留地址：后端优先转发到 `POST /dhcp/reservations/delete`，旧 Agent 返回 `404` 时回退到 `DELETE /dhcp/reservations/{scopeId}/{ip}`，成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 任务。

如果 Agent 调用失败，后端不会修改数据库快照，并返回 `agent_*_failed` 类错误。

如果 Agent 成功但后续局部刷新任务失败，`refresh_tasks` 会记录失败状态，页面仍会保留操作后的本地快照兜底，并依赖下一次全量刷新完成最终收敛。

Go DHCP Agent 执行写入类操作前会做基础参数校验：

- 创建作用域时要求名称、子网、起始地址和结束地址完整。
- 作用域子网必须是 IPv4 地址或 IPv4 CIDR；未填写掩码时按 `/24` 处理。
- 起始地址和结束地址必须属于该作用域网段，且起始地址不能大于结束地址。
- 启停、删除作用域时，作用域 ID 必须是 IPv4 地址。
- 创建或删除排除范围时，作用域 ID、起始 IP 和结束 IP 必须是 IPv4 地址。
- 释放租约、创建或删除保留地址时，作用域 ID 和目标 IP 必须是 IPv4 地址。
- 创建保留地址时，作用域 ID、IP、MAC/ClientId 和名称不能为空。

## 六、任务、审计和 SSE

任务表：

- `refresh_tasks.type=runtime.refresh.all`
- `refresh_tasks.type=runtime.refresh.server`
- `refresh_tasks.type=runtime.refresh.dhcp.scope`

当前 DHCP 相关审计 action：

- `Queued server sync`
- `Created DHCP scope`
- `Updated DHCP scope`
- `Toggled DHCP scope`
- `Queued DHCP scope refresh`
- `Deleted DHCP scope`
- `Released DHCP lease`
- `Created DHCP reservation`
- `Updated DHCP reservation`
- `Deleted DHCP reservation`

SSE 事件：

- `runtime.refresh.all`
- `runtime.refresh.server`
- `runtime.refresh.dhcp.scope`
- `runtime.updated`

Redis 当前用于刷新事件发布和最近刷新事件缓存，不作为 DHCP 明细唯一数据源。

## 七、后续变更同步要求

涉及以下变更时，必须同步更新本文档：

- DHCP Agent PowerShell 或 legacy `netsh` 命令变化。
- DHCP 作用域、租约或保留地址字段采集、解析、默认值、外部 ID 映射规则变化。
- DHCP 创建、启停、删除、释放租约或保留地址操作链路变化。
- `runtime.refresh.all` 或 `runtime.refresh.dhcp.scope` 中 DHCP 同步范围、任务 payload 或 SSE 事件变化。
- DHCP 页面刷新入口、加载状态、SSE 订阅或缓存读取规则变化。
- DHCP 相关数据库表、索引、唯一约束或 upsert 策略变化。
