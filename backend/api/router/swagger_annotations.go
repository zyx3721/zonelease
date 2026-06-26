package router

// swaggerHealth godoc
// @Summary 健康检查
// @Description 返回 PostgreSQL 与 Redis 状态；检测到平台基础服务异常时会尽力写入通知中心，PostgreSQL 不可用时可能无法落库；后续恢复在线时会自动清空对应异常通知。
// @Tags Health
// @Produce json
// @Success 200 {object} healthResponse
// @Router /api/health [get]
func swaggerHealth() {}

// swaggerLogin godoc
// @Summary 登录
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body loginRequest true "登录参数"
// @Success 200 {object} loginResponse
// @Failure 400 {object} errorDocResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/auth/login [post]
func swaggerLogin() {}

// swaggerPublicAuthProviders godoc
// @Summary 获取公开认证方式
// @Description 无需登录，登录页用于读取已启用的本地或 AD/LDAP 认证方式。
// @Tags Auth
// @Produce json
// @Success 200 {object} publicAuthProviderListResponse
// @Router /api/auth/providers [get]
func swaggerPublicAuthProviders() {}

// swaggerLogout godoc
// @Summary 注销当前会话
// @Tags Auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} statusResponse
// @Router /api/auth/logout [post]
func swaggerLogout() {}

// swaggerMe godoc
// @Summary 获取当前用户
// @Tags Auth
// @Produce json
// @Security BearerAuth
// @Success 200 {object} meResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/auth/me [get]
func swaggerMe() {}

// swaggerPasswordResetCaptcha godoc
// @Summary 获取找回密码图形验证码
// @Description 返回的 token 使用 JWT_SECRET 签名并携带过期时间，不写入 Redis。
// @Tags PasswordReset
// @Produce json
// @Success 200 {object} captchaResponse
// @Router /api/auth/password-reset/captcha [get]
func swaggerPasswordResetCaptcha() {}

// swaggerPasswordResetVerify godoc
// @Summary 校验找回密码用户名与图形验证码
// @Description 校验顺序为图形验证码、账号可找回状态和找回密码媒介可用性；用户不存在、禁用、非本地账号或未配置邮箱时返回 password_reset_unavailable，未启用找回密码邮件媒介时返回 no_password_reset_channel。
// @Tags PasswordReset
// @Accept json
// @Produce json
// @Param body body resetVerifyRequest true "校验参数"
// @Success 200 {object} verifyResetResponse
// @Failure 400 {object} errorDocResponse
// @Failure 503 {object} errorDocResponse
// @Router /api/auth/password-reset/verify [post]
func swaggerPasswordResetVerify() {}

// swaggerPasswordResetSend godoc
// @Summary 发送找回密码验证码
// @Tags PasswordReset
// @Accept json
// @Produce json
// @Param body body resetSendRequest true "发送参数"
// @Success 200 {object} sendResetResponse
// @Failure 400 {object} errorDocResponse
// @Failure 429 {object} errorDocResponse
// @Router /api/auth/password-reset/send [post]
func swaggerPasswordResetSend() {}

// swaggerPasswordResetConfirm godoc
// @Summary 确认找回密码并重置密码
// @Description 重置成功后会删除该用户在 sessions 表中的所有登录会话。
// @Tags PasswordReset
// @Accept json
// @Produce json
// @Param body body resetConfirmRequest true "重置参数"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorDocResponse
// @Router /api/auth/password-reset/confirm [post]
func swaggerPasswordResetConfirm() {}

// swaggerChangePassword godoc
// @Summary 修改当前用户密码
// @Tags Auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body changePasswordRequest true "修改密码参数"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorDocResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/auth/change-password [post]
func swaggerChangePassword() {}

// swaggerState godoc
// @Summary 获取控制台状态
// @Tags State
// @Produce json
// @Security BearerAuth
// @Param includeDns query bool false "兼容参数；DNS 区域与记录始终读取数据库快照"
// @Success 200 {object} stateResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/state [get]
func swaggerState() {}

