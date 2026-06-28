# DHCP 管理与同步说明

本文档说明 ZoneLease 中 DHCP 作用域、排除范围、租约和保留地址的展示来源、Agent 交互接口、数据库快照规则、操作链路和后续开发时的文档同步要求。

## 一、整体链路

DHCP 页面默认读取 PostgreSQL 中的 DHCP 快照。后端在手动全量刷新、定时全量刷新、Agent 管理中的同步 Agent，以及 DHCP 管理操作中访问 DHCP Agent。

当前链路如下：

1. 前端 DHCP 页面调用 `/api/state`。
2. 后端从 PostgreSQL 读取 `dhcp_scopes`、`dhcp_exclusions`、`dhcp_leases` 和 `dhcp_reservations`。
3. 手动全量刷新、定时全量刷新、同步当前 Agent 或作用域刷新按钮会访问 DHCP Agent。
4. Go DHCP Agent 通过 Windows PowerShell `DhcpServer` 模块读取作用域、租约和保留地址；Windows Server 2008/2008 R2 legacy Agent 通过 `netsh dhcp server` 提供同一组 HTTP 接口。
5. 后端把 Agent 返回结果写入 PostgreSQL 快照表。
6. 后端通过 Redis Pub/Sub 实时发布，并写入 Redis Stream 支持回放，前端收到事件后重新读取数据库快照。
7. DHCP 创建、启停、删除、排除范围变更、释放租约和保留地址变更会在常规路径下按作用域延迟合并创建 `runtime.refresh.dhcp.scope` 局部刷新任务；作用域刷新按钮在目标 Agent 未同步且同目标刷新未运行时立即创建局部刷新任务。

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
| `subnet` | 子网信息，统一使用 IPv4 CIDR 前缀格式，例如 `10.18.0.0/24` | DHCP Agent 或平台输入 |
| `default_gateway` | 默认网关，Windows DHCP Option 003 Router | DHCP Agent 或平台输入 |
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
| `state` | 租约状态 / 地址状态，例如 `Active`、`Inactive`、`ReservedActive`、`ReservedInactive`，具体值随 Windows DHCP 返回和 legacy 解析结果变化 | DHCP Agent |
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

前端展示规则：

- 左侧作用域按作用域网段 IPv4 自然排序，同网段再按掩码和名称排序。
- 租约列表默认按 IP 地址自然排序，IP 地址、名称、MAC 和租用截止日期列名支持默认、升序和降序三态切换。
- 保留地址列表默认按 IP 地址自然排序，IP、MAC、名称和描述列名支持默认、升序和降序三态切换。
- 排除范围列表默认按起始 IP 地址自然排序，不提供列名排序切换。
- 租约列表默认只渲染前 `200` 条，底部显示“已显示 x / 总数 条”和“加载更多”，点击后按 `200` 条继续展示；搜索和排序仍基于当前作用域全部租约。

全量同步采用“以 Agent 快照为准”的收敛策略：

- Agent 返回的新作用域、排除范围、租约和保留地址会写入数据库。
- Agent 返回的已有作用域、排除范围、租约和保留地址会更新数据库。
- 同一服务器下 Agent 不再返回的旧作用域会从数据库删除。
- 同一作用域下 Agent 不再返回的旧排除范围、租约和保留地址会从数据库删除。

局部同步只收敛当前作用域，不会删除同服务器其他作用域。

全量 DHCP 同步中，单服务器下作用域详情采集并发由「系统配置 / 基础配置 / 同步参数」中的 DHCP 作用域并发控制，可配置 `1` 到 `50` 个，并保存到 PostgreSQL。

DHCP 操作后的二次同步等待时间由「系统配置 / 基础配置 / 同步参数」中的操作后刷新等待控制，默认 `10` 秒，可配置 `1` 到 `60` 秒。同一作用域在等待窗口内继续发生 DHCP 操作时，计时会重新开始；同一作用域仍有操作正在执行时，后端会等操作结束后再开始等待窗口。

