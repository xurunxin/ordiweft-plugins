# SPDX-License-Identifier: Apache-2.0
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Sdk,
    [Parameter(Mandatory)][string]$Out,
    [switch]$Release
)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
$project = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../native/apimart'))
$repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$sdkRoot = (Resolve-Path -LiteralPath $Sdk).Path
$destination = [IO.Path]::GetFullPath($Out)
if (Test-Path -LiteralPath $destination) { throw 'Choose a new output directory.' }
if (!(Test-Path -LiteralPath (Join-Path $sdkRoot 'sdk-export.json'))) { throw 'A trusted public Ordiweft Go SDK export is required.' }
if ($destination.StartsWith($repo + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Choose an output directory outside the source checkout.'
}
$proof = Get-Content -LiteralPath (Join-Path $sdkRoot 'sdk-export.json') -Raw | ConvertFrom-Json -AsHashtable
if ($Release) {
    $sourceCommit = rtk proxy git -C $repo rev-parse HEAD
    $sourceChanges = @(rtk proxy git -C $repo status --porcelain -- native/apimart scripts/build-apimart.ps1 LICENSE NOTICE)
    if ($sourceChanges.Count -gt 0 -or $proof.version -ne 1 -or $proof.sourceDirty -ne $false -or
        $proof.sourceCommit -cnotmatch '^[a-f0-9]{40}$' -or $proof.license -cne 'Apache-2.0' -or
        (rtk proxy go env GOVERSION) -cne ('go' + $proof.goVersion)) {
        throw 'Use clean committed plugin sources and an exact public SDK export.'
    }
    foreach ($file in $proof.files) {
        if ($file.path -cnotmatch '^[a-zA-Z0-9_.-]+(/[a-zA-Z0-9_.-]+)*$' -or $file.path.Split('/') -contains '..') {
            throw 'Invalid SDK inventory path.'
        }
        $item = Get-Item -LiteralPath (Join-Path $sdkRoot $file.path)
        $hash = (Get-FileHash -LiteralPath $item.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($item.Length -ne $file.bytes -or $hash -cne $file.sha256) {
            throw 'SDK export differs from its fixed inventory.'
        }
    }
    $goRoot = rtk proxy go env GOROOT
}
New-Item -ItemType Directory -Path $destination | Out-Null
$modfile = Join-Path $destination 'build.mod'
Copy-Item -LiteralPath (Join-Path $project 'go.mod') -Destination $modfile
$env:GOWORK = 'off'
$env:GOTOOLCHAIN = 'local'
$env:GOENV = 'off'
$env:GOFLAGS = ''
$env:GOPROXY = 'off'
$env:GOSUMDB = 'off'
$env:CGO_ENABLED = '0'
$env:GOOS = ''
$env:GOARCH = ''
$env:GOAMD64 = 'v1'

function Read-Inventory([string]$directory) {
    foreach ($item in Get-ChildItem -LiteralPath $directory -Recurse -File | Sort-Object FullName) {
        [ordered]@{
            path = [IO.Path]::GetRelativePath($directory, $item.FullName).Replace('\','/')
            bytes = $item.Length
            sha256 = (Get-FileHash -LiteralPath $item.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        }
    }
}

Push-Location -LiteralPath $project
try {
    rtk proxy go mod edit "-modfile=$modfile" "-replace=gitea.nkit.top/xurx/Ordiweft=$($sdkRoot.Replace('\','/'))"
    rtk proxy go test "-modfile=$modfile" ./...
    $hostExe = Join-Path $destination 'manifest-emitter.exe'
    rtk proxy go build "-modfile=$modfile" -trimpath -buildvcs=false -ldflags=-buildid= -o $hostExe .
    $manifest = (& $hostExe manifest | ConvertFrom-Json -AsHashtable)
    $env:GOARCH = 'amd64'
    if ($Release) {
        $bundle = Join-Path $destination 'source'
        $null = New-Item -ItemType Directory -Path (Join-Path $bundle 'bin'),(Join-Path $bundle 'licenses')
        foreach ($platform in 'linux','windows') {
            $env:GOOS = $platform
            $file = if ($platform -eq 'windows') { 'bin/windows-amd64.exe' } else { 'bin/linux-amd64' }
            rtk proxy go build -mod=readonly "-modfile=$modfile" -trimpath -buildvcs=false '-ldflags=-buildid= -s -w' -o (Join-Path $bundle $file) .
        }
        Copy-Item -LiteralPath (Join-Path $repo 'LICENSE'),(Join-Path $repo 'NOTICE'),(Join-Path $project 'README.md'),(Join-Path $project 'catalog.json') -Destination $bundle
        $licenses = @{
            'Go-LICENSE.txt' = Join-Path $goRoot 'LICENSE'
            'Go-PATENTS.txt' = Join-Path $goRoot 'PATENTS'
            'Go-memmove_amd64.s.txt' = Join-Path $goRoot 'src/runtime/memmove_amd64.s'
            'Go-math-log.go.txt' = Join-Path $goRoot 'src/math/log.go'
            'Go-fiat-crypto-p256_ordinv.go.txt' = Join-Path $goRoot 'src/crypto/internal/fips140/nistec/p256_ordinv.go'
            'Go-fiat-crypto-edwards25519-scalar.go.txt' = Join-Path $goRoot 'src/crypto/internal/fips140/edwards25519/scalar.go'
            'SDK-LICENSE.txt' = Join-Path $sdkRoot 'LICENSE'
            'SDK-NOTICE.txt' = Join-Path $sdkRoot 'NOTICE'
        }
        foreach ($name in $licenses.Keys) {
            Copy-Item -LiteralPath $licenses[$name] -Destination (Join-Path $bundle "licenses/$name")
        }
        $adapters = @(Read-Inventory $bundle | Where-Object { $_.path.StartsWith('bin/') })
        [ordered]@{version=1; sourceCommit=$sourceCommit; sdkCommit=$proof.sourceCommit; goVersion=$proof.goVersion; adapters=$adapters} |
            ConvertTo-Json -Depth 10 | Set-Content -LiteralPath (Join-Path $bundle 'build-proof.txt') -Encoding utf8NoBOM
        [ordered]@{
            formatVersion=1
            packageId='ordiweft-migrations/apimart'
            version=$manifest.version
            license='Apache-2.0'
            runtime='data'
            providerProfile='apimart-stdio-v1'
            hostProtocol=1
            contributions=@(@{id='apimart';kind='provider'})
            requirements=@()
            generation=$manifest
            files=@(Read-Inventory $bundle)
        } | ConvertTo-Json -Depth 100 -Compress | Set-Content -LiteralPath (Join-Path $bundle 'release.json') -Encoding utf8NoBOM
        Write-Output $bundle
        return
    }
    foreach ($platform in @('windows','linux')) {
        $env:GOOS = $platform
        $name = "ordiweft-apimart-$($manifest.version)-$platform-amd64"
        $folder = Join-Path $destination $name
        New-Item -ItemType Directory -Path $folder | Out-Null
        $file = if ($platform -eq 'windows') { 'provider.exe' } else { 'provider' }
        $binary = Join-Path $folder $file
        rtk proxy go build "-modfile=$modfile" -trimpath -buildvcs=false -ldflags=-buildid= -o $binary .
        $hash = (Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash.ToLowerInvariant()
        $package = @{manifest=$manifest;command=@($file);sha256=$hash} | ConvertTo-Json -Depth 60 -Compress
        Set-Content -LiteralPath (Join-Path $folder 'plugin.json') -Value $package -Encoding utf8NoBOM
        Copy-Item -LiteralPath (Join-Path $repo 'LICENSE'),(Join-Path $repo 'NOTICE'),(Join-Path $project 'README.md') -Destination $folder
        $zip = Join-Path $destination ($name+'.zip')
        [IO.Compression.ZipFile]::CreateFromDirectory($folder, $zip)
        $zipHash = (Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant()
        Set-Content -LiteralPath ($zip+'.sha256') -Value "$zipHash  $name.zip" -Encoding ascii
    }
}
finally {
    Pop-Location
}
