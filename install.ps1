$ErrorActionPreference = "Stop"

$Repo = "Aayush-Rajagopalan/gh-purge"
$Binary = "gh-purge.exe"
$AssetName = "gh-purge_windows_amd64.exe"
$InstallDir = "$env:LOCALAPPDATA\Programs\gh-purge"

Write-Host "Fetching latest release..."
$ApiUrl = "https://api.github.com/repos/$Repo/releases/latest"
$Release = Invoke-RestMethod -Uri $ApiUrl -UseBasicParsing
$Asset = $Release.assets | Where-Object { $_.name -eq $AssetName }

if (-not $Asset) {
    Write-Error "Could not find release asset: $AssetName"
    exit 1
}

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir | Out-Null
}

$Dest = Join-Path $InstallDir $Binary
Write-Host "Downloading $AssetName..."
Invoke-WebRequest -Uri $Asset.browser_download_url -OutFile $Dest -UseBasicParsing

# Add to PATH for current user if not already present
$UserPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("PATH", "$UserPath;$InstallDir", "User")
    Write-Host "Added $InstallDir to PATH (restart your terminal to use it)"
}

Write-Host "Installed successfully. Run: gh-purge"
