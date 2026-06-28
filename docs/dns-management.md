# DNS 管理与同步说明

本文档说明 ZoneLease 中 DNS 区域和 DNS 记录的展示来源、Agent 交互接口、数据库快照规则、操作链路和后续开发时的文档同步要求。

## 一、整体链路

DNS 页面不直接访问 Windows DNS，也不会在刷新浏览器页面时触发 Agent 采集。当前链路如下：

1. 前端 DNS 页面调用 `/api/state?includeDns=true`。
2. 后端从 PostgreSQL 读取 `dns_zones` 和 `dns_records` 快照。
3. 页面展示最近一次同步结果，并在每个区域卡片中提供区域级刷新按钮。
4. 手动全量刷新、后端定时全量刷新、Agent 管理中的同步 Agent，以及区域刷新按钮会访问 DNS Agent。
5. DNS Agent 通过 Windows PowerShell `DnsServer` 模块读取或修改 DNS 区域和记录。
6. 后端把 Agent 返回结果写入 PostgreSQL 快照表。
7. 后端通过 Redis Pub/Sub 发布 SSE 事件，前端收到事件后重新读取数据库快照。

以下系统区域不作为业务 DNS 区域同步：

- DNSSEC 内置信任锚区域 `TrustAnchors`。
- Windows DNS 内置反向区域 `0.in-addr.arpa`、`127.in-addr.arpa`、`255.in-addr.arpa`。

新版 Go Agent、Windows Server 2008/2008 R2 legacy Agent 和后端同步服务都会过滤这些区域。如果旧版本曾同步入库，下一次当前 Agent 刷新或全量同步会按真实区域列表收敛删除。

主要实现位置：

- 后端路由：`backend/api/router/resources.go`
- 后端同步服务：`backend/internal/service/sync/service.go`
- 后端 DNS 仓储：`backend/internal/repository/dns.go`
- DNS 资源 ID：`backend/internal/repository/ids.go`
- DNS Agent 路由：`dns-agent/internal/server/server.go`
- DNS Agent PowerShell 实现：`dns-agent/internal/dns/`
- 前端 DNS 页面：`frontend/src/routes/_authenticated/dns.tsx`
- 前端数据层：`frontend/src/lib/dns-dhcp-store.ts`

## 二、数据库快照

DNS 区域写入 `dns_zones`：

|       字段       |         含义          |             来源             |
| :--------------: | :-------------------: | :--------------------------: |
|       `id`       | 后端生成的区域稳定 ID | `server_id + zone name` 编码 |
|      `name`      |       区域名称        |          DNS Agent           |
|      `type`      |       区域类型        | DNS Agent，新建区域默认 `Primary` |
|    `reverse`     |     是否反向区域      |          DNS Agent           |
| `dynamic_update` |     动态更新模式      | DNS Agent，新建区域默认 `None` |
|   `server_id`    |      所属服务器       |        平台服务器登记        |
|  `sync_status`   |       同步状态        |         后端同步服务         |
| `last_synced_at` |     最近同步时间      |         后端同步服务         |
|   `last_error`   |     最近同步错误      |         后端同步服务         |

DNS 记录写入 `dns_records`：

新版 DNS Agent 使用 `Get-DnsServerResourceRecord` 采集记录，并对 Windows Server 不同版本下的 `RecordData` 字段名做兼容解析，例如 A 记录会同时尝试 `IPv4Address` 和 `IPAddress`。

为保持 Go Agent 与 Windows Server 2008/2008 R2 legacy Agent 的同步口径一致，以下区域不写入业务快照：

- DNSSEC 内置信任锚区域 `TrustAnchors`。
- Windows DNS 内置反向区域 `0.in-addr.arpa`、`127.in-addr.arpa`、`255.in-addr.arpa`。

