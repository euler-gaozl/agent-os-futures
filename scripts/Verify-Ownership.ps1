$ErrorActionPreference='Stop'
$root=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$saved=$env:GOWORK;$env:GOWORK='off'
try{
 Push-Location $root
 try{
  $modules=& go list -mod=vendor -deps -f '{{if .Module}}{{.Module.Path}}{{end}}' ./...
  if($LASTEXITCODE){throw 'Cannot inspect dependency closure'}
  $owners=@($modules|Sort-Object -Unique|Where-Object{$_ -like 'github.com/euler-gaozl/agent-os*'})
  if(@(Compare-Object @('github.com/euler-gaozl/agent-os-futures','github.com/euler-gaozl/agent-os-sdk') $owners).Count){throw 'Unreviewed owner implementation dependency'}
  foreach($file in Get-ChildItem cmd,futures,internal -Filter *.go -File -Recurse){
   $source=Get-Content -LiteralPath $file.FullName -Raw
   if($source -match '(?i)(SELECT\s+.+FROM|INSERT\s+INTO|CREATE\s+TABLE|pgxpool\.(New|NewWithConfig)|\.QueryRow\(|\.BeginTx\()'){throw ('Database code in Web owner: '+$file.Name)}
  }
  if(Test-Path -LiteralPath cmd/migrate){throw 'Web owner must not migrate a database'}
  Write-Output 'Web ownership: only own module and SDK; no DB/migration implementation.'
 }finally{Pop-Location}
}finally{$env:GOWORK=$saved}
