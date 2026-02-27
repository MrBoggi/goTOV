param (
    [Parameter(Mandatory=$true)]
    [string]$Name
)

# Get current date in correct format
$timestamp = Get-Date -Format "yyyyMMddHHmmss"
$fileName = "${timestamp}_${Name}.sql"

# Define path (adjusting relative to compilation context if needed, but absolute is safer in scripts)
# Assuming script runs from root or has relative access. 
# We'll assume this is being run from project root based on instructions.
$migrationDir = Join-Path "database" "migrations"

# Create directory if it doesn't exist
if (-not (Test-Path $migrationDir)) {
    New-Item -ItemType Directory -Force -Path $migrationDir | Out-Null
}

$fullPath = Join-Path $migrationDir $fileName

# Create the file
New-Item -ItemType File -Path $fullPath -Value "-- Migration: $Name`n-- Created: $(Get-Date)`n`n" | Out-Null

Write-Host "Created migration file: $fullPath"
