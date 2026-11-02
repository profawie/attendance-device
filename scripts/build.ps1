# Cross-compile for Windows and Linux from PowerShell. Output goes to dist\.
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")
New-Item -ItemType Directory -Force -Path dist | Out-Null
$env:CGO_ENABLED = "0"
foreach ($t in @("windows/amd64", "linux/amd64", "linux/arm64")) {
    $os, $arch = $t.Split("/")
    $out = "dist/attendance-device-$os-$arch"
    if ($os -eq "windows") { $out = "$out.exe" }
    Write-Host "building $out"
    $env:GOOS = $os; $env:GOARCH = $arch
    go build -trimpath -ldflags "-s -w" -o $out .
    if ($LASTEXITCODE -ne 0) { throw "build failed for $t" }
}
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