如果目标 DHCP Agent 正在执行手动全量同步、DHCP 定时全量同步或单 Agent 同步，DHCP 作用域、排除范围、租约和保留地址管理操作会返回“当前 Agent 正在同步，请稍后再操作”，避免操作与 Agent 同步任务并发访问同一 Agent。DHCP 作用域局部刷新不写入 Agent 同步运行态，只通过同目标刷新锁避免重复刷新。

DHCP 作用域创建、更新、删除、启停、排除范围创建和删除、租约释放、保留地址创建、更新和删除的整体超时时间由「系统配置 / 基础配置 / Agent 判定」中的 Agent 操作超时控制，默认 `20` 秒，可配置 `1` 到 `60` 秒。

Agent 保存前探测、手动测试、自动健康检查和同步前 `/health` 检查由 Agent 连接超时控制，默认 `5` 秒，可配置 `1` 到 `20` 秒。

DHCP 作用域同步和全量同步中的 DHCP 详情采集阶段由「系统配置 / 基础配置 / Agent 判定」中的 Agent 全量同步超时控制，默认 `300` 秒，可配置 `60` 到 `600` 秒。

后端未读取到入库配置时，DHCP 作用域并发使用默认值 `5`，操作后刷新等待使用默认值 `10` 秒，Agent 连接超时使用默认值 `5` 秒，Agent 操作超时使用默认值 `20` 秒，Agent 全量同步超时使用默认值 `300` 秒。

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

后端遍历所有可同步 Agent，其中 `DHCP` 角色服务器执行 DHCP 同步。可同步 Agent 指已登记、配置了 Agent URL，且角色匹配本次刷新类型的服务器。Go DHCP Agent 和 legacy DHCP Agent 都使用逐作用域采集：

Windows Server 2008/2008 R2 legacy Agent 会在 `/health` 中返回 `mode=legacy`。后端识别后的采集规则如下：

- `GET /dhcp/scopes` 通过一次全局 `netsh dhcp server dump` 解析作用域基础信息。
- 优先通过 `GET /dhcp/scopes/{scopeId}/details` 一次读取该作用域详情。
- 详情接口中只有租约列表继续执行 `show clients 1`。
- 保留地址、排除范围、地址范围、名称、描述、状态和租期均复用本次同步的 dump 缓存。
- 每个作用域详情采集成功后会立即写入该作用域快照。
- 如果全量同步总超时，已完成作用域的数据仍会保留在页面和数据库中。
- Go Agent 和 legacy Agent 的作用域详情采集并发都由「系统配置 / 基础配置 / 同步参数」控制。
- legacy Agent 依赖 `netsh` 枚举租约，作用域或租约较多时应适当调大全量同步超时。
- 应根据目标服务器性能把 DHCP 作用域并发从较低值逐步上调。

- `GET /dhcp/scopes`
- `GET /dhcp/scopes/{scopeId}/details`
- `GET /dhcp/scopes/{scopeId}/leases`
- `GET /dhcp/scopes/{scopeId}/reservations`

同步完成后写入 PostgreSQL 快照。

Go DHCP Agent 内部单次 PowerShell 命令由 `DHCP_AGENT_POWERSHELL_TIMEOUT_SECONDS` 控制：

- 默认值为 `180` 秒。
- 可配置范围为 `1` 到 `3600` 秒。
- 覆盖作用域、排除范围、租约、保留地址读取，以及作用域、排除范围、租约和保留地址操作。
- legacy DHCP Agent 不读取该变量，继续使用 `netsh.exe` 直接调用方式。
- Go DHCP Agent 和 legacy DHCP Agent 都会将运行日志写入 `DHCP_AGENT_LOG_PATH` 指定的文件。
- legacy DHCP Agent 会在全量同步期间复用一次全局 `netsh dhcp server dump` 解析出的作用域、保留地址、排除范围、地址范围、状态和租期缓存；后端仅在全量 DHCP 同步结束后调用 Agent 清理本次缓存。
- 大作用域租约或保留地址枚举耗时较长时，如果 Go Agent 内部 PowerShell 超时小于后端全量同步超时，后端可能收到连接被远端关闭或 Agent 返回错误。

Go DHCP Agent 读取作用域、租约和保留地址时会对单条 PowerShell 对象做容错处理：

