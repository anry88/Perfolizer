$ErrorActionPreference = "Stop"

$scripts = @(
    "../macos/build_macos_apps.ps1",
    "../build_linux_apps.ps1",
    "./build_windows_apps.ps1"
)

$failed = @()
$rootDir = (Resolve-Path (Join-Path $PSScriptRoot "../..")).Path
$skipTests = ($env:PERFOLIZER_SKIP_TESTS -eq "1")

if ($skipTests) {
    Write-Host "Skipping pre-build tests (PERFOLIZER_SKIP_TESTS=1)."
}
else {
    Write-Host "=== Running test suite before multi-target build ==="
    & (Join-Path $rootDir "scripts/run_tests.ps1")
}

$previousSkip = $env:PERFOLIZER_SKIP_TESTS
$env:PERFOLIZER_SKIP_TESTS = "1"

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

if ($null -eq $previousSkip) {
    Remove-Item Env:PERFOLIZER_SKIP_TESTS -ErrorAction SilentlyContinue
}
else {
    $env:PERFOLIZER_SKIP_TESTS = $previousSkip
}

if ($failed.Count -gt 0) {
    Write-Warning ("Completed with failures: {0}" -f ($failed -join ", "))
    exit 1
}

Write-Host "`nAll target build scripts completed."
