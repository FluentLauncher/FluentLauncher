param([string]$Repository = $env:GITHUB_REPOSITORY)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
try {
    $plan = Get-Content -Raw build/release-plan.json | ConvertFrom-Json
    if (-not $plan.publish) { throw 'The release plan does not authorize publishing' }
    $tag = $plan.tag
    $files = @('FluentLauncher.exe', "fluent-launcher-$($plan.version)-windows-amd64.tar.gz", 'update-windows-amd64.json', 'SHA256SUMS') |
        ForEach-Object { Join-Path $projectRoot "build/release/windows-amd64/$_" }
    foreach ($file in $files) {
        if (-not (Test-Path -LiteralPath $file -PathType Leaf)) { throw "Missing release asset: $file" }
    }
    # Query the list so network/authentication errors cannot be mistaken for
    # an absent release. Published assets are never replaced.
    $existing = @(gh api --paginate "repos/$Repository/releases?per_page=100" --jq ".[] | select(.tag_name == `"$tag`") | {id, draft}")
    if ($LASTEXITCODE -ne 0) { throw 'Cannot inspect existing releases' }
    if ($existing.Count -gt 0) {
        $release = ($existing -join "`n") | ConvertFrom-Json
        if (-not $release.draft) { throw 'Refusing to replace an already published release' }
    }
    # A draft need not create its Git tag until publication. Create the ref
    # explicitly first, using GITHUB_TOKEN so it cannot trigger another run.
    $remoteTag = @(git ls-remote origin "refs/tags/$tag")
    if ($LASTEXITCODE -ne 0) { throw 'Cannot inspect the release tag' }
    if ($remoteTag.Count -eq 0) {
        if ($plan.action -eq 'tag') { throw 'The manually pushed release tag is missing' }
        gh api "repos/$Repository/git/refs" --method POST --raw-field "ref=refs/tags/$tag" --raw-field "sha=$($plan.sha)" --silent
        if ($LASTEXITCODE -ne 0) { throw 'Creating the release tag failed' }
    }
    # Check the actual tag, including when resuming a draft left by a failed
    # run. --target alone does not override an already existing tag.
    git fetch origin "refs/tags/${tag}:refs/tags/${tag}"
    if ($LASTEXITCODE -ne 0) { throw 'Cannot resolve the release tag' }
    $tagSHA = git rev-list -n 1 $tag
    if ($LASTEXITCODE -ne 0 -or $tagSHA -ne $plan.sha) { throw 'Release tag points at a different commit' }
    if ($existing.Count -eq 0) {
        $createArgs = @('release', 'create', $tag, '--repo', $Repository, '--verify-tag', '--target', $plan.sha, '--draft', '--title', $tag, '--notes-file', 'build/release-notes.md')
        if ($plan.nightly) { $createArgs += '--prerelease' }
        gh @createArgs
        if ($LASTEXITCODE -ne 0) { throw 'Creating draft release failed' }
    }
    gh release upload $tag --repo $Repository --clobber @files
    if ($LASTEXITCODE -ne 0) { throw 'Uploading release assets failed' }
    $editArgs = @('release', 'edit', $tag, '--repo', $Repository, '--draft=false', '--notes-file', 'build/release-notes.md')
    if ($plan.nightly) { $editArgs += @('--prerelease', '--latest=false') }
    else { $editArgs += @('--prerelease=false', '--latest') }
    gh @editArgs
    if ($LASTEXITCODE -ne 0) { throw 'Publishing release failed' }
} finally {
    Pop-Location
}
