#!/bin/bash

#export GOGC=100
# export GOMEMLIMIT=150GiB

#sudo sysctl -w net.core.rmem_max=134217728
#sudo sysctl -w net.core.wmem_max=134217728
#sudo sysctl -w net.ipv4.tcp_wmem="4096 1048576 134217728"
#sudo sysctl -w net.ipv4.tcp_rmem="4096 1048576 134217728"
#sudo sysctl -w net.core.somaxconn=65535
#sudo sysctl -w net.ipv4.tcp_timestamps=0
#sudo sysctl -w net.ipv4.tcp_sack=1
#sudo sysctl -w net.ipv4.tcp_adv_win_scale=1

DEFAULT_DATA_DIR="/tmp/weed"
# read -p "Enter data directory [$DEFAULT_DATA_DIR]: " INPUT
# DATA_DIR="${INPUT:-$DEFAULT_DATA_DIR}"
DATA_DIR="${1:-$DEFAULT_DATA_DIR}"
echo "Using data dir: $DATA_DIR"

LOG_DIR="/tmp/weed/logs"

mkdir -p $DATA_DIR/{master,filer} $LOG_DIR $DATA_DIR/volume{1..16}

sudo mkdir -p /etc/seaweedfs
# sudo cp -fv ./filer.toml /etc/seaweedfs/filer.toml
# sudo cp -fv ./s3.json /etc/seaweedfs/s3.json

WEED_BIN=./weed-4.07-arm64
Ip10Gbps=10.10.10.32
FILER_SOCK="/tmp/seaweedfs-filer-8888.sock"

ulimit -n 102400

echo "Starting..."

## Standalone server with master, volume, filer, s3 all in one
$WEED_BIN server \
        -ip=10.10.10.31 \
        -ip.bind=10.10.10.31 \
        -master.port=9333 \
        -master.port.grpc=19333 \
        -master.dir=/tmp/weed/master \
        -master.volumeSizeLimitMB=64000 \
        -master.defaultReplication=000 \
        -dir=/tmp/weed/volume1,/tmp/weed/volume2,/tmp/weed/volume3,/tmp/weed/volume4,/tmp/weed/volume5,/tmp/weed/volume6,/tmp/weed/volume7,/tmp/weed/volume8,/tmp/weed/volume9,/tmp/weed/volume10,/tmp/weed/volume11,/tmp/weed/volume12,/tmp/weed/volume13,/tmp/weed/volume14,/tmp/weed/volume15,/tmp/weed/volume16 \
        -volume.max=1000 \
        -volume.index=leveldb \
        -volume.fileSizeLimitMB=4096 \
        -filer=true \
        -filer.port=8888 \
        -s3=true \
        -s3.port=8333 \
        -s3.config=./s3.json \
        -debug -debug.port=9321 \
        > $LOG_DIR/server.log 2>&1 &


# -master.volumeSizeLimitMB=65536 \
#       -volume.fileWriteBufferSizeMB=2 \
#        -ip.bind=10.10.10.32 \



####################### 


## THIS IS FOR MASTER INSTNACE ONLY
# # $WEED_BIN -v=0 master \
# #         -defaultReplication=000 \
# #         -ip=$Ip10Gbps -ip.bind=$Ip10Gbps \
# #         -mdir="/bryck/seaweedfs/master"  \
# #         -volumeSizeLimitMB=100000 \
# #         -debug -debug.port=9325 \
# #         > /tmp/seaweedfs-logs/master.log 2>&1 &

# # $WEED_BIN -v=0 master -config=./seaweed.toml > $LOG_DIR/master.log 2>&1 &
# $WEED_BIN -v=0 master -options=./master.conf > $LOG_DIR/master.log 2>&1 &

# echo "[1/4] Started Master"

