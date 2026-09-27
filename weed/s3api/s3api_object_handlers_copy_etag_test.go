package s3api

import (
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/seaweedfs/seaweedfs/weed/pb/filer_pb"
	"github.com/seaweedfs/seaweedfs/weed/s3api/s3_constants"
	"github.com/seaweedfs/seaweedfs/weed/s3api/s3err"
	"github.com/seaweedfs/seaweedfs/weed/util"
)

func mustDecodeHexETagForTest(t *testing.T, etag string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(etag)
	if err != nil {
		t.Fatalf("hex.DecodeString(%q) failed: %v", etag, err)
	}
	return decoded
}

func TestCopyEntryETagPrefersStoredExtendedETag(t *testing.T) {
	storedETag := "11111111111111111111111111111111-2"
	entry := newCopyETagTestEntry(t, storedETag, "22222222222222222222222222222222")

	if got := copyEntryETag(entry); got != storedETag {
		t.Fatalf("copyEntryETag() = %q, want stored extended ETag %q", got, storedETag)
	}
}

func TestCopyEntryETagFallsBackToFilerETag(t *testing.T) {
	computedETag := "33333333333333333333333333333333"
	entry := newCopyETagTestEntry(t, "", computedETag)

	if got := strings.Trim(copyEntryETag(entry), `"`); got != computedETag {
		t.Fatalf("copyEntryETag() = %q, want fallback filer ETag %q", got, computedETag)
	}
}

func TestCopyEntryETagInlineContent(t *testing.T) {
	rawContent := []byte("hello from s3-compat-tests\n")
	entry := &filer_pb.Entry{
		Name:    "hello-copy.txt",
		Content: rawContent,
		Attributes: &filer_pb.FuseAttributes{
			FileSize: uint64(len(rawContent)),
			Md5:      mustDecodeHexETagForTest(t, "d1262755aa59ed68965f14eac4eee0b7"),
		},
	}

	etag := copyEntryETag(entry)
	if strings.Trim(etag, `"`) != "d1262755aa59ed68965f14eac4eee0b7" {
		t.Fatalf("copyEntryETag(inline) = %q, want %q", etag, "d1262755aa59ed68965f14eac4eee0b7")
	}
}

func TestValidateConditionalCopyHeadersUsesStoredExtendedETag(t *testing.T) {
	storedETag := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-2"
	entry := newCopyETagTestEntry(t, storedETag, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	s3a := &S3ApiServer{}

	matchReq := httptest.NewRequest("PUT", "/dst", nil)
	matchReq.Header.Set(s3_constants.AmzCopySourceIfMatch, `"`+storedETag+`"`)
	if got := s3a.validateConditionalCopyHeaders(matchReq, entry); got != s3err.ErrNone {
		t.Fatalf("validateConditionalCopyHeaders(If-Match stored ETag) = %v, want %v", got, s3err.ErrNone)
	}

	noneMatchReq := httptest.NewRequest("PUT", "/dst", nil)
	noneMatchReq.Header.Set(s3_constants.AmzCopySourceIfNoneMatch, storedETag)
	if got := s3a.validateConditionalCopyHeaders(noneMatchReq, entry); got != s3err.ErrPreconditionFailed {
		t.Fatalf("validateConditionalCopyHeaders(If-None-Match stored ETag) = %v, want %v", got, s3err.ErrPreconditionFailed)
	}
}

func TestCopyEntryETagFromChunksMatchesRecomputedMd5(t *testing.T) {
	// Regression test for UploadPartCopy returning an empty <ETag></ETag>: when a
	// copied chunk's ETag is derived from the actual copied bytes (as
	// copySingleChunk/copySingleChunkForRange now do via util.Base64Md5), a
	// single-chunk part must produce a valid, non-empty hex MD5 ETag instead of
	// falling back to "" from an empty/stale chunk ETag.
	data := []byte("partcopy payload bytes")
	entry := &filer_pb.Entry{
		Name: "part",
		Attributes: &filer_pb.FuseAttributes{
			FileSize: uint64(len(data)),
		},
		Chunks: []*filer_pb.FileChunk{
			{
				Offset: 0,
				Size:   uint64(len(data)),
				ETag:   util.Base64Md5(data),
			},
		},
	}

	etag := copyEntryETag(entry)
	if etag == "" {
		t.Fatal("copyEntryETag() = \"\", want a non-empty hex MD5")
	}
	want := fmt.Sprintf("%x", util.Md5(data))
	if etag != want {
		t.Fatalf("copyEntryETag() = %q, want %q", etag, want)
	}
}

func TestCopyEntryETagEmptyChunkETagYieldsEmptyResult(t *testing.T) {
	// Documents the failure mode being fixed: an unset chunk ETag decodes to zero
	// bytes and produces an empty ETag string, which is what UploadPartCopy used
	// to return before chunk ETags were recomputed from the copied bytes.
	entry := &filer_pb.Entry{
		Name:       "part",
		Attributes: &filer_pb.FuseAttributes{FileSize: 5},
		Chunks: []*filer_pb.FileChunk{
			{Offset: 0, Size: 5, ETag: ""},
		},
	}

	if etag := copyEntryETag(entry); etag != "" {
		t.Fatalf("copyEntryETag() = %q, want empty string for unset chunk ETag", etag)
	}
}

func newCopyETagTestEntry(t *testing.T, storedETag, computedETag string) *filer_pb.Entry {
	t.Helper()

	entry := &filer_pb.Entry{
		Name: "object",
		Attributes: &filer_pb.FuseAttributes{
			FileSize: 5,
			Md5:      mustDecodeHexETagForTest(t, computedETag),
		},
	}
	if storedETag != "" {
		entry.Extended = map[string][]byte{
			s3_constants.ExtETagKey: []byte(storedETag),
		}
	}
	return entry
}
