Get-ChildItem -Path $PSScriptRoot -Filter *.exe | ForEach-Object {
    $exe = $_.FullName
    $helpText = & $exe --help 2>&1
    $helpFile = Join-Path -Path $PSScriptRoot -ChildPath "$($_.BaseName)-help.txt"
    $helpText | Out-File -FilePath $helpFile -Encoding UTF8
}