|       字段       |         含义          |                     来源                      |
| :--------------: | :-------------------: | :-------------------------------------------: |
|       `id`       | 后端生成的记录稳定 ID | `server_id + zone + type + name + value` 编码 |
|    `zone_id`     |      所属区域 ID      |                 后端同步服务                  |
|      `name`      |       记录名称        |                   DNS Agent                   |
|      `type`      |       记录类型        |                   DNS Agent                   |
|     `value`      |        记录值         |                   DNS Agent                   |
|      `ttl`       |       TTL 秒数        |           DNS Agent，缺省为 `3600`            |
|   `create_ptr`   |     是否创建 PTR      |             前端创建记录时可传入              |
| `last_synced_at` |     最近同步时间      |                 后端同步服务                  |
|   `updated_at`   |     平台更新时间      |                 后端同步服务                  |

DNS Agent 记录采集支持以下类型：

- `A`
- `AAAA`
- `CNAME`
- `MX`
- `TXT`
- `PTR`
- `NS`
- `SRV`
- `SOA`

如果 Windows DNS 返回的单条记录无法解析出有效值，或某条特殊记录在 PowerShell 解析时抛出异常，Agent 会跳过该记录并继续采集同区域后续记录，不会因为 SOA、未知类型值为空或单条记录结构异常而中断整个区域的记录枚举。

新建记录前，后端会直接基于 PostgreSQL 中的当前区域记录快照做冲突校验：

- 前端新建记录类型只开放 `A` 和 `CNAME`；`A` 按 Windows DNS“新建主机”语义提交为 `A` 记录。
- 名称、类型和值完全相同的记录已存在时，拒绝创建。
- 同名已存在 CNAME 记录时，拒绝创建其他类型记录。
- 同名已存在其他类型记录时，拒绝创建 CNAME 记录。
- A 记录值必须是合法 IPv4 地址。
- CNAME 记录先执行同名互斥校验，再校验记录值必须是以 `.` 结尾的合法域名。
- 校验失败时返回错误 toast，前端新建窗口保持打开。
- 创建、编辑、删除和冲突提示会展示记录名称、类型和值，例如 `www A 10.10.10.10 记录创建成功`。

编辑记录时，前端编辑窗口遵循以下规则：

- 只允许修改记录值。
- 名称、类型、TTL 和所属区域以禁用状态展示。
- 正向区域仅支持编辑和删除 A / CNAME 记录。
- 反向区域仅支持编辑和删除 PTR 记录。
- 其他记录类型的操作按钮会禁用并显示对应提示。
- A 记录会额外展示“更新相关的指针 PTR 记录”配置项，初始启用状态取自该记录创建或上次编辑时保存的 `create_ptr` 标记。
- 允许只切换 PTR 标记而不修改记录值。

后端仍以数据库快照为准读取旧记录，并执行以下校验：

- 名称、类型、所属区域发生变化时拒绝更新。
- 新值与旧值完全相同且 PTR 标记未变化时，后端不会调用 Agent 更新记录，但仍会返回当前记录、写入更新审计并按当前区域排队局部刷新。
- 同区域内已存在同名、同类型、同值的其他记录时拒绝更新。
- A 记录值必须是合法 IPv4 地址。
- CNAME 记录值必须是以 `.` 结尾的合法域名。
- PTR 记录值必须是以 `.` 结尾的合法域名。

新建 A 记录时默认勾选创建相关 PTR 记录。后端会按以下方式处理：

- 先根据 IPv4 地址推导对应的反向查找区域，例如 `10.10.10.10` 对应 `10.10.10.in-addr.arpa`。
- 再在数据库中的同一 Agent 反向区域快照中查找。
- 未找到对应反向区域时，后端仍会创建 A 记录，但不向 Agent 发送 PTR 创建请求。
- 后端会在响应中返回 `未找到参照的反向查找区域，无法创建 PTR 记录` 警告。
- 前端会用警告 toast 展示主记录创建成功和 PTR 警告。

编辑 A 记录并启用“更新相关的指针 PTR 记录”时，后端也会按新记录值检查对应反向查找区域。未找到对应反向区域时，主记录仍会更新成功，但不会向 Agent 发送 PTR 创建请求，并通过同样的警告 toast 提示 PTR 未创建。