- 作用域、租约或保留地址单条对象字段异常时，会写入 PowerShell stderr 并跳过该条对象，不中断同一批次其他对象枚举。
- Go DHCP Agent 会把 Windows 返回的 DHCP 子网掩码转换为 IPv4 CIDR 前缀后写入 `subnet`，例如 `255.255.255.0` 统一返回为 `/24`。
- Go DHCP Agent 会强制 PowerShell stdout 使用 UTF-8，避免中文作用域描述在 Go 侧解析为乱码。
- Go DHCP Agent 会把 Windows `ClientId` 中的 `-`、`:` 和 `.` 分隔符去掉后写入 MAC 字段，和 legacy Agent 的 MAC 展示格式保持一致。
- `LeaseDuration`、`LeaseExpiryTime`、`ClientId`、`HostName`、`Name` 和 `Description` 等字段允许为空，并会转换为 `0` 或空字符串。
- Go DHCP Agent 同步作用域和刷新单个作用域时，如果 `LeaseDuration` 文本以 `[TimeSpan]::MaxValue` 对应的 `10675199` 开头，会返回 `leaseDurationSeconds=-1`，前端按无限制显示。
- 保留地址列表对象缺少名称或描述时，Go Agent 会按 IP 尝试读取单条保留地址详情补齐字段。
- 租约和保留地址缺少 IP 时会跳过该条记录，避免写入无法映射到作用域的快照。

### 3.3 定时全量刷新

后端通过 `RUNTIME_DHCP_DEEP_SYNC_INTERVAL` 周期执行 DHCP 全量同步：

- 默认 `1h`，即每小时执行一次。
- 支持 `m`、`h`、`d` 单位，例如 `30m`、`2h`、`1d`。
- 能整除 24 小时的短间隔按本地当天零点对齐；按天配置的间隔按本地自然日零点对齐，例如 `2d` 从当前本地日期零点起算下一个两天边界；不能整除 24 小时且不是整天数的间隔按 Unix epoch `1970-01-01 00:00:00 UTC` 起算的固定周期对齐。
- 设置为 `0` 可关闭 DHCP 定时全量同步。
- 创建 `runtime.refresh.dhcp.all` 任务，只同步 DHCP Agent。
- 不会因为前端页面刷新而触发。

DHCP Agent 状态更新规则如下：

- 后台同步会先访问 Agent `/health`，该预检查使用 Agent 连接超时；后端自动连通性检查只检查已配置 Agent URL 且当前未处于同步中的 Agent。
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

该任务只同步当前作用域，不会刷新其他作用域，也不会遍历其他 DHCP 服务器。

单作用域刷新会先调用 `GET /dhcp/scopes/{scopeId}` 读取当前作用域基础信息，不再通过 `GET /dhcp/scopes` 枚举全部作用域。legacy Agent 会执行 `netsh dhcp server scope <scopeId> dump` 解析当前作用域基础信息、保留地址、排除范围、地址范围、状态和租期；Go Agent 会通过 `Get-DhcpServerv4Scope -ScopeId` 只读取目标作用域基础信息。随后后端再通过详情接口读取该作用域租约、保留地址和排除范围；legacy Agent 不调用 `/dhcp/cache/clear`。

前端会显示固定 toast：

- 任务执行期间为 `<作用域> 正在刷新`
- 任务完成后更新为 `<作用域> 刷新完成`
- 完成后强制重新读取数据库快照
- 当前右侧卡片会立即显示最新排除范围、租约和保留地址

### 3.6 DHCP 数据导出

DHCP 管理页标题行右侧的「导出」按钮会先重新读取当前状态，再按当前选中的 DHCP Agent 过滤作用域、排除范围、租约和保留地址。

导出对象支持：

- 作用域：导出当前 Agent 的 DHCP 作用域汇总。
- 租约：导出当前 Agent 作用域下的租约列表。
- 保留地址：导出当前 Agent 作用域下的保留地址列表。
- 排除范围：导出当前 Agent 作用域下的排除范围列表。

导出范围支持：

- 全部：导出当前 Agent 的全部作用域数据。
- 启用：只导出启用状态的作用域及其关联数据。
- 停用：只导出非启用状态的作用域及其关联数据。
- 自定义：输入作用域名称、子网或地址范围时实时模糊搜索，必须从下拉结果中点击已有作用域后才会加入导出作用域标签。

