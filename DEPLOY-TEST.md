# SeaweedFS Deployment Guide

## Overview
This document describes how to deploy SeaweedFS with Master, Filer, S3, and Volume servers.

## Architecture
- **Master Server** (port 9333): Cluster coordination and volume management
- **Volume Server** (port 8080): Stores actual data chunks
- **Filer** (port 8888): POSIX-like interface, metadata management
- **S3 Gateway** (port 8333): S3-compatible API

## Quick Start

### Option 1: Using `weed server` (Recommended for single-node)
```bash
./weed server -dir=/data -s3
```
This starts master, volume, filer, and S3 on one machine.

### Option 2: Using `weed mini` (All-in-One Development)
```bash
./weed mini -dir=/data
```
Starts all components with default configuration.

### Option 3: Individual Component Start (Modular)
```bash
# Start master
./weed master -port=9333 -mdir=/data/master

# Start volume server
./weed volume -dir=/data/volumes -max=8 -master=localhost:9333 -port=8080

# Start filer with S3
./weed filer -master=localhost:9333 -port=8888 -s3 -s3.port=8333
```

## Environment
- S3 Access Key: `admin`
- S3 Secret Key: `admin` (configurable via `-s3.config`)

## Ports
| Component | HTTP Port | gRPC Port |
|-----------|-----------|-----------|
| Master    | 9333      | 19333     |
| Volume    | 8080      | 18080     |
| Filer     | 8888      | 18888     |
| S3        | 8333      | 18333     |

## Testing
After starting the servers, use `bucket-info.py` to:
1. Create a bucket
2. Upload test files
3. Get bucket object count and size
