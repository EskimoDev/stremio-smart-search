param(
  [string]$TargetOS = 'windows',
  [string]$TargetArch = 'amd64',
  [string]$Version = '0.1.0-preview.3',
  [string]$GoExecutable = 'go'
)
$ErrorActionPreference = 'Stop'
if ($Version -notmatch '^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$') { throw 'Use a semantic release version, for example 0.1.0-preview.1' }
$projectPath = Split-Path -Parent $PSScriptRoot
Push-Location $projectPath
$previousGoOS, $previousGoArch, $previousCGO = $env:GOOS, $env:GOARCH, $env:CGO_ENABLED
try {
  $env:GOOS = $TargetOS
  $env:GOARCH = $TargetArch
  $env:CGO_ENABLED = '0'
  $binaryName = "smart-search-$TargetOS-$TargetArch"
  if ($TargetOS -eq 'windows') { $binaryName += '.exe' }
  $binaryPath = Join-Path 'dist' $binaryName
  $linkFlags = "-s -w -X main.version=$Version"
  if ($TargetOS -eq 'windows') { $linkFlags += ' -H=windowsgui' }
  & $GoExecutable build -trimpath -ldflags $linkFlags -o $binaryPath ./cmd/smart-search
  if ($LASTEXITCODE -ne 0) { throw "Build failed for $TargetOS/$TargetArch" }
  # The packaging utility runs on the build machine, not the target architecture.
  $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = $previousGoOS, $previousGoArch, $previousCGO
  & $GoExecutable run ./tools/package --os $TargetOS --arch $TargetArch --binary $binaryPath --version $Version
  if ($LASTEXITCODE -ne 0) { throw "Packaging failed for $TargetOS/$TargetArch" }
} finally {
  $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = $previousGoOS, $previousGoArch, $previousCGO
  Pop-Location
}