导出格式支持 `XLSX`、`XLS`、`CSV` 和 `TXT`。

导出列按对象区分：

- 作用域：作用域名称、子网、状态、起始地址、结束地址、地址范围、默认网关、租期、描述、排除范围数、租约数、保留地址数、同步状态、同步时间、错误信息。
- 租约：作用域名称、作用域子网、IP 地址、名称、MAC、租用截止日期；该导出不包含状态列，租用截止日期可能显示为 `保留 (活动的)` 或 `保留 (不活动的)`。
- 保留地址：作用域名称、作用域子网、IP 地址、MAC、名称、描述。
- 排除范围：作用域名称、作用域子网、起始 IP 地址、结束 IP 地址。

导出行按作用域网段 IPv4 自然排序；同一作用域内的租约、保留地址和排除范围按 IP 地址自然排序。租约、保留地址和排除范围导出采用分批异步生成，选择导出对象或范围后，导出按钮会显示准备状态，避免大量租约一次性计算造成页面明显卡顿。

Agent 管理中对服务器执行同步 Agent：

1. 后端创建 `runtime.refresh.server` 任务。
2. 后台调用 Agent `/health`。
3. 成功后更新服务器状态为 `Online`。
4. 后端同步该服务器角色对应的数据。
5. 同步完成后发布 `runtime.updated`。

## 四、Agent 接口

DHCP Agent 提供以下接口；其中 `POST /dhcp/cache/clear` 为 Windows Server 2008/2008 R2 legacy Agent 专用兼容接口，Go Agent 不注册该路由：

| 接口 | 用途 |
| :-: | :-: |
| `GET /health` | 健康检查 |
| `GET /dhcp/probe` | DHCP Agent 轻量连通性测试，不枚举作用域 |
| `GET /dhcp/scopes` | 读取 DHCP 作用域列表 |
| `GET /dhcp/scopes/{scopeId}` | 读取单个作用域基础信息；Go Agent 和 legacy Agent 均支持 |
| `POST /dhcp/scopes` | 创建 DHCP 作用域 |
| `PUT /dhcp/scopes/{scopeId}` | 更新 DHCP 作用域名称、描述、默认网关、租期和地址范围；启停状态建议通过切换接口处理 |
| `POST /dhcp/scopes/state` | 通过 JSON body 启用或停用 DHCP 作用域 |
| `DELETE /dhcp/scopes/{scopeId}` | 删除 DHCP 作用域 |
| `POST /dhcp/scopes/{scopeId}/activate` | 启用 DHCP 作用域 |
| `POST /dhcp/scopes/{scopeId}/deactivate` | 停用 DHCP 作用域 |
| `GET /dhcp/scopes/{scopeId}/details` | 一次读取指定作用域排除范围、租约和保留地址详情；Go Agent 和 legacy Agent 均支持 |
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
| `PUT /dhcp/reservations/{scopeId}/{ip}` | 更新 DHCP 保留地址，保留用于兼容旧调用方 |
| `DELETE /dhcp/reservations/{scopeId}/{ip}` | 删除 DHCP 保留地址 |
| `POST /dhcp/cache/clear` | legacy Agent 清理本次全量 DHCP 同步使用的全局 dump 缓存 |

Windows Server 2008/2008 R2 legacy DHCP Agent 也支持以上接口。legacy 模式说明如下：

