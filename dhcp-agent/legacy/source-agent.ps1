$ErrorActionPreference = "Stop"

# Windows Server 2008/2008 R2 legacy DHCP agent.
# PowerShell 2.0 era HTTP agent. It uses netsh dhcp server instead of the
# DhcpServer PowerShell module, which is unavailable on older systems.

$script:instanceMutex = $null
$script:jsonSerializer = $null
$script:logLock = New-Object System.Object
$script:reservationCacheLock = New-Object System.Object
$script:reservationCache = [hashtable]::Synchronized(@{ loaded = $false; byScope = @{}; rangeByScope = @{}; exclusionsByScope = @{} })

function Enter-SingleInstance {
  param([string]$Name)
  $script:instanceMutex = New-Object System.Threading.Mutex($false, $Name)
  $hasHandle = $script:instanceMutex.WaitOne(0, $false)
  if (-not $hasHandle) {
    throw "Legacy DHCP agent is already running."
  }
}

function Exit-SingleInstance {
  if ($script:instanceMutex -ne $null) {
    try { $script:instanceMutex.ReleaseMutex() | Out-Null } catch {}
    try { $script:instanceMutex.Close() } catch {}
    $script:instanceMutex = $null
  }
}

function Test-BlankString {
  param([object]$Value)
  if ($null -eq $Value) { return $true }
  return ([string]$Value).Trim().Length -eq 0
}

function New-DefaultAgentSettings {
  $settings = New-Object PSObject
  Add-Member -InputObject $settings -MemberType NoteProperty -Name scheme -Value "http"
  Add-Member -InputObject $settings -MemberType NoteProperty -Name port -Value 8462
  Add-Member -InputObject $settings -MemberType NoteProperty -Name apiKey -Value ""
  Add-Member -InputObject $settings -MemberType NoteProperty -Name allowAnonymous -Value $false
  Add-Member -InputObject $settings -MemberType NoteProperty -Name logPath -Value "C:\ProgramData\ZoneLease\dhcp-agent-legacy.log"
  Add-Member -InputObject $settings -MemberType NoteProperty -Name requestConcurrency -Value 5
  return $settings
}

function Get-JsonStringValue {
  param([string]$Raw, [string]$Name, [string]$DefaultValue)
  $pattern = '"' + [Regex]::Escape($Name) + '"\s*:\s*"([^"]*)"'
  $match = [Regex]::Match($Raw, $pattern)
  if ($match.Success) { return $match.Groups[1].Value }
  return $DefaultValue
}

function Get-JsonIntValue {
  param([string]$Raw, [string]$Name, [int]$DefaultValue)
  $pattern = '"' + [Regex]::Escape($Name) + '"\s*:\s*(\d+)'
  $match = [Regex]::Match($Raw, $pattern)
  if ($match.Success) { return [int]$match.Groups[1].Value }
  return $DefaultValue
}

