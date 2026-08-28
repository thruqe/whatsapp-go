# WhatsRook Windows Automated Installer
# Usage: irm https://raw.githubusercontent.com/ThruqeLabs/whatsrook/master/scripts/install.ps1 | iex

$ErrorActionPreference = "Stop"

[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 -bor [Net.SecurityProtocolType]::Tls13

$Repo = "ThruqeLabs/whatsrook"
$BinName = "whatsrook.exe"
$Asset = "whatsrook-windows-amd64.tar.gz"

$InstallDir = "$env:LOCALAPPDATA\Programs\whatsrook"
$TempArchive = Join-Path $env:TEMP "whatsrook-windows-amd64.tar.gz"
$TempExtract = Join-Path $env:TEMP "whatsrook-install-temp"

Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "Installing WhatsRook for Windows (x64)" -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan

# Create Directories
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
}
if (Test-Path $TempExtract) {
    Remove-Item -Recurse -Force -Path $TempExtract | Out-Null
}
New-Item -ItemType Directory -Force -Path $TempExtract | Out-Null

$DownloadUrl = "https://github.com/$Repo/releases/latest/download/$Asset"
$AlphaUrl = "https://github.com/$Repo/releases/download/alpha/$Asset"

Write-Host "Downloading $Asset..." -ForegroundColor Gray
try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $TempArchive -UseBasicParsing
} catch {
    Write-Host "Latest release download failed. Attempting alpha release..." -ForegroundColor Yellow
    Invoke-WebRequest -Uri $AlphaUrl -OutFile $TempArchive -UseBasicParsing
}

Write-Host "Extracting archive..." -ForegroundColor Gray
if (Get-Command tar.exe -ErrorAction SilentlyContinue) {
    tar.exe -xzf $TempArchive -C $TempExtract
} else {
    # Fallback to .NET extraction if tar is unavailable
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    [System.IO.Compression.ZipFile]::ExtractToDirectory($TempArchive, $TempExtract)
}

$ExtractedBin = Join-Path $TempExtract $BinName
if (-not (Test-Path $ExtractedBin)) {
    Write-Error "Extracted archive did not contain $BinName."
    exit 1
}

# Move binary and assets
Copy-Item -Path $ExtractedBin -Destination (Join-Path $InstallDir $BinName) -Force
if (Test-Path (Join-Path $TempExtract "assets")) {
    Copy-Item -Path (Join-Path $TempExtract "assets") -Destination (Join-Path $InstallDir "assets") -Recurse -Force
}

# Cleanup
Remove-Item -Force -Path $TempArchive -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force -Path $TempExtract -ErrorAction SilentlyContinue

# Ensure in User PATH
$UserPath = [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::User)
if ($UserPath -notlike "*$InstallDir*") {
    Write-Host "Adding $InstallDir to User PATH..." -ForegroundColor Gray
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", [EnvironmentVariableTarget]::User)
    $env:Path = "$env:Path;$InstallDir"
}

Write-Host "============================================================" -ForegroundColor Green
Write-Host "WhatsRook installed successfully!" -ForegroundColor Green
Write-Host "Location: $InstallDir\$BinName" -ForegroundColor Gray
Write-Host ""
Write-Host "Quick Start:" -ForegroundColor Cyan
Write-Host "  Interactive TUI Setup:  whatsrook -i"
Write-Host "  Run Configured Session: whatsrook"
Write-Host "  Show CLI Options:       whatsrook --help"
Write-Host "============================================================" -ForegroundColor Green
