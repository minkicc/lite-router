$ErrorActionPreference = "Stop"

$targets = @(
    @{ GOOS = "windows"; GOARCH = "amd64"; Triple = "x86_64-pc-windows-msvc"; Ext = ".exe" },
    @{ GOOS = "darwin";  GOARCH = "amd64"; Triple = "x86_64-apple-darwin";  Ext = "" },
    @{ GOOS = "darwin";  GOARCH = "arm64"; Triple = "aarch64-apple-darwin";  Ext = "" },
    @{ GOOS = "linux";   GOARCH = "amd64"; Triple = "x86_64-unknown-linux-gnu"; Ext = "" },
    @{ GOOS = "linux";   GOARCH = "arm64"; Triple = "aarch64-unknown-linux-gnu"; Ext = "" }
)

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$dist = Join-Path $root "..\src-tauri\binaries"
New-Item -ItemType Directory -Force -Path $dist | Out-Null
Push-Location $root

foreach ($target in $targets) {
    $env:GOOS = $target.GOOS
    $env:GOARCH = $target.GOARCH
    $env:CGO_ENABLED = "0"
    $name = "mkrouter-core-$($target.Triple)$($target.Ext)"
    $out = Join-Path $dist $name
    Write-Host "building $name"
    go build -trimpath -ldflags "-s -w" -o $out .
}

Pop-Location
Write-Host "done -> $dist"
