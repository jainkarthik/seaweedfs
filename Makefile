.PHONY: test admin-generate admin-build admin-clean admin-dev admin-run admin-test admin-fmt admin-help
.PHONY: build build-amd64 build-arm64 build-fips build-all clean version

BINARY = weed
ADMIN_DIR = weed/admin

SOURCE_DIR = .
debug ?= 0

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
GIT_DATE := $(shell git log -1 --format='%ct' 2>/dev/null || echo "0")
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

GO := go
GOFLAGS := -ldflags="-s -w"
CGO_ENABLED := 1

FIPS_ENABLED ?= false
ifeq ($(FIPS_ENABLED),true)
	CGO_ENABLED = 1
	GOFLAGS += -tags=fips
endif

all: install

install: admin-generate
	cd weed; go install

warp_install:
	go install github.com/minio/warp@v0.7.6

full_install: admin-generate
	cd weed; go install -tags "elastic gocdk sqlite ydb tarantool tikv rclone"

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
	cd weed; go test -tags "elastic gocdk sqlite ydb tarantool tikv rclone" -v ./...

# Admin component targets
admin-generate:
	@echo "Generating admin component templates..."
	@templ generate

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

build: build-amd64

build-amd64:
	@echo "Building $(BINARY) for linux/amd64..."
	@VERSION="$(VERSION)" COMMIT="$(GIT_COMMIT)" BUILD_DATE="$(BUILD_DATE)" \
		GOOS=linux GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) \
		-o $(BINARY)-$(VERSION)-linux-amd64 ./weed

build-arm64:
	@echo "Building $(BINARY) for linux/arm64..."
	@VERSION="$(VERSION)" COMMIT="$(GIT_COMMIT)" BUILD_DATE="$(BUILD_DATE)" \
		GOOS=linux GOARCH=arm64 CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) \
		-o $(BINARY)-$(VERSION)-linux-arm64 ./weed

build-fips: FIPS_ENABLED = true
build-fips: build-amd64 build-arm64
	@mv $(BINARY)-$(VERSION)-linux-amd64 $(BINARY)-$(VERSION)-linux-amd64-fips
	@mv $(BINARY)-$(VERSION)-linux-arm64 $(BINARY)-$(VERSION)-linux-arm64-fips

build-all: build-amd64 build-arm64
	@echo "Build complete. Artifacts:"
	@ls -la $(BINARY)-$(VERSION)-linux-* 2>/dev/null || true

version:
	@echo "VERSION=$(VERSION)"
	@echo "GIT_COMMIT=$(GIT_COMMIT)"
	@echo "BUILD_DATE=$(BUILD_DATE)"
	@echo "FIPS_ENABLED=$(FIPS_ENABLED)"

clean:
	@rm -f $(BINARY)-*-linux-*

SBOM_DIR = sbom
SCAN_DIR = scans

sbom-cyclonedx:
	@echo "Generating CycloneDX SBOM..."
	@mkdir -p $(SBOM_DIR)
	@for bin in $(BINARY)-$(VERSION)-linux-*; do \
		if [ -f "$$bin" ]; then \
			name=$$(basename $$bin); \
			syft $$bin -o cyclonedx-json -file $(SBOM_DIR)/$$name.cdx.json 2>/dev/null || true; \
		fi \
	done

sbom-spdx:
	@echo "Generating SPDX SBOM..."
	@mkdir -p $(SBOM_DIR)
	@for bin in $(BINARY)-$(VERSION)-linux-*; do \
		if [ -f "$$bin" ]; then \
			name=$$(basename $$bin); \
			syft $$bin -o spdx-json -file $(SBOM_DIR)/$$name.spdx.json 2>/dev/null || true; \
		fi \
	done

sbom: sbom-cyclonedx sbom-spdx
	@echo "SBOM generation complete"

scan-trivy:
	@echo "Running Trivy vulnerability scan..."
	@mkdir -p $(SCAN_DIR)
	@for bin in $(BINARY)-$(VERSION)-linux-*; do \
		if [ -f "$$bin" ]; then \
			name=$$(basename $$bin); \
			trivy fs --security-checks vuln,config --severity CRITICAL,HIGH --exit-code 1 --timeout 10m . 2>/dev/null || \
			trivy image --security-checks vuln --severity CRITICAL,HIGH --exit-code 1 --timeout 10m $$bin 2>/dev/null || true; \
		fi \
	done
	@echo "Trivy scan complete"

scan-grype:
	@echo "Running Grype vulnerability scan..."
	@mkdir -p $(SCAN_DIR)
	@for bin in $(BINARY)-$(VERSION)-linux-*; do \
		if [ -f "$$bin" ]; then \
			name=$$(basename $$bin); \
			grype $$bin -o json --file $(SCAN_DIR)/$$name.grype.json 2>/dev/null || true; \
		fi \
	done
	@echo "Grype scan complete"

scan: scan-trivy scan-grype
	@echo "All security scans complete"

archive-artifacts:
	@echo "Archiving artifacts..."
	@mkdir -p artifacts
	@cp -r $(SBOM_DIR) artifacts/ 2>/dev/null || true
	@cp -r $(SCAN_DIR) artifacts/ 2>/dev/null || true
	@for bin in $(BINARY)-$(VERSION)-linux-*; do \
		if [ -f "$$bin" ]; then \
			cp $$bin artifacts/; \
		fi \
	done
	@sha256sum $(BINARY)-$(VERSION)-linux-* > artifacts/CHECKSUMS.txt
	@echo "Archive complete"

sign-artifacts:
	@echo "Signing artifacts with GPG..."
	@mkdir -p artifacts
	@for bin in $(BINARY)-$(VERSION)-linux-*; do \
		if [ -f "$$bin" ]; then \
			gpg --detach-sign --armor $$bin 2>/dev/null || echo "GPG not configured, skipping signature"; \
		fi \
	done
	@if [ -f artifacts/CHECKSUMS.txt ]; then \
		gpg --detach-sign --armor artifacts/CHECKSUMS.txt 2>/dev/null || echo "GPG not configured, skipping checksum signature"; \
	fi
	@echo "Signing complete"

verify-signatures:
	@echo "Verifying GPG signatures..."
	@for bin in $(BINARY)-$(VERSION)-linux-*; do \
		if [ -f "$$bin.asc" ]; then \
			gpg --verify $$bin.asc $$bin 2>/dev/null && echo "Verified: $$bin" || echo "Failed: $$bin"; \
		fi \
	done
	@echo "Verification complete"
