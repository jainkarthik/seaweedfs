# SeaweedFS S3 Throughput Executive Summary

## Context
- Storage capability: **~8 GB/s** (NVMe RAID + ZFS)
- Observed SeaweedFS S3 throughput: **~1.2 GB/s**
- Target: **6–8 GB/s**
- Topology: **single copy (no replication fan-out)**

## Primary Bottlenecks Identified
1. **Low fixed per-object chunk upload concurrency** (previously hardcoded to 4)
2. **High per-chunk control-plane overhead** (`AssignVolume` call pressure under high chunk rates)
3. **Upload-path memory copy amplification** (multipart body built as additional full in-memory copy)
4. **Small default chunking for high-bandwidth hardware** (more chunk lifecycle overhead per GiB)

## Changes Implemented
1. **Configurable chunk upload parallelism**
   - Added `uploadChunkParallelism` / `s3.uploadChunkParallelism` (default: `4`)
   - Wired from CLI options into S3 upload chunker path.

2. **Configurable internal chunk size**
   - Added `uploadChunkSizeMB` / `s3.uploadChunkSizeMB` (default: `8`)
   - Enables larger chunks to reduce assign + upload bookkeeping overhead.

3. **Batched assignment consumption in S3 PUT path**
   - `putToFiler` now batches `AssignVolume` usage and consumes reserved FIDs with `_N` suffixing instead of always assigning one chunk at a time.

4. **Streaming multipart upload for large payloads**
   - `operation/upload_content.go` now streams multipart bodies via `io.Pipe` for larger payloads, reducing peak memory and copy overhead.

## Expected Impact
- **Higher effective write throughput** by reducing control-plane calls and per-byte CPU/memory overhead.
- **Better hardware utilization** on high-bandwidth single-copy deployments.
- **Improved stability at high concurrency** from reduced memory pressure during uploads.

## Recommended Initial Tuning (Single-Copy, High-Bandwidth)
Start with:
- `s3.uploadChunkParallelism=16` (then test 24, 32)
- `s3.uploadChunkSizeMB=16` (then test 32)

Validate with sustained multipart PUT workload (large objects, many concurrent clients), and select the best point before CPU saturation or latency regression.

## Benchmark Guidance to Reach 6–8 GB/s
Measure for each candidate setting pair:
- Throughput (GB/s)
- S3 gateway CPU %
- Memory RSS / GC behavior
- Filer/master assign RPC rates and latency
- Volume write latency and error rate

Use a matrix:
- Chunk parallelism: `4, 8, 16, 24, 32`
- Chunk size MB: `8, 16, 32`

Pick the highest stable throughput setting that keeps error rate near zero and avoids runaway memory/CPU contention.

## Baseline Workload Profile (Recommended)
Use a repeatable profile before/after each tuning change.

### Test Matrix
1. Object sizes: `128MiB`, `512MiB`, `2GiB`
2. Client concurrency: `16`, `32`, `64` workers
3. Multipart part size (client side): `16MiB` and `32MiB`
4. SeaweedFS knobs:
   - `s3.uploadChunkParallelism`: `4,8,16,24,32`
   - `s3.uploadChunkSizeMB`: `8,16,32`

### Minimum Metrics to Capture Per Run
- Aggregate upload throughput (GB/s)
- P95/P99 request latency
- S3 process CPU + RSS
- Master/filer assign pressure (request rate + latency)
- Volume server write error rate

### Practical Run Procedure
1. Warm-up: 2 minutes at target concurrency.
2. Measured run: 5–10 minutes sustained upload.
3. Cool-down: 1 minute idle.
4. Repeat each point 3 times and keep median result.

### Acceptance Criterion for Promotion
- Throughput improves vs baseline and stays stable for full run duration.
- No sustained error bursts.
- CPU/memory remain bounded (no runaway behavior).

## Instrumentation Checklist (Hot Path Attribution)
Attribute runtime cost in this order:

1. **S3 gateway CPU/heap**
   - Collect pprof CPU and heap during measured window.
   - Focus symbols:
     - `operation.UploadReaderInChunks`
     - `operation.(*Uploader).upload_content`
     - `s3api.(*S3ApiServer).putToFiler`

