# S3 Throughput Regression CI Runbook

## Overview
This runbook documents the **S3 Throughput Regression** GitHub Actions workflow (`s3-throughput-regression-tests.yml`) and provides local reproduction/triage guidance for S3 throughput, metadata, reliability, and multipart completion optimizations.

## Workflow Jobs

### 1. `s3-throughput-regression` (Main Regression Suite)
**Purpose**: Comprehensive regression test for all throughput optimization features.

**Test Coverage**:
- PUT path metrics and FID batching behavior
- GET path metrics, prefetch, and copy buffer options
- Metadata/listing metrics and CommonPrefix dedup
- Multipart completion metrics and latency optimization
- Reliability/retry mechanics for encrypted chunk fetches
- S3 server option normalization/guardrails

**Targets**:
```bash
# Exact test pattern:
go test -v -timeout 5m ./weed/s3api -run 'Test(NextAssignedFid|RecordPutToFilerResultMetrics|GetDownloadChunk|RecordGetObjectResultMetrics|AppendCommonPrefixDedup|RecordMetadataResultMetrics|RecordMultipartResultMetrics|FetchVolumeServerWithRetry|NormalizeThroughputOptions)'
```

**Failure Triage**:
- If `TestNextAssignedFid` fails: Check FID suffix generation in `weed/s3api/s3api_put_to_filer_metrics_test.go` and `s3api_object_handlers_put.go` assign batching logic.
- If `TestRecordPutToFilerResultMetrics` fails: Verify `SeaweedFS_s3_put_to_filer_result_total` metric emission in `s3api_object_handlers_put.go`.
- If `TestGetDownloadChunk*` fails: Check prefetch/copy-buffer option defaults in `s3api_server_options_test.go` and getter functions.
- If `TestRecordGetObjectResultMetrics` fails: Verify GET object metric emission in `s3api_object_handlers.go` streaming paths.
- If `TestAppendCommonPrefixDedup` fails: Check set-based dedup logic in `s3api_object_handlers_list.go`.
- If `TestRecordMetadataResultMetrics` fails: Verify metadata operation result/stage metrics in list handlers.
- If `TestRecordMultipartResultMetrics` fails: Check multipart result/stage metric emission in `s3api_object_handlers_multipart.go`.
- If `TestFetchVolumeServerWithRetry*` fails: Check retry/backoff logic in `s3api_object_handlers.go` chunk fetch paths.
- If `TestNormalizeThroughputOptions*` fails: Verify guardrail normalization in `s3api_server.go` startup.

### 2. `s3-server-options` (Guardrail Validation)
**Purpose**: Validates throughput option guardrails are enforced at startup.

**Guardrails Verified**:
- `uploadChunkParallelism`: `1..128` (default `4`)
- `uploadChunkSizeMB`: `1..1024` (default `8`)
- `downloadChunkPrefetch`: `1..64` (default `4`)
- `downloadCopyBufferKB`: `1..4096` (default `256`)

**Targets**:
```bash
go test -v -timeout 5m ./weed/s3api -run 'TestNormalizeThroughputOptions'
```

**Failure Triage**:
- Out-of-range values should be clamped or reset to defaults.
- Check `s3api_server.go` normalization functions.
- Run with verbose flags to confirm expected warning logs for invalid values.

### 3. `s3-reliability-guards` (Reliability Hardening)
**Purpose**: Validates reliability features for encrypted GET paths.

**Features Validated**:
- Bounded retry/backoff for chunk fetches (3 attempts, 20ms exponential backoff)
- Retryable response codes: `429`, `5xx`
- Timeout/cancel handling
- Reliability event metrics (`retry_attempt`, `retry_success`, `retry_exhausted`, `rate_limited`, `timeout`)

**Targets**:
```bash
go test -v -timeout 5m ./weed/s3api -run 'TestFetchVolumeServerWithRetry'
```

**Failure Triage**:
- If retry success-after-failure fails: Check retry loop and backoff computation in `s3api_object_handlers.go` `fetchVolumeServerWithRetry`.
- If retry exhaustion fails: Verify max attempts (3) and final error return.
- Check reliability event metric emission in the same function.

### 4. `s3-metadata-latency` (Metadata Optimization)
**Purpose**: Validates metadata/listing latency optimizations.

