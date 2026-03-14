param(
    [Parameter(Mandatory = $true)]
    [string]$Tag,

    [Parameter(Mandatory = $true)]
    [string]$ApiKey
)

$ErrorActionPreference = 'Stop'

if (-not (Get-Command choco -ErrorAction SilentlyContinue)) {
    throw "Chocolatey CLI (choco) is required but was not found on PATH."
}

$version = $Tag.TrimStart('v')
$assetName = "nodeman_${version}_windows_amd64.zip"
$checksumsName = "checksums.txt"
$releaseBase = "https://github.com/arcmantle/nodeman/releases/download/v${version}"

$workspace = Join-Path $PWD ".choco-build"
$packageRoot = Join-Path $workspace "nodeman"
$toolsDir = Join-Path $packageRoot "tools"

if (Test-Path $workspace) {
    Remove-Item -Path $workspace -Recurse -Force
}

New-Item -ItemType Directory -Path $toolsDir -Force | Out-Null

$zipPath = Join-Path $workspace $assetName
$checksumsPath = Join-Path $workspace $checksumsName

Invoke-WebRequest -Uri "$releaseBase/$assetName" -OutFile $zipPath
Invoke-WebRequest -Uri "$releaseBase/$checksumsName" -OutFile $checksumsPath

$checksumLine = Select-String -Path $checksumsPath -Pattern [regex]::Escape($assetName) | Select-Object -First 1
if (-not $checksumLine) {
    throw "Could not find checksum for asset '$assetName' in $checksumsName."
}

$checksum = ($checksumLine.Line -split '\s+')[0].Trim()
if (-not $checksum) {
    throw "Failed to parse checksum for '$assetName'."
}

Copy-Item -Path "packaging/chocolatey/tools/chocolateyuninstall.ps1" -Destination (Join-Path $toolsDir "chocolateyuninstall.ps1")

$installTemplate = Get-Content "packaging/chocolatey/tools/chocolateyinstall.ps1.template" -Raw
$installScript = $installTemplate.Replace("__VERSION__", $version).Replace("__CHECKSUM__", $checksum)
Set-Content -Path (Join-Path $toolsDir "chocolateyinstall.ps1") -Value $installScript -NoNewline

$nuspecTemplate = Get-Content "packaging/chocolatey/nodeman.nuspec" -Raw
$nuspec = $nuspecTemplate.Replace("__VERSION__", $version)
$nuspecPath = Join-Path $packageRoot "nodeman.nuspec"
Set-Content -Path $nuspecPath -Value $nuspec -NoNewline

choco pack $nuspecPath --outputdirectory $workspace

$packagePath = Join-Path $workspace "nodeman.$version.nupkg"
if (-not (Test-Path $packagePath)) {
    throw "Chocolatey package was not created: $packagePath"
}

choco push $packagePath --source "https://push.chocolatey.org/" --api-key $ApiKey