## THIS IS FOR VOLUME INSTANCE ONLY
# # $WEED_BIN -v=0 volume \
# #         -dir="/bryck/seaweedfs/volume" \
# #         -index=leveldb2 \
# #         -ip=$Ip10Gbps -ip.bind=$Ip10Gbps \
# #         -master=$Ip10Gbps:9333 \
# #         -max=100 \
# #         -readBufferSizeMB=64 \
# #         -pprof \
# #         -port=8081 \
# #         -disk=nvme \
# #         -debug -debug.port=9326 \
# #         > /tmp/seaweedfs-logs/volume.log 2>&1 &
# # $WEED_BIN -v=0 master -config=./seaweed.toml > $LOG_DIR/volume.log 2>&1 &
# $WEED_BIN -v=0 volume -options=./volume.conf > $LOG_DIR/volume.log 2>&1 &

# echo "[2/4] started volume"

## THIS IS FOR FILER INSTANCE ONLY
# # $WEED_BIN -v=0 filer \
# #         -debug -debug.port=9327 \
# #         -ip=$Ip10Gbps -ip.bind=$Ip10Gbps \
# #         -master=$Ip10Gbps:9333 \
# #                 -localSocket=$FILER_SOCK \
# #         -disk=nvme \
# #         -maxMB=64 \
# #         -port=8888 \
# #         > /tmp/seaweedfs-logs/filer.log 2>&1 &

# # $WEED_BIN -v=0 filer -master=$Ip10Gbps:9333 -config=./filer.toml > $LOG_DIR/filer.log 2>&1 &
# $WEED_BIN -v=0 filer ip=$Ip10Gbps -ip.bind=$Ip10Gbps -master=$Ip10Gbps:9333 -localSocket=$FILER_SOCK -port=8888 -peers= -disk=nvme > $LOG_DIR/filer.log 2>&1 &

# echo "[3/4] started filer"

## THIS IS FOR S3 INSTANCE ONLY
# # $WEED_BIN -v=0 s3 \
# #         -filer=$Ip10Gbps:8888 \
# #         -ip.bind=$Ip10Gbps\
# #   	-port=8333 \
# #         -localFilerSocket=$FILER_SOCK \
# #         -allowEmptyUpload \
# #         -debug -debug.port=9328 \
# #         > /tmp/seaweedfs-logs/s3-1.log 2>&1 &

# $WEED_BIN -v=0 s3 \
#   -filer=$Ip10Gbps:8888 \
#   -ip.bind=$Ip10Gbps \
#   -port=8333 \
#   -localFilerSocket=$FILER_SOCK \
#   > $LOG_DIR/s3.log 2>&1 &

# #   -s3.config=/etc/seaweedfs/s3.json

# echo "[4/4] started s3 "

# echo "done "


# # ./weed -v=0 s3 -filer=$Ip10Gbps:8888 -ip.bind=$Ip10Gbps -port=8333 -localFilerSocket=$FILER_SOCK > $LOG_DIR/s3.log 2>&1 &


# ./weed-4.07-arm64 server \
#   -ip=10.10.10.32 \
#   -ip.bind=10.10.10.32 \
#   -master.port=9333 \
#   -master.dir=/bryck/weed/master \
#   -master.volumeSizeLimitMB=65536 \
#   -master.defaultReplication=000 \
#   -volume.port=8080 \
#   -volume.port.grpc=18080 \
#   -volume.dir=/bryck/weed/volume1,/bryck/weed/volume2,/bryck/weed/volume3,/bryck/weed/volume4,/bryck/weed/volume5,/bryck/weed/volume6,/bryck/weed/volume7,/bryck/weed/volume8,/bryck/weed/volume9,/bryck/weed/volume10,/bryck/weed/volume11,/bryck/weed/volume12,/bryck/weed/volume13,/bryck/weed/volume14,/bryck/weed/volume15,/bryck/weed/volume16 \
#   -volume.max=0 \
#   -volume.dataCenter=dc1 \
#   -volume.rack=rack1 \
#   -filer.port=8888 \
#   -s3.port=8333 \
#   -s3.config=/etc/seaweedfs/s3.json

