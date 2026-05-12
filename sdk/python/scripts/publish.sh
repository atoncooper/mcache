#!/usr/bin/env bash
# Publish mcache-py to PyPI / TestPyPI.
#
# Usage:
#   ./scripts/publish.sh test      # → TestPyPI
#   ./scripts/publish.sh           # → PyPI (production)
#
# Requires:
#   pip install --upgrade build twine
#   ~/.pypirc configured, or PYPI_API_TOKEN env var set.

set -euo pipefail

cd "$(dirname "$0")/.."

TARGET="${1:-pypi}"

echo "==> Cleaning old build artifacts"
rm -rf build/ dist/ *.egg-info mcache/*.egg-info

echo "==> Building distribution"
python -m build

echo "==> Validating with twine"
python -m twine check dist/*

case "$TARGET" in
    test|testpypi)
        echo "==> Uploading to TestPyPI"
        python -m twine upload --repository testpypi dist/*
        echo
        echo "Test install:"
        echo "  pip install --index-url https://test.pypi.org/simple/ mcache-py"
        ;;
    pypi|prod|production|"")
        echo "==> Uploading to PyPI (production)"
        read -r -p "Type 'yes' to confirm production release: " confirm
        if [ "$confirm" != "yes" ]; then
            echo "Aborted."
            exit 1
        fi
        python -m twine upload dist/*
        echo
        echo "Install:"
        echo "  pip install mcache-py"
        ;;
    *)
        echo "Unknown target: $TARGET (use 'test' or 'pypi')"
        exit 1
        ;;
esac

echo "==> Done."
