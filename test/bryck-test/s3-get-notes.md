S3 GET Operation Flow in SeaweedFS
1. HTTP Request Entry Point
Client GET Request 
    ↓
HTTP Router (s3api_server.go:515) 
    Route: GET /{bucket}/{object}
    Handler: GetObjectHandler (wrapped in middleware chain)
2. Middleware Chain Execution
HTTP Request 
    ↓
Authentication Middleware (s3_iam_middleware.go:73-270)
    ├── Auth Type Detection (s3api_auth.go:81)
    │   ├── AWS Signature V4
    │   ├── AWS Signature V2
    │   ├── Presigned URLs
    │   ├── JWT tokens
    │   └── Anonymous access
    ├── JWT/Signature Validation
    └── IAM Identity Creation
    ↓
Authorization Middleware (s3_iam_middleware.go:202-270)
    ├── Build S3 Resource ARN
    ├── Extract Request Context (IP, User Agent, etc.)
    ├── Policy Engine Evaluation
    └── Return Allow/Deny Decision
    ↓
Rate Limiting & Circuit Breaker
    ↓
GetObjectHandler (s3api_object_handlers.go:596)
3. GetObjectHandler Main Logic
3.1 Special Object Handling (Lines 603-625)
GetObjectHandler Start
    ↓
Check for SOSAPI Virtual Objects (system.xml, capacity.xml)
    ├── YES → Handle SOSAPI request → Return
    └── NO → Continue
    ↓
Handle Directory Objects (objects ending with "/")
    ├── YES → Handle as directory → Return
    └── NO → Continue as regular object
3.2 Conditional Header Processing (Lines 627-632)
Process Conditional Headers (If-Match, If-None-Match, If-Modified-Since, etc.)
    ├── Condition Failed → Write Error Response (304/412) → Return
    └── Condition Met → Continue with potentially reused entry
3.3 Versioning Detection (Lines 634-755)
Check Bucket Versioning State
    ├── Versioning Not Configured → Error (NoSuchBucket)
    ├── Versioning Enabled → Handle versioned object
    └── Versioning Suspended → Handle suspended versioning
4. Version-Aware Object Resolution
4.1 Versioning Enabled Path (Lines 660-748)
Versioning Enabled
    ↓
Specific Version Request? (versionId query parameter)
    ├── YES → getSpecificObjectVersion(bucket, object, versionId)
    │   └── Look in: bucket/object/.versions/versionId
    └── NO (Latest Version) → Optimized Latest Version Lookup
        ├── Quick Check: Does .versions/ directory exist?
        │   ├── YES → Use getLatestObjectVersion() (find newest)
        │   └── NO → Check regular path for null version
        ↓
Check if Delete Marker → Return NoSuchKey if yes
    ↓
Set Version ID Response Header
    ↓
Extract Object Lock Metadata → Add to response headers
4.2 Versioning Suspended Path (Lines 807-941)
Versioning Suspended
    ↓
Delete Existing Null Version from .versions/
    ↓
Upload Object to Regular Path (overwrites existing)
    ↓
Update All Versions to Set IsLatest=false
4.3 Non-Versioned Path (Lines 950-1110)
No Versioning
    ↓
Direct Object Lookup at: bucket/object
    ↓
Handle Remote Objects (cache to local cluster if needed)
5. Server-Side Encryption Detection (Lines 1085-1108)
Object Entry Available
    ↓
Detect Primary SSE Type from Extended Metadata
    ├── SSE-C (Customer Key) → Validate customer key MD5
    ├── SSE-KMS (AWS KMS) → Extract KMS metadata
    ├── SSE-S3 (SeaweedFS) → Extract S3 key metadata
    └── No Encryption → Skip decryption
6. Range Request Processing (Lines 814-878)
Range Header Present?
    ├── NO → Full Object Download
    └── YES → Partial Content Download
        ↓
PartNumber Parameter Present?
        ├── YES → Multipart Part Request
        │   ├── Validate Part Number
        │   ├── Set Parts Count Header
        │   └── Calculate Part Byte Range
        └── NO → Regular Range Request
            ↓
Parse and Validate Range (start, end, suffix)
            ↓
Adjust Range to Part Boundaries (if multipart)
7. Streaming Optimization (Lines 881-1108)
7.1 Bypass Filer Proxy
NEW OPTIMIZATION: Stream directly from volume servers
This eliminates ~19ms filer proxy overhead
    ↓
streamFromVolumeServersWithSSE()
7.2 Volume Server Direct Streaming
Direct Volume Server Streaming
    ↓
Resolve Chunk Manifests (filer.ResolveChunkManifest)
    ├── For Range Requests: Only fetch needed chunks
    └── For Full Object: Fetch all chunks
    ↓
Prepare Streaming Function (filer.PrepareStreamContentWithThrottler)
    ├── JWT Authentication for Volume Servers
    └── Chunk Resolution with VidMap Cache
    ↓
Stream Data with Inline SSE Decryption
    ├── SSE-C: Decrypt chunk-by-chunk with customer keys
    ├── SSE-KMS: Decrypt with KMS key + IV calculation
    ├── SSE-S3: Decrypt with S3 key + IV handling
    └── No SSE: Stream raw data
8. HTTP Response Generation
8.1 Response Headers (Lines 933-980)
Set All Response Headers (BEFORE WriteHeader)
    ├── Content-Type, Content-Length, ETag
    ├── Last-Modified, Accept-Ranges
    ├── Cache-Control, Expires, Content-Disposition
    ├── User Metadata (x-amz-meta-*)
    ├── SSE Response Headers (if encrypted)
    ├── Version ID (if versioned)
    └── Object Lock Headers (if present)
    ↓
Set Range-Specific Headers (if range request)
    ├── Content-Range: bytes=start-end/total
    └── Content-Length: range size
8.2 HTTP Status & Body
Write HTTP Status Code
    ├── 200 OK (full object)
    ├── 206 Partial Content (range request)
    └── 304 Not Modified (conditional match)
    ↓
Stream Response Body
    ├── For Range Requests: Stream requested byte range
    ├── For Full Objects: Stream entire content
    └── Track bytes written for egress accounting
9. Error Handling & Cleanup
9.1 Stream Error Handling (Lines 892-910)
Streaming Error Detection
    ├── Check if Response Already Written (headers + status)
    ├── YES → Return error (avoid duplicate WriteHeader)
    └── NO → Write S3 Error Response
9.2 Remote Object Caching (Lines 803-825)
Remote-Only Object Detected
    ↓
Cache Remote Object to Local Cluster (singleflight deduplication)
    ├── Successful → Use cached chunks for streaming
    └── Failed → Stream from remote with error fallback
Key Performance Optimizations
1. Filer Proxy Bypass: Direct streaming from volume servers eliminates ~19ms overhead
2. Range-Aware Chunking: Only fetch chunks needed for range requests  
3. VidMap Cache: Reuse volume location lookups across requests
4. Inline SSE Decryption: Decrypt during streaming, no separate decryption step
5. Singleflight Caching: Deduplicate concurrent remote object caching
6. Connection Pooling: Reuse HTTP connections to volume servers
Key Files & Line Numbers
- Router: s3api_server.go:515 - GET route registration
- Auth: s3_iam_middleware.go:73-270 - JWT/SIGV4 auth + policy authorization  
- Main Handler: s3api_object_handlers.go:596 - Core GET logic and streaming
- Streaming: s3api_object_handlers.go:912 - Direct volume server streaming with SSE
- Chunk Resolution: s3api_object_handlers.go:46 - VidMap-based chunk location lookup
- Range Processing: s3api_object_handlers.go:96 - HTTP range parsing and validation