$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'release-functions.ps1')

function Assert-Equal($Actual, $Expected) {
    if ($Actual -cne $Expected) { throw "Expected '$Expected', got '$Actual'" }
}
function Assert-Throws([scriptblock]$Run, [string]$Message) {
    try { & $Run } catch {
        if ($_.Exception.Message -notlike "*$Message*") { throw }
        return
    }
    throw "Expected failure containing '$Message'"
}

$tags = @('v0.1.0', 'v0.9.9', 'v0.10.0', 'v0.11.0-nightly.20261110.1.1', 'sdk/v9.0.0')
Assert-Equal (Get-ReleasePlan -Action patch -StableTags $tags).version '0.10.1'
Assert-Equal (Get-ReleasePlan -Action minor -StableTags $tags).version '0.11.0'
Assert-Equal (Get-ReleasePlan -Action major -StableTags $tags).version '1.0.0'
Assert-Equal (Get-ReleasePlan -Action patch).version '0.1.1'
Assert-Equal (Get-ReleasePlan -Action build).publish $false
$nightly = Get-ReleasePlan -Action nightly -StableTags $tags -Date 20261110 -RunNumber 123 -RunAttempt 2
Assert-Equal $nightly.tag 'v0.10.1-nightly.20261110.123.2'
Assert-Equal $nightly.nightly $true
Assert-Equal (Get-ReleasePlan -Action tag -StableTags $tags -Tag v0.11.0).version '0.11.0'
Assert-Throws { Get-ReleasePlan -Action tag -StableTags $tags -Tag v0.1.0 } 'must exceed'
Assert-Throws { Get-ReleasePlan -Action tag -Tag 'v0.1.1+sha' } 'stable vX.Y.Z'
Assert-Throws { Get-ReleasePlan -Action nightly -RunAttempt 0 } 'positive'
Assert-Equal (Get-ReleaseNotesText -Version 0.1.1 -Changelog "## 0.1.1`n`nNew changes`n`n## 0.1.0`nOld changes" -Fallback 'Generated') 'New changes'
Assert-Equal (Get-ReleaseNotesText -Version 0.1.2 -Changelog "## 0.1.1`nChanges" -Fallback 'Generated') 'Generated'

