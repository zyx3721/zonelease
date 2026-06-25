# 任务、审计与通知日志覆盖说明

本文档说明平台“任务 / 审计 / 通知”三类记录的定位、写入边界、当前覆盖范围和后续开发时的同步要求。

## 一、日志类型边界

### 1.1 任务日志

任务日志写入 `refresh_tasks` 表，主要用于记录后台刷新、局部同步和可追踪进度的操作。

适合写入任务日志的场景：

- 后端创建后台任务后立即返回前端。
- 操作耗时较长，需要排队、运行中、完成或失败状态。
- 任务执行期间需要持续更新 payload。
- 前端需要通过 `/api/refresh/tasks` 或 SSE 展示进度。

当前任务状态包括：

- `queued`：任务已排队。
- `running`：任务运行中。
- `completed`：任务已完成。
- `failed`：任务失败。

任务类型包括：

- `runtime.refresh.all`：手动或定时全量刷新。
- `runtime.refresh.fast`：兼容的快速刷新类型，当前进入全量同步链路。
- `runtime.refresh.server`：同步指定服务器 Agent 数据。
- `runtime.refresh.dns.zone`：刷新指定 DNS 区域记录。
- `runtime.refresh.dhcp.scope`：刷新指定 DHCP 作用域快照。

任务 payload 应包含用户能理解的关键字段，例如：

- `message`：当前状态中文描述。
- 操作对象名称，例如 `serverName`、`zoneName` 或 `scopeName`。
- 所属服务器或 Agent 标识。
- 全量刷新进度字段，例如 `totalAgents`、`syncedAgents`、`failedAgents`、`currentAgent`、`startedAt`、`finishedAt` 和 `agentResults`；任务完成或失败后也应保留最终聚合快照。
- 目标定位字段，例如 `targetType` 和 `targetId`。
- 失败原因，例如 `error`。

### 1.2 审计日志

审计日志写入 `audit_entries` 表，主要用于记录用户成功触发的关键操作。

适合写入审计日志的场景：

- 用户登录和修改密码。
- 用户创建、删除、禁用、角色和用户组配置。
- 系统基础配置、通知配置、认证配置变更或测试。
- 服务器登记、删除、同步和测试连接。
- DNS 区域与记录创建、删除和区域刷新。
- DHCP 作用域、租约和保留地址操作。
- 手动全量刷新。
- 通知中心已读和清空。

审计 action 当前同时存在历史英文短语和点分命名。新增 action 应优先使用稳定的点分命名，例如：

- `settings.user.create`
- `settings.notification.test`
- `notifications.read_all`

审计 detail 应避免写入敏感内容：

- 不写入 Bearer Token。
- 不写入 API Key、密码、验证码。
- 不写入 SMTP 密码、LDAP bind 密码等敏感配置。
- 不写入未脱敏的完整请求体。
- 只写入对象名称、开关状态、目标资源、失败原因等排查必需信息。

审计日志会同步记录客户端 IP，来源按 `X-Forwarded-For`、`X-Real-IP`、`RemoteAddr` 顺序解析。前端审计列表将 `detail` 作为“审计元数据”展示，并把动作归一为点分命名、资源归一为 `resourceType/resourceId` 结构，便于和任务记录交叉排查。

### 1.3 通知中心

通知中心消息写入 `notifications` 表，主要用于把 Agent 异常和平台基础服务异常展示给前端。

当前通知来源包括：

- Agent 健康检查连续失败达到离线判定次数并写为 `Offline`。
- PostgreSQL 连接异常。
- Redis 连接异常。

通知中心写入和清理规则如下：

- 刷新任务排队、完成或失败只通过 `refresh_tasks`、SSE 状态和前端 toast 展示，不写入通知中心。
- Agent 异常和平台基础服务异常会计入未读红点。
- 同一 Agent 或同一基础服务已有未读异常通知时，不重复创建同源通知。
- Agent 后续恢复 `Online` 时，对应 Agent 异常通知会由系统自动标记已读并清空，不额外写用户审计。
- PostgreSQL 或 Redis 后续恢复在线时，对应平台基础服务异常通知也会由系统自动标记已读并清空，不额外写用户审计。

PostgreSQL 异常会尽力写入通知中心；如果数据库不可用导致 `notifications` 表无法写入，则该条异常无法保证落库。