**Optimizations Validated**:
- CommonPrefix dedup: moved from repeated scans to O(1) set-based checks
- Metadata stage/result metrics for LIST operations
- Multipart completion: single per-part latest-entry selection (no repeated re-sorting)

**Targets**:
```bash
go test -v -timeout 5m ./weed/s3api -run 'Test(AppendCommonPrefixDedup|RecordMetadataResultMetrics|RecordMultipartResultMetrics)'
```

**Failure Triage**:
- If `TestAppendCommonPrefixDedup` fails: Check set usage in `s3api_object_handlers_list.go` `listFilerEntries`.
- If metadata metrics fail: Verify stage/result metric helpers in handlers and their invocation points.
- If multipart tests fail: Check `latestPartEntries` map flow in `filer_multipart.go` completion-state prep.

## Local Reproduction

### Run All Regression Tests Locally
```bash
cd /path/to/seaweedfs/weed
go test -v -timeout 5m ./s3api -run 'Test(NextAssignedFid|RecordPutToFilerResultMetrics|GetDownloadChunk|RecordGetObjectResultMetrics|AppendCommonPrefixDedup|RecordMetadataResultMetrics|RecordMultipartResultMetrics|FetchVolumeServerWithRetry|NormalizeThroughputOptions)'
```

### Run Specific Test Groups
```bash
# PUT path tests only
go test -v -timeout 5m ./s3api -run 'Test(NextAssignedFid|RecordPutToFilerResultMetrics)'

# GET path tests only
go test -v -timeout 5m ./s3api -run 'Test(GetDownloadChunk|RecordGetObjectResultMetrics)'

# Metadata tests only
go test -v -timeout 5m ./s3api -run 'Test(AppendCommonPrefixDedup|RecordMetadataResultMetrics|RecordMultipartResultMetrics)'

# Reliability tests only
go test -v -timeout 5m ./s3api -run 'TestFetchVolumeServerWithRetry'

# Server options tests only
go test -v -timeout 5m ./s3api -run 'TestNormalizeThroughputOptions'
```

### Run with Coverage
```bash
cd /path/to/seaweedfs/weed
go test -v -cover -timeout 5m ./s3api -run 'Test(NextAssignedFid|RecordPutToFilerResultMetrics|GetDownloadChunk|RecordGetObjectResultMetrics|AppendCommonPrefixDedup|RecordMetadataResultMetrics|RecordMultipartResultMetrics|FetchVolumeServerWithRetry|NormalizeThroughputOptions)'
```

### Run with Race Detection
```bash
cd /path/to/seaweedfs/weed
go test -race -v -timeout 5m ./s3api -run 'Test(NextAssignedFid|RecordPutToFilerResultMetrics|GetDownloadChunk|RecordGetObjectResultMetrics|AppendCommonPrefixDedup|RecordMetadataResultMetrics|RecordMultipartResultMetrics|FetchVolumeServerWithRetry|NormalizeThroughputOptions)'
```

## Failure Investigation Workflow

### Step 1: Identify Failing Test
- Check GitHub Actions run output for test name.
- Local reproduction: Run exact test via `go test -run 'TestName'`.

### Step 2: Review Test Code
- Test files:
  - `weed/s3api/s3api_put_to_filer_metrics_test.go` (PUT tests)
  - `weed/s3api/s3api_get_object_metrics_test.go` (GET tests)
  - `weed/s3api/s3api_list_metadata_test.go` (metadata tests)
  - `weed/s3api/s3api_multipart_metadata_test.go` (multipart tests)
  - `weed/s3api/s3api_reliability_retry_test.go` (reliability tests)
  - `weed/s3api/s3api_server_options_test.go` (option tests)

### Step 3: Verify Implementation
- Implementation files (in same directory `weed/s3api/`):
  - `s3api_object_handlers_put.go` (PUT batching, metrics)
  - `s3api_object_handlers.go` (GET streaming, retry, metrics)
  - `s3api_object_handlers_list.go` (metadata, dedup)
  - `s3api_object_handlers_multipart.go` (multipart ops, metrics)
  - `filer_multipart.go` (multipart backend, completion state)
  - `s3api_server.go` (server startup, option normalization)

### Step 4: Check Supporting Files
- Metric registration: `weed/stats/metrics.go`
- CLI flag wiring: `weed/command/{s3.go,filer.go,server.go,mini.go}`

