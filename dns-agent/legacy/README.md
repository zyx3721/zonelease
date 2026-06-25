# ZoneLease DNS Legacy Agent

This directory contains the PowerShell legacy DNS agent for Windows Server 2008/2008 R2 and older systems where the Go executable cannot run.

## Start

Run `agent.cmd` from the parent `dns-agent` directory as Administrator. The parent script auto-detects Windows Server 2008/2008 R2 and older versions, then starts `legacy\source-agent.ps1`.

You can also force legacy mode:

```cmd
agent.cmd -LegacySource
```

## Configuration

The legacy agent first reads `.env` from the parent `dns-agent` directory:

```env
DNS_AGENT_PORT=8460
DNS_AGENT_API_KEY=change-me
DNS_AGENT_ALLOW_ANONYMOUS=false
DNS_AGENT_LOG_PATH=C:\ProgramData\ZoneLease\dns-agent-legacy.log
```

If `.env` is missing, it falls back to `legacy\agent.json`, `config\agent.json`, or `agent.json`.

Legacy mode supports HTTP only and requires Administrator privileges, DNS Server WMI namespace `root\MicrosoftDNS`, and `dnscmd.exe`.

The legacy HTTP response writer sends a fixed `ContentLength64` response and does not enable chunked transfer. This avoids intermittent Windows Server 2008 / PowerShell `HttpListenerResponse.SendChunked` failures after response headers have already been submitted.