- 不依赖新版 `DhcpServer` PowerShell 模块。
- 使用 `netsh dhcp server` 执行读取和写入操作。
- JSON 请求体依赖 .NET `System.Web.Extensions` 解析；目标服务器未启用该组件时，编辑作用域、释放租约、删除排除范围和删除保留地址等 body 版接口会失败，前端会提示安装或启用 .NET Framework 3.5/4.x。
- legacy Agent 的 JSON 请求体默认按 UTF-8 解码，避免中文名称和描述在 Windows 默认代码页下被误读为乱码。
- 作用域列表不再执行 `netsh dhcp server show scope`。
- legacy Agent 会从一次全局 `netsh dhcp server dump` 解析作用域列表。
- legacy Agent 会从全局 `netsh dhcp server dump` 的 `add scope` 行解析作用域 IP 网段、名称和描述，并写入 `dhcp_scopes.name` 与 `dhcp_scopes.description`；描述为空时保持为空，前端显示为 `-`。
- legacy Agent 会从全局 `netsh dhcp server dump` 的当前作用域上下文和 `add iprange` 行解析地址范围。
- legacy Agent 会从全局 `netsh dhcp server dump` 的当前作用域上下文和 `add excluderange` 行解析排除范围。
- legacy Agent 会从全局 `netsh dhcp server dump` 的 `scope <scopeId> set state` 行解析作用域启用状态。
- legacy Agent 会从全局 `netsh dhcp server dump` 的 `optionvalue 51 DWORD` 按作用域解析租期秒数；值为 `-1` 时表示无限制。
- 修改无限制租期时，Agent 写入 DHCP 服务器的实际值为 `4294967295`；平台查询和展示逻辑仍按 `-1` 识别无限制。
- 后端保存 `lease_duration_seconds`，并兼容写入向上取整后的 `lease_duration_hours`。
- 前端优先使用秒级租期动态展示为天 / 时 / 分；无限制时显示为“无限制”。
- 脚本源码保持 ASCII 以降低 Windows Server 2008/2008 R2 PowerShell 2.0 编码解析风险。
- `GET /dhcp/probe` 用于后端测试 DHCP Agent 连接，不枚举作用域，也不会触发全局 `dump`。
- `GET /dhcp/scopes` 用于同步作用域列表，会执行一次全局 `dump` 并预热本次同步缓存。
- `GET /dhcp/scopes/{scopeId}` 用于单作用域刷新；Go Agent 只读取目标作用域基础信息，legacy Agent 会执行 `scope <scopeId> dump` 并只预热该作用域缓存，不调用 `/dhcp/cache/clear`。
- legacy Agent 的逐作用域详情采集由后端按系统配置中的 DHCP 作用域并发同时请求多个作用域。
- Windows Server 2008/2008 R2 上建议先从 `2` 到 `3` 观察，若终端或 Agent 响应明显变慢，可继续调低并发；若仅因总耗时超过限制失败，可优先调大 Agent 全量同步超时。
- legacy Agent 支持 `GET /dhcp/scopes/{scopeId}/details` 后，每个作用域详情从排除范围、租约和保留地址多个 HTTP 往返降为一个 HTTP 往返；该接口中排除范围和保留地址读取缓存，租约读取继续执行 `show clients 1`。
- Go Agent 支持 `GET /dhcp/scopes/{scopeId}/details` 后，每个作用域详情从 3 次 PowerShell 调用降为 1 次 PowerShell 调用；后端在全量同步和单作用域刷新时都会优先使用该接口。
- Go Agent 支持 `GET /dhcp/scopes/{scopeId}` 后，单作用域刷新不再为了定位目标作用域而调用 `GET /dhcp/scopes` 枚举全部作用域。
- Go Agent 创建和更新作用域时，同一请求内的作用域属性、默认网关和无限租期写入会合并到同一次 PowerShell 脚本执行；底层仍按 DHCP PowerShell 参数集拆成多条 cmdlet。
- legacy 粒度同步对齐 DNS 区域记录同步的入库方式：作用域列表先入库，每个作用域详情成功后单独入库，避免全量任务超时时丢弃已完成的作用域数据。
- 如果后端全量同步超时后关闭 HTTP 连接，legacy Agent 写响应时可能检测到客户端断开，并记录 `Client disconnected` 日志；这表示调用方已取消请求，不代表 `netsh` DHCP 枚举本身失败。
- 完整作用域详情采集时，地址范围直接从全局 `dump` 的 `add iprange` 读取。
- 更新作用域时，后端会把实际变化字段通过 `changedFields` 传给 DHCP Agent。
- Agent 只执行名称 / 描述、默认网关、租期或地址范围中发生变化的操作；请求体仍兼容状态字段，当前前端状态变更通过停用 / 启用入口完成。
- legacy Agent 创建作用域和更新多个作用域配置时，会把同一请求内需要执行的 `netsh dhcp server` 子命令合并为一次 `netsh -f` 批处理会话，减少多次启动 `netsh.exe` 和重复连接 DHCP 服务的耗时。
- 当前前端编辑弹窗提供名称、描述、默认网关、租期和地址范围；状态变更仍通过停用 / 启用入口完成。
- legacy Agent 更新描述时执行 `netsh dhcp server scope <scopeId> set comment <description>`；描述清空时执行不带描述参数的 `netsh dhcp server scope <scopeId> set comment`，名称变化只执行 `set name <name>`。
- 更新作用域地址范围时，前端允许只修改起始或只修改结束 IP；若起始和结束 IP 同时修改，则必须同时缩小范围或同时扩大范围，避免一端越过旧范围后另一端仍落在旧范围内导致 DHCP 服务器判定无效或重叠。
- 新建作用域提交前会校验地址范围必须位于子网内，不能包含子网地址或广播地址，并且作用域子网不能与当前 Agent 已有作用域重复或重叠。
- legacy Agent 地址范围变化时只执行一次 `add iprange <startRange> <endRange>`，不执行 `delete iprange`，也不再执行 `show iprange` 查询旧地址范围。
- 全量同步时，只有逐作用域租约读取的 `show clients 1` 参与 DHCP 作用域并发。
- 租约读取使用 `show clients 1`，从 IP 行识别 IPv4、MAC、租用截止日期和 `-U-` / `-D-` 等租约类型标记，并从同一行类型标记后提取租约名称。
- 租用截止日期为 `永不过期` 时，legacy Agent 会写入 `state=ReservedActive` 和 `expiresAt=never`，前端按 DHCP 管理控制台显示为 `保留 (活动的)`。
- 租用截止日期为 `不活动` 时，legacy Agent 会写入 `state=ReservedInactive` 和 `expiresAt=never`，前端按 DHCP 管理控制台显示为 `保留 (不活动的)`。
- 保留地址读取使用同步期间保留的全局 `netsh dhcp server dump` 缓存，并从 `reservedip` 命令行按作用域分组提取保留 IP、MAC、名称和描述。
- 排除范围读取使用同步期间保留的全局 `netsh dhcp server dump` 缓存，并从当前作用域上下文和 `add excluderange` 命令行按作用域分组提取；未匹配到时按空排除范围返回，不再回退执行逐作用域 `scope <scopeId> dump`。
- 全量同步结束后后端会通知 legacy Agent 清理本次同步使用的全局 `dump` 缓存。
- 任一作用域的租约或保留地址枚举失败时，后端刷新任务会记录失败；已完成作用域会保留已写入的最新快照。
- legacy Agent 使用端口级单实例互斥，并在日志中记录每个 HTTP 请求的 `requestId` 和耗时，便于排查同步链路。
- legacy Agent 会记录 DHCP 采集阶段日志，包括 `dhcp scopes start/done`、`dhcp scope details start/done scope=<scopeId>` 和 `dhcp leases start/done scope=<scopeId>`。如果服务器终端看似卡住，可通过最后一条 `start` 日志定位卡在具体作用域的租约枚举。
- 支持 `.env` 中的 `DHCP_AGENT_PORT`、`DHCP_AGENT_API_KEY`、`DHCP_AGENT_ALLOW_ANONYMOUS` 和 `DHCP_AGENT_LOG_PATH`。

