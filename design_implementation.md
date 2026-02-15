# Enterprise SeaweedFS 4.12 Deployment Proposal
High-Performance Object Storage on BlueField DPU with 250TB+ NVMe RAIDz

Version: 1.0

Date: February 2025

Target Audience: Senior Engineers, VP of Backend, CTO

Proposed Version: SeaweedFS 4.12 (Latest Stable)

## Executive Summary
This proposal outlines the architecture for deploying SeaweedFS 4.12 on a high-performance storage node featuring NVIDIA BlueField-3 DPU with 100Gbps networking, 32+ NVMe drives in RAIDz configuration, and 250TB+ raw capacity. The deployment targets near 100Gbps read throughput and minimum 40Gbps write throughput with 000 replication mode, designed for production workloads exceeding 5 million objects per bucket.

Key Highlights:
- Hardware: BlueField-3 DPU (100Gbps) + 32x NVMe Gen4/Gen5 drives
- Storage: ZFS RAIDz2/RAIDz3 with optimized recordsize for SeaweedFS
- Topology: Decoupled services (Master, Volume, Filer, S3 Gateway)
- HA Design: Multi-instance S3 with HAProxy load balancing
- Observability: Real-time bucket analytics without rclone traversal

## 1. Hardware & Infrastructure Overview
### 1.1 Compute & Network
```
Component              Specification
─────────────────────────────────────────────────────────────
DPU                    NVIDIA BlueField-3 (B3220)
├─ Arm Cores           16x Armv8.2+A78 @ 2.4GHz
├─ DRAM                32GB DDR5
├─ Network             2x 100Gbps QSFP112 (ConnectX-7)
├─ PCIe                Gen5 x16 to host
└─ Offloads            RDMA, NVMe-oF, crypto, compression

Host Server            HPE/Dell/Supermicro 2U
├─ CPU                 2x AMD EPYC 9654 (96c/192t)
├─ DRAM                512GB DDR5-4800
├─ PCIe Switch         Broadcom PEX8900 (Gen5)
└─ NVMe Backplane      32x U.2/U.3 bays
```
### 1.2 Storage Configuration
```
Raw Capacity:           32 x 15.36TB NVMe Gen5 = 491.52TB
ZFS Configuration:      4 x vdevs of 8-drive RAIDz2
Effective Capacity:     ~360TB (22% overhead RAIDz2 + 10% ZFS reserve)
Recordsize:             1MB (optimal for SeaweedFS chunks)
Compression:            zstd-3 (SeaweedFS compresses, ZFS light compression)
Checksum:               edonr (performance optimized)
ARC:                    128GB max (leave RAM for SeaweedFS)
L2ARC:                  Disabled (NVMe latency makes this redundant)
ZIL/SLOG:               Mirror of 2x Optane P5800X 400GB
```
### 1.3 ZPool Layout
```bash
# Create 4 zpools for parallel I/O
zpool create -o ashift=12 -O recordsize=1M -O compression=zstd-3 \
    -O checksum=edonr -O atime=off -O xattr=sa \
    -O mountpoint=/seaweedfs/volumes/zpool1 \
    seaweedfs-1 raidz2 nvme0n1 nvme1n1 nvme2n1 nvme3n1 nvme4n1 nvme5n1 nvme6n1 nvme7n1

# Repeat for zpool2 (nvme8-15), zpool3 (nvme16-23), zpool4 (nvme24-31)

# ZIL on Optane
zpool add seaweedfs-1 log mirror /dev/nvme32n1 /dev/nvme33n1
```

## 2. SeaweedFS Architecture & Service Topology
### 2.1 Service Distribution Strategy
Given the single-node deployment with BlueField DPU, we implement logical service separation with resource isolation:

| Service | Instances | CPU Pinning | Memory | Network | Purpose |
|---|---|---|---|---|---|
| Master | 1 | DPU Core 0-1 | 4GB |  DPU VF | Cluster coordination |
| Volume Server | 4 | Host Cores 0-31 | 64GB | 100Gbps | Data storage (1 per zpool) |
|Filer | 2 | Host Cores 32-47 | 32GB | 100Gbps | Metadata & POSIX |
|S3 Gateway | 4 | Host Cores 48-63 | 32GB | 100Gbps | S3 API endpoint |
|Metrics/Monitor | 1 | DPU Core 2 | 2GB | DPU VF | Prometheus/Grafana |
||

