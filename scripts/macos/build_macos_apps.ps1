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

function Build-IcnsFromPng {
    param(
        [Parameter(Mandatory = $true)][string]$SourcePng,
        [Parameter(Mandatory = $true)][string]$IcnsPath
    )

    if (Test-Path $IcnsPath) {
        Remove-Item $IcnsPath -Force
    }

    $pythonScript = @'
import sys
from PIL import Image

src = sys.argv[1]
dst = sys.argv[2]
img = Image.open(src).convert("RGBA")

# Normalize alpha to avoid platform-dependent matte artifacts in .icns previews.
flattened = Image.new("RGBA", img.size, (0, 0, 0, 255))
flattened.alpha_composite(img)
flattened.convert("RGB").save(dst)
'@

    & python3 -c $pythonScript $SourcePng $IcnsPath
}

function Create-AppBundle {
    param(
        [Parameter(Mandatory = $true)][string]$DistDir,
        [Parameter(Mandatory = $true)][string]$AppName,
        [Parameter(Mandatory = $true)][string]$BundleId,
        [Parameter(Mandatory = $true)][string]$ExecutableName,
        [Parameter(Mandatory = $true)][string]$ExecutablePath,
        [Parameter(Mandatory = $true)][string]$IconIcnsPath
    )

    $appDir = Join-Path $DistDir "$AppName.app"
    $contentsDir = Join-Path $appDir "Contents"
    $macosDir = Join-Path $contentsDir "MacOS"
    $resourcesDir = Join-Path $contentsDir "Resources"

    if (Test-Path $appDir) {
        Remove-Item $appDir -Recurse -Force
    }
    New-Item -ItemType Directory -Path $macosDir -Force | Out-Null
    New-Item -ItemType Directory -Path $resourcesDir -Force | Out-Null

    Copy-Item $ExecutablePath (Join-Path $macosDir $ExecutableName) -Force
    Copy-Item $IconIcnsPath (Join-Path $resourcesDir "AppIcon.icns") -Force

    $plist = @"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDevelopmentRegion</key>
    <string>en</string>
    <key>CFBundleExecutable</key>
    <string>$ExecutableName</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>CFBundleIdentifier</key>
    <string>$BundleId</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>$AppName</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>1.0</string>
    <key>CFBundleVersion</key>
    <string>1</string>
    <key>LSMinimumSystemVersion</key>
    <string>11.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
"@
    Set-Content -Path (Join-Path $contentsDir "Info.plist") -Value $plist -Encoding UTF8
}

$rootDir = (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
$distDir = Join-Path $rootDir "dist"

Require-Command "go"
Require-Command "python3"

$goCache = Join-Path $rootDir ".cache/go-build"
New-Item -ItemType Directory -Path $goCache -Force | Out-Null
$env:GOCACHE = $goCache
New-Item -ItemType Directory -Path $distDir -Force | Out-Null

Write-Host "Building darwin/arm64 binaries..."
Invoke-GoBuild -TargetOS "darwin" -TargetArch "arm64" -OutputPath (Join-Path $distDir "perfolizer-darwin-arm64") -PackagePath "./cmd/perfolizer"
Invoke-GoBuild -TargetOS "darwin" -TargetArch "arm64" -OutputPath (Join-Path $distDir "agent-darwin-arm64") -PackagePath "./cmd/agent"

Write-Host "Packaging macOS .app bundles with custom icons..."
$uiIcns = Join-Path $distDir "PerfolizerUI.icns"
$agentIcns = Join-Path $distDir "PerfolizerAgent.icns"
Build-IcnsFromPng -SourcePng (Join-Path $rootDir "assets/icons/perfolizer-ui.png") -IcnsPath $uiIcns
Build-IcnsFromPng -SourcePng (Join-Path $rootDir "assets/icons/perfolizer-agent.png") -IcnsPath $agentIcns

Create-AppBundle `
    -DistDir $distDir `
    -AppName "Perfolizer" `
    -BundleId "com.github.anry88.perfolizer" `
    -ExecutableName "perfolizer" `
    -ExecutablePath (Join-Path $distDir "perfolizer-darwin-arm64") `
    -IconIcnsPath $uiIcns

Create-AppBundle `
    -DistDir $distDir `
    -AppName "Perfolizer Agent" `
    -BundleId "com.github.anry88.perfolizer.agent" `
    -ExecutableName "perfolizer-agent" `
    -ExecutablePath (Join-Path $distDir "agent-darwin-arm64") `
    -IconIcnsPath $agentIcns

Remove-Item $uiIcns, $agentIcns -Force

Write-Host "Done:"
Write-Host ("  UI app:    {0}" -f (Join-Path $distDir "Perfolizer.app"))
Write-Host ("  Agent app: {0}" -f (Join-Path $distDir "Perfolizer Agent.app"))
