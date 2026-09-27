package s3api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/seaweedfs/seaweedfs/weed/pb/filer_pb"
	"github.com/seaweedfs/seaweedfs/weed/s3api/s3_constants"
	"github.com/seaweedfs/seaweedfs/weed/s3api/s3err"
)

func TestParseConditionalHeadersAcceptsHTTPDateFormats(t *testing.T) {
	testCases := []struct {
		name     string
		header   string
		value    string
		expected time.Time
	}{
		{
			name:     "If-Modified-Since RFC850",
			header:   s3_constants.IfModifiedSince,
			value:    "Sunday, 06-Nov-94 08:49:37 GMT",
			expected: time.Date(1994, time.November, 6, 8, 49, 37, 0, time.UTC),
		},
		{
			name:     "If-Unmodified-Since ANSIC",
			header:   s3_constants.IfUnmodifiedSince,
			value:    "Sun Nov  6 08:49:37 1994",
			expected: time.Date(1994, time.November, 6, 8, 49, 37, 0, time.UTC),
		},
		{
			// Go clients build this with t.UTC().Format(time.RFC1123); the "UTC"
			// zone is rejected by http.ParseTime but was accepted before, so the
			// RFC1123 fallback must keep it working.
			name:     "If-Modified-Since RFC1123 UTC zone",
			header:   s3_constants.IfModifiedSince,
			value:    "Wed, 21 Oct 2015 07:28:00 UTC",
			expected: time.Date(2015, time.October, 21, 7, 28, 0, 0, time.UTC),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/bucket/object", nil)
			r.Header.Set(testCase.header, testCase.value)

			headers, errCode := parseConditionalHeaders(r)
			if errCode != s3err.ErrNone {
				t.Fatalf("expected %s to be accepted, got %v", testCase.header, errCode)
			}
			if !headers.isSet {
				t.Fatal("expected conditional headers to be marked set")
			}
			parsed := headers.ifModifiedSince
			if testCase.header == s3_constants.IfUnmodifiedSince {
				parsed = headers.ifUnmodifiedSince
			}
			if !parsed.Equal(testCase.expected) {
				t.Fatalf("expected parsed time %v, got %v", testCase.expected, parsed)
			}
		})
	}
}

func TestValidateConditionalCopyHeadersAcceptsHTTPDateFormats(t *testing.T) {
	testCases := []struct {
		name   string
		header string
		value  string
		mtime  int64 // source mtime chosen so the condition passes
	}{
		{
			name:   "X-Amz-Copy-Source-If-Modified-Since RFC850",
			header: s3_constants.AmzCopySourceIfModifiedSince,
			value:  "Sunday, 06-Nov-94 08:49:37 GMT",
			mtime:  1577836800, // 2020-01-01, modified after the 1994 header
		},
		{
			name:   "X-Amz-Copy-Source-If-Unmodified-Since ANSIC",
			header: s3_constants.AmzCopySourceIfUnmodifiedSince,
			value:  "Sun Nov  6 08:49:37 1994",
			mtime:  631152000, // 1990-01-01, not modified after the 1994 header
		},
	}

	var s3a *S3ApiServer // method does not use the receiver
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPut, "/bucket/object", nil)
			r.Header.Set(testCase.header, testCase.value)
			entry := &filer_pb.Entry{Attributes: &filer_pb.FuseAttributes{Mtime: testCase.mtime}}

			if errCode := s3a.validateConditionalCopyHeaders(r, entry); errCode != s3err.ErrNone {
				t.Fatalf("expected %s to be accepted, got %v", testCase.header, errCode)
			}
		})
	}
}
