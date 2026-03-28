Param(
    [ValidateSet("up", "down", "logs", "ps", "restart", "health")]
    [string]$Action = "up"
)

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RootDir = (Resolve-Path (Join-Path $ScriptDir "..\..")).Path
$ComposeFile = Join-Path $RootDir "compose.yaml"
$ServiceName = "perfolizer-agent"

function Invoke-Compose {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Args)
    & docker compose -f $ComposeFile @Args
}

switch ($Action) {
    "up" {
        Invoke-Compose up --build -d $ServiceName
        Write-Host "Perfolizer agent is available at http://127.0.0.1:9090"
        Invoke-Compose ps $ServiceName
    }
    "down" {
        Invoke-Compose down
    }
    "logs" {
        Invoke-Compose logs -f $ServiceName
    }
    "ps" {
        Invoke-Compose ps $ServiceName
    }
    "restart" {
        Invoke-Compose restart $ServiceName
    }
    "health" {
        (Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:9090/healthz").Content
    }
}
