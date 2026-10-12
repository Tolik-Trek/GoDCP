[CmdletBinding()]
param(
    [string]$Go,
    [switch]$SkipTests
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$goExecutable = $null
if ($Go) {
    $goExecutable = (Get-Command -Name $Go -CommandType Application -ErrorAction Stop).Source
} else {
    $goCommand = Get-Command -Name go -CommandType Application -ErrorAction SilentlyContinue
    if ($goCommand) {
        $goExecutable = $goCommand.Source
    } elseif ($env:LOCALAPPDATA) {
        $portablePattern = Join-Path $env:LOCALAPPDATA 'GoDCP-tools\go*\go\bin\go.exe'
        $portableGo = Get-ChildItem -Path $portablePattern -File -ErrorAction SilentlyContinue |
            Where-Object { $_.Directory.Parent.Parent.Name -match '^go\d+\.\d+\.\d+$' } |
            Sort-Object { [version]($_.Directory.Parent.Parent.Name -replace '^go', '') } -Descending |
            Select-Object -First 1
        if ($portableGo) {
            $goExecutable = $portableGo.FullName
        }
    }
}
if (-not $goExecutable) {
    throw 'Go was not found. Install Go, add it to PATH, or pass -Go C:\path\to\go.exe.'
}

$projectDirectory = Split-Path -Parent $PSScriptRoot
$previousCgo = [Environment]::GetEnvironmentVariable('CGO_ENABLED', 'Process')
Push-Location -LiteralPath $projectDirectory
try {
    $env:CGO_ENABLED = '1'
    $goEnvironmentText = & $goExecutable env -json GOOS GOHOSTOS GOARCH GOHOSTARCH
    if ($LASTEXITCODE -ne 0) { throw 'Unable to read Go environment.' }
    $goEnvironment = $goEnvironmentText | ConvertFrom-Json
    if ($goEnvironment.GOOS -ne $goEnvironment.GOHOSTOS -or
        $goEnvironment.GOARCH -ne $goEnvironment.GOHOSTARCH) {
        throw 'This script builds for the current OS and architecture. Clear GOOS/GOARCH overrides; cross-compilation needs a separate C toolchain.'
    }

    & $goExecutable version
    if ($LASTEXITCODE -ne 0) { throw 'Unable to run Go.' }
    if (-not $SkipTests) {
        & $goExecutable test ./...
        if ($LASTEXITCODE -ne 0) { throw 'Go tests failed; build stopped.' }
    }

    New-Item -ItemType Directory -Path (Join-Path $projectDirectory 'Build') -Force | Out-Null
    $outputPath = Join-Path $projectDirectory 'Build/GoDCP'
    $linkFlags = '-s -w'
    if ($goEnvironment.GOOS -eq 'windows') {
        $outputPath += '.exe'
        $linkFlags += ' -H=windowsgui'
    }
    & $goExecutable build -trimpath -ldflags $linkFlags -o $outputPath ./src
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed.' }
    Write-Host "Built $outputPath"
} finally {
    [Environment]::SetEnvironmentVariable('CGO_ENABLED', $previousCgo, 'Process')
    Pop-Location
}