通知中心支持的用户操作包括：

- 单条通知标记已读。
- 全部通知标记已读。
- 清空通知中心消息。

这些用户操作成功后会写入 `audit_entries`，但不会改变 `refresh_tasks`。

### 1.4 日志保留

后端启动后会立即执行一次日志保留清理，随后每 24 小时执行一次。

保留天数由 `LOG_RETENTION_DAYS` 控制：

- 默认值为 `30` 天。
- 设为小于等于 `0` 时关闭自动清理。
- `refresh_tasks` 按 `created_at` 清理。
- `audit_entries` 按 `ts` 清理。
- `notifications` 按 `created_at` 清理。

清理只按记录创建或发生时间判断，不依赖任务状态、完成时间、通知已读状态或清空状态。

## 二、当前覆盖范围

### 2.1 认证与账号

| 操作 | 任务 | 审计 | 通知 | 说明 |
| :-: | :-: | :-: | :-: | :-: |
| 登录 | 否 | 是 | 否 | 写入 `User login` |
| 登出 | 否 | 否 | 否 | 删除当前会话，不写审计 |
| 获取当前用户 | 否 | 否 | 否 | 只读查询 |
| 修改当前用户密码 | 否 | 是 | 否 | 写入 `Changed password` |
| 获取公开认证方式 | 否 | 否 | 否 | 登录页只读查询 |
| 找回密码图形验证码 | 否 | 否 | 否 | 认证流程辅助动作 |
| 找回密码身份校验 | 否 | 否 | 否 | 认证流程辅助动作 |
| 找回密码发送验证码 | 否 | 否 | 否 | 当前不写审计 |
| 找回密码确认重置 | 否 | 否 | 否 | 当前不写审计；成功后会清空该用户所有 sessions |

### 2.2 服务器与 Agent

| 操作 | 任务 | 审计 | 通知 | 说明 |
| :-: | :-: | :-: | :-: | :-: |
| 服务器列表读取 | 否 | 否 | 否 | 通过 `GET /api/state` 返回 |
| 新建服务器 | 否 | 是 | 否 | 唯一性校验通过并创建成功后写入 `Created server` |
| 新建前测试连接 | 否 | 否 | 否 | 未保存资源前的临时校验 |
| 删除服务器 | 否 | 是 | 否 | 写入 `Deleted server` |
| 已保存服务器测试连接 | 否 | 是 | 否 | 写入 `Checked server health`，结果为 `success` 或 `failed` |
| 后端自动健康检查 | 否 | 否 | 是 | 按 Agent 连通性检查间隔更新服务器状态；状态从非 `Offline` 进入 `Offline` 时创建离线通知 |
| 同步单个服务器 Agent | 是 | 是 | 否 | 创建 `runtime.refresh.server` 任务，写入 `Queued server sync` |
| 定时全量刷新 | 是 | 否 | 否 | 创建 `runtime.refresh.all` 任务，不写用户审计 |
| 手动全量刷新 | 是 | 是 | 否 | 创建 `runtime.refresh.all` 任务，写入 `Queued refresh` |

### 2.3 DNS 管理

| 操作 | 任务 | 审计 | 通知 | 说明 |
| :-: | :-: | :-: | :-: | :-: |
| DNS 区域和记录读取 | 否 | 否 | 否 | 通过 `GET /api/state` 返回数据库快照 |
| 创建 DNS 区域 | 是 | 是 | 否 | 写入 `Created zone`，并排队 `runtime.refresh.dns.zone` |
| 删除 DNS 区域 | 否 | 是 | 否 | 写入 `Deleted zone`，删除快照后发布 `runtime.updated` |
| 刷新指定 DNS 区域 | 是 | 是 | 否 | 创建 `runtime.refresh.dns.zone` 任务，写入 `Queued DNS zone refresh` |
| 创建 DNS 记录 | 是 | 是 | 否 | 写入 `Created DNS record`，并排队 `runtime.refresh.dns.zone` |
| 编辑 DNS 记录 | 是 | 是 | 否 | 写入 `Updated DNS record`，并排队 `runtime.refresh.dns.zone` |
| 删除 DNS 记录 | 是 | 是 | 否 | 写入 `Deleted DNS record`，并排队 `runtime.refresh.dns.zone` |

