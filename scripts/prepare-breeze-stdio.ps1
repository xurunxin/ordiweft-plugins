param(
    [Parameter(Mandatory)][string]$Archive,
    [Parameter(Mandatory)][ValidatePattern('^[0-9a-f]{64}$')][string]$SHA256,
    [Parameter(Mandatory)][ValidateSet('api','local')][string]$Channel,
    [Parameter(Mandatory)][ValidateSet('linux','windows')][string]$TargetOS,
    [Parameter(Mandatory)][string]$OutDir,
    [uri]$LocalURL,
    [ValidatePattern('^[0-9a-f]{64}$')][string]$ModelDigest
)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
if ($Channel -eq 'local') {
    if (-not $LocalURL -or -not $LocalURL.IsAbsoluteUri -or $LocalURL.Scheme -notin 'http','https' -or
        $LocalURL.UserInfo -or $LocalURL.Query -or $LocalURL.Fragment -or -not $ModelDigest) {
        throw 'Provide a trusted local HTTP(S) service and the verified model digest'
    }
} elseif ($LocalURL -or $ModelDigest) {
    throw 'The hosted API uses its fixed service origin'
}
$archivePath = (Resolve-Path -LiteralPath $Archive).Path
if ((Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash -ne $SHA256) {
    throw 'Archive SHA-256 differs from the reviewed release'
}
$out = [IO.Path]::GetFullPath($OutDir)
if (Test-Path -LiteralPath $out) { throw 'Output directory already exists' }
if ((Split-Path -Leaf $out) -cne "breeze-tts-$Channel") {
    throw 'Use a new directory named for the selected Breeze channel'
}
$adapter = if ($TargetOS -eq 'windows') { 'windows-amd64.exe' } else { 'linux-amd64' }
$zip = [IO.Compression.ZipFile]::OpenRead($archivePath)
try {
    $entry = $zip.GetEntry('release.json')
    if (-not $entry -or $entry.Length -gt 256KB) { throw 'Invalid release metadata' }
    $reader = [IO.StreamReader]::new($entry.Open())
    try { $release = $reader.ReadToEnd() | ConvertFrom-Json -AsHashtable } finally { $reader.Dispose() }
    if ($release.packageId -cne "ordiweft-migrations/breeze-tts-$Channel" -or
        $release.providerProfile -cne 'breeze-stdio-v1' -or $release.generation.id -cne "breeze-tts-$Channel") {
        throw 'Release does not describe the selected Breeze channel'
    }
    $path = "bin/$adapter"
    $binary = $zip.GetEntry($path)
    $verified = @($release.files | Where-Object path -CEQ $path)
    if ($verified.Count -ne 1 -or -not $binary -or $binary.Length -ne $verified[0].bytes -or $binary.Length -gt 32MB) {
        throw 'Binary inventory differs from the release'
    }
    $stream = $binary.Open()
    try {
        $digest = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($stream)).ToLowerInvariant()
    } finally { $stream.Dispose() }
    if ($digest -cne $verified[0].sha256) { throw 'Binary SHA-256 differs from the release' }
    New-Item -ItemType Directory -Path $out | Out-Null
    $destination = Join-Path $out $adapter
    [IO.Compression.ZipFileExtensions]::ExtractToFile($binary, $destination, $false)
    if ($TargetOS -eq 'linux' -and $IsLinux) { chmod 0755 -- $destination }
    $command = @($adapter, '-channel', $Channel)
    if ($Channel -eq 'local') { $command += @('-url', $LocalURL.AbsoluteUri, '-model-digest', $ModelDigest) }
    [ordered]@{manifest=$release.generation; command=$command; sha256=$digest} |
        ConvertTo-Json -Depth 30 | Set-Content -LiteralPath (Join-Path $out 'plugin.json') -Encoding utf8NoBOM
    "Prepared Breeze $Channel at $out with adapter SHA-256 $digest"
} finally { $zip.Dispose() }
