package s3api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/seaweedfs/seaweedfs/weed/pb/filer_pb"
	"github.com/seaweedfs/seaweedfs/weed/s3api/s3_constants"
	"github.com/seaweedfs/seaweedfs/weed/s3api/s3err"
)

type H map[string]string

func (h H) String() string {
	pairs := make([]string, 0, len(h))
	for k, v := range h {
		pairs = append(pairs, fmt.Sprintf("%s : %s", k, v))
	}
	sort.Strings(pairs)
	join := strings.Join(pairs, "\n")
	return "\n" + join + "\n"
}

var processMetadataTestCases = []struct {
	caseId   int
	request  H
	existing H
	getTags  H
	want     H
}{
	{
		201,
		H{
			"User-Agent":         "firefox",
			"X-Amz-Meta-My-Meta": "request",
			"X-Amz-Tagging":      "A=B&a=b&type=request",
		},
		H{
			"X-Amz-Meta-My-Meta": "existing",
			"X-Amz-Tagging-A":    "B",
			"X-Amz-Tagging-Type": "existing",
		},
		H{
			"A":    "B",
			"a":    "b",
			"type": "existing",
		},
		H{
			"User-Agent":         "firefox",
			"X-Amz-Meta-My-Meta": "existing",
			"X-Amz-Tagging":      "A=B&a=b&type=existing",
		},
	},
	{
		202,
		H{
			"User-Agent":                      "firefox",
			"X-Amz-Meta-My-Meta":              "request",
			"X-Amz-Tagging":                   "A=B&a=b&type=request",
			s3_constants.AmzUserMetaDirective: DirectiveReplace,
		},
		H{
			"X-Amz-Meta-My-Meta": "existing",
			"X-Amz-Tagging-A":    "B",
			"X-Amz-Tagging-Type": "existing",
		},
		H{
			"A":    "B",
			"a":    "b",
			"type": "existing",
		},
		H{
			"User-Agent":                      "firefox",
			"X-Amz-Meta-My-Meta":              "request",
			"X-Amz-Tagging":                   "A=B&a=b&type=existing",
			s3_constants.AmzUserMetaDirective: DirectiveReplace,
		},
	},

	{
		203,
		H{
			"User-Agent":                           "firefox",
			"X-Amz-Meta-My-Meta":                   "request",
			"X-Amz-Tagging":                        "A=B&a=b&type=request",
			s3_constants.AmzObjectTaggingDirective: DirectiveReplace,
		},
		H{
			"X-Amz-Meta-My-Meta": "existing",
			"X-Amz-Tagging-A":    "B",
			"X-Amz-Tagging-Type": "existing",
		},
		H{
			"A":    "B",
			"a":    "b",
			"type": "existing",
		},
		H{
			"User-Agent":                           "firefox",
			"X-Amz-Meta-My-Meta":                   "existing",
			"X-Amz-Tagging":                        "A=B&a=b&type=request",
			s3_constants.AmzObjectTaggingDirective: DirectiveReplace,
		},
	},

	{
		204,
		H{
			"User-Agent":                           "firefox",
			"X-Amz-Meta-My-Meta":                   "request",
			"X-Amz-Tagging":                        "A=B&a=b&type=request",
			s3_constants.AmzUserMetaDirective:      DirectiveReplace,
			s3_constants.AmzObjectTaggingDirective: DirectiveReplace,
		},
		H{
			"X-Amz-Meta-My-Meta": "existing",
			"X-Amz-Tagging-A":    "B",
			"X-Amz-Tagging-a":    "b",
			"X-Amz-Tagging-Type": "existing",
		},
		H{
			"A":    "B",
			"a":    "b",
			"type": "existing",
		},
		H{
			"User-Agent":                           "firefox",
			"X-Amz-Meta-My-Meta":                   "request",
			"X-Amz-Tagging":                        "A=B&a=b&type=request",
			s3_constants.AmzUserMetaDirective:      DirectiveReplace,
			s3_constants.AmzObjectTaggingDirective: DirectiveReplace,
		},
	},

	{
		205,
		H{
			"User-Agent":                           "firefox",
			"X-Amz-Meta-My-Meta":                   "request",
			"X-Amz-Tagging":                        "A=B&a=b&type=request",
			s3_constants.AmzUserMetaDirective:      DirectiveReplace,
			s3_constants.AmzObjectTaggingDirective: DirectiveReplace,
		},
		H{},
		H{},
		H{
			"User-Agent":                           "firefox",
			"X-Amz-Meta-My-Meta":                   "request",
			"X-Amz-Tagging":                        "A=B&a=b&type=request",
			s3_constants.AmzUserMetaDirective:      DirectiveReplace,
			s3_constants.AmzObjectTaggingDirective: DirectiveReplace,
		},
	},

	{
		206,
		H{
			"User-Agent":                           "firefox",
			s3_constants.AmzUserMetaDirective:      DirectiveReplace,
			s3_constants.AmzObjectTaggingDirective: DirectiveReplace,
		},
		H{
			"X-Amz-Meta-My-Meta": "existing",
			"X-Amz-Tagging-A":    "B",
			"X-Amz-Tagging-a":    "b",
			"X-Amz-Tagging-Type": "existing",
		},
		H{
			"A":    "B",
			"a":    "b",
			"type": "existing",
		},
		H{
			"User-Agent":                           "firefox",
			s3_constants.AmzUserMetaDirective:      DirectiveReplace,
			s3_constants.AmzObjectTaggingDirective: DirectiveReplace,
		},
	},

	{
		207,
		H{
			"User-Agent":                           "firefox",
			"X-Amz-Meta-My-Meta":                   "request",
			s3_constants.AmzUserMetaDirective:      DirectiveReplace,
			s3_constants.AmzObjectTaggingDirective: DirectiveReplace,
		},
		H{
			"X-Amz-Meta-My-Meta": "existing",
			"X-Amz-Tagging-A":    "B",
			"X-Amz-Tagging-a":    "b",
			"X-Amz-Tagging-Type": "existing",
		},
		H{
			"A":    "B",
			"a":    "b",
			"type": "existing",
		},
		H{
			"User-Agent":                           "firefox",
			"X-Amz-Meta-My-Meta":                   "request",
			s3_constants.AmzUserMetaDirective:      DirectiveReplace,
			s3_constants.AmzObjectTaggingDirective: DirectiveReplace,
		},
	},
}
var processMetadataBytesTestCases = []struct {
	caseId   int
	request  H
	existing H
	want     H
}{
	{
		101,
		H{
			"User-Agent":         "firefox",
			"X-Amz-Meta-My-Meta": "request",
			"X-Amz-Tagging":      "A=B&a=b&type=request",
		},
		H{
			"X-Amz-Meta-My-Meta": "existing",
			"X-Amz-Tagging-A":    "B",
			"X-Amz-Tagging-a":    "b",
			"X-Amz-Tagging-type": "existing",
		},
		H{
			"X-Amz-Meta-My-Meta": "existing",
			"X-Amz-Tagging-A":    "B",
			"X-Amz-Tagging-a":    "b",
			"X-Amz-Tagging-type": "existing",
		},
	},

	{
		102,
		H{
			"User-Agent":                      "firefox",
			"X-Amz-Meta-My-Meta":              "request",
			"X-Amz-Tagging":                   "A=B&a=b&type=request",
			s3_constants.AmzUserMetaDirective: DirectiveReplace,
		},
		H{
			"X-Amz-Meta-My-Meta": "existing",
			"X-Amz-Tagging-A":    "B",
			"X-Amz-Tagging-a":    "b",
			"X-Amz-Tagging-type": "existing",
		},
		H{
			"X-Amz-Meta-My-Meta": "request",
			"X-Amz-Tagging-A":    "B",
			"X-Amz-Tagging-a":    "b",
			"X-Amz-Tagging-type": "existing",
		},
	},

	{
		103,
		H{
			"User-Agent":                           "firefox",
			"X-Amz-Meta-My-Meta":                   "request",
			"X-Amz-Tagging":                        "A=B&a=b&type=request",
			s3_constants.AmzObjectTaggingDirective: DirectiveReplace,
		},
		H{
			"X-Amz-Meta-My-Meta": "existing",
			"X-Amz-Tagging-A":    "B",
			"X-Amz-Tagging-a":    "b",
			"X-Amz-Tagging-type": "existing",
		},
		H{
			"X-Amz-Meta-My-Meta": "existing",
			"X-Amz-Tagging-A":    "B",
			"X-Amz-Tagging-a":    "b",
			"X-Amz-Tagging-type": "request",
		},
	},

	{
		104,
		H{
			"User-Agent":                           "firefox",
			"X-Amz-Meta-My-Meta":                   "request",
			"X-Amz-Tagging":                        "A=B&a=b&type=request",
			s3_constants.AmzUserMetaDirective:      DirectiveReplace,
			s3_constants.AmzObjectTaggingDirective: DirectiveReplace,
		},
		H{
			"X-Amz-Meta-My-Meta": "existing",
			"X-Amz-Tagging-A":    "B",
			"X-Amz-Tagging-a":    "b",
			"X-Amz-Tagging-type": "existing",
		},
		H{
			"X-Amz-Meta-My-Meta": "request",
			"X-Amz-Tagging-A":    "B",
			"X-Amz-Tagging-a":    "b",
			"X-Amz-Tagging-type": "request",
		},
	},

	{
		105,
		H{
			"User-Agent":                           "firefox",
			s3_constants.AmzUserMetaDirective:      DirectiveReplace,
			s3_constants.AmzObjectTaggingDirective: DirectiveReplace,
		},
		H{
			"X-Amz-Meta-My-Meta": "existing",
			"X-Amz-Tagging-A":    "B",
			"X-Amz-Tagging-a":    "b",
			"X-Amz-Tagging-type": "existing",
		},
		H{},
	},

	{
		107,
		H{
			"User-Agent":                           "firefox",
			"X-Amz-Meta-My-Meta":                   "request",
			"X-Amz-Tagging":                        "A=B&a=b&type=request",
			s3_constants.AmzUserMetaDirective:      DirectiveReplace,
			s3_constants.AmzObjectTaggingDirective: DirectiveReplace,
		},
		H{},
		H{
			"X-Amz-Meta-My-Meta": "request",
			"X-Amz-Tagging-A":    "B",
			"X-Amz-Tagging-a":    "b",
			"X-Amz-Tagging-type": "request",
		},
	},

	{
		108,
		H{
			"User-Agent":                           "firefox",
			"X-Amz-Meta-My-Meta":                   "request",
			"X-Amz-Tagging":                        "A=B&a=b&type=request*",
			s3_constants.AmzUserMetaDirective:      DirectiveReplace,
			s3_constants.AmzObjectTaggingDirective: DirectiveReplace,
		},
		H{},
		H{},
	},
}

