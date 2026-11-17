#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
(cd web && npm run build)
export CGO_ENABLED=0
go build -trimpath -o n-manager ./cmd/n-manager
echo "built $(pwd)/n-manager"
