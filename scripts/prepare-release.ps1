param(
    [Parameter(Mandatory)][ValidateSet('build', 'nightly', 'patch', 'minor', 'major', 'tag')][string]$Action,
    [string]$Tag = '',
    [string]$Repository = $env:GITHUB_REPOSITORY
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'release-functions.ps1')
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
try {
    # Automatic publishing only runs from main in the owning repository.
    if ($Action -notin @('build', 'tag') -and $env:GITHUB_REF -ne 'refs/heads/main') {
        throw 'Automatic releases must run from main'
    }
    if ($Action -eq 'nightly') {
        $head = git rev-parse HEAD
        if ($LASTEXITCODE -ne 0) { throw 'Cannot determine the nightly commit' }
        $remoteHead = git ls-remote origin refs/heads/main
        if ($LASTEXITCODE -ne 0 -or -not $remoteHead) { throw 'Cannot determine the current main commit' }
        if ($head -ne ($remoteHead -split '\s+')[0]) {
            Write-Output 'Skipping an outdated main build; the newer push will publish nightly.'
            if ($env:GITHUB_OUTPUT) { Add-Content -LiteralPath $env:GITHUB_OUTPUT -Value 'skip=true' -Encoding utf8 }
            return
        }
    }
    $stableTags = @()
    if ($Action -ne 'build') {
        $stableTags = @(gh api --paginate "repos/$Repository/releases?per_page=100" --jq '.[] | select(.draft == false and .prerelease == false) | .tag_name')
        if ($LASTEXITCODE -ne 0) { throw 'Cannot read published versions; refusing to guess a release version' }
    }
    $config = Get-Content -Raw mygo.json | ConvertFrom-Json
    $argsForPlan = @{
        Action = $Action; Tag = $Tag; StableTags = $stableTags; InitialVersion = $config.version
    }
    if ($Action -eq 'nightly') {
        $argsForPlan.RunNumber = [long]$env:GITHUB_RUN_NUMBER
        $argsForPlan.RunAttempt = [long]$env:GITHUB_RUN_ATTEMPT
    }
    $plan = Get-ReleasePlan @argsForPlan
    $sha = git rev-parse HEAD
    if ($LASTEXITCODE -ne 0) { throw 'Cannot determine the build commit' }
    $plan | Add-Member -NotePropertyName sha -NotePropertyValue $sha
    New-Item -ItemType Directory -Force -Path build | Out-Null
    $utf8 = [Text.UTF8Encoding]::new($false)
    [IO.File]::WriteAllText((Join-Path $projectRoot 'build/release-plan.json'), ($plan | ConvertTo-Json) + "`n", $utf8)
    if ($plan.publish) {
        $range = if ($plan.previousTag) { "$($plan.previousTag)..$sha" } else { $sha }
        $commits = @(git log $range --no-merges -n 50 '--pretty=format:- %s (%h)')
        if ($LASTEXITCODE -ne 0) { throw 'Cannot generate release notes' }
        $fallback = ($commits -join "`n") + "`n`nCommit: https://github.com/$Repository/commit/$sha"
        if ($plan.nightly) { $fallback = "Nightly build from main. This is a preview release.`n`n" + $fallback }
        $changelog = if (Test-Path -LiteralPath CHANGELOG.md) { Get-Content -Raw CHANGELOG.md } else { '' }
        $notes = Get-ReleaseNotesText -Version $plan.version -Changelog $changelog -Fallback $fallback
        [IO.File]::WriteAllText((Join-Path $projectRoot 'build/release-notes.md'), $notes + "`n", $utf8)
        # MyGo requires an exact version section. Both the update card and the
        # GitHub release use these same notes, even without a changelog edit.
        [IO.File]::WriteAllText((Join-Path $projectRoot 'build/update-changelog.md'), "## $($plan.version)`n`n$notes`n", $utf8)
    }
    if ($env:GITHUB_OUTPUT) {
        @("tag=$($plan.tag)", "publish=$($plan.publish.ToString().ToLowerInvariant())", "nightly=$($plan.nightly.ToString().ToLowerInvariant())") |
            Add-Content -LiteralPath $env:GITHUB_OUTPUT -Encoding utf8
    }
    Write-Output "Release plan: $Action $($plan.version) at $sha"
} finally {
    Pop-Location
}
