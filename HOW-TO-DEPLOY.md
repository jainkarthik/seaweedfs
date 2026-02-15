# SeaweedFS Deployment Manual

## Table of Contents
1. [Prerequisites](#prerequisites)
2. [Building from Source](#building-from-source)
3. [Building DEB Packages](#building-deb-packages)
4. [Building RPM Packages](#building-rpm-packages)
5. [Installation](#installation)
6. [Configuration](#configuration)
7. [Running Services](#running-services)

---

## Prerequisites

### System Requirements
- Ubuntu 22.04 (for DEB) or Oracle Linux 8/9 (for RPM)
- Go 1.21+ (for building from source)
- Docker (optional, for containerized builds)

### Required Tools

**For DEB packages (Ubuntu):**
```bash
sudo apt-get update
sudo apt-get install -y \
    build-essential \
    dpkg-dev \
    debhelper \
    dh-systemd \
    pkg-config
```

**For RPM packages (Oracle Linux / Rocky Linux):**
```bash
sudo dnf install -y \
    rpm-build \
    dpkg-dev \
    gcc \
    go \
    make
```

---

## Building from Source

### 1. Clone the Repository
```bash
git clone https://github.com/seaweedfs/seaweedfs.git
cd seaweedfs
```

### 2. Build Standard Binary
```bash
# Build for current architecture
make build

# Build for specific architecture
make build-amd64    # Linux amd64
make build-arm64    # Linux arm64

# Build with FIPS support
make build-fips     # Builds both amd64 and arm64 FIPS binaries
```

### 3. Verify Build
```bash
# Check version
./weed-*-linux-amd64 version

# Output example:
# version 4.12 dev linux amd64
```

---

## Building DEB Packages

### Method 1: Using Makefile (Recommended)

```bash
# Build the binary first
make build-amd64

# The binary will be created as: weed-<VERSION>-linux-amd64
```

### Method 2: Full DEB Package Build

```bash
# Navigate to packaging directory
cd packaging/debian

# Set version
export VERSION=4.12

# Copy built binary to build directory
mkdir -p build
cp ../../weed-${VERSION}-linux-amd64 build/weed

# Install build dependencies
sudo apt-get install -y dpkg-dev debhelper dh-systemd

# Build the package
dpkg-buildpackage -b -uc -us

# The .deb file will be created in the parent directory
ls -la ../*.deb
```

### Method 3: Using Docker (Cross-architecture)

```bash
# Build for amd64
docker run --rm \
    -v $(pwd):/src \
    -w /src \
    ubuntu:22.04 \
    bash -c "apt-get update && apt-get install -y dpkg-dev debhelper && make build-amd64 && cd packaging/debian && dpkg-buildpackage -b -uc -us"

# Copy resulting .deb files
ls -la *.deb
```

---

## Building RPM Packages

### Method 1: Using Docker (Recommended for consistency)

```bash
# For Oracle Linux 9
docker run --rm \
    -v $(pwd):/src \
    -w /src \
    rockylinux:9 \
    bash -c "dnf install -y rpmbuild gcc go make && make build-amd64"

# Set up rpmbuild directory
mkdir -p ~/rpmbuild/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}

# Copy required files
cp weed-*-linux-amd64 ~/rpmbuild/SOURCES/
cp packaging/rpm/seaweedfs.spec ~/rpmbuild/SPECS/
cp -r packaging/debian/* ~/rpmbuild/SOURCES/

# Build RPM
rpmbuild -bb ~/rpmbuild/SPECS/seaweedfs.spec

# Find the RPM
find ~/rpmbuild/RPMS -name "*.rpm"
```

### Method 2: Direct Build (on RHEL/CentOS/Rocky)

```bash
# Install dependencies
sudo dnf install -y rpmbuild gcc go make

# Create rpmbuild directory structure
mkdir -p ~/rpmbuild/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}

# Set version
export VERSION=4.12

# Copy binary
cp weed-${VERSION}-linux-amd64 ~/rpmbuild/SOURCES/

# Copy spec file
cp packaging/rpm/seaweedfs.spec ~/rpmbuild/SPECS/

# Copy supporting files
cp -r packaging/debian/* ~/rpmbuild/SOURCES/

# Build
rpmbuild -bb ~/rpmbuild/SPECS/seaweedfs.spec
```

---

## Installation

### DEB Package Installation (Ubuntu)

```bash
# Install the package
sudo dpkg -i seaweedfs_4.12_amd64.deb

# Or with apt (after copying to repo)
sudo apt-get update
sudo apt-get install seaweedfs
```

### RPM Package Installation (Oracle Linux)

```bash
# Install the package
sudo dnf install seaweedfs-4.12-1.el9.x86_64.rpm

# Or
sudo rpm -ivh seaweedfs-4.12-1.el9.x86_64.rpm
```

### Manual Installation (No Package)

```bash
# Extract binary
sudo cp weed-*-linux-amd64 /opt/weed/weed
sudo chmod +x /opt/weed/weed

# Copy systemd units
sudo cp packaging/debian/weed-*.service /etc/systemd/system/

# Copy config
sudo cp packaging/debian/weed.conf /etc/default/weed/
sudo mkdir -p /etc/weed
sudo cp packaging/debian/examples/*.toml /etc/weed/

# Copy logrotate
sudo cp packaging/debian/logrotate.d/weed /etc/logrotate.d/

# Reload systemd
sudo systemctl daemon-reload
```

---

## Configuration

### 1. Edit Default Configuration

```bash
sudo nano /etc/default/weed/weed.conf
```

```bash
# /etc/default/weed/weed.conf
MASTER_INSTANCES=1
VOLUME_INSTANCES=1
FILER_INSTANCES=1
S3_INSTANCES=1
FIPS_ENABLED=false
```

### 2. Configure ZFS Data Directories

Update your config files to point to ZFS datasets:

```bash
# Create ZFS datasets (on your OpenZFS server)
sudo zfs create tank/seaweedfs
sudo zfs create tank/seaweedfs/master
sudo zfs create tank/seaweedfs/filer
sudo zfs create tank/seaweedfs/volumes
```

```bash
# Edit master config
sudo nano /etc/weed/master-1.toml
```

```toml
[master.metaStore]
  dir = "/tank/seaweedfs/master"
```

```bash
# Edit volume config (with multiple folders)
sudo nano /etc/weed/volume-1.toml
```

```toml
[volume]
  dirs = [
    "/tank/seaweedfs/volumes/disk1",
    "/tank/seaweedfs/volumes/disk2",
    "/tank/seaweedfs/volumes/disk3",
    "/tank/seaweedfs/volumes/disk4"
  ]

[volume.folderMaxLimits]
  max = [5000, 5000, 5000, 5000]
```

```bash
# Edit filer config
sudo nano /etc/weed/filer-1.toml
```

```toml
[leveldb]
  local = "/tank/seaweedfs/filer/leveldb"
```

### 3. Set Permissions

```bash
# Create weed user (if not created by package)
sudo useradd -r -s /sbin/nologin weed

# Set ownership
sudo chown -R weed:weed /opt/weed
sudo chown -R weed:weed /tank/seaweedfs
sudo chown -R weed:weed /var/log/weed
```

---

## Running Services

### Start Services Manually

```bash
# Start Master
sudo systemctl start weed-master@1

# Start Volume Server
sudo systemctl start weed-volume@1

# Start Filer
sudo systemctl start weed-filer@1

# Start S3 API (optional)
sudo systemctl start weed-s3@1
```

### Enable Services on Boot

```bash
# Enable services
sudo systemctl enable weed-master@1
sudo systemctl enable weed-volume@1
sudo systemctl enable weed-filer@1
sudo systemctl enable weed-s3@1
```

### Check Service Status

```bash
# Check status
sudo systemctl status weed-master@1

# View logs
sudo journalctl -u weed-master@1 -f
```

### Multiple Instances

For multiple instances, enable additional units:

```bash
# For 2 volume servers
sudo systemctl enable weed-volume@1
sudo systemctl enable weed-volume@2
sudo systemctl start weed-volume@1
sudo systemctl start weed-volume@2
```

---

## Quick Start Script

Create a deployment script:

```bash
#!/bin/bash
set -e

VERSION=${VERSION:-4.12}
ZFS_POOL=${ZFS_POOL:-tank}

echo "=== SeaweedFS Deployment ==="
echo "Version: $VERSION"
echo "ZFS Pool: $ZFS_POOL"

# Create ZFS datasets
echo "Creating ZFS datasets..."
sudo zfs create -o mountpoint=/tank/seaweedfs $ZFS_POOL/seaweedfs 2>/dev/null || true
sudo zfs create $ZFS_POOL/seaweedfs/master
sudo zfs create $ZFS_POOL/seaweedfs/filer
sudo zfs create $ZFS_POOL/seaweedfs/volumes/disk1
sudo zfs create $ZFS_POOL/seaweedfs/volumes/disk2
sudo zfs create $ZFS_POOL/seaweedfs/volumes/disk3
sudo zfs create $ZFS_POOL/seaweedfs/volumes/disk4
sudo zfs create $ZFS_POOL/seaweedfs/logs

# Build binary
echo "Building SeaweedFS..."
make build-amd64

# Install binary
echo "Installing binary..."
sudo cp weed-${VERSION}-linux-amd64 /opt/weed/weed
sudo chmod +x /opt/weed/weed

# Set permissions
sudo useradd -r -s /sbin/nologin weed 2>/dev/null || true
sudo chown -R weed:weed /opt/weed /tank/seaweedfs

# Copy configs
echo "Configuring SeaweedFS..."
sudo cp packaging/debian/examples/*.toml /etc/weed/

# Update config with ZFS paths
sudo sed -i 's|/var/lib/weed/master|/tank/seaweedfs/master|' /etc/weed/master-1.toml
sudo sed -i 's|/var/lib/weed/filer|/tank/seaweedfs/filer|' /etc/weed/filer-1.toml

# Copy systemd units
sudo cp packaging/debian/weed-*.service /etc/systemd/system/

# Reload and start
echo "Starting services..."
sudo systemctl daemon-reload
sudo systemctl enable weed-master@1 weed-volume@1 weed-filer@1
sudo systemctl start weed-master@1 weed-volume@1 weed-filer@1

echo "=== Deployment Complete ==="
sudo systemctl status weed-master@1 weed-volume@1 weed-filer@1
```

---

## Troubleshooting

### Service Won't Start

```bash
# Check logs
sudo journalctl -xe

# Check config syntax
/opt/weed/weed master -config=/etc/weed/master-1.toml -check
```

### Permission Issues

```bash
# Fix ownership
sudo chown -R weed:weed /opt/weed /tank/seaweedfs /var/log/weed

# Check permissions
ls -la /opt/weed/
```

### ZFS Dataset Issues

```bash
# Check ZFS health
sudo zpool status

# Check dataset mountpoints
sudo zfs get mountpoint tank/seaweedfs
```

---

## Next Steps

- See [STRUCTURE_ZFS_POOL.md](./STRUCTURE_ZFS_POOL.md) for ZFS optimization details
- Configure replication: Update `replication = "000"` in volume config
- Set up monitoring: Configure Prometheus metrics endpoint
- Configure backups: Use ZFS snapshots (`zfs snapshot tank/seaweedfs/volumes@backup`)
