# Simulates a Face ID 5 talking to the server, for Windows PowerShell 5.1+ or pwsh.
# Usage: .\scripts\simulate-device.ps1 [-Url http://localhost:8081] [-SN DEMO-FACEID5-001]
param(
    [string]$Url = "http://localhost:8081",
    [string]$SN = "DEMO-FACEID5-001"
)
$ErrorActionPreference = "Stop"

function Step($t) { Write-Host "`n=== $t ===" -ForegroundColor Cyan }
function Get-Text($path) { (Invoke-WebRequest -UseBasicParsing -Uri "$Url$path").Content }
function Post-Text($path, $body) {
    (Invoke-WebRequest -UseBasicParsing -Method Post -Uri "$Url$path" -ContentType "text/plain" -Body $body).Content
}
function Post-Json($path, $obj) {
    Invoke-RestMethod -Method Post -Uri "$Url$path" -ContentType "application/json" -Body ($obj | ConvertTo-Json)
}

Step "1. Device powers on and asks for settings (handshake)"
Get-Text "/iclock/cdata?SN=$SN&options=all&pushver=2.4.1"

Step "2. System queues INFO and add_user"
Post-Json "/command" @{ sn = $SN; action = "info" } | Out-Null
Post-Json "/command" @{ sn = $SN; action = "add_user"; pin = "1001"; name = "Ali Demo" } | ConvertTo-Json -Depth 5

Step "3. Device polls getrequest (gets first command)"
$cmd = Get-Text "/iclock/getrequest?SN=$SN"; $cmd
$id = $cmd.Split(":")[1]

Step "4. Device reports INFO result"
Post-Text "/iclock/devicecmd?SN=$SN" "ID=$id&Return=0&CMD=INFO`nDeviceName=Face ID 5`nFWVersion=Ver 8.0.4.2`nUserCount=1`n"

Step "5. Device takes second command and reports success"
$cmd = Get-Text "/iclock/getrequest?SN=$SN"; $cmd
$id = $cmd.Split(":")[1]
Post-Text "/iclock/devicecmd?SN=$SN" "ID=$id&Return=0&CMD=DATA"

Step "6. Employee scans face -> device pushes ATTLOG (realtime)"
$now = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
Post-Text "/iclock/cdata?SN=$SN&table=ATTLOG&Stamp=9999" "1001`t$now`t0`t15`t0`t0`n"

Step "7. No more commands"
Get-Text "/iclock/getrequest?SN=$SN"

Step "8. System checks device status"
Invoke-RestMethod -Uri "$Url/status?sn=$SN" | ConvertTo-Json -Depth 5

Step "9. Command history"
Invoke-RestMethod -Uri "$Url/command?sn=$SN" | ConvertTo-Json -Depth 5