func TestProcessMetadata(t *testing.T) {
	for _, tc := range processMetadataTestCases {
		reqHeader := transferHToHeader(tc.request)
		existing := transferHToHeader(tc.existing)
		replaceMeta, replaceTagging := replaceDirective(reqHeader)
		err := processMetadata(reqHeader, existing, replaceMeta, replaceTagging, func(_ string, _ string) (tags map[string]string, err error) {
			return tc.getTags, nil
		}, "", "")
		if err != nil {
			t.Error(err)
		}

		result := transferHeaderToH(reqHeader)
		fmtTagging(result, tc.want)

		if !reflect.DeepEqual(result, tc.want) {
			t.Error(fmt.Errorf("\n### CaseID: %d ###"+
				"\nRequest:%v"+
				"\nExisting:%v"+
				"\nGetTags:%v"+
				"\nWant:%v"+
				"\nActual:%v",
				tc.caseId, tc.request, tc.existing, tc.getTags, tc.want, result))
		}
	}
}

func TestProcessMetadataBytes(t *testing.T) {
	for _, tc := range processMetadataBytesTestCases {
		reqHeader := transferHToHeader(tc.request)
		existing := transferHToBytesArr(tc.existing)
		replaceMeta, replaceTagging := replaceDirective(reqHeader)
		extends, _ := processMetadataBytes(reqHeader, existing, replaceMeta, replaceTagging)

		result := transferBytesArrToH(extends)
		fmtTagging(result, tc.want)

		if !reflect.DeepEqual(result, tc.want) {
			t.Error(fmt.Errorf("\n### CaseID: %d ###"+
				"\nRequest:%v"+
				"\nExisting:%v"+
				"\nWant:%v"+
				"\nActual:%v",
				tc.caseId, tc.request, tc.existing, tc.want, result))
		}
	}
}