function Get-JsonBoolValue {
  param([string]$Raw, [string]$Name, [bool]$DefaultValue)
  $pattern = '"' + [Regex]::Escape($Name) + '"\s*:\s*(true|false)'
  $match = [Regex]::Match($Raw, $pattern, [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
  if ($match.Success) { return $match.Groups[1].Value.ToLowerInvariant() -eq "true" }
  return $DefaultValue
}

function Read-AgentSettings {
  param([string]$Path)
  if (-not (Test-Path -LiteralPath $Path)) {
    throw "Config file not found: $Path"
  }
  $raw = [System.IO.File]::ReadAllText($Path)
  $settings = New-DefaultAgentSettings
  $settings.scheme = Get-JsonStringValue -Raw $raw -Name "scheme" -DefaultValue $settings.scheme
  $settings.port = Get-JsonIntValue -Raw $raw -Name "port" -DefaultValue $settings.port
  $settings.apiKey = Get-JsonStringValue -Raw $raw -Name "apiKey" -DefaultValue $settings.apiKey
  $settings.allowAnonymous = Get-JsonBoolValue -Raw $raw -Name "allowAnonymous" -DefaultValue $settings.allowAnonymous
  $settings.logPath = Get-JsonStringValue -Raw $raw -Name "logPath" -DefaultValue $settings.logPath
  $settings.requestConcurrency = Get-JsonIntValue -Raw $raw -Name "requestConcurrency" -DefaultValue $settings.requestConcurrency
  $settings.requestConcurrency = ConvertTo-AgentIntRange -Value ([string]$settings.requestConcurrency) -DefaultValue 5 -MinValue 1 -MaxValue 50
  return $settings
}

function ConvertTo-AgentBool {
  param([string]$Value, [bool]$DefaultValue)
  if (Test-BlankString $Value) { return $DefaultValue }
  $normalized = $Value.Trim().ToLowerInvariant()
  return (($normalized -eq "1") -or ($normalized -eq "true") -or ($normalized -eq "yes") -or ($normalized -eq "y"))
}

function ConvertTo-AgentIntRange {
  param([string]$Value, [int]$DefaultValue, [int]$MinValue, [int]$MaxValue)
  if (Test-BlankString $Value) { return $DefaultValue }
  $parsed = 0
  if (-not [int]::TryParse($Value, [ref]$parsed)) { return $DefaultValue }
  if ($parsed -lt $MinValue) { return $MinValue }
  if ($parsed -gt $MaxValue) { return $MaxValue }
  return $parsed
}

function Unquote-DotEnvValue {
  param([string]$Value)
  if ($null -eq $Value) { return "" }
  $text = $Value.Trim()
  if ($text.Length -ge 2) {
    if (($text.StartsWith('"') -and $text.EndsWith('"')) -or ($text.StartsWith("'") -and $text.EndsWith("'"))) {
      return $text.Substring(1, $text.Length - 2)
    }
  }
  return $text
}

function Read-DotEnvAgentSettings {
  param([string]$Path)
  if (-not (Test-Path -LiteralPath $Path)) {
    throw ".env file not found: $Path"
  }

  $settings = New-DefaultAgentSettings
  foreach ($line in [System.IO.File]::ReadAllLines($Path)) {
    $trimmed = $line.Trim()
    if (Test-BlankString $trimmed) { continue }
    if ($trimmed.StartsWith("#")) { continue }
    $eq = $trimmed.IndexOf("=")
    if ($eq -lt 0) { continue }
    $key = $trimmed.Substring(0, $eq).Trim()
    $value = Unquote-DotEnvValue $trimmed.Substring($eq + 1)
    switch ($key) {
      "DHCP_AGENT_PORT" {
        $parsed = 0
        if ([int]::TryParse($value, [ref]$parsed)) { $settings.port = $parsed }
      }
      "DHCP_AGENT_API_KEY" { $settings.apiKey = $value }
      "DHCP_AGENT_ALLOW_ANONYMOUS" { $settings.allowAnonymous = ConvertTo-AgentBool -Value $value -DefaultValue $settings.allowAnonymous }
      "DHCP_AGENT_LOG_PATH" { if (-not (Test-BlankString $value)) { $settings.logPath = $value } }
      "DHCP_AGENT_LEGACY_REQUEST_CONCURRENCY" { $settings.requestConcurrency = ConvertTo-AgentIntRange -Value $value -DefaultValue $settings.requestConcurrency -MinValue 1 -MaxValue 50 }
    }
  }
  return $settings
}

function Write-AgentLog {
  param([string]$LogPath, [string]$Message)
  [System.Threading.Monitor]::Enter($script:logLock)
  try {
    $dir = Split-Path -Path $LogPath -Parent
    if ((-not (Test-BlankString $dir)) -and (-not (Test-Path -LiteralPath $dir))) {
      New-Item -Path $dir -ItemType Directory -Force | Out-Null
    }
    Add-Content -LiteralPath $LogPath -Value "$(Get-Date -Format s) $Message" -Encoding UTF8
  }
  finally {
    [System.Threading.Monitor]::Exit($script:logLock)
  }
}

function Write-DhcpTrace {
  param([string]$Message)
  Write-AgentLog -LogPath $settings.logPath -Message $Message
}

function Escape-Json {
  param([object]$Value)
  if ($null -eq $Value) { return "" }
  $text = [string]$Value
  $text = $text.Replace("\", "\\")
  $text = $text.Replace('"', '\"')
  $text = $text.Replace("`r", "\r")
  $text = $text.Replace("`n", "\n")
  $text = $text.Replace("`t", "\t")
  return $text
}

function Json-String {
  param([object]$Value)
  return '"' + (Escape-Json -Value $Value) + '"'
}

function Json-Bool {
  param([bool]$Value)
  if ($Value) { return "true" }
  return "false"
}

function Read-JsonBody {
  param([System.Net.HttpListenerRequest]$Request)
  if (-not $Request.HasEntityBody) { return $null }

  $reader = New-Object System.IO.StreamReader($Request.InputStream, $Request.ContentEncoding)
  try {
    $raw = $reader.ReadToEnd()
  }
  finally {
    $reader.Close()
  }

  if (Test-BlankString $raw) { return $null }
  if ($script:jsonSerializer -eq $null) {
    try {
      Add-Type -AssemblyName System.Web.Extensions -ErrorAction Stop
      $script:jsonSerializer = New-Object System.Web.Script.Serialization.JavaScriptSerializer
    }
    catch {
      throw "JSON request body parsing requires .NET System.Web.Extensions. Install/enable .NET Framework 3.5/4.x on this server."
    }
  }
  return $script:jsonSerializer.DeserializeObject($raw)
}

function Get-MapValue {
  param([object]$Map, [string]$Name, [object]$DefaultValue)
  if ($null -eq $Map) { return $DefaultValue }
  try {
    if ($Map.ContainsKey($Name) -and $null -ne $Map[$Name]) { return $Map[$Name] }
  }
  catch {}
  try {
    $value = $Map.$Name
    if ($null -ne $value) { return $value }
  }
  catch {}
  return $DefaultValue
}

function UrlDecode {
  param([string]$Value)
  if ($null -eq $Value) { return "" }
  return [System.Uri]::UnescapeDataString($Value.Replace("+", "%20"))
}

function Test-ClientDisconnectedError {
  param([System.Exception]$ErrorObject)
  if ($null -eq $ErrorObject) { return $false }
  try {
    if ($ErrorObject.HResult -eq -2147024832) { return $true }
    if ($ErrorObject.HResult -eq -2147023667) { return $true }
  }
  catch {}
  try {
    if ($ErrorObject.NativeErrorCode -eq 64) { return $true }
    if ($ErrorObject.NativeErrorCode -eq 1236) { return $true }
  }
  catch {}
  $message = [string]$ErrorObject.Message
  if ($message -match "network name is no longer available") { return $true }
  if ($message -match "specified network name") { return $true }
  if ($message -match "connection was aborted") { return $true }
  if ($ErrorObject.InnerException -ne $null) {
    return (Test-ClientDisconnectedError -ErrorObject $ErrorObject.InnerException)
  }
  return $false
}

function Send-Json {
  param(
    [System.Net.HttpListenerResponse]$Response,
    [int]$StatusCode,
    [string]$Json
  )
  try {
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($Json)
    $Response.StatusCode = $StatusCode
    $Response.ContentType = "application/json; charset=utf-8"
    $Response.ContentEncoding = [System.Text.Encoding]::UTF8
    try { $Response.ContentLength64 = $bytes.Length } catch {}
    try {
      $Response.OutputStream.Write($bytes, 0, $bytes.Length)
    }
    catch {
      if (Test-ClientDisconnectedError -ErrorObject $_.Exception) { return }
      throw
    }
  }
  finally {
    try { $Response.OutputStream.Close() } catch {}
  }
}

function Send-Envelope {
  param(
    [System.Net.HttpListenerResponse]$Response,
    [int]$StatusCode,
    [bool]$Success,
    [string]$DataJson,
    [string]$ErrorCode,
    [string]$ErrorMessage,
    [string]$RequestId
  )

  $json = "{"
  $json += (Json-String "success") + ":" + (Json-Bool $Success)
  if ($Success) {
    if (Test-BlankString $DataJson) { $DataJson = "null" }
    $json += "," + (Json-String "data") + ":" + $DataJson
  }
  else {
    $json += "," + (Json-String "error") + ":{" + (Json-String "code") + ":" + (Json-String $ErrorCode) + "," + (Json-String "message") + ":" + (Json-String $ErrorMessage) + "}"
  }
  $json += "," + (Json-String "requestId") + ":" + (Json-String $RequestId)
  $json += "}"
  Send-Json -Response $Response -StatusCode $StatusCode -Json $json
}

function Send-Data {
  param([System.Net.HttpListenerResponse]$Response, [string]$RequestId, [string]$DataJson, [int]$StatusCode = 200)
  Send-Envelope -Response $Response -StatusCode $StatusCode -Success $true -DataJson $DataJson -ErrorCode "" -ErrorMessage "" -RequestId $RequestId
}

function Send-Error {
  param([System.Net.HttpListenerResponse]$Response, [string]$RequestId, [int]$StatusCode, [string]$Code, [string]$Message)
  Send-Envelope -Response $Response -StatusCode $StatusCode -Success $false -DataJson "null" -ErrorCode $Code -ErrorMessage $Message -RequestId $RequestId
}

function New-RequestId {
  return [Guid]::NewGuid().ToString("N")
}

function Get-NextContext {
  param([System.Net.HttpListener]$Listener)
  $async = $Listener.BeginGetContext($null, $null)
  try {
    while (-not $async.AsyncWaitHandle.WaitOne(500, $false)) {
      if (Test-ConsoleStopRequested) {
        return $null
      }
    }
    return $Listener.EndGetContext($async)
  }
  finally {
    try { $async.AsyncWaitHandle.Close() } catch {}
  }
}

function Test-ConsoleStopRequested {
  try {
    if ([Console]::KeyAvailable) {
      $key = [Console]::ReadKey($true)
      if ($key.Key -eq [ConsoleKey]::Q) { return $true }
    }
  }
  catch {}
  return $false
}

function Test-NetshAvailable {
  try {
    $cmd = Get-Command netsh.exe -ErrorAction Stop
    return ($null -ne $cmd)
  }
  catch {
    return $false
  }
}

function Invoke-Netsh {
  param([string[]]$Arguments)
  if (-not (Test-NetshAvailable)) {
    throw "netsh.exe not found. Install Windows DHCP Server tools on Windows Server 2008/2008 R2."
  }
  $oldErrorActionPreference = $ErrorActionPreference
  $ErrorActionPreference = "Continue"
  try {
    $output = & netsh.exe @Arguments 2>&1
    $exitCode = $LASTEXITCODE
  }
  finally {
    $ErrorActionPreference = $oldErrorActionPreference
  }
  $text = [string]::Join("`n", @($output | ForEach-Object { [string]$_ }))
  if ($exitCode -ne 0) {
    if (Test-BlankString $text) { $text = "netsh exited with code $exitCode" }
    throw $text
  }
  return $text
}

function Get-NetshDhcp {
  param([string[]]$Arguments)
  $args = @("dhcp", "server") + $Arguments
  return Invoke-Netsh -Arguments $args
}

function Test-DhcpProbe {
  [void](Get-NetshDhcp -Arguments @("show", "server"))
}

function ConvertTo-Mac {
  param([string]$Value)
  if (Test-BlankString $Value) { return "" }
  return ([string]$Value).Trim().Replace("-", "").Replace(":", "").Replace(".", "")
}

function Normalize-State {
  param([string]$Value)
  if (Test-BlankString $Value) { return "Active" }
  $text = $Value.Trim().ToLowerInvariant()
  if (($text -match "inactive") -or ($text -match "disabled") -or ($text -match "off")) { return "Inactive" }
  return "Active"
}

function Convert-DateToIso {
  param([string]$Value)
  if (Test-BlankString $Value) { return "" }
  try {
    return ([DateTime]::Parse($Value)).ToUniversalTime().ToString("o")
  }
  catch {
    return [string]$Value
  }
}

function Test-DhcpLeaseTypeText {
  param([string]$Value)
  if (Test-BlankString $Value) { return $false }
  $text = $Value.Trim().ToLowerInvariant()
  $normalized = $text.Trim("-")
  return (($normalized -eq "dhcp") -or ($normalized -eq "bootp") -or ($normalized -eq "both") -or ($normalized -eq "dhcp/bootp") -or ($normalized -eq "d") -or ($normalized -eq "u"))
}

function Test-DateLikeText {
  param([string]$Value)
  if (Test-BlankString $Value) { return $false }
  $parsed = [DateTime]::MinValue
  return [DateTime]::TryParse($Value.Trim(), [ref]$parsed)
}

function Test-NeverExpiresText {
  param([string]$Value)
  if (Test-BlankString $Value) { return $false }
  $text = $Value.Trim().ToLowerInvariant()
  return (($text -match "永不过期") -or ($text -match "never") -or ($text -match "infinite") -or ($text -match "no expiration"))
}

function Get-QuotedValues {
  param([string]$Text)
  $items = @()
  foreach ($match in [Regex]::Matches([string]$Text, '"([^"]*)"')) {
    $items += $match.Groups[1].Value
  }
  return $items
}

function Read-DhcpLeaseTail {
  param([string]$Tail)
  $result = @{ ExpiresAt = ""; Type = ""; Name = "" }
  if (Test-BlankString $Tail) { return $result }
  $text = $Tail.Trim()
  if ($text.StartsWith("-")) { $text = $text.Substring(1).Trim() }

  $marker = [Regex]::Match($text, '(^|\s)(-[A-Za-z]-)\s*')
  if ($marker.Success) {
    $prefix = $text.Substring(0, $marker.Index).Trim()
    if (Test-DateLikeText $prefix) { $result.ExpiresAt = Convert-DateToIso $prefix }
    $result.Type = $marker.Groups[2].Value.Trim("-")
    if (Test-NeverExpiresText $prefix) { $result.Type = "ReservedActive" }
    $result.Name = $text.Substring($marker.Index + $marker.Length).Trim()
    return $result
  }

  $parts = $text -split '\s+-\s+|\s{2,}'
  foreach ($part in $parts) {
    $value = $part.Trim()
    if (Test-BlankString $value) { continue }
    if ((Test-BlankString $result.ExpiresAt) -and (Test-DateLikeText $value)) {
      $result.ExpiresAt = Convert-DateToIso $value
      continue
    }
    if (Test-NeverExpiresText $value) {
      $result.Type = "ReservedActive"
      continue
    }
    if ((Test-BlankString $result.Type) -and (Test-DhcpLeaseTypeText $value)) {
      $result.Type = $value.Trim("-")
      continue
    }
  }
  return $result
}

function Get-DhcpReservationScopeFromLine {
  param([string]$Text, [string]$DefaultScopeId)
  $scopeMatch = [Regex]::Match($Text, '(?i)\bscope\s+(\d{1,3}(?:\.\d{1,3}){3})\b')
  if ($scopeMatch.Success) { return $scopeMatch.Groups[1].Value }
  return $DefaultScopeId
}

function Get-DhcpReservationsFromDumpText {
  param([string]$Dump, [string]$DefaultScopeId)
  $items = @()
  foreach ($line in $Dump -split "`n") {
    $text = $line.Trim()
    if (Test-BlankString $text) { continue }
    if ($text.ToLowerInvariant().IndexOf("reservedip") -lt 0) { continue }
    $scopeId = Get-DhcpReservationScopeFromLine -Text $text -DefaultScopeId $DefaultScopeId
    if (Test-BlankString $scopeId) { continue }
    $reservationMatch = [Regex]::Match($text, '(?i)\breservedip\s+(\d{1,3}(?:\.\d{1,3}){3})\s+([0-9a-fA-F:-]{12,17})\b')
    if (-not $reservationMatch.Success) { continue }
    $ip = $reservationMatch.Groups[1].Value
    $mac = ConvertTo-Mac $reservationMatch.Groups[2].Value
    $quotes = @(Get-QuotedValues -Text $text)
    $name = ""
    $description = ""
    if ($quotes.Length -ge 1) { $name = $quotes[0] }
    if ($quotes.Length -ge 2) { $description = $quotes[1] }
    $items += @{
      id = ($scopeId + "|" + $ip)
      scopeId = $scopeId
      ip = $ip
      mac = $mac
      name = $name
      description = $description
    }
  }
  return $items
}

function Get-DhcpScopeRangesFromDumpText {
  param([string]$Dump, [string]$DefaultScopeId)
  $ranges = @{}
  $currentScopeId = $DefaultScopeId
  foreach ($line in $Dump -split "`n") {
    $text = $line.Trim()
    if (Test-BlankString $text) { continue }
    $lineScopeId = Get-DhcpReservationScopeFromLine -Text $text -DefaultScopeId ""
    if (-not (Test-BlankString $lineScopeId)) { $currentScopeId = $lineScopeId }
    if (-not [Regex]::IsMatch($text, '(?i)\badd\s+iprange\b')) { continue }
    $scopeId = Get-DhcpReservationScopeFromLine -Text $text -DefaultScopeId $currentScopeId
    if (Test-BlankString $scopeId) { continue }
    $match = [Regex]::Match($text, '(?i)\badd\s+iprange\s+"?(\d{1,3}(?:\.\d{1,3}){3})"?\s+"?(\d{1,3}(?:\.\d{1,3}){3})"?\b')
    if (-not $match.Success) { continue }
    $ranges[$scopeId] = @{ Start = $match.Groups[1].Value; End = $match.Groups[2].Value }
  }
  return $ranges
}

function Get-DhcpExclusionsFromDumpText {
  param([string]$Dump, [string]$DefaultScopeId)
  $items = @()
  $currentScopeId = $DefaultScopeId
  foreach ($line in $Dump -split "`n") {
    $text = $line.Trim()
    if (Test-BlankString $text) { continue }
    $lineScopeId = Get-DhcpReservationScopeFromLine -Text $text -DefaultScopeId ""
    if (-not (Test-BlankString $lineScopeId)) { $currentScopeId = $lineScopeId }
    if (-not [Regex]::IsMatch($text, '(?i)\bexcluderange\b')) { continue }
    $scopeId = Get-DhcpReservationScopeFromLine -Text $text -DefaultScopeId $currentScopeId
    if (Test-BlankString $scopeId) { continue }
    $match = [Regex]::Match($text, '(?i)\bexcluderange\s+"?(\d{1,3}(?:\.\d{1,3}){3})"?\s+"?(\d{1,3}(?:\.\d{1,3}){3})"?\b')
    if (-not $match.Success) { continue }
    $startIp = $match.Groups[1].Value
    $endIp = $match.Groups[2].Value
    $items += @{
      id = ($scopeId + "|" + $startIp + "|" + $endIp)
      scopeId = $scopeId
      startIp = $startIp
      endIp = $endIp
    }
  }
  return $items
}

function Group-DhcpReservationsByScope {
  param([object[]]$Reservations)
  $byScope = @{}
  foreach ($reservation in $Reservations) {
    $scopeId = [string]$reservation.scopeId
    if (Test-BlankString $scopeId) { continue }
    if (-not $byScope.ContainsKey($scopeId)) { $byScope[$scopeId] = @() }
    $byScope[$scopeId] = @($byScope[$scopeId]) + $reservation
  }
  return $byScope
}

function Group-DhcpExclusionsByScope {
  param([object[]]$Exclusions)
  $byScope = @{}
  foreach ($exclusion in $Exclusions) {
    $scopeId = [string]$exclusion.scopeId
    if (Test-BlankString $scopeId) { continue }
    if (-not $byScope.ContainsKey($scopeId)) { $byScope[$scopeId] = @() }
    $byScope[$scopeId] = @($byScope[$scopeId]) + $exclusion
  }
  return $byScope
}

function Clear-DhcpReservationCache {
  [System.Threading.Monitor]::Enter($script:reservationCacheLock)
  try {
    $script:reservationCache["loaded"] = $false
    $script:reservationCache["byScope"] = @{}
    $script:reservationCache["rangeByScope"] = @{}
    $script:reservationCache["exclusionsByScope"] = @{}
  }
  finally {
    [System.Threading.Monitor]::Exit($script:reservationCacheLock)
  }
}

function Get-DhcpReservationCacheByScope {
  [System.Threading.Monitor]::Enter($script:reservationCacheLock)
  try {
    if ($script:reservationCache["loaded"]) {
      return $script:reservationCache["byScope"]
    }
    $dump = Get-NetshDhcp -Arguments @("dump")
    $byScope = Group-DhcpReservationsByScope -Reservations @(Get-DhcpReservationsFromDumpText -Dump $dump -DefaultScopeId "")
    $rangeByScope = Get-DhcpScopeRangesFromDumpText -Dump $dump -DefaultScopeId ""
    $exclusionsByScope = Group-DhcpExclusionsByScope -Exclusions @(Get-DhcpExclusionsFromDumpText -Dump $dump -DefaultScopeId "")
    $script:reservationCache["byScope"] = $byScope
    $script:reservationCache["rangeByScope"] = $rangeByScope
    $script:reservationCache["exclusionsByScope"] = $exclusionsByScope
    $script:reservationCache["loaded"] = $true
    return $byScope
  }
  finally {
    [System.Threading.Monitor]::Exit($script:reservationCacheLock)
  }
}

function Get-DhcpLeaseDurationsFromDumpText {
  param([string]$Dump)
  $durations = @{}
  foreach ($line in $Dump -split "`n") {
    $text = $line.Trim()
    if (Test-BlankString $text) { continue }
    if ($text.ToLowerInvariant().IndexOf("optionvalue 51") -lt 0) { continue }
    $match = [Regex]::Match($text, '(?i)\bscope\s+(\d{1,3}(?:\.\d{1,3}){3})\b.*\boptionvalue\s+51\s+DWORD\s+"?(-?\d+)"?')
    if (-not $match.Success) { continue }
    $seconds = 0
    if (-not [int]::TryParse($match.Groups[2].Value, [ref]$seconds)) { continue }
    if ($seconds -lt -1) { continue }
    $durations[$match.Groups[1].Value] = $seconds
  }
  return $durations
}

function Get-DhcpLeaseDurationMap {
  try {
    $dump = Get-NetshDhcp -Arguments @("dump")
    $byScope = Group-DhcpReservationsByScope -Reservations @(Get-DhcpReservationsFromDumpText -Dump $dump -DefaultScopeId "")
    $rangeByScope = Get-DhcpScopeRangesFromDumpText -Dump $dump -DefaultScopeId ""
    $exclusionsByScope = Group-DhcpExclusionsByScope -Exclusions @(Get-DhcpExclusionsFromDumpText -Dump $dump -DefaultScopeId "")
    [System.Threading.Monitor]::Enter($script:reservationCacheLock)
    try {
      $script:reservationCache["byScope"] = $byScope
      $script:reservationCache["rangeByScope"] = $rangeByScope
      $script:reservationCache["exclusionsByScope"] = $exclusionsByScope
      $script:reservationCache["loaded"] = $true
    }
    finally {
      [System.Threading.Monitor]::Exit($script:reservationCacheLock)
    }
    return Get-DhcpLeaseDurationsFromDumpText -Dump $dump
  }
  catch {
    return @{}
  }
}

function Convert-MaskToPrefix {
  param([string]$Mask)
  $prefix = 0
  foreach ($part in $Mask.Split(".")) {
    $number = 0
    if (-not [int]::TryParse($part, [ref]$number)) { return 24 }
    while ($number -gt 0) {
      $prefix += ($number -band 1)
      $number = [Math]::Floor($number / 2)
    }
  }
  return $prefix
}

function Convert-PrefixToMask {
  param([int]$Prefix)
  if ($Prefix -lt 0 -or $Prefix -gt 32) { throw "CIDR prefix must be between 0 and 32" }
  $octets = @()
  for ($i = 0; $i -lt 4; $i++) {
    $remaining = $Prefix - ($i * 8)
    if ($remaining -ge 8) {
      $octets += 255
    }
    elseif ($remaining -le 0) {
      $octets += 0
    }
    else {
      $octets += (256 - [Math]::Pow(2, 8 - $remaining))
    }
  }
  return [string]::Join(".", $octets)
}

function Parse-Subnet {
  param([string]$Subnet)
  if (Test-BlankString $Subnet) { throw "scope subnet is required" }
  $text = $Subnet.Trim()
  if ($text.Contains("/")) {
    $parts = $text.Split("/")
    if ($parts.Length -ne 2) { throw "scope subnet must be IPv4 CIDR" }
    return @{ ScopeId = $parts[0]; Mask = Convert-PrefixToMask -Prefix ([int]$parts[1]) }
  }
  return @{ ScopeId = $text; Mask = "255.255.255.0" }
}

function Get-ScopeOptionValue {
  param([string]$Details, [string[]]$Names)
  foreach ($name in $Names) {
    $pattern = [Regex]::Escape($name) + "\s*[:=]\s*(.+)"
    $match = [Regex]::Match($Details, $pattern, [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
    if ($match.Success) { return $match.Groups[1].Value.Trim() }
  }
  return ""
}

function Test-IPv4Text {
  param([string]$Value)
  if (Test-BlankString $Value) { return $false }
  $text = $Value.Trim()
  return [Regex]::IsMatch($text, '^\d{1,3}(?:\.\d{1,3}){3}$')
}

function Get-FirstIPv4Matches {
  param([string]$Text)
  $textValue = ""
  if ($null -ne $Text) { $textValue = [string]$Text }
  return [Regex]::Matches($textValue, '\b\d{1,3}(?:\.\d{1,3}){3}\b')
}

function Get-FirstMacMatch {
  param([string]$Text)
  $textValue = ""
  if ($null -ne $Text) { $textValue = [string]$Text }
  return [Regex]::Match($textValue, '\b(?:[0-9A-Fa-f]{2}[-:]){5}[0-9A-Fa-f]{2}\b|\b[0-9A-Fa-f]{12}\b')
}

function Test-DhcpStateText {
  param([string]$Value)
  if (Test-BlankString $Value) { return $false }
  $text = $Value.Trim().ToLowerInvariant()
  return ($text -eq "active" -or $text -eq "inactive" -or $text -eq "enabled" -or $text -eq "disabled")
}

function Parse-DhcpScopeLine {
  param([string]$Text)
  if (Test-BlankString $Text) { return $null }

  $columns = $Text -split '\s+-\s*'
  if ($columns.Length -ge 3 -and (Test-IPv4Text $columns[0]) -and (Test-IPv4Text $columns[1])) {
    $scopeId = $columns[0].Trim()
    $mask = $columns[1].Trim()
    $state = "Active"
    $name = $scopeId
    if ($columns.Length -ge 4) {
      $state = Normalize-State $columns[2]
      $name = $columns[3].Trim()
    }
    else {
      $name = $columns[2].Trim()
      if (Test-DhcpStateText $name) {
        $state = Normalize-State $name
        $name = $scopeId
      }
    }
    $description = ""
    if ($columns.Length -ge 5) { $description = $columns[4].Trim() }
    if (Test-BlankString $name) { $name = $scopeId }
    return @{ ScopeId = $scopeId; Mask = $mask; Name = $name; Description = $description; State = $state }
  }

  $match = [Regex]::Match($Text, '^\s*(\d{1,3}(?:\.\d{1,3}){3})\s+(\d{1,3}(?:\.\d{1,3}){3})\s+(\S+)\s*(.*)$')
  if ($match.Success) {
    $scopeId = $match.Groups[1].Value
    $mask = $match.Groups[2].Value
    $state = Normalize-State $match.Groups[3].Value
    $name = $match.Groups[4].Value.Trim()
    if (Test-BlankString $name) { $name = $scopeId }
    return @{ ScopeId = $scopeId; Mask = $mask; Name = $name; State = $state }
  }

  return $null
}

function Get-DhcpScopeRange {
  param([string]$ScopeId)
  $range = @{ Start = ""; End = "" }
  try {
    $raw = Get-NetshDhcp -Arguments @("scope", $ScopeId, "show", "iprange")
    foreach ($line in $raw -split "`n") {
      $text = $line.Trim()
      if ($text.ToLowerInvariant().IndexOf("iprange") -lt 0) { continue }
      $match = [Regex]::Match($text, '(?i)\biprange\s+(\d{1,3}(?:\.\d{1,3}){3})\s+(\d{1,3}(?:\.\d{1,3}){3})\b')
      if ($match.Success) {
        $range.Start = $match.Groups[1].Value
        $range.End = $match.Groups[2].Value
        return $range
      }
    }
  }
  catch {}
  return $range
}

function Get-DhcpScopes {
  return Get-DhcpScopesInternal -IncludeDetails $true
}

function Get-DhcpScopesLight {
  return Get-DhcpScopesInternal -IncludeDetails $false
}

function Get-DhcpScopesInternal {
  param([bool]$IncludeDetails)
  Write-DhcpTrace -Message ("dhcp scopes start includeDetails=" + $IncludeDetails)
  $raw = Get-NetshDhcp -Arguments @("show", "scope")
  $leaseDurations = Get-DhcpLeaseDurationMap
  $rangeByScope = @{}
  [System.Threading.Monitor]::Enter($script:reservationCacheLock)
  try {
    if ($script:reservationCache.ContainsKey("rangeByScope")) {
      $rangeByScope = $script:reservationCache["rangeByScope"]
    }
  }
  finally {
    [System.Threading.Monitor]::Exit($script:reservationCacheLock)
  }
  $items = @()
  foreach ($line in $raw -split "`n") {
    $text = $line.Trim()
    if (Test-BlankString $text) { continue }
    $parsed = Parse-DhcpScopeLine -Text $text
    if ($null -eq $parsed) { continue }
    $scopeId = $parsed.ScopeId
    $mask = $parsed.Mask
    $name = $parsed.Name
    $description = $parsed.Description
    $state = $parsed.State

    $startRange = ""
    $endRange = ""
    if ($rangeByScope -ne $null -and $rangeByScope.ContainsKey($scopeId)) {
      $startRange = [string]$rangeByScope[$scopeId].Start
      $endRange = [string]$rangeByScope[$scopeId].End
    }
    $leaseSeconds = 86400
    if ($leaseDurations.ContainsKey($scopeId)) {
      $leaseSeconds = [int]$leaseDurations[$scopeId]
    }
    $leaseHours = 0
    if ($leaseSeconds -gt 0) {
      $leaseHours = [int][Math]::Ceiling($leaseSeconds / 3600)
      if ($leaseHours -lt 1) { $leaseHours = 1 }
    }
    if ($IncludeDetails) {
      try {
        $details = Get-NetshDhcp -Arguments @("scope", $scopeId, "show", "scope")
        $startRange = Get-ScopeOptionValue -Details $details -Names @("Start IP Address", "Start Range", "Start")
        $endRange = Get-ScopeOptionValue -Details $details -Names @("End IP Address", "End Range", "End")
        if ((Test-BlankString $startRange) -or (Test-BlankString $endRange)) {
          $range = Get-DhcpScopeRange -ScopeId $scopeId
          if (Test-BlankString $startRange) { $startRange = $range.Start }
          if (Test-BlankString $endRange) { $endRange = $range.End }
        }
      }
      catch {}
    }

    $items += @{
      id = $scopeId
      name = $name
      description = $description
      subnet = ($scopeId + "/" + (Convert-MaskToPrefix -Mask $mask))
      startRange = $startRange
      endRange = $endRange
      leaseDurationHours = $leaseHours
      leaseDurationSeconds = $leaseSeconds
      state = $state
      serverId = "local"
    }
  }
  Write-DhcpTrace -Message ("dhcp scopes done includeDetails=" + $IncludeDetails + " count=" + $items.Count)
  return $items
}

function Json-Scope {
  param([object]$Scope)
  return "{" +
    (Json-String "id") + ":" + (Json-String $Scope.id) + "," +
    (Json-String "name") + ":" + (Json-String $Scope.name) + "," +
    (Json-String "description") + ":" + (Json-String $Scope.description) + "," +
    (Json-String "subnet") + ":" + (Json-String $Scope.subnet) + "," +
    (Json-String "startRange") + ":" + (Json-String $Scope.startRange) + "," +
    (Json-String "endRange") + ":" + (Json-String $Scope.endRange) + "," +
    (Json-String "leaseDurationHours") + ":" + ([int]$Scope.leaseDurationHours) + "," +
    (Json-String "leaseDurationSeconds") + ":" + ([int]$Scope.leaseDurationSeconds) + "," +
    (Json-String "state") + ":" + (Json-String $Scope.state) + "," +
    (Json-String "serverId") + ":" + (Json-String $Scope.serverId) +
    "}"
}

function Json-SingleScope {
  param([object]$Scope)
  return Json-Scope -Scope $Scope
}

function Json-Scopes {
  param([object[]]$Scopes)
  $parts = @()
  foreach ($scope in $Scopes) { $parts += (Json-Scope -Scope $scope) }
  return "[" + [string]::Join(",", $parts) + "]"
}

function Create-DhcpScope {
  param([object]$Body)
  $name = [string](Get-MapValue -Map $Body -Name "name" -DefaultValue "")
  $subnet = [string](Get-MapValue -Map $Body -Name "subnet" -DefaultValue "")
  $startRange = [string](Get-MapValue -Map $Body -Name "startRange" -DefaultValue "")
  $endRange = [string](Get-MapValue -Map $Body -Name "endRange" -DefaultValue "")
  $leaseHours = [int](Get-MapValue -Map $Body -Name "leaseDurationHours" -DefaultValue 24)
  $leaseSeconds = [int](Get-MapValue -Map $Body -Name "leaseDurationSeconds" -DefaultValue 0)
  if (Test-BlankString $name) { throw "scope name is required" }
  if (Test-BlankString $startRange) { throw "scope start range is required" }
  if (Test-BlankString $endRange) { throw "scope end range is required" }
  if ($leaseSeconds -ne -1) {
    if ($leaseSeconds -le 0) {
      if ($leaseHours -le 0) { $leaseHours = 24 }
      $leaseSeconds = $leaseHours * 3600
    }
  }
  $parsed = Parse-Subnet -Subnet $subnet
  [void](Get-NetshDhcp -Arguments @("add", "scope", $parsed.ScopeId, $parsed.Mask, $name))
  [void](Get-NetshDhcp -Arguments @("scope", $parsed.ScopeId, "add", "iprange", $startRange, $endRange))
  try { [void](Get-NetshDhcp -Arguments @("scope", $parsed.ScopeId, "set", "state", "1")) } catch {}
  try { [void](Get-NetshDhcp -Arguments @("scope", $parsed.ScopeId, "set", "optionvalue", "51", "DWORD", ([string]$leaseSeconds))) } catch {}
}

function Update-DhcpScope {
  param([string]$ScopeId, [object]$Body)
  $name = [string](Get-MapValue -Map $Body -Name "name" -DefaultValue "")
  $leaseHours = [int](Get-MapValue -Map $Body -Name "leaseDurationHours" -DefaultValue 24)
  $leaseSeconds = [int](Get-MapValue -Map $Body -Name "leaseDurationSeconds" -DefaultValue 0)
  $state = [string](Get-MapValue -Map $Body -Name "state" -DefaultValue "Active")
  $startRange = [string](Get-MapValue -Map $Body -Name "startRange" -DefaultValue "")
  $endRange = [string](Get-MapValue -Map $Body -Name "endRange" -DefaultValue "")
  if (Test-BlankString $ScopeId) { throw "scope id is required" }
  if (Test-BlankString $name) { throw "scope name is required" }
  if ($leaseSeconds -ne -1) {
    if ($leaseSeconds -le 0) {
      if ($leaseHours -le 0) { $leaseHours = 24 }
      $leaseSeconds = $leaseHours * 3600
    }
  }
  try { [void](Get-NetshDhcp -Arguments @("scope", $ScopeId, "set", "name", $name)) } catch {}
  try { [void](Get-NetshDhcp -Arguments @("scope", $ScopeId, "set", "optionvalue", "51", "DWORD", ([string]$leaseSeconds))) } catch {}
  if ((-not (Test-BlankString $startRange)) -and (-not (Test-BlankString $endRange))) {
    $oldRange = Get-DhcpScopeRange -ScopeId $ScopeId
    if ($oldRange.Start -ne $startRange -or $oldRange.End -ne $endRange) {
      [void](Get-NetshDhcp -Arguments @("scope", $ScopeId, "add", "iprange", $startRange, $endRange))
      if ((-not (Test-BlankString $oldRange.Start)) -and (-not (Test-BlankString $oldRange.End))) {
        try { [void](Get-NetshDhcp -Arguments @("scope", $ScopeId, "delete", "iprange", $oldRange.Start, $oldRange.End)) } catch {}
      }
    }
  }
  Set-DhcpScopeState -ScopeId $ScopeId -Active ($state -eq "Active")
  $result = @{
    id = $ScopeId
    name = $name
    subnet = [string](Get-MapValue -Map $Body -Name "subnet" -DefaultValue $ScopeId)
    startRange = $startRange
    endRange = $endRange
    leaseDurationHours = $leaseHours
    leaseDurationSeconds = $leaseSeconds
    state = $state
    serverId = "local"
  }
  return $result
}

function Set-DhcpScopeState {
  param([string]$ScopeId, [bool]$Active)
  $state = "0"
  if ($Active) { $state = "1" }
  [void](Get-NetshDhcp -Arguments @("scope", $ScopeId, "set", "state", $state))
}

function Delete-DhcpScope {
  param([string]$ScopeId)
  [void](Get-NetshDhcp -Arguments @("delete", "scope", $ScopeId, "dhcpfullforce"))
}

function Create-DhcpExclusion {
  param([object]$Body)
  $scopeId = [string](Get-MapValue -Map $Body -Name "scopeId" -DefaultValue "")
  $startIp = [string](Get-MapValue -Map $Body -Name "startIp" -DefaultValue "")
  $endIp = [string](Get-MapValue -Map $Body -Name "endIp" -DefaultValue "")
  if (Test-BlankString $scopeId) { throw "scope id is required" }
  if (Test-BlankString $startIp) { throw "exclusion start ip is required" }
  if (Test-BlankString $endIp) { throw "exclusion end ip is required" }
  [void](Get-NetshDhcp -Arguments @("scope", $scopeId, "add", "excluderange", $startIp, $endIp))
  return @{
    id = ($scopeId + "|" + $startIp + "|" + $endIp)
    scopeId = $scopeId
    startIp = $startIp
    endIp = $endIp
  }
}

function Delete-DhcpExclusion {
  param([string]$ScopeId, [string]$StartIP, [string]$EndIP)
  if (Test-BlankString $ScopeId) { throw "scope id is required" }
  if (Test-BlankString $StartIP) { throw "exclusion start ip is required" }
  if (Test-BlankString $EndIP) { throw "exclusion end ip is required" }
  [void](Get-NetshDhcp -Arguments @("scope", $ScopeId, "delete", "excluderange", $StartIP, $EndIP))
}

function Get-DhcpLeases {
  param([string]$ScopeId)
  Write-DhcpTrace -Message ("dhcp leases start scope=" + $ScopeId)
  $raw = Get-NetshDhcp -Arguments @("scope", $ScopeId, "show", "clients", "1")
  $items = @()
  $lines = @($raw -split "`n")
  for ($index = 0; $index -lt $lines.Length; $index++) {
    $line = $lines[$index]
    $text = $line.Trim()
    if (Test-BlankString $text) { continue }
    $ipMatch = [Regex]::Match($text, '\b\d{1,3}(?:\.\d{1,3}){3}\b')
    if (-not $ipMatch.Success) { continue }
    $macMatch = Get-FirstMacMatch -Text $text
    if (-not $macMatch.Success) { continue }
    $ip = $ipMatch.Value
    $mac = ConvertTo-Mac $macMatch.Value
    $parsedLease = Read-DhcpLeaseTail -Tail $text.Substring($macMatch.Index + $macMatch.Length)
    $name = $parsedLease.Name
    $items += @{
      id = ($ScopeId + "|" + $ip)
      scopeId = $ScopeId
      ip = $ip
      mac = $mac
      hostname = $name
      state = $parsedLease.Type
      expiresAt = $parsedLease.ExpiresAt
    }
  }
  Write-DhcpTrace -Message ("dhcp leases done scope=" + $ScopeId + " count=" + $items.Count)
  return $items
}

function Json-Leases {
  param([object[]]$Leases)
  $parts = @()
  foreach ($lease in $Leases) {
    $parts += "{" +
      (Json-String "id") + ":" + (Json-String $lease.id) + "," +
      (Json-String "scopeId") + ":" + (Json-String $lease.scopeId) + "," +
      (Json-String "ip") + ":" + (Json-String $lease.ip) + "," +
      (Json-String "mac") + ":" + (Json-String $lease.mac) + "," +
      (Json-String "hostname") + ":" + (Json-String $lease.hostname) + "," +
      (Json-String "state") + ":" + (Json-String $lease.state) + "," +
      (Json-String "expiresAt") + ":" + (Json-String $lease.expiresAt) +
      "}"
  }
  return "[" + [string]::Join(",", $parts) + "]"
}

function Release-DhcpLease {
  param([string]$ScopeId, [string]$IP)
  [void](Get-NetshDhcp -Arguments @("scope", $ScopeId, "delete", "lease", $IP))
}

function Get-DhcpReservations {
  param([string]$ScopeId)
  Write-DhcpTrace -Message ("dhcp reservations start scope=" + $ScopeId)
  $byScope = Get-DhcpReservationCacheByScope
  if ($byScope -ne $null -and $byScope.ContainsKey($ScopeId)) {
    $items = @($byScope[$ScopeId])
  }
  else {
    $items = @()
  }
  Write-DhcpTrace -Message ("dhcp reservations done scope=" + $ScopeId + " count=" + $items.Count)
  return $items
}

function Get-DhcpExclusions {
  param([string]$ScopeId)
  Write-DhcpTrace -Message ("dhcp exclusions start scope=" + $ScopeId)
  $items = @()
  [void](Get-DhcpReservationCacheByScope)
  [System.Threading.Monitor]::Enter($script:reservationCacheLock)
  try {
    if ($script:reservationCache.ContainsKey("exclusionsByScope")) {
      $byScope = $script:reservationCache["exclusionsByScope"]
      if ($byScope -ne $null -and $byScope.ContainsKey($ScopeId)) {
        $items = @($byScope[$ScopeId])
      }
    }
  }
  finally {
    [System.Threading.Monitor]::Exit($script:reservationCacheLock)
  }
  Write-DhcpTrace -Message ("dhcp exclusions done scope=" + $ScopeId + " count=" + $items.Count)
  return $items
}

function Json-Exclusions {
  param([object[]]$Exclusions)
  $parts = @()
  foreach ($exclusion in $Exclusions) {
    $parts += "{" +
      (Json-String "id") + ":" + (Json-String $exclusion.id) + "," +
      (Json-String "scopeId") + ":" + (Json-String $exclusion.scopeId) + "," +
      (Json-String "startIp") + ":" + (Json-String $exclusion.startIp) + "," +
      (Json-String "endIp") + ":" + (Json-String $exclusion.endIp) +
      "}"
  }
  return "[" + [string]::Join(",", $parts) + "]"
}

function Json-Exclusion {
  param([object]$Exclusion)
  return "{" +
    (Json-String "id") + ":" + (Json-String $Exclusion.id) + "," +
    (Json-String "scopeId") + ":" + (Json-String $Exclusion.scopeId) + "," +
    (Json-String "startIp") + ":" + (Json-String $Exclusion.startIp) + "," +
    (Json-String "endIp") + ":" + (Json-String $Exclusion.endIp) +
    "}"
}

function Json-Reservations {
  param([object[]]$Reservations)
  $parts = @()
  foreach ($reservation in $Reservations) {
    $parts += "{" +
      (Json-String "id") + ":" + (Json-String $reservation.id) + "," +
      (Json-String "scopeId") + ":" + (Json-String $reservation.scopeId) + "," +
      (Json-String "ip") + ":" + (Json-String $reservation.ip) + "," +
      (Json-String "mac") + ":" + (Json-String $reservation.mac) + "," +
      (Json-String "name") + ":" + (Json-String $reservation.name) + "," +
      (Json-String "description") + ":" + (Json-String $reservation.description) +
      "}"
  }
  return "[" + [string]::Join(",", $parts) + "]"
}

function Json-Reservation {
  param([object]$Reservation)
  return "{" +
    (Json-String "id") + ":" + (Json-String $Reservation.id) + "," +
    (Json-String "scopeId") + ":" + (Json-String $Reservation.scopeId) + "," +
    (Json-String "ip") + ":" + (Json-String $Reservation.ip) + "," +
    (Json-String "mac") + ":" + (Json-String $Reservation.mac) + "," +
    (Json-String "name") + ":" + (Json-String $Reservation.name) + "," +
    (Json-String "description") + ":" + (Json-String $Reservation.description) +
    "}"
}

function Invoke-DhcpScopeDetailTask {
  param([string]$Kind, [string]$ScopeId)
  if ($Kind -eq "leases") {
    return @{ Kind = "leases"; Json = (Json-Leases -Leases @(Get-DhcpLeases -ScopeId $ScopeId)) }
  }
  if ($Kind -eq "exclusions") {
    return @{ Kind = "exclusions"; Json = (Json-Exclusions -Exclusions @(Get-DhcpExclusions -ScopeId $ScopeId)) }
  }
  if ($Kind -eq "reservations") {
    return @{ Kind = "reservations"; Json = (Json-Reservations -Reservations @(Get-DhcpReservations -ScopeId $ScopeId)) }
  }
  throw "unsupported scope detail task"
}

function Get-ScopeDetailFunctionNames {
  return @(
    "Test-BlankString",
    "Write-AgentLog",
    "Write-DhcpTrace",
    "Escape-Json",
    "Json-String",
    "Json-Bool",
    "Test-NetshAvailable",
    "Invoke-Netsh",
    "Get-NetshDhcp",
    "Test-DhcpProbe",
    "ConvertTo-Mac",
    "Convert-DateToIso",
    "Test-DhcpLeaseTypeText",
    "Test-DateLikeText",
    "Test-NeverExpiresText",
    "Get-QuotedValues",
    "Read-DhcpLeaseTail",
    "Get-DhcpReservationScopeFromLine",
    "Get-DhcpReservationsFromDumpText",
    "Get-DhcpScopeRangesFromDumpText",
    "Get-DhcpExclusionsFromDumpText",
    "Group-DhcpReservationsByScope",
    "Group-DhcpExclusionsByScope",
    "Clear-DhcpReservationCache",
    "Get-DhcpReservationCacheByScope",
    "Test-IPv4Text",
    "Get-FirstMacMatch",
    "Get-DhcpLeases",
    "Json-Leases",
    "Get-DhcpReservations",
    "Get-DhcpExclusions",
    "Json-Exclusions",
    "Json-Reservations",
    "Invoke-DhcpScopeDetailTask"
  )
}

function New-DhcpScopeDetailRunspacePool {
  $initialState = [System.Management.Automation.Runspaces.InitialSessionState]::CreateDefault()
  foreach ($name in (Get-ScopeDetailFunctionNames)) {
    $command = Get-Command $name -CommandType Function -ErrorAction Stop
    $entry = New-Object System.Management.Automation.Runspaces.SessionStateFunctionEntry -ArgumentList $name, $command.Definition
    $initialState.Commands.Add($entry)
  }
  $initialState.Variables.Add((New-Object System.Management.Automation.Runspaces.SessionStateVariableEntry -ArgumentList "settings", $settings, "Agent settings"))
  $initialState.Variables.Add((New-Object System.Management.Automation.Runspaces.SessionStateVariableEntry -ArgumentList "logLock", $script:logLock, "Log write lock"))
  $initialState.Variables.Add((New-Object System.Management.Automation.Runspaces.SessionStateVariableEntry -ArgumentList "reservationCacheLock", $script:reservationCacheLock, "Reservation cache lock"))
  $initialState.Variables.Add((New-Object System.Management.Automation.Runspaces.SessionStateVariableEntry -ArgumentList "reservationCache", $script:reservationCache, "Reservation cache"))
  $pool = [System.Management.Automation.Runspaces.RunspaceFactory]::CreateRunspacePool(1, 3, $initialState, $Host)
  $pool.Open()
  return $pool
}

function Start-DhcpScopeDetailTask {
  param([System.Management.Automation.Runspaces.RunspacePool]$Pool, [string]$Kind, [string]$ScopeId)
  $powerShell = [PowerShell]::Create()
  $powerShell.RunspacePool = $Pool
  [void]$powerShell.AddScript("param(`$TaskKind, `$TaskScopeId)`nInvoke-DhcpScopeDetailTask -Kind `$TaskKind -ScopeId `$TaskScopeId").AddArgument($Kind).AddArgument($ScopeId)
  $handle = $powerShell.BeginInvoke()
  return @{ Kind = $Kind; PowerShell = $powerShell; Handle = $handle }
}

function Receive-DhcpScopeDetailTask {
  param([object]$Task)
  $powerShell = $Task["PowerShell"]
  $handle = $Task["Handle"]
  try {
    $result = @($powerShell.EndInvoke($handle))
    if ($result.Count -gt 0) { return $result[0] }
    throw "scope detail task returned no result"
  }
  finally {
    try { $powerShell.Dispose() } catch {}
  }
}

function Json-ScopeDetails {
  param([string]$ScopeId)
  Write-DhcpTrace -Message ("dhcp scope details start scope=" + $ScopeId)
  $detailPool = New-DhcpScopeDetailRunspacePool
  $exclusionsJson = "[]"
  $leasesJson = "[]"
  $reservationsJson = "[]"
  try {
    $tasks = @(
      (Start-DhcpScopeDetailTask -Pool $detailPool -Kind "exclusions" -ScopeId $ScopeId),
      (Start-DhcpScopeDetailTask -Pool $detailPool -Kind "leases" -ScopeId $ScopeId),
      (Start-DhcpScopeDetailTask -Pool $detailPool -Kind "reservations" -ScopeId $ScopeId)
    )
    foreach ($task in $tasks) {
      $result = Receive-DhcpScopeDetailTask -Task $task
      if ($result.Kind -eq "exclusions") { $exclusionsJson = $result.Json }
      if ($result.Kind -eq "leases") { $leasesJson = $result.Json }
      if ($result.Kind -eq "reservations") { $reservationsJson = $result.Json }
    }
  }
  finally {
    try { $detailPool.Close() } catch {}
    try { $detailPool.Dispose() } catch {}
  }
  Write-DhcpTrace -Message ("dhcp scope details done scope=" + $ScopeId)
  return "{" +
    (Json-String "exclusions") + ":" + $exclusionsJson + "," +
    (Json-String "leases") + ":" + $leasesJson + "," +
    (Json-String "reservations") + ":" + $reservationsJson +
    "}"
}

function Invoke-DhcpHttpRequest {
  param([System.Net.HttpListenerContext]$Context)
  if ($null -eq $Context) { return }

  $request = $Context.Request
  $response = $Context.Response
  $requestId = New-RequestId
  $started = Get-Date
  $method = ""
  $path = ""

  try {
    $method = $request.HttpMethod.ToUpperInvariant()
    $path = $request.Url.AbsolutePath
    if (Test-BlankString $path) { $path = "/" }

    if (($path -ne "/health") -and (-not $settings.allowAnonymous)) {
      if ($request.Headers["X-API-Key"] -ne $settings.apiKey) {
        Send-Error -Response $response -RequestId $requestId -StatusCode 401 -Code "UNAUTHORIZED" -Message "Invalid API key"
        return
      }
    }

    if ($method -eq "GET" -and $path -eq "/health") {
      $data = "{" +
        (Json-String "status") + ":" + (Json-String "ok") + "," +
        (Json-String "role") + ":" + (Json-String "dhcp-agent") + "," +
        (Json-String "mode") + ":" + (Json-String "legacy") + "," +
        (Json-String "time") + ":" + (Json-String ((Get-Date).ToUniversalTime().ToString("o"))) + "," +
        (Json-String "netsh") + ":" + (Json-String $(if (Test-NetshAvailable) { "available" } else { "missing" })) +
        "}"
      Send-Data -Response $response -RequestId $requestId -DataJson $data
      return
    }

    if ($method -eq "GET" -and $path -eq "/dhcp/scopes") {
      Send-Data -Response $response -RequestId $requestId -DataJson (Json-Scopes -Scopes @(Get-DhcpScopesLight))
      return
    }

    if ($method -eq "GET" -and $path -eq "/dhcp/probe") {
      Test-DhcpProbe
      Send-Data -Response $response -RequestId $requestId -DataJson '{"status":"ok"}'
      return
    }

    if ($method -eq "POST" -and $path -eq "/dhcp/scopes") {
      Create-DhcpScope -Body (Read-JsonBody -Request $request)
      Send-Data -Response $response -RequestId $requestId -DataJson '{"created":true}'
      return
    }

    if ($method -eq "POST" -and $path -eq "/dhcp/scopes/state") {
      $body = Read-JsonBody -Request $request
      $scopeId = [string](Get-MapValue -Map $body -Name "scopeId" -DefaultValue "")
      $active = [bool](Get-MapValue -Map $body -Name "active" -DefaultValue $false)
      Set-DhcpScopeState -ScopeId $scopeId -Active $active
      Send-Data -Response $response -RequestId $requestId -DataJson ("{" + (Json-String "active") + ":" + (Json-Bool $active) + "}")
      return
    }

    if ($method -eq "DELETE" -and $path -match "^/dhcp/scopes/([^/]+)$") {
      Delete-DhcpScope -ScopeId (UrlDecode $matches[1])
      Send-Data -Response $response -RequestId $requestId -DataJson '{"deleted":true}'
      return
    }

    if ($method -eq "PUT" -and $path -match "^/dhcp/scopes/([^/]+)$") {
      $updatedScope = Update-DhcpScope -ScopeId (UrlDecode $matches[1]) -Body (Read-JsonBody -Request $request)
      Send-Data -Response $response -RequestId $requestId -DataJson (Json-SingleScope -Scope $updatedScope)
      return
    }

    if ($method -eq "POST" -and $path -match "^/dhcp/scopes/([^/]+)/activate$") {
      Set-DhcpScopeState -ScopeId (UrlDecode $matches[1]) -Active $true
      Send-Data -Response $response -RequestId $requestId -DataJson '{"active":true}'
      return
    }

    if ($method -eq "POST" -and $path -match "^/dhcp/scopes/([^/]+)/deactivate$") {
      Set-DhcpScopeState -ScopeId (UrlDecode $matches[1]) -Active $false
      Send-Data -Response $response -RequestId $requestId -DataJson '{"active":false}'
      return
    }

    if ($method -eq "GET" -and $path -match "^/dhcp/scopes/([^/]+)/details$") {
      $scopeId = UrlDecode $matches[1]
      Send-Data -Response $response -RequestId $requestId -DataJson (Json-ScopeDetails -ScopeId $scopeId)
      return
    }

    if ($method -eq "GET" -and $path -match "^/dhcp/scopes/([^/]+)/leases$") {
      $scopeId = UrlDecode $matches[1]
      Send-Data -Response $response -RequestId $requestId -DataJson (Json-Leases -Leases @(Get-DhcpLeases -ScopeId $scopeId))
      return
    }

    if ($method -eq "DELETE" -and $path -match "^/dhcp/scopes/([^/]+)/leases/([^/]+)$") {
      Release-DhcpLease -ScopeId (UrlDecode $matches[1]) -IP (UrlDecode $matches[2])
      Send-Data -Response $response -RequestId $requestId -DataJson '{"released":true}'
      return
    }

    if ($method -eq "POST" -and $path -eq "/dhcp/leases/release") {
      $body = Read-JsonBody -Request $request
      Release-DhcpLease -ScopeId ([string](Get-MapValue -Map $body -Name "scopeId" -DefaultValue "")) -IP ([string](Get-MapValue -Map $body -Name "ip" -DefaultValue ""))
      Send-Data -Response $response -RequestId $requestId -DataJson '{"released":true}'
      return
    }

    if ($method -eq "GET" -and $path -match "^/dhcp/scopes/([^/]+)/exclusions$") {
      $scopeId = UrlDecode $matches[1]
      Send-Data -Response $response -RequestId $requestId -DataJson (Json-Exclusions -Exclusions @(Get-DhcpExclusions -ScopeId $scopeId))
      return
    }

    if ($method -eq "GET" -and $path -match "^/dhcp/scopes/([^/]+)/reservations$") {
      $scopeId = UrlDecode $matches[1]
      Send-Data -Response $response -RequestId $requestId -DataJson (Json-Reservations -Reservations @(Get-DhcpReservations -ScopeId $scopeId))
      return
    }

    if ($method -eq "POST" -and $path -eq "/dhcp/cache/clear") {
      Clear-DhcpReservationCache
      Send-Data -Response $response -RequestId $requestId -DataJson '{"cleared":true}'
      return
    }

    if ($method -eq "POST" -and $path -eq "/dhcp/exclusions") {
      $created = Create-DhcpExclusion -Body (Read-JsonBody -Request $request)
      Send-Data -Response $response -RequestId $requestId -DataJson (Json-Exclusion -Exclusion $created)
      return
    }

    if ($method -eq "POST" -and $path -eq "/dhcp/exclusions/delete") {
      $body = Read-JsonBody -Request $request
      Delete-DhcpExclusion -ScopeId ([string](Get-MapValue -Map $body -Name "scopeId" -DefaultValue "")) -StartIP ([string](Get-MapValue -Map $body -Name "startIp" -DefaultValue "")) -EndIP ([string](Get-MapValue -Map $body -Name "endIp" -DefaultValue ""))
      Send-Data -Response $response -RequestId $requestId -DataJson '{"deleted":true}'
      return
    }

    if ($method -eq "POST" -and $path -eq "/dhcp/reservations") {
      $created = Create-DhcpReservation -Body (Read-JsonBody -Request $request)
      Send-Data -Response $response -RequestId $requestId -DataJson (Json-Reservation -Reservation $created)
      return
    }

    if ($method -eq "POST" -and $path -eq "/dhcp/reservations/update") {
      $updated = Update-DhcpReservation -Body (Read-JsonBody -Request $request)
      Send-Data -Response $response -RequestId $requestId -DataJson (Json-Reservation -Reservation $updated)
      return
    }

    if ($method -eq "POST" -and $path -eq "/dhcp/reservations/delete") {
      $body = Read-JsonBody -Request $request
      Delete-DhcpReservation -ScopeId ([string](Get-MapValue -Map $body -Name "scopeId" -DefaultValue "")) -IP ([string](Get-MapValue -Map $body -Name "ip" -DefaultValue ""))
      Send-Data -Response $response -RequestId $requestId -DataJson '{"deleted":true}'
      return
    }

    if ($method -eq "DELETE" -and $path -match "^/dhcp/reservations/([^/]+)/([^/]+)$") {
      Delete-DhcpReservation -ScopeId (UrlDecode $matches[1]) -IP (UrlDecode $matches[2])
      Send-Data -Response $response -RequestId $requestId -DataJson '{"deleted":true}'
      return
    }

    Send-Error -Response $response -RequestId $requestId -StatusCode 404 -Code "NOT_FOUND" -Message "route not found"
  }
  catch {
    if (Test-ClientDisconnectedError -ErrorObject $_.Exception) {
      Write-AgentLog -LogPath $settings.logPath -Message ("Client disconnected " + $method + " " + $path)
    }
    else {
      Write-AgentLog -LogPath $settings.logPath -Message ("Request failed " + $method + " " + $path + " " + $_.Exception.Message)
      Send-Error -Response $response -RequestId $requestId -StatusCode 500 -Code "AGENT_ERROR" -Message $_.Exception.Message
    }
  }
  finally {
    $elapsed = [int]((Get-Date) - $started).TotalMilliseconds
    Write-AgentLog -LogPath $settings.logPath -Message ($method + " " + $path + " requestId=" + $requestId + " elapsedMs=" + $elapsed)
  }
}

function Get-AgentFunctionNames {
  return @(
    "Test-BlankString",
    "Get-JsonStringValue",
    "Get-JsonIntValue",
    "Get-JsonBoolValue",
    "ConvertTo-AgentBool",
    "ConvertTo-AgentIntRange",
    "Unquote-DotEnvValue",
    "Write-AgentLog",
    "Write-DhcpTrace",
    "Escape-Json",
    "Json-String",
    "Json-Bool",
    "Read-JsonBody",
    "Get-MapValue",
    "UrlDecode",
    "Test-ClientDisconnectedError",
    "Send-Json",
    "Send-Envelope",
    "Send-Data",
    "Send-Error",
    "New-RequestId",
    "Test-NetshAvailable",
    "Invoke-Netsh",
    "Get-NetshDhcp",
    "Test-DhcpProbe",
    "ConvertTo-Mac",
    "Normalize-State",
    "Convert-DateToIso",
    "Test-DhcpLeaseTypeText",
    "Test-DateLikeText",
    "Test-NeverExpiresText",
    "Get-QuotedValues",
    "Read-DhcpLeaseTail",
    "Get-DhcpReservationScopeFromLine",
    "Get-DhcpReservationsFromDumpText",
    "Get-DhcpScopeRangesFromDumpText",
    "Get-DhcpExclusionsFromDumpText",
    "Group-DhcpReservationsByScope",
    "Group-DhcpExclusionsByScope",
    "Clear-DhcpReservationCache",
    "Get-DhcpReservationCacheByScope",
    "Get-DhcpLeaseDurationsFromDumpText",
    "Get-DhcpLeaseDurationMap",
    "Convert-MaskToPrefix",
    "Convert-PrefixToMask",
    "Parse-Subnet",
    "Get-ScopeOptionValue",
    "Test-IPv4Text",
    "Get-FirstIPv4Matches",
    "Get-FirstMacMatch",
    "Test-DhcpStateText",
    "Parse-DhcpScopeLine",
    "Get-DhcpScopeRange",
    "Get-DhcpScopes",
    "Get-DhcpScopesLight",
    "Get-DhcpScopesInternal",
    "Json-Scope",
    "Json-SingleScope",
    "Json-Scopes",
    "Create-DhcpScope",
    "Update-DhcpScope",
    "Set-DhcpScopeState",
    "Delete-DhcpScope",
    "Create-DhcpExclusion",
    "Delete-DhcpExclusion",
    "Get-DhcpLeases",
    "Json-Leases",
    "Release-DhcpLease",
    "Get-DhcpExclusions",
    "Json-Exclusions",
    "Json-Exclusion",
    "Get-DhcpReservations",
    "Json-Reservations",
    "Json-Reservation",
    "Invoke-DhcpScopeDetailTask",
    "Get-ScopeDetailFunctionNames",
    "New-DhcpScopeDetailRunspacePool",
    "Start-DhcpScopeDetailTask",
    "Receive-DhcpScopeDetailTask",
    "Json-ScopeDetails",
    "Create-DhcpReservation",
    "Delete-DhcpReservation",
    "Update-DhcpReservation",
    "Invoke-DhcpHttpRequest"
  )
}

function New-DhcpRequestRunspacePool {
  param([int]$Concurrency)
  $initialState = [System.Management.Automation.Runspaces.InitialSessionState]::CreateDefault()
  foreach ($name in (Get-AgentFunctionNames)) {
    $command = Get-Command $name -CommandType Function -ErrorAction Stop
    $entry = New-Object System.Management.Automation.Runspaces.SessionStateFunctionEntry -ArgumentList $name, $command.Definition
    $initialState.Commands.Add($entry)
  }
  $initialState.Variables.Add((New-Object System.Management.Automation.Runspaces.SessionStateVariableEntry -ArgumentList "settings", $settings, "Agent settings"))
  $initialState.Variables.Add((New-Object System.Management.Automation.Runspaces.SessionStateVariableEntry -ArgumentList "jsonSerializer", $null, "JSON serializer"))
  $initialState.Variables.Add((New-Object System.Management.Automation.Runspaces.SessionStateVariableEntry -ArgumentList "logLock", $script:logLock, "Log write lock"))
  $initialState.Variables.Add((New-Object System.Management.Automation.Runspaces.SessionStateVariableEntry -ArgumentList "reservationCacheLock", $script:reservationCacheLock, "Reservation cache lock"))
  $initialState.Variables.Add((New-Object System.Management.Automation.Runspaces.SessionStateVariableEntry -ArgumentList "reservationCache", $script:reservationCache, "Reservation cache"))
  $pool = [System.Management.Automation.Runspaces.RunspaceFactory]::CreateRunspacePool(1, $Concurrency, $initialState, $Host)
  $pool.Open()
  return $pool
}

function Start-DhcpRequestWorker {
  param([System.Management.Automation.Runspaces.RunspacePool]$Pool, [System.Net.HttpListenerContext]$Context)
  $powerShell = [PowerShell]::Create()
  $powerShell.RunspacePool = $Pool
  [void]$powerShell.AddScript("param(`$RequestContext)`nInvoke-DhcpHttpRequest -Context `$RequestContext").AddArgument($Context)
  $handle = $powerShell.BeginInvoke()
  return @{ PowerShell = $powerShell; Handle = $handle }
}

function Clear-CompletedDhcpRequestWorkers {
  param([System.Collections.ArrayList]$Workers, [string]$LogPath)
  for ($index = $Workers.Count - 1; $index -ge 0; $index--) {
    $worker = $Workers[$index]
    $handle = $worker["Handle"]
    $powerShell = $worker["PowerShell"]
    if (-not $handle.IsCompleted) { continue }
    try {
      $powerShell.EndInvoke($handle) | Out-Null
    }
    catch {
      Write-AgentLog -LogPath $LogPath -Message ("Request worker failed " + $_.Exception.Message)
    }
    finally {
      try { $powerShell.Dispose() } catch {}
      $Workers.RemoveAt($index)
    }
  }
}

function Wait-DhcpRequestWorkers {
  param([System.Collections.ArrayList]$Workers, [string]$LogPath)
  while ($Workers.Count -gt 0) {
    Clear-CompletedDhcpRequestWorkers -Workers $Workers -LogPath $LogPath
    if ($Workers.Count -gt 0) { Start-Sleep -Milliseconds 200 }
  }
}

function Wait-DhcpRequestWorkerSlot {
  param([System.Collections.ArrayList]$Workers, [int]$Limit, [string]$LogPath)
  while ($Workers.Count -ge $Limit) {
    Clear-CompletedDhcpRequestWorkers -Workers $Workers -LogPath $LogPath
    if ($Workers.Count -ge $Limit) { Start-Sleep -Milliseconds 100 }
  }
}

function Create-DhcpReservation {
  param([object]$Body)
  $scopeId = [string](Get-MapValue -Map $Body -Name "scopeId" -DefaultValue "")
  $ip = [string](Get-MapValue -Map $Body -Name "ip" -DefaultValue "")
  $mac = ConvertTo-Mac ([string](Get-MapValue -Map $Body -Name "mac" -DefaultValue ""))
  $name = [string](Get-MapValue -Map $Body -Name "name" -DefaultValue "")
  $description = [string](Get-MapValue -Map $Body -Name "description" -DefaultValue "")
  if (Test-BlankString $scopeId) { throw "scope id is required" }
  if (Test-BlankString $ip) { throw "reservation ip is required" }
  if (Test-BlankString $mac) { throw "reservation mac is required" }
  if (Test-BlankString $name) { $name = $ip }
  [void](Get-NetshDhcp -Arguments @("scope", $scopeId, "add", "reservedip", $ip, $mac, $name, $description))
  Clear-DhcpReservationCache
  return @{
    id = ($scopeId + "|" + $ip)
    scopeId = $scopeId
    ip = $ip
    mac = $mac
    name = $name
    description = $description
  }
}

function Delete-DhcpReservation {
  param([string]$ScopeId, [string]$IP)
  [void](Get-NetshDhcp -Arguments @("scope", $ScopeId, "delete", "reservedip", $IP))
  Clear-DhcpReservationCache
}

function Update-DhcpReservation {
  param([object]$Body)
  $old = Get-MapValue -Map $Body -Name "old" -DefaultValue $null
  $new = Get-MapValue -Map $Body -Name "new" -DefaultValue $null
  if ($null -eq $old) { throw "old reservation is required" }
  if ($null -eq $new) { throw "new reservation is required" }
  $oldScopeId = [string](Get-MapValue -Map $old -Name "scopeId" -DefaultValue "")
  $oldIP = [string](Get-MapValue -Map $old -Name "ip" -DefaultValue "")
  if (Test-BlankString $oldScopeId) { throw "old reservation scope id is required" }
  if (Test-BlankString $oldIP) { throw "old reservation ip is required" }
  Delete-DhcpReservation -ScopeId $oldScopeId -IP $oldIP
  return Create-DhcpReservation -Body $new
}

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$parentRoot = Split-Path -Parent $scriptRoot

$settings = $null
$envPath = Join-Path $parentRoot ".env"
if (Test-Path -LiteralPath $envPath) {
  $settings = Read-DotEnvAgentSettings -Path $envPath
}
else {
  $candidates = @(
    (Join-Path $scriptRoot "agent.json"),
    (Join-Path $parentRoot "config\agent.json"),
    (Join-Path $parentRoot "agent.json")
  )
  foreach ($candidate in $candidates) {
    if (Test-Path -LiteralPath $candidate) {
      $settings = Read-AgentSettings -Path $candidate
      break
    }
  }
}
if ($null -eq $settings) { $settings = New-DefaultAgentSettings }

$scheme = ([string]$settings.scheme).ToLowerInvariant()
if ($scheme -ne "http") {
  throw "Legacy DHCP agent supports http only."
}

if ((-not $settings.allowAnonymous) -and (Test-BlankString $settings.apiKey)) {
  throw "apiKey is required when allowAnonymous is false."
}

Enter-SingleInstance -Name ("Global\ZoneLeaseDhcpLegacyAgent-" + $settings.port)

$listener = New-Object System.Net.HttpListener
$prefix = "http://+:" + $settings.port + "/"
$listener.Prefixes.Add($prefix)

$requestPool = $null
$workers = New-Object System.Collections.ArrayList
try {
  $requestPool = New-DhcpRequestRunspacePool -Concurrency $settings.requestConcurrency
  $listener.Start()
  Write-AgentLog -LogPath $settings.logPath -Message ("Legacy DHCP agent started on " + $prefix + " requestConcurrency=" + $settings.requestConcurrency)
  Write-Host ("ZoneLease DHCP Legacy Agent listening on " + $prefix)
  Write-Host ("Request concurrency: " + $settings.requestConcurrency)
  Write-Host "Press Q to stop."

  while ($listener.IsListening) {
    Clear-CompletedDhcpRequestWorkers -Workers $workers -LogPath $settings.logPath
    Wait-DhcpRequestWorkerSlot -Workers $workers -Limit $settings.requestConcurrency -LogPath $settings.logPath
    $context = Get-NextContext -Listener $listener
    if ($null -eq $context) { break }
    [void]$workers.Add((Start-DhcpRequestWorker -Pool $requestPool -Context $context))
  }

  Wait-DhcpRequestWorkers -Workers $workers -LogPath $settings.logPath
}
finally {
  try { $listener.Stop() } catch {}
  try { $listener.Close() } catch {}
  Wait-DhcpRequestWorkers -Workers $workers -LogPath $settings.logPath
  if ($requestPool -ne $null) {
    try { $requestPool.Close() } catch {}
    try { $requestPool.Dispose() } catch {}
  }
  Exit-SingleInstance
  Write-AgentLog -LogPath $settings.logPath -Message "Legacy DHCP agent stopped"
}
