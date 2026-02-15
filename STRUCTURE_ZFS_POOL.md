# SeaweedFS on ZFS Pool Structure

## Overview

This guide covers configuring SeaweedFS data storage on OpenZFS for a high-performance environment:
- 32 NVMe drives × 8TB in RAIDz configuration
- Total raw storage: ~250TB
- 100Gbps network (NVIDIA BlueField DPU)
- Replication: 000 (no replication - rely on ZFS for data protection)

---

## 1. Data Directory Configuration

### Component Data Locations

| Component | Flag | Default | What It Stores |
|-----------|------|---------|----------------|
| **Master** | `-mdir` or `[master.metaStore].dir` | `/var/lib/weed/master` | Cluster topology, volume location metadata, Raft logs |
| **Volume** | `-dir` | `/tmp/weed` | Actual data chunks (blobs) |
| **Filer** | `[leveldb.local].dir` or `[filer.store].dir` | `./filerldb2` | File metadata (directories, attributes) |

### How to Configure

Edit the config files in `/etc/weed/` or use the example configs:

```toml
# /etc/weed/master-1.toml
[master.metaStore]
  dir = "/tank/seaweedfs/master/meta"

# /etc/weed/volume-1.toml
[volume]
  # Multiple dirs separated by comma (see Section 2 below)
  dirs = ["/tank/seaweedfs/volumes/disk1", "/tank/seaweedfs/volumes/disk2"]

# /etc/weed/filer-1.toml
[leveldb]
  local = "/tank/seaweedfs/filer/leveldb"
```

---

## 2. Multiple Folders for Volume Server

SeaweedFS supports **multiple directories** per volume server instance for better I/O parallelism.

### Configuration Options

**Option A: Via command line (comma-separated)**

```bash
weed volume \
  -dir=/tank/vol/disk1,/tank/vol/disk2,/tank/vol/disk3,/tank/vol/disk4 \
  -max=1000,1000,1000,1000 \
  -port=8080 \
  -master=localhost:9333
```

**Option B: Via config file**

```toml
# /etc/weed/volume-1.toml
[volume]
  dirs = [
    "/tank/seaweedfs/volumes/disk1",
    "/tank/seaweedfs/volumes/disk2",
    "/tank/seaweedfs/volumes/disk3",
    "/tank/seaweedfs/volumes/disk4"
  ]

# Optional: different max volumes per folder
[volume.folderMaxLimits]
  max = [2000, 2000, 2000, 2000]
```

### Why Multiple Folders?

- **Parallel I/O**: Distributes read/write across multiple disk paths
- **Capacity management**: Each folder can have different max limits
- **Failure isolation**: Issues in one folder don't affect others

---

## 3. LevelDB for Filer Metadata

The Filer stores file metadata (paths, permissions, extended attributes). LevelDB is recommended for high-throughput workloads.

### Configuration

```toml
# /etc/weed/filer-1.toml
[filer]
  [filer.port]
    port = 8888
  [filer.bind]
    host = "0.0.0.0"
  [filer.master]
    master = "localhost:9333"

# LevelDB configuration (recommended for high performance)
[leveldb]
  local = "/tank/seaweedfs/filer/leveldb"

# Alternative: postgres (if already running)
# [filer.store]
#   provider = "postgres"
#   hostname = "localhost"
#   port = 5432
#   username = "seaweed"
#   password = "secret"
#   database = "seaweedfs"
```

---

## 4. Network Considerations (100Gbps BlueField DPU)

With 100Gbps networking, the bottleneck shifts to storage I/O. Optimize accordingly:

### Network Tuning

```bash
# Increase network buffer sizes
ethtool -G eth0 rx 4096 tx 4096

# Set IRQ affinity to CPU cores
# Configure in /etc/irqbalance/irqbalance.conf

# Enable jumbo frames
ifconfig eth0 mtu 9000
```

### SeaweedFS Tuning for High-Speed Network

```toml
# /etc/weed/volume-1.toml
[volume]
  [volume.port]
    port = 8080
  # Increase concurrent connections
  [volume.concurrentUploadOption]
    max = 1024
  
# /etc/weed/filer-1.toml  
[filer]
  # Increase filer grpc workers
  [filer.grpc]
    workers = 256
```

---

## 5. ZFS Pool Recommendations

### Your Setup
- 32 NVMe drives × 8TB = 256TB raw
- RAIDz3 (parity for 3 drive failures)
- Usable: ~200-220TB (after parity)

### Pool Design

Given the high-speed network and NVMe drives, we recommend:

| Dataset | ZFS Dataset | Purpose | Compression |
|---------|-------------|---------|-------------|
| **Master Metadata** | `tank/seaweedfs/master` | Raft logs, topology | lz4 |
| **Filer Metadata** | `tank/seaweedfs/filer` | LevelDB files | lz4 |
| **Volume Data** | `tank/seaweedfs/volumes` | Data chunks | lz4 |
| **Logs** | `tank/seaweedfs/logs` | SeaweedFS logs | lz4 |

