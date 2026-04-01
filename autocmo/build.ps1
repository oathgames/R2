# Build AutoCMO.exe — single-file, no runtime needed
# Requires: Go installed (https://go.dev/dl/)

$ErrorActionPreference = "Stop"
Push-Location $PSScriptRoot

Write-Host "Building AutoCMO.exe..." -ForegroundColor Cyan

$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"

go build -ldflags="-s -w" -o "AutoCMO.exe" .

if ($LASTEXITCODE -eq 0) {
    $size = [math]::Round((Get-Item "AutoCMO.exe").Length / 1MB, 1)
    Write-Host "[OK] Built AutoCMO.exe (${size} MB)" -ForegroundColor Green
    Write-Host ""
    Write-Host "To deploy:" -ForegroundColor Yellow
    Write-Host "  1. Copy AutoCMO.exe to .claude/tools/"
    Write-Host "  2. Copy autocmo-config.json to .claude/tools/"
    Write-Host "  3. Copy cmo.md to .claude/commands/"
} else {
    Write-Host "[FAIL] Build failed" -ForegroundColor Red
}

Pop-Location