当 `DHCP_AGENT_ALLOW_ANONYMOUS=false` 且 `DHCP_AGENT_API_KEY` 非空时，业务接口需要携带：

```text
X-API-Key: <DHCP_AGENT_API_KEY>
```

如果 Agent 显式开启匿名访问，或 `DHCP_AGENT_API_KEY` 留空，则不会校验业务接口 API Key。生产环境应配置非空强随机 API Key，并保持匿名访问关闭。

## 五、操作链路

DHCP 管理操作遵循“Agent 成功后按作用域延迟合并局部刷新任务”的规则：

- 创建 DHCP 作用域：后端转发到 `POST /dhcp/scopes`，成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 任务；go Agent 通过 `Add-DhcpServerv4Scope -Description` 写入描述，通过 `Set-DhcpServerv4OptionValue -Router` 写入默认网关，并把同一请求内的写入合并为一次 PowerShell 脚本执行；legacy Agent 在 `add scope` 后通过 `set comment` 写入非空描述，并通过 `set optionvalue 3 IPADDRESS` 写入默认网关；legacy Agent 会把创建、地址范围、启用、租期和默认网关写入合并为一次 `netsh -f` 批处理会话。
- Go Agent 创建有限租期作用域时使用 `leaseDurationSeconds` 生成 `New-TimeSpan -Seconds`，不再使用 `leaseDurationHours` 兜底，避免分钟级租期被向上取整为整小时；无限租期不使用 `Set-DhcpServerv4Scope -LeaseDuration`，而是在同一 PowerShell 脚本中执行 `Set-DhcpServerv4OptionValue -ScopeId <scopeId> -OptionId 51 -Value 4294967295 -ErrorAction Stop`。
- 更新 DHCP 作用域：后端转发到 `PUT /dhcp/scopes/{scopeId}`，当前编辑弹窗支持更新作用域名称、描述、默认网关、租期和地址范围；默认网关为必填项；子网仍以 Agent 同步快照为准；状态通过停用 / 启用入口更新；后端会传递变化字段，Agent 只执行实际变化项；Go Agent 同一请求内有多个变化项时会合并为一次 PowerShell 脚本执行，有限租期继续通过 `Set-DhcpServerv4Scope -LeaseDuration (New-TimeSpan -Seconds <seconds>)` 写入，无限租期通过 DHCP Option 51 写入；legacy Agent 修改描述使用 `set comment`，清空描述时省略描述参数，默认网关变化使用 `set optionvalue 3 IPADDRESS`，地址范围变化只执行一次 `add iprange`，不删除旧范围，也不再额外查询 DHCP 服务器；同一请求内有多个变化项时，legacy Agent 会合并为一次 `netsh -f` 批处理会话执行。
- 启用或停用作用域：后端根据当前数据库状态转发到 `activate` 或 `deactivate`，成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 任务。
- 刷新 DHCP 作用域：目标 Agent 未同步且同目标刷新未运行时，后端立即创建 `runtime.refresh.dhcp.scope` 任务，只同步当前作用域基础信息、排除范围、租约和保留地址；如果目标 Agent 正在同步或同目标刷新正在运行，则返回冲突提示且不创建任务。
- 删除 DHCP 作用域：后端转发到 `DELETE /dhcp/scopes/{scopeId}`，成功后删除数据库中该作用域及其排除范围、租约、保留地址快照。
- 创建 DHCP 排除范围：后端转发到 `POST /dhcp/exclusions`，成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 任务。
- 删除 DHCP 排除范围：后端转发到 `POST /dhcp/exclusions/delete`，成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 任务。
- 释放 DHCP 租约：后端优先转发到 `POST /dhcp/leases/release`，旧 Agent 返回 `404` 时回退到 `DELETE /dhcp/scopes/{scopeId}/leases/{ip}`；Go Agent 执行 `Remove-DhcpServerv4Lease -IPAddress <ip>` 释放租约；成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 任务。
- 创建 DHCP 保留地址：前端从租约列表行内“添加到保留”图标按钮进入，弹窗展示 IP、MAC、名称和描述配置后转发到 `POST /dhcp/reservations`，名称为空时保持为空；Go Agent 执行 `Add-DhcpServerv4Reservation` 时固定写入 `-Type 'dhcp'`；成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 任务，并将本地同 IP 租约快照名称更新为保留名称，同时标记为 `ReservedInactive`。
- 更新 DHCP 保留地址：后端转发到 `POST /dhcp/reservations/update`，成功后会立即更新同作用域同 IP 的租约名称快照，并按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 任务；Go Agent 执行 `Set-DhcpServerv4Reservation -IPAddress <ip> -Name <name> -Description <description> -Type 'dhcp'` 更新名称、描述和类型，不删除再创建；legacy Agent 会先执行 `delete reservedip <ip> <mac>` 删除旧保留，再执行 `add reservedip` 写入新名称和描述。
- 删除 DHCP 保留地址：后端优先转发到 `POST /dhcp/reservations/delete` 并携带数据库快照中的 MAC，旧 Agent 返回 `404` 时回退到 `DELETE /dhcp/reservations/{scopeId}/{ip}`，成功后删除本地同 IP 租约快照，并按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 任务；Go Agent 删除时执行 `Remove-DhcpServerv4Reservation -IPAddress <ip>`；legacy Agent 删除时直接执行 `delete reservedip <ip> <mac>`，不会在缺少 MAC 时额外执行全局 `dump` 兜底。