func fmtTagging(maps ...map[string]string) {
	for _, m := range maps {
		if tagging := m[s3_constants.AmzObjectTagging]; len(tagging) > 0 {
			split := strings.Split(tagging, "&")
			sort.Strings(split)
			m[s3_constants.AmzObjectTagging] = strings.Join(split, "&")
		}
	}
}

func transferHToHeader(data map[string]string) http.Header {
	header := http.Header{}
	for k, v := range data {
		header.Add(k, v)
	}
	return header
}

func transferHToBytesArr(data map[string]string) map[string][]byte {
	m := make(map[string][]byte, len(data))
	for k, v := range data {
		m[k] = []byte(v)
	}
	return m
}

func transferBytesArrToH(data map[string][]byte) H {
	m := make(map[string]string, len(data))
	for k, v := range data {
		m[k] = string(v)
	}
	return m
}

func transferHeaderToH(data map[string][]string) H {
	m := make(map[string]string, len(data))
	for k, v := range data {
		m[k] = v[len(v)-1]
	}
	return m
}

// TestShouldCreateVersionForCopy tests the production function that determines
// whether a version should be created during a copy operation.
// This addresses issue #7505 where copies were incorrectly creating versions for non-versioned buckets.
func TestShouldCreateVersionForCopy(t *testing.T) {
	testCases := []struct {
		name            string
		versioningState string
		expectedResult  bool
		description     string
	}{
		{
			name:            "VersioningEnabled",
			versioningState: s3_constants.VersioningEnabled,
			expectedResult:  true,
			description:     "Should create versions in .versions/ directory when versioning is Enabled",
		},
		{
			name:            "VersioningSuspended",
			versioningState: s3_constants.VersioningSuspended,
			expectedResult:  false,
			description:     "Should NOT create versions when versioning is Suspended",
		},
		{
			name:            "VersioningNotConfigured",
			versioningState: "",
			expectedResult:  false,
			description:     "Should NOT create versions when versioning is not configured",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the actual production function
			result := shouldCreateVersionForCopy(tc.versioningState)

			if result != tc.expectedResult {
				t.Errorf("Test case %s failed: %s\nExpected shouldCreateVersionForCopy(%q)=%v, got %v",
					tc.name, tc.description, tc.versioningState, tc.expectedResult, result)
			}
		})
	}
}