Total Resource Allocation: 128 cores, 134GB RAM, full 100Gbps bandwidth

## 2.2 Instance Justification
### Volume Servers (4 instances):
- One per zpool enables parallel I/O across RAIDz vdevs
- Isolates failure domains (zpool failure affects only 1 volume server)
- Maximizes NVMe queue depth utilization (4x vs single instance)
- Each manages ~90TB, 8 drives

### S3 Gateways (4 instances):
- Required for 100Gbps aggregate throughput (25Gbps per instance)
- HAProxy distributes connections across instances
- Graceful rotation for zero-downtime updates
- Isolates API request handling from storage I/O

### Filer Instances (2 instances):
- Active-active with shared Redis/ETCD backend
- Split by bucket hash (bucket % 2) for load distribution
- Automatic failover via VIP

## 3. Network Architecture & BlueField DPU Integration
### 3.1 Network Topology
```
                    ┌─────────────────────────────────────┐
                    │         Top-of-Rack Switch          │
                    │        (100Gbps, ECN-enabled)       │
                    └──────────────┬──────────────────────┘
                                   │
                    ┌──────────────┴──────────────────────┐
                    │     BlueField-3 DPU (p0, p1)        │
                    │  ┌──────────────┬──────────────┐    │
                    │  │   p0f0       │   p1f0       │    │
                    │  │ (Host PF)    │ (Host PF)    │    │
                    │  │  100Gbps     │  100Gbps     │    │
                    │  └──────────────┴──────────────┘    │
                    │  ┌──────────────┬──────────────┐    │
                    │  │   p0f0v0     │   p1f0v0     │    │
                    │  │ (DPU Arm)    │ (DPU Arm)    │    │
                    │  │   25Gbps     │   25Gbps     │    │
                    │  └──────────────┴──────────────┘    │
                    └─────────────────────────────────────┘
                                   │
        ┌──────────────────────────┼──────────────────────────┐
        │                          │                          │
   Host Kernel                  DPU Arm Cores              Host Kernel
   (SR-IOV VF)                  (Containerized)            (SR-IOV VF)
        │                          │                          │
   Volume Servers              Master/Monitor              S3 Gateways
   (4 instances)               (1 instance)                (4 instances)
   Filer (2 instances)                                      HAProxy (VIP)
```

### 3.2 IP Addressing & Dynamic IP Handling
**Challenge:** System admin may change 100Gbps NIC IP addresses. SeaweedFS must adapt without data loss.
**Solution:** Service discovery via DNS + systemd watch + controlled restart
```yaml
# /etc/seaweedfs/network-config.yaml
network:
  interface: p0  # Primary 100Gbps
  discovery:
    method: dns  # DNS A record lookup
    fqdn: seaweedfs-storage.internal.company.com
    ttl: 60

  # IP Change Detection
  watcher:
    enabled: true
    interval: 30s
    script: /opt/seaweedfs/scripts/ip-change-handler.sh

  # Graceful transition
  transition:
    pre_change_hook: /opt/seaweedfs/scripts/drain-connections.sh
    post_change_hook: /opt/seaweedfs/scripts/resume-connections.sh
    max_downtime: 10s
```
### IP Change Handler Script:
```bash
#!/bin/bash
# /opt/seaweedfs/scripts/ip-change-handler.sh

OLD_IP=$1
NEW_IP=$2

# 1. Update DNS record (via nsupdate or API)
nsupdate -k /etc/bind/ddns.key << EOF
server dns.internal.company.com
update delete seaweedfs-storage.internal.company.com A
update add seaweedfs-storage.internal.company.com 60 A $NEW_IP
send
EOF

# 2. Notify SeaweedFS Master of endpoint change
weed shell -master=localhost:9333 << EOF
lock
volumeServer.leave -node $OLD_IP:8080
volumeServer.join -node $NEW_IP:8080 -dir /seaweedfs/volumes
unlock
EOF

# 3. Update filer and S3 gateway configs
systemctl reload seaweedfs-filer@{1,2}
systemctl reload seaweedfs-s3-gateway@{1,2,3,4}

# 4. Update HAProxy backend IPs
/opt/seaweedfs/scripts/update-haproxy.sh $NEW_IP
```

