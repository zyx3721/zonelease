$ErrorActionPreference = "Stop"

$Exe = ""
$LegacySource = $false

for ($i = 0; $i -lt $args.Count; $i++) {
  $arg = [string]$args[$i]
  switch -Regex ($arg) {
    "^-Exe$" {
      if ($i + 1 -ge $args.Count) { throw "-Exe requires a value." }
      $i++
      $Exe = [string]$args[$i]
      continue
    }
    "^-NoPause$" {
      continue
    }
    "^-LegacySource$" {
      $LegacySource = $true
      continue
    }
    default {
      throw "Unknown argument: $arg"
    }
  }
}

function Test-Administrator {
  $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = New-Object Security.Principal.WindowsPrincipal($identity)
  return $principal.IsInRole([Security.Principal.WindowsBuiltinRole]::Administrator)
}

function Test-Blank {
  param([string]$Value)
  if ($null -eq $Value) { return $true }
  return $Value.Trim().Length -eq 0
}

try {
  $scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path

  if (-not (Test-Administrator)) {
    throw "Please run this script as Administrator. DNS Server PowerShell operations require elevated privileges."
  }

  $osVersion = [Environment]::OSVersion.Version
  if ((-not $LegacySource) -and (($osVersion.Major -lt 6) -or (($osVersion.Major -eq 6) -and ($osVersion.Minor -le 1)))) {
    Write-Host "Windows version $($osVersion.ToString()) detected; using legacy PowerShell agent because Go agent is not supported on Windows Server 2008/2008 R2 and older systems."
    $LegacySource = $true
  }

  if ($LegacySource) {
    $legacyScript = Join-Path $scriptRoot "legacy\source-agent.ps1"
    if (-not (Test-Path -LiteralPath $legacyScript)) {
      $legacyScript = Join-Path $scriptRoot "source-agent.ps1"
    }
    if (-not (Test-Path -LiteralPath $legacyScript)) {
      throw "Legacy DNS agent script not found. Expected legacy\source-agent.ps1 or source-agent.ps1 in $scriptRoot."
    }

    $envFile = Join-Path $scriptRoot ".env"
    if (Test-Path -LiteralPath $envFile) {
      Write-Host "Using .env: $envFile"
    }
    else {
      Write-Host ".env not found beside agent.ps1. The legacy agent will use agent.json or built-in defaults." -ForegroundColor Yellow
    }

    Write-Host "Starting ZoneLease DNS Legacy Agent..."
    Write-Host "Script: $legacyScript"
    & $legacyScript
    exit $LASTEXITCODE
  }

  if (Test-Blank $Exe) {
    $candidates = @(
      (Join-Path $scriptRoot "zonelease-dns-agent.exe"),
      (Join-Path $scriptRoot "dns-agent.exe"),
      (Join-Path $scriptRoot "agent.exe")
    )
    foreach ($candidate in $candidates) {
      if (Test-Path -LiteralPath $candidate) {
        $Exe = $candidate
        break
      }
    }
  }
  elseif (-not [System.IO.Path]::IsPathRooted($Exe)) {
    $Exe = Join-Path $scriptRoot $Exe
  }

  if (-not (Test-Path -LiteralPath $Exe)) {
    throw "DNS agent executable not found. Expected zonelease-dns-agent.exe, dns-agent.exe, or agent.exe in $scriptRoot."
  }

  $envFile = Join-Path $scriptRoot ".env"
  if (Test-Path -LiteralPath $envFile) {
    Write-Host "Using .env: $envFile"
  }
  else {
    Write-Host ".env not found beside executable. The agent will use process environment variables or built-in defaults." -ForegroundColor Yellow
  }

  Write-Host "Starting ZoneLease DNS Agent..."
  Write-Host "Executable: $Exe"
  & $Exe
  exit $LASTEXITCODE
}
catch {
  Write-Host "Start DNS agent failed: $($_.Exception.Message)" -ForegroundColor Red
  exit 1
}