// TestCleanupVersioningMetadata tests the production function that removes versioning metadata.
// This ensures objects copied to non-versioned buckets don't carry invalid versioning metadata
// or stale ETag values from the source.
func TestCleanupVersioningMetadata(t *testing.T) {
	testCases := []struct {
		name           string
		sourceMetadata map[string][]byte
		expectedKeys   []string // Keys that should be present after cleanup
		removedKeys    []string // Keys that should be removed
	}{
		{
			name: "RemovesAllVersioningMetadata",
			sourceMetadata: map[string][]byte{
				s3_constants.ExtVersionIdKey:    []byte("version-123"),
				s3_constants.ExtDeleteMarkerKey: []byte("false"),
				s3_constants.ExtIsLatestKey:     []byte("true"),
				s3_constants.ExtETagKey:         []byte("\"abc123\""),
				"X-Amz-Meta-Custom":             []byte("value"),
			},
			expectedKeys: []string{"X-Amz-Meta-Custom"},
			removedKeys:  []string{s3_constants.ExtVersionIdKey, s3_constants.ExtDeleteMarkerKey, s3_constants.ExtIsLatestKey, s3_constants.ExtETagKey},
		},
		{
			name:           "HandlesEmptyMetadata",
			sourceMetadata: map[string][]byte{},
			expectedKeys:   []string{},
			removedKeys:    []string{s3_constants.ExtVersionIdKey, s3_constants.ExtDeleteMarkerKey, s3_constants.ExtIsLatestKey, s3_constants.ExtETagKey},
		},
		{
			name: "PreservesNonVersioningMetadata",
			sourceMetadata: map[string][]byte{
				s3_constants.ExtVersionIdKey: []byte("version-456"),
				s3_constants.ExtETagKey:      []byte("\"def456\""),
				"X-Amz-Meta-Custom":          []byte("value1"),
				"X-Amz-Meta-Another":         []byte("value2"),
				s3_constants.ExtIsLatestKey:  []byte("true"),
			},
			expectedKeys: []string{"X-Amz-Meta-Custom", "X-Amz-Meta-Another"},
			removedKeys:  []string{s3_constants.ExtVersionIdKey, s3_constants.ExtETagKey, s3_constants.ExtIsLatestKey},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a copy of the source metadata
			dstMetadata := make(map[string][]byte)
			for k, v := range tc.sourceMetadata {
				dstMetadata[k] = v
			}

			// Call the actual production function
			cleanupVersioningMetadata(dstMetadata)

			// Verify expected keys are present
			for _, key := range tc.expectedKeys {
				if _, exists := dstMetadata[key]; !exists {
					t.Errorf("Expected key %s to be present in destination metadata", key)
				}
			}

			// Verify removed keys are absent
			for _, key := range tc.removedKeys {
				if _, exists := dstMetadata[key]; exists {
					t.Errorf("Expected key %s to be removed from destination metadata, but it's still present", key)
				}
			}

			// Verify the count matches to ensure no extra keys are present
			if len(dstMetadata) != len(tc.expectedKeys) {
				t.Errorf("Expected %d metadata keys, but got %d. Extra keys might be present.", len(tc.expectedKeys), len(dstMetadata))
			}
		})
	}
}