## 4. ZFS Configuration Deep Dive
### 4.1 ZPool Selection for SeaweedFS
Recommended: 4x RAIDz2 vdevs vs 1x RAIDz3
| Metric | 1x RAIDz3 (32 drives) | 4x RAIDz2 (8 drives each)|
|---|---|---|
| Read IOPS | 3.2M (limited by single vdev) | 12.8M (4x parallel) |
| Write IOPS | 800K | 3.2M |
| Resilver Speed | Slow (28 drives to read) | Fast (6 drives to read) |
| Failure Domain | Entire pool | 1/4 of data |
| CPU Overhead | High (parity calc) | Distributed |
| SeaweedFS Mapping | Complex | 1:1 Volume Server |
||

Decision: 4x RAIDz2 zpools (seaweedfs-1 through seaweedfs-4)
### 4.2 Mount Points & Directory Structure
```
/seaweedfs/
├── volumes/
│   ├── zpool1/           # ZFS mountpoint
│   │   ├── index/        # Volume index files (SSD metadata)
│   │   └── dat/          # Actual .dat and .idx files
│   ├── zpool2/
│   ├── zpool3/
│   └── zpool4/
├── metadata/
│   ├── filer/            # Filer metadata (LevelDB/RocksDB)
│   │   ├── filer1/
│   │   └── filer2/
│   └── master/           # Master metadata (Raft)
├── logs/
│   └── journal/          # Persistent logs for crash recovery
└── config/
    └── dynamic/          # Runtime config updates
```
### 4.3 ZFS Tuning for SeaweedFS
```bash
# /etc/modprobe.d/zfs.conf
options zfs zfs_arc_max=137438953472  # 128GB ARC
options zfs zfs_vdev_async_read_max_active=32
options zfs zfs_vdev_sync_read_max_active=32
options zfs zfs_vdev_async_write_max_active=32
options zfs zfs_vdev_sync_write_max_active=32
options zfs zfs_dirty_data_max=17179869184  # 16GB max dirty

# Per-pool settings
zfs set recordsize=1M seaweedfs-1
zfs set compression=zstd-3 seaweedfs-1
zfs set xattr=sa seaweedfs-1
zfs set atime=off seaweedfs-1
zfs set dedup=off seaweedfs-1  # SeaweedFS handles dedup
zfs set primarycache=metadata seaweedfs-1  # Cache metadata, not data
zfs set secondarycache=none seaweedfs-1
```

## 5. Service Configuration & Systemd Integration
### 5.1 Master Server Configuration
```ini
# /etc/systemd/system/seaweedfs-master.service
[Unit]
Description=SeaweedFS Master Server
After=network-online.target time-sync.target
Wants=network-online.target

[Service]
Type=simple
User=seaweedfs
Group=seaweedfs
ExecStart=/usr/local/bin/weed master \
    -ip.bind=0.0.0.0 \
    -ip.publish=192.168.100.10 \
    -port=9333 \
    -mdir=/seaweedfs/metadata/master \
    -defaultReplication=000 \
    -volumeSizeLimitMB=102400 \
    -garbageThreshold=0.1 \
    -pulseSeconds=5 \
    -electionTimeout=3s \
    -heartbeatTimeout=6s

Restart=always
RestartSec=5
CPUAffinity=0-1  # DPU cores
MemoryMax=4G

[Install]
WantedBy=multi-user.target
```
### 5.2 Volume Server Configuration (4 instances)
```ini
# /etc/systemd/system/seaweedfs-volume@.service
[Unit]
Description=SeaweedFS Volume Server %i
After=network-online.target seaweedfs-master.service zfs-mount.service
Requires=zfs-mount.service
Wants=network-online.target

[Service]
Type=simple
User=seaweedfs
Group=seaweedfs
Environment="POOL_ID=%i"
ExecStartPre=/bin/sh -c 'if [ ! -d /seaweedfs/volumes/zpool${POOL_ID} ]; then echo "ZPool not mounted"; exit 1; fi'
ExecStart=/usr/local/bin/weed volume \
    -ip.bind=0.0.0.0 \
    -ip.publish=192.168.100.10 \
    -port=808${POOL_ID} \
    -mserver=192.168.100.10:9333 \
    -dir=/seaweedfs/volumes/zpool${POOL_ID}/dat \
    -dir.idx=/seaweedfs/volumes/zpool${POOL_ID}/index \
    -max=0 \
    -port.public=1808${POOL_ID} \
    -readMode=proxy

Restart=always
RestartSec=10
CPUAffinity=%i*8-7+%i*8  # Dynamic CPU pinning
MemoryMax=16G

# Crash recovery
TimeoutStartSec=60
TimeoutStopSec=120
KillMode=process

[Install]
WantedBy=multi-user.target
```