找到反向查找区域且 Agent 成功创建 PTR 时：

- DNS Agent 会使用正向区域拼出 PTR 值，例如正向区域 `test.com` 下的 `www A 10.24.0.10` 会生成 `www.test.com.`。
- 后端会立即把对应 PTR 写入反向区域数据库快照，前端切换或搜索反向区域时无需等待下一轮区域刷新即可看到。
- 反向区域 PTR 记录名称按完整 IPv4 加尾点展示，例如 `10.24.0.10.`；Go Agent 和 Windows Server 2008/2008 R2 legacy Agent 刷新区域时都会把 Windows DNS 返回的相对名称规范化为完整 IPv4 展示，后端下发创建、编辑或删除命令时会转换为 Windows DNS 需要的区域内相对名称。
- 删除正向 A 记录时，后端会按名称和值查找实际匹配的 PTR 快照；无论 `create_ptr` 标记是否来自平台创建、编辑或同步反推，只要存在对应 PTR，都会请求 Agent 同步删除并立即删除数据库中的反向区域 PTR 快照。若只存在 `create_ptr` 标记但数据库中没有对应反向区域，后端不会凭 IP 推导创建反向区域刷新任务，避免把 Agent 上不存在的反向区域写入快照。
- Go DNS Agent 删除或编辑记录时，会复用采集逻辑中的 `RecordData` 字段兼容策略定位旧记录，例如 A 记录同时尝试 `IPv4Address` 和 `IPAddress`，避免同名同类型不同值记录只能精确按值匹配时误报 `DNS record not found`。
- Go DNS Agent 删除 CNAME、PTR、NS 等域名值记录时，会对目标值做去尾点和大小写不敏感比较；如果值没有精确匹配但同名同类型记录只有一条，会按该唯一记录兜底删除，避免数据库快照和 Windows DNS 当前值轻微不一致时误报 `DNS record not found`。

区域记录刷新采用“整区替换”策略：

- 后端先确保 `dns_zones` 中存在该区域。
- 读取 Agent 返回的当前区域记录。
- 局部刷新正向区域时，会按 A 记录 IP 推导可能关联的反向区域；只有该反向区域已存在于数据库快照中时，才会同步其记录，用于反推 A 记录的 `create_ptr` 标记并更新反向区域快照。
- 删除数据库中该 `zone_id` 下旧记录。
- 插入 Agent 返回的新记录。

这样可以避免 Windows DNS 中已删除的记录继续残留在页面里。

全量 DNS 同步还会按服务器收敛区域列表：

- Agent 返回的新区域会写入数据库。
- Agent 返回的已有区域会更新数据库。
- 同一 DNS 服务器下 Agent 不再返回的旧区域会从数据库删除。
- 区域删除会级联删除该区域下的 DNS 记录快照。

单服务器下 DNS 区域记录采集并发由「系统配置 / 基础配置 / 同步参数」中的 DNS 区域并发控制，可配置 `1` 到 `50` 个，并保存到 PostgreSQL。

DNS 区域创建和 DNS 记录变更后的二次同步等待时间由「系统配置 / 基础配置 / 同步参数」中的操作后刷新等待控制，默认 `10` 秒，可配置 `1` 到 `60` 秒。同一区域在等待窗口内继续发生 DNS 操作时，计时会重新开始；同一区域仍有操作正在执行时，后端会等操作结束后再开始等待窗口。

如果目标 DNS Agent 正在执行手动全量同步、DNS 定时全量同步或单 Agent 同步，DNS 区域和记录管理操作会返回“当前 Agent 正在同步，请稍后再操作”，避免操作与 Agent 同步任务并发访问同一 Agent。DNS 区域局部刷新不写入 Agent 同步运行态，只通过同目标刷新锁避免重复刷新。

DNS 区域创建、区域删除、记录创建、记录编辑和记录删除的整体超时时间由「系统配置 / 基础配置 / Agent 判定」中的 Agent 操作超时控制，默认 `20` 秒，可配置 `1` 到 `60` 秒。

