#!/usr/bin/env sh
set -eu

go test ./...
go vet ./...
npm --prefix web test
npm --prefix web run build
