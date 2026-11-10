param(
    [string]$Tag = '',
    [string]$Repository = $env:GITHUB_REPOSITORY,
    [switch]$Signed
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
$configPath = Join-Path $projectRoot 'mygo.json'
$originalConfig = [IO.File]::ReadAllText($configPath)
$utf8 = [Text.UTF8Encoding]::new($false)
try {
    $config = $originalConfig | ConvertFrom-Json
    if ($Tag) {
        if ($Tag -notmatch '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') {
            throw 'Release tags must be stable versions such as v0.2.0'
        }
        $config.version = $Tag.Substring(1)
    }
    if ($Signed) {
        if ($Repository -notmatch '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$') {
            throw 'Set Repository to the public GitHub owner/repo serving releases'
        }
        if (-not $env:MYGO_UPDATER_PUBLIC_KEY -or -not $env:MYGO_UPDATER_PRIVATE_KEY) {
            throw 'Signed releases require MYGO_UPDATER_PUBLIC_KEY and MYGO_UPDATER_PRIVATE_KEY'
        }
        $config | Add-Member -NotePropertyName updates -NotePropertyValue @{
            publicKey = $env:MYGO_UPDATER_PUBLIC_KEY.Trim()
            github = $Repository
            deltas = 0
        } -Force
    }
    [IO.File]::WriteAllText($configPath, ($config | ConvertTo-Json -Depth 10) + "`n", $utf8)
    # Verify the embedded DLL against the manifest in the pinned MyGo module.
    $mygoModule = go list -m -f '{{.Dir}}' github.com/egoist/mygo
    if ($LASTEXITCODE -ne 0) { throw 'Cannot locate pinned MyGo module' }
    $nativeManifest = Get-Content -Raw -LiteralPath (Join-Path $mygoModule 'plugins/sqlite/mygo-plugin.json') | ConvertFrom-Json
    $expectedHash = $nativeManifest.libraries[0].files.'windows-amd64'.sha256
    $actualHash = (Get-FileHash -LiteralPath internal/bundled/mygo-sqlite3.dll -Algorithm SHA256).Hash
    if ($actualHash -ne $expectedHash) { throw 'Embedded SQLite DLL does not match pinned MyGo; refresh the runtime before releasing' }
    go tool mygo build -platform windows/amd64 -skip-installer -o build/release
    if ($LASTEXITCODE -ne 0) { throw 'MyGo release build failed' }

    $output = Join-Path $projectRoot 'build/release/windows-amd64'
    $exe = Join-Path $output 'FluentLauncher.exe'
    if (-not (Test-Path -LiteralPath $exe)) { throw "Missing executable: $exe" }
    if ($Signed) {
        go run ./scripts/portable -dir $output
        if ($LASTEXITCODE -ne 0) { throw 'Single-file update packaging failed' }
        $archive = Join-Path $output ('fluent-launcher-' + $config.version + '-windows-amd64.tar.gz')
        $manifest = Join-Path $output 'update-windows-amd64.json'
        if (-not (Test-Path -LiteralPath $archive) -or -not (Test-Path -LiteralPath $manifest)) {
            throw 'Signed build did not produce its update archive and manifest'
        }
        $archiveFiles = @(tar -tzf $archive)
        if ($LASTEXITCODE -ne 0 -or $archiveFiles.Count -ne 1 -or $archiveFiles[0] -ne 'FluentLauncher.exe') {
            throw 'The update archive must contain only FluentLauncher.exe'
        }
    }
    # MyGo copies native libraries and legacy development examples beside the
    # EXE. They are not runtime dependencies of the portable distribution.
    foreach ($sidecarName in @('examples', 'mygo-sqlite3.dll')) {
        $sidecar = [IO.Path]::GetFullPath((Join-Path $output $sidecarName))
        if (-not $sidecar.StartsWith([IO.Path]::GetFullPath($output) + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
            throw 'Sidecar path outside release output'
        }
        if (Test-Path -LiteralPath $sidecar) { Remove-Item -LiteralPath $sidecar -Recurse -Force }
    }
    $checksums = Get-ChildItem -LiteralPath $output -File |
        Where-Object Name -ne 'SHA256SUMS' |
        Sort-Object Name |
        ForEach-Object { (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant() + '  ' + $_.Name }
    [IO.File]::WriteAllText((Join-Path $output 'SHA256SUMS'), ($checksums -join "`n") + "`n", $utf8)
    Write-Output "Portable release: $exe"
} finally {
    [IO.File]::WriteAllText($configPath, $originalConfig, $utf8)
    Pop-Location
}
