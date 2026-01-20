1. Create a Test Configuration File
Create a configuration file simulation.toml:
cat > simulation.toml << 'EOF'
[storage.backend.simulation.default]
enabled = true
latency = "10ms"
error_rate = 0.001
bandwidth = "50MB/s"
disk_space = "1GB"
enable_failure = false
EOF
2. Start Volume Server with Simulation Backend
# Start volume server with simulation configuration
./weed volume -port 8080 -dir=/tmp/volumes_simulation -dataCenter=dc1 -rack=rack1 \
  -master=localhost:9333 -config=simulation.toml -ip=127.0.0.1
3. Start Master Server
# Start master server
./weed master -port 9333 -ip=127.0.0.1
4. Test File Operations
# Upload a test file
echo "Hello SeaweedFS Simulation!" > test.txt
./weed upload -master=localhost:9333 test.txt
# Download the file
./weed download -master=localhost:9333 <file_id_from_upload>
5. Monitor Simulation Metrics
# Check simulation status
curl http://localhost:8888/simulation/status
# Get detailed metrics
curl http://localhost:8888/simulation/metrics
# Get current configuration
curl http://localhost:8888/simulation/config
6. Test Runtime Configuration Changes
# Increase latency to test slower disk simulation
curl -X POST http://localhost:8888/simulation/config \
  -H "Content-Type: application/json" \
  -d '{"latency": "50ms"}'
# Add 10% error rate for testing
curl -X POST http://localhost:8888/simulation/config \
  -H "Content-Type: application/json" \
  -d '{"error_rate": 0.1}'
# Reset metrics to start fresh measurements
curl -X POST http://localhost:8888/simulation/reset
7. Test Error Injection
# Enable failure after 10MB of data
curl -X POST http://localhost:8888/simulation/config \
  -H "Content-Type: application/json" \
  -d '{
    "enable_failure": true,
    "failure_point": 10485760
  }'
# Try uploading files until failure occurs
for i in {1..20}; do
  dd if=/dev/zero of=test_$i.dat bs=1M count=1
  ./weed upload -master=localhost:9333 test_$i.dat
done
8. Performance Testing
# Reset metrics first
curl -X POST http://localhost:8888/simulation/reset
# Upload multiple files in parallel to test performance
for i in {1..10}; do
  dd if=/dev/zero of=perf_test_$i.dat bs=1M count=5 &
done
wait
for i in {1..10}; do
  ./weed upload -master=localhost:9333 perf_test_$i.dat &
done
wait
# Check performance metrics
curl http://localhost:8888/simulation/metrics | jq '.'
9. Compare with Real Disk Performance
# First test with real disk (default backend)
# Start a regular volume server on a different port
./weed volume -port 8081 -dir=/tmp/volumes_real -dataCenter=dc1 -rack=rack1 \
  -master=localhost:9333 -ip=127.0.0.1
# Upload test files to both backends and compare times
time ./weed upload -master=localhost:9333 -replication=001 -volumeServer=localhost:8080 real_backend_test.txt
time ./weed upload -master=localhost:9333 -replication=001 -volumeServer=localhost:8081 simulation_backend_test.txt
# Compare metrics
curl http://localhost:8888/simulation/status
10. Clean Up Test Environment
# Stop servers
pkill -f "weed volume"
pkill -f "weed master"
# Clean up test files
rm -f test*.txt perf_test*.dat simulation.toml
rm -rf /tmp/volumes_simulation /tmp/volumes_real
Quick Test Script
Here's a complete test script you can save as test_simulation.sh:
#!/bin/bash
set -e
echo "Starting SeaweedFS with Simulation Backend Test..."
# Create config
cat > simulation.toml << 'EOF'
[storage.backend.simulation.default]
enabled = true
latency = "5ms"
error_rate = 0.0
bandwidth = "100MB/s"
disk_space = "1GB"
EOF
# Start master (background)
./weed master -port 9333 -ip=127.0.0.1 &
MASTER_PID=$!
sleep 2
# Start volume server with simulation (background)
./weed volume -port 8080 -dir=/tmp/volumes_simulation -dataCenter=dc1 -rack=rack1 \
  -master=localhost:9333 -config=simulation.toml -ip=127.0.0.1 &
VOLUME_PID=$!
sleep 2
# Create test file
echo "Test data for simulation backend" > test.txt
# Upload and download test
echo "Uploading test file..."
FILE_ID=$(./weed upload -master=localhost:9333 test.txt | grep -o 'fileId:[^,]*' | cut -d: -f2)
echo "Uploaded file ID: $FILE_ID"
echo "Downloading test file..."
./weed download -master=localhost:9333 $FILE_ID -o downloaded_test.txt
# Verify file content
if diff test.txt downloaded_test.txt > /dev/null; then
    echo "✓ File content matches!"
else
    echo "✗ File content differs!"
fi
# Check simulation metrics
echo "Simulation metrics:"
curl -s http://localhost:8888/simulation/metrics | jq '.'
# Clean up
echo "Cleaning up..."
kill $MASTER_PID $VOLUME_PID 2>/dev/null || true
rm -f test.txt downloaded_test.txt simulation.toml
rm -rf /tmp/volumes_simulation
echo "Test completed!"
Run it with:
chmod +x test_simulation.sh
./test_simulation.sh
This will give you a complete manual