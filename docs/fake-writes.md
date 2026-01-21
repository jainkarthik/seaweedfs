# SeaweedFS Volume IO Write Faking

This feature allows SeaweedFS to fake volume write operations, which means when files are uploaded to SeaweedFS, the system accepts the files and processes all metadata operations but doesn't actually write the file data to disk volumes. This is useful for testing, benchmarking, and development scenarios where you want to test the system behavior without consuming disk space.

## How It Works

The fake write implementation works at the **backend storage layer** (`weed/storage/backend/`), intercepting write operations before they reach the actual disk. When enabled:

1. **Write Operations**: Write calls return success immediately without writing data to disk
2. **Metadata Operations**: All index and metadata operations continue to work normally
3. **File Statistics**: File size and modification times are updated to maintain consistency
4. **Read Operations**: Reads work normally but return empty or default data

## Configuration

### Environment Variables

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `SEAWEED_FAKE_WRITES` | Enable/disable fake writes globally | false | `true` |
| `SEAWEED_FAKE_WRITE_VOLUMES` | Comma-separated list of volume IDs to fake | all volumes | `1,2,3,10` |
| `SEAWEED_FAKE_WRITE_PATTERNS` | File patterns to match for faking | none | `*.dat` |
| `SEAWEED_FAKE_WRITE_MAX_SIZE` | Maximum file size to fake (bytes) | unlimited | `1048576` |
| `SEAWEED_FAKE_WRITE_LOG` | Enable detailed logging of fake operations | false | `true` |
| `SEAWEED_FAKE_WRITE_LOG_LEVEL` | Logging verbosity level (0-4) | 2 | `3` |

### Volume Patterns

Volume patterns support simple wildcard matching:
- `1` - Only volume ID 1
- `1,2,3` - Volumes 1, 2, and 3
- `*` - All volumes (default when enabled)

### Safety Mechanisms

The fake write feature includes several safety mechanisms:

1. **Production Detection**: Automatically disabled in production environments
2. **Size Limits**: Can limit maximum fake file size to prevent memory issues
3. **Validation**: Configuration validation prevents invalid settings

## Usage Examples

### Basic Usage

```bash
# Enable fake writes for all volumes
export SEAWEED_FAKE_WRITES=true

# Start SeaweedFS volume server
./weed volume -port=8080 -dir=/tmp/volumes
```

### Selective Volume Faking

```bash
# Enable fake writes only for specific volumes
export SEAWEED_FAKE_WRITES=true
export SEAWEED_FAKE_WRITE_VOLUMES="1,2,3"

# Start volume server
./weed volume -port=8080 -dir=/tmp/volumes
```

### With Logging

```bash
# Enable fake writes with detailed logging
export SEAWEED_FAKE_WRITES=true
export SEAWEED_FAKE_WRITE_LOG=true
export SEAWEED_FAKE_WRITE_LOG_LEVEL=3

# Start volume server
./weed volume -port=8080 -dir=/tmp/volumes
```

## Testing with Warp

Here's how to use the fake write feature with `warp put` for testing:

```bash
# Setup environment
export SEAWEED_FAKE_WRITES=true
export SEAWEED_FAKE_WRITE_LOG=true

# Start SeaweedFS components
./weed master -port=9333 &
./weed volume -port=8080 -mserver=localhost:9333 &
./weed filer -port=8888 -master=localhost:9333 &

# Configure S3 endpoint
export AWS_ACCESS_KEY_ID=any
export AWS_SECRET_ACCESS_KEY=any
export AWS_ENDPOINT_URL=http://localhost:8333
export AWS_REGION=us-east-1

# Run warp put test
warp put --bucket=test --objects=1000 --size=10MB --threads=10
```

## Benefits

1. **Performance Testing**: Test upload performance without disk I/O bottlenecks
2. **Memory Testing**: Test system behavior with high-volume uploads
3. **Development**: Rapid testing without disk space concerns
4. **Benchmarking**: Isolate different performance factors
5. **CI/CD**: Faster test execution in pipeline environments

## Limitations

1. **No Real Data**: Files are not actually stored, so reads return empty data
2. **Memory Usage**: Index data is still maintained in memory
3. **Replication**: Fake writes affect all replication scenarios
4. **Backup/Restore**: Not applicable for fake writes

## Monitoring and Debugging

### Logging

When logging is enabled, fake write operations are logged with details:

```
INFO[2024-01-01T12:00:00Z] FAKE WRITE: /path/to/1.dat offset=0 size=1048576 total_writes=1 total_bytes=1048576
INFO[2024-01-01T12:00:01Z] FAKE SYNC: /path/to/1.dat
```

### Statistics

You can retrieve fake write statistics through the volume server admin API (when implemented):

```bash
curl http://localhost:8080/fake_write_status
```

## Implementation Details

The fake write implementation consists of several key components:

1. **FakeDiskFile** (`fake_disk_file.go`): Wraps the original DiskFile and intercepts write operations
2. **Configuration** (`fake_config.go`): Handles environment variable parsing and validation
3. **Helper Functions** (`fake_helpers.go`): Provides utility functions for pattern matching and volume ID extraction
4. **Interface Updates**: Modified BackendStorageFile interface to support the new functionality

The implementation maintains full compatibility with existing SeaweedFS features while providing the ability to bypass actual disk writes when needed.

## Troubleshooting

### Fake Writes Not Working

1. Check environment variables are set correctly
2. Verify volume ID patterns match your volumes
3. Ensure you're not in a production environment
4. Check logs for configuration errors

### Unexpected Behavior

1. Enable debug logging: `export SEAWEED_FAKE_WRITE_LOG_LEVEL=4`
2. Check volume file names match expected patterns
3. Verify file permissions are correct

### Performance Issues

1. Monitor memory usage (index data still stored)
2. Check if fake writes are actually being enabled
3. Verify volume server logs show fake write operations

## Security Considerations

1. **Production Safety**: Built-in protection prevents accidental production use
2. **Data Integrity**: Fake writes don't affect real data
3. **Access Control**: Feature can be enabled/disabled via environment variables
4. **Audit Trail**: All fake operations can be logged for auditing

## Future Enhancements

Potential future improvements to the fake write feature:

1. **Selective Content Faking**: Only fake specific file types or sizes
2. **Time-based Faking**: Enable fake writes during specific time windows
3. **Hybrid Mode**: Mix real and fake writes based on criteria
4. **Statistics API**: More comprehensive monitoring and statistics
5. **Configuration Hot-reload**: Change fake write settings without restart