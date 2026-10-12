[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# Exercise the real build script with tiny command stubs. No Go installation,
# compiler, network access, or changes to the machine's PATH are required.
if (-not $env:WINDIR) { throw 'This regression test requires Windows.' }
$temporaryDirectory = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')
$testDirectory = Join-Path $temporaryDirectory ('GoDCP-build-test-' + [guid]::NewGuid().ToString('N'))
$previousPath = $env:PATH
$previousCgo = [Environment]::GetEnvironmentVariable('CGO_ENABLED', 'Process')
try {
    $fixtureScripts = Join-Path $testDirectory 'project\scripts'
    $firstToolchain = Join-Path $testDirectory 'first toolchain'
    $secondToolchain = Join-Path $testDirectory 'second toolchain'
    New-Item -ItemType Directory -Path $fixtureScripts, $firstToolchain, $secondToolchain | Out-Null
    $buildScript = Join-Path $fixtureScripts 'build.ps1'
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'build.ps1') -Destination $buildScript
    $stub = @'
@echo off
if not "%CGO_ENABLED%"=="1" exit /b 10
>>"%~dp0calls.log" echo %*
if "%~1"=="env" (
    echo {"GOOS":"windows","GOHOSTOS":"windows","GOARCH":"amd64","GOHOSTARCH":"amd64"}
    exit /b 0
)
if "%~1"=="version" (
    echo go version fixture windows/amd64
    exit /b 0
)
if "%~1"=="test" exit /b 0
if "%~1"=="build" (
    >"%~6" echo regression fixture
    exit /b 0
)
exit /b 11
'@
    foreach ($directory in @($firstToolchain, $secondToolchain)) {
        Set-Content -LiteralPath (Join-Path $directory 'go.cmd') -Value $stub -Encoding Ascii
    }
    $env:PATH = "$firstToolchain;$secondToolchain;$previousPath"
    $env:CGO_ENABLED = '0'
    $candidates = @(Get-Command -Name go -CommandType Application)
    if ($candidates.Count -lt 2 -or $candidates[0].Source -ne (Join-Path $firstToolchain 'go.cmd') -or
        $candidates[1].Source -ne (Join-Path $secondToolchain 'go.cmd')) {
        throw 'The fixture did not expose both Go candidates in PATH order.'
    }

    function Assert-BuildCalls([string]$Directory, [int]$ExpectedTests) {
        $calls = @(Get-Content -LiteralPath (Join-Path $Directory 'calls.log'))
        foreach ($command in @('env', 'version', 'build')) {
            if (@($calls | Where-Object { $_ -match ('^' + $command + '( |$)') }).Count -ne 1) {
                throw "Expected exactly one $command invocation in $Directory."
            }
        }
        if (@($calls | Where-Object { $_ -match '^test( |$)' }).Count -ne $ExpectedTests) {
            throw "Unexpected test invocations in $Directory."
        }
        if ($env:CGO_ENABLED -ne '0') { throw 'The build script did not restore CGO_ENABLED.' }
        if (-not (Test-Path -LiteralPath (Join-Path $testDirectory 'project\Build\GoDCP.exe'))) {
            throw 'The build script did not pass the expected Windows output path.'
        }
        Remove-Item -LiteralPath (Join-Path $Directory 'calls.log')
    }

    # GitHub Windows runners can have several Go installations in PATH.
    & $buildScript
    Assert-BuildCalls $firstToolchain 1
    if (Test-Path -LiteralPath (Join-Path $secondToolchain 'calls.log')) {
        throw 'Automatic selection invoked the second Go installation.'
    }

    # A command name passed explicitly still resolves in PATH order.
    & $buildScript -Go go -SkipTests
    Assert-BuildCalls $firstToolchain 0
    if (Test-Path -LiteralPath (Join-Path $secondToolchain 'calls.log')) {
        throw 'Explicit command-name selection invoked the second Go installation.'
    }

    # An explicit path must override PATH, including a directory with spaces.
    & $buildScript -Go (Join-Path $secondToolchain 'go.cmd')
    Assert-BuildCalls $secondToolchain 1
    if (Test-Path -LiteralPath (Join-Path $firstToolchain 'calls.log')) {
        throw 'Explicit path selection invoked the first Go installation.'
    }
    Write-Host 'PASS: PATH order, explicit Go name/path, SkipTests, output path, and CGO restoration.'
} finally {
    $env:PATH = $previousPath
    [Environment]::SetEnvironmentVariable('CGO_ENABLED', $previousCgo, 'Process')
    if (Test-Path -LiteralPath $testDirectory) {
        $cleanupDirectory = (Get-Item -LiteralPath $testDirectory).FullName
        if ($cleanupDirectory -ne $testDirectory -or (Split-Path -Parent $cleanupDirectory) -ne $temporaryDirectory) {
            throw 'Refusing to remove a regression-test directory outside its temporary parent.'
        }
        Remove-Item -LiteralPath $cleanupDirectory -Recurse -Force
    }
}
