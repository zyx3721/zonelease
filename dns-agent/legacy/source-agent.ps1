$ErrorActionPreference = "Stop"

# Windows Server 2008/2008 R2 legacy agent.
# PowerShell 2.0 era HTTP agent. It uses MicrosoftDNS WMI for zone reads and
# dnscmd.exe for record reads/writes because the newer DnsServer module is unavailable.

$script:instanceMutex = $null
$script:jsonSerializer = $null

function Enter-SingleInstance {
  param([string]$Name)
  $script:instanceMutex = New-Object System.Threading.Mutex($false, $Name)
  $hasHandle = $script:instanceMutex.WaitOne(0, $false)
  if (-not $hasHandle) {
    throw "Legacy source agent is already running."
  }
}

function Exit-SingleInstance {
  if ($script:instanceMutex -ne $null) {
    try { $script:instanceMutex.ReleaseMutex() | Out-Null } catch {}
    try { $script:instanceMutex.Close() } catch {}
    $script:instanceMutex = $null
  }
}

function Get-JsonStringValue {
  param([string]$Raw, [string]$Name, [string]$DefaultValue)
  $match = [Regex]::Match($Raw, '"' + [Regex]::Escape($Name) + '"\s*:\s*"([^"\\]*(?:\\.[^"\\]*)*)"')
  if ($match.Success) { return $match.Groups[1].Value.Replace('\\', '\') }
  return $DefaultValue
}

function Get-JsonIntValue {
  param([string]$Raw, [string]$Name, [int]$DefaultValue)
  $match = [Regex]::Match($Raw, '"' + [Regex]::Escape($Name) + '"\s*:\s*([0-9]+)')
  if ($match.Success) { return [int]$match.Groups[1].Value }
  return $DefaultValue
}

function Get-JsonBoolValue {
  param([string]$Raw, [string]$Name, [bool]$DefaultValue)
  $match = [Regex]::Match($Raw, '"' + [Regex]::Escape($Name) + '"\s*:\s*(true|false)', [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
  if ($match.Success) { return $match.Groups[1].Value.ToLowerInvariant() -eq "true" }
  return $DefaultValue
}

function Read-AgentSettings {
  param([string]$Path)
  if (-not (Test-Path -LiteralPath $Path)) {
    throw "Config file not found: $Path"
  }
  $raw = [System.IO.File]::ReadAllText($Path)
  $settings = New-Object PSObject
  Add-Member -InputObject $settings -MemberType NoteProperty -Name scheme -Value (Get-JsonStringValue -Raw $raw -Name "scheme" -DefaultValue "http")
  Add-Member -InputObject $settings -MemberType NoteProperty -Name port -Value (Get-JsonIntValue -Raw $raw -Name "port" -DefaultValue 8460)
  Add-Member -InputObject $settings -MemberType NoteProperty -Name allowAnonymous -Value (Get-JsonBoolValue -Raw $raw -Name "allowAnonymous" -DefaultValue $false)
  Add-Member -InputObject $settings -MemberType NoteProperty -Name apiKey -Value (Get-JsonStringValue -Raw $raw -Name "apiKey" -DefaultValue "")
  Add-Member -InputObject $settings -MemberType NoteProperty -Name logPath -Value (Get-JsonStringValue -Raw $raw -Name "logPath" -DefaultValue "C:\ProgramData\ZoneLease\dns-agent-legacy.log")
  return $settings
}

function New-DefaultAgentSettings {
  $settings = New-Object PSObject
  Add-Member -InputObject $settings -MemberType NoteProperty -Name scheme -Value "http"
  Add-Member -InputObject $settings -MemberType NoteProperty -Name port -Value 8460
  Add-Member -InputObject $settings -MemberType NoteProperty -Name allowAnonymous -Value $false
  Add-Member -InputObject $settings -MemberType NoteProperty -Name apiKey -Value ""
  Add-Member -InputObject $settings -MemberType NoteProperty -Name logPath -Value "C:\ProgramData\ZoneLease\dns-agent-legacy.log"
  return $settings
}

function ConvertTo-AgentBool {
  param([string]$Value, [bool]$DefaultValue)
  if (Test-BlankString $Value) { return $DefaultValue }
  $normalized = $Value.Trim().ToLowerInvariant()
  return (($normalized -eq "1") -or ($normalized -eq "true") -or ($normalized -eq "yes") -or ($normalized -eq "y"))
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

    $separator = $trimmed.IndexOf("=")
    if ($separator -lt 1) { continue }

    $key = $trimmed.Substring(0, $separator).Trim()
    $value = Unquote-DotEnvValue -Value $trimmed.Substring($separator + 1)

    if ($key -eq "DNS_AGENT_PORT") {
      try { $settings.port = [int]$value } catch {}
      continue
    }
    if ($key -eq "DNS_AGENT_API_KEY") {
      $settings.apiKey = $value
      continue
    }
    if ($key -eq "DNS_AGENT_ALLOW_ANONYMOUS") {
      $settings.allowAnonymous = ConvertTo-AgentBool -Value $value -DefaultValue $settings.allowAnonymous
      continue
    }
    if ($key -eq "DNS_AGENT_LOG_PATH") {
      $settings.logPath = $value
      continue
    }
  }

  return $settings
}

function Write-AgentLog {
  param([string]$LogPath, [string]$Message)
  $dir = Split-Path -Path $LogPath -Parent
  if ((-not (Test-BlankString $dir)) -and (-not (Test-Path -LiteralPath $dir))) {
    New-Item -Path $dir -ItemType Directory -Force | Out-Null
  }
  Add-Content -LiteralPath $LogPath -Value "$(Get-Date -Format s) $Message" -Encoding UTF8
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

function Json-WarningsFragment {
  param([string]$Message)
  return "," + (Json-String "warnings") + ":[" + (Json-String $Message) + "]"
}

function Json-PtrFragment {
  param([string]$Name, [string]$Value)
  return "," + (Json-String "ptr") + ":{" +
    (Json-String "name") + ":" + (Json-String $Name) + "," +
    (Json-String "type") + ":" + (Json-String "PTR") + "," +
    (Json-String "value") + ":" + (Json-String $Value) + "," +
    (Json-String "ttl") + ":3600}"
}

function Test-BlankString {
  param([object]$Value)
  if ($null -eq $Value) { return $true }
  return ([string]$Value).Trim().Length -eq 0
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
  return $DefaultValue
}

function Get-RecordField {
  param([object]$Record, [string]$Name, [object]$DefaultValue)
  return Get-MapValue -Map $Record -Name $Name -DefaultValue $DefaultValue
}

function Test-ProtectedRecordType {
  param([string]$Type)
  $typeName = ([string]$Type).Trim().ToUpperInvariant()
  return ($typeName -eq "SOA")
}

function Test-ProtectedRecord {
  param([string]$Type, [string]$Name)
  $typeName = ([string]$Type).Trim().ToUpperInvariant()
  if ($typeName -eq "SOA") { return $true }
  return ($typeName -eq "NS" -and (Test-DnsCmdRootNodeName -NodeName $Name))
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
    $Response.OutputStream.Write($bytes, 0, $bytes.Length)
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

  if ($Success) {
    Send-Json -Response $Response -StatusCode $StatusCode -Json ('{"success":true,"data":' + $DataJson + ',"requestId":' + (Json-String $RequestId) + '}')
  }
  else {
    $errorJson = '{"code":' + (Json-String $ErrorCode) + ',"message":' + (Json-String $ErrorMessage) + '}'
    Send-Json -Response $Response -StatusCode $StatusCode -Json ('{"success":false,"error":' + $errorJson + ',"requestId":' + (Json-String $RequestId) + '}')
  }
}

function New-RequestId {
  return [Guid]::NewGuid().ToString("N")
}

function Test-MicrosoftDnsWmi {
  try {
    Get-WmiObject -Namespace "root\MicrosoftDNS" -Class "MicrosoftDNS_Server" -ErrorAction Stop | Out-Null
    return $true
  }
  catch {
    return $false
  }
}

function Test-DnsCmdAvailable {
  $cmd = Get-Command dnscmd.exe -ErrorAction SilentlyContinue
  return ($cmd -ne $null)
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

function Wait-HttpContext {
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

function Invoke-DnsCmd {
  param([object[]]$Arguments)
  if (-not (Test-DnsCmdAvailable)) {
    throw "dnscmd.exe not found. Install DNS Server Tools on Windows Server 2008/2008 R2."
  }
  $oldErrorActionPreference = $ErrorActionPreference
  $ErrorActionPreference = "Continue"
  try {
    $output = & dnscmd.exe @Arguments 2>&1
    $exitCode = $LASTEXITCODE
  }
  finally {
    $ErrorActionPreference = $oldErrorActionPreference
  }
  if ($exitCode -ne 0) {
    $message = ([string]::Join("`n", [string[]]$output)).Trim()
    if (Test-BlankString $message) { $message = "dnscmd.exe exit code $exitCode" }
    throw $message
  }
  return $output
}

function Test-DnsCmdRecordAlreadyExistsError {
  param([string]$Message)
  $text = [string]$Message
  return ($text -match "DNS_ERROR_RECORD_ALREADY_EXISTS") -or ($text -match "\b9711\b") -or ($text -match "0x25EF")
}

function Test-DnsCmdNameDoesNotExistError {
  param([string]$Message)
  $text = [string]$Message
  return ($text -match "DNS_ERROR_NAME_DOES_NOT_EXIST") -or ($text -match "\b9714\b") -or ($text -match "0x25F2")
}

function Test-ZoneExists {
  param([string]$ZoneName)
  if (Test-BlankString $ZoneName) { return $false }
  $filterZone = $ZoneName.Replace("'", "''")
  $zone = @(Get-WmiObject -Namespace "root\MicrosoftDNS" -Class "MicrosoftDNS_Zone" -Filter "Name='$filterZone'" -ErrorAction SilentlyContinue)
  return ($zone.Count -gt 0)
}

function New-LegacyZone {
  param([object]$Zone)
  $name = [string](Get-MapValue -Map $Zone -Name "name" -DefaultValue "")
  if (Test-BlankString $name) { $name = [string](Get-MapValue -Map $Zone -Name "id" -DefaultValue "") }
  if (Test-BlankString $name) { throw "zone name is required" }
  if (Test-ZoneExists -ZoneName $name) {
    Write-Warning ("Zone already exists, skip create: " + $name)
    return
  }
  [void](Invoke-DnsCmd -Arguments @(".", "/ZoneAdd", $name, "/Primary"))
}

function Remove-LegacyZone {
  param([string]$ZoneName)
  if (Test-BlankString $ZoneName) { throw "zone name is required" }
  if (-not (Test-ZoneExists -ZoneName $ZoneName)) {
    Write-Warning ("Zone not found, skip delete: " + $ZoneName)
    return
  }
  [void](Invoke-DnsCmd -Arguments @(".", "/ZoneDelete", $ZoneName, "/f"))
}

function Convert-ZoneTypeName {
  param([object]$ZoneType)
  $value = [string]$ZoneType
  switch ($value) {
    "1" { return "Primary" }
    "2" { return "Secondary" }
    "3" { return "Stub" }
    default {
      if (Test-BlankString $value) { return "Primary" }
      return $value
    }
  }
}

function Test-SyncableZoneType {
  param([string]$Type)
  $typeName = ([string]$Type).Trim().ToLowerInvariant()
  if (Test-BlankString $typeName) { return $true }
  return @("primary", "secondary", "stub") -contains $typeName
}

function Test-SyncableZoneName {
  param([string]$Name)
  if (Test-BlankString $Name) { return $false }
  return $Name.Trim().ToLowerInvariant() -ne "trustanchors"
}

function Test-ReverseZoneName {
  param([string]$ZoneName)
  if (Test-BlankString $ZoneName) { return $false }
  $name = $ZoneName.Trim().ToLowerInvariant()
  return $name.EndsWith(".in-addr.arpa") -or $name.EndsWith(".ip6.arpa")
}

function Get-ZoneJson {
  $zones = @(Get-WmiObject -Namespace "root\MicrosoftDNS" -Class "MicrosoftDNS_Zone" -ErrorAction Stop)
  $items = New-Object System.Collections.ArrayList
  foreach ($zone in $zones) {
    $name = [string]$zone.Name
    if (Test-BlankString $name) { continue }
    if (-not (Test-SyncableZoneName -Name $name)) { continue }
    $type = Convert-ZoneTypeName -ZoneType $zone.ZoneType
    if (-not (Test-SyncableZoneType -Type $type)) { continue }
    $reverse = Test-ReverseZoneName -ZoneName $name
    $json = '{' +
      '"id":' + (Json-String $name) + ',' +
      '"name":' + (Json-String $name) + ',' +
      '"type":' + (Json-String $type) + ',' +
      '"reverse":' + (Json-Bool $reverse) + ',' +
      '"dynamicUpdate":"None",' +
      '"serverId":"legacy-local"' +
      '}'
    [void]$items.Add($json)
  }
  return '[' + ([string]::Join(',', [string[]]$items.ToArray())) + ']'
}

function Get-WmiPropertyValue {
  param([object]$Record, [string[]]$Names)
  foreach ($name in $Names) {
    try {
      $property = $Record.Properties_[$name]
      if ($property -ne $null -and $property.Value -ne $null) { return [string]$property.Value }
    }
    catch {}
  }
  return ""
}

function Convert-RecordTypeCode {
  param([string]$Value)
  switch ($Value.Trim()) {
    "1" { return "A" }
    "2" { return "NS" }
    "5" { return "CNAME" }
    "6" { return "SOA" }
    "12" { return "PTR" }
    "15" { return "MX" }
    "16" { return "TXT" }
    "28" { return "AAAA" }
    "33" { return "SRV" }
    default { return $Value.Trim().ToUpperInvariant() }
  }
}

function Get-RecordTypeName {
  param([object]$Record)
  $className = [string]$Record.__CLASS
  if ($className -match '^MicrosoftDNS_(.+)Type$') {
    return Convert-RecordTypeCode -Value $Matches[1]
  }
  $recordType = Get-WmiPropertyValue -Record $Record -Names @("RecordType")
  if (-not (Test-BlankString $recordType)) { return Convert-RecordTypeCode -Value $recordType }
  return ""
}

function Convert-OwnerToHostName {
  param([string]$OwnerName, [string]$ZoneName)
  if (Test-BlankString $OwnerName) { return "@" }
  $owner = $OwnerName.TrimEnd('.')
  $zone = $ZoneName.TrimEnd('.')
  if ($owner.ToLowerInvariant() -eq $zone.ToLowerInvariant()) { return "@" }
  $suffix = "." + $zone
  if ($owner.ToLowerInvariant().EndsWith($suffix.ToLowerInvariant())) {
    return $owner.Substring(0, $owner.Length - $suffix.Length)
  }
  return $owner
}

function Convert-ReversePtrNameForDisplay {
  param([string]$ZoneName, [string]$Name)
  if (-not (Test-ReverseZoneName -ZoneName $ZoneName)) { return $Name }
  if (Test-BlankString $Name) { return $Name }
  $trimmed = $Name.Trim().TrimEnd('.')
  if (-not [Regex]::IsMatch($trimmed, '^\d+$')) { return $Name }
  $zone = $ZoneName.Trim().TrimEnd('.').ToLowerInvariant()
  if (-not $zone.EndsWith(".in-addr.arpa")) { return $Name }
  $network = $zone.Substring(0, $zone.Length - ".in-addr.arpa".Length)
  $parts = @($network.Split("."))
  [Array]::Reverse($parts)
  return (([string]::Join(".", [string[]]$parts)) + "." + $trimmed + ".")
}

function Convert-ReversePtrNameForWrite {
  param([string]$ZoneName, [string]$Name)
  if (-not (Test-ReverseZoneName -ZoneName $ZoneName)) { return $Name }
  if (Test-BlankString $Name) { return $Name }
  $trimmed = $Name.Trim().TrimEnd('.')
  $match = [Regex]::Match($trimmed, '^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$')
  if (-not $match.Success) { return $Name }
  $bytes = @()
  for ($i = 1; $i -le 4; $i++) {
    $octet = 0
    if (-not [int]::TryParse($match.Groups[$i].Value, [ref]$octet)) { return $Name }
    if ($octet -lt 0 -or $octet -gt 255) { return $Name }
    $bytes += $octet
  }
  $zone = $ZoneName.Trim().TrimEnd('.').ToLowerInvariant()
  if (-not $zone.EndsWith(".in-addr.arpa")) { return $Name }
  $network = $zone.Substring(0, $zone.Length - ".in-addr.arpa".Length)
  $parts = @($network.Split("."))
  if ($parts.Count -ge 4) { return $Name }
  for ($i = 0; $i -lt $parts.Count; $i++) {
    $expected = 0
    if (-not [int]::TryParse($parts[$parts.Count - 1 - $i], [ref]$expected)) { return $Name }
    if ($bytes[$i] -ne $expected) { return $Name }
  }
  return [string]($bytes[$parts.Count])
}

function Get-RecordValue {
  param([object]$Record, [string]$Type)
  $value = ""
  switch ($Type) {
    "A" { $value = Get-WmiPropertyValue -Record $Record -Names @("IPAddress") }
    "AAAA" { $value = Get-WmiPropertyValue -Record $Record -Names @("IPv6Address", "IPAddress") }
    "CNAME" { $value = Get-WmiPropertyValue -Record $Record -Names @("PrimaryName") }
    "MX" {
      $preference = Get-WmiPropertyValue -Record $Record -Names @("Preference")
      $exchange = Get-WmiPropertyValue -Record $Record -Names @("MailExchange", "MailExchangeHost")
      $value = ($preference + " " + $exchange).Trim()
    }
    "TXT" { $value = Get-WmiPropertyValue -Record $Record -Names @("DescriptiveText", "Text") }
    "PTR" { $value = Get-WmiPropertyValue -Record $Record -Names @("PTRDomainName") }
    "NS" { $value = Get-WmiPropertyValue -Record $Record -Names @("NSHost", "NameServer") }
    "SRV" {
      $priority = Get-WmiPropertyValue -Record $Record -Names @("Priority")
      $weight = Get-WmiPropertyValue -Record $Record -Names @("Weight")
      $port = Get-WmiPropertyValue -Record $Record -Names @("Port")
      $target = Get-WmiPropertyValue -Record $Record -Names @("SRVDomainName", "DomainName")
      $value = ($priority + " " + $weight + " " + $port + " " + $target).Trim()
    }
    default { $value = "" }
  }
  if (-not (Test-BlankString $value)) { return $value }
  return Get-RecordValueFromText -Record $Record -Type $Type
}

function Get-RecordValueFromText {
  param([object]$Record, [string]$Type)
  $text = Get-WmiPropertyValue -Record $Record -Names @("TextRepresentation")
  if (Test-BlankString $text) { return "" }
  $data = ""
  $pattern = "\sIN\s+" + [Regex]::Escape($Type) + "\s+(.+)$"
  $match = [Regex]::Match($text, $pattern, [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
  if ($match.Success) {
    $data = $match.Groups[1].Value.Trim()
  }
  else {
    $parts = $text.Trim() -split "\s+"
    for ($i = 0; $i -lt $parts.Count; $i++) {
      if ($parts[$i].ToUpperInvariant() -eq $Type -and ($i + 1) -lt $parts.Count) {
        $data = ([string]::Join(" ", [string[]]$parts[($i + 1)..($parts.Count - 1)])).Trim()
        break
      }
    }
  }
  if (Test-BlankString $data) { return "" }
  switch ($Type) {
    "MX" {
      $parts = $data -split "\s+", 2
      if ($parts.Count -ge 2) { return ($parts[0] + " " + $parts[1]).Trim() }
      return $data
    }
    "SRV" {
      $parts = $data -split "\s+", 4
      if ($parts.Count -ge 4) { return ($parts[0] + " " + $parts[1] + " " + $parts[2] + " " + $parts[3]).Trim() }
      return $data
    }
    "TXT" {
      if ($data.StartsWith('"') -and $data.EndsWith('"') -and $data.Length -ge 2) {
        return $data.Substring(1, $data.Length - 2).Replace('\"', '"')
      }
      return $data
    }
    default { return $data }
  }
}

function Convert-DnsCmdNodeName {
  param([string]$NodeName, [string]$ZoneName)
  if (Test-DnsCmdRootNodeName -NodeName $NodeName) { return "@" }
  $name = $NodeName.Trim().TrimEnd('.')
  return Convert-OwnerToHostName -OwnerName $name -ZoneName $ZoneName
}

function Convert-LegacyRecordNameForDisplay {
  param([string]$ZoneName, [string]$Name, [string]$Type)
  if (([string]$Type).Trim().ToUpperInvariant() -eq "PTR") {
    return Convert-ReversePtrNameForDisplay -ZoneName $ZoneName -Name $Name
  }
  return $Name
}

function Test-DnsCmdRootNodeName {
  param([string]$NodeName)
  if (Test-BlankString $NodeName) { return $true }
  $name = $NodeName.Trim().TrimEnd('.')
  if ($name -eq "@" -or $name -eq ".") { return $true }
  $label = $name.Trim()
  if ($label.StartsWith("(") -and $label.EndsWith(")") -and $label.Length -gt 2) {
    $label = $label.Substring(1, $label.Length - 2).Trim()
  }
  $sameAsParentZh = [string]::Join('', @([char]0x4E0E, [char]0x7236, [char]0x6587, [char]0x4EF6, [char]0x5939, [char]0x76F8, [char]0x540C))
  if ($label -eq $sameAsParentZh) { return $true }
  return $label.ToLowerInvariant() -eq "same as parent folder"
}

function Join-DnsCmdNodeName {
  param([string]$LeafName, [string]$ParentName, [string]$ZoneName)
  $leaf = Convert-DnsCmdNodeName -NodeName $LeafName -ZoneName $ZoneName
  $parent = Convert-DnsCmdNodeName -NodeName $ParentName -ZoneName $ZoneName

  if ($leaf -eq "@") { return $parent }
  if ($parent -eq "@") { return $leaf }
  if ($leaf.ToLowerInvariant().EndsWith("." + $parent.ToLowerInvariant())) { return $leaf }
  return $leaf + "." + $parent
}

function Test-DnsCmdInteger {
  param([string]$Value)
  if (Test-BlankString $Value) { return $false }
  return [Regex]::IsMatch($Value.Trim(), '^\d+$')
}

function Get-DnsCmdRecordValue {
  param([string[]]$Parts, [int]$TypeIndex, [string]$Type)
  if (($TypeIndex + 1) -ge $Parts.Count) { return "" }
  $data = ([string]::Join(" ", [string[]]$Parts[($TypeIndex + 1)..($Parts.Count - 1)])).Trim()
  if (Test-BlankString $data) { return "" }

  switch ($Type) {
    "MX" {
      $match = [Regex]::Match($data, '^\[(\d+)\]\s+(.+)$')
      if ($match.Success) { return ($match.Groups[1].Value + " " + $match.Groups[2].Value.Trim()).Trim() }
      return $data
    }
    "SRV" {
      $match = [Regex]::Match($data, '^\[(\d+)\]\s*\[(\d+)\]\s*\[(\d+)\]\s+(.+)$')
      if ($match.Success) { return ($match.Groups[1].Value + " " + $match.Groups[2].Value + " " + $match.Groups[3].Value + " " + $match.Groups[4].Value.Trim()).Trim() }
      return $data
    }
    "TXT" {
      if ($data.StartsWith('"') -and $data.EndsWith('"') -and $data.Length -ge 2) {
        return $data.Substring(1, $data.Length - 2).Replace('\"', '"')
      }
      return $data
    }
    default { return $data }
  }
}

function Get-DnsCmdSupportedRecordTypes {
  return @{ "A" = $true; "AAAA" = $true; "CNAME" = $true; "MX" = $true; "TXT" = $true; "PTR" = $true; "NS" = $true; "SRV" = $true; "SOA" = $true }
}

function Get-DnsCmdRecordTypeIndex {
  param([string[]]$Parts, [hashtable]$Supported)
  for ($i = 0; $i -lt $Parts.Count; $i++) {
    $candidateType = $Parts[$i].Trim().ToUpperInvariant()
    if ($Supported.ContainsKey($candidateType)) { return $i }
  }
  return -1
}

function Invoke-DnsCmdEnumRecords {
  param([string]$ZoneName, [string]$NodeName)
  try {
    return Invoke-DnsCmd -Arguments @(".", "/EnumRecords", $ZoneName, $NodeName, "/Additional")
  }
  catch {
    return Invoke-DnsCmd -Arguments @(".", "/EnumRecords", $ZoneName, $NodeName, "/Addtional")
  }
}

function Add-DnsCmdZonePrintParentNodes {
  param([string]$RecordName, [string]$ZoneName, [hashtable]$Seen, [System.Collections.ArrayList]$Nodes)
  $relativeName = Convert-DnsCmdNodeName -NodeName $RecordName -ZoneName $ZoneName
  if (Test-DnsCmdRootNodeName -NodeName $relativeName) { return }
  if (-not $relativeName.Contains(".")) { return }

  $segments = $relativeName -split "\."
  if ($segments.Count -lt 2) { return }
  for ($i = $segments.Count - 1; $i -ge 1; $i--) {
    $parent = ([string]::Join(".", [string[]]$segments[$i..($segments.Count - 1)])).Trim()
    if (Test-BlankString $parent) { continue }
    $key = $parent.ToLowerInvariant()
    if ($Seen.ContainsKey($key)) { continue }
    $Seen[$key] = $true
    [void]$Nodes.Add($parent)
  }
}

function Add-DnsCmdZonePrintNode {
  param([string]$NodeName, [string]$ZoneName, [hashtable]$Seen, [System.Collections.ArrayList]$Nodes)
  $relativeName = Convert-DnsCmdNodeName -NodeName $NodeName -ZoneName $ZoneName
  if (Test-DnsCmdRootNodeName -NodeName $relativeName) { return }
  $key = $relativeName.ToLowerInvariant()
  if ($Seen.ContainsKey($key)) { return }
  $Seen[$key] = $true
  [void]$Nodes.Add($relativeName)
}

function Get-DnsCmdZonePrintChildNodes {
  param([string]$ZoneName, [hashtable]$Supported)
  $nodes = New-Object System.Collections.ArrayList
  $seen = @{ "@" = $true }

  try {
    $output = Invoke-DnsCmd -Arguments @(".", "/ZonePrint", $ZoneName)
  }
  catch {
    return $nodes
  }

  foreach ($line in $output) {
    $text = ([string]$line).Trim()
    if (Test-BlankString $text) { continue }
    if ($text.StartsWith(";")) { continue }
    $lower = $text.ToLowerInvariant()
    if ($lower.Contains("command completed") -or $lower.Contains("completed zone") -or $lower.Contains("zone:") -or $lower.Contains("server:") -or $lower.Contains("time:")) { continue }

    $parts = $text -split "\s+"
    if ($parts.Count -lt 3) { continue }
    $typeIndex = Get-DnsCmdRecordTypeIndex -Parts $parts -Supported $Supported
    if ($typeIndex -lt 1) { continue }
    if (Test-DnsCmdInteger -Value $parts[0]) { continue }

    $recordType = $parts[$typeIndex].Trim().ToUpperInvariant()
    if ($recordType -eq "NS") {
      Add-DnsCmdZonePrintNode -NodeName $parts[0] -ZoneName $ZoneName -Seen $seen -Nodes $nodes
    }
    Add-DnsCmdZonePrintParentNodes -RecordName $parts[0] -ZoneName $ZoneName -Seen $seen -Nodes $nodes
  }

  return $nodes
}

function Add-DnsCmdRecordJsonItemsFromNode {
  param([string]$ZoneName, [string]$NodeName, [hashtable]$Supported, [System.Collections.ArrayList]$Items, [hashtable]$Seen, [string]$Now, [hashtable]$State = $null)
  $currentNode = Convert-DnsCmdNodeName -NodeName $NodeName -ZoneName $ZoneName
  $lastRecordName = $currentNode

  try {
    $output = Invoke-DnsCmdEnumRecords -ZoneName $ZoneName -NodeName $NodeName
    if ($State -ne $null) { $State["succeeded"] = $true }
  }
  catch {
    if ($State -ne $null) { $State["succeeded"] = $false }
    return
  }

  foreach ($line in $output) {
    $text = ([string]$line).Trim()
    if (Test-BlankString $text) { continue }
    $lower = $text.ToLowerInvariant()
    if ($lower.Contains("command completed") -or $lower.Contains("enumerated") -or $lower.Contains("returned records") -or $lower.Contains("node name") -or $lower.StartsWith("----")) { continue }

    $nodeMatch = [Regex]::Match($text, '^(?i)(?:node\s*name|nodename)\s*[:=]\s*(.+)$')
    if ($nodeMatch.Success) {
      $currentNode = Convert-DnsCmdNodeName -NodeName $nodeMatch.Groups[1].Value -ZoneName $ZoneName
      $lastRecordName = $currentNode
      continue
    }

    $parts = $text -split "\s+"
    if ($parts.Count -lt 3) { continue }

    $typeIndex = Get-DnsCmdRecordTypeIndex -Parts $parts -Supported $Supported
    if ($typeIndex -lt 0) {
      if ($parts.Count -eq 1) {
        $currentNode = Convert-DnsCmdNodeName -NodeName $parts[0] -ZoneName $ZoneName
        $lastRecordName = $currentNode
      }
      continue
    }

    $type = $parts[$typeIndex].Trim().ToUpperInvariant()
    $ttl = 3600
    if ($typeIndex -gt 0 -and (Test-DnsCmdInteger -Value $parts[$typeIndex - 1])) {
      try { $ttl = [int]$parts[$typeIndex - 1] } catch { $ttl = 3600 }
    }

    if ($typeIndex -eq 0) {
      $name = Convert-DnsCmdNodeName -NodeName $lastRecordName -ZoneName $ZoneName
    }
    elseif (($typeIndex -eq 1) -and (Test-DnsCmdInteger -Value $parts[0])) {
      $name = Convert-DnsCmdNodeName -NodeName $lastRecordName -ZoneName $ZoneName
    }
    else {
      $name = Join-DnsCmdNodeName -LeafName $parts[0] -ParentName $currentNode -ZoneName $ZoneName
      $lastRecordName = $name
    }
    $value = Get-DnsCmdRecordValue -Parts $parts -TypeIndex $typeIndex -Type $type
    if (Test-BlankString $value) { continue }
    if ($type -eq "PTR") {
      $name = Convert-ReversePtrNameForDisplay -ZoneName $ZoneName -Name $name
    }

    $key = $ZoneName + "|" + $type + "|" + $name + "|" + $value
    if ($Seen.ContainsKey($key)) { continue }
    $Seen[$key] = $true

    $json = '{' +
      '"id":' + (Json-String $key) + ',' +
      '"zoneId":' + (Json-String $ZoneName) + ',' +
      '"name":' + (Json-String $name) + ',' +
      '"type":' + (Json-String $type) + ',' +
      '"value":' + (Json-String $value) + ',' +
      '"ttl":' + $ttl + ',' +
      '"updatedAt":' + (Json-String $Now) +
      '}'
    [void]$Items.Add($json)
  }
}

function Get-DnsCmdRecordJsonItems {
  param([string]$ZoneName)
  $supported = Get-DnsCmdSupportedRecordTypes
  $items = New-Object System.Collections.ArrayList
  $seen = @{}
  $now = (Get-Date).ToUniversalTime().ToString("o")

  Add-DnsCmdRecordJsonItemsFromNode -ZoneName $ZoneName -NodeName "@" -Supported $supported -Items $items -Seen $seen -Now $now
  foreach ($node in @(Get-DnsCmdZonePrintChildNodes -ZoneName $ZoneName -Supported $supported)) {
    Add-DnsCmdRecordJsonItemsFromNode -ZoneName $ZoneName -NodeName $node -Supported $supported -Items $items -Seen $seen -Now $now
  }

  return $items
}

function Get-DnsCmdRecordJsonItemsFromTargetNode {
  param([string]$ZoneName, [string]$Name, [string]$Type)
  $supported = Get-DnsCmdSupportedRecordTypes
  $items = New-Object System.Collections.ArrayList
  $seen = @{}
  $now = (Get-Date).ToUniversalTime().ToString("o")
  $state = @{ succeeded = $false }
  $nodeName = Convert-LegacyRecordNameForWrite -Name $Name -ZoneName $ZoneName -Type $Type
  Add-DnsCmdRecordJsonItemsFromNode -ZoneName $ZoneName -NodeName $nodeName -Supported $supported -Items $items -Seen $seen -Now $now -State $state
  return @{ succeeded = $state["succeeded"]; items = $items }
}

function Get-RecordJson {
  param([string]$ZoneName)
  $items = New-Object System.Collections.ArrayList
  foreach ($json in @(Get-DnsCmdRecordJsonItems -ZoneName $ZoneName)) {
    [void]$items.Add($json)
  }
  return '[' + ([string]::Join(',', [string[]]$items.ToArray())) + ']'
}

function Compare-RecordValue {
  param([string]$Left, [string]$Right, [string]$Type)
  if ($Type -eq "TXT") { return ([string]$Left) -eq ([string]$Right) }
  return ([string]$Left).Trim().ToLowerInvariant() -eq ([string]$Right).Trim().ToLowerInvariant()
}

function Find-LegacyRecordInJsonItems {
  param([object[]]$JsonItems, [string]$CompareName, [string]$TypeName, [string]$Value)
  foreach ($json in @($JsonItems)) {
    $recordType = Get-JsonStringValue -Raw $json -Name "type" -DefaultValue ""
    if ($recordType.Trim().ToUpperInvariant() -ne $typeName) { continue }

    $recordName = Get-JsonStringValue -Raw $json -Name "name" -DefaultValue "@"
    if ($recordName.Trim().ToLowerInvariant() -ne $compareName) { continue }

    $recordValue = Get-JsonStringValue -Raw $json -Name "value" -DefaultValue ""
    if (Compare-RecordValue -Left $recordValue -Right $Value -Type $typeName) { return $json }
  }
  return $null
}

function Find-LegacyRecord {
  param([string]$ZoneName, [string]$Name, [string]$Type, [string]$Value)
  if (Test-BlankString $ZoneName) { return $null }
  if (Test-BlankString $Name) { $Name = "@" }
  $typeName = ([string]$Type).Trim().ToUpperInvariant()
  if (Test-BlankString $typeName) { return $null }
  $compareName = $Name.Trim().ToLowerInvariant()
  if ($typeName -eq "PTR") {
    $compareName = (Convert-ReversePtrNameForDisplay -ZoneName $ZoneName -Name $Name).Trim().ToLowerInvariant()
  }

  $targetResult = Get-DnsCmdRecordJsonItemsFromTargetNode -ZoneName $ZoneName -Name $Name -Type $typeName
  $targetRecord = Find-LegacyRecordInJsonItems -JsonItems @($targetResult.items) -CompareName $compareName -TypeName $typeName -Value $Value
  if ($targetRecord -ne $null) { return $targetRecord }
  if ($targetResult.succeeded) { return $null }

  return Find-LegacyRecordInJsonItems -JsonItems @(Get-DnsCmdRecordJsonItems -ZoneName $ZoneName) -CompareName $compareName -TypeName $typeName -Value $Value
}

function Get-LegacyRecordName {
  param([object]$Record)
  $name = [string](Get-RecordField -Record $Record -Name "name" -DefaultValue "@")
  if (Test-BlankString $name) { return "@" }
  return $name
}

function Convert-LegacyRecordNameForWrite {
  param([string]$Name, [string]$ZoneName = "", [string]$Type = "")
  if (Test-DnsCmdRootNodeName -NodeName $Name) { return "@" }
  if (([string]$Type).Trim().ToUpperInvariant() -eq "PTR") {
    return Convert-ReversePtrNameForWrite -ZoneName $ZoneName -Name $Name
  }
  return $Name
}

function Get-LegacyRecordNameCandidatesForWrite {
  param([string]$Name, [string]$ZoneName = "", [string]$Type = "")
  if (Test-DnsCmdRootNodeName -NodeName $Name) { return @("@", ".", "") }
  if (([string]$Type).Trim().ToUpperInvariant() -eq "PTR") {
    return @(Convert-ReversePtrNameForWrite -ZoneName $ZoneName -Name $Name)
  }
  return @($Name)
}

function Get-LegacyRecordType {
  param([object]$Record)
  return ([string](Get-RecordField -Record $Record -Name "type" -DefaultValue "")).Trim().ToUpperInvariant()
}

function Get-LegacyRecordValue {
  param([object]$Record)
  return [string](Get-RecordField -Record $Record -Name "value" -DefaultValue "")
}

function Get-LegacyRecordCreatePtr {
  param([object]$Record)
  $value = Get-RecordField -Record $Record -Name "createPtr" -DefaultValue $false
  try { return [bool]$value } catch { return $false }
}

function Get-DnsCmdAddArguments {
  param([string]$ZoneName, [string]$Name, [string]$Type, [string]$Value, [string]$WriteName = $null)
  if (Test-BlankString $WriteName) { $nodeName = Convert-LegacyRecordNameForWrite -Name $Name -ZoneName $ZoneName -Type $Type } else { $nodeName = $WriteName }
  switch ($Type) {
    "A" { return @(".", "/RecordAdd", $ZoneName, $nodeName, "A", $Value) }
    "AAAA" { return @(".", "/RecordAdd", $ZoneName, $nodeName, "AAAA", $Value) }
    "CNAME" { return @(".", "/RecordAdd", $ZoneName, $nodeName, "CNAME", $Value) }
    "PTR" { return @(".", "/RecordAdd", $ZoneName, $nodeName, "PTR", $Value) }
    "TXT" { return @(".", "/RecordAdd", $ZoneName, $nodeName, "TXT", $Value) }
    "NS" { return @(".", "/RecordAdd", $ZoneName, $nodeName, "NS", $Value) }
    "MX" {
      $parts = $Value -split "\s+", 2
      if ($parts.Count -lt 2) { throw "MX value format: preference mail.example.com" }
      return @(".", "/RecordAdd", $ZoneName, $nodeName, "MX", $parts[0], $parts[1])
    }
    "SRV" {
      $parts = $Value -split "\s+", 4
      if ($parts.Count -lt 4) { throw "SRV value format: priority weight port target" }
      return @(".", "/RecordAdd", $ZoneName, $nodeName, "SRV", $parts[0], $parts[1], $parts[2], $parts[3])
    }
    default { throw "Unsupported record type: $Type" }
  }
}

function Get-DnsCmdDeleteArguments {
  param([string]$ZoneName, [string]$Name, [string]$Type, [string]$Value, [string]$WriteName = $null)
  if (Test-BlankString $WriteName) { $nodeName = Convert-LegacyRecordNameForWrite -Name $Name -ZoneName $ZoneName -Type $Type } else { $nodeName = $WriteName }
  switch ($Type) {
    "A" { return @(".", "/RecordDelete", $ZoneName, $nodeName, "A", $Value, "/f") }
    "AAAA" { return @(".", "/RecordDelete", $ZoneName, $nodeName, "AAAA", $Value, "/f") }
    "CNAME" { return @(".", "/RecordDelete", $ZoneName, $nodeName, "CNAME", $Value, "/f") }
    "PTR" { return @(".", "/RecordDelete", $ZoneName, $nodeName, "PTR", $Value, "/f") }
    "TXT" { return @(".", "/RecordDelete", $ZoneName, $nodeName, "TXT", $Value, "/f") }
    "NS" { return @(".", "/RecordDelete", $ZoneName, $nodeName, "NS", $Value, "/f") }
    "MX" {
      $parts = $Value -split "\s+", 2
      if ($parts.Count -lt 2) { throw "MX value format: preference mail.example.com" }
      return @(".", "/RecordDelete", $ZoneName, $nodeName, "MX", $parts[0], $parts[1], "/f")
    }
    "SRV" {
      $parts = $Value -split "\s+", 4
      if ($parts.Count -lt 4) { throw "SRV value format: priority weight port target" }
      return @(".", "/RecordDelete", $ZoneName, $nodeName, "SRV", $parts[0], $parts[1], $parts[2], $parts[3], "/f")
    }
    default { throw "Unsupported record type: $Type" }
  }
}

function Add-LegacyPtrBestEffort {
  param([string]$ZoneName, [string]$Name, [string]$IPv4)
  try {
    $ip = [System.Net.IPAddress]::Parse($IPv4)
    $bytes = $ip.GetAddressBytes()
    if ($bytes.Length -ne 4) { return }
    $reverseZone = ([string]$bytes[2]) + "." + ([string]$bytes[1]) + "." + ([string]$bytes[0]) + ".in-addr.arpa"
    $ptrName = [string]$bytes[3]
    if ($Name -eq "@") { $fqdn = $ZoneName } else { $fqdn = $Name + "." + $ZoneName }
    if (-not $fqdn.EndsWith(".")) { $fqdn = $fqdn + "." }
    if (-not (Test-ZoneExists -ZoneName $reverseZone)) {
      $warning = "PTR_REVERSE_ZONE_NOT_FOUND"
      Write-Warning ($warning + ": " + $reverseZone)
      return (Json-WarningsFragment -Message $warning)
    }
    $existing = Find-LegacyRecord -ZoneName $reverseZone -Name $ptrName -Type "PTR" -Value $fqdn
    $displayName = Convert-ReversePtrNameForDisplay -ZoneName $reverseZone -Name $ptrName
    if ($existing -ne $null) {
      return (Json-PtrFragment -Name $displayName -Value $fqdn)
    }
    [void](Invoke-DnsCmd -Arguments (Get-DnsCmdAddArguments -ZoneName $reverseZone -Name $ptrName -Type "PTR" -Value $fqdn))
    return (Json-PtrFragment -Name $displayName -Value $fqdn)
  }
  catch {
    $warning = "PTR_REVERSE_ZONE_NOT_FOUND"
    Write-Warning $warning
    return (Json-WarningsFragment -Message $warning)
  }
  return ""
}

function Remove-LegacyPtrBestEffort {
  param([string]$ZoneName, [string]$Name, [string]$IPv4)
  try {
    $ip = [System.Net.IPAddress]::Parse($IPv4)
    $bytes = $ip.GetAddressBytes()
    if ($bytes.Length -ne 4) { return }
    $reverseZone = ([string]$bytes[2]) + "." + ([string]$bytes[1]) + "." + ([string]$bytes[0]) + ".in-addr.arpa"
    $ptrName = [string]$bytes[3]
    if ($Name -eq "@") { $fqdn = $ZoneName } else { $fqdn = $Name + "." + $ZoneName }
    if (-not $fqdn.EndsWith(".")) { $fqdn = $fqdn + "." }
    if (-not (Test-ZoneExists -ZoneName $reverseZone)) { return }
    $existing = Find-LegacyRecord -ZoneName $reverseZone -Name $ptrName -Type "PTR" -Value $fqdn
    if ($existing -eq $null) { return }
    [void](Invoke-DnsCmd -Arguments (Get-DnsCmdDeleteArguments -ZoneName $reverseZone -Name $ptrName -Type "PTR" -Value $fqdn))
  }
  catch {
    Write-Warning ("PTR record deletion skipped: " + $_.Exception.Message)
  }
}

function Add-LegacyRecord {
  param([string]$ZoneName, [object]$Record)
  $name = Get-LegacyRecordName -Record $Record
  $type = Get-LegacyRecordType -Record $Record
  $value = Get-LegacyRecordValue -Record $Record
  if (Test-BlankString $type) { return }
  if (Test-ProtectedRecord -Type $type -Name $name) {
    Write-Warning ("Protected record type skipped: " + $type)
    return
  }
  if (Test-BlankString $value) { throw "record value is required" }

  $existing = Find-LegacyRecord -ZoneName $ZoneName -Name $name -Type $type -Value $value
  if ($existing -ne $null) {
    Write-Warning ("Record already exists, skip add: " + $type + " " + $name + " " + $value)
    return
  }

  $errors = New-Object System.Collections.ArrayList
  foreach ($writeName in @(Get-LegacyRecordNameCandidatesForWrite -Name $name -ZoneName $ZoneName -Type $type)) {
    try {
      [void](Invoke-DnsCmd -Arguments (Get-DnsCmdAddArguments -ZoneName $ZoneName -Name $name -Type $type -Value $value -WriteName $writeName))
    }
    catch {
      if (Test-DnsCmdRecordAlreadyExistsError -Message $_.Exception.Message) {
        Write-Warning ("Record already exists, skip add: " + $type + " " + $name + " " + $value)
        return
      }
      [void]$errors.Add($_.Exception.Message)
    }
    $created = Find-LegacyRecord -ZoneName $ZoneName -Name $name -Type $type -Value $value
    if ($created -ne $null) { break }
  }
  $created = Find-LegacyRecord -ZoneName $ZoneName -Name $name -Type $type -Value $value
  if ($created -eq $null) { throw ("record add verification failed: $type $name $value; " + ([string]::Join("; ", [string[]]$errors.ToArray()))) }
  if (($type -eq "A") -and (Get-LegacyRecordCreatePtr -Record $Record)) {
    return ('{"created":true' + (Add-LegacyPtrBestEffort -ZoneName $ZoneName -Name $name -IPv4 $value) + '}')
  }
  return '{"created":true}'
}

function Remove-LegacyRecord {
  param([string]$ZoneName, [object]$Record)
  $name = Get-LegacyRecordName -Record $Record
  $type = Get-LegacyRecordType -Record $Record
  $value = Get-LegacyRecordValue -Record $Record
  if (Test-BlankString $type) { return }
  if (Test-ProtectedRecord -Type $type -Name $name) {
    Write-Warning ("Protected record type skipped: " + $type)
    return
  }
  if (Test-BlankString $value) { throw "record value is required" }

  $existing = Find-LegacyRecord -ZoneName $ZoneName -Name $name -Type $type -Value $value
  if ($existing -eq $null) {
    Write-Warning ("Record not found, skip delete: " + $type + " " + $name + " " + $value)
    return
  }
  try {
    [void](Invoke-DnsCmd -Arguments (Get-DnsCmdDeleteArguments -ZoneName $ZoneName -Name $name -Type $type -Value $value))
  }
  catch {
    if (Test-DnsCmdNameDoesNotExistError -Message $_.Exception.Message) {
      Write-Warning ("Record not found, skip delete: " + $type + " " + $name + " " + $value)
      return
    }
    throw
  }
  if (($type -eq "A") -and (Get-LegacyRecordCreatePtr -Record $Record)) {
    Remove-LegacyPtrBestEffort -ZoneName $ZoneName -Name $name -IPv4 $value
  }
}

function Update-LegacyRecord {
  param([string]$ZoneName, [object]$Update)
  $old = Get-MapValue -Map $Update -Name "old" -DefaultValue $null
  $new = Get-MapValue -Map $Update -Name "new" -DefaultValue $null
  if (($null -eq $old) -or ($null -eq $new)) { return }
  $type = Get-LegacyRecordType -Record $old
  if (Test-ProtectedRecord -Type $type -Name (Get-LegacyRecordName -Record $old)) {
    Write-Warning ("Protected record type skipped: " + $type)
    return
  }
  if ((Get-LegacyRecordType -Record $new) -ne $type) { throw "record update type cannot change" }

  $newExisting = Find-LegacyRecord -ZoneName $ZoneName -Name (Get-LegacyRecordName -Record $new) -Type $type -Value (Get-LegacyRecordValue -Record $new)
  if ($newExisting -ne $null) {
    Write-Warning ("Target record already exists, skip update: " + $type + " " + (Get-LegacyRecordName -Record $new) + " " + (Get-LegacyRecordValue -Record $new))
    return
  }

  Remove-LegacyRecord -ZoneName $ZoneName -Record $old
  Add-LegacyRecord -ZoneName $ZoneName -Record $new
}

function Invoke-LegacyRecordBatch {
  param([string]$ZoneName, [object]$Batch)
  foreach ($update in @(Get-MapValue -Map $Batch -Name "update" -DefaultValue @())) {
    if ($null -ne $update) { Update-LegacyRecord -ZoneName $ZoneName -Update $update }
  }
  foreach ($record in @(Get-MapValue -Map $Batch -Name "add" -DefaultValue @())) {
    if ($null -ne $record) { Add-LegacyRecord -ZoneName $ZoneName -Record $record }
  }
  foreach ($record in @(Get-MapValue -Map $Batch -Name "delete" -DefaultValue @())) {
    if ($null -ne $record) { Remove-LegacyRecord -ZoneName $ZoneName -Record $record }
  }
}

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$parentRoot = Split-Path -Parent $scriptRoot
$envPath = Join-Path $parentRoot ".env"

if (Test-Path -LiteralPath $envPath) {
  $settings = Read-DotEnvAgentSettings -Path $envPath
  Write-Host ("Using .env: " + $envPath)
}
else {
  $settingsPath = Join-Path $scriptRoot "agent.json"
  if (-not (Test-Path -LiteralPath $settingsPath)) {
    $settingsPath = Join-Path $parentRoot "config\agent.json"
  }
  if (-not (Test-Path -LiteralPath $settingsPath)) {
    $settingsPath = Join-Path $parentRoot "agent.json"
  }

  if (Test-Path -LiteralPath $settingsPath) {
    $settings = Read-AgentSettings -Path $settingsPath
    Write-Host ("Using config: " + $settingsPath)
  }
  else {
    $settings = New-DefaultAgentSettings
    Write-Host "No .env or agent.json found; using built-in defaults." -ForegroundColor Yellow
  }
}

$scheme = ([string]$settings.scheme).ToLowerInvariant()
if ($scheme -ne "http") {
  throw "Legacy source agent supports http only."
}

if ((-not $settings.allowAnonymous) -and (Test-BlankString $settings.apiKey)) {
  throw "apiKey is required when allowAnonymous is false."
}

Enter-SingleInstance -Name ("Global\ZoneLeaseDnsLegacyAgent-" + $settings.port)

$listener = New-Object System.Net.HttpListener
$prefix = "http://+:" + $settings.port + "/"
$listener.Prefixes.Add($prefix)

try {
  $listener.Start()
  Write-AgentLog -LogPath $settings.logPath -Message ("Legacy source agent started on " + $prefix)
  Write-Host ("ZoneLease DNS Legacy Agent listening on " + $prefix)
  Write-Host "Press Q to stop."

  while ($listener.IsListening) {
    $context = Wait-HttpContext -Listener $listener
    if ($null -eq $context) { break }
    $request = $context.Request
    $response = $context.Response
    $requestId = New-RequestId
    $started = Get-Date

    try {
      $path = $request.Url.AbsolutePath.TrimEnd("/")
      if (Test-BlankString $path) { $path = "/" }
      $method = $request.HttpMethod.ToUpperInvariant()

      if (($path -ne "/health") -and (-not $settings.allowAnonymous) -and $request.Headers["X-API-Key"] -ne $settings.apiKey) {
        Send-Envelope -Response $response -StatusCode 401 -Success $false -DataJson "null" -ErrorCode "UNAUTHORIZED" -ErrorMessage "Invalid API key" -RequestId $requestId
        continue
      }

      if ($method -eq "GET" -and $path -eq "/health") {
        $data = '{"status":"ok","mode":"legacy","time":' + (Json-String ((Get-Date).ToUniversalTime().ToString("o"))) + ',"microsoftDnsWmi":' + (Json-String $(if (Test-MicrosoftDnsWmi) { "available" } else { "missing" })) + ',"dnscmd":' + (Json-String $(if (Test-DnsCmdAvailable) { "available" } else { "missing" })) + '}'
        Send-Envelope -Response $response -StatusCode 200 -Success $true -DataJson $data -ErrorCode "" -ErrorMessage "" -RequestId $requestId
        continue
      }

      if ($method -eq "GET" -and $path -eq "/dns/zones") {
        Send-Envelope -Response $response -StatusCode 200 -Success $true -DataJson (Get-ZoneJson) -ErrorCode "" -ErrorMessage "" -RequestId $requestId
        continue
      }

      if ($method -eq "POST" -and $path -eq "/dns/zones") {
        $body = Read-JsonBody -Request $request
        New-LegacyZone -Zone $body
        Send-Envelope -Response $response -StatusCode 200 -Success $true -DataJson '{"created":true}' -ErrorCode "" -ErrorMessage "" -RequestId $requestId
        continue
      }

      if ($method -eq "POST" -and $path -eq "/dns/records/query") {
        $body = Read-JsonBody -Request $request
        $zoneName = [string](Get-MapValue -Map $body -Name "zone" -DefaultValue "")
        if (Test-BlankString $zoneName) { throw "zone is required" }
        Send-Envelope -Response $response -StatusCode 200 -Success $true -DataJson (Get-RecordJson -ZoneName $zoneName) -ErrorCode "" -ErrorMessage "" -RequestId $requestId
        continue
      }

      if ($method -eq "POST" -and $path -eq "/dns/records/create") {
        $body = Read-JsonBody -Request $request
        $zoneName = [string](Get-MapValue -Map $body -Name "zone" -DefaultValue "")
        $record = Get-MapValue -Map $body -Name "record" -DefaultValue $null
        if (Test-BlankString $zoneName) { throw "zone is required" }
        if ($null -eq $record) { throw "record is required" }
        $resultJson = Add-LegacyRecord -ZoneName $zoneName -Record $record
        Send-Envelope -Response $response -StatusCode 200 -Success $true -DataJson $resultJson -ErrorCode "" -ErrorMessage "" -RequestId $requestId
        continue
      }

      if ($method -eq "POST" -and $path -eq "/dns/records/delete") {
        $body = Read-JsonBody -Request $request
        $zoneName = [string](Get-MapValue -Map $body -Name "zone" -DefaultValue "")
        $record = Get-MapValue -Map $body -Name "record" -DefaultValue $null
        if (Test-BlankString $zoneName) { throw "zone is required" }
        if ($null -eq $record) { throw "record is required" }
        Remove-LegacyRecord -ZoneName $zoneName -Record $record
        Send-Envelope -Response $response -StatusCode 200 -Success $true -DataJson '{"deleted":true}' -ErrorCode "" -ErrorMessage "" -RequestId $requestId
        continue
      }

      if ($method -eq "POST" -and $path -eq "/dns/records/update") {
        $body = Read-JsonBody -Request $request
        $zoneName = [string](Get-MapValue -Map $body -Name "zone" -DefaultValue "")
        $update = Get-MapValue -Map $body -Name "update" -DefaultValue $null
        if (Test-BlankString $zoneName) { throw "zone is required" }
        if ($null -eq $update) { throw "update is required" }
        Update-LegacyRecord -ZoneName $zoneName -Update $update
        Send-Envelope -Response $response -StatusCode 200 -Success $true -DataJson '{"created":true}' -ErrorCode "" -ErrorMessage "" -RequestId $requestId
        continue
      }

      if ($method -eq "DELETE" -and $path -match "^/dns/zones/([^/]+)$") {
        $zoneName = [Uri]::UnescapeDataString($Matches[1])
        Remove-LegacyZone -ZoneName $zoneName
        Send-Envelope -Response $response -StatusCode 200 -Success $true -DataJson '{"deleted":true}' -ErrorCode "" -ErrorMessage "" -RequestId $requestId
        continue
      }

      if ($method -eq "GET" -and $path -match "^/dns/zones/([^/]+)/records$") {
        $zoneName = [Uri]::UnescapeDataString($Matches[1])
        Send-Envelope -Response $response -StatusCode 200 -Success $true -DataJson (Get-RecordJson -ZoneName $zoneName) -ErrorCode "" -ErrorMessage "" -RequestId $requestId
        continue
      }

      if ($method -eq "POST" -and $path -match "^/dns/zones/([^/]+)/records/batch$") {
        $zoneName = [Uri]::UnescapeDataString($Matches[1])
        $body = Read-JsonBody -Request $request
        Invoke-LegacyRecordBatch -ZoneName $zoneName -Batch $body
        Send-Envelope -Response $response -StatusCode 200 -Success $true -DataJson '{"applied":true}' -ErrorCode "" -ErrorMessage "" -RequestId $requestId
        continue
      }

      if ($method -eq "POST" -and $path -match "^/dns/zones/([^/]+)/records$") {
        $zoneName = [Uri]::UnescapeDataString($Matches[1])
        $body = Read-JsonBody -Request $request
        $resultJson = Add-LegacyRecord -ZoneName $zoneName -Record $body
        Send-Envelope -Response $response -StatusCode 200 -Success $true -DataJson $resultJson -ErrorCode "" -ErrorMessage "" -RequestId $requestId
        continue
      }

      if ($method -eq "DELETE" -and $path -match "^/dns/zones/([^/]+)/records/([^/]+)/([^/]+)$") {
        $zoneName = [Uri]::UnescapeDataString($Matches[1])
        $typeName = [Uri]::UnescapeDataString($Matches[2])
        $recordName = [Uri]::UnescapeDataString($Matches[3])
        $record = @{ name = $recordName; type = $typeName; value = $request.QueryString["value"]; createPtr = ($request.QueryString["createPtr"] -eq "true") }
        Remove-LegacyRecord -ZoneName $zoneName -Record $record
        Send-Envelope -Response $response -StatusCode 200 -Success $true -DataJson '{"deleted":true}' -ErrorCode "" -ErrorMessage "" -RequestId $requestId
        continue
      }

      Send-Envelope -Response $response -StatusCode 404 -Success $false -DataJson "null" -ErrorCode "NOT_FOUND" -ErrorMessage ("Route not found: " + $method + " " + $path) -RequestId $requestId
    }
    catch {
      Send-Envelope -Response $response -StatusCode 500 -Success $false -DataJson "null" -ErrorCode "INTERNAL_ERROR" -ErrorMessage $_.Exception.Message -RequestId $requestId
    }
    finally {
      $elapsed = [int]((Get-Date) - $started).TotalMilliseconds
      Write-AgentLog -LogPath $settings.logPath -Message ($request.HttpMethod + " " + $request.Url.AbsolutePath + " requestId=" + $requestId + " elapsedMs=" + $elapsed)
    }
  }
}
finally {
  if ($listener -ne $null) {
    try { $listener.Stop() } catch {}
    try { $listener.Close() } catch {}
  }
  Exit-SingleInstance
}
