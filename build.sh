#!/usr/bin/env bash
set -euo pipefail

mkdir -p dist
go mod tidy
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o dist/media-backup-linux-amd64 ./cmd/media-backup
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o dist/media-backup-linux-arm64 ./cmd/media-backup
cp install-systemd-service.sh dist/install-systemd-service.sh
chmod +x dist/install-systemd-service.sh
