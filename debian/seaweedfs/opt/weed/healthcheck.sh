#!/bin/bash

WEED_BINARY="${WEED_BINARY:-/opt/weed/weed}"
WEED_HOME="${WEED_HOME:-/opt/weed}"
LOG_DIR="${LOG_DIR:-/var/log/weed}"

check_weed_process() {
    local service=$1
    local instance=$2
    local port=$3

    if ! pgrep -f "weed.*$service.*$instance" >/dev/null 2>&1; then
        return 1
    fi

    if [ -n "$port" ]; then
        if ! nc -z localhost "$port" 2>/dev/null; then
            return 1
        fi
    fi

    return 0
}

check_master() {
    check_weed_process "master" "$1" "9333"
}

check_volume() {
    check_weed_process "volume" "$1" "8080"
}

check_filer() {
    check_weed_process "filer" "$1" "8888"
}

check_s3() {
    check_weed_process "s3" "$1" "8000"
}

if [ $# -eq 0 ]; then
    echo "Usage: $0 <master|volume|filer|s3> [instance]"
    exit 1
fi

SERVICE=$1
INSTANCE=${2:-1}

case "$SERVICE" in
    master)
        check_master "$INSTANCE"
        ;;
    volume)
        check_volume "$INSTANCE"
        ;;
    filer)
        check_filer "$INSTANCE"
        ;;
    s3)
        check_s3 "$INSTANCE"
        ;;
    *)
        echo "Unknown service: $SERVICE"
        exit 1
        ;;
esac

if [ $? -eq 0 ]; then
    echo "OK: $SERVICE@$INSTANCE is healthy"
    exit 0
else
    echo "FAIL: $SERVICE@$INSTANCE is not running"
    exit 1
fi