### 5.3 Filer Configuration (2 instances with shared backend)
```ini
# /etc/systemd/system/seaweedfs-filer@.service
[Unit]
Description=SeaweedFS Filer %i
After=network-online.target seaweedfs-master.service redis.service
Requires=redis.service

[Service]
Type=simple
User=seaweedfs
ExecStart=/usr/local/bin/weed filer \
    -ip.bind=0.0.0.0 \
    -ip.publish=192.168.100.10 \
    -port=888%i \
    -port.readonly=1888%i \
    -master=192.168.100.10:9333 \
    -defaultReplicaPlacement=000 \
    -maxMB=64 \
    -dir=/seaweedfs/metadata/filer/filer%i \
    -redis.server=localhost:6379 \
    -redis.database=%i \
    -redis.superLargeCollection=true

Restart=always
RestartSec=5
CPUAffinity=32-47
MemoryMax=16G

[Install]
WantedBy=multi-user.target
```

### 5.4 S3 Gateway Configuration (4 instances)
```ini
# /etc/systemd/system/seaweedfs-s3@.service
[Unit]
Description=SeaweedFS S3 Gateway %i
After=network-online.target seaweedfs-master.service seaweedfs-filer@1.service seaweedfs-filer@2.service
Wants=seaweedfs-filer@1.service seaweedfs-filer@2.service

[Service]
Type=simple
User=seaweedfs
ExecStart=/usr/local/bin/weed s3 \
    -ip.bind=0.0.0.0 \
    -port=833%i \
    -filer=192.168.100.10:8881,192.168.100.10:8882 \
    -config=/etc/seaweedfs/s3-config.json \
    -metricsPort=932%i

Restart=always
RestartSec=2
CPUAffinity=48-63
MemoryMax=8G

# Graceful shutdown for connection draining
TimeoutStopSec=30
KillSignal=SIGTERM

[Install]
WantedBy=multi-user.target
```

### 5.5 HAProxy Load Balancer Configuration
```haproxy
# /etc/haproxy/haproxy.cfg
global
    maxconn 100000
    nbthread 16
    cpu-map auto:1/1-16 48-63

defaults
    mode http
    timeout connect 5s
    timeout client 300s
    timeout server 300s
    option httpchk GET /status

frontend s3_frontend
    bind 192.168.100.10:443 ssl crt /etc/ssl/certs/seaweedfs.pem
    bind 192.168.100.10:80
    maxconn 50000

    # Health check for backends
    default_backend s3_backends

    # Stats
    stats enable
    stats uri /admin?stats
    stats auth admin:secure_password

backend s3_backends
    balance roundrobin
    option httpchk GET /status
    http-check expect status 200

    server s3-1 127.0.0.1:8331 check weight 25 maxconn 12500
    server s3-2 127.0.0.1:8332 check weight 25 maxconn 12500
    server s3-3 127.0.0.1:8333 check weight 25 maxconn 12500
    server s3-4 127.0.0.1:8334 check weight 25 maxconn 12500

    # Connection draining on reload
    option redispatch
    retries 3
```

