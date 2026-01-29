#!/bin/bash
set -e

USER_DIR=karthik
pushd ~/$USER_DIR
if [ ! -d "go" ]; then
    echo "Downloading Go..."
    wget -c https://go.dev/dl/go1.25.6.linux-amd64.tar.gz
    echo "Extracting Go..."
    tar -xvf go1.25.6.linux-amd64.tar.gz
else
    echo "Go folder already exists. Skipping extraction."
fi
popd

export PATH=$PATH:~/$USER_DIR/go/bin

# ## Clean build caches
# go clean -cache -modcache -testcache -i
# du -sh $(go env GOCACHE) && go clean -cache -testcache && echo "Cache Purged"

ARCH=$(uname -m)

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

#export CGO_ENABLED=0
unset CGO_ENABLED

# Execute the build
go build \
  -a \
  -v \
  -tags 5BytesOffset \
  -ldflags="-s -w" \
  -o "weed-$GOARCH" ../../weed

echo "Build complete: weed-$GOARCH"

# go build \
#   -a \
#   -v \
#   -tags 5BytesOffset \
#   -gcflags='all=-N -l' \
#   -o "weed-$GOARCH-debug" ../../weed

# echo "Build complete: weed-$GOARCH-debug"