DNS 区域同步和全量同步中的 DNS 记录采集由「系统配置 / 基础配置 / Agent 判定」中的 Agent 全量同步超时控制，默认 `300` 秒，可配置 `60` 到 `600` 秒。

Go DNS Agent 内部单次 PowerShell 命令另有 `DNS_AGENT_POWERSHELL_TIMEOUT_SECONDS` 超时保护：

- 默认值为 `180` 秒。
- 可配置范围为 `1` 到 `3600` 秒。
- 覆盖 Go Agent 内部的区域列表、区域记录读取、区域创建、区域删除和记录操作。
- legacy PowerShell Agent 不读取该变量，记录枚举继续使用原有 `dnscmd.exe` 直接调用方式。
- 大区域记录枚举耗时较长时，如果 Go Agent 内部 PowerShell 超时小于后端全量同步超时，后端可能收到连接被远端关闭或读取失败错误。

后端未读取到入库配置时，DNS 区域并发使用默认值 `3`，Agent 操作超时使用默认值 `20` 秒，Agent 全量同步超时使用默认值 `300` 秒。

## 三、刷新入口

### 3.1 页面打开

页面打开或浏览器刷新只读取数据库快照：

```text
frontend DNS page
  -> GET /api/state?includeDns=true
  -> PostgreSQL dns_zones / dns_records
```

不会触发：

- `/dns/zones`
- `/dns/zones/{zone}/records`
- `POST /api/refresh`

前端展示规则：

- 左侧区域列表支持按区域名称、区域类型以及正向 / 反向标识搜索，搜索只过滤当前 Agent 的数据库快照。
- 左侧区域排序先显示正向区域，再显示反向区域，同类区域按自然顺序升序。
- 右侧记录默认按记录名称层级排序，同层级下按域名 label 从右向左自然比较，让同一父级或后缀下的二级、三级记录相邻展示。
- 右侧名称、类型和值表头仍支持升序、降序和不排序三态切换。
- 右侧记录列表默认只渲染前 `200` 条，底部显示“已显示 x / 总数 条”和“加载更多”，点击后按 `200` 条继续展示；搜索和排序仍基于当前区域全部记录。

### 3.2 手动全量刷新

顶部工具栏全量刷新调用：

```text
POST /api/refresh
```

默认任务类型为：

```text
runtime.refresh.all
```

后端会遍历所有可同步 Agent。可同步 Agent 指已登记、配置了 Agent URL，且角色匹配本次刷新类型的服务器：

- `DNS` 角色服务器执行 DNS 同步。
- `DHCP` 角色服务器执行 DHCP 同步。
- 同步完成后发布 `runtime.updated`。

### 3.3 定时全量刷新

后端通过 `RUNTIME_DNS_DEEP_SYNC_INTERVAL` 周期执行 DNS 全量同步：

- 默认 `1d`，即每天执行一次。
- 支持 `m`、`h`、`d` 单位，例如 `30m`、`2h`、`1d`。
- 能整除 24 小时的短间隔按本地当天零点对齐；按天配置的间隔按本地自然日零点对齐，例如 `2d` 从当前本地日期零点起算下一个两天边界；不能整除 24 小时且不是整天数的间隔按 Unix epoch `1970-01-01 00:00:00 UTC` 起算的固定周期对齐。
- 设置为 `0` 可关闭 DNS 定时全量同步。
- 创建 `runtime.refresh.dns.all` 任务，只同步 DNS Agent。
- 不会因为前端页面刷新而触发。

DNS Agent 状态更新规则如下：

