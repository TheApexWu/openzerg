#!/bin/bash
# Quick start for OpenZerg backend
# Usage: ./run.sh [--reload]
set -euo pipefail
cd "$(dirname "$0")"
pip install -r requirements.txt 2>/dev/null
uvicorn controller:app --host 0.0.0.0 --port 8000 "$@"