// swaggerEvents godoc
// @Summary 订阅运行态刷新事件
// @Tags Realtime
// @Produce text/event-stream
// @Success 200 {string} string "SSE event stream"
// @Router /api/events [get]
func swaggerEvents() {}

// swaggerCreateRefresh godoc
// @Summary 创建刷新任务
// @Tags Refresh
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body createRefreshRequest true "刷新参数"
// @Success 202 {object} refreshTaskResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/refresh [post]
func swaggerCreateRefresh() {}

// swaggerRefreshTasks godoc
// @Summary 获取刷新任务列表
// @Tags Refresh
// @Produce json
// @Security BearerAuth
// @Param limit query string false "返回数量，默认 30；传 all 时返回全部"
// @Success 200 {object} refreshTaskListResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/refresh/tasks [get]
func swaggerRefreshTasks() {}

// swaggerNotifications godoc
// @Summary 获取通知中心消息
// @Tags Notifications
// @Produce json
// @Security BearerAuth
// @Param limit query int false "返回数量，默认 20，最大 100"
// @Success 200 {object} notificationListResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/notifications [get]
func swaggerNotifications() {}

// swaggerUnreadNotificationCount godoc
// @Summary 获取未读通知数量
// @Description 返回右上角通知图标红点数量；刷新任务不写入通知中心，也不会计入该未读数量。
// @Tags Notifications
// @Produce json
// @Security BearerAuth
// @Success 200 {object} unreadNotificationCountResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/notifications/unread-count [get]
func swaggerUnreadNotificationCount() {}

// swaggerReadNotification godoc
// @Summary 标记单条通知已读
// @Tags Notifications
// @Produce json
// @Security BearerAuth
// @Param id path string true "通知 ID"
// @Success 200 {object} statusResponse
// @Failure 401 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Router /api/notifications/{id} [post]
func swaggerReadNotification() {}

// swaggerReadAllNotifications godoc
// @Summary 标记全部通知已读
// @Tags Notifications
// @Produce json
// @Security BearerAuth
// @Success 200 {object} statusResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/notifications/read-all [post]
func swaggerReadAllNotifications() {}

// swaggerClearNotifications godoc
// @Summary 清空通知中心消息
// @Tags Notifications
// @Produce json
// @Security BearerAuth
// @Success 200 {object} statusResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/notifications/clear [post]
func swaggerClearNotifications() {}

// swaggerCreateServer godoc
// @Summary 添加服务器
// @Description 保存前会检查 Agent 名称、接口地址是否重复和角色是否合法；Agent 连通性由前端先调用 /api/servers/probe 完成，保存接口不再重复访问 Agent。
// @Tags Servers
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body serverResponse true "服务器参数，包含 name、role、agentUrl、可选 apiKey 和可选 tlsInsecure"
// @Success 201 {object} serverResponse
// @Failure 400 {object} errorDocResponse
// @Failure 401 {object} errorDocResponse
// @Failure 409 {object} errorDocResponse
// @Router /api/servers [post]
func swaggerCreateServer() {}

// swaggerProbeServer godoc
// @Summary 测试未保存的服务器 Agent
// @Description 使用请求体中的 Agent 地址、可选 API Key、角色和 tlsInsecure 执行健康检查及受保护业务接口验证，不写入服务器表。
// @Tags Servers
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body serverResponse true "服务器连接参数，包含 agentUrl、可选 apiKey 和可选 tlsInsecure"
// @Success 200 {object} serverHealthResponse
// @Failure 400 {object} errorDocResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/servers/probe [post]
func swaggerProbeServer() {}

// swaggerDeleteServer godoc
// @Summary 删除服务器
// @Tags Servers
// @Produce json
// @Security BearerAuth
// @Param id path string true "服务器 ID"
// @Success 200 {object} statusResponse
// @Failure 401 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Router /api/servers/{id} [delete]
func swaggerDeleteServer() {}