### Create the Pool

```bash
# Single large RAIDz3 pool (recommended for NVMe)
zpool create -f \
  -o ashift=12 \
  -O compression=lz4 \
  -O atime=off \
  -O recordsize=1M \
  -O sync=standard \
  tank \
  raidz3 \
  /dev/nvme0n1 /dev/nvme1n1 /dev/nvme2n1 /dev/nvme3n1 \
  /dev/nvme4n1 /dev/nvme5n1 /dev/nvme6n1 /dev/nvme7n1 \
  /dev/nvme8n1 /dev/nvme9n1 /dev/nvme10n1 /dev/nvme11n1 \
  /dev/nvme12n1 /dev/nvme13n1 /dev/nvme14n1 /dev/nvme15n1 \
  /dev/nvme16n1 /dev/nvme17n1 /dev/nvme18n1 /dev/nvme19n1 \
  /dev/nvme20n1 /dev/nvme21n1 /dev/nvme22n1 /dev/nvme23n1 \
  /dev/nvme24n1 /dev/nvme25n1 /dev/nvme26n1 /dev/nvme27n1 \
  /dev/nvme28n1 /dev/nvme29n1 /dev/nvme30n1 /dev/nvme31n1

# Create datasets
zfs create tank/seaweedfs
zfs create tank/seaweedfs/master
zfs create tank/seaweedfs/filer
zfs create tank/seaweedfs/volumes
zfs create tank/seaweedfs/logs

# Create volume subdirectories for multiple folders
zfs create tank/seaweedfs/volumes/disk1
zfs create tank/seaweedfs/volumes/disk2
zfs create tank/seaweedfs/volumes/disk3
zfs create tank/seaweedfs/volumes/disk4
```

### ZFS Properties for NVMe

```bash
# Recommended properties
zfs set ashift=12 tank
zfs set compression=lz4 tank
zfs set atime=off tank
zfs set recordsize=1M tank/seaweedfs/volumes
zfs set sync=standard tank  # standard (default) for data integrity
zfs set primarycache=metadata tank/seaweedfs/filer  # cache metadata for leveldb
zfs set secondarycache=metadata tank/seaweedfs/filer
```

---

## 6. Final Directory Layout

```
tank/
└── seaweedfs/
    ├── master/           # Master metadata (-mdir)
    │   └── meta/
    ├── filer/            # Filer metadata (LevelDB)
    │   └── leveldb/
    ├── volumes/          # Volume data (multiple folders)
    │   ├── disk1/
    │   ├── disk2/
    │   ├── disk3/
    │   └── disk4/
    └── logs/             # System logs
        └── weed/
```

---

## 7. Complete Example Configs

### master-1.toml
```toml
[master]
  [master.port]
    port = 9333
  [master.bind]
    host = "0.0.0.0"
  [master.metaStore]
    dir = "/tank/seaweedfs/master/meta"
  [master.volumeStore]
    dir = "/tank/seaweedfs/volumes"
  [master.cluster]
    replicas = 1

[heartbeat]
  interval = 30

[raft]
  heartbeat = 30
  election = 65
```

### volume-1.toml
```toml
[volume]
  [volume.port]
    port = 8080
  [volume.bind]
    host = "0.0.0.0"
  [volume.dataCenter]
    name = "dc1"
  [volume.rack]
    name = "rack1"
  [volume.server]
    name = "volume-1"
  [volume.filer]
    default = "localhost:8888"
  [volume.master]
    volume = "localhost:9333"
    replication = "000"
    collection = "weed"

[volume.folderMaxLimits]
  max = [5000, 5000, 5000, 5000]

[disk]
  [disk.os]
    atime = false
```

### filer-1.toml
```toml
[filer]
  [filer.port]
    port = 8888
  [filer.bind]
    host = "0.0.0.0"
  [filer.master]
    master = "localhost:9333"

[leveldb]
  local = "/tank/seaweedfs/filer/leveldb"
```

---

## 8. Important Notes

### Data Protection
- With replication `000`, ZFS RAIDz3 provides protection against 3 drive failures
- Enable ZFS snapshots for backup: `zfs snapshot tank/seaweedfs/volumes@daily`
- Consider periodic `zfs send/receive` for offsite backup

### Performance Tips
- Use `recordsize=1M` for volume data (large sequential writes)
- Use `recordsize=128k` for filer metadata (smaller random writes)
- Keep leveldb on separate dataset from volume data
- Monitor with `zpool iostat -v tank 1`

### Monitoring
```bash
# ZFS health
zpool status tank

# I/O statistics
zpool iostat tank 1

# ARC statistics
arcstat 1
```
