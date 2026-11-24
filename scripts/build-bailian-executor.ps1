param(
  [Parameter(Mandatory)][string]$HostSource,
  [Parameter(Mandatory)][string]$Version,
  [Parameter(Mandatory)][string]$Out
)
$ErrorActionPreference = 'Stop'
if ($Version -cnotmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') { throw 'Pass a stable package version' }
if (![IO.Path]::IsPathFullyQualified($Out) -or (Test-Path -LiteralPath $Out)) { throw 'Pass a new absolute build directory' }
$root = Split-Path -Parent $PSScriptRoot
$sdk = (Resolve-Path -LiteralPath $HostSource).Path
$source = Join-Path $root 'executors/dashscope'
$native = Join-Path $root 'native/dashscope'
$bundle = Join-Path $Out 'source'
New-Item -ItemType Directory -Path $bundle | Out-Null
$mod = Join-Path $Out 'build.mod'
Copy-Item -LiteralPath (Join-Path $source 'go.mod') -Destination $mod

function Run-Go {
  & go @args
  if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
}
Push-Location $source
try {
  $env:GOWORK = 'off'
  Run-Go mod edit "-modfile=$mod" "-replace=gitea.nkit.top/xurx/Ordiweft=$sdk" "-replace=gitea.nkit.top/xurx/Ordiweft/internal/dashscope=$native"
  $env:GOOS = 'linux'
  $env:GOARCH = 'amd64'
  $env:CGO_ENABLED = '0'
  Run-Go build "-modfile=$mod" -trimpath -buildvcs=false "-ldflags=-buildid= -s -w -X main.adapterVersion=$Version" -o (Join-Path $bundle 'adapter') .
  $env:GOOS = 'windows'
  Run-Go build "-modfile=$mod" -trimpath -buildvcs=false "-ldflags=-X main.adapterVersion=$Version" -o (Join-Path $Out 'manifest.exe') .
} finally {
  Pop-Location
}
$env:DASHSCOPE_REGION = 'cn-beijing'
$generation = & (Join-Path $Out 'manifest.exe') manifest $Version | ConvertFrom-Json -AsHashtable
if ($LASTEXITCODE -ne 0 -or $generation.id -cne 'dashscope' -or $generation.version -cne $Version) { throw 'Invalid emitted manifest' }
foreach ($item in 'LICENSE','NOTICE') {
  Copy-Item -LiteralPath (Join-Path $native $item) -Destination (Join-Path $bundle $item)
}
@"
# 阿里云百炼完整渠道

独立的 Linux amd64 适配器通过宿主的受限任务接口使用获准的百炼账号。图片、视频、合成与音色设计保留原模型和渠道标识。安装、启用、升级和回退由插件中心管理。
"@ | Set-Content -LiteralPath (Join-Path $bundle 'README.md') -Encoding utf8NoBOM
$files = foreach ($name in 'LICENSE','NOTICE','README.md','adapter') {
  $item = Get-Item -LiteralPath (Join-Path $bundle $name)
  [ordered]@{path=$name;bytes=$item.Length;sha256=(Get-FileHash -LiteralPath $item.FullName -Algorithm SHA256).Hash.ToLowerInvariant()}
}
[ordered]@{
  formatVersion=1
  packageId='ordiweft-migrations/dashscope'
  version=$Version
  license='Apache-2.0'
  runtime='process'
  providerProfile='dashscope-exec-v1'
  hostProtocol=1
  contributions=@(@{id='dashscope';kind='provider'})
  requirements=@(
    @{service='assets';operations=@('read');version=1},
    @{service='bailian';operations=@('context','write','request','read','upload-audio','clone-get','clone-put','checkpoint','stage');version=1}
  )
  generation=$generation
  files=@($files)
} | ConvertTo-Json -Depth 100 | Set-Content -LiteralPath (Join-Path $bundle 'release.json') -Encoding utf8NoBOM