// swaggerPingServer godoc
// @Summary 测试服务器 Agent 健康状态
// @Description 调用已登记 Agent 的 /health 和角色对应业务接口；手动检查失败会立即将服务器状态写为 Offline。
// @Tags Servers
// @Produce json
// @Security BearerAuth
// @Param id path string true "服务器 ID"
// @Param mode query string false "检查模式；传 auto 时按系统离线失败次数累计，默认手动检查失败立即 Offline"
// @Success 200 {object} serverHealthResponse
// @Failure 401 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Router /api/servers/{id}/ping [post]
func swaggerPingServer() {}

// swaggerSyncServer godoc
// @Summary 同步指定服务器 Agent 数据
// @Description 创建 runtime.refresh.server 刷新任务，仅同步指定 Windows Agent 的 DNS/DHCP 快照。
// @Tags Servers
// @Produce json
// @Security BearerAuth
// @Param id path string true "服务器 ID"
// @Success 202 {object} refreshTaskResponse
// @Failure 401 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Router /api/servers/{id}/sync [post]
func swaggerSyncServer() {}

// swaggerPublicSystemBaseConfig godoc
// @Summary 获取公开系统基础配置
// @Description 无需登录，用于登录页、启动页和控制台布局读取站点名称、品牌名称和图标。
// @Tags Settings
// @Produce json
// @Success 200 {object} systemBaseConfigResponse
// @Router /api/public/base [get]
func swaggerPublicSystemBaseConfig() {}

// swaggerGetSystemBaseConfig godoc
// @Summary 获取系统基础配置
// @Tags Settings
// @Produce json
// @Security BearerAuth
// @Success 200 {object} systemBaseConfigResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/settings/base [get]
func swaggerGetSystemBaseConfig() {}

// swaggerUpdateSystemBaseConfig godoc
// @Summary 保存系统基础配置
// @Tags Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body systemBaseConfigResponse true "基础配置"
// @Success 200 {object} systemBaseConfigResponse
// @Failure 400 {object} errorDocResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/settings/base [put]
func swaggerUpdateSystemBaseConfig() {}

// swaggerListSettingsUsers godoc
// @Summary 获取用户配置列表
// @Tags Settings
// @Produce json
// @Security BearerAuth
// @Success 200 {object} settingsUserListResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/settings/users [get]
func swaggerListSettingsUsers() {}

// swaggerCreateSettingsUser godoc
// @Summary 创建用户配置
// @Tags Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body managedUserRequest true "用户配置"
// @Success 201 {object} settingsUserResponse
// @Failure 400 {object} errorDocResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/settings/users [post]
func swaggerCreateSettingsUser() {}

// swaggerUpdateSettingsUser godoc
// @Summary 更新用户配置
// @Tags Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "用户 ID"
// @Param body body managedUserRequest true "用户配置"
// @Success 200 {object} settingsUserResponse
// @Failure 400 {object} errorDocResponse
// @Failure 401 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Router /api/settings/users/{id} [put]
func swaggerUpdateSettingsUser() {}

// swaggerDisableSettingsUser godoc
// @Summary 启用或禁用用户配置
// @Tags Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "用户 ID"
// @Param body body settingsUserDisabledRequest true "用户启禁状态"
// @Success 200 {object} settingsUserResponse
// @Failure 400 {object} errorDocResponse
// @Failure 401 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Router /api/settings/users/{id}/disabled [post]
func swaggerDisableSettingsUser() {}

// swaggerDeleteSettingsUser godoc
// @Summary 删除用户配置
// @Description 需要先禁用目标用户；默认管理员和当前登录用户受保护。
// @Tags Settings
// @Produce json
// @Security BearerAuth
// @Param id path string true "用户 ID"
// @Success 204
// @Failure 400 {object} errorDocResponse
// @Failure 401 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Router /api/settings/users/{id} [delete]
func swaggerDeleteSettingsUser() {}

// swaggerListRoles godoc
// @Summary 获取用户角色列表
// @Tags Settings
// @Produce json
// @Security BearerAuth
// @Success 200 {object} roleListResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/settings/roles [get]
func swaggerListRoles() {}

// swaggerCreateRole godoc
// @Summary 创建自定义角色
// @Tags Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body roleRequest true "角色配置"
// @Success 201 {object} roleResponse
// @Failure 400 {object} errorDocResponse
// @Failure 401 {object} errorDocResponse
// @Failure 409 {object} errorDocResponse
// @Router /api/settings/roles [post]
func swaggerCreateRole() {}

