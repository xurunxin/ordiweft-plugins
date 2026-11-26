param(
    [Parameter(Mandatory)][string]$SDK,
    [Parameter(Mandatory)][ValidatePattern('^[a-f0-9]{40}$')][string]$SDKCommit,
    [Parameter(Mandatory)][string]$OutDir
)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
$repo = Split-Path -Parent $PSScriptRoot
$source = Join-Path $repo 'native/breeze-tts'
$sdkRoot = (Resolve-Path -LiteralPath $SDK).Path
$out = [IO.Path]::GetFullPath($OutDir)
$relative = [IO.Path]::GetRelativePath($repo, $out)
if (-not [IO.Path]::IsPathRooted($relative) -and -not $relative.StartsWith('..' + [IO.Path]::DirectorySeparatorChar)) {
    throw 'Use an output directory outside the repository'
}
if (Test-Path -LiteralPath $out) { throw 'Output directory already exists' }
$sourceCommit = git -C $repo rev-parse HEAD
if (git -C $repo status --porcelain -- native/breeze-tts scripts/build-breeze-tts.ps1) {
    throw 'Commit the reviewed Breeze adapter source before building a release'
}
$proof = Get-Content -Raw -LiteralPath (Join-Path $sdkRoot 'sdk-export.json') | ConvertFrom-Json
if ($proof.version -ne 1 -or $proof.license -cne 'Apache-2.0' -or $proof.sourceCommit -cne $SDKCommit -or $proof.sourceDirty -ne $false) {
    throw 'Use a clean fixed public Go SDK export'
}
$module = Join-Path $out 'source'
$bin = Join-Path $out 'bin'
$licenses = Join-Path $out 'licenses'
New-Item -ItemType Directory -Path $module,$bin,$licenses | Out-Null
foreach ($directory in 'cmd','pkg') {
    Copy-Item -LiteralPath (Join-Path $source $directory) -Destination $module -Recurse
}
foreach ($file in $proof.files) {
    if ($file.path -cne 'go.mod' -and $file.path -cnotmatch '^pkg/aigc/[A-Za-z0-9_.-]+$') { continue }
    $path = Join-Path $sdkRoot $file.path
    $item = Get-Item -LiteralPath $path
    if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint -or $item.Length -ne $file.bytes -or
        (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -cne $file.sha256) {
        throw 'Public SDK file differs from export inventory'
    }
    $destination = Join-Path $module $file.path
    [IO.Directory]::CreateDirectory((Split-Path -Parent $destination)) | Out-Null
    Copy-Item -LiteralPath $path -Destination $destination
}
foreach ($path in 'pkg/aigc/LICENSE','pkg/aigc/NOTICE','go.mod') {
    if (-not (Test-Path -LiteralPath (Join-Path $module $path) -PathType Leaf)) {
        throw 'Public Go SDK license or module information is missing'
    }
}
Copy-Item -LiteralPath (Join-Path $module 'pkg/aigc/LICENSE') -Destination (Join-Path $licenses 'SDK-LICENSE.txt')
Copy-Item -LiteralPath (Join-Path $module 'pkg/aigc/NOTICE') -Destination (Join-Path $licenses 'SDK-NOTICE.txt')
$environment = @{
    GOENV='off'; GOFLAGS=''; GOTOOLCHAIN='local'; GOPROXY='off'; GOSUMDB='off'; GOWORK='off'
    CGO_ENABLED='0'; GOOS=''; GOARCH=''; GOAMD64='v1'
}
$previous = @{}
foreach ($name in $environment.Keys) {
    $previous[$name] = [Environment]::GetEnvironmentVariable($name)
    [Environment]::SetEnvironmentVariable($name, $environment[$name], 'Process')
}
Push-Location -LiteralPath $module
try {
    $toolchain = go version
    $goRoot = go env GOROOT
    $goNotices = [ordered]@{
        'Go-LICENSE.txt' = 'LICENSE'
        'Go-PATENTS.txt' = 'PATENTS'
        'Go-memmove_amd64.s.txt' = 'src/runtime/memmove_amd64.s'
        'Go-math-log.go.txt' = 'src/math/log.go'
        'Go-fiat-crypto-p256_ordinv.go.txt' = 'src/crypto/internal/fips140/nistec/p256_ordinv.go'
        'Go-fiat-crypto-edwards25519-scalar.go.txt' = 'src/crypto/internal/fips140/edwards25519/scalar.go'
    }
    foreach ($name in $goNotices.Keys) {
        Copy-Item -LiteralPath (Join-Path $goRoot $goNotices[$name]) -Destination (Join-Path $licenses $name)
    }
    go test ./pkg/breezetts
    $env:GOARCH = 'amd64'
    foreach ($target in 'linux','windows') {
        $env:GOOS = $target
        $filename = if ($target -eq 'windows') { 'windows-amd64.exe' } else { 'linux-amd64' }
        go build -trimpath -buildvcs=false -ldflags '-s -w -buildid=' -o (Join-Path $bin $filename) ./cmd/breeze-tts-plugin
    }
} finally {
    Pop-Location
    foreach ($name in $previous.Keys) {
        [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process')
    }
}
$adapters = @(Get-ChildItem -LiteralPath $bin -File | Sort-Object Name | ForEach-Object {
    [ordered]@{path = "bin/$($_.Name)"; bytes = $_.Length; sha256 = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()}
})
[ordered]@{version = 1; sourceCommit = $sourceCommit; sdkCommit = $SDKCommit; goVersion = $toolchain; adapters = $adapters} |
    ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $out 'build-proof.json') -Encoding utf8NoBOM
