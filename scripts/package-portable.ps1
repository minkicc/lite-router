param(
    [string]$TargetTriple = "x86_64-pc-windows-msvc"
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$package = Get-Content -Raw (Join-Path $root "package.json") | ConvertFrom-Json
$targetRoot = Join-Path $root "src-tauri\target"
$targetRelease = Join-Path $targetRoot "$TargetTriple\release"
$localRelease = Join-Path $targetRoot "release"
$releaseDir = if (Test-Path -LiteralPath (Join-Path $targetRelease "mkswitch-desktop.exe")) {
    $targetRelease
} else {
    $localRelease
}

$desktopExe = Join-Path $releaseDir "mkswitch-desktop.exe"
$backendExe = Join-Path $root "src-tauri\binaries\mkswitch-$TargetTriple.exe"
if (-not (Test-Path -LiteralPath $desktopExe)) {
    throw "Desktop executable not found: $desktopExe"
}
if (-not (Test-Path -LiteralPath $backendExe)) {
    throw "Backend executable not found: $backendExe"
}

$bundleDir = Join-Path $releaseDir "bundle\portable"
New-Item -ItemType Directory -Force -Path $bundleDir | Out-Null
$zipPath = Join-Path $bundleDir "MKSwitch_$($package.version)_windows_x64_portable.zip"
$stageRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("mkswitch-portable-" + [guid]::NewGuid().ToString("N"))
$stageDir = Join-Path $stageRoot "MKSwitch"

try {
    New-Item -ItemType Directory -Force -Path $stageDir | Out-Null
    Copy-Item -LiteralPath $desktopExe -Destination (Join-Path $stageDir "MKSwitch.exe")
    Copy-Item -LiteralPath $backendExe -Destination (Join-Path $stageDir "mkswitch.exe")
    Copy-Item -LiteralPath (Join-Path $root "LICENSE") -Destination (Join-Path $stageDir "LICENSE.txt")

    @"
MKSwitch portable edition

Run "MKSwitch.exe" to start.

Configuration and usage records are stored in the system application data
directory, the same as the installed edition. Removing this folder does not
delete your MKSwitch configuration.

Unsigned open-source builds may show a Microsoft Defender SmartScreen warning.
Verify the SHA256 checksum published with the release before running the app.
"@ | Set-Content -LiteralPath (Join-Path $stageDir "README.txt") -Encoding UTF8

    Compress-Archive -LiteralPath $stageDir -DestinationPath $zipPath -CompressionLevel Optimal -Force
} finally {
    if (Test-Path -LiteralPath $stageRoot) {
        Remove-Item -LiteralPath $stageRoot -Recurse -Force
    }
}

$sha256 = [System.Security.Cryptography.SHA256]::Create()
$stream = [System.IO.File]::OpenRead($zipPath)
try {
    $hashBytes = $sha256.ComputeHash($stream)
    $hash = -join ($hashBytes | ForEach-Object { $_.ToString("X2") })
} finally {
    $stream.Dispose()
    $sha256.Dispose()
}
Set-Content -LiteralPath "$zipPath.sha256" -Value "$hash  $(Split-Path -Leaf $zipPath)" -Encoding ASCII
Write-Host "Portable package: $zipPath"
Write-Host "SHA256: $hash"