// swaggerUpdateRole godoc
// @Summary 更新自定义角色
// @Tags Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "角色 ID"
// @Param body body roleRequest true "角色配置"
// @Success 200 {object} roleResponse
// @Failure 400 {object} errorDocResponse
// @Failure 401 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Router /api/settings/roles/{id} [put]
func swaggerUpdateRole() {}

// swaggerDeleteRole godoc
// @Summary 删除自定义角色
// @Description 内置角色不可删除。
// @Tags Settings
// @Produce json
// @Security BearerAuth
// @Param id path string true "角色 ID"
// @Success 200 {object} statusResponse
// @Failure 401 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Router /api/settings/roles/{id} [delete]
func swaggerDeleteRole() {}

// swaggerListUserGroups godoc
// @Summary 获取用户群组列表
// @Tags Settings
// @Produce json
// @Security BearerAuth
// @Success 200 {object} userGroupListResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/settings/user-groups [get]
func swaggerListUserGroups() {}

// swaggerCreateUserGroup godoc
// @Summary 创建用户群组
// @Tags Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body userGroupRequest true "用户群组配置"
// @Success 201 {object} userGroupResponse
// @Failure 400 {object} errorDocResponse
// @Failure 401 {object} errorDocResponse
// @Failure 409 {object} errorDocResponse
// @Router /api/settings/user-groups [post]
func swaggerCreateUserGroup() {}

// swaggerUpdateUserGroup godoc
// @Summary 更新用户群组
// @Tags Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "用户群组 ID"
// @Param body body userGroupRequest true "用户群组配置"
// @Success 200 {object} userGroupResponse
// @Failure 400 {object} errorDocResponse
// @Failure 401 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Router /api/settings/user-groups/{id} [put]
func swaggerUpdateUserGroup() {}

// swaggerDeleteUserGroup godoc
// @Summary 删除用户群组
// @Tags Settings
// @Produce json
// @Security BearerAuth
// @Param id path string true "用户群组 ID"
// @Success 200 {object} statusResponse
// @Failure 401 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Router /api/settings/user-groups/{id} [delete]
func swaggerDeleteUserGroup() {}

// swaggerListPermissions godoc
// @Summary 获取可分配权限列表
// @Tags Settings
// @Produce json
// @Security BearerAuth
// @Success 200 {object} permissionListResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/settings/permissions [get]
func swaggerListPermissions() {}

// swaggerListAuthProviders godoc
// @Summary 获取认证配置列表
// @Description 当前支持 AD/LDAP 目录认证配置，启用后登录页显示对应认证方式。
// @Tags Settings
// @Produce json
// @Security BearerAuth
// @Success 200 {object} authProviderListResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/settings/auth-providers [get]
func swaggerListAuthProviders() {}

// swaggerUpdateAuthProvider godoc
// @Summary 保存认证配置
// @Description 当前仅支持 ldap；绑定密码留空且已有配置时保留原值。
// @Tags Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "认证配置 ID，当前为 ldap"
// @Param body body authProviderRequest true "认证配置"
// @Success 200 {object} authProviderResponse
// @Failure 400 {object} errorDocResponse
// @Failure 401 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Router /api/settings/auth-providers/{id} [put]
func swaggerUpdateAuthProvider() {}

// swaggerTestAuthProvider godoc
// @Summary 测试认证配置
// @Description 当前仅支持 ldap，返回 LDAP 搜索匹配用户数量。
// @Tags Settings
// @Produce json
// @Security BearerAuth
// @Param id path string true "认证配置 ID，当前为 ldap"
// @Success 200 {object} authProviderTestResponse
// @Failure 401 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Failure 503 {object} errorDocResponse
// @Router /api/settings/auth-providers/{id}/test [post]
func swaggerTestAuthProvider() {}

