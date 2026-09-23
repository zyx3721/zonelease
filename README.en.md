<div align="center">

<h1>ZoneLease</h1>

<p><b>Visual Management Console for Windows DNS / DHCP</b> — Zones · Records · Scopes · Leases · Reservations · Audit</p>

<p><a href="README.md">简体中文</a> · <b>English</b></p>

Windows DNS and DHCP administration is usually scattered across Server Manager, the DNS Manager snap-in and one-off PowerShell commands — one wrong step and nobody knows what changed.  
ZoneLease brings them into a single **self-hosted** console: a Go backend, a React dashboard, PostgreSQL and Redis, plus lightweight agents that run PowerShell on your Windows servers.  
Manage DNS/DHCP across multiple Windows Servers from one place — every change hits the real server first, then converges the database snapshot, with a full audit trail.

<p>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?labelColor=1f2937" alt="MIT License"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white&labelColor=1f2937" alt="Go 1.25+"></a>
  <a href="https://react.dev/"><img src="https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=white&labelColor=1f2937" alt="React 19"></a>
  <a href="https://www.postgresql.org/"><img src="https://img.shields.io/badge/PostgreSQL-17-4169E1?logo=postgresql&logoColor=white&labelColor=1f2937" alt="PostgreSQL 17"></a>
  <img src="https://img.shields.io/badge/Permissions-20-059669?labelColor=1f2937" alt="20 permissions">
</p>

<p>
  <b><a href="#project-preview">Preview</a></b> ·
  <a href="#what-it-does">What it does</a> ·
  <a href="#how-it-works">How it works</a> ·
  <a href="#tech-stack">Tech stack</a> ·
  <a href="#quick-start">Quick start</a> ·
  <a href="#deployment">Deployment</a> ·
  <a href="#permission-model">Permissions</a> ·
  <a href="#security">Security</a> ·
  <a href="#faq">FAQ</a> ·
  <a href="#repository-layout">Project structure</a> ·
  <a href="#docs">Docs</a>
</p>

</div>

---

## Project preview

### Sign in

Local password, AD/LDAP and WeCom (WeChat Work) QR sign-in, with CAPTCHA + email code password recovery.

![Sign-in page](.github/images/zonelease-login.jpg)

### Dashboard

The dashboard summarizes DNS zones, DNS records, DHCP scopes, leases and server health; server cards support manual refresh and scheduled health checks, with recent activity streamed from the audit log.

![Dashboard](.github/images/zonelease-home.jpg)

## What it does

- **DNS management** — Syncs Windows DNS zones and records into snapshots; create forward / reverse zones, create, edit and delete A / CNAME records, opt into related PTR records with automatic reverse-zone snapshot maintenance; conflicts (same-name exclusivity, CNAME value format, PTR reverse-zone existence) are validated before saving.
- **DHCP management** — Manage Windows DHCP scopes, exclusions, leases and reservations through agents; create / edit / activate / deactivate / delete scopes, release leases, create / edit / delete reservations; successful operations trigger delayed, merged per-scope snapshot syncs.
- **Server management** — Register Windows DNS / DHCP agent endpoints with optional API keys; connection tests, manual syncs and scheduled health checks; offline thresholds, timeouts and concurrency are all tunable in system settings.
- **Real-time refresh** — Full refresh, per-agent sync, DNS zone refresh and DHCP scope refresh all run as background tasks with progress published over Redis as SSE events; post-operation partial refreshes are delayed and merged per target.
- **Users & permissions** — Local password, AD/LDAP and WeCom (WeChat Work) QR login (direct WeCom or unified SSO auth center), password recovery (CAPTCHA + email code), WeCom account binding / unbinding; built-in `admin` / `operator` / `viewer` roles plus custom roles with 20 granular permissions — unauthorized entries are hidden in the UI and enforced per endpoint in the backend.
- **Audit & notifications** — Logins, server, DNS, DHCP, refresh and auth-config changes are fully audited; agent offline and platform service incidents surface in the notification center and auto-clear on recovery.
- **System settings** — Base settings (branding, security TTLs, sync parameters, agent thresholds), users / groups / roles, auth settings (AD/LDAP and WeCom), email channel for password recovery codes.

