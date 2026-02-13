$ErrorActionPreference = "Stop"

function Require-Command {
    param([Parameter(Mandatory = $true)][string]$Name)
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "required command not found: $Name"
    }
}

function Invoke-GoBuild {
    param(
        [Parameter(Mandatory = $true)][string]$TargetOS,
        [Parameter(Mandatory = $true)][string]$TargetArch,
        [Parameter(Mandatory = $true)][string]$OutputPath,
        [Parameter(Mandatory = $true)][string]$PackagePath
    )

    $prevGOOS = $env:GOOS
    $prevGOARCH = $env:GOARCH
    try {
        $env:GOOS = $TargetOS
        $env:GOARCH = $TargetArch
        & go build -o $OutputPath $PackagePath
    }
    finally {
        if ($null -eq $prevGOOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $prevGOOS }
        if ($null -eq $prevGOARCH) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $prevGOARCH }
    }
}

function Build-WindowsBundle {
    param(
        [Parameter(Mandatory = $true)][string]$DistDir,
        [Parameter(Mandatory = $true)][string]$AppDirName,
        [Parameter(Mandatory = $true)][string]$ExeName,
        [Parameter(Mandatory = $true)][string]$SourcePkg,
        [Parameter(Mandatory = $true)][string]$IconPng,
        [Parameter(Mandatory = $true)][string]$IconIcoName,
        [Parameter(Mandatory = $true)][string]$ArchiveName,
        [Parameter(Mandatory = $true)][string]$ShortcutPs1Name,
        [Parameter(Mandatory = $true)][string]$ShortcutName
    )

    $appDir = Join-Path $DistDir $AppDirName
    $archivePath = Join-Path $DistDir $ArchiveName

    if (Test-Path $appDir) { Remove-Item $appDir -Recurse -Force }
    if (Test-Path $archivePath) { Remove-Item $archivePath -Force }
    New-Item -ItemType Directory -Path (Join-Path $appDir "icons") -Force | Out-Null

    try {
        Invoke-GoBuild -TargetOS "windows" -TargetArch "amd64" -OutputPath (Join-Path $appDir $ExeName) -PackagePath $SourcePkg
    }
    catch {
        Write-Warning "failed to build $SourcePkg for windows/amd64"
        Write-Warning "UI build may require Windows-native toolchain for Fyne/CGO"
        if (Test-Path $appDir) { Remove-Item $appDir -Recurse -Force }
        return $false
    }

    $pythonScript = @'
import sys
from PIL import Image

src = sys.argv[1]
dst = sys.argv[2]
img = Image.open(src).convert("RGBA")
img.save(dst, sizes=[(16, 16), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)])
'@
    & python3 -c $pythonScript $IconPng (Join-Path $appDir "icons/$IconIcoName")

    $shortcutTemplate = @'
$WshShell = New-Object -ComObject WScript.Shell
$DesktopPath = [Environment]::GetFolderPath("Desktop")
$Shortcut = $WshShell.CreateShortcut((Join-Path $DesktopPath "__SHORTCUT_NAME__"))
$Shortcut.TargetPath = (Join-Path $PSScriptRoot "__EXE_NAME__")
$Shortcut.IconLocation = (Join-Path $PSScriptRoot "icons\__ICON_NAME__")
$Shortcut.WorkingDirectory = $PSScriptRoot
$Shortcut.Save()
Write-Host "Shortcut created: $DesktopPath\__SHORTCUT_NAME__"
'@
    $shortcutContent = $shortcutTemplate.Replace("__SHORTCUT_NAME__", $ShortcutName).Replace("__EXE_NAME__", $ExeName).Replace("__ICON_NAME__", $IconIcoName)
    Set-Content -Path (Join-Path $appDir $ShortcutPs1Name) -Value $shortcutContent -Encoding UTF8

    Compress-Archive -Path $appDir -DestinationPath $archivePath -Force
    return $true
}

$rootDir = (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
$distDir = Join-Path $rootDir "dist/windows"

Require-Command "go"
Require-Command "python3"

$goCache = Join-Path $rootDir ".cache/go-build"
New-Item -ItemType Directory -Path $goCache -Force | Out-Null
$env:GOCACHE = $goCache
New-Item -ItemType Directory -Path $distDir -Force | Out-Null

Write-Host "Building Windows bundles..."

$uiBuilt = Build-WindowsBundle `
    -DistDir $distDir `
    -AppDirName "Perfolizer-windows-amd64" `
    -ExeName "perfolizer.exe" `
    -SourcePkg "./cmd/perfolizer" `
    -IconPng (Join-Path $rootDir "assets/icons/perfolizer-ui.png") `
    -IconIcoName "perfolizer-ui.ico" `
    -ArchiveName "Perfolizer-windows-amd64.zip" `
    -ShortcutPs1Name "create-desktop-shortcut.ps1" `
    -ShortcutName "Perfolizer.lnk"

if (-not $uiBuilt) {
    Write-Host "UI Windows bundle skipped in this environment."
}

$agentBuilt = Build-WindowsBundle `
    -DistDir $distDir `
    -AppDirName "Perfolizer-Agent-windows-amd64" `
    -ExeName "perfolizer-agent.exe" `
    -SourcePkg "./cmd/agent" `
    -IconPng (Join-Path $rootDir "assets/icons/perfolizer-agent.png") `
    -IconIcoName "perfolizer-agent.ico" `
    -ArchiveName "Perfolizer-Agent-windows-amd64.zip" `
    -ShortcutPs1Name "create-desktop-shortcut.ps1" `
    -ShortcutName "Perfolizer Agent.lnk"

if (-not $agentBuilt) {
    Write-Host "Agent Windows bundle skipped in this environment."
}

Write-Host "Done:"
if ($uiBuilt) {
    Write-Host ("  {0}" -f (Join-Path $distDir "Perfolizer-windows-amd64"))
}
if ($agentBuilt) {
    Write-Host ("  {0}" -f (Join-Path $distDir "Perfolizer-Agent-windows-amd64"))
}
Write-Host "  archives:"
if ($uiBuilt) {
    Write-Host ("    {0}" -f (Join-Path $distDir "Perfolizer-windows-amd64.zip"))
}
if ($agentBuilt) {
    Write-Host ("    {0}" -f (Join-Path $distDir "Perfolizer-Agent-windows-amd64.zip"))
}