2. **Assign path pressure**
   - Track assign request rate and tail latency while increasing concurrency.
   - Verify batching effect by checking lower assign-RPC frequency per GiB uploaded.

3. **Volume write path**
   - Track write failures/timeouts and disk write utilization.
   - Confirm no bottleneck shift from S3 gateway to volume server append path.

4. **Memory amplification**
   - Compare RSS/GC before and after streaming multipart upload path.
   - Validate reduced peak memory under same workload.

### New Built-In Prometheus Metrics Added
- `SeaweedFS_s3_put_to_filer_stage_seconds{stage,bucket}`
  - Stages include: `total`, `assign_rpc`, `chunk_upload`
- `SeaweedFS_s3_put_to_filer_assign_rpc_total{bucket}`
- `SeaweedFS_s3_put_to_filer_assign_batch_size{bucket}`
- `SeaweedFS_s3_put_to_filer_chunk_count{bucket}`
- `SeaweedFS_s3_put_to_filer_result_total{result,bucket}`
- `SeaweedFS_s3_put_to_filer_uploaded_bytes{bucket}`

## Execution Runbook (Operator-Focused)

### 1. Start with baseline settings
- `s3.uploadChunkParallelism=4`
- `s3.uploadChunkSizeMB=8`

Run one full matrix pass, then increase tuning:
- parallelism: `8 -> 16 -> 24 -> 32`
- chunk size: `16 -> 32`

### 2. PromQL queries to compare runs
Use the same time window for each candidate setting pair.

Throughput (bytes/s):
```promql
sum(rate(SeaweedFS_s3_bucket_traffic_received_bytes_total[1m]))
```

putToFiler total p95:
```promql
histogram_quantile(
  0.95,
  sum by (le) (rate(SeaweedFS_s3_put_to_filer_stage_seconds_bucket{stage="total"}[1m]))
)
```

Assign RPC stage p95:
```promql
histogram_quantile(
  0.95,
  sum by (le) (rate(SeaweedFS_s3_put_to_filer_stage_seconds_bucket{stage="assign_rpc"}[1m]))
)
```

Chunk upload stage p95:
```promql
histogram_quantile(
  0.95,
  sum by (le) (rate(SeaweedFS_s3_put_to_filer_stage_seconds_bucket{stage="chunk_upload"}[1m]))
)
```

Assign RPC rate:
```promql
sum(rate(SeaweedFS_s3_put_to_filer_assign_rpc_total[1m]))
```

Average assign batch size:
```promql
sum(rate(SeaweedFS_s3_put_to_filer_assign_batch_size_sum[1m]))
/
sum(rate(SeaweedFS_s3_put_to_filer_assign_batch_size_count[1m]))
```

Average chunk count per uploaded object:
```promql
sum(rate(SeaweedFS_s3_put_to_filer_chunk_count_sum[1m]))
/
sum(rate(SeaweedFS_s3_put_to_filer_chunk_count_count[1m]))
```

putToFiler success ratio:
```promql
sum(rate(SeaweedFS_s3_put_to_filer_result_total{result="success"}[1m]))
/
sum(rate(SeaweedFS_s3_put_to_filer_result_total[1m]))
```

Average uploaded object size (bytes):
```promql
sum(rate(SeaweedFS_s3_put_to_filer_uploaded_bytes_sum[1m]))
/
sum(rate(SeaweedFS_s3_put_to_filer_uploaded_bytes_count[1m]))
```

### 3. Selection rule
Choose the setting pair that gives the highest sustained throughput while:
- keeping error rate flat,
- keeping p95/p99 stage latency stable,
- avoiding runaway memory growth.

## PR-Ready Summary Note
Improves SeaweedFS S3 PUT/multipart throughput for single-copy deployments by removing hot-path bottlenecks in chunk upload orchestration and attribution. Adds configurable upload chunk parallelism (`uploadChunkParallelism` / `s3.uploadChunkParallelism`, default `4`) and configurable internal chunk size (`uploadChunkSizeMB` / `s3.uploadChunkSizeMB`, default `8`). Optimizes control-plane behavior in `putToFiler` via batched assignment consumption using FID suffixes (`_N`). Reduces upload-path memory amplification by streaming large multipart request bodies instead of always constructing an additional full in-memory multipart copy. Adds `put_to_filer_*` Prometheus metrics for stage timing, assign RPC counts, batch size, chunk count, uploaded bytes, and success/error result attribution. Includes targeted regression coverage for batched FID suffix generation and putToFiler success/error metric emission paths.