### Step 5: Validate Against Test Assertions
- Use test assertions to understand expected behavior.
- Add verbose logging (`-v` flag) for detailed output.
- Check metric state using test helpers like `metricCounterValue`, `metricHistogramSampleCount`.

## Common Failure Scenarios and Fixes

### Scenario 1: Guardrail Test Fails
**Symptom**: `TestNormalizeThroughputOptions*` fails.

**Investigation**:
1. Check if normalization functions exist in `s3api_server.go`.
2. Verify guardrail min/max values are correct (see `sNormalizeThroughputOptions` or similar).
3. Check CLI flag wiring passes values to normalization on startup.

**Fix**:
- Update guardrail ranges if intentional.
- Ensure normalization is called during S3 server startup.
- Add/update warnings for out-of-range values.

### Scenario 2: Metric Emission Test Fails
**Symptom**: `TestRecord*ResultMetrics` fails.

**Investigation**:
1. Check if metrics are registered in `weed/stats/metrics.go`.
2. Verify handlers invoke metric recording helpers (e.g., `recordPutToFilerResult`, `recordMetadataResult`).
3. Check helpers exist in handler file and use correct metric names.

**Fix**:
- Ensure metric registration in `metrics.go`.
- Call helpers at all exit paths (error and success cases).
- Verify metric names and label sets match test expectations.

### Scenario 3: Dedup or Optimization Test Fails
**Symptom**: `TestAppendCommonPrefix*` or multipart completion test fails.

**Investigation**:
1. Check if optimization is actually implemented in handler.
2. Verify data structure changes (e.g., `latestPartEntries` map vs. `partEntries` slice).
3. Look for repeated work that should be deduped.

**Fix**:
- Implement optimization if missing.
- Use test helpers to validate optimization (e.g., `seen` map in dedup test).
- Verify performance-critical paths use optimized code.

### Scenario 4: Retry/Reliability Test Fails
**Symptom**: `TestFetchVolumeServerWithRetry*` fails.

**Investigation**:
1. Check if retry loop exists in `s3api_object_handlers.go` (search for `fetchVolumeServerWithRetry`).
2. Verify retry conditions (429, 5xx) are checked.
3. Check backoff computation and max attempts (should be 3).
4. Verify reliability event metrics are emitted.

**Fix**:
- Implement retry loop if missing.
- Ensure correct response codes trigger retry.
- Set max attempts to 3 and backoff base to 20ms.
- Emit reliability metrics on each event.

## Monitoring and Alerting (Optional)

After deploying with these optimizations, use these PromQL queries in Prometheus:

### Throughput Tuning Effectiveness
```promql
# Multipart complete latency (should decrease with optimization)
histogram_quantile(0.95, rate(SeaweedFS_s3_metadata_stage_seconds_bucket{operation="multipart_complete",stage="complete_call"}[1m]))

# Multipart success ratio (should stay high)
sum(rate(SeaweedFS_s3_metadata_result_total{operation="multipart_complete",result="success"}[1m]))
/
sum(rate(SeaweedFS_s3_metadata_result_total{operation="multipart_complete"}[1m]))
```

### Reliability Event Coverage
```promql
# Retry events should be rare under normal load
sum(rate(SeaweedFS_s3_reliability_event_total{event="retry_attempt"}[1m]))

# Retry success ratio (most retries should succeed)
sum(rate(SeaweedFS_s3_reliability_event_total{event="retry_success"}[1m]))
/
sum(rate(SeaweedFS_s3_reliability_event_total{event="retry_attempt"}[1m]))
```

### Guardrail Validation
- If operators manually set invalid options, expected logs appear in SeaweedFS startup.
- Check `docker logs` or `journalctl` for "invalid s3" warnings.

## References
- Executive Summary: `S3_THROUGHPUT_EXEC_SUMMARY.md`
- README S3 Tuning Section: `README.md` (S3 Upload/Read Throughput Tuning)
- Source Implementation: `weed/s3api/` (see files listed in "Step 3" above)
- Test Files: `weed/s3api/*test.go` files (listed in "Step 3" above)

## Support
For issues with:
- **Test failures**: Follow "Failure Investigation Workflow" above.
- **CI job visibility**: Check GitHub Actions run details and artifact logs.
- **Performance degradation**: Use runbook queries from `S3_THROUGHPUT_EXEC_SUMMARY.md` to attribute cost.
- **Regression confirmation**: Run full test suite locally with `-race` and `-cover` flags.
