package s3api

import (
	"crypto/tls"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/seaweedfs/seaweedfs/weed/pb/filer_pb"
	"github.com/seaweedfs/seaweedfs/weed/s3api/s3err"
	"github.com/stretchr/testify/assert"
)

func TestInitiateMultipartUploadResult(t *testing.T) {

	expected := `<?xml version="1.0" encoding="UTF-8"?>
<InitiateMultipartUploadResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Bucket>example-bucket</Bucket><Key>example-object</Key><UploadId>VXBsb2FkIElEIGZvciA2aWWpbmcncyBteS1tb3ZpZS5tMnRzIHVwbG9hZA</UploadId></InitiateMultipartUploadResult>`
	response := &InitiateMultipartUploadResult{
		CreateMultipartUploadOutput: s3.CreateMultipartUploadOutput{
			Bucket:   aws.String("example-bucket"),
			Key:      aws.String("example-object"),
			UploadId: aws.String("VXBsb2FkIElEIGZvciA2aWWpbmcncyBteS1tb3ZpZS5tMnRzIHVwbG9hZA"),
		},
	}

	encoded := string(s3err.EncodeXMLResponse(response))
	if encoded != expected {
		t.Errorf("unexpected output: %s\nexpecting:%s", encoded, expected)
	}

}

func TestListPartsResult(t *testing.T) {

	expected := `<?xml version="1.0" encoding="UTF-8"?>
<ListPartsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Part><ETag>&#34;12345678&#34;</ETag><LastModified>1970-01-01T00:00:00Z</LastModified><PartNumber>1</PartNumber><Size>123</Size></Part></ListPartsResult>`
	response := &ListPartsResult{
		Part: []*s3.Part{
			{
				PartNumber:   aws.Int64(int64(1)),
				LastModified: aws.Time(time.Unix(0, 0).UTC()),
				Size:         aws.Int64(int64(123)),
				ETag:         aws.String("\"12345678\""),
			},
		},
	}

	encoded := string(s3err.EncodeXMLResponse(response))
	if encoded != expected {
		t.Errorf("unexpected output: %s\nexpecting:%s", encoded, expected)
	}

}

func Test_parsePartNumber(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		partNum  int
	}{
		{
			"first",
			"0001_uuid.part",
			1,
		},
		{
			"second",
			"0002.part",
			2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			partNumber, _ := parsePartNumber(tt.fileName)
			assert.Equalf(t, tt.partNum, partNumber, "parsePartNumber(%v)", tt.fileName)
		})
	}
}

func TestGetEntryNameAndDir(t *testing.T) {
	s3a := &S3ApiServer{
		option: &S3ApiServerOption{
			BucketsPath: "/buckets",
		},
	}

	tests := []struct {
		name           string
		bucket         string
		key            string
		expectedName   string
		expectedDirEnd string // We check the suffix since dir includes BucketsPath
	}{
		{
			name:           "simple file at root",
			bucket:         "test-bucket",
			key:            "/file.txt",
			expectedName:   "file.txt",
			expectedDirEnd: "/buckets/test-bucket",
		},
		{
			name:           "file in subdirectory",
			bucket:         "test-bucket",
			key:            "/folder/file.txt",
			expectedName:   "file.txt",
			expectedDirEnd: "/buckets/test-bucket/folder",
		},
		{
			name:           "file in nested subdirectory",
			bucket:         "test-bucket",
			key:            "/folder/subfolder/file.txt",
			expectedName:   "file.txt",
			expectedDirEnd: "/buckets/test-bucket/folder/subfolder",
		},
		{
			name:           "key without leading slash",
			bucket:         "test-bucket",
			key:            "folder/file.txt",
			expectedName:   "file.txt",
			expectedDirEnd: "/buckets/test-bucket/folder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &s3.CompleteMultipartUploadInput{
				Bucket: aws.String(tt.bucket),
				Key:    aws.String(tt.key),
			}
			entryName, dirName := s3a.getEntryNameAndDir(input)
			assert.Equal(t, tt.expectedName, entryName, "entry name mismatch")
			assert.Equal(t, tt.expectedDirEnd, dirName, "directory mismatch")
		})
	}
}