如果 Agent 调用失败，后端不会修改数据库快照，并返回 `agent_*_failed` 类错误。

如果已有关联数据库快照的 DHCP 作用域操作在 Agent 阶段失败，后端也会标记当前作用域需要延迟局部刷新。

这样可以在 legacy `netsh -f` 批处理部分命令已生效但整体返回失败时，尽快重新读取 DHCP 服务器真实状态。

如果 Agent 成功但后续局部刷新任务失败，`refresh_tasks` 会记录失败状态，页面仍会保留操作后的本地快照兜底，并依赖下一次全量刷新完成最终收敛。

Go DHCP Agent 执行写入类操作前会做基础参数校验：

- 创建作用域时要求名称、子网、起始地址和结束地址完整。
- 作用域子网必须是 IPv4 CIDR 前缀格式，例如 `10.18.0.0/24`；不再接受 `10.18.0.0/255.255.255.0` 这类子网掩码格式。
- 起始地址和结束地址必须属于该作用域网段，且起始地址不能大于结束地址。
- 启停、删除作用域时，作用域 ID 必须是 IPv4 地址。
- 创建或删除排除范围时，作用域 ID、起始 IP 和结束 IP 必须是 IPv4 地址。
- 新增排除范围提交前会校验起止 IP 格式、起始 IP 不大于结束 IP、排除范围必须位于当前作用域地址范围内，且不能与已有排除范围重叠。
- 释放租约、创建或删除保留地址时，作用域 ID 和目标 IP 必须是 IPv4 地址。
- 创建保留地址时，作用域 ID、IP 和 MAC/ClientId 不能为空；名称允许为空。

