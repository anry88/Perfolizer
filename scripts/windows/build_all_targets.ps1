$ErrorActionPreference = "Stop"

$scripts = @(
    "../macos/build_macos_apps.ps1",
    "../build_linux_apps.ps1",
    "./build_windows_apps.ps1"
)

$failed = @()

foreach ($scriptName in $scripts) {
    $scriptPath = (Resolve-Path (Join-Path $PSScriptRoot $scriptName)).Path
    Write-Host ("`n=== Running {0} ===" -f $scriptName)
    try {
        & $scriptPath
    }
    catch {
        Write-Warning ("{0} failed: {1}" -f $scriptName, $_.Exception.Message)
        $failed += $scriptName
    }
}

if ($failed.Count -gt 0) {
    Write-Warning ("Completed with failures: {0}" -f ($failed -join ", "))
    exit 1
}

Write-Host "`nAll target build scripts completed."