// TestCopyVersioningIntegration validates the interaction between
// shouldCreateVersionForCopy and cleanupVersioningMetadata functions.
// This integration test ensures the complete fix for issue #7505.
func TestCopyVersioningIntegration(t *testing.T) {
	testCases := []struct {
		name               string
		versioningState    string
		sourceMetadata     map[string][]byte
		expectVersionPath  bool
		expectMetadataKeys []string
	}{
		{
			name:            "EnabledPreservesMetadata",
			versioningState: s3_constants.VersioningEnabled,
			sourceMetadata: map[string][]byte{
				s3_constants.ExtVersionIdKey: []byte("v123"),
				"X-Amz-Meta-Custom":          []byte("value"),
			},
			expectVersionPath: true,
			expectMetadataKeys: []string{
				s3_constants.ExtVersionIdKey,
				"X-Amz-Meta-Custom",
			},
		},
		{
			name:            "SuspendedCleansMetadata",
			versioningState: s3_constants.VersioningSuspended,
			sourceMetadata: map[string][]byte{
				s3_constants.ExtVersionIdKey: []byte("v123"),
				"X-Amz-Meta-Custom":          []byte("value"),
			},
			expectVersionPath: false,
			expectMetadataKeys: []string{
				"X-Amz-Meta-Custom",
			},
		},
		{
			name:            "NotConfiguredCleansMetadata",
			versioningState: "",
			sourceMetadata: map[string][]byte{
				s3_constants.ExtVersionIdKey:    []byte("v123"),
				s3_constants.ExtDeleteMarkerKey: []byte("false"),
				"X-Amz-Meta-Custom":             []byte("value"),
			},
			expectVersionPath: false,
			expectMetadataKeys: []string{
				"X-Amz-Meta-Custom",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test version creation decision using production function
			shouldCreateVersion := shouldCreateVersionForCopy(tc.versioningState)
			if shouldCreateVersion != tc.expectVersionPath {
				t.Errorf("shouldCreateVersionForCopy(%q) = %v, expected %v",
					tc.versioningState, shouldCreateVersion, tc.expectVersionPath)
			}

			// Test metadata cleanup using production function
			metadata := make(map[string][]byte)
			for k, v := range tc.sourceMetadata {
				metadata[k] = v
			}

			if !shouldCreateVersion {
				cleanupVersioningMetadata(metadata)
			}

			// Verify only expected keys remain
			for _, expectedKey := range tc.expectMetadataKeys {
				if _, exists := metadata[expectedKey]; !exists {
					t.Errorf("Expected key %q to be present in metadata", expectedKey)
				}
			}

			// Verify the count matches (no extra keys)
			if len(metadata) != len(tc.expectMetadataKeys) {
				t.Errorf("Expected %d metadata keys, got %d", len(tc.expectMetadataKeys), len(metadata))
			}
		})
	}
}