// swaggerListNotificationChannels godoc
// @Summary 获取通知媒介配置
// @Tags Settings
// @Produce json
// @Security BearerAuth
// @Success 200 {object} notificationChannelListResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/settings/notifications [get]
func swaggerListNotificationChannels() {}

// swaggerUpdateNotificationChannel godoc
// @Summary 保存通知媒介配置
// @Description 当前支持 email 邮件媒介，前端仅启用找回密码用途。邮件配置包含 passwordResetEnabled、SMTP 主机、端口、用户名、密码、发件人、TLS、STARTTLS 和明文认证开关；SMTP 密码留空且已有配置时保留原值。
// @Tags Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "媒介 ID，当前为 email"
// @Param body body notificationChannelRequest true "通知媒介配置"
// @Success 200 {object} notificationChannelResponse
// @Failure 400 {object} errorDocResponse
// @Failure 401 {object} errorDocResponse
// @Router /api/settings/notifications/{id} [put]
func swaggerUpdateNotificationChannel() {}

// swaggerTestNotificationChannel godoc
// @Summary 测试通知媒介
// @Description 请求体中的 to 为本次测试发送的临时收件人，不保存到邮件媒介配置。
// @Tags Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "媒介 ID，当前为 email"
// @Param body body testNotificationRequest true "测试收件人"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorDocResponse
// @Failure 401 {object} errorDocResponse
// @Failure 502 {object} errorDocResponse
// @Router /api/settings/notifications/{id}/test [post]
func swaggerTestNotificationChannel() {}

// swaggerPreviewNotificationChannel godoc
// @Summary 预览通知模板
// @Description 当前仅支持 email，返回问题和恢复通知模板预览。
// @Tags Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "媒介 ID，当前为 email"
// @Param body body notificationChannelRequest true "通知媒介配置"
// @Success 200 {object} templatePreviewResponse
// @Failure 400 {object} errorDocResponse
// @Failure 401 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Router /api/settings/notifications/{id}/preview [post]
func swaggerPreviewNotificationChannel() {}

// swaggerCreateZone godoc
// @Summary 创建 DNS 区域
// @Description Agent 确认区域存在后，后端会立即读取该区域默认 SOA、NS 等记录并通过 records 返回；即时读取失败时区域仍创建成功，并通过 warning 返回提示。
// @Tags DNS
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body dnsZoneResponse true "DNS 区域参数"
// @Success 201 {object} dnsZoneCreateResponseDoc
// @Failure 400 {object} errorDocResponse
// @Router /api/dns/zones [post]
func swaggerCreateZone() {}

// swaggerDeleteZone godoc
// @Summary 删除 DNS 区域
// @Tags DNS
// @Produce json
// @Security BearerAuth
// @Param id path string true "DNS 区域标识"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Router /api/dns/zones/{id} [delete]
func swaggerDeleteZone() {}

// swaggerRefreshZone godoc
// @Summary 刷新指定 DNS 区域记录
// @Tags DNS
// @Produce json
// @Security BearerAuth
// @Param id path string true "DNS 区域标识"
// @Success 202 {object} refreshTaskResponse
// @Failure 404 {object} errorDocResponse
// @Router /api/dns/zones/{id}/refresh [post]
func swaggerRefreshZone() {}

// swaggerCreateRecord godoc
// @Summary 创建 DNS 记录
// @Description 后端基于数据库快照校验同名同类型同值重复记录、CNAME 同名互斥、CNAME 值格式和 A 记录 IPv4 格式；勾选 createPtr 时会先检查数据库中是否存在对应反向查找区域，缺失时仍创建 A 记录并返回 PTR 警告。成功创建 PTR 时响应会通过 relatedRecords 返回关联 PTR 记录。
// @Tags DNS
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body dnsRecordResponse true "DNS 记录参数"
// @Success 201 {object} dnsRecordCreateResponse
// @Failure 400 {object} errorDocResponse
// @Failure 409 {object} errorDocResponse
// @Router /api/dns/records [post]
func swaggerCreateRecord() {}

