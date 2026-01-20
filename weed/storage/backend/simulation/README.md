# Simulation Backend for SeaweedFS

The simulation backend provides a way to fake volume write operations without actual disk I/O, perfect for performance testing without the overhead of real storage operations.

## Features

- **In-memory storage**: All data is stored in memory, no disk I/O
- **Configurable latency**: Simulate disk latency with customizable delays
- **Error injection**: Simulate write failures for testing error handling
- **Bandwidth limiting**: Simulate slow disks or network storage
- **Disk space limits**: Enforce storage quotas
- **Real-time metrics**: Monitor read/write operations, latency, and errors
- **HTTP control interface**: Configure and monitor simulation at runtime

## Configuration

Add the simulation backend to your SeaweedFS configuration:

```yaml
storage:
  backend:
    simulation:
      default:
        enabled: true
        latency: "5ms"                # Simulated disk latency
        error_rate: 0.001            # 0.1% write error rate
        bandwidth: "100MB/s"          # Simulated bandwidth limit
        disk_space: "10GB"            # Total available disk space
        enable_failure: false         # Enable failure injection
        failure_point: 0              # Fail after N bytes written
```

### Configuration Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `latency` | duration | "5ms" | Simulated disk I/O latency |
| `error_rate` | float | 0.0 | Probability of write errors (0.0-1.0) |
| `bandwidth` | string | "0" | Bandwidth limit (B/s, KB/s, MB/s, GB/s) |
| `disk_space` | string | "0" | Total available space (B, KB, MB, GB, TB) |
| `enable_failure` | bool | false | Enable failure injection at specific point |
| `failure_point` | int | 0 | Fail after writing this many bytes |

## HTTP Control Interface

The simulation backend provides HTTP endpoints for monitoring and control:

### Get Simulation Status
```
GET /simulation/status
```

Returns overall simulation status including:
- Enabled/disabled state
- Global configuration
- Total bytes written/read
- Total operations and errors
- Average latency

### Get Metrics
```
GET /simulation/metrics[?file=filename]
```

Returns detailed metrics for all files or a specific file:
- Write/read operation counts
- Bytes written/read
- Sync and truncate operations
- Average latency

### Get/Set Configuration
```
GET /simulation/config[?file=filename]
POST /simulation/config[?file=filename]
```

View or modify configuration globally or for specific files.

POST body example:
```json
{
  "latency": "10ms",
  "error_rate": 0.01,
  "bandwidth": "50MB/s"
}
```

### Reset Metrics
```
POST /simulation/reset[?file=filename]
```

Reset metrics counters for all files or a specific file.

### Enable/Disable Simulation
```
POST /simulation/enable
POST /simulation/disable
```

Enable or disable the simulation backend.

## Usage Examples

### Basic Performance Testing

1. Configure SeaweedFS to use the simulation backend
2. Run your normal workloads
3. Monitor performance metrics via HTTP interface
4. Adjust parameters to simulate different conditions

### Testing Error Scenarios

```yaml
storage:
  backend:
    simulation:
      default:
        enabled: true
        error_rate: 0.01        # 1% error rate
        enable_failure: true
        failure_point: 1073741824  # Fail after 1GB
```

### Simulating Slow Storage

```yaml
storage:
  backend:
    simulation:
      default:
        enabled: true
        latency: "100ms"        # High latency
        bandwidth: "10MB/s"     # Slow bandwidth
```

### Runtime Configuration

You can modify simulation parameters at runtime:

```bash
# Increase latency
curl -X POST http://localhost:8888/simulation/config \
  -H "Content-Type: application/json" \
  -d '{"latency": "50ms"}'

# Set 5% error rate for testing
curl -X POST http://localhost:8888/simulation/config \
  -H "Content-Type: application/json" \
  -d '{"error_rate": 0.05}'

# Reset metrics
curl -X POST http://localhost:8888/simulation/reset
```

## Integration with HTTP Server

To enable the HTTP control interface, add this to your volume server startup:

```go
import _ "github.com/seaweedfs/seaweedfs/weed/storage/backend/simulation"

// In your HTTP server setup
mux := http.NewServeMux()
simulation.SetupHTTPHandlers(mux)
```

## Benefits for Performance Testing

1. **No Disk I/O Overhead**: Eliminates disk contention and bottlenecks
2. **Deterministic Performance**: Consistent timing for repeatable tests
3. **Controlled Environment**: Simulate various storage conditions
4. **Real-time Monitoring**: Track performance metrics during tests
5. **Error Scenarios**: Test error handling without actual hardware failures
6. **Scalability Testing**: Test with large volumes without using actual disk space

## Metrics Available

The simulation backend tracks:
- **Bytes Written/Read**: Total data transferred
- **Operation Counts**: Number of read/write operations
- **Latency Metrics**: Average and total operation time
- **Error Statistics**: Number of injected errors
- **Sync Operations**: Count of sync calls
- **Truncate Operations**: Count of truncate calls

## Example Use Cases

1. **Benchmarking**: Compare algorithm performance without disk variability
2. **Load Testing**: Test system behavior under high load
3. **Error Handling**: Verify application resilience to storage failures
4. **Capacity Planning**: Simulate different storage performance characteristics
5. **CI/CD Testing**: Run performance tests in environments without fast storage
6. **Algorithm Development**: Test new storage algorithms with controlled I/O

## Limitations

- Data is stored in memory and will be lost on restart
- Not suitable for production data storage
- Memory usage grows with stored data size
- Cannot test actual storage hardware characteristics

## Comparison with Real Storage

| Characteristic | Real Disk | Simulation |
|---------------|-----------|------------|
| Latency | Variable (1-20ms+) | Configurable, consistent |
| Bandwidth | Hardware dependent | Configurable limit |
| Error Rate | Hardware dependent | Configurable |
| Data Persistence | Persistent | Lost on restart |
| Test Repeatability | Variable | Consistent |
| Resource Usage | CPU + Disk | CPU + Memory |