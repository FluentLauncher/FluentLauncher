# Pure release planning helpers, also exercised by test-release.ps1.
function Get-ReleasePlan {
    param(
        [ValidateSet('build', 'nightly', 'patch', 'minor', 'major', 'tag')][string]$Action,
        [string[]]$StableTags = @(),
        [string]$InitialVersion = '0.1.0',
        [string]$Tag = '',
        [string]$Date = [DateTime]::UtcNow.ToString('yyyyMMdd'),
        [long]$RunNumber = 1,
        [long]$RunAttempt = 1
    )
    $stablePattern = '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
    $previous = $StableTags | Where-Object { $_ -cmatch $stablePattern } |
        Sort-Object { [version]$_.Substring(1) } -Descending | Select-Object -First 1
    $base = if ($previous) { $previous.Substring(1) } else { $InitialVersion }
    if ('v' + $base -cnotmatch $stablePattern) { throw 'Invalid initial stable version' }
    $v = [version]$base
    $nightly = $Action -eq 'nightly'
    switch ($Action) {
        'build' { $version = $InitialVersion; $Tag = '' }
        'tag' {
            if ($Tag -cnotmatch $stablePattern) { throw 'Manual tags must be stable vX.Y.Z versions' }
            $version = $Tag.Substring(1)
            if ($previous -and [version]$version -le $v) { throw 'Release version must exceed the latest published stable version' }
        }
        'major' { $version = '{0}.0.0' -f ($v.Major + 1) }
        'minor' { $version = '{0}.{1}.0' -f $v.Major, ($v.Minor + 1) }
        default {
            $version = '{0}.{1}.{2}' -f $v.Major, $v.Minor, ($v.Build + 1)
            if ($nightly) {
                if ($Date -notmatch '^[1-9][0-9]{7}$' -or $RunNumber -lt 1 -or $RunAttempt -lt 1) {
                    throw 'Nightly requires a UTC date and positive run number/attempt'
                }
                $version += '-nightly.{0}.{1}.{2}' -f $Date, $RunNumber, $RunAttempt
            }
        }
    }
    if ($Action -ne 'build') { $Tag = 'v' + $version }
    [pscustomobject]@{
        action = $Action
        tag = $Tag
        version = $version
        nightly = $nightly
        publish = $Action -ne 'build'
        previousTag = $previous
    }
}

function Get-ReleaseNotesText {
    param([string]$Version, [string]$Changelog, [string]$Fallback)
    $pattern = '(?ms)^## ' + [regex]::Escape($Version) + '\r?\n(.*?)(?=^## |\z)'
    $section = [regex]::Match($Changelog, $pattern)
    if ($section.Success -and $section.Groups[1].Value.Trim()) {
        return $section.Groups[1].Value.Trim()
    }
    return $Fallback.Trim()
}