// swaggerUpdateRecord godoc
// @Summary 编辑 DNS 记录值
// @Description 支持修改记录值和可选 createPtr；后端会基于数据库快照校验同名同类型同值重复记录和 CNAME 值格式。A 记录带 createPtr 标记时会同步维护关联 PTR 快照；未修改内容时也会返回当前记录。
// @Tags DNS
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "DNS 记录标识"
// @Param body body dnsRecordUpdateRequest true "DNS 记录值"
// @Success 200 {object} dnsRecordCreateResponse
// @Failure 400 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Failure 409 {object} errorDocResponse
// @Router /api/dns/records/{id} [put]
func swaggerUpdateRecord() {}

// swaggerDeleteRecord godoc
// @Summary 删除 DNS 记录
// @Description 删除反向区域 PTR 记录时，后端会把完整 IPv4 展示名转换为 Windows DNS 区域内相对名称后再转发给 Agent。删除正向 A 记录时会按实际匹配关系同步删除对应 PTR 快照，不只依赖 createPtr 标记；如果数据库中没有实际匹配的 PTR 且对应反向区域不存在，不会凭 IP 推导创建反向区域刷新任务。
// @Tags DNS
// @Produce json
// @Security BearerAuth
// @Param id path string true "DNS 记录标识"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Router /api/dns/records/{id} [delete]
func swaggerDeleteRecord() {}

// swaggerCreateScope godoc
// @Summary 创建 DHCP 作用域
// @Description 转发到 DHCP Agent 创建 Windows DHCP 作用域，成功后按当前作用域延迟合并创建 runtime.refresh.dhcp.scope 局部刷新任务。
// @Tags DHCP
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body dhcpScopeResponse true "DHCP 作用域参数"
// @Success 201 {object} dhcpScopeResponse
// @Failure 400 {object} errorDocResponse
// @Failure 502 {object} errorDocResponse
// @Router /api/dhcp/scopes [post]
func swaggerCreateScope() {}

// swaggerUpdateScope godoc
// @Summary 更新 DHCP 作用域
// @Description 转发到 DHCP Agent 更新 Windows DHCP 作用域名称、租期、状态和地址范围，成功后按当前作用域延迟合并创建 runtime.refresh.dhcp.scope 局部刷新任务。
// @Tags DHCP
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "DHCP 作用域 ID"
// @Param body body dhcpScopeResponse true "DHCP 作用域更新参数"
// @Success 200 {object} dhcpScopeResponse
// @Failure 400 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Failure 502 {object} errorDocResponse
// @Router /api/dhcp/scopes/{id} [put]
func swaggerUpdateScope() {}

// swaggerToggleScope godoc
// @Summary 切换 DHCP 作用域状态
// @Description 根据当前数据库快照状态转发到 DHCP Agent 的 activate 或 deactivate，成功后按当前作用域延迟合并创建 runtime.refresh.dhcp.scope 局部刷新任务。
// @Tags DHCP
// @Produce json
// @Security BearerAuth
// @Param id path string true "DHCP 作用域 ID"
// @Success 200 {object} statusResponse
// @Failure 404 {object} errorDocResponse
// @Failure 502 {object} errorDocResponse
// @Router /api/dhcp/scopes/{id}/toggle [post]
func swaggerToggleScope() {}

// swaggerRefreshDHCPScope godoc
// @Summary 刷新指定 DHCP 作用域
// @Description 创建 runtime.refresh.dhcp.scope 局部刷新任务，只同步当前 DHCP 作用域的基础信息、排除范围、租约和保留地址快照。
// @Tags DHCP
// @Produce json
// @Security BearerAuth
// @Param id path string true "DHCP 作用域 ID"
// @Success 202 {object} refreshTaskResponse
// @Failure 404 {object} errorDocResponse
// @Failure 502 {object} errorDocResponse
// @Router /api/dhcp/scopes/{id}/refresh [post]
func swaggerRefreshDHCPScope() {}

// swaggerDeleteScope godoc
// @Summary 删除 DHCP 作用域
// @Description 转发到 DHCP Agent 删除 Windows DHCP 作用域，成功后删除数据库中的作用域、排除范围、租约和保留地址快照。
// @Tags DHCP
// @Produce json
// @Security BearerAuth
// @Param id path string true "DHCP 作用域 ID"
// @Success 200 {object} statusResponse
// @Failure 404 {object} errorDocResponse
// @Failure 502 {object} errorDocResponse
// @Router /api/dhcp/scopes/{id} [delete]
func swaggerDeleteScope() {}

