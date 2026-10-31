param([Parameter(Mandatory)][string]$Binary)
$ErrorActionPreference = 'Stop'
$binaryPath = (Resolve-Path -LiteralPath $Binary).Path
$projectPath = Split-Path -Parent $PSScriptRoot
$fixturePath = Join-Path $projectPath 'testdata/catalog.json'
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('smart-search-smoke-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot | Out-Null
$helperProcess = $null
try {
  $processOptions = @{
    FilePath = $binaryPath
    ArgumentList = @('--no-browser', '--listen', '127.0.0.1:11498', '--data-dir', ('"' + $testRoot + '"'), '--catalog', ('"' + $fixturePath + '"'))
    PassThru = $true
  }
  if ($IsWindows) { $processOptions.WindowStyle = 'Hidden' }
  $helperProcess = Start-Process @processOptions
  $status = $null
  for ($attempt = 0; $attempt -lt 30; $attempt++) {
    try {
      $controlKey = Get-Content -LiteralPath (Join-Path $testRoot 'control.key') -Raw
      $status = Invoke-RestMethod 'http://127.0.0.1:11498/api/status' -Headers @{'X-Smart-Search-Key' = $controlKey}
      break
    } catch { Start-Sleep -Milliseconds 100 }
  }
  if (-not $status.ready) { throw 'Packaged helper did not become ready' }
  foreach ($case in @(@('series','HOTD','tt11198330'),@('movie','interstelar','tt0816692'),@('movie','LOTR','tt0120737'))) {
    $query = [uri]::EscapeDataString($case[1])
    $result = Invoke-RestMethod ('http://127.0.0.1:11498/catalog/' + $case[0] + '/smart-search/search=' + $query + '.json')
    if ($result.metas[0].id -ne $case[2]) { throw ('Incorrect packaged search result for ' + $case[1]) }
  }
  $manifest = Invoke-RestMethod 'http://127.0.0.1:11498/manifest.json'
  if ($manifest.catalogs.Count -ne 2) { throw 'Packaged manifest is incomplete' }
  Invoke-RestMethod 'http://127.0.0.1:11498/api/quit' -Method Post -Headers @{'X-Smart-Search-Key' = $controlKey} | Out-Null
  if (-not $helperProcess.WaitForExit(5000)) { throw 'Helper did not stop cleanly' }
  Write-Output 'Packaged addon smoke test passed: startup, manifest, real protocol searches and shutdown.'
} finally {
  if ($helperProcess -and -not $helperProcess.HasExited) { $helperProcess.Kill(); $helperProcess.WaitForExit() }
  $resolvedRoot = [IO.Path]::GetFullPath($testRoot)
  $resolvedTemp = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
  if ($resolvedRoot.StartsWith($resolvedTemp, [StringComparison]::OrdinalIgnoreCase) -and (Split-Path -Leaf $resolvedRoot).StartsWith('smart-search-smoke-')) {
    Remove-Item -LiteralPath $resolvedRoot -Recurse -Force
  } else { throw 'Refused cleanup outside the smoke-test directory' }
}