### Risk / Compatibility
- Backward compatible by default: existing behavior remains with defaults unchanged.
- No protocol changes required for clients.
- Rollback is immediate with:
  - `uploadChunkParallelism=4`
  - `uploadChunkSizeMB=8`

## S3 Read/Download Throughput Track (Started)

### New Read Throughput Knob
- `downloadChunkPrefetch` / `s3.downloadChunkPrefetch` (default `4`)
  - Controls chunk prefetch depth in S3 GET streaming paths (including encrypted full-object fetch path).
- `downloadCopyBufferKB` / `s3.downloadCopyBufferKB` (default `256`)
  - Controls `io.CopyBuffer` size for S3 GET response streaming in non-SSE and SSE paths.

### New Read Path Metrics
- `SeaweedFS_s3_get_object_stage_seconds{stage,bucket}`
- `SeaweedFS_s3_get_object_result_total{result,bucket}` where `result ∈ {success,error,canceled}`
- `SeaweedFS_s3_get_object_downloaded_bytes{bucket}`
- `SeaweedFS_s3_metadata_stage_seconds{operation,stage,bucket}`
- `SeaweedFS_s3_metadata_result_total{operation,result,bucket}`

### Read Track Rollback Default
- `downloadChunkPrefetch=4`
- `downloadCopyBufferKB=256`

### Throughput Knob Guardrails
- `uploadChunkParallelism`: valid range `1..128` (invalid values auto-reset to default `4`)
- `uploadChunkSizeMB`: valid range `1..1024` (invalid values auto-reset to default `8`)
- `downloadChunkPrefetch`: valid range `1..64` (invalid values auto-reset to default `4`)
- `downloadCopyBufferKB`: valid range `1..4096` (invalid values auto-reset to default `256`)

### Read Tuning Addendum (Latest)
Use this read-focused matrix when you run benchmarks later:

- `downloadChunkPrefetch`: `4, 8, 16, 32, 64`
- `downloadCopyBufferKB`: `128, 256, 512, 1024`

Compare candidates with:

Read throughput (bytes/s):
```promql
sum(rate(SeaweedFS_s3_bucket_traffic_sent_bytes_total[1m]))
```

GET total p95:
```promql
histogram_quantile(
  0.95,
  sum by (le) (rate(SeaweedFS_s3_get_object_stage_seconds_bucket{stage="total"}[1m]))
)
```

GET stream execution p95:
```promql
histogram_quantile(
  0.95,
  sum by (le) (rate(SeaweedFS_s3_get_object_stage_seconds_bucket{stage="stream_exec"}[1m]))
)
```

GET success ratio:
```promql
sum(rate(SeaweedFS_s3_get_object_result_total{result="success"}[1m]))
/
sum(rate(SeaweedFS_s3_get_object_result_total[1m]))
```

LIST V2 total p95:
```promql
histogram_quantile(
  0.95,
  sum by (le) (rate(SeaweedFS_s3_metadata_stage_seconds_bucket{operation="list_v2",stage="total"}[1m]))
)
```

LIST V2 filer-list stage p95:
```promql
histogram_quantile(
  0.95,
  sum by (le) (rate(SeaweedFS_s3_metadata_stage_seconds_bucket{operation="list_v2",stage="list_filer_entries"}[1m]))
)
```

## Metadata/Listing Latency Optimization (Latest)

### What changed
1. Added stage-level metadata/listing telemetry:
   - `SeaweedFS_s3_metadata_stage_seconds{operation,stage,bucket}`
   - `SeaweedFS_s3_metadata_result_total{operation,result,bucket}`
