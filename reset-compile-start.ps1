Set-Location -Path $PSScriptRoot

# delete old app data roaming files
$appDataPath = Join-Path $env:APPDATA "jxleet"
Remove-Item -Recurse -Force $appDataPath

$local = Join-Path $env:LOCALAPPDATA "jxleet"
Remove-Item -Recurse -Force $local

# remove old binaries, rebuild, and run the new binary
remove-item bin\jxleet.exe
task build
bin\jxleet.exe