### 2.4 DHCP 管理

| 操作 | 任务 | 审计 | 通知 | 说明 |
| :-: | :-: | :-: | :-: | :-: |
| DHCP 作用域、租约和保留地址读取 | 否 | 否 | 否 | 通过 `GET /api/state` 返回数据库快照 |
| 创建 DHCP 作用域 | 是 | 是 | 否 | 写入 `Created DHCP scope`，并排队 `runtime.refresh.dhcp.scope` |
| 更新 DHCP 作用域 | 是 | 是 | 否 | 写入 `Updated DHCP scope`，并排队 `runtime.refresh.dhcp.scope` |
| 切换 DHCP 作用域状态 | 是 | 是 | 否 | 写入 `Toggled DHCP scope`，并排队 `runtime.refresh.dhcp.scope` |
| 刷新指定 DHCP 作用域 | 是 | 是 | 否 | 创建 `runtime.refresh.dhcp.scope` 任务，写入 `Queued DHCP scope refresh` |
| 删除 DHCP 作用域 | 否 | 是 | 否 | 写入 `Deleted DHCP scope`，删除快照后发布 `runtime.updated` |
| 释放 DHCP 租约 | 是 | 是 | 否 | 写入 `Released DHCP lease`，并排队 `runtime.refresh.dhcp.scope` |
| 创建 DHCP 保留地址 | 是 | 是 | 否 | 写入 `Created DHCP reservation`，并排队 `runtime.refresh.dhcp.scope` |
| 更新 DHCP 保留地址 | 是 | 是 | 否 | 写入 `Updated DHCP reservation`，并排队 `runtime.refresh.dhcp.scope` |
| 删除 DHCP 保留地址 | 是 | 是 | 否 | 写入 `Deleted DHCP reservation`，并排队 `runtime.refresh.dhcp.scope` |

### 2.5 系统配置与权限

| 操作 | 任务 | 审计 | 通知 | 说明 |
| :-: | :-: | :-: | :-: | :-: |
| 读取公开基础配置 | 否 | 否 | 否 | 登录页、启动页和控制台布局使用 |
| 读取基础配置 | 否 | 否 | 否 | 只读查询 |
| 更新基础配置 | 否 | 是 | 否 | 写入 `Updated system base config` |
| 用户列表读取 | 否 | 否 | 否 | 只读查询 |
| 用户创建、更新、禁用、删除 | 否 | 是 | 否 | 写入 `settings.user.*` |
| 角色创建、更新、删除 | 否 | 是 | 否 | 写入 `settings.role.*` |
| 用户组创建、更新、删除 | 否 | 是 | 否 | 写入 `settings.user_group.*` |
| 权限列表读取 | 否 | 否 | 否 | 只读查询 |
| 认证配置读取 | 否 | 否 | 否 | 只读查询 |
| 认证配置更新 | 否 | 是 | 否 | 写入 `settings.auth_provider.update` |
| 认证配置测试 | 否 | 是 | 否 | 写入 `settings.auth_provider.test` |
| 通知媒介读取 | 否 | 否 | 否 | 只读查询 |
| 通知媒介更新 | 否 | 是 | 否 | 写入 `settings.notification.update` |
| 通知媒介测试 | 否 | 是 | 否 | 写入 `settings.notification.test` |
| 通知模板预览 | 否 | 否 | 否 | 只做预览，不写审计 |

### 2.6 通知中心

| 操作 | 任务 | 审计 | 通知 | 说明 |
| :-: | :-: | :-: | :-: | :-: |
| 通知列表读取 | 否 | 否 | 否 | 只读查询 |
| 未读数量读取 | 否 | 否 | 否 | 只读查询 |
| 单条通知标记已读 | 否 | 是 | 否 | 写入 `notifications.read` |
| 全部通知标记已读 | 否 | 是 | 否 | 写入 `notifications.read_all` |
| 清空通知中心 | 否 | 是 | 否 | 写入 `notifications.clear` |
| Agent 异常通知 | 否 | 否 | 是 | Agent 健康检查连续失败达到离线判定次数并写为 `Offline` 时创建，同源未读不重复 |
| Agent 恢复后自动清空异常通知 | 否 | 否 | 否 | Agent 健康检查或同步恢复 `Online` 时自动清空同源通知，不写用户审计 |
| 平台基础服务异常通知 | 否 | 否 | 是 | PostgreSQL 或 Redis 连接异常时创建，同源未读不重复 |
| 平台基础服务恢复后自动清空异常通知 | 否 | 否 | 否 | PostgreSQL 或 Redis 健康检查恢复 `online` 时自动清空同源通知，不写用户审计 |

