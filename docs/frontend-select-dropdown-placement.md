# 前端 Select 下拉方向说明

本文档记录项目内 Select / listbox 控件的展开方向，避免弹窗、卡片或滚动容器裁切下拉菜单。

## Select 语义

项目统一优先复用 `frontend/src/components/ui/select.tsx`。

该组件基于 Radix Select 封装，通过 portal 挂载弹出层，并默认使用 Radix 的 `popper` 定位能力计算展开方向。

## 当前 Select 使用场景

下表登记当前项目内复用 `frontend/src/components/ui/select.tsx` 的 Select 控件。新增或迁移选择器时，需要同步补充页面、文件、场景和定位策略。

| 页面 | 文件 | 场景 | placement |
| :-: | :-: | :-: | :-: |
| 登录页 | `frontend/src/features/auth/LoginPage.tsx` | 登录方式选择 | Radix `popper` |
| 仪表板 | `frontend/src/routes/_authenticated/index.tsx` | 最近活动显示数量选择 | Radix `popper` |
| DNS 管理 | `frontend/src/routes/_authenticated/dns.tsx`、`frontend/src/features/dns/DnsAddRecordDialog.tsx`、`frontend/src/features/dns/DnsZoneExportDialog.tsx`、`frontend/src/components/agent-scope-toolbar.tsx` | 页面标题行右侧当前 Agent 选择、记录类型、导出范围和导出格式；DNS 导出格式支持 XLSX / XLS / CSV / TXT；新建区域使用当前 Agent，不再单独选择服务器，弹窗内不再提供区域类型或动态更新选择 | Radix `popper` |
| DHCP 管理 | `frontend/src/routes/_authenticated/dhcp.tsx`、`frontend/src/features/dhcp/DhcpScopeExportDialog.tsx`、`frontend/src/components/agent-scope-toolbar.tsx` | 页面标题行右侧当前 Agent 选择、导出对象、导出范围和导出格式；DHCP 导出格式支持 XLSX / XLS / CSV / TXT；新建作用域使用当前 Agent，不再单独选择服务器 | Radix `popper` |
| Agent 设置 | `frontend/src/routes/_authenticated/settings.tsx` | Agent 角色选择仅保留 DNS 和 DHCP，默认显示“请选择角色”，下拉开头显示禁用的“请选择角色”占位项，触发器和选项文本不加粗 | Radix `popper` |
| 操作审计 | `frontend/src/routes/_authenticated/audit.tsx` | 显示数量选择 | Radix `popper` |
| 导出弹窗 | `frontend/src/components/export-dialog.tsx` | 通用导出文件扩展名选择，支持 XLSX / XLS / CSV / TXT | Radix `popper` |

`frontend/src/components/agent-scope-toolbar.tsx` 是 DNS / DHCP 页面共用的当前 Agent 选择控件；`frontend/src/components/export-dialog.tsx` 是任务日志和审计日志共用的导出格式选择控件。两者仍按实际承载页面在上表登记。

## 手写 listbox 控件说明

| 页面 | 文件 | 场景 | 说明 |
| :-: | :-: | :-: | :-: |
| DNS 管理 | `frontend/src/features/dns/DnsZoneExportDialog.tsx` | 导出弹窗自定义区域多选搜索 | 这是多选标签输入，需要边输入边过滤并点击已有区域生成标签；下拉层通过 portal 挂载到 `document.body`，按输入框位置固定定位并使用 `z-[1800]` 高于导出弹窗内容，限制最大高度并滚动展示 |
| DHCP 管理 | `frontend/src/features/dhcp/DhcpScopeExportDialog.tsx` | 导出弹窗自定义作用域多选搜索 | 这是多选标签输入，需要边输入边过滤并点击已有作用域生成标签；下拉层通过 portal 挂载到 `document.body`，按输入框位置固定定位并使用 `z-[1800]` 高于导出弹窗内容，限制最大高度并滚动展示 |

新增或修改其他选择器时优先复用 `frontend/src/components/ui/select.tsx`。
