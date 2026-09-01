# CI runs the same lint via golangci/golangci-lint-action. Locally the lint
# step is skipped when the binary is not installed, so `task check` keeps
# working on machines without it.
$ErrorActionPreference = 'Stop'
if (-not (Get-Command golangci-lint -ErrorAction SilentlyContinue)) {
    Write-Host 'golangci-lint not installed - skipping lint (install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest)'
    exit 0
}
golangci-lint run
exit $LASTEXITCODE