## 6. Failure Scenarios & Recovery Procedures
### 6.1 ZPool Unmount / Drive Failure
#### Scenario: One of 4 zpools becomes unavailable (drive failure, ZFS error, manual unmount)
#### Behavior:
```
1. ZFS detects fault → ZPool degrades to DEGRADED or UNAVAIL
2. Systemd notifies seaweedfs-volume@N.service
3. Volume server receives SIGTERM → Stops accepting new writes
4. Existing connections drain (up to 120s timeout)
5. Master marks volumes on that server as "ReadOnly"
6. S3 gateways redirect new writes to remaining 3 volume servers
7. Reads from failed server return 503 → Client retries other gateways
```

#### Recovery:
```bash
# 1. Replace failed drive
zpool replace seaweedfs-1 /dev/faulty_drive /dev/new_drive

# 2. Wait for resilver
watch zpool status seaweedfs-1

# 3. Restart volume server
systemctl start seaweedfs-volume@1

# 4. Verify volume registration
weed shell -master=192.168.100.10:9333 -c "volume.list"
```

### 6.2 SeaweedFS Service Crash
#### Master Crash:
- Impact: No new volume allocation, read/write continues
- Recovery: Systemd restarts automatically, reloads state from /seaweedfs/metadata/master
- Downtime: <5 seconds
#### Volume Server Crash:
- Impact: 25% capacity loss, 25% throughput reduction
- Recovery: Automatic restart, re-registration with master
- Data Integrity: ZFS guarantees consistency; SeaweedFS replays journal
#### Filer Crash:
- Impact: One filer instance down, other handles 100% load
- Recovery: Systemd restart, reconnects to Redis
- Failover: HAProxy detects failure, routes to healthy filer
#### S3 Gateway Crash:
- Impact: 25% API capacity loss
- Recovery: Automatic restart, HAProxy redistributes load
- Client Impact: Retry on 503, no data loss

## 6.3 System Reboot
### Startup Order (systemd After= dependencies):
```
1. network-online.target
2. zfs-import-cache.service (import zpools)
3. zfs-mount.service (mount to /seaweedfs/volumes)
4. seaweedfs-master.service
5. seaweedfs-volume@{1,2,3,4}.service (parallel)
6. redis.service (for filer)
7. seaweedfs-filer@{1,2}.service
8. seaweedfs-s3@{1,2,3,4}.service
9. haproxy.service
```
### Verification Script:
```bash
#!/bin/bash
# /opt/seaweedfs/scripts/health-check.sh

check_service() {
    systemctl is-active --quiet $1 && echo "✓ $1" || echo "✗ $1 FAILED"
}

check_zpool() {
    zpool list -H -o health $1 | grep -q "ONLINE" && echo "✓ $1 ONLINE" || echo "✗ $1 DEGRADED"
}

echo "=== SeaweedFS Health Check ==="
check_service seaweedfs-master
check_service seaweedfs-volume@1
check_service seaweedfs-volume@2
check_service seaweedfs-volume@3
check_service seaweedfs-volume@4
check_service seaweedfs-filer@1
check_service seaweedfs-filer@2
check_service seaweedfs-s3@1
check_service seaweedfs-s3@2
check_service seaweedfs-s3@3
check_service seaweedfs-s3@4
check_service haproxy

echo ""
echo "=== ZPool Status ==="
check_zpool seaweedfs-1
check_zpool seaweedfs-2
check_zpool seaweedfs-3
check_zpool seaweedfs-4

echo ""
echo "=== Cluster Status ==="
weed shell -master=192.168.100.10:9333 -c "cluster.check"
```

## 6.4 Network IP Change
### Detection: systemd-networkd dispatcher script
```bash
#!/bin/bash
# /etc/systemd/network/ip-change-handler.sh

INTERFACE=$1
ACTION=$2

if [[ "$INTERFACE" == "p0" && "$ACTION" == "up" ]]; then
    NEW_IP=$(ip -4 addr show p0 | grep -oP '(?<=inet\s)\d+(\.\d+){3}')
    OLD_IP=$(cat /var/run/seaweedfs_last_ip 2>/dev/null || echo "")

    if [[ "$NEW_IP" != "$OLD_IP" ]]; then
        logger "SeaweedFS: IP change detected $OLD_IP -> $NEW_IP"

        # Graceful restart sequence
        /opt/seaweedfs/scripts/ip-change-handler.sh "$OLD_IP" "$NEW_IP"

        echo "$NEW_IP" > /var/run/seaweedfs_last_ip
    fi
fi
```

