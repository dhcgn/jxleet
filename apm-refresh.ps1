# Check if APM is installed
if (-not (Get-Command apm -ErrorAction SilentlyContinue)) {
    Write-Host "APM is not installed. Please install APM before running this script."
    Write-Host "You can install APM with: " -NoNewline
    Write-Host "irm https://aka.ms/apm-windows | iex" -ForegroundColor Cyan
    exit 1
}

# Refresh APM dependencies to the latest matching refs
apm update

# Install APM, MCP, and LSP dependencies
apm install