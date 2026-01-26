S3 PUT Operation Flow in SeaweedFS
1. HTTP Request Entry Point
Client PUT Request 
    ↓
HTTP Router (s3api_server.go:522)
    Route: PUT /{bucket}/{object}
    Handler: PutObjectHandler (wrapped in middleware chain)
2. Middleware Chain Execution
HTTP Request 
    ↓
Authentication Middleware (s3_iam_middleware.go)
    ├── Auth Type Detection (s3api_auth.go:81)
    ├── JWT/Signature Validation
    └── IAM Identity Creation
    ↓
Authorization Middleware (s3_iam_middleware.go:202-270)
    ├── Build S3 Resource ARN
    ├── Extract Request Context
    └── Policy Engine Evaluation
    ↓
Rate Limiting & Circuit Breaker
    ↓
PutObjectHandler (s3api_object_handlers_put.go:74)
3. PutObjectHandler Main Logic
3.1 Request Validation (Lines 81-117)
PutObjectHandler Start
    ↓
Validate Content-MD5 Header
    ↓
Check Conditional Headers (If-Match, If-None-Match, etc.)
    ↓
Check Bucket Policy Permissions
    ↓
Validate Cache-Control/Expires Headers
3.2 Directory vs Regular Object (Lines 121-154)
Check if object ends with "/" and small content
    ├── YES → Handle as Directory Object
    │         └── Create directory entry with optional inline content
    └── NO → Continue as Regular Object
3.3 Versioning State Detection (Lines 156-177)
Get Bucket Versioning State
    ├── "Enabled" → putVersionedObject()
    ├── "Suspended" → putSuspendedVersioningObject()  
    └── ""/Other → putToFiler() (regular path)
3.4 Object Lock Validation (Lines 184-208)
Check if Object Lock enabled for bucket
    ├── Validate object lock headers if present
    ├── Check governance bypass permissions
    └── Enforce existing object protections
4. Core Storage Path
4.1 Versioning Enabled Path
putVersionedObject() (s3api_object_handlers_put.go:10012)
    ├── Generate version ID
    ├── Upload to: bucket/object/.versions/versionId
    ├── Update .versions directory metadata
    └── Return: versionId, etag, SSE metadata
4.2 Versioning Suspended Path
putSuspendedVersioningObject() (s3api_object_handlers_put.go:807)
    ├── Set version ID to "null"
    ├── Delete existing null version from .versions
    ├── Upload to: bucket/object (overwrites existing)
    └── Return: etag, SSE metadata
4.3 Regular Path (No Versioning)
putToFiler() (s3api_object_handlers_put.go:281)
    ↓
Server-Side Encryption Processing (s3api_put_handlers.go:291)
    ├── Handle SSE-C (Customer keys)
    ├── Handle SSE-KMS (AWS KMS)  
    ├── Handle SSE-S3 (SeaweedFS managed)
    └── Apply Bucket Default Encryption (if needed)
    ↓
Chunked Upload Preparation
    ├── 8MB chunk size (prevents OOM)
    ├── Volume assignment function
    └── Auto-chunking for large files
    ↓
UploadReaderInChunks() (operation/upload_chunked.go:51)
    ┌─────────────────────────────────────────┐
    │ For each 8MB chunk:              │
    │ ├── Assign volume via filer          │  ← Filer gRPC
    │ ├── Upload chunk to volume server     │  ← HTTP POST
    │ ├── Create FileChunk metadata         │
    │ └── Store encryption per-chunk        │
    └─────────────────────────────────────────┘
    ↓
Create Filer Entry (s3api_object_handlers_put.go:508-637)
    ├── Collect all FileChunks
    ├── Calculate MD5 hash (ETag)
    ├── Set metadata (user, encryption, etc.)
    └── CreateEntry via filer gRPC
    ↓
Save to Filer (s3api_handlers.go:639-669)
    ├── CreateEntry gRPC call
    ├── Store metadata + chunk references
    └── Return success/error
5. Volume Server Storage Layer
Chunk Upload → Volume Server
    ├── Data stored in .dat files
    ├── Index entry in .idx files  
    ├── ETag and size metadata stored
    └── Respond with success/failure
6. HTTP Response Generation
Upload Success
    ↓
Set ETag Header
    ↓
Set SSE Response Headers (if encryption used)
    ↓  
HTTP 200 OK (empty body)
7. Error Handling & Cleanup
Any Failure Point
    ↓
Delete Orphaned Chunks (s3api_object_handlers_put.go:1817)
    ├── Extract file IDs from failed chunks
    ├── Batch delete via volume servers
    └── Log cleanup results
    ↓
Write S3 Error Response
    └── HTTP 4xx/5xx with error XML
Key Files & Line Numbers
- Router: s3api_server.go:522 - PUT route registration
- Auth: s3_iam_middleware.go:73-270 - JWT/SIGV4 auth + policy auth  
- Main Handler: s3api_object_handlers_put.go:74-279 - Entry point and flow control
- Encryption: s3api_put_handlers.go:291-343 - All SSE handling
- Chunking: operation/upload_chunked.go:51-268 - Volume assignment + upload
- Filer Client: s3api_handlers.go:18-88 - gRPC connection management
- Volume Assignment: server/filer_grpc_server.go:312-336 - Volume allocation