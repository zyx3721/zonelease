# ZoneLease

ZoneLease 是一个面向 Windows DNS / DHCP 环境的统一管理控制台，包含 React 前端、Go 后端、PostgreSQL 数据库、Redis 事件通道、短期运行态缓存与分布式锁，以及分别部署在 Windows DNS / DHCP 服务器上的轻量 Agent。

## 目录

- [一、项目介绍](#一项目介绍)
- [二、本地开发快速启动](#二本地开发快速启动)
- [三、Docker Compose 快速部署（推荐）](#三docker-compose-快速部署推荐)
- [四、生产环境部署](#四生产环境部署)
- [五、DNS/DHCP Agent 部署](#五dnsdhcp-agent-部署)
- [六、使用说明](#六使用说明)
- [七、安全说明](#七安全说明)
- [八、注意事项](#八注意事项)
- [九、常见问题](#九常见问题)
- [十、API 文档](#十api-文档)
- [十一、版本历史](#十一版本历史)
- [十二、许可证](#十二许可证)
- [十三、致谢](#十三致谢)
- [十四、联系方式](#十四联系方式)

# 一、项目介绍

## 1.1 项目简介

ZoneLease 用于把分散在 Windows Server 上的 DNS 区域、DNS 记录、DHCP 作用域、租约、保留地址和服务器健康状态集中到同一个控制台中管理。平台采用控制中心与 Agent 分离的架构：控制中心负责用户认证、会话、服务器登记、状态聚合、刷新事件、任务记录和审计日志；DNS Agent 和 DHCP Agent 部署在目标 Windows 服务器上，负责通过 PowerShell `DnsServer` / `DhcpServer` 模块执行受控采集与操作。

项目不直接连接 Windows DNS / DHCP 服务数据库。服务器配置、平台用户、会话、刷新任务、审计记录、DNS 区域、DNS 记录以及 DHCP 运行快照保存在 PostgreSQL 中；刷新事件、最近事件回放、刷新任务进度、Agent 健康检查结果和通知未读数等短期运行态数据通过 Redis 分发或缓存。前端页面默认展示 PostgreSQL 中的最近一次同步快照，不会在刷新网页时主动触发 Agent 采集；只有保存 Agent 后同步、手动同步 Agent、手动全量刷新、后端定时全量刷新、DNS 区域卡片刷新按钮或 DNS / DHCP 管理操作才会访问 Agent 并更新数据库快照。

平台面向内网基础设施运维场景，覆盖服务器接入、DNS 区域与记录维护、DHCP 作用域与租约维护、地址保留、全局刷新、SSE 实时同步、操作审计、密码修改和找回密码流程。前端提供深色 / 浅色主题、统一导航、全局刷新按钮和面向 DNS / DHCP 的工作台；后端可通过 API Key 调用 Agent，并在关键变更后写入审计记录。

## 1.2 项目预览

|                  项目登录页                  |
| :------------------------------------------: |
| ![login](.github\images\zonelease-login.jpg) |

|                  项目首页                  |
| :----------------------------------------: |
| ![home](.github\images\zonelease-home.jpg) |

## 1.3 核心功能

- **用户认证**：默认管理员初始化、登录、注销、当前用户查询、修改密码、找回密码验证码流程。
- **服务器管理**：登记 Windows DNS / DHCP Agent 地址、可选保存 API Key、删除服务器、手动健康检查。
- **仪表板**：汇总 DNS 区域、DNS 记录、DHCP 作用域、DHCP 租约、服务器在线状态和最近操作，并在每个 Agent 状态卡片中展示对应的区域 / 作用域、记录 / 租约 / 保留地址快照统计。
- **DNS 管理**：默认读取数据库中的 DNS 区域与记录快照，支持区域级刷新、创建 / 删除 DNS 区域，以及创建、编辑和删除 DNS 记录。
- **DHCP 管理**：通过 DHCP Agent 管理 Windows DHCP 作用域、排除范围、租约和保留地址，支持作用域编辑、启停、释放租约、创建 / 删除排除范围、创建 / 编辑 / 删除保留地址，并在成功后按作用域合并局部同步数据库快照。
- **实时刷新**：前端可提交全局刷新任务或区域刷新任务，后端后台同步 Agent 数据到 PostgreSQL，并通过 Redis 发布 SSE 事件；新 SSE 连接会回放 Redis Stream 中最近的刷新事件，订阅页面收到事件后重新读取数据库快照；Redis 短期锁用于避免多实例或重复触发导致同一刷新目标并发执行。
- **操作审计**：记录登录、服务器、DNS、DHCP、刷新和密码修改等关键用户操作，便于追溯。
- **双 Agent 支持**：DNS Agent 支持新版 Windows `DnsServer` 模块，并提供 Windows Server 2008 / 2008 R2 及更老版本兼容脚本；DHCP Agent 支持新版 `DhcpServer` 模块，并提供基于 `netsh dhcp server` 的 legacy 兼容脚本。
- **API 文档**：后端已接入 swag，可通过 `/swagger/index.html` 查看控制中心 Swagger UI。

## 1.4 实时数据边界

PostgreSQL 是平台数据和 DNS / DHCP 资源快照的最终事实源，保存用户、会话、服务器登记、DNS 区域与记录、DHCP 作用域、租约、保留地址、刷新任务和审计记录。

页面默认读取数据库快照，不会因浏览器刷新而实时遍历 Agent。只有保存 Agent 后同步、手动同步 Agent、手动全量刷新、定时全量刷新、DNS / DHCP 局部刷新或管理操作，才会访问目标 Agent 并更新快照。

Redis 只承载短期运行态能力，包括 SSE 刷新事件、最近事件回放、刷新任务进度、Agent 健康检查缓存、通知未读数缓存和短期分布式锁。接口读取仍以 PostgreSQL 为准；刷新链路、锁边界和 Redis key 细节见 [docs/refresh-sync.md](docs/refresh-sync.md) 与 [docs/redis-runtime.md](docs/redis-runtime.md)。

## 1.5 专题文档索引

README 主要覆盖安装部署、常用操作和接口概览；更细的 DNS/DHCP 采集口径、数据库快照、刷新边界和同步链路放在 `docs/` 目录：

|                               文档                               |                                     适用场景                                     |
| :--------------------------------------------------------------: | :------------------------------------------------------------------------------: |
|         [docs/dns-management.md](docs/dns-management.md)         | 查看 DNS 区域、DNS 记录、Agent 接口、数据库快照、区域刷新和记录变更后的同步链路  |
|        [docs/dhcp-management.md](docs/dhcp-management.md)        |      查看 DHCP 作用域、租约、保留地址、Agent 接口、数据库快照和当前操作边界      |
|           [docs/refresh-sync.md](docs/refresh-sync.md)           | 查看页面刷新、手动全量刷新、定时全量刷新、区域刷新、Redis/SSE 事件和环境变量边界 |
|          [docs/redis-runtime.md](docs/redis-runtime.md)          |        查看 Redis key、Stream、Pub/Sub、运行态缓存、分布式锁和运维查看方式        |
| [docs/operation-log-coverage.md](docs/operation-log-coverage.md) |               查看刷新任务、操作审计和通知中心的写入边界与覆盖范围               |
| [docs/frontend-select-dropdown-placement.md](docs/frontend-select-dropdown-placement.md) |       查看前端 Select 与手写 listbox 控件的下拉定位、层级和裁切规避规则       |

## 1.6 技术栈

### 1.6.1 后端

- **语言**：Go 1.25+
- **HTTP**：Go 标准库 `net/http` 与 `ServeMux`
- **数据库**：PostgreSQL
- **数据库驱动**：pgxpool
- **缓存 / 事件**：Redis Pub/Sub、Stream、运行态缓存与分布式锁
- **配置加载**：godotenv + 环境变量
- **认证**：Bearer Token + PostgreSQL Session
- **密码安全**：bcrypt
- **API 文档**：swag + http-swagger

### 1.6.2 前端

- **框架**：React 19
- **应用框架**：TanStack Router / TanStack React Start
- **构建工具**：Vite 7
- **语言**：TypeScript
- **样式**：Tailwind CSS 4
- **组件基础**：Radix UI
- **图标**：lucide-react
- **通知**：sonner

### 1.6.3 Agent

- **语言**：Go 1.22+
- **运行环境**：Windows Server
- **DNS 能力**：PowerShell `DnsServer` 模块，兼容 legacy PowerShell Agent
- **DHCP 能力**：PowerShell `DhcpServer` 模块，兼容 legacy `netsh dhcp server`
- **安全机制**：`X-API-Key` 请求头，支持显式开启匿名访问

## 1.7 项目结构

```text
zonelease/
├── backend/                         # Go 后端控制中心
│   ├── api/
│   │   └── router/                   # HTTP API 路由、处理函数、中间件、DNS 记录校验、Swagger 注解与文档模型
│   ├── cmd/server/                   # 后端启动入口
│   ├── config/                       # 后端环境变量配置加载
│   ├── docs/                         # 后端 Swagger/OpenAPI 生成文件
│   ├── internal/                     # 后端内部领域、仓储、Agent 客户端和业务服务
│   │   ├── agent/                    # 访问 DNS / DHCP Agent 的 HTTP 客户端
│   │   ├── domain/                   # 后端领域模型
│   │   ├── repository/               # PostgreSQL 仓储与日志保留清理
│   │   ├── service/                  # 用户认证、实时刷新、同步刷新和后台保留任务服务
│   │   └── strutil/                  # 后端内部字符串工具
│   └── pkg/                          # 后端基础设施与可复用能力
│       └── database/                 # PostgreSQL 连接与初始化迁移
├── dns-agent/                        # Windows DNS Agent
│   ├── cmd/dns-agent/                # DNS Agent 启动入口
│   ├── internal/                     # DNS Agent 内部配置、HTTP 服务和 DNS 操作能力
│   │   ├── config/                   # DNS Agent 环境变量配置加载
│   │   ├── dns/                      # Windows DNS 采集、解析、区域和记录操作
│   │   └── server/                   # DNS Agent HTTP 路由、鉴权和响应封装
│   └── legacy/                       # Windows Server 2008/2008 R2 及更老版本兼容脚本
├── dhcp-agent/                       # Windows DHCP Agent
│   ├── cmd/dhcp-agent/               # DHCP Agent 启动入口
│   ├── internal/                     # DHCP Agent 内部配置、HTTP 服务和 DHCP 操作能力
│   │   ├── config/                   # DHCP Agent 环境变量配置加载
│   │   ├── dhcp/                     # Windows DHCP 采集、作用域、排除范围、租约和保留地址操作
│   │   └── server/                   # DHCP Agent HTTP 路由、鉴权和响应封装
│   └── legacy/                       # Windows Server 2008/2008 R2 及更老版本兼容脚本
├── frontend/                         # React 前端控制台
│   ├── public/                       # 静态资源
│   └── src/
│       ├── components/               # 主布局、启动页、统计卡片、密码弹窗、导出弹窗、Agent 角色徽标、Agent 选择工具栏和基础 UI 组件
│       │   └── ui/                   # Button、Dialog、Select 等通用 UI 控件
│       ├── features/                 # 页面级业务模块，当前包含认证、DNS、DHCP 和系统配置页面组件
│       │   ├── auth/                 # 登录和忘记密码页面
│       │   ├── dns/                  # DNS 管理页排序、色彩标识、表头、新建区域、记录操作和导出组件
│       │   ├── dhcp/                 # DHCP 管理页作用域、排除范围、保留地址创建 / 编辑和导出组件
│       │   └── system/               # 系统配置中心、基础配置、Agent 判定、用户/群组/角色、认证和邮件配置面板
│       ├── lib/                      # API 客户端、认证、品牌基础配置快照、刷新事件、错误处理和工具函数
│       ├── routes/                   # TanStack Router 文件路由，包含仪表板、DNS、DHCP、审计和设置页面
│       ├── routeTree.gen.ts          # TanStack Router 自动生成路由树
│       ├── router.tsx                # 前端路由实例
│       ├── server.ts                 # React Start 自定义服务端入口
│       ├── start.ts                  # React Start 中间件与启动实例配置
│       └── styles.css                # 全局样式与主题变量
├── deploy/                           # Docker Compose、Nginx 与 Supervisor 部署配置
├── docs/                             # DNS、DHCP、刷新同步等运行链路说明文档
├── AGENTS.md                         # 项目开发规范
├── LICENSE
└── README.md
```

前端弹窗统一保留显式关闭语义：鼠标点击弹窗外部区域不会关闭弹窗，用户需点击关闭、取消或业务按钮完成关闭。

# 二、本地开发快速启动

## 2.1 环境要求

- Go 1.25+（后端）
- Node.js 20+
- PostgreSQL 16+
- Redis 6.0+
- Windows Server DNS / DHCP 服务器需安装对应 PowerShell 模块

> 如果本地没有安装部署 PostgreSQL /Redis，可参考以下docker快速部署相关数据库（可选）。

创建 `pgsql` 指令：

```bash
docker run -d --name pg-prod \
  -p 5432:5432 \
  -v /data/PgSqlData:/var/lib/postgresql/data \
  -e POSTGRES_PASSWORD="123456ok!" \
  -e LANG=C.UTF-8 \
  -e TZ=Asia/Shanghai \
  postgres:17-alpine
```

创建 `redis` 指令：

```bash
docker run -d --name redis-prod \
  -p 6379:6379 \
  --restart=always \
  -v /data/redisData:/data \
  -e REDIS_PASSWORD=123456 \
  -e TZ=Asia/Shanghai \
  redis:7-alpine \
  redis-server --requirepass 123456 --appendonly yes
```

查看是否创建成功：

```bash
[root@docker-server ~]# docker ps
CONTAINER ID   IMAGE                COMMAND                  CREATED          STATUS          PORTS                                         NAMES
51e019841d66   redis:7-alpine       "docker-entrypoint.s…"   18 minutes ago   Up 18 minutes   0.0.0.0:6379->6379/tcp, [::]:6379->6379/tcp   redis-prod
22205f8e78c6   postgres:17-alpine   "docker-entrypoint.s…"   34 minutes ago   Up 34 minutes   0.0.0.0:5432->5432/tcp, [::]:5432->5432/tcp   pg-prod
```

## 2.2 克隆项目

```bash
git clone https://github.com/zyx3721/zonelease.git
cd zonelease
```

## 2.3 数据库配置

### 2.3.1 本地数据库创建

创建 PostgreSQL 数据库：

```bash
psql -Upostgres -c "CREATE DATABASE zonelease;"
```

### 2.3.2 容器数据库创建

进入容器内的 psql 交互界面：

```bash
docker exec -it pg-prod psql -U postgres
```

在 psql 中创建 `zonelease` 库（执行后输入 `\q` 退出）：

```bash
CREATE DATABASE zonelease;
```

后端启动时会按顺序执行 `backend/pkg/database/migrations/` 下的数据库迁移，初始化数据库结构、内置配置并升级默认配置值；迁移记录保存在 `schema_migrations` 表中，重复启动会跳过已应用版本。当前数据库保存用户、会话、找回密码请求、服务器登记、DNS 区域快照、DNS 记录快照、DHCP 作用域快照、DHCP 租约快照、DHCP 保留地址快照、刷新任务、通知消息和审计记录等平台数据；页面刷新默认读取数据库快照，不会直接触发 DNS / DHCP Agent 采集。

## 2.4 后端配置与启动

> 如果没有配置go的镜像代理，可以参考 [Go 国内加速：Go 国内加速镜像 | Go 技术论坛](https://learnku.com/go/wikis/38122)。

1. 进入后端目录下载相关依赖：

```bash
cd backend
go mod download
```

2. 配置数据库连接等信息：

```bash
# 步骤1：复制模板文件
cp .env.example .env

# 步骤2：编辑 .env，配置数据库连接等信息
vim .env
# 服务配置
SERVER_HOST=localhost
SERVER_PORT=8080
SERVER_MODE=release
CORS_ORIGIN=http://localhost:5173

# 数据库配置
DB_HOST=localhost
DB_PORT=5432
DB_NAME=zonelease
DB_USER=postgres
DB_PASSWORD=your_database_password
DB_SSLMODE=disable

# 登录与会话配置
JWT_SECRET=your_jwt_secret_key
JWT_EXPIRE_HOURS=24
SESSION_IDLE_TIMEOUT_HOURS=12

# Redis 缓存与后台刷新
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=123456
REDIS_DB=0
RUNTIME_DNS_DEEP_SYNC_INTERVAL=1d
RUNTIME_DHCP_DEEP_SYNC_INTERVAL=1h
METRIC_RETENTION_DAYS=30
METRIC_STREAM_MAXLEN=10000
LOG_RETENTION_DAYS=30
```

**配置参数说明详情见 [6.8](#68-后端配置)。**

3. 运行后端服务：

```bash
# 方式1：前台运行（终端关闭则服务停止）
go run cmd/server/main.go

# 方式2：后台运行（日志输出到 app.log）
nohup go run cmd/server/main.go > app.log 2>&1 &
```

后端服务默认运行在 `http://localhost:8080` ，如需指定地址和端口，请修改环境变量文件内的 `SERVER_HOST` 和 `SERVER_PORT` 参数。首次启动会自动创建数据库和默认管理员账户 `admin / 123456` 。

## 2.5 前端配置与启动

1. 进入前端目录下载相关依赖：

```bash
cd frontend
npm install
```

2. 配置 API 地址（可选）：

```bash
# 配置说明：
# - 后端端口 = 8080：无需创建 .env 文件（默认值为 http://127.0.0.1:8080）
# - 后端端口 ≠ 8080：需要创建 .env 文件（指定正确端口，例如后端端口改为 8090）
#   创建 .env 文件，例如：
echo "VITE_API_BASE_URL=http://127.0.0.1:8080" > .env
```

3. 启动前端服务：

```powershell
# 方式1：前台运行（终端关闭则服务停止）
npm run dev
# 如果要指定外部访问和监听端口，可执行例如：
npm run dev -- --host --port 5173

# 方式2：后台运行（日志输出到 zonelease-frontend.log）
nohup npm run dev > zonelease-frontend.log 2>&1 &
```

前端服务默认运行在 `http://localhost:5173/` 。

## 2.6 访问系统

- **首页**：`http://localhost:5173`
  - **默认用户名**：`admin`
  - **默认密码**：`123456`
- **API 文档**：`http://localhost:8080/swagger/index.html`

# 三、Docker Compose 快速部署（推荐）

## 3.1 部署目录结构

Docker Compose 部署相关文件统一放在 `deploy/` 目录下。`zonelease` 单镜像内包含 Go 后端、Nginx 和前端 Nitro SSR 服务，并通过 Supervisor 管理多进程。

仓库内置文件结构：

```bash
deploy/
├── Dockerfile            # 多阶段镜像构建：前端构建、后端构建、运行时镜像
├── docker-compose.yml    # PostgreSQL、Redis 和 zonelease 服务编排
├── entrypoint.sh         # 容器启动入口，交给 Supervisor 拉起各进程
├── nginx.conf            # 容器内 Nginx 配置，负责静态资源、API、SSE 和页面反代
├── supervisord.conf      # 容器内多进程管理配置
└── .env.example          # 环境变量模板
```

首次部署时需要从 `.env.example` 复制生成 `.env`，运行后会在 `deploy/` 下生成持久化目录：

```bash
deploy/
├── .env                  # 实际环境变量文件
├── ZLData/               # zonelease 应用数据挂载目录
│   └── logs/             # 后端与前端 SSR 运行日志
├── PgSqlData/            # PostgreSQL 数据目录，使用外部数据库时可不创建
└── RedisData/            # Redis 数据目录，使用外部 Redis 时可不创建
```

镜像构建时会分别生成前端 `.output` 产物和后端二进制；运行时由 Supervisor 同时管理 Go 后端、Nginx 和前端 Nitro SSR 服务。

运行时只复制前端 `.output` 产物，并在 `/app/frontend` 执行 `node .output/server/index.mjs`。Nginx 直接托管 `.output/public/assets` 等静态资源，并将 `/api/`、`/api/events` 和 `/swagger/` 反向代理到后端。

## 3.2 准备配置文件

进入 `deploy` 目录，创建 `.env` 环境变量文件：

```bash
cd deploy
vim .env
```

`.env` 文件内容参考：

```bash
SERVER_MODE=release
CORS_ORIGIN=*

DB_HOST=postgres
DB_PORT=5432
DB_NAME=zonelease
DB_USER=postgres
DB_PASSWORD=123456ok!
DB_SSLMODE=disable

JWT_SECRET=change-me-in-production
JWT_EXPIRE_HOURS=24
SESSION_IDLE_TIMEOUT_HOURS=12

REDIS_ADDR=redis:6379
REDIS_PASSWORD=123456
REDIS_DB=0

RUNTIME_DNS_DEEP_SYNC_INTERVAL=1d
RUNTIME_DHCP_DEEP_SYNC_INTERVAL=1h
METRIC_RETENTION_DAYS=30
METRIC_STREAM_MAXLEN=10000

LOG_RETENTION_DAYS=30
```

**配置参数说明详情见 [6.8](#68-后端配置)。**

## 3.3 构建镜像（可选）

如果不想使用阿里云镜像仓库的镜像，可直接在本地手动构建（默认使用阿里云镜像仓库地址）：

```bash
# 在 deploy/ 目录下构建（构建上下文为项目根目录）
cd deploy
docker build \
  -f Dockerfile \
  -t zonelease:latest \
  --build-arg ALPINE_MIRROR=mirrors.aliyun.com \
  ..
```

然后修改 `deploy/docker-compose.yml` 中 `zonelease` 服务的 `image` 字段为 `zonelease:latest`。

## 3.4 启动服务

`docker-compose.yml` 支持两种模式，按需选择：

**模式一：新建 PostgreSQL 容器（默认）**

首次启动会按 `.env` 中的 `DB_NAME` 自动创建数据库，默认数据库名为 `zonelease`：

```bash
cd deploy
docker compose up -d
```

**模式二：使用已有容器**

`.env` 环境变量文件中确保数据库配置填入已有容器地址，并编辑 `deploy/docker-compose.yml`：

1. 注释掉 `postgres` 和 `redis` 服务块
2. 注释掉 `zonelease.depends_on` 块

```bash
cd deploy
docker compose up -d
```

## 3.5 服务管理

```bash
# 查看服务状态
docker compose ps

# 查看实时日志
docker compose logs -f zonelease

# 重启 zonelease 服务
docker compose restart zonelease

# 停止所有服务
docker compose down

# 停止并删除数据卷（谨慎！数据会丢失）
docker compose down -v
```

## 3.6 访问系统

服务启动后，访问以下地址：

- **首页**：`http://your-domain.com`
  - **默认用户名**：`admin`
  - **默认密码**：`123456`
- **API 文档**：`http://your-domain.com/swagger/index.html`
- **健康检查**：`http://your-domain.com/health`

## 3.7 宿主机 Nginx 反代（可选）

如需通过宿主机 Nginx 统一配置公网域名、HTTPS 证书或多站点入口，可将 `deploy/docker-compose.yml` 中的端口映射改为非 80 端口（如 `8080:80`），再由宿主机 Nginx 反向代理到容器内 Nginx。

此时请求链路为：

```text
浏览器 -> 宿主机 Nginx -> zonelease 容器内 Nginx -> Go 后端 / 前端 SSR 服务
```

容器内 Nginx 已经配置了 `/api/events`，但宿主机 Nginx 是额外的一层代理。SSE 长连接在任意一层被缓冲都会导致前端同步进度延迟，因此宿主机 Nginx 示例中仍建议保留 `location = /api/events`，用于在外层代理关闭缓冲和延长读取超时。

### 3.7.1 HTTP 示例

```nginx
server {
    listen 80;
    server_name your-domain.com;
    
    # 限制上传文件大小（可选）
    client_max_body_size 50m;
    
    # 日志配置
    access_log /usr/local/nginx/logs/zonelease-access.log;
    error_log /usr/local/nginx/logs/zonelease-error.log warn;
    
    # SSE 长连接接口：关闭代理缓冲，避免刷新事件被缓存
    location = /api/events {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 1h;
        proxy_send_timeout 1h;
        add_header X-Accel-Buffering no;
    }
    
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # 超时配置
        proxy_connect_timeout 600s;
        proxy_send_timeout 600s;
        proxy_read_timeout 600s;
    }
}
```

### 3.7.2 HTTPS 示例

> HTTPS 示例（含 80→443 跳转，请替换证书路径）：

```nginx
# HTTP 80端口配置，自动重定向到HTTPS
server {
    listen 80;
    server_name your-domain.com;   # 修改为你的域名/主机名，例如：zonelease.cn
    return 301 https://$host$request_uri;
}

# zonelease 站点 HTTPS 配置
server {
    # listen 443 ssl http2;  # Nginx 1.25 以下版本写法
    listen 443 ssl;
    http2 on;
    server_name your-domain.com;   # 修改为你的域名/主机名，例如：zonelease.cn
    
    # 证书路径（替换为实际证书文件）
    ssl_certificate     /usr/local/nginx/ssl/your-domain.com.pem;  # 例如：/usr/local/nginx/ssl/zonelease.cn.pem
    ssl_certificate_key /usr/local/nginx/ssl/your-domain.com.key;  # 例如：/usr/local/nginx/ssl/zonelease.cn.key

    # SSL安全优化
    ssl_protocols              TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers  on;
    ssl_ciphers                ECDHE-RSA-AES256-GCM-SHA512:DHE-RSA-AES256-GCM-SHA512:ECDHE-RSA-AES256-GCM-SHA384:DHE-RSA-AES256-GCM-SHA384;
    ssl_session_timeout        10m;
    ssl_session_cache          shared:SSL:10m;
    
    # 限制上传文件大小（可选）
    client_max_body_size 50m;
    
    # 日志配置
    access_log /usr/local/nginx/logs/zonelease-access.log;
    error_log /usr/local/nginx/logs/zonelease-error.log warn;
    
    # SSE 长连接接口：关闭代理缓冲，避免刷新事件被缓存
    location = /api/events {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 1h;
        proxy_send_timeout 1h;
        add_header X-Accel-Buffering no;
    }
    
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # 超时配置
        proxy_connect_timeout 600s;
        proxy_send_timeout 600s;
        proxy_read_timeout 600s;
    }
}
```

# 四、生产环境部署

## 4.1 克隆项目

```bash
git clone https://github.com/zyx3721/zonelease.git /data/zonelease
cd /data/zonelease
```

## 4.2 后端构建与配置

1. 进入后端目录下载相关依赖：

```bash
cd backend
go mod download
```

2. 配置数据库连接等信息：

```bash
# 步骤1：复制模板文件
cp .env.example .env

# 步骤2：编辑 .env，配置数据库连接等信息
vim .env
# 服务配置
SERVER_HOST=localhost
SERVER_PORT=8080
SERVER_MODE=release
CORS_ORIGIN=*

# 数据库配置
DB_HOST=localhost
DB_PORT=5432
DB_NAME=zonelease
DB_USER=postgres
DB_PASSWORD=your_database_password
DB_SSLMODE=disable

# 登录与会话配置
JWT_SECRET=your_jwt_secret_key
JWT_EXPIRE_HOURS=24
SESSION_IDLE_TIMEOUT_HOURS=12

# Redis 缓存与后台刷新
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=123456
REDIS_DB=0
RUNTIME_DNS_DEEP_SYNC_INTERVAL=1d
RUNTIME_DHCP_DEEP_SYNC_INTERVAL=1h
METRIC_RETENTION_DAYS=30
METRIC_STREAM_MAXLEN=10000
LOG_RETENTION_DAYS=30
```

**配置参数说明详情见 [6.8](#68-后端配置)。**

3. 构建后端可执行文件：

```bash
go build -o zonelease-backend cmd/server/main.go
```

4. 运行后端服务：

```bash
# 方式1：前台运行（终端关闭则服务停止）
./zonelease-backend

# 方式2：后台运行（日志输出到 app.log）
nohup ./zonelease-backend > app.log 2>&1 &

# 方法3：加入 systemd 管理启动运行
# 服务配置参考如下，请自行修改相应目录路径
cat > /etc/systemd/system/zonelease-backend.service <<EOF
[Unit]
Description=ZoneLease Backend Golang Service
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/data/zonelease/backend
ExecStart=/data/zonelease/backend/zonelease-backend
Restart=on-failure
RestartSec=5
LimitNOFILE=65535
StandardOutput=journal
StandardError=journal
SyslogIdentifier=zonelease-backend

[Install]
WantedBy=multi-user.target
EOF

# 重载服务配置并启动
systemctl daemon-reload
systemctl start zonelease-backend

# 设置开机自启
systemctl enable --now zonelease-backend
```

## 4.3 前端构建与配置

1. 进入前端目录下载相关依赖：

```bash
cd frontend
npm install
```

2. 构建前端项目：

```
npm run build
```

构建产物在 `.output` 目录。当前前端使用 TanStack React Start + Nitro，构建后会生成可直接运行的 Node 服务端入口和静态资源目录：

- `.output/server/index.mjs`：生产环境 Node SSR 入口；
- `.output/public/`：浏览器静态资源，包含 JS、CSS、favicon 等文件；
- 生产环境前端无需单独配置 API 地址，统一通过 Nginx 将 `/api/` 反向代理到后端。

因此生产部署时需要先启动 `.output/server/index.mjs`，再由 Nginx 将页面请求反向代理到该前端服务；不要只把 `.output/public` 配置为 Nginx 静态根目录，否则服务端渲染页面无法正常返回。

3. 启动前端 SSR 服务：

```bash
# 方式1：前台运行（终端关闭则服务停止）
HOST=127.0.0.1 PORT=5173 npm run start

# 方式2：后台运行（日志输出到 zonelease-frontend.log）
nohup env HOST=127.0.0.1 PORT=5173 npm run start > zonelease-frontend.log 2>&1 &
```

## 4.4 配置Nginx反向代理

在服务器上准备前端目录（例如 `/data/zonelease/frontend/.output`），**将本地 `.output` 目录中的所有文件和子目录整体上传到该目录**，保持 `public/` 与 `server/` 结构不变，例如：

```bash
/data/zonelease/frontend/.output/
├── public/
│   ├── assets/             # 前端浏览器端 JS/CSS 静态资源
│   └── favicon.svg         # 站点图标
└── server/
    └── index.mjs           # Nitro 生产 SSR 入口
```

上传完成后，在 `.output` 所属的前端项目目录执行 `HOST=127.0.0.1 PORT=5173 npm run start` 启动前端服务。Nginx 的 `/` 请求应反向代理到该服务，例如下方示例中的 `127.0.0.1:5173`；`/api/`、`/api/events` 和 `/swagger/` 仍反向代理到 Go 后端 `127.0.0.1:8080`。

`/assets/` 与 `/favicon.svg` 可以由 Nginx 直接读取 `.output/public` 返回，避免静态资源经过前端 SSR 服务，并可为带 hash 的构建资源启用长期缓存。示例中的 `root /data/zonelease/admin/.output/public;` 请按实际上传目录替换。

### 4.4.1 HTTP 示例

> 配置 Nginx （按需替换域名/路径/证书），`HTTP 示例` ：

```nginx
server {
    listen 80;
    server_name your-domain.com;   # 修改为你的域名/主机名，例如：zonelease.cn
    
    # 限制上传文件大小（可选）
    client_max_body_size 50m;
    
    # 日志配置
    access_log /usr/local/nginx/logs/zonelease-access.log;
    error_log /usr/local/nginx/logs/zonelease-error.log warn;

    # 前端静态资源：直接读取 .output/public，避免 JS/CSS 经过 SSR 服务
    location ^~ /assets/ {
        root /data/zonelease/frontend/.output/public;
        try_files $uri =404;
        access_log off;
        expires 1y;
        add_header Cache-Control "public, immutable";
    }

    # 站点图标
    location = /favicon.svg {
        root /data/zonelease/frontend/.output/public;
        try_files $uri =404;
        access_log off;
        expires 7d;
        add_header Cache-Control "public";
    }
    
    # SSE 长连接接口：关闭代理缓冲，避免刷新事件被缓存
    location = /api/events {
        proxy_pass http://127.0.0.1:8080;  # 与后端 API 相同地址
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 1h;
        proxy_send_timeout 1h;
        add_header X-Accel-Buffering no;
    }
    
    # 后端 API 反向代理
    location /api/ {
        proxy_pass http://127.0.0.1:8080;  # 与后端 API 相同地址
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_connect_timeout 60s;
        proxy_send_timeout 300s;
        proxy_read_timeout 300s;
    }
    
    # 后端 API 文档
    location /swagger/ {
        proxy_pass http://127.0.0.1:8080;  # 与后端 API 相同地址
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    
    # 前端 Nitro SSR 服务
    location / {
        proxy_pass http://127.0.0.1:5173;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    
    # 健康检查
    location = /health {
        proxy_pass http://127.0.0.1:8080/api/health;
    }
}
```

### 4.4.2 HTTPS 示例

> HTTPS 示例（含 80→443 跳转，请替换证书路径）：

```nginx
# HTTP 80端口配置，自动重定向到HTTPS
server {
    listen 80;
    server_name your-domain.com;   # 修改为你的域名/主机名，例如：zonelease.cn
    return 301 https://$host$request_uri;
}

# zonelease 站点 HTTPS 配置
server {
    # listen 443 ssl http2;  # Nginx 1.25 以下版本写法
    listen 443 ssl;
    http2 on;
    server_name your-domain.com;   # 修改为你的域名/主机名，例如：zonelease.cn
    
    # 证书路径（替换为实际证书文件）
    ssl_certificate     /usr/local/nginx/ssl/your-domain.com.pem;  # 例如：/usr/local/nginx/ssl/zonelease.cn.pem
    ssl_certificate_key /usr/local/nginx/ssl/your-domain.com.key;  # 例如：/usr/local/nginx/ssl/zonelease.cn.key

    # SSL安全优化
    ssl_protocols              TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers  on;
    ssl_ciphers                ECDHE-RSA-AES256-GCM-SHA512:DHE-RSA-AES256-GCM-SHA512:ECDHE-RSA-AES256-GCM-SHA384:DHE-RSA-AES256-GCM-SHA384;
    ssl_session_timeout        10m;
    ssl_session_cache          shared:SSL:10m;
    
    # 限制上传文件大小（可选）
    client_max_body_size 50m;
    
    # 日志配置
    access_log /usr/local/nginx/logs/zonelease-access.log;
    error_log /usr/local/nginx/logs/zonelease-error.log warn;

    # 前端静态资源：直接读取 .output/public，避免 JS/CSS 经过 SSR 服务
    location ^~ /assets/ {
        root /data/zonelease/admin/.output/public;
        try_files $uri =404;
        access_log off;
        expires 1y;
        add_header Cache-Control "public, immutable";
    }

    # 站点图标
    location = /favicon.svg {
        root /data/zonelease/admin/.output/public;
        try_files $uri =404;
        access_log off;
        expires 7d;
        add_header Cache-Control "public";
    }
    
    # SSE 长连接接口：关闭代理缓冲，避免刷新事件被缓存
    location = /api/events {
        proxy_pass http://127.0.0.1:8080;  # 与后端 API 相同地址
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 1h;
        proxy_send_timeout 1h;
        add_header X-Accel-Buffering no;
    }
    
    # 后端 API 反向代理
    location /api/ {
        proxy_pass http://127.0.0.1:8080;  # 与后端 API 相同地址
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_connect_timeout 60s;
        proxy_send_timeout 300s;
        proxy_read_timeout 300s;
    }
    
    # 后端 API 文档
    location /swagger/ {
        proxy_pass http://127.0.0.1:8080;  # 与后端 API 相同地址
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    
    # 前端 Nitro SSR 服务
    location / {
        proxy_pass http://127.0.0.1:5173;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    
    # 健康检查
    location = /health {
        proxy_pass http://127.0.0.1:8080/api/health;
    }
}
```

## 4.5 访问系统

服务启动后，访问以下地址：

- **首页**：`http://your-domain.com`
  - **默认用户名**：`admin`
  - **默认密码**：`123456`
- **API 文档**：`http://your-domain.com/swagger/index.html`
- **健康检查**：`http://your-domain.com/health`

# 五、DNS/DHCP Agent 部署

DNS/DHCP Agent 部署启动方式有两种：

- **使用 Go Agent 方式启动**：该方式主要针对非 Windows Server 2008 / 2008 R2 及更老版本使用；推荐先在本机或 Linux 服务器（非 Windows Server 服务器）上进行构建可执行文件后，将其上传到 Windows Server 服务器后再进行配置与启动
- **使用 Legacy Agent 方式启动**：该方式主要针对 Windows Server 2008 / 2008 R2 及更老版本使用

请自行根据 Windows Server 服务器版本选择对应的启动方式。

## 5.1 DNS Agent 配置与启动

**方法一：使用 Go Agent 方式启动**

1. 在本机上构建 DNS Agent 可执行文件：

```bash
cd dns-agent
go build -ldflags="-s -w" -o agent.exe cmd/dns-agent/main.go
```

构建完成后，将其上传到 Windows Server 服务器，以上传路径位于 `C:\dns-agent` 目录下为例，以下操作均在 Windows Server 服务器上操作。

2. 配置环境变量信息：

```bash
# 在目录下创建 .env 文件，配置相关信息，示例如下：
DNS_AGENT_HOST=0.0.0.0
DNS_AGENT_PORT=8460
DNS_AGENT_API_KEY=change-me
DNS_AGENT_ALLOW_ANONYMOUS=false
DNS_AGENT_POWERSHELL_TIMEOUT_SECONDS=180
DNS_AGENT_LOG_PATH=agent.log
```

**配置参数说明详情见 [6.9](#69-Agent 配置)。**

3. 运行 DNS Agent 服务：

将 `agent.cmd` 、`agent.ps1` 脚本文件上传到 Windows Server 服务器上，目录结构为：

```bash
C:\dns-agent
├── .env
├── agent.cmd
├── agent.exe
├── agent.ps1
```

然后双击运行 `agent.cmd` 脚本即可，运行服务后，默认监听 `0.0.0.0:8460` ，运行日志写入 `DNS_AGENT_LOG_PATH` 指定的文件。

**方法二：使用 Legacy Agent 方式启动**

> **注意说明**：使用 legacy Agent 时，推荐先在目标服务器安装并启用 .NET Framework 3.x 以上版本，确保 PowerShell 可加载 `System.Web.Extensions` 处理 JSON 请求体；未安装时，部分 body 版 DNS 操作接口可能无法使用，只能依赖后端兼容回退路径。

同样路径以 `C:\dns-agent` 目录下为例，以下操作均在 Windows Server 服务器上操作。

1. 配置环境变量信息：

```bash
# 在目录下创建 .env 文件，配置相关信息，示例如下：
DNS_AGENT_HOST=0.0.0.0
DNS_AGENT_PORT=8460
DNS_AGENT_API_KEY=change-me
DNS_AGENT_ALLOW_ANONYMOUS=false
DNS_AGENT_POWERSHELL_TIMEOUT_SECONDS=180
DNS_AGENT_LOG_PATH=agent.log
```

**配置参数说明详情见 [6.9](#69-Agent 配置)。**

2. 运行 DNS Agent 服务：

将 `agent.cmd` 、`agent.ps1` 、`legacy/source-agent.ps1` 脚本文件上传到 Windows Server 服务器上，目录结构为：

```bash
C:\dns-agent
├── legacy/
│   ├── source-agent.ps1
├── .env
├── agent.cmd
├── agent.ps1
```

然后双击运行 `agent.cmd` 脚本即可，运行服务后，默认监听 `0.0.0.0:8460` ，运行日志写入 `DNS_AGENT_LOG_PATH` 指定的文件。

## 5.2 DHCP Agent 配置与启动

**方法一：使用 Go Agent 方式启动**

1. 在本机上构建 DHCP Agent 可执行文件：

```bash
cd dhcp-agent
go build -ldflags="-s -w" -o agent.exe cmd/dhcp-agent/main.go
```

构建完成后，将其上传到 Windows Server 服务器，以上传路径位于 `C:\dhcp-agent` 目录下为例，以下操作均在 Windows Server 服务器上操作。

2. 配置环境变量信息：

```bash
# 在目录下创建 .env 文件，配置相关信息，示例如下：
DHCP_AGENT_HOST=0.0.0.0
DHCP_AGENT_PORT=8462
DHCP_AGENT_API_KEY=change-me
DHCP_AGENT_ALLOW_ANONYMOUS=false
DHCP_AGENT_POWERSHELL_TIMEOUT_SECONDS=180
DHCP_AGENT_LOG_PATH=agent.log
```

**配置参数说明详情见 [6.9](#69-Agent 配置)。**

3. 运行 DHCP Agent 服务：

将 `agent.cmd` 、`agent.ps1` 脚本文件上传到 Windows Server 服务器上，目录结构为：

```bash
C:\dhcp-agent
├── .env
├── agent.cmd
├── agent.exe
├── agent.ps1
```

然后双击运行 `agent.cmd` 脚本即可，运行服务后，默认监听 `0.0.0.0:8462` ，运行日志写入 `DHCP_AGENT_LOG_PATH` 指定的文件。

**方法二：使用 Legacy Agent 方式启动**

> **注意说明**：使用 legacy Agent 时，目标服务器需要安装或启用 .NET Framework 3.5/4.x，确保 PowerShell 可加载 `System.Web.Extensions` 处理 DHCP 写入和删除接口的 JSON 请求体。缺失时前端会提示补装对应 .NET Framework 组件。

同样路径以 `C:\dhcp-agent` 目录下为例，以下操作均在 Windows Server 服务器上操作。

1. 配置环境变量信息：

```bash
# 在目录下创建 .env 文件，配置相关信息，示例如下：
DHCP_AGENT_HOST=0.0.0.0
DHCP_AGENT_PORT=8462
DHCP_AGENT_API_KEY=change-me
DHCP_AGENT_ALLOW_ANONYMOUS=false
DHCP_AGENT_POWERSHELL_TIMEOUT_SECONDS=180
DHCP_AGENT_LOG_PATH=agent.log
```

**配置参数说明详情见 [6.9](#69-Agent 配置)。**

2. 运行 DHCP Agent 服务：

将 `agent.cmd` 、`agent.ps1` 、`legacy/source-agent.ps1` 脚本文件上传到 Windows Server 服务器上，目录结构为：

```bash
C:\dhcp-agent
├── legacy/
│   ├── source-agent.ps1
├── .env
├── agent.cmd
├── agent.ps1
```

然后双击运行 `agent.cmd` 脚本即可，运行服务后，默认监听 `0.0.0.0:8462` ，运行日志写入 `DHCP_AGENT_LOG_PATH` 指定的文件。

# 六、使用说明

## 6.1 登录与默认账号

后端首次启动且用户表为空时会创建 `admin / 123456` 默认管理员。登录后可在右上角用户菜单中修改密码；修改成功后前端会清除当前会话并返回登录页。

找回密码流程包含图形验证码、身份校验、邮箱验证码发送和密码重置四步。图形验证码 Token 使用 `JWT_SECRET` 签名并携带过期时间，不写入 Redis；生产环境必须使用 `SERVER_MODE=release`，并在「系统配置」中的「邮件媒介」启用找回密码发送。

## 6.2 添加 Windows 服务器

进入「Agent 管理」页面，在「Windows Agent 接入」区域填写：

- **名称**：服务器在控制台中的显示名称，例如 `DC01`
- **角色**：仅支持 `DNS` 或 `DHCP`，默认未选择时显示“请选择角色”
- **Agent 地址**：DNS Agent 或 DHCP Agent 的 HTTP 地址，例如 `http://10.0.0.10:8460`
- **API Key**：可选；配置后应与 Agent `.env` 中 `DNS_AGENT_API_KEY` 或 `DHCP_AGENT_API_KEY` 一致，留空时后端不会发送 `X-API-Key`
- **跳过 TLS 校验**：连接自签名 HTTPS Agent 时可启用，保存后后续测试、同步和 DNS / DHCP 操作都会沿用该设置

DNS Agent 和 DHCP Agent 需要分别按 `DNS` 与 `DHCP` 两个角色登记。后端不会再把同一个 Agent 登记同时用于 DNS 与 DHCP 同步。

常用操作：

- **测试**：调用 `POST /api/servers/probe`，先验证未保存 Agent 地址和 TLS 校验设置；如果已选择角色，则继续使用可选 API Key 验证角色对应业务接口，不写入服务器表。
- **保存**：只有当前 Agent 地址、角色、API Key 和 TLS 设置测试成功后才可点击；保存时只检查 Agent 名称和接口地址唯一性，不再重复访问 Agent，保存成功后显示固定同步 toast 并自动同步该 Agent。保存后的首次自动同步会复用刚刚的测试结果，跳过同步前 `/health` 预检查。
- **测试连接**：调用 `POST /api/servers/{id}/ping`，验证已登记 Agent 的 `/health` 和角色对应业务接口并更新状态；手动测试会写入 `Checked server health` 审计记录。
- **同步 Agent**：调用 `POST /api/servers/{id}/sync`，创建 `runtime.refresh.server` 任务，只同步当前服务器对应角色的数据；前端会等待任务完成后提示成功或失败。
- **删除 Agent**：弹窗确认后调用 `DELETE /api/servers/{id}` 删除服务器登记及关联快照，并清理该 Agent 的离线通知和健康缓存。

## 6.3 DNS 管理

进入「DNS 管理」页面时，前端会以 `includeDns=true` 读取控制台状态。该参数仅用于兼容现有前端调用，后端不会因为页面刷新而访问 Agent，DNS 区域和记录均来自 PostgreSQL 中的最近一次同步快照。

DNS 数据会在以下场景写入或更新数据库：

- Agent 管理中点击「同步 Agent」后，后端会后台同步该服务器对应角色的数据。
- 顶部工具栏手动全量刷新会同步所有可同步 DNS / DHCP Agent；可同步 Agent 指已登记、配置了 Agent URL，且角色匹配本次刷新类型的服务器。
- 后端 `RUNTIME_DNS_DEEP_SYNC_INTERVAL` 默认 `1d`、`RUNTIME_DHCP_DEEP_SYNC_INTERVAL` 默认 `1h`，会分别按本地自然日零点和整点边界执行 DNS / DHCP 全量同步；设为 `0` 可关闭对应角色定时任务。
- DNS 管理页标题行右侧可选择当前 DNS Agent，并点击「刷新」同步当前 Agent 的数据库快照。刷新期间图标持续旋转，固定 toast 可复制同步文本，任务完成后 3 秒自动隐藏。
- DNS 管理页标题行右侧可点击「导出」读取当前 Agent 的全部 DNS 区域和记录快照，并按全部、正向、反向或自定义区域导出为 XLSX、XLS、CSV 或 TXT。
- DNS 区域卡片右侧刷新按钮只刷新该区域下的记录。
- 新增 DNS 区域成功后，DNS Agent 会先确认 Windows DNS 中已能读取该区域，后端会立即读取该区域的默认 SOA、NS 等记录并写入快照，并按区域延迟合并创建 `runtime.refresh.dns.zone` 任务用于最终收敛。
- 新增、编辑或删除 DNS 记录成功后，后端只触发该记录所在区域的记录同步。

当前支持：

- 查看 DNS 区域列表和区域下记录
- 刷新单个 DNS 区域下记录
- 创建 DNS 区域
- 删除 DNS 区域
- 创建 DNS 记录
- 编辑 DNS 记录值
- 删除 DNS 记录
- 搜索左侧区域名称、区域类型和正向 / 反向标识
- 搜索当前区域中的记录名称和值
- 按当前 Agent 过滤 DNS 区域与记录，并用正向 / 反向区域色彩区分区域类型
- 导出当前 Agent 的 DNS 区域记录，支持全部、正向、反向和自定义区域范围；自定义区域需从模糊搜索下拉结果中选择，导出表第一列固定为区域名称
- DNS 记录默认按名称层级和域名 label 从右向左自然排序，让同一父级或后缀下的二级、三级记录相邻展示
- DNS 记录列表默认渲染前 `200` 条，底部通过“加载更多”按 `200` 条继续展示，搜索和排序仍基于当前区域全部记录
- DNS 记录类型使用不同颜色标识，便于快速区分 A、AAAA、CNAME、MX、TXT、PTR、NS、SRV、SOA 等同步展示记录
- 记录管理边界分为展示、创建和编辑 / 删除：同步展示会保留 Agent 能解析的多种记录类型；前端新建记录仅开放 A 和 CNAME；正向区域仅支持编辑 / 删除 A 和 CNAME 记录；反向区域仅支持编辑 / 删除 PTR 记录，其他记录类型会禁用操作按钮并显示对应提示
- 新建记录前，后端会基于数据库快照校验同名同类型同值重复记录、CNAME 互斥规则和当前开放记录类型的值格式
- A 记录值必须是合法 IPv4 地址，创建和编辑时都会校验
- CNAME 记录值必须是以 `.` 结尾的合法域名，且同名互斥校验优先于值格式校验
- 新建 A 记录时默认勾选创建相关 PTR 记录；后端会先基于数据库中的反向查找区域判断是否存在对应区域，缺失时仍创建 A 记录并返回 PTR 警告 toast
- DNS 记录创建、编辑、删除和冲突提示会展示记录名称、类型和值，例如 `www A 10.10.10.10 记录创建成功`
- Go DNS Agent 编辑或删除记录时会复用采集时的 `RecordData` 字段兼容策略定位旧记录，支持同名同类型不同值记录按目标值精确匹配
- DNSSEC 内置信任锚区域 `TrustAnchors` 以及 Windows DNS 内置反向区域 `0.in-addr.arpa`、`127.in-addr.arpa`、`255.in-addr.arpa` 不作为业务 DNS 区域同步

DNS 区域和记录操作会转发到对应 DNS Agent，并在成功后写入审计记录。新增、编辑或删除记录不会触发全量刷新，只会刷新当前区域。

DNS 区域、记录字段、区域级刷新、Agent 接口和数据库快照策略详情见 [docs/dns-management.md](docs/dns-management.md)。

## 6.4 DHCP 管理

进入「DHCP 管理」页面后，可查看平台已维护的 DHCP 作用域、排除范围、租约和保留地址。页面标题行右侧可选择当前 DHCP Agent，并点击「刷新」同步当前 Agent 的数据库快照。新建 DHCP 作用域会使用标题行右侧当前选择的 Agent，不再在弹窗内单独选择服务器。刷新期间图标持续旋转，固定 toast 可复制同步文本，任务完成后 3 秒自动隐藏。

当前支持：

- 创建 DHCP 作用域
- 编辑 DHCP 作用域名称、描述、默认网关、租期和地址范围
- 启用或停用作用域
- 删除 DHCP 作用域
- 刷新单个 DHCP 作用域下排除范围、租约和保留地址
- 查看作用域下排除范围、租约和保留地址
- 创建排除范围
- 删除排除范围
- 释放租约
- 从租约行添加到保留地址
- 编辑保留地址
- 删除保留地址
- 按当前 Agent 过滤 DHCP 作用域、排除范围、租约和保留地址
- 搜索左侧作用域名称、注释、网段、地址范围和状态
- 左侧作用域按作用域网段 IPv4 自然排序，同网段再按掩码和名称排序
- 租约和保留地址默认按 IP 地址自然排序，列名支持默认、升序和降序三态切换
- 排除范围默认按起始 IP 地址自然排序
- 租约列表默认渲染前 `200` 条，底部通过“加载更多”按 `200` 条继续展示，搜索和排序仍基于当前作用域全部租约

DHCP 作用域、排除范围、租约和保留地址操作会转发到对应 DHCP Agent。Agent 执行成功后，后端会等待同一作用域在配置窗口内不再有操作，并在常规路径下创建 `runtime.refresh.dhcp.scope` 局部刷新任务同步数据库快照；如果等待到点时目标 Agent 正在同步或同目标手动刷新正在运行，本次延迟刷新会跳过且不创建任务。已存在数据库快照的 DHCP 作用域相关操作如果在 Agent 阶段失败，也会标记当前作用域需要延迟局部刷新，用于收敛 DHCP 服务器真实状态。

DHCP 作用域、排除范围、租约、保留地址字段、Agent 接口、数据库快照策略和当前操作边界详情见 [docs/dhcp-management.md](docs/dhcp-management.md)。

## 6.5 全局刷新、区域刷新与 SSE

顶部工具栏的刷新按钮会调用 `POST /api/refresh` 创建全量刷新任务。后端会：

- 写入 `refresh_tasks` 记录
- 后台同步所有可同步 Agent 的 DNS / DHCP 数据到 PostgreSQL；可同步 Agent 指已登记、配置了 Agent URL，且角色匹配本次刷新类型的服务器
- 发布 `runtime.refresh.all` 排队、运行、进度、完成或失败事件
- 发布 `runtime.updated` SSE 事件
- 写入 `Queued refresh` 审计记录

手动点击顶部全量刷新按钮时：

- 前端会先显示排队 toast。
- 每个 Agent 开始同步后会显示独立 loading toast。
- 该 Agent 完成或失败后，同一 toast 更新为成功或失败提示，并在 3 秒后自动隐藏。
- 全部完成后会额外显示独立总结 toast。
- 全部成功时，总结 toast 为 `[总数/总数] 所有 Agent 已同步完成`。
- 存在失败时，总结 toast 为 `[已完成/总数] 全量同步完成，异常 N`。
- 存在跳过 Agent 时，总结 toast 为 `[已完成/总数] 全量同步完成，跳过 N`。
- 进度来自 `refresh_tasks.payload` 中的 `totalAgents`、`startedAgents`、`syncedAgents`、`failedAgents`、`skippedAgents`、`currentAgent` 和 `agentEvent` 字段。
- 任务完成或失败后会保留最终进度快照，并写入 `startedAt`、`finishedAt` 和 `agentResults` 便于排查。
- 跳过 Agent 的原因写入 `warn` 字段，不写入 `error`。
- 任务导出包含任务 ID、类型、状态、目标、进度、错误信息、警告信息、创建时间、完成时间和载荷。
- 审计导出包含审计 ID、动作、模块、结果、用户、资源类型、资源、IP、时间和详情。
- 前端收到 SSE 后只重新读取数据库快照和更新 toast，不会再弹出“刷新事件已同步”类提示。

DNS 区域卡片右侧刷新按钮会调用 `POST /api/dns/zones/{id}/refresh`。目标 Agent 未同步且同目标刷新未运行时，后端创建 `runtime.refresh.dns.zone` 区域刷新任务，只访问该区域对应 DNS Agent 的记录接口，并用最新记录替换数据库中该区域的记录快照。

前端全局布局订阅 `/api/events`。收到 `runtime.refresh.all`、`runtime.refresh.dns.all`、`runtime.refresh.dhcp.all`、`runtime.refresh.server`、`runtime.refresh.dns.zone`、`runtime.refresh.dhcp.scope` 或 `runtime.updated` 事件后，会触发页面重新读取数据库快照。

全量同步、DNS 区域和 DHCP 作用域采集并发，以及操作后局部刷新等待时间从「系统配置 / 基础配置 / 同步参数」读取并入库保存。Agent 连接测试、自动健康检查和同步前 `/health` 检查默认 `5` 秒超时；DNS / DHCP 管理操作超时时间默认 `20` 秒；全量同步、单 Agent 同步和局部同步的资源采集超时时间默认 `300` 秒。各刷新入口是否访问 Agent、刷新范围、任务类型、Redis/SSE 事件和配置边界详情见 [docs/refresh-sync.md](docs/refresh-sync.md)。

## 6.6 系统配置

「系统配置」页面参考配置中心布局，包含：

- **基础配置**：维护站点名称、登录展示、控制台品牌、找回密码安全时效、同步并发、操作后刷新等待、Agent 离线判定次数、Agent 连接超时、Agent 操作超时、Agent 全量同步超时、自动健康检查间隔和自动检查并发。
- **用户配置**：维护平台用户、用户群组和角色权限。启用 AD/LDAP 后，外部账号也必须先在用户中创建并启用。
- **认证配置**：维护 AD/LDAP 目录认证连接、必填参数、可选 TLS 参数，并支持连接测试。
- **通知配置**：仅保留邮件 SMTP 配置，主要用于忘记密码流程发送邮箱验证码。

邮件媒介保存后：

- 后端保存到 `notification_channels` 表，SMTP 密码不回显给前端。
- 点击「测试」会弹出测试邮箱输入窗口，并调用 `POST /api/settings/notifications/{id}/test` 发送测试邮件。
- 当前界面不展示邮件模板预览入口。
- 启用「用于找回密码」后，`POST /api/auth/password-reset/send` 会校验输入邮箱与账号邮箱一致，并发送验证码邮件。
- 测试收件人仅用于本次测试发送，不保存到邮件媒介配置；找回密码验证码会发送到用户配置中的账号邮箱。
- 找回密码验证码有效期、图形验证码有效期、发送冷却和频率限制统计窗口从「系统配置 / 基础配置 / 安全时效」读取；同一账号在统计窗口内最多请求 5 次验证码，冷却期重复发送会提示剩余等待秒数。
- 登录会话保存到 `sessions` 表，并记录本次会话使用的认证来源 `provider`，用于登录和登出审计保持一致。
- 找回密码重置成功后，后端会清空该账号在 `sessions` 表中的所有登录会话。
- 非 `release` 模式仍会在响应中返回 `devCode`，仅用于本地调试。

右上角通知中心用于展示站内运行消息：

- 站内消息保存到 `notifications` 表。
- Agent 健康检查连续失败达到离线判定次数并写为 `Offline` 后，会写入站内通知并计入右上角未读红点；未达到阈值的短暂失败只累计失败次数。
- 同一 Agent 或同一平台基础服务已有未读异常通知时，不会重复创建同源通知。
- Agent 后续恢复 `Online` 时，对应的 Agent 离线通知会自动标记已读并清空，无需人工处理。
- PostgreSQL 或 Redis 后续恢复 `online` 时，对应的平台基础服务异常通知会自动标记已读并清空。
- 刷新任务排队、完成或失败只通过任务日志、SSE 状态和 toast 提示展示，不写入通知中心。
- 点击通知按钮会打开消息面板，拥有 `notifications.read` 权限时可查看最近消息。
- 拥有 `notifications.manage` 权限时可标记全部已读和清空；点击未读单条通知时，前端会调用标记已读接口后再跳转到「操作审计」页面辅助排查。
- 只有 `notifications.read` 权限时，后端不允许调用单条标记已读接口；前端点击单条通知只跳转到「操作审计」页面，不会标记已读。

## 6.7 操作审计

「操作审计」页面展示后端 `audit_entries` 中最近的用户操作。当前覆盖：

- 用户登录
- 修改密码
- 添加 / 删除服务器
- 服务器健康检查
- Agent 同步任务排队
- 创建 / 删除 DNS 区域
- 刷新指定 DNS 区域
- 创建 / 编辑 / 删除 DNS 记录
- 创建 / 编辑 / 删除 DHCP 作用域
- 切换 DHCP 作用域状态
- 刷新指定 DHCP 作用域
- 创建 / 删除 DHCP 排除范围
- 释放 DHCP 租约
- 创建 / 编辑 / 删除 DHCP 保留地址
- 创建刷新任务
- 系统基础配置更新
- 创建 / 更新 / 禁用 / 删除用户配置
- 创建 / 更新 / 删除角色和用户组
- 认证配置更新与测试
- 邮件媒介配置更新与测试
- 通知中心单条已读、全部已读和清空

审计列表展示动作、用户、资源、客户端 IP 和发生时间；详情中的“审计元数据”来自 `audit_entries.detail`，用于保存不含敏感信息的对象名称、状态、任务 ID、服务器、DNS / DHCP 资源等排查字段。

## 6.8 后端配置

可复制 `backend/.env.example` 作为环境变量参考：

| 变量                         | 默认值                  | 说明                                                               |
| ---------------------------- | ----------------------- | ------------------------------------------------------------------ |
| `SERVER_HOST`                | `127.0.0.1`             | 后端监听主机，容器或远程访问可设为 `0.0.0.0`                       |
| `SERVER_PORT`                | `8080`                  | 后端监听端口                                                       |
| `SERVER_MODE`                | `release`               | 运行模式，影响找回密码开发验证码返回                               |
| `CORS_ORIGIN`                | `http://localhost:5173` | 允许跨域访问的前端来源                                             |
| `DB_HOST`                    | `localhost`             | PostgreSQL 主机                                                    |
| `DB_PORT`                    | `5432`                  | PostgreSQL 端口                                                    |
| `DB_NAME`                    | `zonelease`             | PostgreSQL 数据库名                                                |
| `DB_USER`                    | `zonelease`             | PostgreSQL 用户名                                                  |
| `DB_PASSWORD`                | `zonelease_dev`         | PostgreSQL 密码                                                    |
| `DB_SSLMODE`                 | `disable`               | PostgreSQL SSL 模式                                                |
| `JWT_SECRET`                 | 启动时临时生成          | 会话 Token 服务端密钥，生产环境必须固定配置                        |
| `JWT_EXPIRE_HOURS`           | `24`                    | 登录会话最长有效期，单位小时                                       |
| `SESSION_IDLE_TIMEOUT_HOURS` | `12`                    | 会话空闲超时时间，单位小时                                         |
| `REDIS_ADDR`                 | `localhost:6379`        | Redis 地址                                                         |
| `REDIS_PASSWORD`             | 空                      | Redis 密码                                                         |
| `REDIS_DB`                   | `0`                     | Redis 数据库编号                                                   |
| `RUNTIME_DNS_DEEP_SYNC_INTERVAL` | `1d`                    | DNS 定时全量同步间隔，支持 `m`、`h`、`d`，设为 `0` 关闭             |
| `RUNTIME_DHCP_DEEP_SYNC_INTERVAL` | `1h`                    | DHCP 定时全量同步间隔，支持 `m`、`h`、`d`，设为 `0` 关闭            |
| `METRIC_RETENTION_DAYS`      | `30`                    | 预留指标保留天数                                                   |
| `LOG_RETENTION_DAYS`         | `30`                    | 任务、审计和通知中心日志保留天数；设为小于等于 `0` 时关闭自动清理  |
| `METRIC_STREAM_MAXLEN`       | `10000`                 | Redis 刷新事件 Stream 最大保留长度，用于 SSE 新连接回放最近事件    |

全量同步服务器级并发、DNS 区域并发、DHCP 作用域并发和操作后刷新等待不再由环境变量控制，统一在「系统配置 / 基础配置 / 同步参数」中配置并保存到 PostgreSQL。

- DNS 区域并发和 DHCP 作用域并发可配置 `1` 到 `50` 个，其中 DHCP 作用域并发默认 `5` 个。
- 操作后刷新等待默认 `10` 秒，可配置 `1` 到 `60` 秒。
- DNS / DHCP 操作成功后，同一区域或作用域在等待窗口内尚未创建局部刷新任务；如果窗口内又发生同目标操作，会取消原计时并重新等待，直到窗口内没有新的操作才创建一条局部刷新任务。
- 已存在数据库快照的 DHCP 作用域相关操作在 Agent 阶段失败时，也会按当前作用域进入同一延迟刷新窗口。
- 手动点击 DNS 区域刷新或 DHCP 作用域刷新时，如果同目标刷新任务正在执行，会直接提示当前刷新目标正在执行，请稍后再试，不创建重复任务。
- 操作后延迟刷新到点时，如果目标 Agent 正在执行全量同步、自动全量同步或单 Agent 同步，会跳过本次延迟局部刷新，不创建任务日志。
- DNS / DHCP 管理操作和手动 DNS 区域 / DHCP 作用域刷新会先检查目标 Agent 是否正在同步；正在同步时会返回“当前 Agent 正在同步，请稍后再操作”，不创建重复刷新任务。
- Agent 离线失败次数、Agent 连接超时时间、Agent 操作超时时间、Agent 全量同步超时时间、自动健康检查间隔和自动检查并发由「系统配置 / 基础配置 / Agent 判定」控制。
- 后台同步和自动连通性检查连续失败达到阈值后才标记为 `Offline` 并创建 Agent 离线通知。
- 仪表板或设置页手动测试连接失败会立即标记为 `Offline`；状态进入离线阈值边界时会创建 Agent 离线通知。
- 成功同步或健康检查会清零失败计数并更新最近检查时间。
- Agent 连接超时默认 `5` 秒，可配置 `1` 到 `20` 秒，覆盖 Agent 保存前探测、已登记 Agent 手动测试、自动健康检查和同步前 `/health` 检查。
- Agent 操作超时默认 `20` 秒，可配置 `1` 到 `60` 秒，覆盖 DNS 区域和记录操作、DHCP 作用域、排除范围、租约和保留地址操作。
- Agent 全量同步超时默认 `300` 秒，可配置 `60` 到 `600` 秒，覆盖全量同步、单 Agent 同步、DNS 区域同步和 DHCP 作用域同步中的资源采集阶段。
- 前端等待刷新任务完成的 toast 会按该配置轮询并额外预留短暂缓冲。
- 自动健康检查间隔默认 `1` 分钟，可配置 `1` 到 `60` 分钟。
- 自动检查并发默认 `1` 个，可配置 `1` 到 `20` 个。
- 自动健康检查只检查已配置 Agent URL 且当前未处于同步中的 Agent。

## 6.9 Agent 配置

DNS Agent：

| 变量                                   | 默认值      | 说明                                                                                                       |
| -------------------------------------- | ----------- | ---------------------------------------------------------------------------------------------------------- |
| `DNS_AGENT_HOST`                       | `0.0.0.0`   | DNS Agent 监听地址                                                                                         |
| `DNS_AGENT_PORT`                       | `8460`      | DNS Agent 监听端口                                                                                         |
| `DNS_AGENT_API_KEY`                    | 空          | DNS Agent API Key；为空时即使关闭匿名访问也不会校验业务接口 API Key，生产环境应配置强随机值                |
| `DNS_AGENT_ALLOW_ANONYMOUS`            | `false`     | 是否允许匿名访问非健康检查接口                                                                             |
| `DNS_AGENT_POWERSHELL_TIMEOUT_SECONDS` | `180`       | Go DNS Agent 内部单次 PowerShell 命令超时，覆盖区域和记录读取、创建、删除等操作；legacy Agent 不读取该变量 |
| `DNS_AGENT_LOG_PATH`                   | `agent.log` | DNS Agent 日志路径，Go Agent 和 legacy Agent 都会将运行日志写入该文件                                      |

DHCP Agent：

| 变量                                    | 默认值      | 说明                                                                                     |
| --------------------------------------- | ----------- | ---------------------------------------------------------------------------------------- |
| `DHCP_AGENT_HOST`                       | `0.0.0.0`   | DHCP Agent 监听地址                                                                      |
| `DHCP_AGENT_PORT`                       | `8462`      | DHCP Agent 监听端口                                                                      |
| `DHCP_AGENT_API_KEY`                    | 空          | DHCP Agent API Key；为空时即使关闭匿名访问也不会校验业务接口 API Key，生产环境应配置强随机值 |
| `DHCP_AGENT_ALLOW_ANONYMOUS`            | `false`     | 是否允许匿名访问非健康检查接口                                                           |
| `DHCP_AGENT_POWERSHELL_TIMEOUT_SECONDS` | `180`       | Go DHCP Agent 单次 PowerShell 命令超时，覆盖作用域、排除范围、租约和保留地址读取、创建、更新、删除等操作 |
| `DHCP_AGENT_LOG_PATH`                   | `agent.log` | DHCP Agent 日志路径，Go Agent 和 legacy Agent 都会将运行日志写入该文件                   |

# 七、安全说明

- 生产环境必须设置固定且足够随机的 `JWT_SECRET`，避免服务重启导致会话失效或 Token 可被伪造。
- 生产环境 Agent API Key 必须使用强随机值，并仅通过内网或 HTTPS 传输；如果 `DNS_AGENT_API_KEY` 或 `DHCP_AGENT_API_KEY` 留空，Agent 不会校验业务接口 API Key。
- 不建议开启 `DNS_AGENT_ALLOW_ANONYMOUS` 或 `DHCP_AGENT_ALLOW_ANONYMOUS`；仅允许在隔离测试环境临时使用。
- 后端 CORS 不建议设置为 `*`，应配置为真实前端访问域名。
- Agent 应只允许后端服务器访问，可通过 Windows 防火墙、网段 ACL 或反向代理限制来源。
- PostgreSQL、Redis、SMTP 或后续通知配置中的密码不得写入审计 metadata、任务 payload 或前端可见日志。
- 找回密码仅适用于本地账号；如果后续接入 AD/LDAP，应在目录服务侧重置目录账号密码。

# 八、注意事项

- DNS 区域与记录展示依赖 PostgreSQL 快照。Agent 离线时页面仍显示最近一次同步数据，但无法获得最新 Windows DNS 视图。
- `/api/state?includeDns=true` 只读取数据库，不会遍历 DNS 服务器。需要最新数据时请使用顶部全量刷新、定时全量刷新或 DNS 区域卡片刷新按钮。
- DHCP 作用域、排除范围、租约和保留地址变更会先转发到 DHCP Agent，Agent 成功后才更新或按作用域合并局部同步 PostgreSQL 快照。
- DNS、DHCP 和刷新同步的详细边界分别见 [docs/dns-management.md](docs/dns-management.md)、[docs/dhcp-management.md](docs/dhcp-management.md) 和 [docs/refresh-sync.md](docs/refresh-sync.md)。
- 删除 DNS 区域或记录会直接转发到 Agent 执行，操作前请确认目标服务器和区域标识正确。
- 前端用户可见提示使用中文，且语句末尾不添加标点；后端和 Agent 服务日志使用英文。
- 使用 Docker Compose 部署时，镜像内已包含后端、Nginx 和轻量前端 SSR 服务；Windows DNS / DHCP Agent 仍需按目标服务器环境独立部署。

# 九、常见问题

## Q1: 首次登录账号是什么？

后端会在用户表为空时创建 `admin / 123456`。已有任意用户时不会重复创建默认账号。

## Q2: 为什么重启后已有 Token 失效？

如果没有配置 `JWT_SECRET`，后端会在进程启动时生成临时密钥。生产环境必须在 `.env` 中固定 `JWT_SECRET`。

## Q3: DNS 页面没有区域或记录怎么办？

请检查：

- Agent 管理中服务器角色是否为 `DNS`
- Agent 地址是否可从后端访问
- Agent API Key 是否与后端保存值一致
- Windows 服务器是否安装 `DnsServer` PowerShell 模块
- 是否已执行同步 Agent、顶部全量刷新、定时全量刷新或 DNS 区域卡片刷新
- 后端日志中是否出现 Agent 调用失败

## Q4: 健康检查通过但 DNS 操作失败怎么办？

`/health` 只验证 Agent 进程可达。DNS 创建、删除和查询还依赖 PowerShell 模块、Windows 权限和目标 DNS 配置。请查看 Agent 日志和后端返回的 `agent_*_failed` 错误。

## Q5: Swagger 页面在哪里？

后端启动后访问：

```text
http://localhost:8080/swagger/index.html
```

如变更后端接口，需要在 `backend/` 目录执行：

```bash
swag init -g cmd/server/main.go -o docs
```

## Q6: 修改后是否必须构建？

开发阶段不需要每次修改后立即构建。前端优先使用热重载或 `npm run type-check`；后端和 Agent 优先使用针对性测试。仅在发布、部署或制作镜像时执行构建。

# 十、API 文档

以下接口除 `POST /api/auth/login`、`GET /api/auth/providers`、`POST /api/auth/logout`、找回密码相关公开接口、`GET /api/public/base`、`GET /api/events` 和 `GET /api/health` 健康检查外，均需要在请求头中携带 `Authorization: Bearer <token>`。

## 10.1 认证

- `POST /api/auth/login` - 登录，返回会话 Token、本次认证来源、用户信息、最长过期时间和最近活跃时间；用户名或密码为空会返回 `invalid_login`
- `GET /api/auth/providers` - 获取公开认证方式，登录页用于读取已启用的本地或 AD/LDAP 登录方式
- `POST /api/auth/logout` - 注销当前会话；携带 Bearer Token 时后端会删除对应会话，未携带或会话已失效时也会幂等返回成功
- `GET /api/auth/me` - 获取当前登录用户；Token 无效或过期时返回 `unauthorized`
- `POST /api/auth/change-password` - 修改当前用户密码
  - 请求字段为 `old_password`、`new_password`、`confirm_password`
  - 新密码至少 6 位，且不能与旧密码相同
  - 修改成功后写入 `Changed password` 审计记录

登录请求示例：

```json
{
  "username": "admin",
  "password": "123456"
}
```

## 10.2 找回密码

- `GET /api/auth/password-reset/captcha` - 获取找回密码图形验证码，返回 `token`、`question` 和 `expiresAt`；`token` 使用 `JWT_SECRET` 签名并携带过期时间，不写入 Redis
- `POST /api/auth/password-reset/verify` - 校验用户名、图形验证码 Token 和答案，成功后返回短期 `verificationToken` 与可用发送渠道
  - 校验顺序为图形验证码、账号可找回状态、找回密码媒介可用性
  - 用户不存在、用户被禁用、非本地账号或账号未配置邮箱时返回 `password_reset_unavailable`
  - 未启用找回密码邮件媒介时返回 `no_password_reset_channel`
- `POST /api/auth/password-reset/send` - 根据短期校验 Token 发送找回密码验证码
  - 请求字段包含 `username`、`verificationToken`、`channel`、`verifyEmail` 和 `to`
  - `channel` 当前为 `email`，后端会校验 `verifyEmail` 与账号邮箱一致
  - 邮件媒介启用后会发送验证码邮件；未启用或发送失败时返回 `send_failed`
  - 安全时效由基础配置控制，同一账号在频率限制统计窗口内最多请求 5 次
  - 非 `release` 模式下响应会返回 `devCode` 便于本地调试
- `POST /api/auth/password-reset/confirm` - 使用验证码重置本地账号密码
  - 请求字段包含 `username`、`verificationToken`、`code`、`newPassword` 和 `confirmPassword`
  - 新密码至少 6 位，且两次输入必须一致
  - 重置成功后会删除该用户在 `sessions` 表中的所有登录会话

## 10.3 DHCP 管理

DHCP 字段、Agent 接口、数据库快照策略和当前操作边界详见 [docs/dhcp-management.md](docs/dhcp-management.md)。

- `POST /api/dhcp/scopes` - 创建 DHCP 作用域
  - 请求字段包含 `name`、`description`、`subnet`、`defaultGateway`、`startRange`、`endRange`、`leaseDurationHours`、`leaseDurationSeconds`、`state` 和 `serverId`
  - `name`、`subnet`、`defaultGateway`、`serverId` 必填
  - `subnet` 必须使用 IPv4 CIDR 前缀格式，例如 `10.0.1.0/24`
  - 后端会转发到目标 DHCP Agent 的 `POST /dhcp/scopes`
  - go Agent 新建时通过 `Add-DhcpServerv4Scope -Description` 写入描述，legacy Agent 新建时在 `add scope` 后通过 `set comment` 写入非空描述
  - go Agent 新建有限租期作用域时使用 `leaseDurationSeconds` 生成 `New-TimeSpan -Seconds`，不再用 `leaseDurationHours` 兜底；无限租期通过 DHCP Option 51 写入 `4294967295`
  - 默认网关为必填项，后端会校验其位于作用域子网内，并写入 Windows DHCP Option 003 Router
  - go Agent 会把同一创建请求内的作用域属性、默认网关和无限租期写入合并为一次 PowerShell 脚本执行
  - legacy Agent 会将创建、地址范围、启用、租期和默认网关写入合并为一次 `netsh -f` 批处理会话
  - Agent 成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 局部刷新任务
  - 成功后写入 `Created DHCP scope` 审计记录
- `PUT /api/dhcp/scopes/{id}` - 更新 DHCP 作用域
  - 当前编辑弹窗支持更新名称、描述、默认网关、租期和地址范围；子网仍以 Agent 同步快照为准
  - 后端会转发到目标 DHCP Agent 的 `PUT /dhcp/scopes/{scopeId}`
  - 后端会传递实际变化字段，Agent 只执行对应的名称 / 描述、默认网关、租期或地址范围操作；请求体仍兼容 `state` 字段，当前前端启停状态通过独立切换接口处理
  - go Agent 在同一编辑请求内有多个变化项时会合并为一次 PowerShell 脚本执行；有限租期继续通过 `Set-DhcpServerv4Scope -LeaseDuration` 写入，无限租期通过 `Set-DhcpServerv4OptionValue -OptionId 51 -Value 4294967295` 写入
  - legacy Agent 修改描述时执行 `set comment`；清空描述时省略描述参数，名称变化只执行 `set name`
  - legacy Agent 在同一请求内有多个变化项时，会合并为一次 `netsh -f` 批处理会话执行
  - 地址范围修改允许只改起始或只改结束 IP；若起始和结束 IP 同时修改，必须同时缩小范围或同时扩大范围
  - legacy Agent 地址范围变化只执行一次 `add iprange <startRange> <endRange>`，不删除旧范围，也不执行 `show iprange`
  - Agent 成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 局部刷新任务
  - 成功后写入 `Updated DHCP scope` 审计记录
- 新建 DHCP 作用域前端会校验地址范围必须位于子网内，不能包含子网地址或广播地址，并且作用域子网不能与当前 Agent 已有作用域重复或重叠
- `POST /api/dhcp/scopes/{id}/toggle` - 切换 DHCP 作用域状态
  - 后端根据数据库快照中的当前状态转发到 Agent 的 `activate` 或 `deactivate`
  - Agent 成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 局部刷新任务
  - 成功后写入 `Toggled DHCP scope` 审计记录
- `POST /api/dhcp/scopes/{id}/refresh` - 刷新指定 DHCP 作用域
  - 后端在目标 Agent 未同步且同目标刷新未运行时创建 `runtime.refresh.dhcp.scope` 局部刷新任务
  - 仅同步当前作用域的基础信息、排除范围、租约和保留地址快照
  - 成功后写入 `Queued DHCP scope refresh` 审计记录
- `DELETE /api/dhcp/scopes/{id}` - 删除 DHCP 作用域
  - 后端会转发到目标 DHCP Agent 的 `DELETE /dhcp/scopes/{scopeId}`
  - Agent 成功后删除数据库中的作用域快照及其排除范围、租约、保留地址
  - 成功后写入 `Deleted DHCP scope` 审计记录
- `POST /api/dhcp/exclusions` - 创建 DHCP 排除范围
  - 请求字段包含 `scopeId`、`startIp` 和 `endIp`
  - 后端会转发到目标 DHCP Agent 的 `POST /dhcp/exclusions`
  - Agent 成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 局部刷新任务
  - 成功后写入 `Created DHCP exclusion` 审计记录
- `DELETE /api/dhcp/exclusions/{id}` - 删除 DHCP 排除范围
  - 后端会转发到目标 DHCP Agent 的 `POST /dhcp/exclusions/delete`
  - Agent 成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 局部刷新任务
  - 成功后写入 `Deleted DHCP exclusion` 审计记录
- `DELETE /api/dhcp/leases/{id}` - 释放 DHCP 租约
  - 后端优先转发到目标 DHCP Agent 的 `POST /dhcp/leases/release`，旧 Agent 返回 `404` 时回退到 `DELETE /dhcp/scopes/{scopeId}/leases/{ip}`
  - go Agent 执行 `Remove-DhcpServerv4Lease -IPAddress <ip>` 释放租约
  - Agent 成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 局部刷新任务
  - 成功后写入 `Released DHCP lease` 审计记录
- `POST /api/dhcp/reservations` - 创建 DHCP 保留地址
  - 请求字段包含 `scopeId`、`ip`、`mac`、`name` 和 `description`
  - `scopeId`、`ip`、`mac` 必填，`name` 可为空
  - 前端入口为租约列表每行的“添加到保留”图标按钮，点击后可在弹窗内填写名称和描述
  - 后端会转发到目标 DHCP Agent 的 `POST /dhcp/reservations`
  - 租约名称为空时添加到保留会保持名称为空，不再用 IP 地址填充
  - go Agent 执行 `Add-DhcpServerv4Reservation` 时固定写入 `-Type 'dhcp'`
  - Agent 成功后本地同 IP 租约快照会更新为保留名称并标记为 `ReservedInactive`，前端显示 `保留 (不活动的)`
  - Agent 成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 局部刷新任务
  - 成功后写入 `Created DHCP reservation` 审计记录
- `PUT /api/dhcp/reservations/{id}` - 更新 DHCP 保留地址
  - 请求字段包含 `ip`、`mac`、`name` 和 `description`
  - `ip`、`mac`、`name` 必填
  - 后端会转发到目标 DHCP Agent 的 `POST /dhcp/reservations/update`
  - go Agent 使用 `Set-DhcpServerv4Reservation -IPAddress <ip> -Name <name> -Description <description> -Type 'dhcp'` 更新保留地址，不删除再创建
  - legacy Agent 会先携带旧 IP 和 MAC 删除旧保留，再重新创建保留地址以更新名称和描述
  - Agent 成功后会立即更新同作用域同 IP 的租约名称快照
  - Agent 成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 局部刷新任务
  - 成功后写入 `Updated DHCP reservation` 审计记录
- `DELETE /api/dhcp/reservations/{id}` - 删除 DHCP 保留地址
  - 后端优先转发到目标 DHCP Agent 的 `POST /dhcp/reservations/delete`，旧 Agent 返回 `404` 时回退到 `DELETE /dhcp/reservations/{scopeId}/{ip}`
  - go Agent 删除时执行 `Remove-DhcpServerv4Reservation -IPAddress`
  - legacy Agent 删除时会携带数据库快照中的 IP 和 MAC 执行 `delete reservedip`，不会因缺少 MAC 额外执行全局 `dump`
  - Agent 成功后会删除本地同 IP 租约快照
  - Agent 成功后按当前作用域延迟合并创建 `runtime.refresh.dhcp.scope` 局部刷新任务
  - 成功后写入 `Deleted DHCP reservation` 审计记录

创建 DHCP 作用域请求示例：

```json
{
  "name": "Office-LAN",
  "subnet": "10.0.1.0/24",
  "defaultGateway": "10.0.1.1",
  "startRange": "10.0.1.100",
  "endRange": "10.0.1.250",
  "leaseDurationHours": 24,
  "leaseDurationSeconds": 86400,
  "state": "Active",
  "serverId": "server-id"
}
```

## 10.4 DNS 管理

- `POST /api/dns/zones` - 创建 DNS 区域
  - 新建弹窗只提供区域名称和正向 / 反向模式；区域类型与动态更新不再开放选择，前端按 `Primary` 和 `None` 默认值提交
  - 控制中心接口仍接收 `name`、`type`、`reverse`、`dynamicUpdate` 和 `serverId` 字段，其中 `name` 和 `serverId` 必填
  - 后端会转发到目标 DNS Agent 的 `POST /dns/zones`
  - Agent 确认区域存在后，后端会立即读取该区域的默认 SOA、NS 等记录并写入快照，响应通过 `records` 返回给前端
  - 默认记录即时读取失败时，区域创建仍成功，响应通过 `warning` 返回提示，并继续按区域延迟合并创建刷新任务兜底
  - 成功后写入 `Created zone` 审计记录，并按区域延迟合并排队 `runtime.refresh.dns.zone` 采集新区域记录
- `DELETE /api/dns/zones/{id}` - 删除 DNS 区域
  - `{id}` 为后端编码后的实时 DNS 区域标识
  - 后端解码出服务器 ID 和区域名称后转发到 DNS Agent
  - 成功后写入 `Deleted zone` 审计记录
- `POST /api/dns/zones/{id}/refresh` - 刷新指定 DNS 区域记录
  - 后端在目标 Agent 未同步且同目标刷新未运行时创建 `runtime.refresh.dns.zone` 刷新任务
  - 仅拉取当前区域记录并替换 PostgreSQL 中该区域的记录快照
  - 成功后写入 `Queued DNS zone refresh` 审计记录
  - DNS 区域和记录快照策略详见 [docs/dns-management.md](docs/dns-management.md)
- `POST /api/dns/records` - 创建 DNS 记录
  - 请求字段包含 `zoneId`、`name`、`type`、`value`、`ttl` 和可选 `createPtr`
  - 前端新建记录类型只开放 `A` 和 `CNAME`；`A` 按 Windows DNS“新建主机”语义提交为 `A` 记录
  - `zoneId`、`name`、`type`、`value` 必填
  - 后端会根据 `zoneId` 解码服务器和区域，并先基于数据库快照校验记录冲突
  - 名称、类型和值完全相同的记录已存在时会拒绝创建
  - 同名已存在 CNAME 时不能创建其他类型记录，同名已存在其他类型记录时不能创建 CNAME
  - A 记录值必须是合法 IPv4 地址
  - CNAME 记录先执行同名互斥校验，再校验记录值必须是以 `.` 结尾的合法域名
  - 记录创建、删除和冲突提示会包含名称、类型和值
  - 勾选 `createPtr` 时仅对 A 记录生效；后端会先检查数据库中是否存在该 IPv4 对应的反向查找区域，缺失时仍创建 A 记录并返回 PTR 警告
  - 找到反向查找区域且 Agent 成功创建 PTR 时，响应会通过 `relatedRecords` 返回关联 PTR 记录，前端会立即合并到反向区域快照
  - 局部刷新正向区域时，后端只会同步数据库中已存在且可能关联的反向区域，并根据匹配的 PTR 记录反推 A 记录的 `createPtr` 标记
  - 数据库校验通过后，后端再转发到 DNS Agent
  - 成功后写入 `Created DNS record` 审计记录，并按正向区域和关联反向区域延迟合并创建 `runtime.refresh.dns.zone` 局部刷新任务
- `PUT /api/dns/records/{id}` - 编辑 DNS 记录值
  - 请求体支持 `value` 和可选 `createPtr`
  - `{id}` 为后端编码后的实时 DNS 记录标识
  - 正向区域仅支持编辑 A 和 CNAME 记录；反向区域仅支持编辑 PTR 记录
  - 仅允许修改记录值和 A 记录的 `createPtr` 标记，名称、类型、TTL 和所属区域不变
  - 后端会基于数据库快照校验同名同类型同值重复记录
  - A 记录值必须是合法 IPv4 地址
  - CNAME 记录值必须是以 `.` 结尾的合法域名
  - PTR 记录值必须是以 `.` 结尾的合法域名
  - 后端优先通过 DNS Agent 的 body 版更新接口完成编辑；旧 Agent 不支持该接口并返回 `404` 时，会回退为删除旧记录并创建新记录；未修改内容时也会返回当前记录
  - A 记录编辑窗口会展示“更新相关的指针 PTR 记录”配置项；A 记录带 `createPtr` 标记时，会同步维护旧值和新值对应的 PTR 快照，并在响应中通过 `relatedRecords` 返回新 PTR 记录
  - 成功后写入 `Updated DNS record` 审计记录，并按当前区域延迟合并创建 `runtime.refresh.dns.zone` 局部刷新任务
- `DELETE /api/dns/records/{id}` - 删除 DNS 记录
  - `{id}` 为后端编码后的实时 DNS 记录标识
  - 标识中包含服务器 ID、区域名称、记录类型、记录名称和值
  - 正向区域仅支持删除 A 和 CNAME 记录；反向区域仅支持删除 PTR 记录
  - 删除反向区域 PTR 记录时，后端会把完整 IPv4 展示名转换为 Windows DNS 区域内相对名称后再转发给 Agent
  - 删除正向 A 记录时，后端会按实际匹配关系同步删除对应 PTR 快照，不只依赖 `createPtr` 标记
  - 如果数据库中没有实际匹配的 PTR 且对应反向区域不存在，删除 A 记录后不会凭 IP 推导创建反向区域刷新任务
  - Go DNS Agent 删除 CNAME、PTR、NS 等域名值记录时，会忽略值末尾点和大小写差异；值未精确匹配但同名同类型记录唯一时，会按该唯一记录兜底删除
  - 成功后写入 `Deleted DNS record` 审计记录

创建 DNS 记录请求示例：

```json
{
  "zoneId": "encoded-zone-id",
  "name": "www",
  "type": "A",
  "value": "10.0.0.20",
  "ttl": 3600
}
```

编辑 DNS 记录值请求示例：

```json
{
  "value": "10.10.10.20"
}
```

## 10.5 实时事件

- `GET /api/events` - SSE 事件流
  - 建立连接后先发送 `connected` 事件
  - Redis 收到刷新消息后按事件 `type` 输出，缺省类型为 `runtime.updated`

## 10.6 健康检查

- `GET /api/health` - 后端健康检查
  - 返回整体 `status`、当前 UTC 时间和 `services`
  - `services.postgresql.status` 表示 PostgreSQL 连接状态
  - `services.redis.status` 表示 Redis 连接状态
  - 前端右上角服务状态图标会根据该接口动态显示 PostgreSQL 与 Redis 在线状态
  - 检测到 PostgreSQL 或 Redis 异常时会尽力写入通知中心；PostgreSQL 不可用时通知可能无法落库
  - 后续检测到 PostgreSQL 或 Redis 恢复在线时，会自动标记已读并清空对应异常通知

## 10.7 通知中心

- `GET /api/notifications` - 获取右上角通知中心消息，仅返回 Agent 异常和平台基础服务异常等非刷新任务消息
  - 需要 `notifications.read` 权限
  - 查询参数 `limit` 默认 20
- `GET /api/notifications/unread-count` - 获取未读通知数量，计入 Agent 异常和平台基础服务异常
  - 需要 `notifications.read` 权限
- `POST /api/notifications/{id}` - 标记单条通知已读
  - 需要 `notifications.manage` 权限
- `POST /api/notifications/read-all` - 标记全部通知已读
  - 需要 `notifications.manage` 权限
  - 成功后写入 `notifications.read_all` 审计记录
- `POST /api/notifications/clear` - 清空通知中心消息
  - 需要 `notifications.manage` 权限
  - 成功后写入 `notifications.clear` 审计记录

## 10.8 系统配置

- `GET /api/public/base` - 获取公开系统基础配置
  - 无需登录，返回站点名称、登录展示、控制台品牌和图标等公开字段
  - 登录页、启动页和控制台布局会读取该接口同步品牌展示
- `GET /api/settings/base` - 获取系统基础配置
  - 返回站点名称、登录展示、控制台品牌、安全时效、同步并发、操作后刷新等待、Agent 离线判定次数、Agent 连接超时、Agent 操作超时、Agent 全量同步超时、自动健康检查间隔和自动检查并发
  - 会话有效期和定时全量同步间隔由后端环境变量控制，不属于基础配置接口
- `PUT /api/settings/base` - 保存系统基础配置
  - 请求体为完整基础配置对象
  - `dnsRecordConcurrency` 表示 DNS 区域记录采集并发，范围 1 到 50 个，默认 3 个
  - `dhcpScopeConcurrency` 表示 DHCP 作用域详情采集并发，范围 1 到 50 个，默认 5 个
  - `operationRefreshDelaySeconds` 表示 DNS / DHCP 操作成功后局部刷新等待窗口，范围 1 到 60 秒，默认 10 秒
  - `agentConnectionTimeoutSeconds` 表示 Agent 保存前探测、手动测试、自动健康检查和同步前 `/health` 检查超时时间，范围 1 到 20 秒，默认 5 秒
  - `agentOperationTimeoutSeconds` 表示 DNS / DHCP 管理操作超时时间，范围 1 到 60 秒，默认 20 秒
  - `agentFullSyncTimeoutSeconds` 表示全量同步、单 Agent 同步和局部同步超时时间，范围 60 到 600 秒，默认 300 秒
  - `agentHealthCheckIntervalMinutes` 表示后端自动健康检查间隔，范围 1 到 60 分钟，默认 1 分钟
  - `agentHealthCheckConcurrency` 表示后端自动健康检查同时检查的 Agent 数量，范围 1 到 20 个，默认 1 个
  - 保存后找回密码安全时效、同步并发、操作后刷新等待、Agent 离线判定次数、Agent 连接超时、Agent 操作超时、Agent 全量同步超时、自动健康检查间隔和自动检查并发会被运行逻辑读取
  - 保存成功后写入 `Updated system base config` 审计记录
- `GET /api/settings/users` - 获取用户配置列表
  - 返回平台用户列表，`directRoles` 表示用户直接勾选的角色
  - `roles` 和 `permissions` 表示叠加用户群组后的有效角色与权限集合
  - 用户邮箱用于找回密码身份校验
- `POST /api/settings/users` - 创建平台用户
  - 请求字段包含 `username`、`email`、`password`、`displayName`、`roleKeys` 和 `disabled`
  - 新建用户密码至少 6 位
  - 保存成功后写入 `settings.user.create` 审计记录
- `PUT /api/settings/users/{id}` - 更新平台用户
  - 请求字段包含 `username`、`email`、`password`、`displayName`、`roleKeys` 和 `disabled`
  - `password` 留空时不修改密码
  - 保存成功后写入 `settings.user.update` 审计记录
- `POST /api/settings/users/{id}/disabled` - 启用或禁用平台用户
  - 请求字段包含 `disabled`
  - 默认管理员和当前登录用户受保护
- `DELETE /api/settings/users/{id}` - 删除平台用户
  - 需要先禁用目标用户
  - 默认管理员和当前登录用户受保护
- `GET /api/settings/roles` - 获取用户角色列表
- `POST /api/settings/roles` - 创建自定义角色
- `PUT /api/settings/roles/{id}` - 更新自定义角色
- `DELETE /api/settings/roles/{id}` - 删除自定义角色
  - 内置角色不可修改或删除
- `GET /api/settings/user-groups` - 获取用户群组列表
- `POST /api/settings/user-groups` - 创建用户群组
- `PUT /api/settings/user-groups/{id}` - 更新用户群组成员、角色和状态
- `DELETE /api/settings/user-groups/{id}` - 删除用户群组
- `GET /api/settings/permissions` - 获取可分配权限列表
  - `*.manage` 权限会自动补齐对应 `*.read` 权限
  - `notifications.manage` 会自动补齐 `notifications.read`
  - `refresh.manage` 仅控制刷新执行，通常建议搭配 `audit.read` 查看任务执行记录
  - `export.manage` 控制 DNS、DHCP、任务和审计数据导出入口
  - `dashboard.read` 会允许仪表板读取统计所需的 Agent、DNS、DHCP 和最近活动数据
  - 后端对 DNS、DHCP、Agent、刷新、通知中心和系统配置接口执行权限校验，前端会按权限隐藏对应操作入口；DNS / DHCP 管理页右上刷新和局部刷新按钮需要 `refresh.manage`
- `GET /api/settings/auth-providers` - 获取认证配置列表
  - 当前返回 AD/LDAP 认证配置
- `PUT /api/settings/auth-providers/{id}` - 保存认证配置
  - `{id}` 当前仅支持 `ldap`
  - 请求字段包含 `name`、`enabled` 和 LDAP 连接配置
  - 密码留空且已有配置时保留原绑定密码
- `POST /api/settings/auth-providers/{id}/test` - 测试认证配置
  - `{id}` 当前仅支持 `ldap`
  - 返回 `matchedUsers` 表示 LDAP 搜索匹配用户数量
- `GET /api/settings/notifications` - 获取通知媒介列表
  - 当前返回 `email` 邮件媒介
  - SMTP 密码只返回 `passwordConfigured`，不返回原文
- `PUT /api/settings/notifications/{id}` - 保存通知媒介配置
  - `{id}` 当前仅支持 `email`
  - 当前界面只配置 `passwordResetEnabled`，用于找回密码验证码发送
  - 可配置 SMTP 主机、端口、用户名、密码、发件人、TLS、STARTTLS 和明文认证开关
  - 密码留空时保留原已配置密码
  - 保存成功后写入 `settings.notification.update` 审计记录
- `POST /api/settings/notifications/{id}/test` - 测试通知媒介
  - `{id}` 当前仅支持 `email`
  - 请求字段 `to` 为临时测试收件人，不保存到配置
  - 成功后写入 `settings.notification.test` 审计记录
- `POST /api/settings/notifications/{id}/preview` - 预览邮件通知模板
  - `{id}` 当前仅支持 `email`
  - 该兼容接口当前不在前端邮件媒介界面中使用

邮件媒介请求示例：

```json
{
  "id": "email",
  "name": "邮件媒介",
  "enabled": false,
  "passwordResetEnabled": true,
  "config": {
    "smtpHost": "smtp.example.com",
    "smtpPort": 465,
    "username": "zonelease@example.com",
    "password": "smtp-password",
    "from": "zonelease@example.com",
    "fromName": "ZoneLease",
    "useTLS": true
  }
}
```

## 10.9 刷新

- `POST /api/refresh` - 创建刷新任务并发布 SSE 事件
  - 需要 `refresh.manage` 权限
  - 请求字段 `type` 为空时默认为 `runtime.refresh.all`
  - `type` 仅允许 `runtime.refresh.all`、`runtime.refresh.dns.all` 和 `runtime.refresh.dhcp.all`
  - 同类型全量刷新任务正在执行时返回 `409 refresh_running`，不创建重复任务
  - 后端写入 `refresh_tasks` 记录，后台同步所有可同步 Agent，并写入 `Queued refresh` 审计记录
  - 后端定时任务会分别创建 `runtime.refresh.dns.all` 和 `runtime.refresh.dhcp.all`，仅同步对应角色 Agent
  - 全量刷新执行期间和完成 / 失败后会在 payload 中写入 `totalAgents`、`startedAgents`、`syncedAgents`、`failedAgents`、`skippedAgents`、`warn`、`currentAgent`、`agentEvent`、`startedAt`、`finishedAt` 和 `agentResults`
  - 刷新范围、任务状态、Redis/SSE 事件和定时刷新规则详见 [docs/refresh-sync.md](docs/refresh-sync.md)
- `GET /api/refresh/tasks` - 获取刷新任务列表
  - 需要 `audit.read` 权限
  - 查询参数 `limit` 默认 30；传 `all` 时返回全部任务

刷新请求示例：

```json
{
  "type": "runtime.refresh.all"
}
```

## 10.10 服务器管理

- `POST /api/servers` - 添加 Windows DNS / DHCP 服务器登记
  - 请求字段包含 `name`、`host`、`role`、`agentUrl`、`apiKey` 和 `tlsInsecure`
  - `name`、`role` 与 `agentUrl` 必填，`apiKey` 可选
  - `host` 为空时默认使用 `name`
  - 保存前会校验 Agent 名称、接口地址唯一性和角色合法性；重复时返回 `409`，提示 `Agent 名称已存在` 或 `Agent 接口地址已存在`
  - 保存接口不再重复访问 Agent；前端会要求当前 Agent 地址、角色、API Key 和 TLS 设置先通过 `POST /api/servers/probe` 测试
  - `role` 仅支持 `DNS` 或 `DHCP`；DNS 与 DHCP 需要分别登记为两个服务器 Agent
  - 校验通过后以 `Online` 状态写入服务器登记，前端随后自动创建单 Agent 同步任务，并通过 `skipHealthCheck=1` 跳过首次同步前 `/health` 预检查
  - 创建成功后写入 `Created server` 审计记录
- `POST /api/servers/probe` - 测试未保存的 Agent 连接
  - 请求字段包含 `name`、`host`、`role`、`agentUrl`、`apiKey` 和 `tlsInsecure`
  - 调用请求体中的 Agent `/health`；DNS 角色继续探测 `/dns/zones`，DHCP 角色继续探测 `/dhcp/probe`，不写入服务器表和审计记录
- `DELETE /api/servers/{id}` - 删除服务器登记
  - 服务器不存在时返回 `server_not_found`，删除动作执行失败时返回 `delete_server_failed`
  - 删除成功后会清理该 Agent 的离线通知和健康检查运行态缓存
  - 删除成功后写入 `Deleted server` 审计记录
- `POST /api/servers/{id}/ping` - 调用已登记 Agent 的 `/health`，并按角色探测 `/dns/zones` 或 `/dhcp/probe` 后更新服务器状态；手动检查失败会立即标记为 `Offline`
  - 可选查询参数 `mode=auto`，用于静默检查，失败时按 Agent 离线失败次数阈值累计且不写审计
  - Agent 可达时状态写入 `Online`
  - Agent 不可达时默认状态写入 `Offline`，响应 `detail` 包含失败原因
  - 手动检查完成后写入 `Checked server health` 审计记录；后端自动健康检查不写审计
- `POST /api/servers/{id}/sync` - 同步指定服务器 Agent 数据
  - 创建 `runtime.refresh.server` 刷新任务
  - 仅同步当前服务器对应角色的 DNS / DHCP 快照
  - 成功排队后写入 `Queued server sync` 审计记录

添加服务器请求示例：

```json
{
  "name": "DC01",
  "host": "dc01.example.local",
  "role": "DNS",
  "agentUrl": "http://10.0.0.10:8460",
  "apiKey": "change-me",
  "tlsInsecure": false
}
```

## 10.11 状态

- `GET /api/state` - 获取控制台状态
  - 返回服务器、DNS 区域、DNS 记录、DHCP 作用域、排除范围、租约、保留地址和审计记录
  - 默认只读取 PostgreSQL 快照，不会实时读取 DNS Agent
  - 查询参数 `includeDns=true` 为兼容参数，当前不会触发 Agent 实时读取
  - 区域和记录标识由后端编码生成，不代表 Windows DNS 内部 ID

审计记录由后端在关键用户操作成功后写入 `audit_entries`，并通过 `GET /api/state` 的 `audit` 字段返回给前端。当前覆盖范围包括：

- 认证与账号、服务器与 Agent、DNS、DHCP、系统配置、通知中心和刷新任务等关键操作。
- 审计项包含动作、用户、资源、客户端 IP、发生时间和结构化审计元数据。
- 刷新任务写入 `refresh_tasks`，通知中心消息写入 `notifications`。
- 后端启动后会按 `LOG_RETENTION_DAYS` 每日清理过期的 `refresh_tasks`、`audit_entries` 和 `notifications`。

任务、审计和通知中心的详细写入边界见 [docs/operation-log-coverage.md](docs/operation-log-coverage.md)。

## 10.12 Agent API

以下接口由部署在 Windows 服务器上的 Agent 服务提供，不属于后端控制中心 Swagger 文档范围。

Agent 除 `GET /health` 外的业务接口会在 `ALLOW_ANONYMOUS=false` 且对应 `*_AGENT_API_KEY` 非空时校验 `X-API-Key: <AGENT_API_KEY>`；如果 API Key 留空，Agent 不会执行业务接口 API Key 校验。响应统一采用 envelope：`success` 表示是否成功，`data` 承载业务数据，`error` 承载错误对象，`requestId` 用于排查日志。

DNS Agent 默认监听 `8460`：

- `GET /health` - DNS Agent 健康检查，返回 `status=ok` 和 `role=dns-agent`
- `GET /dns/zones` - 读取 Windows DNS 区域列表
- `POST /dns/zones` - 创建 Windows DNS 区域
  - Go DNS Agent 会优先创建 AD 集成 Primary Zone；如果目标服务器不支持或不适用 AD 集成，会回退为文件型 Primary Zone
  - 创建反向区域时，Agent 会把完整 `.in-addr.arpa` 区域名转换为 PowerShell `-NetworkId` 需要的网络 ID
  - Agent 创建后会用 `Get-DnsServerZone` 二次确认区域存在，确认失败时接口返回错误，后端不会写入成功快照
- `DELETE /dns/zones/{zone}` - 删除指定 DNS 区域
- `GET /dns/zones/{zone}/records` - 读取指定区域下 DNS 记录
  - 新版后端优先调用 `POST /dns/records/query`，把区域名放在 JSON body 中，避免明文 HTTP 路径中的特殊域名被网络安全设备误拦截；该 GET 接口保留用于兼容旧 Agent
  - 目标 Agent 不支持 body 版接口返回 `404`，或 Windows Server 2008/2008 R2 legacy Agent 缺少 `.NET System.Web.Extensions` 导致 JSON body 解析返回 `500` 时，会回退到该 GET 接口
- `POST /dns/records/query` - 读取指定区域下 DNS 记录
  - 请求体为 `{"zone":"example.com"}`
- `POST /dns/records/create` - 创建指定区域下 DNS 记录
  - 请求体为 `{"zone":"example.com","record":{...}}`
- `POST /dns/records/delete` - 删除指定 DNS 记录
  - 请求体为 `{"zone":"example.com","record":{...}}`
- `POST /dns/records/update` - 更新指定 DNS 记录
  - 请求体为 `{"zone":"example.com","update":{"old":{...},"new":{...}}}`
  - Go DNS Agent 会在单个 PowerShell 脚本内完成主记录旧值查找、删除和新值创建；legacy PowerShell Agent 也支持该 body 版接口
- `POST /dns/zones/{zone}/records` - 创建指定区域下 DNS 记录，保留用于兼容旧 Agent
- `PUT /dns/zones/{zone}/records` - 更新指定区域下 DNS 记录，保留用于兼容旧 Agent
- `DELETE /dns/zones/{zone}/records/{type}/{name}?value={value}` - 删除指定 DNS 记录，保留用于兼容旧 Agent

新版后端的 DNS 记录读取、创建、删除和编辑相关操作说明：

- 优先调用 `/dns/records/*` body 版接口。
- 区域名放在 JSON body 中，避免 `youtube.com` 等特殊区域名出现在明文 HTTP URL 路径里被网络安全设备或代理误拦截。
- 目标 Agent 尚未支持 body 版接口并返回 `404` 时，后端会回退到旧路径接口兼容 Windows Server 2008/2008 R2 legacy Agent。
- 读取记录时，如果 legacy Agent 因缺少 `.NET System.Web.Extensions` 无法解析 JSON body 并返回 `500`，后端也会回退到旧 GET 记录接口。

DHCP Agent 默认监听 `8462`：

- `GET /health` - DHCP Agent 健康检查，返回 `status=ok` 和 `role=dhcp-agent`
- `GET /dhcp/probe` - DHCP Agent 轻量连通性测试，不枚举作用域
- `GET /dhcp/scopes` - 读取 Windows DHCP 作用域列表
- `GET /dhcp/scopes/{scopeId}` - 读取单个作用域基础信息；Go Agent 和 legacy Agent 均支持
- `POST /dhcp/scopes` - 创建 DHCP 作用域
- `PUT /dhcp/scopes/{scopeId}` - 更新 DHCP 作用域名称、描述、默认网关、租期和地址范围；启停状态建议通过切换接口处理
- `POST /dhcp/scopes/state` - 通过 JSON body 启用或停用 DHCP 作用域
- `DELETE /dhcp/scopes/{scopeId}` - 删除 DHCP 作用域
- `POST /dhcp/scopes/{scopeId}/activate` - 启用 DHCP 作用域
- `POST /dhcp/scopes/{scopeId}/deactivate` - 停用 DHCP 作用域
- `GET /dhcp/scopes/{scopeId}/details` - 一次读取指定作用域排除范围、租约和保留地址详情；Go Agent 和 legacy Agent 均支持
- `GET /dhcp/scopes/{scopeId}/exclusions` - 读取指定作用域排除范围
- `POST /dhcp/exclusions` - 创建 DHCP 排除范围
- `POST /dhcp/exclusions/delete` - 删除 DHCP 排除范围
- `GET /dhcp/scopes/{scopeId}/leases` - 读取指定作用域租约
- `POST /dhcp/leases/release` - 通过 JSON body 释放指定租约
- `DELETE /dhcp/scopes/{scopeId}/leases/{ip}` - 释放指定租约
- `GET /dhcp/scopes/{scopeId}/reservations` - 读取指定作用域保留地址
- `POST /dhcp/reservations` - 创建 DHCP 保留地址
- `POST /dhcp/reservations/update` - 通过 JSON body 更新 DHCP 保留地址
- `POST /dhcp/reservations/delete` - 通过 JSON body 删除 DHCP 保留地址
- `PUT /dhcp/reservations/{scopeId}/{ip}` - 更新 DHCP 保留地址，保留用于兼容旧调用方
- `DELETE /dhcp/reservations/{scopeId}/{ip}` - 删除 DHCP 保留地址
- `POST /dhcp/cache/clear` - legacy Agent 清理本次全量 DHCP 同步使用的全局 dump 缓存

Windows Server 2008/2008 R2 legacy DHCP Agent 也支持以上 DHCP 接口。legacy 模式说明如下：

- 使用 `netsh dhcp server` 执行操作，不依赖新版 `DhcpServer` PowerShell 模块。
- 对中文系统输出与列分隔差异做行内解析兼容。
- 后端同步服务识别到 `/health` 返回 `mode=legacy` 后，会从全局 `dump` 读取作用域基础信息，再改用逐作用域接口采集租约，并发遵循系统配置中的 DHCP 作用域并发。
- legacy Agent 本地通过 runspace worker 池处理这些 HTTP 请求；实际 DHCP 作用域详情并发由后端同步参数中的 DHCP 作用域并发控制。
- 作用域列表会先入库。
- 每个作用域详情优先通过 `GET /dhcp/scopes/{scopeId}/details` 一次返回排除范围、租约和保留地址。
- `GET /dhcp/scopes/{scopeId}/details` 中排除范围和保留地址读取 dump 缓存，租约继续执行 `show clients 1`，成功后立即写入该作用域快照。
- 单作用域刷新会通过 `GET /dhcp/scopes/{scopeId}` 只读取目标作用域基础信息，不再调用 `GET /dhcp/scopes` 枚举全部作用域；legacy Agent 会执行 `scope <scopeId> dump` 并不调用 `/dhcp/cache/clear`。
- 后端仅在 legacy 全量 DHCP 同步结束后调用 legacy Agent 清理本次同步使用的全局 dump 缓存。
- legacy 日志会记录 scopes、scope details 和 leases 的 start/done 阶段，便于定位卡住的作用域租约枚举。
- 如果调用方超时断开连接，响应写入阶段会记录 `Client disconnected`。

Go DHCP Agent 同步说明：

- 后端在全量同步和单作用域刷新时优先调用 `GET /dhcp/scopes/{scopeId}/details`。
- 后端在 Go Agent 单作用域刷新时通过 `GET /dhcp/scopes/{scopeId}` 读取目标作用域基础信息。
- 每个作用域详情从 3 次 PowerShell 调用减少为 1 次 PowerShell 调用。
- Go Agent 会强制 PowerShell stdout 使用 UTF-8，避免中文描述乱码。
- Go Agent 会去除 Windows `ClientId` 中的 `-`、`:` 和 `.` 分隔符后写入 MAC 字段。

# 十一、版本历史

## v1.0.1 - 2026-07-15

修复前端开发服务启用 Nitro 插件后 `/api` 请求无法通过 Vite 代理转发到后端的问题，同时保留生产构建的 Nitro `.output` 输出链路。

- Nitro Vite 插件仅在 `vite build` 时启用。
- `npm run dev` 恢复使用 Vite 开发服务器的 `/api` 代理配置。
- 优化 README 文档内容。
- 详细更新内容见 [verchanglog/v1.0.1.md](verchanglog/v1.0.1.md)。

## v1.0.0 - 2026-06-29

首个正式版本，提供 Windows DNS / DHCP 统一管理控制台、Go 后端服务、DNS / DHCP Agent、刷新同步链路、操作审计、通知中心、Docker Compose 部署和 GitHub Actions 发布流水线。

- 前端生产服务改为 TanStack React Start + Nitro `.output` 输出。
- Docker 单镜像内集成 Go 后端、Nginx 和前端 Nitro SSR 服务。
- DHCP Go Agent 优化作用域详情聚合、租期识别、保留地址和租约操作。
- 完善 DNS / DHCP 刷新同步、Redis/SSE 通知、审计日志和导出能力。
- 详细更新内容见 [verchanglog/v1.0.0.md](verchanglog/v1.0.0.md)。

# 十二、许可证

本项目采用 MIT License，详见 [LICENSE](LICENSE)。

# 十三、致谢

感谢 Go、React、Vite、Tailwind CSS、PostgreSQL、Redis、PowerShell 以及 Windows DNS / DHCP 相关生态。

# 十四、联系方式

- **作者**：Jerion
- **邮箱**：416685476@qq.com
- **项目地址**：https://github.com/zyx3721/zonelease
