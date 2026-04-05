#!/bin/bash
set -e
export PATH=$PATH:/usr/local/go/bin
cd ~/picoclaw
echo "[rebuild] Building..."
GOFLAGS="-tags=goolm,stdjson" CGO_ENABLED=0 go build -o build/picoclaw ./cmd/picoclaw
echo "[rebuild] Restarting service..."
sudo systemctl restart picoclaw
echo "[rebuild] Done"