## 7. Bucket Analytics & Metadata Operations
### 7.1 Problem Statement
#### Challenge: Buckets with 5M+ objects or 25TB+ data cause rclone/s3cmd operations to timeout or consume excessive resources.
Requirements:
- Real-time bucket object count
- Real-time bucket size (logical and physical)
- Sub-second response for bucket list operations
- No full bucket traversal

## 7.2 Solution: Filer Metadata + Redis Counters
### Architecture:
```
S3 PUT/DELETE → S3 Gateway → Filer → Redis INCR/DECR
                                    ↓
                              Persistent Counter
                                    ↓
                         SeaweedFS Master (periodic sync)
```
### Implementation:
```json
// /etc/seaweedfs/s3-config.json
{
  "buckets": [
    {
      "name": "production-data",
      "anonymous": false,
      "analytics": {
        "enabled": true,
        "backend": "redis",
        "redisAddr": "localhost:6379",
        "counters": {
          "object_count": "bucket:production-data:count",
          "total_size": "bucket:production-data:size",
          "last_modified": "bucket:production-data:mtime"
        }
      }
    }
  ],
  "notifications": {
    "kafka": {
      "enabled": true,
      "brokers": ["kafka:9092"],
      "topic": "seaweedfs-events"
    }
  }
}
```

### Filer Extension for Atomic Counters:
```go
// Custom filer store wrapper (pseudo-code)
type AnalyticsStore struct {
    store filer.FilerStore
    redis *redis.Client
}

func (as *AnalyticsStore) InsertEntry(ctx context.Context, entry *filer.Entry) error {
    // 1. Insert metadata
    err := as.store.InsertEntry(ctx, entry)
    if err != nil {
        return err
    }

    // 2. Update counters (async, best effort)
    bucket := extractBucket(entry.FullPath)
    pipe := as.redis.Pipeline()
    pipe.Incr(ctx, fmt.Sprintf("bucket:%s:count", bucket))
    pipe.IncrBy(ctx, fmt.Sprintf("bucket:%s:size", bucket), entry.FileSize)
    pipe.Set(ctx, fmt.Sprintf("bucket:%s:mtime", bucket), time.Now().Unix(), 0)
    _, _ = pipe.Exec(ctx) // Fire and forget

    return nil
}
```

### API Endpoint for Fast Stats:
```bash
# Custom S3 API extension (via S3 gateway plugin)
GET /?analytics&bucket=production-data

Response:
{
  "bucket": "production-data",
  "object_count": 5242880,
  "total_size_bytes": 27487790694400,
  "total_size_human": "25 TiB",
  "last_modified": "2025-02-15T10:30:00Z",
  "storage_class_breakdown": {
    "STANDARD": 5242880
  },
  "cache_hit": true,
  "response_time_ms": 12
}
```

## 7.3 Background Consistency Check
### Since Redis counters are best-effort, run daily reconciliation:
```bash
#!/bin/bash
# /opt/seaweedfs/scripts/reconcile-bucket-stats.sh

weed shell -master=192.168.100.10:9333 << 'EOF'
# Trigger filer to scan and update counters
filer.meta.save -timeAgo=24h
EOF

# Force counter update from filer metadata
redis-cli -h localhost -p 6379 << 'EOF'
EVAL "
  local buckets = redis.call('keys', 'bucket:*:count')
  for _,key in ipairs(buckets) do
    local bucket = key:match('bucket:(.+):count')
    -- Reset counters based on actual filer scan
    -- This is a placeholder for actual reconciliation logic
  end
" 0
EOF
```

## 8. Performance Optimization & Benchmarking
### 8.1 Target Performance
| Metric | Target | Configuration |
|---|---|---|
| Sequential Read | 95 Gbps | 4x Volume servers, 1MB chunks, RDMA |
| Sequential Write | 45 Gbps | Async replication, ZFS SLOG, 000 mode |
| Random Read (4KB) | 2M IOPS | NVMe parallelism, ZFS recordsize=1M |
| Random Write (4KB) | 800K IOPS | ZIL aggregation, SeaweedFS compaction |
| S3 Latency (p99) | <10ms | Local filer cache, Redis backend |
| ListObjects (1M keys) | <2s | Filer prefix indexing, no S3 pagination |
||

