param([Parameter(Mandatory=$true)][string]$Destination,[string]$Pnpm='pnpm')
$ErrorActionPreference='Stop'
$root=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$out=[IO.Path]::GetFullPath($Destination)
if(Test-Path -LiteralPath $out){throw 'Package destination must be new'}
$revision=(& git -C $root rev-parse HEAD).Trim();if($LASTEXITCODE -or @(& git -C $root status --porcelain).Count){throw 'Package requires clean committed source'}
$null=New-Item -ItemType Directory -Path $out
& (Join-Path $PSScriptRoot 'Build.ps1') -Destination (Join-Path $out 'bin')
& $Pnpm --dir (Join-Path $root 'web') install --frozen-lockfile --ignore-scripts
if($LASTEXITCODE){throw 'Web dependency install failed'}
& $Pnpm --dir (Join-Path $root 'web') exec tsc --noEmit
if($LASTEXITCODE){throw 'Web typecheck failed'}
& $Pnpm --dir (Join-Path $root 'web') exec vite build --outDir (Join-Path $out 'web')
if($LASTEXITCODE){throw 'Web build failed'}
$null=New-Item -ItemType Directory -Path (Join-Path $out 'deploy')
foreach($file in @('release-contract.json','service.example.json')){Copy-Item -LiteralPath (Join-Path $root ('deploy/'+$file)) -Destination (Join-Path $out ('deploy/'+$file))}
$examplePath=Join-Path $out 'deploy/service.example.json'
$example=Get-Content -LiteralPath $examplePath -Raw|ConvertFrom-Json -AsHashtable
$example.FUTURES_WEB_DIR='web'
[IO.File]::WriteAllText($examplePath,(ConvertTo-Json $example))
Copy-Item -LiteralPath (Join-Path $root 'README.md') -Destination (Join-Path $out 'README.md')
Copy-Item -LiteralPath (Join-Path $root 'source-provenance.json') -Destination (Join-Path $out 'source-provenance.json')
if(@(Get-ChildItem -LiteralPath $out -Recurse -Force|Where-Object{$_.Attributes -band [IO.FileAttributes]::ReparsePoint}).Count){throw 'Portable Web package cannot contain links'}
if(@(& git -C $root status --porcelain).Count -or (& git -C $root rev-parse HEAD).Trim() -cne $revision){throw 'Source changed during packaging'}
$files=@(Get-ChildItem -LiteralPath $out -Recurse -File | ForEach-Object {@{path=[IO.Path]::GetRelativePath($out,$_.FullName).Replace('\','/');bytes=$_.Length;sha256=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()}} | Sort-Object path)
$manifest=@{format=1;repository='agent-os-futures';commit=$revision;role='futures';files=$files}
[IO.File]::WriteAllText((Join-Path $out 'manifest.json'),(ConvertTo-Json $manifest -Depth 8))
Write-Output ('Complete Web release: '+$out)
