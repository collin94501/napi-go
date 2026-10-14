param(
    [Parameter(Mandatory = $true)][string]$HeadersPath,
    [Parameter(Mandatory = $true)][string]$RuntimePath
)
$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path "$PSScriptRoot/../..").Path
$headers = (Resolve-Path $HeadersPath).Path
$runtime = (Resolve-Path $RuntimePath).Path
$build = Join-Path $repo '.test-tmp/lifetime'
New-Item -ItemType Directory -Force $build | Out-Null
$env:TEMP = $build
$env:TMP = $build

# All build/cache outputs stay in the assigned checkout. No module downloads.
$env:CC = 'clang'
$env:CGO_ENABLED = '1'
$env:GOPROXY = 'off'
$env:GOSUMDB = 'off'
$env:GOTOOLCHAIN = 'local'
$env:GOCACHE = Join-Path $build 'go-cache'
$env:GOTMPDIR = $build
$env:GOEXPERIMENT = 'cgocheck2'
$env:CGO_CFLAGS = "-I `"$headers/include`" -DNAPI_VERSION=9"
$env:CGO_LDFLAGS_ALLOW = '.*'
$symbols = nm --defined-only --extern-only "$headers/x64/node.lib"
if ($LASTEXITCODE -ne 0) { throw 'Cannot read official Node import library' }
$exports = $symbols | ForEach-Object {
    if ($_ -match ' __imp_((?:napi_|node_api_)\w+)$') { $Matches[1] }
} | Sort-Object -Unique
if (!$exports) { throw 'No Node-API exports found' }
@('EXPORTS') + $exports | Set-Content "$build/node.def"
dlltool -d "$build/node.def" -D node.exe -y "$build/libnode-delay.a"
if ($LASTEXITCODE -ne 0) { throw 'Cannot create delay import library' }
$env:CGO_LDFLAGS = "-L `"$build`" -lnode-delay -Wl,--wrap=napi_create_function -Wl,--wrap=napi_add_finalizer -Wl,--wrap=napi_set_instance_data"
Push-Location $repo
try {
    go build -p 2 -gcflags=all=-d=checkptr=2 -buildmode=c-shared -o "$build/lifetime.node" ./tests/lifetime
    if ($LASTEXITCODE -ne 0) { throw 'Lifetime addon build failed' }
    $env:ELECTRON_RUN_AS_NODE = '1'
    & $runtime --expose-gc "$PSScriptRoot/test.cjs" "$build/lifetime.node"
    if ($LASTEXITCODE -ne 0) { throw 'Lifetime integration test failed' }
} finally {
    Pop-Location
}