// TestIsOrphanedSSES3Header tests detection of orphaned SSE-S3 headers.
// This is a regression test for GitHub issue #7562 where copying from an
// encrypted bucket to an unencrypted bucket left behind the encryption header
// without the actual key, causing subsequent copy operations to fail.
func TestIsOrphanedSSES3Header(t *testing.T) {
	testCases := []struct {
		name      string
		headerKey string
		metadata  map[string][]byte
		expected  bool
	}{
		{
			name:      "Not an encryption header",
			headerKey: "X-Amz-Meta-Custom",
			metadata: map[string][]byte{
				"X-Amz-Meta-Custom": []byte("value"),
			},
			expected: false,
		},
		{
			name:      "SSE-S3 header with key present (valid)",
			headerKey: s3_constants.AmzServerSideEncryption,
			metadata: map[string][]byte{
				s3_constants.AmzServerSideEncryption: []byte("AES256"),
				s3_constants.SeaweedFSSSES3Key:       []byte("key-data"),
			},
			expected: false,
		},
		{
			name:      "SSE-S3 header without key (orphaned - GitHub #7562)",
			headerKey: s3_constants.AmzServerSideEncryption,
			metadata: map[string][]byte{
				s3_constants.AmzServerSideEncryption: []byte("AES256"),
			},
			expected: true,
		},
		{
			name:      "SSE-KMS header (not SSE-S3)",
			headerKey: s3_constants.AmzServerSideEncryption,
			metadata: map[string][]byte{
				s3_constants.AmzServerSideEncryption: []byte("aws:kms"),
			},
			expected: false,
		},
		{
			name:      "Different header key entirely",
			headerKey: s3_constants.SeaweedFSSSES3Key,
			metadata: map[string][]byte{
				s3_constants.AmzServerSideEncryption: []byte("AES256"),
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isOrphanedSSES3Header(tc.headerKey, tc.metadata)
			if result != tc.expected {
				t.Errorf("isOrphanedSSES3Header(%q, metadata) = %v, expected %v",
					tc.headerKey, result, tc.expected)
			}
		})
	}
}

func TestPathToBucketObjectAndVersionUsesPathUnescape(t *testing.T) {
	raw := "bucket/folder%2Ffile%2Bname?versionId=v%2B1"
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		t.Fatalf("PathUnescape failed: %v", err)
	}

	bucket, object, versionID := pathToBucketObjectAndVersion(raw, decoded)
	if bucket != "bucket" {
		t.Fatalf("expected bucket %q, got %q", "bucket", bucket)
	}
	if object != "folder/file+name" {
		t.Fatalf("expected object %q, got %q", "folder/file+name", object)
	}
	if versionID != "v+1" {
		t.Fatalf("expected versionId %q, got %q", "v+1", versionID)
	}
}

func TestClassifyCopySourceLookupError(t *testing.T) {
	if got := classifyCopySourceLookupError(filer_pb.ErrNotFound, nil); got != s3err.ErrNoSuchKey {
		t.Fatalf("expected ErrNoSuchKey for missing source, got %v", got)
	}
	if got := classifyCopySourceLookupError(nil, &filer_pb.Entry{IsDirectory: true}); got != s3err.ErrNoSuchKey {
		t.Fatalf("expected ErrNoSuchKey for directory source, got %v", got)
	}
	if got := classifyCopySourceLookupError(nil, &filer_pb.Entry{}); got != s3err.ErrNone {
		t.Fatalf("expected ErrNone for regular source, got %v", got)
	}
}