- 后台同步会先访问 Agent `/health`；后端自动连通性检查只检查已配置 Agent URL 且当前未处于同步中的 Agent。
- 后端自动连通性检查的检查间隔和并发数量由「系统配置 / 基础配置 / Agent 判定」控制，默认每 `1` 分钟串行检查。
- `/health` 失败会累计 `servers.failure_count`。
- 连续失败次数达到「系统配置 / 基础配置 / Agent 判定」中的离线失败次数后，服务器状态才会标记为 `Offline`，并在状态从非 `Offline` 进入 `Offline` 时创建 Agent 离线通知。
- 仪表板或设置页手动测试连接失败会立即标记为 `Offline`。
- 健康检查或同步成功后会恢复为 `Online`，清零失败计数、更新最近检查时间并自动清理对应 Agent 的离线通知。
- 如果 `/health` 成功但后续 DNS 资源同步失败，刷新任务会记录失败，不按 Agent 离线失败次数累计。

### 3.4 当前 Agent 刷新

DNS 管理页标题行右侧的当前 Agent 选择框只过滤 PostgreSQL 中已同步的 DNS 快照，不会因为切换 Agent 直接访问 Windows DNS。

选择当前 DNS Agent 后点击「刷新」会调用：

```text
POST /api/servers/{id}/sync
```

后端创建 `runtime.refresh.server` 任务，只同步该 Agent 对应角色的数据，并在完成后发布 `runtime.updated` 让页面重新读取数据库快照。

前端会在任务执行期间让刷新图标保持旋转，并显示固定 toast，toast 文本可复制且保留关闭按钮。同步完成后会更新为完成提示，并在 3 秒后自动隐藏。

### 3.5 区域级刷新

DNS 区域卡片右侧刷新按钮调用：

```text
POST /api/dns/zones/{id}/refresh
```

目标 Agent 未同步且同目标刷新未运行时，后端创建任务：

```text
runtime.refresh.dns.zone
```

如果目标 Agent 正在同步或同目标刷新正在运行，后端返回冲突提示且不创建任务。创建成功后，该任务只访问当前区域：

```text
POST /dns/records/query
```

不会刷新其他区域，也不会遍历所有 DNS 服务器。

### 3.6 DNS 区域创建

新建 DNS 区域时，前端不再单独选择服务器。后端会使用 DNS 管理页标题行右侧当前选择的 DNS Agent 作为目标服务器。

新建弹窗只保留以下输入：

- 正向 / 反向查找区域模式。
- 区域名称。

前端不再提供区域类型和动态更新选择。提交时仍按后端当前契约传递默认值：

- `type`: `Primary`
- `dynamicUpdate`: `None`

创建正向查找区域时按以下规则处理：

- 前端只需要提交域名区域，例如 `example.com`。
- Go DNS Agent 按 Primary Zone 创建。
- 后端写入数据库快照时保留 Agent 返回或默认补齐的区域类型和动态更新字段。

创建反向查找区域时按以下规则处理：

- 前端只需要提交网络 ID，例如 `1.168.192`。
- 后端会自动补充 `.in-addr.arpa` 后缀后再调用 DNS Agent 创建区域。
- 数据库快照以补全后的区域名写入。
- Go DNS Agent 同样按 Primary Zone 创建，不再由弹窗选择区域类型。
- Go DNS Agent 下发 PowerShell 时会把完整反向区域名转换回 `-NetworkId` 需要的网络 ID，避免把 `1.168.192.in-addr.arpa` 误当作网络 ID。

Go DNS Agent 创建区域时按以下顺序执行：

- 优先尝试 AD 集成 Primary Zone，并使用 `ReplicationScope=Domain`。
- 如果目标 DNS 服务器不支持或不适用 AD 集成，会回退为文件型 Primary Zone。
- 创建命令返回后，Agent 会再执行 `Get-DnsServerZone` 确认该区域已存在。
- 确认失败时接口返回错误，后端不会写入成功快照。

DNS 管理右侧记录列表在选中反向查找区域时禁用新建记录入口，避免从正向记录表单误创建 PTR 类记录。

创建成功后：

