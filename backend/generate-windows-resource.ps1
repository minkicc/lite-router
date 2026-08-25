$ErrorActionPreference = "Stop"

$backendDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$tool = "github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.7.0"

Push-Location $backendDir
try {
    go run $tool `
        -64 `
        -icon "../src-tauri/icons/icon.ico" `
        -o "rsrc_windows_amd64.syso" `
        "windows/versioninfo.json"
    if ($LASTEXITCODE -ne 0) {
        throw "goversioninfo failed with exit code $LASTEXITCODE."
    }
} finally {
    Pop-Location
}

Write-Host "Generated: $(Join-Path $backendDir 'rsrc_windows_amd64.syso')"