**It is not** a discovery or monitoring platform: ZoneLease focuses on "visual management + audited changes + snapshot browsing" — no network scanning, no metrics collection, no zone-transfer proxying.

The same job, two ways:

| Today | Manual PowerShell / Server Manager | With ZoneLease |
| --- | --- | --- |
| Change a record | Remote in, run commands, retype on typos | Edit in the UI with pre-save validation |
| Who changed it | No record | Full audit by action / target / result |
| Multiple servers | One by one | Register agents once, switch in the UI |
| PTR records | Manually maintain reverse zones | One checkbox, snapshots kept in sync |
| Outages | Wait for complaints | Scheduled health checks + notification center |
| Access | Shared admin password | 20 permissions enforced per endpoint |

## How it works

```text
        Browser
           │  http
           ▼
  ┌────────────────────────────────────────────┐
  │  Nginx (in-container or on the host)       │
  │  /            → Frontend SSR               │
  │  /api/ · /swagger/ → Go backend            │
  └────────────────────────────────────────────┘
        │                        │
        ▼                        ▼
  React 19 console           Go 1.25 backend
  Nitro SSR :5173            net/http :8080
        │                        │
        │              ┌─────────┴─────────┐
        │              ▼                   ▼
        │        PostgreSQL 17         Redis 7
        │        snapshots & audit     SSE events & locks
        │                                 │
        └──────────┐                      │
                   ▼                      │
        ┌──────────────────────┐          │
        │ Windows agents (xN)  │◀── HTTPS ┘
        │  dns-agent           │  health / collect / mutate
        │  dhcp-agent          │  → Windows PowerShell
        └──────────────────────┘
```

- **Who does what** — The console reads PostgreSQL snapshots for display; create / edit / delete operations are forwarded by the backend to the target Windows agent, which runs PowerShell; only after success does the backend update the snapshot and write the audit entry.
- **Where data lives** — PostgreSQL is the source of truth for snapshots and audit; Redis only carries SSE refresh events, recent-event replay, refresh-task progress and short-lived distributed locks.
- **Agent boundary** — Agents expose `/health` and DNS/DHCP resource endpoints guarded by `X-API-Key`; Windows Server 2008/2008 R2 uses the legacy scripts, newer systems use the Go agents.
- **Exposure** — Only health check, login, public auth providers, WeCom callbacks, password recovery and branding endpoints are anonymous; everything else requires `Authorization: Bearer <token>`.

## Tech stack

