# Run as Administrator in PowerShell

# Ensure winget is available
Write-Host "Winget not available. Installing App Installer..."
Invoke-WebRequest -Uri "https://aka.ms/getwinget" -OutFile "$env:TEMP\AppInstaller.msixbundle"
Add-AppxPackage "$env:TEMP\AppInstaller.msixbundle"


# Refresh environment variables
$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine")

# Install Git
winget install --id Git.Git -e --source winget

# Install Python
winget install --id Python.Python.3 -e --source winget

# Install Go
winget install --id GoLang.Go -e --source winget

Write-Host "`n✅ All installations complete."