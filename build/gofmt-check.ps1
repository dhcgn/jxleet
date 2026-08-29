$ErrorActionPreference = 'Stop'
$unformatted = gofmt -l internal main.go
if ($unformatted) {
    Write-Host "gofmt needed for:"
    Write-Host $unformatted
    exit 1
}