## 8.2 BlueField DPU Optimizations
### RDMA Acceleration:
```bash

# Enable RDMA for SeaweedFS volume communication
modprobe mlx5_ib
modprobe rdma_cm

# Configure DPU to offload SeaweedFS networking
doca-seaweedfs-config --enable-rdma --cpu-cores 0-7

# SeaweedFS volume server with RDMA
weed volume -rdma -rdma.port=20000
```
### DPU Offload for Checksumming:
```bash
# Enable hardware checksum offload in ZFS
zfs set checksum=edonr seaweedfs-1  # CPU efficient
# OR offload to DPU
zfs set checksum=off seaweedfs-1  # Let DPU handle it
```

### 8.3 Kernel Tuning
```bash
# /etc/sysctl.d/99-seaweedfs.conf
# Network
net.core.rmem_max = 134217728
net.core.wmem_max = 134217728
net.ipv4.tcp_rmem = 4096 87380 134217728
net.ipv4.tcp_wmem = 4096 65536 134217728
net.core.netdev_max_backlog = 300000
net.ipv4.tcp_congestion_control = bbr
net.ipv4.tcp_notsent_lowat = 16384

# NVMe/ZFS
vm.swappiness = 1
vm.dirty_ratio = 80
vm.dirty_background_ratio = 5
vm.vfs_cache_pressure = 50

# File descriptors
fs.file-max = 2097152
fs.nr_open = 2097152
```

### 8.4 SeaweedFS-Specific Tuning
```bash
# Master
-volumeSizeLimitMB=102400  # 100GB volumes (larger = fewer metadata ops)
-garbageThreshold=0.1      # Aggressive GC for 000 mode
-pulseSeconds=5            # Fast failure detection

# Volume
-max=0                     # Unlimited volumes
-readMode=proxy            # Direct read from disk
-index=memory              # Keep indexes in RAM

# Filer
-maxMB=64                  # Large upload chunks
-cacheCapacityMB=32768     # 32GB filer cache
```

## 9. Monitoring & Alerting
### 9.1 Prometheus Metrics Collection
#### Endpoints:
| Service | Metrics Port | Key Metrics | 
|---|---|---|
| Master | 9333/metrics | Volume count, free space, leader status | 
| Volume | 9321-9324 | IOPS, throughput, error rates | 
| Filer | 9325-9326 | Query latency, cache hit rate | 
| S3 | 9331-9334 | API latency, request rate, error rate | 
| ZFS | Node exporter | ARC hit rate, pool health, I/O wait | 
||

### 9.2 Critical Alerts
```yaml
# Prometheus AlertManager rules
groups:
  - name: seaweedfs-critical
    rules:
      - alert: SeaweedFSMasterDown
        expr: up{job="seaweedfs-master"} == 0
        for: 30s
        severity: critical

      - alert: ZPoolDegraded
        expr: zfs_pool_health{pool=~"seaweedfs-.*"} != 0
        for: 0s
        severity: critical

      - alert: VolumeServerDown
        expr: up{job=~"seaweedfs-volume-.*"} == 0
        for: 1m
        severity: warning

      - alert: S3LatencyHigh
        expr: histogram_quantile(0.99, s3_request_duration_seconds_bucket) > 0.1
        for: 5m
        severity: warning

      - alert: DiskNearFull
        expr: (seaweedfs_volume_max_size - seaweedfs_volume_used_size) / seaweedfs_volume_max_size < 0.1
        for: 5m
        severity: warning
```

### 9.3 Grafana Dashboards
- Overview: Cluster health, throughput, capacity
- Performance: Per-volume IOPS, latency heatmaps
- S3 API: Request rates, error rates, bucket analytics
- ZFS: ARC efficiency, pool fragmentation, scrub status