1. 后端先调用 DNS Agent 创建 Windows DNS 区域。
2. Agent 创建并确认区域存在后，后端写入 PostgreSQL 区域快照。
3. 后端会立即读取该区域记录，并把 Windows DNS 自动生成的 SOA、NS 等默认记录写入 PostgreSQL 快照，响应中通过 `records` 返回给前端。
4. 前端会把新区域和默认记录同时合并进本地缓存，因此创建成功后无需等待后台刷新即可看到默认记录。
5. 如果即时读取或写入默认记录失败，区域创建仍返回成功，并通过 `warning` 告知前端；后端仍会按区域延迟合并创建刷新任务作为兜底。
6. 后端写入 `Created zone` 审计记录。
7. 后端按区域延迟合并创建 `runtime.refresh.dns.zone` 任务，用于最终收敛新区域记录。
8. 前端收到 SSE 事件后重新读取数据库。

### 3.7 DNS 记录变更后的刷新

新增、编辑或删除 DNS 记录成功后：

1. 后端先调用 DNS Agent 执行变更。
2. 创建记录成功后，后端会先写入该记录的本地快照，让前端能立即显示；如果同步创建了 PTR，也会同时写入反向区域 PTR 快照。
3. 删除记录成功后，后端会先删除该记录的本地快照，让前端能立即移除；如果该 A 记录存在实际匹配的 PTR 快照，也会同步移除关联 PTR 快照。
4. 编辑记录时，后端优先调用 DNS Agent 的 body 版更新接口；旧 Agent 不支持该接口并返回 `404` 时，后端会回退为删除旧记录并创建新记录。
5. 编辑记录成功后，后端会删除旧记录快照并写入新记录快照，让前端能立即显示新值；如果 A 记录带 `create_ptr` 标记，也会同步维护新旧 PTR 快照。
6. 成功后写入审计记录。
7. 后端按当前区域延迟合并创建 `runtime.refresh.dns.zone` 任务；如果涉及已存在的 PTR 或反向区域，也会按关联反向区域合并创建刷新任务。
8. 区域刷新任务完成后用 Agent 快照最终收敛数据库记录。
9. 前端收到 SSE 事件后重新读取数据库。

### 3.8 DNS 区域记录导出

DNS 管理页标题行右侧的「导出」按钮会先重新读取当前 `includeDns=true` 状态，再按当前选中的 DNS Agent 过滤区域和记录。

导出范围支持：

- 全部：导出当前 Agent 的全部 DNS 区域记录。
- 正向：只导出正向查找区域记录。
- 反向：只导出反向查找区域记录。
- 自定义：输入区域名称时实时模糊搜索，必须从下拉结果中点击已有区域后才会加入导出区域标签。

导出格式支持 `XLSX`、`XLS`、`CSV` 和 `TXT`。导出表第一列固定为区域名称，即使同一区域包含多条记录，每条记录行的第一列都会写入该区域名称。

导出行按 DNS 管理页左侧区域列表的口径排序：先正向区域、后反向区域，同一方向内按区域名称自然升序排序；同一区域内记录按当前记录默认自然顺序输出。

## 四、Agent 接口

DNS Agent 提供以下接口：

|                              接口                              |                           用途                            |
| :------------------------------------------------------------: | :-------------------------------------------------------: |
|                         `GET /health`                          |                         健康检查                          |
|                        `GET /dns/zones`                        |                     读取 DNS 区域列表                     |
|                       `POST /dns/zones`                        |                       创建 DNS 区域                       |
|                   `DELETE /dns/zones/{zone}`                   |                       删除 DNS 区域                       |
|                `GET /dns/zones/{zone}/records`                 |                     读取指定区域记录                      |
|                   `POST /dns/records/query`                    | 读取指定区域记录，区域名通过 JSON body 的 `zone` 字段传递 |
|                   `POST /dns/records/create`                   | 创建指定区域记录，区域名通过 JSON body 的 `zone` 字段传递 |
|                   `POST /dns/records/delete`                   | 删除指定区域记录，区域名通过 JSON body 的 `zone` 字段传递 |
|                   `POST /dns/records/update`                   | 更新指定区域记录，区域名通过 JSON body 的 `zone` 字段传递 |
|                `POST /dns/zones/{zone}/records`                |          创建指定区域记录，保留用于兼容旧 Agent           |
|                 `PUT /dns/zones/{zone}/records`                |          更新指定区域记录，保留用于兼容旧 Agent           |
| `DELETE /dns/zones/{zone}/records/{type}/{name}?value={value}` |            删除指定记录，保留用于兼容旧 Agent             |

