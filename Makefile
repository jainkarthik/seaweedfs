.PHONY: test admin-generate admin-build admin-clean admin-dev admin-run admin-test admin-fmt admin-help
.PHONY: build-amd64 build-arm64 gclear version

VERSION		:= $(shell git describe --tags --exact-match 2>/dev/null || shell git describe --tags --always --dirty 2>/dev/null || echo "dev" )
GIT_COMMIT	:= $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
GIT_DATE	:= $(shell git log -1 --format='%ct' 2>/dev/null || echo "0")
BUILD_DATE	:= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
#GO			:= $(shell which go)
GO			?= $(shell which go 2>/dev/null || echo "/home/bryck/karthik/bin/go/bin/go")

#GOFLAGS	:= -a -ldflags="-s -w"
GOFLAGS		:= -a -trimpath
CGO_ENABLED	:= 0

LARGE_DISK_ENABLED	?= true
ifeq ($(LARGE_DISK_ENABLED),true)
GOFLAGS		+= -tags 5BytesOffset
VERSION		:= $(VERSION)-8TB
else
VERSION		:= $(VERSION)-30GB
endif

FIPS_ENABLED ?= false
ifeq ($(FIPS_ENABLED),true)
CGO_ENABLED	= 1
GOFLAGS		+= -tags=fips
endif

BINARY = weed
ADMIN_DIR = weed/admin

SOURCE_DIR = .
debug ?= 0

all: install

install: admin-generate
	cd weed; $(GO) install

warp_install:
	$(GO) install github.com/minio/warp@v0.7.6

full_install: admin-generate
	cd weed; $(GO) install -tags "elastic gocdk sqlite ydb tarantool tikv rclone"

server: install
	weed -v 0 server -s3 -filer -filer.maxMB=64 -volume.max=0 -master.volumeSizeLimitMB=100 -volume.preStopSeconds=1 -s3.port=8000 -s3.allowDeleteBucketNotEmpty=true -s3.config=./docker/compose/s3.json -metricsPort=9324

benchmark: install warp_install
	pkill weed || true
	pkill warp || true
	weed server -debug=$(debug) -s3 -filer -volume.max=0 -master.volumeSizeLimitMB=100 -volume.preStopSeconds=1 -s3.port=8000 -s3.allowDeleteBucketNotEmpty=false -s3.config=./docker/compose/s3.json &
	warp client &
	while ! nc -z localhost 8000 ; do sleep 1 ; done
	warp mixed --host=127.0.0.1:8000 --access-key=some_access_key1 --secret-key=some_secret_key1 --autoterm
	pkill warp
	pkill weed

# curl -o profile "http://127.0.0.1:6060/debug/pprof/profile?debug=1"
benchmark_with_pprof: debug = 1
benchmark_with_pprof: benchmark

test: admin-generate
	cd weed; $(GO) test -tags "elastic gocdk sqlite ydb tarantool tikv rclone" -v ./...

# Admin component targets
admin-generate:
	@echo "Generating admin component templates..."
	@cd $(ADMIN_DIR) && $(MAKE) generate

admin-build: admin-generate
	@echo "Building admin component..."
	@cd $(ADMIN_DIR) && $(MAKE) build

admin-clean:
	@echo "Cleaning admin component..."
	@cd $(ADMIN_DIR) && $(MAKE) clean

admin-dev:
	@echo "Starting admin development server..."
	@cd $(ADMIN_DIR) && $(MAKE) dev

admin-run:
	@echo "Running admin server..."
	@cd $(ADMIN_DIR) && $(MAKE) run

admin-test:
	@echo "Testing admin component..."
	@cd $(ADMIN_DIR) && $(MAKE) test

admin-fmt:
	@echo "Formatting admin component..."
	@cd $(ADMIN_DIR) && $(MAKE) fmt

admin-help:
	@echo "Admin component help..."
	@cd $(ADMIN_DIR) && $(MAKE) help

build-amd64:
	@echo "Building $(BINARY) $(if $(filter true,$(LARGE_DISK_ENABLED)),large disk,standard) version for linux/amd64..."
	@VERSION="$(VERSION)" COMMIT="$(GIT_COMMIT)" BUILD_DATE="$(BUILD_DATE)" \
		GOOS=linux GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) \
		-o $(BINARY)-$(VERSION)-linux-amd64 ./weed

build-arm64:
	@echo "Building $(BINARY) $(if $(filter true,$(LARGE_DISK_ENABLED)),large disk,standard) version for linux/arm64..."
	@VERSION="$(VERSION)" COMMIT="$(GIT_COMMIT)" BUILD_DATE="$(BUILD_DATE)" \
		GOOS=linux GOARCH=arm64 CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) \
		-o $(BINARY)-$(VERSION)-linux-arm64 ./weed

version:
	@echo "VERSION=$(VERSION)"
	@echo "GIT_COMMIT=$(GIT_COMMIT)"
	@echo "BUILD_DATE=$(BUILD_DATE)"
	@echo "FIPS_ENABLED=$(FIPS_ENABLED)"

