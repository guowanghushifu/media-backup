#!/usr/bin/env bash
set -euo pipefail

mkdir -p dist
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o dist/media-backup-linux-amd64 ./cmd/media-backup
