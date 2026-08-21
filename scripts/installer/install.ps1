# WhatsRook Windows PowerShell Installer
$ErrorActionPreference = "Stop"

$Repo = "Thruqe/whatsrook"
$BinName = "whatsrook.exe"
$InstallDir = "$env:USERPROFILE\.whatsrook\bin"

# 1. Detect Architecture
$Arch = "amd64"
if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") {
    $Arch = "arm64"
}

Write-Host "==> Installing WhatsRook for Windows ($Arch)..." -ForegroundColor Cyan

$AssetName = "whatsrook-windows-$Arch.tar.gz"
$DownloadUrl = "https://github.com/$Repo/releases/latest/download/$AssetName"
$TempDir = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), [System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $TempDir -Force | Out-Null

try {
    $ArchiveFile = Join-Path $TempDir $AssetName
    Write-Host "==> Downloading $DownloadUrl..." -ForegroundColor Cyan

    if (Get-Command curl.exe -ErrorAction SilentlyContinue) {
        & curl.exe -fsSL "$DownloadUrl" -o "$ArchiveFile"
    } else {
        Invoke-WebRequest -Uri $DownloadUrl -OutFile $ArchiveFile -UseBasicParsing
    }

    Write-Host "==> Extracting archive..." -ForegroundColor Cyan
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }

    # Extract tar.gz (Windows 10+ has tar.exe built-in)
    if (Get-Command tar.exe -ErrorAction SilentlyContinue) {
        & tar.exe -xzf "$ArchiveFile" -C "$TempDir"
    } else {
        throw "tar.exe was not found on your system to extract $AssetName."
    }

    # Locate executable
    $FoundBin = Get-ChildItem -Path $TempDir -Filter "whatsrook*" -Recurse -File | Where-Object { $_.Name -like "*.exe" -or $_.Name -eq "whatsrook" } | Select-Object -First 1
    if (-not $FoundBin) {
        throw "Binary whatsrook.exe not found in downloaded release archive."
    }

    $TargetExe = Join-Path $InstallDir $BinName
    Move-Item -Path $FoundBin.FullName -Destination $TargetExe -Force

    # 2. Add to User PATH environment variable
    $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($UserPath -notlike "*$InstallDir*") {
        $NewPath = "$InstallDir;$UserPath"
        [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
        $env:Path = "$InstallDir;$env:Path"
        Write-Host "✓ Added $InstallDir to User PATH environment variable." -ForegroundColor Green
    }

    Write-Host ""
    Write-Host "🎉 WhatsRook installed successfully to $TargetExe" -ForegroundColor Green
    Write-Host "Restart your PowerShell / Command Prompt terminal, then run:" -ForegroundColor Yellow
    Write-Host "  whatsrook -h" -ForegroundColor White
}
finally {
    if (Test-Path $TempDir) {
        Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