## 六、任务、审计和 SSE

任务表：

- `refresh_tasks.type=runtime.refresh.all`
- `refresh_tasks.type=runtime.refresh.dhcp.all`
- `refresh_tasks.type=runtime.refresh.server`
- `refresh_tasks.type=runtime.refresh.dhcp.scope`

当前 DHCP 相关审计 action：

- `Queued server sync`
- `Created DHCP scope`
- `Updated DHCP scope`
- `Toggled DHCP scope`
- `Queued DHCP scope refresh`
- `Created DHCP exclusion`
- `Deleted DHCP exclusion`
- `Deleted DHCP scope`
- `Released DHCP lease`
- `Created DHCP reservation`
- `Updated DHCP reservation`
- `Deleted DHCP reservation`

SSE 事件：

- `runtime.refresh.all`
- `runtime.refresh.dhcp.all`
- `runtime.refresh.server`
- `runtime.refresh.dhcp.scope`
- `runtime.updated`

Redis 当前用于刷新事件发布和最近刷新事件缓存，不作为 DHCP 明细唯一数据源。

## 七、后续变更同步要求

涉及以下变更时，必须同步更新本文档：

- DHCP Agent PowerShell 或 legacy `netsh` 命令变化。
- DHCP 作用域、排除范围、租约或保留地址字段采集、解析、默认值、外部 ID 映射规则变化。
- DHCP 创建、启停、删除、排除范围变更、释放租约或保留地址操作链路变化。
- `runtime.refresh.all`、`runtime.refresh.dhcp.all` 或 `runtime.refresh.dhcp.scope` 中 DHCP 同步范围、任务 payload 或 SSE 事件变化。
- DHCP 导出对象、范围、列名、排序或大量数据准备策略变化。
- DHCP 页面刷新入口、加载状态、SSE 订阅或缓存读取规则变化。
- DHCP 相关数据库表、索引、唯一约束或 upsert 策略变化。