# Exercise the actual publisher with stubbed GitHub/git commands in an isolated
# output tree. No token, network, executable build, or personal data is used.
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('fluent-release-test-' + [guid]::NewGuid().ToString('N'))
$scripts = Join-Path $testRoot 'scripts'
$assets = Join-Path $testRoot 'build/release/windows-amd64'
New-Item -ItemType Directory -Force -Path $scripts, $assets | Out-Null
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'publish-release.ps1') -Destination $scripts
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'prepare-release.ps1') -Destination $scripts
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'release-functions.ps1') -Destination $scripts
$utf8 = [Text.UTF8Encoding]::new($false)
$sha = '1234567890123456789012345678901234567890'
foreach ($name in @('FluentLauncher.exe', 'fluent-launcher-0.10.1-nightly.20261110.123.2-windows-amd64.tar.gz', 'fluent-launcher-0.10.1-windows-amd64.tar.gz', 'update-windows-amd64.json', 'SHA256SUMS')) {
    [IO.File]::WriteAllText((Join-Path $assets $name), 'test', $utf8)
}
[IO.File]::WriteAllText((Join-Path $testRoot 'build/release-notes.md'), 'Notes', $utf8)
[IO.File]::WriteAllText((Join-Path $testRoot 'mygo.json'), '{"version":"0.1.0"}', $utf8)
$nightly | Add-Member -NotePropertyName sha -NotePropertyValue $sha
[IO.File]::WriteAllText((Join-Path $testRoot 'build/release-plan.json'), ($nightly | ConvertTo-Json), $utf8)
$global:releaseTest = @{ Calls = [Collections.Generic.List[string]]::new(); Existing = ''; TagSHA = $sha; TagExists = $false; FailAPI = $false; FailUpload = $false; StableTags = $tags; MainSHA = $sha; HeadSHA = $sha }
function global:gh {
    $global:releaseTest.Calls.Add('gh ' + ($args -join ' '))
    $global:LASTEXITCODE = 0
    if ($args[0] -eq 'api') {
        if ($global:releaseTest.FailAPI) { $global:LASTEXITCODE = 1; return }
        if ($args -contains '--method') { $global:releaseTest.TagExists = $true; return }
        if ($args[-1] -like '*.tag_name') { $global:releaseTest.StableTags }
        elseif ($global:releaseTest.Existing) { $global:releaseTest.Existing }
    }
    if ($args[0] -eq 'release' -and $args[1] -eq 'upload' -and $global:releaseTest.FailUpload) { $global:LASTEXITCODE = 1 }
}
function global:git {
    $global:releaseTest.Calls.Add('git ' + ($args -join ' '))
    $global:LASTEXITCODE = 0
    if ($args[0] -eq 'rev-list') { $global:releaseTest.TagSHA }
    if ($args[0] -eq 'rev-parse') { $global:releaseTest.HeadSHA }
    if ($args[0] -eq 'ls-remote') {
        if ($args[-1] -eq 'refs/heads/main') { "$($global:releaseTest.MainSHA)`trefs/heads/main" }
        elseif ($global:releaseTest.TagExists) { "$($global:releaseTest.TagSHA)`t$($args[-1])" }
    }
    if ($args[0] -eq 'log') { '- Generated commit (1234567)' }
}
$originalEnv = @{}
foreach ($name in @('GITHUB_REF', 'GITHUB_OUTPUT', 'GITHUB_RUN_NUMBER', 'GITHUB_RUN_ATTEMPT')) {
    $originalEnv[$name] = [Environment]::GetEnvironmentVariable($name)
}
try {
    $publisher = Join-Path $scripts 'publish-release.ps1'
    & $publisher -Repository example/launcher
    $calls = $global:releaseTest.Calls -join "`n"
    if ($calls -notmatch 'release create .*--target 1234567890.*--draft.*--prerelease' -or
        $calls -notmatch 'release edit .*--prerelease --latest=false') { throw 'Nightly publishing flags are incorrect' }
    if ($calls -notmatch 'git/refs --method POST --raw-field ref=refs/tags/v0.10.1-nightly.20261110.123.2 --raw-field sha=1234567890') { throw 'The nightly tag was not explicitly created at the build commit' }

    $global:releaseTest.Calls.Clear()
    $global:releaseTest.Existing = '{"id":1,"draft":false}'
    Assert-Throws { & $publisher -Repository example/launcher } 'already published'
    if (($global:releaseTest.Calls -join "`n") -match 'release (upload|edit)') { throw 'Published release was mutated' }

    $global:releaseTest.Existing = '{"id":1,"draft":true}'
    $global:releaseTest.TagSHA = 'different-commit'
    Assert-Throws { & $publisher -Repository example/launcher } 'different commit'

    $global:releaseTest.TagSHA = $sha
    $global:releaseTest.Calls.Clear()
    $global:releaseTest.FailUpload = $true
    Assert-Throws { & $publisher -Repository example/launcher } 'Uploading'
    if (($global:releaseTest.Calls -join "`n") -match 'release edit') { throw 'Failed assets were published' }

    $global:releaseTest.FailUpload = $false
    $global:releaseTest.FailAPI = $true
    Assert-Throws { & $publisher -Repository example/launcher } 'Cannot inspect'

    $global:releaseTest.FailAPI = $false
    $global:releaseTest.Existing = ''
    $global:releaseTest.Calls.Clear()
    $stable = Get-ReleasePlan -Action patch -StableTags $tags
    $stable | Add-Member -NotePropertyName sha -NotePropertyValue $sha
    [IO.File]::WriteAllText((Join-Path $testRoot 'build/release-plan.json'), ($stable | ConvertTo-Json), $utf8)
    & $publisher -Repository example/launcher
    if (($global:releaseTest.Calls -join "`n") -notmatch 'release edit .*--prerelease=false --latest') { throw 'Stable release was not marked latest' }

    $prepare = Join-Path $scripts 'prepare-release.ps1'
    $env:GITHUB_REF = 'refs/heads/feature'
    Assert-Throws { & $prepare -Action patch -Repository example/launcher } 'from main'
    $env:GITHUB_REF = 'refs/heads/main'
    $env:GITHUB_OUTPUT = Join-Path $testRoot 'outputs.txt'
    $env:GITHUB_RUN_NUMBER = '123'
    $env:GITHUB_RUN_ATTEMPT = '2'
    $global:releaseTest.FailAPI = $true
    Assert-Throws { & $prepare -Action patch -Repository example/launcher } 'Cannot read published versions'
    $global:releaseTest.FailAPI = $false
    & $prepare -Action nightly -Repository example/launcher
    $prepared = Get-Content -Raw (Join-Path $testRoot 'build/release-plan.json') | ConvertFrom-Json
    Assert-Equal $prepared.version "0.10.1-nightly.$([DateTime]::UtcNow.ToString('yyyyMMdd')).123.2"
    Assert-Equal $prepared.sha $sha
    $notes = Get-Content -Raw (Join-Path $testRoot 'build/update-changelog.md')
    if (-not $notes.StartsWith("## $($prepared.version)`n") -or $notes -notmatch 'Generated commit') { throw 'Generated update notes do not match the release' }
    $global:releaseTest.MainSHA = 'newer-main'
    & $prepare -Action nightly -Repository example/launcher
    if ((Get-Content -Raw $env:GITHUB_OUTPUT) -notmatch 'skip=true') { throw 'An outdated nightly was not skipped' }
} finally {
    foreach ($name in $originalEnv.Keys) { [Environment]::SetEnvironmentVariable($name, $originalEnv[$name]) }
    Remove-Item Function:\gh, Function:\git
    Remove-Variable releaseTest -Scope Global
    $resolved = [IO.Path]::GetFullPath($testRoot)
    $tempPrefix = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\fluent-release-test-'
    if (-not $resolved.StartsWith($tempPrefix, [StringComparison]::OrdinalIgnoreCase)) { throw 'Test cleanup path outside temporary workspace' }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
Write-Output 'Release planning and publishing guard tests passed.'
