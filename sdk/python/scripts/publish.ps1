# Publish mcache-py to PyPI / TestPyPI (PowerShell version).
#
# Usage:
#   .\scripts\publish.ps1 test      # → TestPyPI
#   .\scripts\publish.ps1           # → PyPI (production)
#
# Requires:
#   pip install --upgrade build twine
#   ~/.pypirc configured, or PYPI_API_TOKEN env var set.

param(
    [Parameter(Position = 0)]
    [string]$Target = "pypi"
)

$ErrorActionPreference = "Stop"

# cd to project root (sdk/python)
Set-Location -Path (Split-Path -Parent $PSScriptRoot)

Write-Host "==> Cleaning old build artifacts" -ForegroundColor Cyan
Remove-Item -Recurse -Force -ErrorAction SilentlyContinue build, dist, *.egg-info, mcache/*.egg-info

Write-Host "==> Building distribution" -ForegroundColor Cyan
python -m build
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "==> Validating with twine" -ForegroundColor Cyan
python -m twine check dist/*
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

switch ($Target.ToLower()) {
    { $_ -in "test", "testpypi" } {
        Write-Host "==> Uploading to TestPyPI" -ForegroundColor Yellow
        python -m twine upload --repository testpypi dist/*
        Write-Host ""
        Write-Host "Test install:"
        Write-Host "  pip install --index-url https://test.pypi.org/simple/ mcache-py"
    }
    { $_ -in "pypi", "prod", "production", "" } {
        Write-Host "==> Uploading to PyPI (production)" -ForegroundColor Red
        $confirm = Read-Host "Type 'yes' to confirm production release"
        if ($confirm -ne "yes") {
            Write-Host "Aborted." -ForegroundColor Yellow
            exit 1
        }
        python -m twine upload dist/*
        Write-Host ""
        Write-Host "Install:"
        Write-Host "  pip install mcache-py"
    }
    default {
        Write-Host "Unknown target: $Target (use 'test' or 'pypi')" -ForegroundColor Red
        exit 1
    }
}

Write-Host "==> Done." -ForegroundColor Green
