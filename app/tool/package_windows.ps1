# Package a built Windows desktop app (`flutter build windows --release`) as:
#   out/gpix_<ver>_windows-x64.zip         portable; unzip and run gpix_app.exe
#   out/gpix_<ver>_windows-x64-setup.exe   installer with Start-menu shortcut
#
# Usage (from app/):  pwsh tool/package_windows.ps1 -Version v0.1.0
# Needs Inno Setup 6.3+ for the installer (choco install innosetup).
param([string]$Version = "dev")

$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

$bundle = "build\windows\x64\runner\Release"
if (-not (Test-Path "$bundle\gpix_app.exe")) {
    throw "No release build at $bundle - run 'flutter build windows --release' first."
}

# Flutter apps need the MSVC runtime; ship it so the app starts on machines
# without the Visual C++ Redistributable installed.
foreach ($dll in "msvcp140.dll", "vcruntime140.dll", "vcruntime140_1.dll") {
    $src = Join-Path $env:WINDIR "System32\$dll"
    if (Test-Path $src) { Copy-Item $src $bundle -Force }
    else { Write-Warning "$dll not found; the app may need the VC++ Redistributable." }
}

New-Item -ItemType Directory -Force out | Out-Null
$outDir = (Resolve-Path out).Path

$zip = Join-Path $outDir "gpix_${Version}_windows-x64.zip"
Compress-Archive -Path "$bundle\*" -DestinationPath $zip -Force
Write-Host "created $zip"

$iscc = (Get-Command iscc.exe -ErrorAction SilentlyContinue).Source
if (-not $iscc) {
    foreach ($p in "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe", "$env:ProgramFiles\Inno Setup 6\ISCC.exe") {
        if (Test-Path $p) { $iscc = $p; break }
    }
}
if (-not $iscc) { throw "Inno Setup 6 (ISCC.exe) not found. Install it with: choco install innosetup" }

$appVersion = $Version.TrimStart("v")
if ($appVersion -notmatch '^\d') { $appVersion = "0.0.0" }

& $iscc `
    "/DAppVersion=$appVersion" `
    "/DBundleDir=$((Resolve-Path $bundle).Path)" `
    "/DIconFile=$((Resolve-Path 'windows\runner\resources\app_icon.ico').Path)" `
    "/DOutDir=$outDir" `
    "/DOutName=gpix_${Version}_windows-x64-setup" `
    "packaging\windows\gpix.iss"
if ($LASTEXITCODE -ne 0) { throw "ISCC failed with exit code $LASTEXITCODE" }
Write-Host "created $outDir\gpix_${Version}_windows-x64-setup.exe"
