# ZoneLease DHCP Legacy Agent

This directory contains the PowerShell legacy DHCP agent for Windows Server 2008/2008 R2 and older systems where the Go executable cannot run.

## Start

Run `agent.cmd` from the parent `dhcp-agent` directory as Administrator. The parent script auto-detects Windows Server 2008/2008 R2 and older versions, then starts `legacy\source-agent.ps1`.

You can also force legacy mode:

```cmd
agent.cmd -LegacySource
```

## Configuration

The legacy agent first reads `.env` from the parent `dhcp-agent` directory:

```env
DHCP_AGENT_PORT=8462
DHCP_AGENT_API_KEY=change-me
DHCP_AGENT_ALLOW_ANONYMOUS=false
DHCP_AGENT_LOG_PATH=C:\ProgramData\ZoneLease\dhcp-agent-legacy.log
```

If `.env` is missing, it falls back to `legacy\agent.json`, `config\agent.json`, or `agent.json`.

Legacy mode supports HTTP only and requires Administrator privileges, the Windows DHCP Server role, `netsh.exe`, and .NET `System.Web.Extensions` for JSON request body parsing.

DHCP scope detail concurrency is controlled by the backend DHCP scope concurrency setting.

During DHCP synchronization, the backend clears the legacy dump cache after the sync finishes. The cache reuses one global `netsh dhcp server dump` result for reservation, scope range, and exclusion range reads while the sync is running.

The legacy HTTP response writer sends a fixed `ContentLength64` response and does not enable chunked transfer. This avoids intermittent Windows Server 2008 / PowerShell `HttpListenerResponse.SendChunked` failures after response headers have already been submitted.

## Notes

The legacy DHCP agent uses `netsh dhcp server` instead of the `DhcpServer` PowerShell module. It exposes the same HTTP endpoints used by the ZoneLease backend for scope, lease, and reservation operations.