| Layer | Choice |
| --- | --- |
| Backend language | Go 1.25+ |
| HTTP service | Standard library `net/http` (Go 1.22+ method routing) |
| Database | PostgreSQL 17 ([jackc/pgx](https://github.com/jackc/pgx) v5) |
| Cache & events | Redis 7 ([redis/go-redis](https://github.com/redis/go-redis) v9) |
| Auth & crypto | Bearer sessions (bcrypt), AD/LDAP (go-ldap/ldap v3), WeCom QR login (direct / unified SSO auth center) |
| API docs | swag + http-swagger (Swagger UI) |
| Frontend | React 19 + TanStack Start / Router |
| Language & build | TypeScript 5 + Vite 7 + Nitro |
| Styling & components | Tailwind CSS v4, Radix UI, lucide-react, sonner |
| Windows collection | dns-agent / dhcp-agent (Go + PowerShell), legacy scripts for 2008/2008 R2 |
| Runtime packaging | Docker (Nginx + Supervisor multi-process) |

## Quick start

Local development needs **Go 1.25+**, **Node.js 20+**, **PostgreSQL 17** and **Redis 7**.

```bash
git clone https://github.com/zyx3721/zonelease.git
cd zonelease
```

**Database**

```bash
createdb -U postgres zonelease   # or use psql / a GUI tool
```

**Backend**

```bash
cd backend
cp .env.example .env      # adjust database, Redis connection and JWT_SECRET
go run cmd/server/main.go
```

The backend listens on `http://127.0.0.1:8080` by default, runs migrations on first start and creates the default admin `admin / 123456`.

**Frontend** (in another terminal)

```bash
cd frontend
npm install
npm run dev
```

The frontend runs on `http://localhost:5173` and proxies `/api` to the backend.

Open `http://localhost:5173`, sign in with `admin / 123456`, and **change the password right away**. Swagger is at `http://127.0.0.1:8080/swagger/index.html`.

Windows servers also need an agent deployed before anything can be collected or changed — see [chapter 5 of the full manual](docs/manual.md) (Chinese).

## Deployment

Two paths only: **Docker deployment** (recommended) and **Release binaries**. Building from source and running under systemd directly are covered in the [full manual](docs/manual.md) (Chinese).

### Option 1: Docker

The image bundles the Go backend, Nginx and the frontend Nitro SSR behind Supervisor, exposing only port 80. PostgreSQL and Redis come from the bundled Compose file:

```bash
git clone https://github.com/zyx3721/zonelease.git && cd zonelease/deploy
# prepare backend/.env first (copy from backend/.env.example)
docker compose up -d
```

Compose creates `zonelease-postgres`, `zonelease-redis` and `zonelease` containers; the app image is `registry.cn-shenzhen.aliyuncs.com/zyx3721/zonelease:latest` (also published to Docker Hub as `zyx3721/zonelease`).

Key environment variables (full list in [chapter 6.8 of the manual](docs/manual.md)):

| Variable | Default | Description |
| --- | --- | --- |
| `JWT_SECRET` | empty | Signing key for password-reset captcha and WeCom OAuth state; does not affect issued sessions; set a long random string in production |
| `SERVER_PORT` | `8080` | Backend listen port, reverse-proxied by Nginx in the container |
| `SERVER_MODE` | `release` | Run mode; `release` hides debug recovery codes |
| `DB_HOST` / `DB_PORT` | `localhost` / `5432` | PostgreSQL address |
| `DB_NAME` / `DB_USER` / `DB_PASSWORD` | `zonelease` / `zonelease` / `zonelease_dev` | Database and credentials |
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `JWT_EXPIRE_HOURS` | `12` | Session lifetime in hours, applied uniformly to password, LDAP and WeCom logins; no idle timeout |

Service management:

```bash
docker compose ps                    # status
docker logs -f zonelease             # logs
docker compose restart zonelease     # restart
docker compose up -d --force-recreate zonelease   # recreate after pulling a new image

# upgrade
docker pull registry.cn-shenzhen.aliyuncs.com/zyx3721/zonelease:latest
```

**Access**

- Console: `http://your-host/` with `admin / 123456`
- API docs: `http://your-host/swagger/index.html`
- Health check: `http://your-host/api/health`

**Last step**: deploy `dns-agent` / `dhcp-agent` on each Windows Server (legacy PowerShell scripts for old systems), then register them under "Agent management". See [chapter 5 of the manual](docs/manual.md).

### Option 2: Release binaries

Grab the matching archives from [GitHub Releases](https://github.com/zyx3721/zonelease/releases), then verify, extract, configure and start.

**Which package to download**

| Purpose | File |
| --- | --- |
| Backend (Linux x86_64) | `zonelease_<version>_linux_amd64.tar.gz` |
| Backend (Linux ARM64) | `zonelease_<version>_linux_arm64.tar.gz` |
| Frontend (required on any platform) | `zonelease-frontend_<version>.tar.gz` |
| DNS agent (Windows x86_64 / ARM64) | `zonelease-dns-agent_<version>_windows_amd64.zip` / `zonelease-dns-agent_<version>_windows_arm64.zip` |
| DHCP agent (Windows x86_64 / ARM64) | `zonelease-dhcp-agent_<version>_windows_amd64.zip` / `zonelease-dhcp-agent_<version>_windows_arm64.zip` |
| Checksums | `zonelease-xxx_<version>_checksums.txt` |

The backend archive contains the `zonelease` binary, `.env.example` and a `README.txt`; the frontend archive contains the Nitro SSR `.output` artifacts. The backend binary has no external runtime dependencies; the frontend SSR requires Node.js on the target machine. Windows agents still need to be deployed on each server — see [chapter 5 of the manual](docs/manual.md).

**1. Verify the download**

```bash
VERSION=1.0.1
mkdir -p /data/zonelease && cd /data/zonelease
sha256sum -c zonelease-frontend_${VERSION}_checksums.txt
```

**2. Extract**

```bash
mkdir -p backend frontend/.output
tar -xzf zonelease_${VERSION}_linux_amd64.tar.gz -C backend --strip-components=1
tar -xzf zonelease-frontend_${VERSION}.tar.gz -C frontend/.output
```

Resulting layout:

```text
/data/zonelease/
├── backend/
│   ├── zonelease          # backend binary
│   ├── .env.example
│   └── data/              # created on first start
└── frontend/
    └── .output/
        ├── public/        # browser static assets
        └── server/
            └── index.mjs  # Nitro SSR entry
```

**3. Configure and start the backend**

```bash
cd /data/zonelease/backend
cp .env.example .env
vim .env               # at minimum set JWT_SECRET
./zonelease
```

The backend listens on `127.0.0.1:8080` by default, runs migrations on first start and creates the default admin `admin / 123456`.

The backend also supports command-line flags; explicitly passed flags take precedence over environment variables and the `.env` file. `./zonelease -v` prints version info (version, commit, build date) and `./zonelease -h` lists all flags:

| Flag | Equivalent env var | Description |
| --- | --- | --- |
| `-host` | `SERVER_HOST` | Backend listen address |
| `-port` | `SERVER_PORT` | Backend listen port |
| `-mode` | `SERVER_MODE` | Run mode `release` or `debug` |
| `-db-host` | `DB_HOST` | PostgreSQL host |
| `-db-port` | `DB_PORT` | PostgreSQL port |
| `-db-name` | `DB_NAME` | PostgreSQL database name |
| `-db-user` | `DB_USER` | PostgreSQL user |
| `-db-password` | `DB_PASSWORD` | PostgreSQL password |
| `-db-sslmode` | `DB_SSLMODE` | PostgreSQL SSL mode |
| `-redis-addr` | `REDIS_ADDR` | Redis address |
| `-redis-password` | `REDIS_PASSWORD` | Redis password |
| `-redis-db` | `REDIS_DB` | Redis database index |
| `-jwt-secret` | `JWT_SECRET` | Signing key for password-reset captcha and WeCom OAuth state |
| `-session-ttl` | `JWT_EXPIRE_HOURS` | Session lifetime in hours |
| `-dns-sync-interval` | `RUNTIME_DNS_DEEP_SYNC_INTERVAL` | DNS deep sync interval (e.g. 1h, 1d) |
| `-dhcp-sync-interval` | `RUNTIME_DHCP_DEEP_SYNC_INTERVAL` | DHCP deep sync interval (e.g. 1h, 1d) |
| `-metric-retention-days` | `METRIC_RETENTION_DAYS` | Metric retention days |
| `-log-retention-days` | `LOG_RETENTION_DAYS` | Log retention days |
| `-metric-stream-maxlen` | `METRIC_STREAM_MAXLEN` | Redis metric stream max length |
| `-cors-origin` | `CORS_ORIGIN` | Allowed CORS origin |
| `-env` | none | Path to the `.env` config file |
| `-v`, `-version` | none | Print version info and exit |

For a persistent service, use systemd:

```ini
# /etc/systemd/system/zonelease-backend.service
[Unit]
Description=ZoneLease Backend
After=network.target postgresql.service redis.service

[Service]
Type=simple
WorkingDirectory=/data/zonelease/backend
ExecStart=/data/zonelease/backend/zonelease
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
systemctl daemon-reload && systemctl enable --now zonelease-backend
```

**4. Start the frontend SSR**

```bash
cd /data/zonelease/frontend
SSR_API_ORIGIN=http://127.0.0.1:8080 HOST=127.0.0.1 PORT=5173 node .output/server/index.mjs
```

When starting up, the SSR process fetches brand settings from the backend and renders the site name and icon into the first-frame HTML. The backend address is resolved in this order: the `SSR_API_ORIGIN` environment variable at runtime → a `.env` file in any directory from the `index.mjs` entry upward (the first existing one wins; it can share the same `.env` as the backend in the deploy root) → the default `http://127.0.0.1:8080` (in development mode it falls back to `VITE_API_BASE_URL` first, sharing the origin with the dev proxy). Make sure the address is reachable from the SSR process; otherwise the first frame falls back to the default brand before switching to the configured one, and the SSR process log prints a `[zonelease-ssr] fetch brand failed` warning.

**5. Put Nginx in front**

```nginx
server {
    listen 80;
    server_name your-domain.com;

    # Frontend static assets: serve .output/public directly so JS/CSS skip the SSR process
    location ^~ /assets/ {
        root /data/zonelease/frontend/.output/public;
        try_files $uri =404;
        access_log off;
        expires 1y;
        add_header Cache-Control "public, immutable";
    }

    # Site favicon
    location = /favicon.svg {
        root /data/zonelease/frontend/.output/public;
        try_files $uri =404;
        access_log off;
        expires 7d;
        add_header Cache-Control "public";
    }

    # SSE long-connection endpoint: disable proxy buffering so refresh events are not cached
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

    location /swagger/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
    }

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 600s;
    }

    location / {
        proxy_pass http://127.0.0.1:5173;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location = /health {
        proxy_pass http://127.0.0.1:8080/api/health;
    }
}
```

The frontend must be served by `node .output/server/index.mjs`; **pointing Nginx only at `.output/public` breaks server-side rendering**. A full example with HTTPS and 80→443 redirect lives in [chapter 4.4 of the manual](docs/manual.md).

**6. Deploy Windows agents and sign in**

Extract the matching DNS / DHCP agent zips on each Windows Server, configure the API key and collection settings, start them, then register under "Agent management". See [chapter 5 of the manual](docs/manual.md).

Access is the same as Docker: console `http://your-domain.com` (`admin / 123456`), API docs at `/swagger/index.html`, health check at `/api/health`.


## Permission model

Endpoints are checked per permission; unauthorized requests get 403. Permission keys look like `module.resource.action` — 20 in total.

| Identity | Default permissions |
| --- | --- |
| Default admin `admin` | All 20 |
| Built-in role `admin` | All 20 |
| Built-in role `operator` | DNS/DHCP read + manage, servers read + manage, refresh, export, notification manage, audit read, read-only for the rest of settings |
| Built-in role `viewer` | Read-only across dashboard, DNS/DHCP, servers, audit and settings |
| Custom roles | Pick from the 20 permissions |

By module:

- **Dashboard** — `dashboard.read`
- **DNS management** — `dns.read` / `dns.manage`
- **DHCP management** — `dhcp.read` / `dhcp.manage`
- **Server management** — `servers.read` / `servers.manage`
- **Refresh / export** — `refresh.manage`, `export.manage`
- **Audit / notifications** — `audit.read`, `notifications.read` / `notifications.manage`
- **System settings** — `settings.{base|users|auth|notifications}.read` / `.manage`

## Security

- **Change the default password first** — update `admin` immediately after the first deployment.
- **Set JWT_SECRET explicitly** — required in production; it signs password-reset captcha and WeCom OAuth state, so rotating it does not affect logged-in sessions, while in-flight password-reset flows must restart.
- **Agent authentication** — configure `X-API-Key` on Windows agents and store matching keys per server in the console; legacy agents support API keys too.
- **Credential redaction** — LDAP bind password, WeCom secret / app secret and SMTP password are stored server-side; API responses only carry "configured" markers.
- **Audit boundaries** — pre-validation failures (bad request, auth failure) are not audited; successful changes always are.
- **Tighten access** — terminate HTTPS at Nginx and restrict source IPs in production; full examples in [chapter 4.4 of the manual](docs/manual.md).

## API docs

The backend ships Swagger/OpenAPI:

- **Swagger UI**: `http://localhost:8080/swagger/index.html`
- **OpenAPI JSON**: `http://localhost:8080/swagger/doc.json`
- **Health check**: `GET /api/health`

Anonymous endpoints are limited to: `POST /api/auth/login`, `GET /api/auth/providers`, `GET /api/auth/wecom/authorize`, `GET /api/auth/wecom/callback`, `POST /api/auth/wecom/exchange`, `POST /api/auth/wecom/login`, `POST /api/auth/logout`, `GET /api/auth/me`, `GET /api/auth/password-reset/captcha`, `POST /api/auth/password-reset/verify`, `POST /api/auth/password-reset/send`, `POST /api/auth/password-reset/confirm`, `GET /api/public/base`, `GET /api/events`, `GET /api/health`; everything else requires `Authorization: Bearer <token>`.

Login request example:

```json
{
  "username": "admin",
  "password": "123456"
}
```

The full endpoint list grouped by module (auth, password reset, DNS, DHCP, events, health, notifications, settings, refresh, servers, state, agent API) lives in [chapter 10 of the manual](docs/manual.md) (Chinese).

After changing endpoints, regenerate the Swagger artifacts in `backend/`:

```bash
swag init -g cmd/server/main.go -o docs
```

## Database

A PostgreSQL database initialized automatically on first start via `migrations/001_init.sql` and `002_user_wecom_bindings.sql` — 22 tables in total.

| Group | Tables |
| --- | --- |
| Users & auth | `users`, `sessions`, `password_reset_requests`, `auth_providers`, `user_wecom_bindings`, `roles`, `user_roles`, `user_groups`, `user_group_members`, `user_group_roles` |
| DNS / DHCP snapshots | `servers`, `dns_zones`, `dns_records`, `dhcp_scopes`, `dhcp_leases`, `dhcp_reservations`, `dhcp_exclusions` |
| Config & runtime records | `system_settings`, `notification_channels`, `audit_entries`, `refresh_tasks`, `notifications` |

## FAQ

**What are the default credentials?**

The backend creates `admin / 123456` on first start when the users table is empty. Change it right away from the user menu.

**Why are tokens invalid after a restart?**

Sessions live in the PostgreSQL `sessions` table and survive restarts. Invalidation usually means the session expired. The lifetime is controlled by `JWT_EXPIRE_HOURS` (default 12h) with no idle timeout or sliding renewal.

**The DNS page shows no zones or records**

Make sure the Windows server is registered and healthy under "Agent management", then trigger a full refresh or a per-agent sync.

**How do I enable AD/LDAP login?**

Fill in the LDAP connection parameters under "System settings → Authentication" and enable it (a connection test is available); external accounts must also exist as enabled users with matching names under "Users".

**How do I enable WeCom QR login?**

Under "System settings → Authentication → WeCom", pick a mode: **Direct WeCom** needs a self-built app in the WeCom admin console (corpid, AgentID, Secret) with this system's domain added as a trusted callback domain and the server IP added to trusted IPs; once WeCom is selected on the login page, the QR code is rendered inline by default and falls back to full-page redirection when the embedded mode is unavailable. **Unified auth center** needs the wecom-auth-center URL, app ID and app secret, with `domain` and `callback_path: /wecom-qr-callback` configured on the auth-center side (a transfer route shared by the embedded QR code and full-page redirection; the auth-center login page must allow being embedded in an iframe by this system, otherwise the login page falls back to full-page redirection). Users sign in with password first, bind their WeCom account from the user menu, and can then use QR login — only WeCom accounts bound to a platform user can sign in.

**An agent shows Offline — what now?**

Check the agent process and port on the Windows server, verify the `X-API-Key` matches the registered one, and review `DNS_AGENT_POWERSHELL_TIMEOUT_SECONDS` and related settings; Windows 2008/2008 R2 must use the scripts under `dns-agent/legacy/` and `dhcp-agent/legacy/`.

## Repository layout

```text
zonelease/
├── backend/                         # Go backend
│   ├── api/router/                  # HTTP routes, handlers, validation, Swagger annotations & models
│   ├── cmd/server/                  # Entrypoint
│   ├── config/                      # Environment config loading
│   ├── docs/                        # Generated Swagger/OpenAPI artifacts
│   ├── internal/                    # Domain models, PostgreSQL repositories, agent client, auth/refresh/sync services
│   ├── pkg/database/                # PostgreSQL connection and migrations
│   └── .env.example                 # Environment template
├── dns-agent/                       # Windows DNS agent (Go + PowerShell)
│   ├── cmd/dns-agent/               # Entrypoint
│   ├── internal/                    # Config, HTTP server, DNS collection & mutation
│   └── legacy/                      # Windows Server 2008/2008 R2 compat scripts
├── dhcp-agent/                      # Windows DHCP agent (Go + PowerShell)
│   ├── cmd/dhcp-agent/              # Entrypoint
│   ├── internal/                    # Config, HTTP server, DHCP collection & mutation
│   └── legacy/                      # Legacy scripts (netsh)
├── frontend/                        # React console
│   └── src/
│       ├── components/              # Layout, boot screen, stat cards, dialogs, base UI
│       ├── features/                # Auth, DNS, DHCP and system settings domains
│       ├── lib/                     # Session auth, branding, refresh events, utilities
│       ├── routes/                  # TanStack Router pages
│       ├── router.tsx               # Router instance
│       ├── server.ts / start.ts     # React Start entries
│       └── styles.css               # Global styles and theme variables
├── deploy/                          # Docker Compose, Dockerfile, Nginx, Supervisor
├── docs/                            # Full manual and runtime docs (DNS/DHCP/refresh/Redis)
├── verchanglog/                     # Release notes
├── AGENTS.md                        # Development conventions
├── LICENSE
└── README.md                        # English readme is README.en.md
```

## Docs

| Start here | Then read |
| --- | --- |
| [Quick start](#quick-start) | Local backend + frontend, default credentials and ports |
| [Deployment](#deployment) | Docker and source builds, environment variables, agent deployment |
| [Permission model](#permission-model) | How the 20 permissions group, built-in roles |
| [Full manual](docs/manual.md) | Complete endpoint list, step-by-step deployment, Nginx/HTTPS examples, troubleshooting (Chinese) |
| [中文 README](README.md) | Same content in Chinese |

## Releases

| Version | Date | Release notes |
| --- | --- | --- |
| v1.1.4 | 2026-09-23 | [verchanglog/v1.1.4.md](verchanglog/v1.1.4.md) |
| v1.1.3 | 2026-09-23 | [verchanglog/v1.1.3.md](verchanglog/v1.1.3.md) |
| v1.1.2 | 2026-09-22 | [verchanglog/v1.1.2.md](verchanglog/v1.1.2.md) |
| v1.1.1 | 2026-09-22 | [verchanglog/v1.1.1.md](verchanglog/v1.1.1.md) |
| v1.1.0 | 2026-09-20 | [verchanglog/v1.1.0.md](verchanglog/v1.1.0.md) |
| v1.0.1 | 2026-07-15 | [verchanglog/v1.0.1.md](verchanglog/v1.0.1.md) |
| v1.0.0 | 2026-06-29 | [verchanglog/v1.0.0.md](verchanglog/v1.0.0.md) |

Build artifacts and notes for each release are on [GitHub Releases](https://github.com/zyx3721/zonelease/releases).

## License

Released under the [MIT License](LICENSE).

## Contact

- **Email**: 416685476@qq.com
- **GitHub Issues**: [zyx3721/zonelease/issues](https://github.com/zyx3721/zonelease/issues)
- **Project home**: [github.com/zyx3721/zonelease](https://github.com/zyx3721/zonelease)

---

**⭐ If this project helps you, a star is appreciated!**
