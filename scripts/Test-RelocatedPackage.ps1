param([Parameter(Mandatory=$true)][string]$Package,[string]$Node='node')
$ErrorActionPreference='Stop'
$from=[IO.Path]::GetFullPath($Package);$parent=Split-Path -Parent $from
if(-not(Test-Path -LiteralPath (Join-Path $from 'manifest.json'))){throw 'Expected package manifest'}
$to=[IO.Path]::GetFullPath((Join-Path $parent ('web-relocated-'+[guid]::NewGuid().ToString('N'))))
if([IO.Path]::GetDirectoryName($to) -cne $parent -or (Test-Path -LiteralPath $to)){throw 'Relocation must be a new sibling directory'}
Move-Item -LiteralPath $from -Destination $to
if(Test-Path -LiteralPath $from){throw 'Original package still exists'}
$manifest=Get-Content -LiteralPath (Join-Path $to 'manifest.json') -Raw|ConvertFrom-Json
$files=@(Get-ChildItem -LiteralPath $to -Recurse -File|Where-Object{$_.FullName -cne (Join-Path $to 'manifest.json')})
if($files.Count -ne $manifest.files.Count){throw 'Relocated inventory count changed'}
foreach($item in $manifest.files){
 $path=[IO.Path]::GetFullPath((Join-Path $to $item.path))
 if(-not $path.StartsWith($to+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)){throw 'Escaping package path'}
 if((Get-Item -LiteralPath $path).Length -ne $item.bytes -or (Get-FileHash -LiteralPath $path).Hash.ToLowerInvariant() -cne $item.sha256){throw 'Relocated package hash changed'}
}
& $Node (Join-Path $PSScriptRoot 'bff-smoke.mjs') --package $to
if($LASTEXITCODE){throw 'Actual relocated BFF test failed'}
Write-Output ('Relocation and actual process verified: '+$to)