## 三、失败记录规则

### 3.1 同步接口失败

同步接口在请求内立即执行预检查或 Agent 调用，失败时直接返回错误。

当前规则：

- 成功后写审计日志。
- 参数校验失败、权限失败和资源不存在通常不写审计。
- Agent 调用失败通常不写审计，直接返回用户可见错误。
- 已保存服务器测试连接会写审计，并用 `result` 区分 `success` 或 `failed`。
- 后端自动健康检查不写审计，只更新服务器健康状态和最近检查时间；状态从非 `Offline` 进入 `Offline` 时创建 Agent 离线通知。

典型接口：

- `POST /api/servers/{id}/ping`
- `POST /api/dns/zones`
- `DELETE /api/dns/zones/{id}`
- `POST /api/dhcp/scopes`

### 3.2 异步任务失败

异步刷新任务已经完成排队，后续在后台执行。

当前规则：

- 任务创建后写入 `refresh_tasks`，初始状态为 `queued`。
- 任务开始后更新为 `running`。
- Agent 执行或同步失败时更新为 `failed`。
- 任务失败不写入通知中心消息，由任务状态、payload 错误信息和 SSE 状态展示。
- 当前异步刷新任务失败不额外写 `.failed` 审计 action。

### 3.3 普通请求失败

普通请求失败通常不写审计。

不写审计的原因：

- 参数格式错误可能来自误操作或扫描请求。
- 权限失败可能产生大量噪音。
- 资源不存在或预检查失败未真正改变系统状态。

如果未来需要审计高风险失败尝试，应单独设计失败审计策略，避免影响操作审计页面可读性。

## 四、后续开发同步要求

### 4.1 新增或修改用户操作

新增或修改用户可触发操作时，必须评估以下问题：

- 是否改变平台状态、Windows DNS / DHCP 资源、系统配置或通知状态。
- 是否需要写审计日志。
- 是否是后台长耗时操作。
- 是否需要写任务日志。
- 是否会创建通知中心消息。
- 是否需要同步 README、本文档和相关运行文档。

### 4.2 审计字段要求

新增审计日志时，detail 至少应包含：

- 用户可识别的资源名称。
- 服务器、Agent、DNS 区域、DHCP 作用域或目标资源名称。
- 操作目标和关键开关值。
- 失败审计中的用户可读错误信息。

detail 禁止包含：

- Bearer Token。
- Agent API Key。
- 登录密码、SMTP 密码、LDAP bind 密码、找回密码验证码。
- 未脱敏的完整请求体。

### 4.3 任务字段要求

新增任务日志时，payload 至少应包含：

- `message`：当前状态中文描述。
- 操作对象名称，例如 `serverName`、`zoneName` 或 `scopeName`。
- 所属服务器或 Agent 标识。

如果任务遍历多个 Agent，应包含：

- `totalAgents`
- `syncedAgents`
- `failedAgents`
- `currentAgent`

### 4.4 通知字段要求

新增通知中心消息时，应保证：

- `level` 使用稳定枚举，例如 `info`、`success`、`critical`。
- `title` 面向用户可读。
- `message` 能说明当前状态或失败原因。
- `source_type` 和 `source_id` 能定位来源资源。
- `metadata` 不包含敏感信息。

## 五、文档同步清单

修改任务、审计或通知相关功能时，需要同步检查：

- `docs/operation-log-coverage.md`
- `docs/refresh-sync.md`
- `docs/dns-management.md`
- `docs/dhcp-management.md`
- `README.md`
- `AGENTS.md`

如果只是新增审计 action、调整任务 payload 或新增通知中心消息，通常更新本文档和 README 简述即可。

如果日志变更伴随刷新链路变化，需要同步更新 `docs/refresh-sync.md`。

如果日志变更伴随 DNS / DHCP Agent 采集或操作命令变化，需要同步更新 `docs/dns-management.md` 或 `docs/dhcp-management.md`。
