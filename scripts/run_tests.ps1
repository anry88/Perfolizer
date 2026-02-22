$ErrorActionPreference = "Stop"

function Require-Command {
    param([Parameter(Mandatory = $true)][string]$Name)
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "required command not found: $Name"
    }
}

$rootDir = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$coverageOut = if ($env:PERFOLIZER_COVERAGE_OUT) {
    $env:PERFOLIZER_COVERAGE_OUT
}
else {
    Join-Path $rootDir "dist/coverage.out"
}

Require-Command "go"

New-Item -ItemType Directory -Path (Join-Path $rootDir ".cache/go-build") -Force | Out-Null
New-Item -ItemType Directory -Path (Split-Path -Parent $coverageOut) -Force | Out-Null
$env:GOCACHE = Join-Path $rootDir ".cache/go-build"

Write-Host "Running test suite..."
& go test ./... "-covermode=atomic" "-coverpkg=./..." "-coverprofile=$coverageOut"

Write-Host "`nCoverage summary:"
& go tool cover "-func=$coverageOut"

Write-Host ("`nCoverage profile: {0}" -f $coverageOut)