func TestCalculateMultipartETag(t *testing.T) {
	// Test case: 2 parts matching AWS S3 standard multipart calculation
	// Part 1 MD5: 436a0d2b5cf44766f8ab1fe7520821d9
	// Part 2 MD5: 11797949a441800095985deb7e765752
	// Expected AWS S3 ETag: b6acfe2b815be05dc6ac2bb4870fda16-2
	p1Bytes, _ := hex.DecodeString("436a0d2b5cf44766f8ab1fe7520821d9")
	p2Bytes, _ := hex.DecodeString("11797949a441800095985deb7e765752")

	completedPartNumbers := []int{1, 2}
	partEntries := map[int][]*filer_pb.Entry{
		1: {
			{
				Name: "0001.part",
				Attributes: &filer_pb.FuseAttributes{
					Md5:      p1Bytes,
					FileSize: 5 * 1024 * 1024,
				},
			},
		},
		2: {
			{
				Name: "0002.part",
				Attributes: &filer_pb.FuseAttributes{
					Md5:      p2Bytes,
					FileSize: 1 * 1024 * 1024,
				},
			},
		},
	}
	completedPartMap := map[int][]string{
		1: {"\"436a0d2b5cf44766f8ab1fe7520821d9\""},
		2: {"\"11797949a441800095985deb7e765752\""},
	}

	etag := calculateMultipartETag(completedPartNumbers, partEntries, completedPartMap, nil)
	assert.Equal(t, "b6acfe2b815be05dc6ac2bb4870fda16-2", etag)

	// Test fallback to completedPartMap when entry MD5 is missing
	partEntriesNoMd5 := map[int][]*filer_pb.Entry{
		1: {{Name: "0001.part", Attributes: &filer_pb.FuseAttributes{FileSize: 5 * 1024 * 1024}}},
		2: {{Name: "0002.part", Attributes: &filer_pb.FuseAttributes{FileSize: 1 * 1024 * 1024}}},
	}
	etagFallback := calculateMultipartETag(completedPartNumbers, partEntriesNoMd5, completedPartMap, nil)
	assert.Equal(t, "b6acfe2b815be05dc6ac2bb4870fda16-2", etagFallback)
}

func TestGetRequestScheme(t *testing.T) {
	// 1. Plain HTTP
	req := httptest.NewRequest("POST", "http://example.com/bucket/obj", nil)
	assert.Equal(t, "http", getRequestScheme(req))

	// 2. TLS connection
	reqTLS := httptest.NewRequest("POST", "http://example.com/bucket/obj", nil)
	reqTLS.TLS = &tls.ConnectionState{}
	assert.Equal(t, "https", getRequestScheme(reqTLS))

	// 3. X-Forwarded-Proto: https, http (multi-proxy chain)
	reqFwdProto := httptest.NewRequest("POST", "http://example.com/bucket/obj", nil)
	reqFwdProto.Header.Set("X-Forwarded-Proto", "https, http")
	assert.Equal(t, "https", getRequestScheme(reqFwdProto))

	// 4. RFC 7239 Forwarded header (quoted & mixed case)
	reqForwarded := httptest.NewRequest("POST", "http://example.com/bucket/obj", nil)
	reqForwarded.Header.Set("Forwarded", "for=192.168.1.1;Proto=\"https\";host=example.com")
	assert.Equal(t, "https", getRequestScheme(reqForwarded))

	// 5. X-Forwarded-Ssl: on
	reqFwdSsl := httptest.NewRequest("POST", "http://example.com/bucket/obj", nil)
	reqFwdSsl.Header.Set("X-Forwarded-Ssl", "on")
	assert.Equal(t, "https", getRequestScheme(reqFwdSsl))

	// 6. X-Url-Scheme: https
	reqUrlScheme := httptest.NewRequest("POST", "http://example.com/bucket/obj", nil)
	reqUrlScheme.Header.Set("X-Url-Scheme", "https")
	assert.Equal(t, "https", getRequestScheme(reqUrlScheme))
}