2. Instrumented `ListObjectsV1Handler` and `ListObjectsV2Handler` with stage attribution:
   - `parse_args`
   - `list_filer_entries`
   - `bucket_exists_probe`
   - `response_write`
   - `total`
3. Optimized CommonPrefix deduplication in `listFilerEntries`:
   - Replaced repeated linear scans with set-based O(1) dedup checks.
   - Preserves behavior while reducing CPU overhead under high-cardinality delimiter listings.

### Validation scope
- Added focused regression tests for:
  - CommonPrefix dedup helper behavior
  - metadata/listing success/error result metric emission

### Rollback
- Code rollback is standard deploy rollback; no new runtime flags were introduced for this optimization.

## Reliability Hardening

### Implemented
- Added bounded retry/backoff for direct volume-server chunk fetches used by encrypted GET paths:
  - operations: `get_chunk_full`, `get_chunk_view`
  - max attempts: `3`
  - exponential backoff base: `20ms`
  - retryable responses: HTTP `429` and `5xx`
- Added reliability event metric:
  - `SeaweedFS_s3_reliability_event_total{operation,event}`
  - events include: `retry_attempt`, `retry_success`, `rate_limited`, `timeout`, `retry_exhausted`

### Targeted tests added
- retry success-after-failure path
- retry exhaustion path

### Reliability Runbook (Operator)
Use these queries to verify reliability behavior during load/failure tests:

Retry attempt rate:
```promql
sum(rate(SeaweedFS_s3_reliability_event_total{event="retry_attempt"}[1m]))
```

Retry success rate:
```promql
sum(rate(SeaweedFS_s3_reliability_event_total{event="retry_success"}[1m]))
```

Retry exhausted rate:
```promql
sum(rate(SeaweedFS_s3_reliability_event_total{event="retry_exhausted"}[1m]))
```

Rate-limited event rate (429 pressure):
```promql
sum(rate(SeaweedFS_s3_reliability_event_total{event="rate_limited"}[1m]))
```

Timeout event rate:
```promql
sum(rate(SeaweedFS_s3_reliability_event_total{event="timeout"}[1m]))
```

## Multipart Completion/List Latency Optimization (Latest)

### What changed
1. Added stage/result metadata metrics to multipart control-path handlers:
   - operations: `multipart_complete`, `multipart_list_parts`, `multipart_list_uploads`
   - stages: `parse_args` / `decode_xml`, `complete_call` / `list_call`, `response_write`, `total`
   - metrics:
     - `SeaweedFS_s3_metadata_stage_seconds{operation,stage,bucket}`
     - `SeaweedFS_s3_metadata_result_total{operation,result,bucket}`
2. Reduced duplicate work in multipart completion backend:
   - `prepareMultipartCompletionState` now chooses the latest entry per part once, records it, and reuses it for:
     - final chunk assembly
     - multipart ETag calculation
     - composite checksum calculation
     - SSE header propagation source selection
   - avoids repeated per-part resort/reselection passes in follow-on helpers.

### Targeted tests added/updated
- metric result accounting for multipart metadata operation (`success`/`error`, `total` stage sample count)
- multipart ETag tests updated for the new latest-entry map flow

### Rollback
- No runtime flag changes were introduced for this optimization.

### Runbook queries
Multipart complete total p95:
```promql
histogram_quantile(
  0.95,
  sum by (le) (rate(SeaweedFS_s3_metadata_stage_seconds_bucket{operation="multipart_complete",stage="total"}[1m]))
)
```

Multipart complete backend call p95:
```promql
histogram_quantile(
  0.95,
  sum by (le) (rate(SeaweedFS_s3_metadata_stage_seconds_bucket{operation="multipart_complete",stage="complete_call"}[1m]))
)
```

List parts total p95:
```promql
histogram_quantile(
  0.95,
  sum by (le) (rate(SeaweedFS_s3_metadata_stage_seconds_bucket{operation="multipart_list_parts",stage="total"}[1m]))
)
```

Multipart complete success ratio:
```promql
sum(rate(SeaweedFS_s3_metadata_result_total{operation="multipart_complete",result="success"}[1m]))
/
sum(rate(SeaweedFS_s3_metadata_result_total{operation="multipart_complete"}[1m]))
```
