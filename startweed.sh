#!/bin/bash

# SeaweedFS Startup Script
# Starts Master, Volume Server, Filer, and S3 Gateway

set -e

WEED="./weed-4.12-4-g7c610143f-dirty-linux-amd64"
DATA_DIR="/tmp/weed-data"
mkdir -p "$DATA_DIR"/{master,volume,filer}

echo "Starting SeaweedFS services..."

# Start Master Server
echo "Starting Master server on port 9333..."
$WEED master \
    -port=9333 \
    -mdir="$DATA_DIR/master" \
    -defaultReplication=000 &

# Wait for master to be ready
sleep 2

# Start Volume Server
echo "Starting Volume server on port 8080..."
$WEED volume \
    -dir="$DATA_DIR/volume" \
    -max=8 \
    -master=localhost:9333 \
    -port=8080 &

# Wait for volume to register
sleep 2

# Start Filer with S3 Gateway
echo "Starting Filer on port 8888 with S3 on port 8333..."
$WEED filer \
    -master=localhost:9333 \
    -port=8888 \
    -s3 \
    -s3.port=8333 \
    -defaultStoreDir="$DATA_DIR/filer" &

# Wait for filer to be ready
sleep 3

echo ""
echo "SeaweedFS services started!"
echo "  Master UI:      http://localhost:9333"
echo "  Volume Server:  http://localhost:8080"
echo "  Filer UI:       http://localhost:8888"
echo "  S3 Endpoint:    http://localhost:8333"
echo ""
echo "S3 Credentials:"
echo "  Access Key: admin"
echo "  Secret Key: admin"
echo ""
echo "Use '$WEED shell' for command line operations"
echo "Use python bucket-info.py to test bucket operations"
