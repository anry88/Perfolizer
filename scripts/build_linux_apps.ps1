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

function Build-LinuxBundle {
    param(
        [Parameter(Mandatory = $true)][string]$DistDir,
        [Parameter(Mandatory = $true)][string]$AppDirName,
        [Parameter(Mandatory = $true)][string]$BinName,
        [Parameter(Mandatory = $true)][string]$SourcePkg,
        [Parameter(Mandatory = $true)][string]$IconPng,
        [Parameter(Mandatory = $true)][string]$AppTitle,
        [Parameter(Mandatory = $true)][string]$DesktopName,
        [Parameter(Mandatory = $true)][string]$Comment,
        [Parameter(Mandatory = $true)][string]$ArchiveName
    )

    $appDir = Join-Path $DistDir $AppDirName
    $archivePath = Join-Path $DistDir $ArchiveName

    if (Test-Path $appDir) { Remove-Item $appDir -Recurse -Force }
    if (Test-Path $archivePath) { Remove-Item $archivePath -Force }
    New-Item -ItemType Directory -Path (Join-Path $appDir "bin") -Force | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $appDir "icons") -Force | Out-Null

    try {
        Invoke-GoBuild -TargetOS "linux" -TargetArch "amd64" -OutputPath (Join-Path $appDir "bin/$BinName") -PackagePath $SourcePkg
    }
    catch {
        Write-Warning "failed to build $SourcePkg for linux/amd64"
        Write-Warning "UI build usually requires Linux host toolchain for Fyne/CGO"
        if (Test-Path $appDir) { Remove-Item $appDir -Recurse -Force }
        return $false
    }

    Copy-Item $IconPng (Join-Path $appDir "icons/app-icon.png") -Force

    $desktopTemplate = @'
[Desktop Entry]
Type=Application
Version=1.0
Name=__NAME__
Comment=__COMMENT__
Exec=${APPDIR}/bin/__BIN__
Icon=${APPDIR}/icons/app-icon.png
Terminal=false
Categories=Development;Utility;
'@

    $desktopContent = $desktopTemplate.Replace("__NAME__", $AppTitle).Replace("__COMMENT__", $Comment).Replace("__BIN__", $BinName)
    Set-Content -Path (Join-Path $appDir $DesktopName) -Value $desktopContent -Encoding UTF8

    & tar -C $DistDir -czf $archivePath $AppDirName
    return $true
}

$rootDir = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$distDir = Join-Path $rootDir "dist/linux"

Require-Command "go"
Require-Command "tar"

$goCache = Join-Path $rootDir ".cache/go-build"
New-Item -ItemType Directory -Path $goCache -Force | Out-Null
$env:GOCACHE = $goCache
New-Item -ItemType Directory -Path $distDir -Force | Out-Null

Write-Host "Building Linux bundles..."

$uiBuilt = Build-LinuxBundle `
    -DistDir $distDir `
    -AppDirName "Perfolizer-linux-amd64" `
    -BinName "perfolizer" `
    -SourcePkg "./cmd/perfolizer" `
    -IconPng (Join-Path $rootDir "assets/icons/perfolizer-ui.png") `
    -AppTitle "Perfolizer" `
    -DesktopName "perfolizer.desktop" `
    -Comment "Perfolizer UI application" `
    -ArchiveName "Perfolizer-linux-amd64.tar.gz"

if (-not $uiBuilt) {
    Write-Host "UI Linux bundle skipped in this environment."
}

$agentBuilt = Build-LinuxBundle `
    -DistDir $distDir `
    -AppDirName "Perfolizer-Agent-linux-amd64" `
    -BinName "perfolizer-agent" `
    -SourcePkg "./cmd/agent" `
    -IconPng (Join-Path $rootDir "assets/icons/perfolizer-agent.png") `
    -AppTitle "Perfolizer Agent" `
    -DesktopName "perfolizer-agent.desktop" `
    -Comment "Perfolizer Agent service" `
    -ArchiveName "Perfolizer-Agent-linux-amd64.tar.gz"

if (-not $agentBuilt) {
    Write-Host "Agent Linux bundle skipped in this environment."
}

Write-Host "Done:"
if ($uiBuilt) {
    Write-Host ("  {0}" -f (Join-Path $distDir "Perfolizer-linux-amd64"))
}
if ($agentBuilt) {
    Write-Host ("  {0}" -f (Join-Path $distDir "Perfolizer-Agent-linux-amd64"))
}
Write-Host "  archives:"
if ($uiBuilt) {
    Write-Host ("    {0}" -f (Join-Path $distDir "Perfolizer-linux-amd64.tar.gz"))
}
if ($agentBuilt) {
    Write-Host ("    {0}" -f (Join-Path $distDir "Perfolizer-Agent-linux-amd64.tar.gz"))
}