## 10. Service Dependencies & Startup Orchestration
### 10.1 Dependency Graph
```
System Boot
    │
    ├── network-online.target
    │       └── systemd-networkd-wait-online.service
    │
    ├── zfs-import.target
    │       └── zpool import (4 pools)
    │
    ├── seaweedfs-master.service
    │       └── Depends: network-online, zfs-import
    │
    ├── seaweedfs-volume@.service (x4)
    │       └── Depends: seaweedfs-master, zfs-mount
    │
    ├── redis.service (for filer)
    │       └── Depends: network-online
    │
    ├── seaweedfs-filer@.service (x2)
    │       └── Depends: seaweedfs-master, redis
    │
    ├── seaweedfs-s3@.service (x4)
    │       └── Depends: seaweedfs-filer@1, seaweedfs-filer@2
    │
    └── haproxy.service
            └── Depends: seaweedfs-s3@1..4
```
### 10.2 Reverse Dependencies (Services Depending on SeaweedFS)
#### Application Services:
```ini
# /etc/systemd/system/ai-training-pipeline.service
[Unit]
Description=AI Training Data Pipeline
After=seaweedfs-s3@1.service seaweedfs-s3@2.service seaweedfs-s3@3.service seaweedfs-s3@4.service
Requires=seaweedfs-s3@1.service

[Service]
Type=simple
ExecStart=/opt/ai-pipeline/start.sh --s3-endpoint=http://192.168.100.10:80
Restart=on-failure

[Install]
WantedBy=multi-user.target
```
#### Shutdown Order:
```
1. Stop dependent services (ai-training-pipeline, etc.)
2. Stop haproxy (drain connections)
3. Stop s3 gateways (wait for active requests)
4. Stop filers (flush metadata to Redis)
5. Stop volume servers (sync ZFS)
6. Stop master (persist raft log)
7. Unmount zpools (export)
```

## 11. Security Considerations
### 11.1 Network Security
- DPU Isolation: Management traffic (SSH) on DPU Arm cores, data on host
- TLS: S3 gateways terminate TLS with hardware acceleration (BlueField crypto)
- Firewall: nftables rules restricting 9333, 808x, 888x to storage network only

### 11.2 Data Security
- Encryption at Rest: ZFS native encryption (AES-256-GCM) with keys in DPU secure enclave
- Encryption in Transit: TLS 1.3 for S3, WireGuard for inter-service communication
- Access Control: IAM policies via S3 gateway configuration

### 11.3 Audit Logging
```json
{
  "audit": {
    "enabled": true,
    "logPath": "/seaweedfs/logs/audit",
    "events": ["PUT", "DELETE", "GET", "LIST"],
    "format": "json",
    "rotation": "daily"
  }
}
```

## 12. Deployment Timeline
| Phase | Duration | Tasks | 
|---|---|---|
| Phase 1: Hardware Prep | 2 days | DPU flashing, ZFS pool creation, network validation | 
| Phase 2: SeaweedFS Install | 1 day | Binary deployment, systemd unit creation, config validation | 
| Phase 3: Service Integration | 2 days | Dependency wiring, HAProxy tuning, health checks | 
| Phase 4: Performance Tuning | 3 days | Benchmarking, kernel tuning, DPU optimization | 
| Phase 5: Failure Testing | 2 days | Chaos engineering, recovery procedures, documentation | 
| Phase 6: Production Cutover | 1 day | Migration, monitoring, go-live | 
||

Total: 11 days


## 13. Conclusion
This architecture delivers:
- Scalability: 250TB+ with linear expansion via additional zpools
- Performance: Near line-rate 100Gbps utilizing BlueField DPU offloads
- Reliability: RAIDz2 redundancy, automatic failover, zero RPO
- Observability: Real-time bucket analytics without expensive operations
- Maintainability: Clear service boundaries, automated recovery, dynamic IP handling

Next Steps:
- Review and approve architecture
- Procure hardware (if not already available)
- Schedule deployment window
- Execute Phase 1 (hardware preparation)

Appendices:
- Appendix A: Complete systemd unit files
- Appendix B: ZFS pool creation scripts
- Appendix C: Benchmark results template
- Appendix D: Runbook for common failures

Document Owner: Infrastructure Team
Review Cycle: Quarterly or on major version upgrade