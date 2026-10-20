# Arma el paquete MSIX (sin firmar: la Microsoft Store lo firma al publicarlo).
# Uso: scripts/build-msix.ps1 -Exe out\RebirthTrackUploader-1.2.3.exe -Version 1.2.3 -Out out
param(
  [Parameter(Mandatory)][string]$Exe,
  [Parameter(Mandatory)][string]$Version,
  [string]$Out = "out",
  [string]$IdentityName = $(if ($env:MSIX_IDENTITY_NAME) { $env:MSIX_IDENTITY_NAME } else { "RebirthKnights.RebirthTrackUploader" }),
  [string]$Publisher = $(if ($env:MSIX_PUBLISHER) { $env:MSIX_PUBLISHER } else { "CN=RebirthKnights" }),
  [string]$PublisherDisplayName = $(if ($env:MSIX_PUBLISHER_DISPLAY_NAME) { $env:MSIX_PUBLISHER_DISPLAY_NAME } else { "Rebirth Knights" })
)
$ErrorActionPreference = "Stop"

# La Store exige X.Y.Z.0 (el cuarto número reservado en 0).
$num = ($Version -split '-')[0]
$parts = $num.Split('.')
while ($parts.Count -lt 3) { $parts += '0' }
$msixVersion = "$($parts[0]).$($parts[1]).$($parts[2]).0"

# La versión más nueva del Windows SDK que trae makeappx Y makepri (no todas las carpetas los tienen).
$sdk = Get-ChildItem "${env:ProgramFiles(x86)}\Windows Kits\10\bin\10.*\x64" -Directory |
  Where-Object { (Test-Path (Join-Path $_.FullName "makeappx.exe")) -and (Test-Path (Join-Path $_.FullName "makepri.exe")) } |
  Sort-Object { [version]($_.Parent.Name) } -Descending | Select-Object -First 1
if (-not $sdk) { throw "No encontré el Windows SDK (makeappx/makepri)" }
Write-Host "Windows SDK: $($sdk.FullName)"
$makeappx = Join-Path $sdk.FullName "makeappx.exe"
$makepri = Join-Path $sdk.FullName "makepri.exe"

$stage = Join-Path $env:RUNNER_TEMP "msix-stage"
if (-not $env:RUNNER_TEMP) { $stage = Join-Path ([IO.Path]::GetTempPath()) "msix-stage" }
Remove-Item -Recurse -Force $stage -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Path "$stage\Assets" | Out-Null
Copy-Item $Exe "$stage\RebirthTrackUploader.exe"
Copy-Item "msix\Assets\*" "$stage\Assets\"
Copy-Item "installer\que-hace.txt" "$stage\que-hace.txt"

$manifest = Get-Content "msix\AppxManifest.xml" -Raw -Encoding UTF8
$manifest = $manifest.Replace("__IDENTITY_NAME__", $IdentityName).Replace("__PUBLISHER__", $Publisher).Replace("__PUBLISHER_DISPLAY_NAME__", $PublisherDisplayName).Replace("__VERSION__", $msixVersion)
[IO.File]::WriteAllText("$stage\AppxManifest.xml", $manifest, (New-Object Text.UTF8Encoding $false))

# resources.pri (índice de recursos que espera la Store).
& $makepri createconfig /cf "$stage\priconfig.xml" /dq es-ES /o | Out-Null
& $makepri new /pr $stage /cf "$stage\priconfig.xml" /of "$stage\resources.pri" /o | Out-Null
Remove-Item "$stage\priconfig.xml"

New-Item -ItemType Directory -Force -Path $Out | Out-Null
$msix = Join-Path $Out "RebirthTrackUploader-$Version.msix"
& $makeappx pack /d $stage /p $msix /o
if ($LASTEXITCODE -ne 0) { throw "makeappx falló ($LASTEXITCODE)" }
Write-Host "MSIX: $msix (versión $msixVersion, identidad $IdentityName / $Publisher)"
