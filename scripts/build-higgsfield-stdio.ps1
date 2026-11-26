param(
  [Parameter(Mandatory)][string]$Sdk,
  [Parameter(Mandatory)][ValidatePattern('^[0-9a-f]{40}$')][string]$SdkCommit,
  [Parameter(Mandatory)][string]$Manifest,
  [Parameter(Mandatory)][string]$Out
)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true

$repo = (Resolve-Path -LiteralPath (Split-Path -Parent $PSScriptRoot)).Path
$source = Join-Path $repo 'native/higgsfield'
$sdkRoot = (Resolve-Path -LiteralPath $Sdk).Path
if (-not [IO.Path]::IsPathFullyQualified($Out)) { throw 'Specify an absolute output directory' }
$destination = [IO.Path]::GetFullPath($Out)
$relative = [IO.Path]::GetRelativePath($repo, $destination)
if (-not [IO.Path]::IsPathRooted($relative) -and -not $relative.StartsWith('..' + [IO.Path]::DirectorySeparatorChar)) {
  throw 'Build outside the plugin repository'
}
if (Test-Path -LiteralPath $destination) { throw 'Use a new output directory' }

$sourceCommit = git -C $repo rev-parse HEAD
if (git -C $repo status --porcelain -- native/higgsfield scripts/build-higgsfield-stdio.ps1) {
  throw 'Commit the Higgsfield source and builder before producing a release'
}

$proofFile = Join-Path $sdkRoot 'sdk-export.json'
$proof = Get-Content -LiteralPath $proofFile -Raw | ConvertFrom-Json -AsHashtable
if ($proof.version -ne 1 -or $proof.sourceCommit -cne $SdkCommit -or $proof.sourceDirty -ne $false -or $proof.license -cne 'Apache-2.0') {
  throw 'Use the requested clean public SDK export'
}
foreach ($file in $proof.files) {
  if ($file.path -notmatch '^[a-zA-Z0-9_.-]+(/[a-zA-Z0-9_.-]+)*$' -or $file.path.Split('/') -contains '..') {
    throw 'Invalid SDK export path'
  }
  $item = Get-Item -LiteralPath (Join-Path $sdkRoot $file.path)
  if ($item.Length -ne $file.bytes -or (Get-FileHash -LiteralPath $item.FullName -Algorithm SHA256).Hash.ToLowerInvariant() -cne $file.sha256) {
    throw 'SDK export file differs from its fixed inventory'
  }
}

$generation = Get-Content -LiteralPath $Manifest -Raw | ConvertFrom-Json -AsHashtable
if ($generation.id -cne 'higgsfield' -or $generation.runtime -cne 'process' -or
    $generation.version -cnotmatch '^[0-9]+\.[0-9]+\.[0-9]+$') {
  throw 'Use the manifest emitted by the reviewed Higgsfield source'
}
$env:GOTOOLCHAIN = 'local'
$env:GOENV = 'off'
$env:GOFLAGS = ''
$env:GOWORK = 'off'
$env:GOPROXY = 'off'
$env:GOSUMDB = 'off'
$env:CGO_ENABLED = '0'
$env:GOARCH = 'amd64'
$env:GOAMD64 = 'v1'
$env:GOOS = ''
$hostOS = go env GOHOSTOS
if ($hostOS -notin 'windows','linux' -or (go env GOVERSION) -cne ('go' + $proof.goVersion)) {
  throw 'Use the exact SDK Go version on Linux or Windows'
}
$goRoot = go env GOROOT
$moduleCache = go env GOMODCACHE

$bundle = Join-Path $destination 'source'
$null = New-Item -ItemType Directory -Path (Join-Path $bundle 'bin'),(Join-Path $bundle 'licenses')
$mod = Join-Path $destination 'build.mod'
Copy-Item -LiteralPath (Join-Path $source 'go.mod') -Destination $mod
Copy-Item -LiteralPath (Join-Path $source 'go.sum') -Destination (Join-Path $destination 'build.sum')
Push-Location -LiteralPath $source
try {
  go mod edit "-modfile=$mod" "-replace=gitea.nkit.top/xurx/Ordiweft=$sdkRoot"
  foreach ($target in 'linux','windows') {
    $env:GOOS = $target
    $name = if ($target -eq 'linux') { 'bin/linux-amd64' } else { 'bin/windows-amd64.exe' }
    go build -mod=readonly "-modfile=$mod" -trimpath -buildvcs=false '-ldflags=-buildid= -s -w' -o (Join-Path $bundle $name) .
  }
} finally {
  Pop-Location
}

foreach ($name in 'README.md','LICENSE','NOTICE','catalog.json') {
  Copy-Item -LiteralPath (Join-Path $source $name) -Destination (Join-Path $bundle $name)
}
$licenses = @{
  'Go-LICENSE.txt' = Join-Path $goRoot 'LICENSE'
  'Go-PATENTS.txt' = Join-Path $goRoot 'PATENTS'
  'Go-memmove_amd64.s.txt' = Join-Path $goRoot 'src/runtime/memmove_amd64.s'
  'Go-math-log.go.txt' = Join-Path $goRoot 'src/math/log.go'
  'Go-fiat-crypto-p256_ordinv.go.txt' = Join-Path $goRoot 'src/crypto/internal/fips140/nistec/p256_ordinv.go'
  'Go-fiat-crypto-edwards25519-scalar.go.txt' = Join-Path $goRoot 'src/crypto/internal/fips140/edwards25519/scalar.go'
  'SDK-LICENSE.txt' = Join-Path $sdkRoot 'LICENSE'
  'SDK-NOTICE.txt' = Join-Path $sdkRoot 'NOTICE'
  'jsonschema-go-LICENSE.txt' = Join-Path $moduleCache 'github.com/google/jsonschema-go@v0.4.3/LICENSE'
}
foreach ($name in $licenses.Keys) {
  Copy-Item -LiteralPath $licenses[$name] -Destination (Join-Path $bundle "licenses/$name")
}
function Inventory($directory) {
  foreach ($file in (Get-ChildItem -LiteralPath $directory -Recurse -File | Sort-Object FullName)) {
    [ordered]@{
      path = [IO.Path]::GetRelativePath($directory, $file.FullName).Replace('\','/')
      bytes = $file.Length
      sha256 = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    }
  }
}
$adapters = @(Inventory $bundle | Where-Object { $_.path.StartsWith('bin/') })
[ordered]@{version=1; sourceCommit=$sourceCommit; sdkCommit=$SdkCommit; goVersion=$proof.goVersion; adapters=$adapters} |
  ConvertTo-Json -Depth 10 | Set-Content -LiteralPath (Join-Path $bundle 'build-proof.txt') -Encoding utf8NoBOM
[ordered]@{
  formatVersion=1
  packageId='ordiweft-migrations/higgsfield'
  version=$generation.version
  license='Apache-2.0'
  runtime='data'
  providerProfile='higgsfield-stdio-v1'
  hostProtocol=1
  contributions=@(@{id='higgsfield';kind='provider'})
  requirements=@()
  generation=$generation
  files=@(Inventory $bundle)
} | ConvertTo-Json -Depth 100 | Set-Content -LiteralPath (Join-Path $bundle 'release.json') -Encoding utf8NoBOM
$bundle