func TestCopyObjectResultSerialization(t *testing.T) {
	t.Run("serializes checksum fields when present", func(t *testing.T) {
		res := CopyObjectResult{
			ETag:         "\"dc311a9a491138e9d08a49dde9149d1d\"",
			LastModified: time.Now().UTC(),
			ChecksumType: "FULL_OBJECT",
		}
		res.SetChecksum("x-amz-checksum-crc32", "9CDPXw==")

		data := s3err.EncodeXMLResponse(res)
		xmlStr := string(data)

		if !strings.Contains(xmlStr, "<ChecksumCRC32>9CDPXw==</ChecksumCRC32>") {
			t.Errorf("expected XML to contain ChecksumCRC32, got: %s", xmlStr)
		}
		if !strings.Contains(xmlStr, "<ChecksumType>FULL_OBJECT</ChecksumType>") {
			t.Errorf("expected XML to contain ChecksumType, got: %s", xmlStr)
		}
		if !strings.Contains(xmlStr, "<ETag>&#34;dc311a9a491138e9d08a49dde9149d1d&#34;</ETag>") && !strings.Contains(xmlStr, "<ETag>\"dc311a9a491138e9d08a49dde9149d1d\"</ETag>") {
			t.Errorf("expected XML to contain ETag, got: %s", xmlStr)
		}
	})

	t.Run("omits checksum fields when empty", func(t *testing.T) {
		res := CopyObjectResult{
			ETag:         "\"dc311a9a491138e9d08a49dde9149d1d\"",
			LastModified: time.Now().UTC(),
		}

		data := s3err.EncodeXMLResponse(res)
		xmlStr := string(data)

		if strings.Contains(xmlStr, "Checksum") {
			t.Errorf("expected XML to omit checksum fields, got: %s", xmlStr)
		}
	})
}

func TestCopyPartResultSerialization(t *testing.T) {
	res := CopyPartResult{
		ETag:         "\"434be3943355280edc10633946d454f3\"",
		LastModified: time.Now().UTC(),
	}
	res.SetChecksum("CRC64NVME", "checksum64==")

	data := s3err.EncodeXMLResponse(res)
	xmlStr := string(data)

	if !strings.Contains(xmlStr, "<ChecksumCRC64NVME>checksum64==</ChecksumCRC64NVME>") {
		t.Errorf("expected XML to contain ChecksumCRC64NVME, got: %s", xmlStr)
	}
}

func TestWriteCopyObjectResponse(t *testing.T) {
	s3a := &S3ApiServer{}

	entry := &filer_pb.Entry{
		Extended: map[string][]byte{
			s3_constants.ExtChecksumAlgorithm: []byte("x-amz-checksum-crc32"),
			s3_constants.ExtChecksumValue:     []byte("9CDPXw=="),
			s3_constants.ExtChecksumType:      []byte("FULL_OBJECT"),
			s3_constants.AmzServerSideEncryption: []byte("AES256"),
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/dst-bucket/dst-object", nil)
	r.Header.Set("X-Amz-Copy-Source", "/src-bucket/src-object")

	tNow := time.Now().UTC()
	s3a.writeCopyObjectResponse(w, r, "dst-bucket", entry, "\"dc311a9a491138e9d08a49dde9149d1d\"", tNow)

	if w.Header().Get("x-amz-checksum-crc32") != "9CDPXw==" {
		t.Errorf("expected x-amz-checksum-crc32 header, got %q", w.Header().Get("x-amz-checksum-crc32"))
	}
	if w.Header().Get("x-amz-checksum-type") != "FULL_OBJECT" {
		t.Errorf("expected x-amz-checksum-type header, got %q", w.Header().Get("x-amz-checksum-type"))
	}
	if w.Header().Get("x-amz-server-side-encryption") != "AES256" {
		t.Errorf("expected x-amz-server-side-encryption header, got %q", w.Header().Get("x-amz-server-side-encryption"))
	}
	if len(w.Header()["ETag"]) == 0 || w.Header()["ETag"][0] != "\"dc311a9a491138e9d08a49dde9149d1d\"" {
		t.Errorf("expected ETag header, got %v", w.Header()["ETag"])
	}

	body := w.Body.String()
	if !strings.Contains(body, "<ChecksumCRC32>9CDPXw==</ChecksumCRC32>") {
		t.Errorf("expected body to contain ChecksumCRC32, got %s", body)
	}
	if !strings.Contains(body, "<ChecksumType>FULL_OBJECT</ChecksumType>") {
		t.Errorf("expected body to contain ChecksumType, got %s", body)
	}
}

