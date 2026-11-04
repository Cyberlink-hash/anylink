#!/bin/sh
set -eu

cd /server
sha256sum -c /tmp/dependencies.sha256
go mod verify

echo "start build"
ldflags="-s -w -X main.appVer=$appVer -X main.commitId=$commitId -X main.buildDate=$(date -Iseconds) -extldflags \"-static\" "
export CGO_ENABLED=1
go build -mod=readonly -v -o anylink -trimpath -ldflags "$ldflags"
sha256sum -c /tmp/dependencies.sha256
