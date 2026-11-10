#requires -Version 7.0
$ErrorActionPreference = 'Stop'

$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
try {
  $env:CGO_ENABLED = '0'
  $env:GOFLAGS = '-mod=readonly'
  $output = Join-Path $projectRoot 'dist/windows-amd64'
  if (Test-Path -LiteralPath $output) {
    $resolved = (Resolve-Path -LiteralPath $output).Path
    if ($resolved -ne [IO.Path]::GetFullPath($output)) {
      throw "Unexpected build output path: $resolved"
    }
    Remove-Item -LiteralPath $resolved -Recurse -Force
  }
  go tool mygo build -platform windows/amd64 -skip-installer
  if ($LASTEXITCODE -ne 0) { throw "MyGo build failed with code $LASTEXITCODE" }
  Remove-Item -LiteralPath (Join-Path $output '.mygo-files') -ErrorAction SilentlyContinue
  $files = @(Get-ChildItem -LiteralPath $output -Force)
  if ($files.Count -ne 1 -or $files[0].Name -ne 'FontLens.exe' -or $files[0].PSIsContainer) {
    throw 'Portable build must contain only FontLens.exe'
  }
} finally {
  Pop-Location
}