后端读取指定区域记录时优先调用 `POST /dns/records/query`，避免区域名出现在明文 HTTP URL 路径里，被网络安全设备或代理按域名关键字误拦截。以下情况会回退到 `GET /dns/zones/{zone}/records` 兼容旧版本：

- 目标 Agent 尚未支持 body 版接口并返回 `404`。
- Windows Server 2008/2008 R2 legacy Agent 缺少 `.NET System.Web.Extensions`，导致 JSON body 解析返回 `500`。

后端创建、删除和编辑 DNS 记录时同样优先调用 body 版接口：

- `/dns/records/create`
- `/dns/records/delete`
- `/dns/records/update`

这样可以避免 `youtube.com` 等特殊区域名出现在 HTTP URL 路径里。若目标 Agent 返回 `404`，后端再回退到旧路径接口以兼容 Windows Server 2008/2008 R2 legacy Agent。

Go DNS Agent 对写入类操作做了以下合并：

- 创建 A 记录并启用 PTR 时，会在同一个 PowerShell 脚本中完成主记录创建和 PTR best-effort 处理，避免为主记录与 PTR 分别启动两次 PowerShell。
- 编辑记录时，主记录的旧值查找、删除和新值创建也会在同一个 PowerShell 脚本中完成，避免为“删旧记录 + 建新记录”分别启动两次 PowerShell。
- A 记录关联 PTR 的清理和编辑后的创建仍按 best-effort 独立处理，避免反向区域缺失影响主记录更新结果。

Windows Server 2008/2008 R2 legacy PowerShell Agent 的兼容规则如下：

- 支持 `/dns/records/query`、`/dns/records/create`、`/dns/records/delete` 和 `/dns/records/update` body 版接口。
- 旧路径接口仍保留作为兼容兜底。
- 在创建、删除、编辑和 PTR best-effort 处理前查找指定记录时，会优先通过 `dnscmd.exe /EnumRecords` 枚举目标节点。
- 只有目标节点查询失败时才回退到全区域枚举，避免大区域写操作频繁扫描整个区域。

业务接口需要携带：

```text
X-API-Key: <DNS_AGENT_API_KEY>
```

除非 Agent 显式开启匿名访问。

## 五、任务、审计和 SSE

任务表：

- `refresh_tasks.type=runtime.refresh.all`
- `refresh_tasks.type=runtime.refresh.dns.all`
- `refresh_tasks.type=runtime.refresh.server`
- `refresh_tasks.type=runtime.refresh.dns.zone`

当前 DNS 相关审计 action：

- `Queued server sync`
- `Created zone`
- `Deleted zone`
- `Created DNS record`
- `Updated DNS record`
- `Deleted DNS record`
- `Queued DNS zone refresh`

SSE 事件：

- `runtime.refresh.all`
- `runtime.refresh.dns.all`
- `runtime.refresh.server`
- `runtime.refresh.dns.zone`
- `runtime.updated`

Redis 当前用于刷新事件发布和最近刷新事件缓存，不作为 DNS 明细唯一数据源。

## 六、后续变更同步要求

涉及以下变更时，必须同步更新本文档：

- DNS Agent PowerShell 命令变化。
- DNS 区域或记录字段采集、解析、默认值、ID 生成规则变化。
- DNS 创建、删除、刷新链路变化。
- `runtime.refresh.all`、`runtime.refresh.dns.all`、`runtime.refresh.dns.zone` 事件或任务 payload 变化。
- DNS 页面刷新按钮、加载状态、SSE 订阅或缓存读取规则变化。
- DNS 相关数据库表、索引、唯一约束或快照替换策略变化。