// swaggerCreateExclusion godoc
// @Summary 创建 DHCP 排除范围
// @Description 转发到 DHCP Agent 创建 Windows DHCP 排除范围，成功后按当前作用域延迟合并创建 runtime.refresh.dhcp.scope 局部刷新任务。
// @Tags DHCP
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body dhcpExclusionResponse true "DHCP 排除范围参数"
// @Success 201 {object} dhcpExclusionResponse
// @Failure 400 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Failure 502 {object} errorDocResponse
// @Router /api/dhcp/exclusions [post]
func swaggerCreateExclusion() {}

// swaggerDeleteExclusion godoc
// @Summary 删除 DHCP 排除范围
// @Description 转发到 DHCP Agent 删除 Windows DHCP 排除范围，成功后按当前作用域延迟合并创建 runtime.refresh.dhcp.scope 局部刷新任务。
// @Tags DHCP
// @Produce json
// @Security BearerAuth
// @Param id path string true "DHCP 排除范围 ID"
// @Success 200 {object} statusResponse
// @Failure 404 {object} errorDocResponse
// @Failure 502 {object} errorDocResponse
// @Router /api/dhcp/exclusions/{id} [delete]
func swaggerDeleteExclusion() {}

// swaggerDeleteLease godoc
// @Summary 释放 DHCP 租约
// @Description 转发到 DHCP Agent 释放 Windows DHCP 租约，成功后按当前作用域延迟合并创建 runtime.refresh.dhcp.scope 局部刷新任务。
// @Tags DHCP
// @Produce json
// @Security BearerAuth
// @Param id path string true "DHCP 租约 ID"
// @Success 200 {object} statusResponse
// @Failure 404 {object} errorDocResponse
// @Failure 502 {object} errorDocResponse
// @Router /api/dhcp/leases/{id} [delete]
func swaggerDeleteLease() {}

// swaggerCreateReservation godoc
// @Summary 创建 DHCP 保留地址
// @Description 转发到 DHCP Agent 创建 Windows DHCP 保留地址，成功后按当前作用域延迟合并创建 runtime.refresh.dhcp.scope 局部刷新任务。
// @Tags DHCP
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body dhcpReservationResponse true "DHCP 保留地址参数"
// @Success 201 {object} dhcpReservationResponse
// @Failure 400 {object} errorDocResponse
// @Failure 502 {object} errorDocResponse
// @Router /api/dhcp/reservations [post]
func swaggerCreateReservation() {}

// swaggerUpdateReservation godoc
// @Summary 更新 DHCP 保留地址
// @Description 转发到 DHCP Agent 更新 Windows DHCP 保留地址，成功后按当前作用域延迟合并创建 runtime.refresh.dhcp.scope 局部刷新任务。
// @Tags DHCP
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "DHCP 保留地址 ID"
// @Param body body dhcpReservationResponse true "DHCP 保留地址更新参数"
// @Success 200 {object} dhcpReservationResponse
// @Failure 400 {object} errorDocResponse
// @Failure 404 {object} errorDocResponse
// @Failure 502 {object} errorDocResponse
// @Router /api/dhcp/reservations/{id} [put]
func swaggerUpdateReservation() {}

// swaggerDeleteReservation godoc
// @Summary 删除 DHCP 保留地址
// @Description 转发到 DHCP Agent 删除 Windows DHCP 保留地址，成功后按当前作用域延迟合并创建 runtime.refresh.dhcp.scope 局部刷新任务。
// @Tags DHCP
// @Produce json
// @Security BearerAuth
// @Param id path string true "DHCP 保留地址 ID"
// @Success 200 {object} statusResponse
// @Failure 404 {object} errorDocResponse
// @Failure 502 {object} errorDocResponse
// @Router /api/dhcp/reservations/{id} [delete]
func swaggerDeleteReservation() {}
