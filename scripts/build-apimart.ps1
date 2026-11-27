# SPDX-License-Identifier: Apache-2.0
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Sdk,
    [Parameter(Mandatory)][string]$Out
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
New-Item -ItemType Directory -Path $destination | Out-Null
$modfile = Join-Path $destination 'build.mod'
Copy-Item -LiteralPath (Join-Path $project 'go.mod') -Destination $modfile
$env:GOWORK = 'off'
$env:GOTOOLCHAIN = 'local'
$env:GOPROXY = 'off'
$env:CGO_ENABLED = '0'
Push-Location -LiteralPath $project
try {
    rtk proxy go mod edit "-modfile=$modfile" "-replace=gitea.nkit.top/xurx/Ordiweft=$($sdkRoot.Replace('\','/'))"
    rtk proxy go test "-modfile=$modfile" ./...
    $hostExe = Join-Path $destination 'manifest-emitter.exe'
    rtk proxy go build "-modfile=$modfile" -trimpath -buildvcs=false -ldflags=-buildid= -o $hostExe .
    $manifest = (& $hostExe manifest | ConvertFrom-Json -AsHashtable)
    $env:GOARCH = 'amd64'
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
