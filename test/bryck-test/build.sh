#!/bin/bash
set -e

ARCH=$(uname -m)

export CGO_ENABLED=1


if [[ "$ARCH" == "x86_64" ]]; then
    echo "Configuring build for Intel/AMD64..."
    export GOARCH=amd64
    export GOAMD64=v3
elif [[ "$ARCH" == "aarch64" || "$ARCH" == "arm64" ]]; then
    echo "Configuring build for ARM64..."
    export GOARCH=arm64
    # GOAMD64 is unset here as it is invalid for ARM
    unset GOAMD64
else
    echo "Unsupported architecture: $ARCH"
    exit 1
fi

# Execute the build
go build \
  -a \
  -tags 5BytesOffset \
  -gcflags='all=-N -l' \
  -o "weed-$GOARCH-debug" ../../weed

echo "Build complete: weed-$GOARCH-debug"

go build \
  -a \
  -tags 5BytesOffset \
  -ldflags="-s -w" \
  -buildmode=pie \
  -pgo=auto \
  -o "weed-$GOARCH" ../../weed

echo "Build complete: weed-$GOARCH"


