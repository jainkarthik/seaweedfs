package s3api

import (
	"cmp"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/seaweedfs/seaweedfs/weed/s3api/s3_constants"
	"github.com/seaweedfs/seaweedfs/weed/stats"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/google/uuid"
	"github.com/seaweedfs/seaweedfs/weed/s3api/s3err"

	"net/http"

	"github.com/seaweedfs/seaweedfs/weed/filer"
	"github.com/seaweedfs/seaweedfs/weed/glog"
	"github.com/seaweedfs/seaweedfs/weed/pb/filer_pb"
	"github.com/seaweedfs/seaweedfs/weed/util"
)

const (
	multipartExt     = ".part"
	multiPartMinSize = 5 * 1024 * 1024
)

type InitiateMultipartUploadResult struct {
	XMLName xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ InitiateMultipartUploadResult"`
	s3.CreateMultipartUploadOutput

	// Checksum fields — returned as HTTP response headers, not in the XML body
	ChecksumAlgorithm string `xml:"-"`
	ChecksumType      string `xml:"-"`
}

// getRequestScheme determines the URL scheme (http or https) from the request
// Checks X-Forwarded-Proto header first (for proxies), then TLS state
func getRequestScheme(r *http.Request) string {
	// Check X-Forwarded-Proto header for proxied requests (may be comma-separated list: "client, proxy1")
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		if idx := strings.Index(proto, ","); idx != -1 {
			proto = proto[:idx]
		}
		if p := strings.TrimSpace(strings.ToLower(proto)); p != "" {
			return p
		}
	}
	// Check RFC 7239 Forwarded header (e.g., proto=https or Proto="https")
	if forwarded := r.Header.Get("Forwarded"); forwarded != "" {
		for _, part := range strings.Split(forwarded, ";") {
			part = strings.TrimSpace(part)
			lower := strings.ToLower(part)
			if strings.HasPrefix(lower, "proto=") {
				val := strings.Trim(strings.TrimSpace(part[len("proto="):]), `"`)
				if val != "" {
					return strings.ToLower(val)
				}
			}
		}
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Ssl"), "on") ||
		strings.EqualFold(r.Header.Get("X-Url-Scheme"), "https") ||
		strings.EqualFold(r.Header.Get("Front-End-Https"), "on") {
		return "https"
	}
	// Check if connection is TLS
	if r.TLS != nil {
		return "https"
	}
	if r.URL != nil && r.URL.Scheme != "" {
		return r.URL.Scheme
	}
	return "http"
}

// calculateMultipartETag calculates the AWS S3 standard multipart ETag:
// MD5 of concatenated 16-byte raw binary MD5 digests of each part + "-" + part_count
func calculateMultipartETag(completedPartNumbers []int, partEntries map[int][]*filer_pb.Entry, completedPartMap map[int][]string, finalParts []*filer_pb.FileChunk) string {
	var combinedPartMd5s []byte
	allPartsHaveMd5 := true
	for _, partNumber := range completedPartNumbers {
		partEntriesByNumber := partEntries[partNumber]
		var partMd5 []byte
		if len(partEntriesByNumber) > 0 && len(partEntriesByNumber[0].Attributes.GetMd5()) == 16 {
			partMd5 = partEntriesByNumber[0].Attributes.GetMd5()
		} else if len(completedPartMap[partNumber]) > 0 {
			partHex := strings.Trim(completedPartMap[partNumber][0], `"`)
			if decoded, err := hex.DecodeString(partHex); err == nil && len(decoded) == 16 {
				partMd5 = decoded
			}
		}
		if len(partMd5) != 16 && len(partEntriesByNumber) > 0 {
			partETag := filer.ETag(partEntriesByNumber[0])
			if decoded, err := hex.DecodeString(strings.Trim(partETag, `"`)); err == nil && len(decoded) == 16 {
				partMd5 = decoded
			}
		}
		if len(partMd5) == 16 {
			combinedPartMd5s = append(combinedPartMd5s, partMd5...)
		} else {
			allPartsHaveMd5 = false
			break
		}
	}

	if allPartsHaveMd5 && len(completedPartNumbers) > 0 {
		sum := md5.Sum(combinedPartMd5s)
		return fmt.Sprintf("%x-%d", sum, len(completedPartNumbers))
	}
	return filer.ETagChunks(finalParts)
}

func (s3a *S3ApiServer) createMultipartUpload(r *http.Request, input *s3.CreateMultipartUploadInput) (output *InitiateMultipartUploadResult, code s3err.ErrorCode) {

	glog.V(2).Infof("createMultipartUpload input %v", input)

	uploadIdString := s3a.generateUploadID(*input.Key)

	uploadIdString = uploadIdString + "_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// Validate checksum algorithm before creating the upload directory
	checksumAlgo, checksumHeaderName, checksumErrCode := detectRequestedChecksumAlgorithm(r)
	if checksumErrCode != s3err.ErrNone {
		return nil, checksumErrCode
	}

	checksumType := ""
	if checksumHeaderName != "" {
		resolvedType, typeErr := resolveMultipartChecksumType(checksumAlgo, r.Header.Get(s3_constants.AmzChecksumType))
		if typeErr != nil {
			glog.Warningf("createMultipartUpload: %v", typeErr)
			return nil, s3err.ErrInvalidRequest
		}
		checksumType = resolvedType
	}

	// Prepare error handling outside callback scope
	var encryptionError error

	if err := s3a.mkdir(s3a.genUploadsFolder(*input.Bucket), uploadIdString, func(entry *filer_pb.Entry) {
		if entry.Extended == nil {
			entry.Extended = make(map[string][]byte)
		}
		entry.Extended[s3_constants.ExtMultipartObjectKey] = []byte(*input.Key)
		// Set object owner for multipart upload
		amzAccountId := r.Header.Get(s3_constants.AmzAccountId)
		if amzAccountId != "" {
			entry.Extended[s3_constants.ExtAmzOwnerKey] = []byte(amzAccountId)
		}

		// Store checksum algorithm and type
		if checksumHeaderName != "" {
			entry.Extended[s3_constants.ExtChecksumAlgorithm] = []byte(checksumHeaderName)
			if checksumType != "" {
				entry.Extended[s3_constants.ExtChecksumType] = []byte(checksumType)
			}
		}

		for k, v := range input.Metadata {
			entry.Extended[k] = []byte(*v)
		}
		if input.ContentType != nil {
			entry.Attributes.Mime = *input.ContentType
		}

		// Prepare and apply encryption configuration within directory creation
		// This ensures encryption resources are only allocated if directory creation succeeds
		encryptionConfig, prepErr := s3a.prepareMultipartEncryptionConfig(r, *input.Bucket, uploadIdString)
		if prepErr != nil {
			encryptionError = prepErr
			return // Exit callback, letting mkdir handle the error
		}
		s3a.applyMultipartEncryptionConfig(entry, encryptionConfig)

		// Extract and store object lock metadata from request headers
		// This ensures object lock settings from create_multipart_upload are preserved
		if err := s3a.extractObjectLockMetadataFromRequest(r, entry); err != nil {
			glog.Errorf("createMultipartUpload: failed to extract object lock metadata: %v", err)
			// Don't fail the upload - this matches AWS behavior for invalid metadata
		}
	}); err != nil {
		_, errorCode := handleMultipartInternalError("create multipart upload directory", err)
		return nil, errorCode
	}

	// Check for encryption configuration errors that occurred within the callback
	if encryptionError != nil {
		_, errorCode := handleMultipartInternalError("prepare encryption configuration", encryptionError)
		return nil, errorCode
	}

	output = &InitiateMultipartUploadResult{
		CreateMultipartUploadOutput: s3.CreateMultipartUploadOutput{
			Bucket:   input.Bucket,
			Key:      objectKey(input.Key),
			UploadId: aws.String(uploadIdString),
		},
		ChecksumAlgorithm: checksumAlgorithmNameFromHeaderName(checksumHeaderName),
		ChecksumType:      checksumType,
	}

	return
}

type CompleteMultipartUploadResult struct {
	XMLName  xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ CompleteMultipartUploadResult"`
	Location *string  `xml:"Location,omitempty"`
	Bucket   *string  `xml:"Bucket,omitempty"`
	Key      *string  `xml:"Key,omitempty"`
	ETag     *string  `xml:"ETag,omitempty"`

	ChecksumResult
	ChecksumType string `xml:"ChecksumType,omitempty"`

	// VersionId is NOT included in XML body - it should only be in x-amz-version-id HTTP header

	// Store the VersionId internally for setting HTTP header, but don't marshal to XML
	VersionId *string `xml:"-"`
}

// copySSEHeadersFromFirstPart copies all SSE-related headers from the first part to the destination entry
// This is critical for detectPrimarySSEType to work correctly and ensures encryption metadata is preserved
func copySSEHeadersFromFirstPart(dst *filer_pb.Entry, firstPart *filer_pb.Entry, context string) {
	if firstPart == nil || firstPart.Extended == nil {
		return
	}

	// Copy ALL SSE-related headers (not just SeaweedFSSSEKMSKey)
	sseKeys := []string{
		// SSE-C headers
		s3_constants.SeaweedFSSSEIV,
		s3_constants.AmzServerSideEncryptionCustomerAlgorithm,
		s3_constants.AmzServerSideEncryptionCustomerKeyMD5,
		// SSE-KMS headers
		s3_constants.SeaweedFSSSEKMSKey,
		s3_constants.AmzServerSideEncryptionAwsKmsKeyId,
		// SSE-S3 headers
		s3_constants.SeaweedFSSSES3Key,
		// Common SSE header (for SSE-KMS and SSE-S3)
		s3_constants.AmzServerSideEncryption,
	}

	for _, key := range sseKeys {
		if value, exists := firstPart.Extended[key]; exists {
			dst.Extended[key] = value
			glog.V(4).Infof("completeMultipartUpload: copied SSE header %s from first part (%s)", key, context)
		}
	}
}

func (s3a *S3ApiServer) completeMultipartUpload(r *http.Request, input *s3.CompleteMultipartUploadInput, parts *CompleteMultipartUpload) (output *CompleteMultipartUploadResult, code s3err.ErrorCode) {

	glog.V(2).Infof("completeMultipartUpload input %v", input)
	if len(parts.Parts) == 0 {
		stats.S3HandlerCounter.WithLabelValues(stats.ErrorCompletedNoSuchUpload).Inc()
		return nil, s3err.ErrNoSuchUpload
	}
	completedPartNumbers := []int{}
	completedPartMap := make(map[int][]string)

	maxPartNo := 1

	for _, part := range parts.Parts {
		if _, ok := completedPartMap[part.PartNumber]; !ok {
			completedPartNumbers = append(completedPartNumbers, part.PartNumber)
		}
		completedPartMap[part.PartNumber] = append(completedPartMap[part.PartNumber], part.ETag)
		maxPartNo = maxInt(maxPartNo, part.PartNumber)
	}
	sort.Ints(completedPartNumbers)

	uploadDirectory := s3a.genUploadsFolder(*input.Bucket) + "/" + *input.UploadId
	// Use explicit limit to ensure all parts are listed (up to S3's max of 10,000 parts)
	// Previously limit=0 relied on server's DirListingLimit default (1000 in weed server mode),
	// which caused CompleteMultipartUpload to fail for uploads with more than 1000 parts.
	entries, _, err := s3a.list(uploadDirectory, "", "", false, s3_constants.MaxS3MultipartParts+1)
	if err != nil {
		glog.Errorf("completeMultipartUpload %s %s error: %v, entries:%d", *input.Bucket, *input.UploadId, err, len(entries))
		stats.S3HandlerCounter.WithLabelValues(stats.ErrorCompletedNoSuchUpload).Inc()
		return nil, s3err.ErrNoSuchUpload
	}

	if len(entries) == 0 {
		entryName, dirName := s3a.getEntryNameAndDir(input)
		if entry, _ := s3a.getEntry(dirName, entryName); entry != nil && entry.Extended != nil {
			if uploadId, ok := entry.Extended[s3_constants.SeaweedFSUploadId]; ok && *input.UploadId == string(uploadId) {
				// Location uses the S3 endpoint that the client connected to
				// Format: scheme://s3-endpoint/bucket/object (following AWS S3 API)
				etag := filer.ETagChunks(entry.GetChunks())
				if savedEtag, ok := entry.Extended[s3_constants.ExtETagKey]; ok && len(savedEtag) > 0 {
					etag = strings.Trim(string(savedEtag), `"`)
				}
				res := &CompleteMultipartUploadResult{
					Location:     aws.String(fmt.Sprintf("%s://%s/%s/%s", getRequestScheme(r), r.Host, url.PathEscape(*input.Bucket), urlPathEscape(*input.Key))),
					Bucket:       input.Bucket,
					ETag:         aws.String("\"" + etag + "\""),
					Key:          objectKey(input.Key),
					ChecksumType: string(entry.Extended[s3_constants.ExtChecksumType]),
				}
				res.SetChecksum(string(entry.Extended[s3_constants.ExtChecksumAlgorithm]), string(entry.Extended[s3_constants.ExtChecksumValue]))
				return res, s3err.ErrNone
			}
		}
		stats.S3HandlerCounter.WithLabelValues(stats.ErrorCompletedNoSuchUpload).Inc()
		return nil, s3err.ErrNoSuchUpload
	}

	pentry, err := s3a.getEntry(s3a.genUploadsFolder(*input.Bucket), *input.UploadId)
	if err != nil {
		glog.Errorf("completeMultipartUpload %s %s error: %v", *input.Bucket, *input.UploadId, err)
		stats.S3HandlerCounter.WithLabelValues(stats.ErrorCompletedNoSuchUpload).Inc()
		return nil, s3err.ErrNoSuchUpload
	}
	deleteEntries := []*filer_pb.Entry{}
	partEntries := make(map[int][]*filer_pb.Entry, len(entries))
	entityTooSmall := false
	entityWithTtl := false
	for _, entry := range entries {
		foundEntry := false
		glog.V(4).Infof("completeMultipartUpload part entries %s", entry.Name)
		if entry.IsDirectory || !strings.HasSuffix(entry.Name, multipartExt) {
			continue
		}
		partNumber, err := parsePartNumber(entry.Name)
		if err != nil {
			stats.S3HandlerCounter.WithLabelValues(stats.ErrorCompletedPartNumber).Inc()
			glog.Errorf("completeMultipartUpload failed to pasre partNumber %s:%s", entry.Name, err)
			continue
		}
		completedPartsByNumber, ok := completedPartMap[partNumber]
		if !ok {
			continue
		}
		for _, partETag := range completedPartsByNumber {
			partETag = strings.Trim(partETag, `"`)
			entryETag := hex.EncodeToString(entry.Attributes.GetMd5())
			if partETag != "" && len(partETag) == 32 && entryETag != "" {
				if entryETag != partETag {
					glog.Errorf("completeMultipartUpload %s ETag mismatch chunk: %s part: %s", entry.Name, entryETag, partETag)
					stats.S3HandlerCounter.WithLabelValues(stats.ErrorCompletedEtagMismatch).Inc()
					continue
				}
			} else {
				glog.Warningf("invalid complete etag %s, partEtag %s", partETag, entryETag)
				stats.S3HandlerCounter.WithLabelValues(stats.ErrorCompletedEtagInvalid).Inc()
			}
			if len(entry.Chunks) == 0 && partNumber != maxPartNo {
				glog.Warningf("completeMultipartUpload %s empty chunks", entry.Name)
				stats.S3HandlerCounter.WithLabelValues(stats.ErrorCompletedPartEmpty).Inc()
				continue
			}
			//there maybe multi same part, because of client retry
			partEntries[partNumber] = append(partEntries[partNumber], entry)
			foundEntry = true
		}
		if foundEntry {
			if !entityWithTtl && entry.Attributes != nil && entry.Attributes.TtlSec > 0 {
				entityWithTtl = true
			}
			if len(completedPartNumbers) > 1 && partNumber != completedPartNumbers[len(completedPartNumbers)-1] &&
				entry.Attributes.FileSize < multiPartMinSize {
				glog.Warningf("completeMultipartUpload %s part file size less 5mb", entry.Name)
				entityTooSmall = true
			}
		} else {
			deleteEntries = append(deleteEntries, entry)
		}
	}
	if entityTooSmall {
		stats.S3HandlerCounter.WithLabelValues(stats.ErrorCompleteEntityTooSmall).Inc()
		return nil, s3err.ErrEntityTooSmall
	}
	mime := pentry.Attributes.Mime
	var finalParts []*filer_pb.FileChunk
	var offset int64

	// Track part boundaries for later retrieval with PartNumber parameter
	type PartBoundary struct {
		PartNumber int    `json:"part"`
		StartChunk int    `json:"start"`
		EndChunk   int    `json:"end"` // exclusive
		ETag       string `json:"etag"`
	}
	var partBoundaries []PartBoundary

	for _, partNumber := range completedPartNumbers {
		partEntriesByNumber, ok := partEntries[partNumber]
		if !ok {
			glog.Errorf("part %d has no entry", partNumber)
			stats.S3HandlerCounter.WithLabelValues(stats.ErrorCompletedPartNotFound).Inc()
			return nil, s3err.ErrInvalidPart
		}
		found := false
		if len(partEntriesByNumber) > 1 {
			slices.SortFunc(partEntriesByNumber, func(a, b *filer_pb.Entry) int {
				return cmp.Compare(b.Chunks[0].ModifiedTsNs, a.Chunks[0].ModifiedTsNs)
			})
		}
		for _, entry := range partEntriesByNumber {
			if found {
				deleteEntries = append(deleteEntries, entry)
				stats.S3HandlerCounter.WithLabelValues(stats.ErrorCompletedPartEntryMismatch).Inc()
				continue
			}

			// Record the start chunk index for this part
			partStartChunk := len(finalParts)

			// Calculate the part's ETag (for GetObject with PartNumber)
			partETag := filer.ETag(entry)

			for _, chunk := range entry.GetChunks() {
				// CRITICAL: Do NOT modify SSE metadata offsets during assembly!
				// The encrypted data was created with the offset stored in chunk.SseMetadata.
				// Changing the offset here would cause decryption to fail because CTR mode
				// uses the offset to initialize the counter. We must decrypt with the same
				// offset that was used during encryption.

				p := &filer_pb.FileChunk{
					FileId:       chunk.GetFileIdString(),
					Offset:       offset,
					Size:         chunk.Size,
					ModifiedTsNs: chunk.ModifiedTsNs,
					CipherKey:    chunk.CipherKey,
					ETag:         chunk.ETag,
					IsCompressed: chunk.IsCompressed,
					// Preserve SSE metadata UNCHANGED - do not modify the offset!
					SseType:     chunk.SseType,
					SseMetadata: chunk.SseMetadata,
				}
				finalParts = append(finalParts, p)
				offset += int64(chunk.Size)
			}

			// Record the part boundary
			partEndChunk := len(finalParts)
			partBoundaries = append(partBoundaries, PartBoundary{
				PartNumber: partNumber,
				StartChunk: partStartChunk,
				EndChunk:   partEndChunk,
				ETag:       partETag,
			})

			found = true
		}
	}

	entryName, dirName := s3a.getEntryNameAndDir(input)
	etag := "\"" + calculateMultipartETag(completedPartNumbers, partEntries, completedPartMap, finalParts) + "\""

	// Compute composite or full-object checksum if the upload was initiated with a checksum algorithm
	checksumHeaderName := ""
	checksumType := ""
	checksumValue := ""
	if pentry.Extended != nil {
		if algoName, ok := pentry.Extended[s3_constants.ExtChecksumAlgorithm]; ok {
			checksumHeaderName = string(algoName)
		}
	}
	if checksumHeaderName != "" {
		algo := checksumAlgorithmFromHeaderName(checksumHeaderName)
		requestedType := ""
		if pentry.Extended != nil {
			requestedType = string(pentry.Extended[s3_constants.ExtChecksumType])
		}
		resolvedType, typeErr := resolveMultipartChecksumType(algo, requestedType)
		if typeErr != nil {
			glog.Errorf("completeMultipartUpload: %v", typeErr)
			return nil, s3err.ErrInvalidRequest
		}
		checksumType = resolvedType

		var checksumErr error
		if checksumType == s3_constants.ChecksumTypeFullObject {
			checksumValue, checksumErr = computeFullObjectChecksum(checksumHeaderName, partEntries, completedPartNumbers)
		} else {
			checksumValue, checksumErr = computeCompositeChecksum(checksumHeaderName, partEntries, completedPartNumbers)
		}
		if checksumErr != nil {
			glog.Errorf("completeMultipartUpload: %s checksum computation failed: %v", checksumType, checksumErr)
			return nil, s3err.ErrInvalidPart
		}
	}

	// Check if versioning is configured for this bucket BEFORE creating any files
	versioningState, vErr := s3a.getVersioningState(*input.Bucket)
	if vErr == nil && versioningState == s3_constants.VersioningEnabled {
		// Use full object key (not just entryName) to ensure correct .versions directory is checked
		normalizedKey := strings.TrimPrefix(*input.Key, "/")
		useInvertedFormat := s3a.getVersionIdFormat(*input.Bucket, normalizedKey)
		versionId := generateVersionId(useInvertedFormat)
		versionFileName := s3a.getVersionFileName(versionId)
		versionDir := dirName + "/" + entryName + s3_constants.VersionsFolder

		// Capture timestamp and owner once for consistency between version entry and cache entry
		versionMtime := time.Now().Unix()
		amzAccountId := r.Header.Get(s3_constants.AmzAccountId)

		// Create the version file in the .versions directory
		err = s3a.mkFile(versionDir, versionFileName, finalParts, func(versionEntry *filer_pb.Entry) {
			if versionEntry.Extended == nil {
				versionEntry.Extended = make(map[string][]byte)
			}
			versionEntry.Extended[s3_constants.ExtVersionIdKey] = []byte(versionId)
			versionEntry.Extended[s3_constants.SeaweedFSUploadId] = []byte(*input.UploadId)
			versionEntry.Extended[s3_constants.ExtETagKey] = []byte(etag)
			// Store parts count for x-amz-mp-parts-count header
			versionEntry.Extended[s3_constants.SeaweedFSMultipartPartsCount] = []byte(fmt.Sprintf("%d", len(completedPartNumbers)))
			// Store part boundaries for GetObject with PartNumber
			if partBoundariesJSON, err := json.Marshal(partBoundaries); err == nil {
				versionEntry.Extended[s3_constants.SeaweedFSMultipartPartBoundaries] = partBoundariesJSON
			}

			// Store composite checksum if computed
			if checksumHeaderName != "" && checksumValue != "" {
				versionEntry.Extended[s3_constants.ExtChecksumAlgorithm] = []byte(checksumHeaderName)
				versionEntry.Extended[s3_constants.ExtChecksumValue] = []byte(checksumValue)
				if checksumType != "" {
					versionEntry.Extended[s3_constants.ExtChecksumType] = []byte(checksumType)
				}
			}

			// Set object owner for versioned multipart objects
			if amzAccountId != "" {
				versionEntry.Extended[s3_constants.ExtAmzOwnerKey] = []byte(amzAccountId)
			}

			for k, v := range pentry.Extended {
				if k != s3_constants.ExtMultipartObjectKey {
					versionEntry.Extended[k] = v
				}
			}

			// Preserve ALL SSE metadata from the first part (if any)
			// SSE metadata is stored in individual parts, not the upload directory
			if len(completedPartNumbers) > 0 && len(partEntries[completedPartNumbers[0]]) > 0 {
				firstPartEntry := partEntries[completedPartNumbers[0]][0]
				copySSEHeadersFromFirstPart(versionEntry, firstPartEntry, "versioned")
			}
			if pentry.Attributes.Mime != "" {
				versionEntry.Attributes.Mime = pentry.Attributes.Mime
			} else if mime != "" {
				versionEntry.Attributes.Mime = mime
			}
			versionEntry.Attributes.FileSize = uint64(offset)
			versionEntry.Attributes.Mtime = versionMtime
		})

		if err != nil {
			glog.Errorf("completeMultipartUpload: failed to create version %s: %v", versionId, err)
			return nil, s3err.ErrInternalError
		}

		// Construct entry with metadata for caching in .versions directory
		// Reuse versionMtime to keep list vs. HEAD timestamps aligned
		versionEntryForCache := &filer_pb.Entry{
			Attributes: &filer_pb.FuseAttributes{
				FileSize: uint64(offset),
				Mtime:    versionMtime,
			},
			Extended: map[string][]byte{
				s3_constants.ExtETagKey: []byte(etag),
			},
		}
		if amzAccountId != "" {
			versionEntryForCache.Extended[s3_constants.ExtAmzOwnerKey] = []byte(amzAccountId)
		}

		// Update the .versions directory metadata to indicate this is the latest version
		// Pass entry to cache its metadata for single-scan list efficiency
		err = s3a.updateLatestVersionInDirectory(*input.Bucket, *input.Key, versionId, versionFileName, versionEntryForCache)
		if err != nil {
			glog.Errorf("completeMultipartUpload: failed to update latest version in directory: %v", err)
			return nil, s3err.ErrInternalError
		}

		// For versioned buckets, don't create a main object file - all content is stored in .versions directory
		// The latest version information is tracked in the .versions directory metadata

		output = &CompleteMultipartUploadResult{
			Location:     aws.String(fmt.Sprintf("%s://%s/%s/%s", getRequestScheme(r), r.Host, url.PathEscape(*input.Bucket), urlPathEscape(*input.Key))),
			Bucket:       input.Bucket,
			ETag:         aws.String(etag),
			Key:          objectKey(input.Key),
			VersionId:    aws.String(versionId),
			ChecksumType: checksumType,
		}
		output.SetChecksum(checksumHeaderName, checksumValue)
	} else if vErr == nil && versioningState == s3_constants.VersioningSuspended {
		// For suspended versioning, add "null" version ID metadata and return "null" version ID
		err = s3a.mkFile(dirName, entryName, finalParts, func(entry *filer_pb.Entry) {
			if entry.Extended == nil {
				entry.Extended = make(map[string][]byte)
			}
			entry.Extended[s3_constants.ExtVersionIdKey] = []byte("null")
			entry.Extended[s3_constants.SeaweedFSUploadId] = []byte(*input.UploadId)
			entry.Extended[s3_constants.ExtETagKey] = []byte(etag)
			// Store parts count for x-amz-mp-parts-count header
			entry.Extended[s3_constants.SeaweedFSMultipartPartsCount] = []byte(fmt.Sprintf("%d", len(completedPartNumbers)))
			// Store part boundaries for GetObject with PartNumber
			if partBoundariesJSON, jsonErr := json.Marshal(partBoundaries); jsonErr == nil {
				entry.Extended[s3_constants.SeaweedFSMultipartPartBoundaries] = partBoundariesJSON
			}

			// Store composite checksum if computed
			if checksumHeaderName != "" && checksumValue != "" {
				entry.Extended[s3_constants.ExtChecksumAlgorithm] = []byte(checksumHeaderName)
				entry.Extended[s3_constants.ExtChecksumValue] = []byte(checksumValue)
				if checksumType != "" {
					entry.Extended[s3_constants.ExtChecksumType] = []byte(checksumType)
				}
			}

			// Set object owner for suspended versioning multipart objects
			amzAccountId := r.Header.Get(s3_constants.AmzAccountId)
			if amzAccountId != "" {
				entry.Extended[s3_constants.ExtAmzOwnerKey] = []byte(amzAccountId)
			}

			for k, v := range pentry.Extended {
				if k != s3_constants.ExtMultipartObjectKey {
					entry.Extended[k] = v
				}
			}

			// Preserve ALL SSE metadata from the first part (if any)
			// SSE metadata is stored in individual parts, not the upload directory
			if len(completedPartNumbers) > 0 && len(partEntries[completedPartNumbers[0]]) > 0 {
				firstPartEntry := partEntries[completedPartNumbers[0]][0]
				copySSEHeadersFromFirstPart(entry, firstPartEntry, "suspended versioning")
			}
			if pentry.Attributes.Mime != "" {
				entry.Attributes.Mime = pentry.Attributes.Mime
			} else if mime != "" {
				entry.Attributes.Mime = mime
			}
			entry.Attributes.FileSize = uint64(offset)
		})

		if err != nil {
			glog.Errorf("completeMultipartUpload: failed to create suspended versioning object: %v", err)
			return nil, s3err.ErrInternalError
		}

		// Note: Suspended versioning should NOT return VersionId field according to AWS S3 spec
		output = &CompleteMultipartUploadResult{
			Location:     aws.String(fmt.Sprintf("%s://%s/%s/%s", getRequestScheme(r), r.Host, url.PathEscape(*input.Bucket), urlPathEscape(*input.Key))),
			Bucket:       input.Bucket,
			ETag:         aws.String(etag),
			Key:          objectKey(input.Key),
			ChecksumType: checksumType,
			// VersionId field intentionally omitted for suspended versioning
		}
		output.SetChecksum(checksumHeaderName, checksumValue)
	} else {
		// For non-versioned buckets, create main object file
		err = s3a.mkFile(dirName, entryName, finalParts, func(entry *filer_pb.Entry) {
			if entry.Extended == nil {
				entry.Extended = make(map[string][]byte)
			}
			entry.Extended[s3_constants.SeaweedFSUploadId] = []byte(*input.UploadId)
			entry.Extended[s3_constants.ExtETagKey] = []byte(etag)
			// Store parts count for x-amz-mp-parts-count header
			entry.Extended[s3_constants.SeaweedFSMultipartPartsCount] = []byte(fmt.Sprintf("%d", len(completedPartNumbers)))
			// Store part boundaries for GetObject with PartNumber
			if partBoundariesJSON, err := json.Marshal(partBoundaries); err == nil {
				entry.Extended[s3_constants.SeaweedFSMultipartPartBoundaries] = partBoundariesJSON
			}

			// Store composite checksum if computed
			if checksumHeaderName != "" && checksumValue != "" {
				entry.Extended[s3_constants.ExtChecksumAlgorithm] = []byte(checksumHeaderName)
				entry.Extended[s3_constants.ExtChecksumValue] = []byte(checksumValue)
				if checksumType != "" {
					entry.Extended[s3_constants.ExtChecksumType] = []byte(checksumType)
				}
			}

			// Set object owner for non-versioned multipart objects
			amzAccountId := r.Header.Get(s3_constants.AmzAccountId)
			if amzAccountId != "" {
				entry.Extended[s3_constants.ExtAmzOwnerKey] = []byte(amzAccountId)
			}

			for k, v := range pentry.Extended {
				if k != s3_constants.ExtMultipartObjectKey {
					entry.Extended[k] = v
				}
			}

			// Preserve ALL SSE metadata from the first part (if any)
			// SSE metadata is stored in individual parts, not the upload directory
			if len(completedPartNumbers) > 0 && len(partEntries[completedPartNumbers[0]]) > 0 {
				firstPartEntry := partEntries[completedPartNumbers[0]][0]
				copySSEHeadersFromFirstPart(entry, firstPartEntry, "non-versioned")
			}
			if pentry.Attributes.Mime != "" {
				entry.Attributes.Mime = pentry.Attributes.Mime
			} else if mime != "" {
				entry.Attributes.Mime = mime
			}
			entry.Attributes.FileSize = uint64(offset)
			// Set TTL-based S3 expiry (modification time)
			if entityWithTtl {
				entry.Extended[s3_constants.SeaweedFSExpiresS3] = []byte("true")
			}
		})

		if err != nil {
			glog.Errorf("completeMultipartUpload %s/%s error: %v", dirName, entryName, err)
			return nil, s3err.ErrInternalError
		}

		// For non-versioned buckets, return response without VersionId
		output = &CompleteMultipartUploadResult{
			Location:     aws.String(fmt.Sprintf("%s://%s/%s/%s", getRequestScheme(r), r.Host, url.PathEscape(*input.Bucket), urlPathEscape(*input.Key))),
			Bucket:       input.Bucket,
			ETag:         aws.String(etag),
			Key:          objectKey(input.Key),
			ChecksumType: checksumType,
		}
		output.SetChecksum(checksumHeaderName, checksumValue)
	}

	for _, deleteEntry := range deleteEntries {
		//delete unused part data
		if err = s3a.rm(uploadDirectory, deleteEntry.Name, true, true); err != nil {
			glog.Warningf("completeMultipartUpload cleanup %s upload %s unused %s : %v", *input.Bucket, *input.UploadId, deleteEntry.Name, err)
		}
	}
	if err = s3a.rm(s3a.genUploadsFolder(*input.Bucket), *input.UploadId, false, true); err != nil {
		glog.V(1).Infof("completeMultipartUpload cleanup %s upload %s: %v", *input.Bucket, *input.UploadId, err)
	}

	return
}

func (s3a *S3ApiServer) getEntryNameAndDir(input *s3.CompleteMultipartUploadInput) (string, string) {
	entryName := path.Base(*input.Key)
	dirName := path.Dir(*input.Key)
	if dirName == "." {
		dirName = ""
	}
	dirName = strings.TrimPrefix(dirName, "/")
	dirName = fmt.Sprintf("%s/%s/%s", s3a.option.BucketsPath, *input.Bucket, dirName)

	// remove suffix '/'
	dirName = strings.TrimSuffix(dirName, "/")
	return entryName, dirName
}

func parsePartNumber(fileName string) (int, error) {
	var partNumberString string
	index := strings.Index(fileName, "_")
	if index != -1 {
		partNumberString = fileName[:index]
	} else {
		partNumberString = fileName[:len(fileName)-len(multipartExt)]
	}
	return strconv.Atoi(partNumberString)
}

func (s3a *S3ApiServer) abortMultipartUpload(input *s3.AbortMultipartUploadInput) (output *s3.AbortMultipartUploadOutput, code s3err.ErrorCode) {

	glog.V(2).Infof("abortMultipartUpload input %v", input)

	exists, err := s3a.exists(s3a.genUploadsFolder(*input.Bucket), *input.UploadId, true)
	if err != nil {
		glog.V(1).Infof("bucket %s abort upload %s: %v", *input.Bucket, *input.UploadId, err)
		return nil, s3err.ErrNoSuchUpload
	}
	if exists {
		err = s3a.rm(s3a.genUploadsFolder(*input.Bucket), *input.UploadId, true, true)
	}
	if err != nil {
		glog.V(1).Infof("bucket %s remove upload %s: %v", *input.Bucket, *input.UploadId, err)
		return nil, s3err.ErrInternalError
	}

	return &s3.AbortMultipartUploadOutput{}, s3err.ErrNone
}

type ListMultipartUploadsResult struct {
	XMLName xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ ListMultipartUploadsResult"`

	// copied from s3.ListMultipartUploadsOutput, the Uploads is not converting to <Upload></Upload>
	Bucket             *string               `type:"string"`
	Delimiter          *string               `type:"string"`
	EncodingType       *string               `type:"string" enum:"EncodingType"`
	IsTruncated        *bool                 `type:"boolean"`
	KeyMarker          *string               `type:"string"`
	MaxUploads         *int64                `type:"integer"`
	NextKeyMarker      *string               `type:"string"`
	NextUploadIdMarker *string               `type:"string"`
	Prefix             *string               `type:"string"`
	UploadIdMarker     *string               `type:"string"`
	Upload             []*s3.MultipartUpload `locationName:"Upload" type:"list" flattened:"true"`
}

func (s3a *S3ApiServer) listMultipartUploads(input *s3.ListMultipartUploadsInput) (output *ListMultipartUploadsResult, code s3err.ErrorCode) {
	// https://docs.aws.amazon.com/AmazonS3/latest/API/API_ListMultipartUploads.html

	glog.V(2).Infof("listMultipartUploads input %v", input)

	output = &ListMultipartUploadsResult{
		Bucket:         input.Bucket,
		KeyMarker:      input.KeyMarker,
		UploadIdMarker: input.UploadIdMarker,
		MaxUploads:     input.MaxUploads,
		IsTruncated:    aws.Bool(false),
	}
	// Delimiter/EncodingType/Prefix are only echoed back when the client actually
	// requested them; AWS omits these elements entirely when unset, whereas a
	// pointer to an empty string would still render as an empty XML tag.
	if input.Delimiter != nil && *input.Delimiter != "" {
		output.Delimiter = input.Delimiter
	}
	if input.EncodingType != nil && *input.EncodingType != "" {
		output.EncodingType = input.EncodingType
	}
	if input.Prefix != nil && *input.Prefix != "" {
		output.Prefix = input.Prefix
	}

	entries, _, err := s3a.list(s3a.genUploadsFolder(*input.Bucket), "", *input.UploadIdMarker, false, math.MaxInt32)
	if err != nil {
		glog.Errorf("listMultipartUploads %s error: %v", *input.Bucket, err)
		return
	}

	uploadsCount := int64(0)
	for _, entry := range entries {
		if entry.Extended != nil {
			key := string(entry.Extended[s3_constants.ExtMultipartObjectKey])
			if *input.KeyMarker != "" && *input.KeyMarker != key {
				continue
			}
			if *input.Prefix != "" && !strings.HasPrefix(key, *input.Prefix) {
				continue
			}
			upload := &s3.MultipartUpload{
				Key:          objectKey(aws.String(key)),
				UploadId:     aws.String(entry.Name),
				StorageClass: aws.String("STANDARD"),
			}
			if ownerId := string(entry.Extended[s3_constants.ExtAmzOwnerKey]); ownerId != "" {
				displayName := s3a.iam.GetAccountNameById(ownerId)
				upload.Initiator = &s3.Initiator{ID: aws.String(ownerId), DisplayName: aws.String(displayName)}
				upload.Owner = &s3.Owner{ID: aws.String(ownerId)}
			}
			if entry.Attributes != nil {
				crtime := entry.Attributes.Crtime
				if crtime == 0 {
					crtime = entry.Attributes.Mtime
				}
				upload.Initiated = aws.Time(time.Unix(crtime, 0).UTC())
			}
			output.Upload = append(output.Upload, upload)
			uploadsCount += 1
		}
		if uploadsCount >= *input.MaxUploads {
			output.IsTruncated = aws.Bool(true)
			break
		}
	}

	// AWS always echoes NextKeyMarker/NextUploadIdMarker based on the last
	// returned Upload entry, regardless of whether the result is truncated.
	if lastUpload := len(output.Upload) - 1; lastUpload >= 0 {
		output.NextKeyMarker = output.Upload[lastUpload].Key
		output.NextUploadIdMarker = output.Upload[lastUpload].UploadId
	}

	return
}

type ListPartsResult struct {
	XMLName xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ ListPartsResult"`

	// copied from s3.ListPartsOutput, the Parts is not converting to <Part></Part>
	Bucket               *string       `type:"string"`
	Initiator            *s3.Initiator `type:"structure"`
	IsTruncated          *bool         `type:"boolean"`
	Key                  *string       `min:"1" type:"string"`
	MaxParts             *int64        `type:"integer"`
	NextPartNumberMarker *int64        `type:"integer"`
	Owner                *s3.Owner     `type:"structure"`
	PartNumberMarker     *int64        `type:"integer"`
	Part                 []*s3.Part    `locationName:"Part" type:"list" flattened:"true"`
	StorageClass         *string       `type:"string" enum:"StorageClass"`
	UploadId             *string       `type:"string"`
}

func isMultipartUploadEntry(entry *filer_pb.Entry) bool {
	return entry != nil && len(entry.Extended[s3_constants.ExtMultipartObjectKey]) > 0
}

func (s3a *S3ApiServer) listObjectParts(input *s3.ListPartsInput) (output *ListPartsResult, code s3err.ErrorCode) {
	// https://docs.aws.amazon.com/AmazonS3/latest/API/API_ListParts.html

	glog.V(2).Infof("listObjectParts input %v", input)

	// complete/abort delete the upload directory, but most stores list a missing
	// directory as empty instead of erroring, so listing alone cannot tell a
	// completed upload (NoSuchUpload on AWS) from an open upload with no parts
	// yet (200 with an empty list). Probe the upload record instead.
	pentry, err := s3a.getEntry(s3a.genUploadsFolder(*input.Bucket), *input.UploadId)
	if err != nil {
		if errors.Is(err, filer_pb.ErrNotFound) {
			return nil, s3err.ErrNoSuchUpload
		}
		glog.Errorf("listObjectParts %s %s error: %v", *input.Bucket, *input.UploadId, err)
		return nil, s3err.ErrInternalError
	}
	// only createMultipartUpload stamps the key; a directory a part write left behind is not an upload
	if !isMultipartUploadEntry(pentry) {
		return nil, s3err.ErrNoSuchUpload
	}

	output = &ListPartsResult{
		Bucket:           input.Bucket,
		Key:              objectKey(input.Key),
		UploadId:         input.UploadId,
		MaxParts:         input.MaxParts,         // the maximum number of parts to return.
		PartNumberMarker: input.PartNumberMarker, // the part number starts after this, exclusive
		StorageClass:     aws.String("STANDARD"),
	}
	if ownerId := string(pentry.Extended[s3_constants.ExtAmzOwnerKey]); ownerId != "" {
		displayName := s3a.iam.GetAccountNameById(ownerId)
		output.Initiator = &s3.Initiator{ID: aws.String(ownerId), DisplayName: aws.String(displayName)}
		output.Owner = &s3.Owner{ID: aws.String(ownerId)}
	}

	entries, isLast, err := s3a.list(s3a.genUploadsFolder(*input.Bucket)+"/"+*input.UploadId, "", fmt.Sprintf("%04d%s", *input.PartNumberMarker, multipartExt), false, uint32(*input.MaxParts))
	if err != nil {
		glog.Errorf("listObjectParts %s %s error: %v", *input.Bucket, *input.UploadId, err)
		return nil, s3err.ErrNoSuchUpload
	}

	// Note: The upload directory is sort of a marker of the existence of an multipart upload request.
	// So can not just delete empty upload folders.

	output.IsTruncated = aws.Bool(!isLast)

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name, multipartExt) && !entry.IsDirectory {
			partNumber, err := parsePartNumber(entry.Name)
			if err != nil {
				glog.Errorf("listObjectParts %s %s parse %s: %v", *input.Bucket, *input.UploadId, entry.Name, err)
				continue
			}
			partETag := filer.ETag(entry)
			part := &s3.Part{
				PartNumber:   aws.Int64(int64(partNumber)),
				LastModified: aws.Time(time.Unix(entry.Attributes.Mtime, 0).UTC()),
				Size:         aws.Int64(int64(filer.FileSize(entry))),
				ETag:         aws.String("\"" + partETag + "\""),
			}
			output.Part = append(output.Part, part)
			glog.V(3).Infof("listObjectParts: Added part %d, size=%d, etag=%s",
				partNumber, filer.FileSize(entry), partETag)
			// AWS always returns NextPartNumberMarker as the last part number in the
			// response (even when IsTruncated is false), not only when there are more
			// pages; match that so ListParts responses are byte-for-byte comparable.
			output.NextPartNumberMarker = aws.Int64(int64(partNumber))
		}
	}

	glog.V(2).Infof("listObjectParts: Returning %d parts for uploadId=%s", len(output.Part), *input.UploadId)
	return
}

// maxInt returns the maximum of two int values
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// MultipartEncryptionConfig holds pre-prepared encryption configuration to avoid error handling in callbacks
type MultipartEncryptionConfig struct {
	// SSE-KMS configuration
	IsSSEKMS          bool
	KMSKeyID          string
	BucketKeyEnabled  bool
	EncryptionContext string
	KMSBaseIVEncoded  string

	// SSE-S3 configuration
	IsSSES3          bool
	S3BaseIVEncoded  string
	S3KeyDataEncoded string
}

// prepareMultipartEncryptionConfig prepares encryption configuration with proper error handling
// This eliminates the need for criticalError variable in callback functions
// Updated to support bucket-default encryption (matches putToFiler behavior)
func (s3a *S3ApiServer) prepareMultipartEncryptionConfig(r *http.Request, bucket string, uploadIdString string) (*MultipartEncryptionConfig, error) {
	config := &MultipartEncryptionConfig{}

	// Check for explicit encryption headers first (priority over bucket defaults)
	hasExplicitSSEKMS := IsSSEKMSRequest(r)
	hasExplicitSSES3 := IsSSES3RequestInternal(r)

	// Prepare SSE-KMS configuration (explicit request headers)
	if hasExplicitSSEKMS {
		config.IsSSEKMS = true
		config.KMSKeyID = r.Header.Get(s3_constants.AmzServerSideEncryptionAwsKmsKeyId)
		config.BucketKeyEnabled = strings.ToLower(r.Header.Get(s3_constants.AmzServerSideEncryptionBucketKeyEnabled)) == "true"
		config.EncryptionContext = r.Header.Get(s3_constants.AmzServerSideEncryptionContext)

		// Generate and encode base IV with proper error handling
		baseIV := make([]byte, s3_constants.AESBlockSize)
		n, err := rand.Read(baseIV)
		if err != nil || n != len(baseIV) {
			return nil, fmt.Errorf("failed to generate secure IV for SSE-KMS multipart upload: %v (read %d/%d bytes)", err, n, len(baseIV))
		}
		config.KMSBaseIVEncoded = base64.StdEncoding.EncodeToString(baseIV)
		glog.V(4).Infof("Generated base IV %x for explicit SSE-KMS multipart upload %s", baseIV[:8], uploadIdString)
	}

	// Prepare SSE-S3 configuration (explicit request headers)
	if hasExplicitSSES3 {
		config.IsSSES3 = true

		// Generate and encode base IV with proper error handling
		baseIV := make([]byte, s3_constants.AESBlockSize)
		n, err := rand.Read(baseIV)
		if err != nil || n != len(baseIV) {
			return nil, fmt.Errorf("failed to generate secure IV for SSE-S3 multipart upload: %v (read %d/%d bytes)", err, n, len(baseIV))
		}
		config.S3BaseIVEncoded = base64.StdEncoding.EncodeToString(baseIV)
		glog.V(4).Infof("Generated base IV %x for explicit SSE-S3 multipart upload %s", baseIV[:8], uploadIdString)

		// Generate and serialize SSE-S3 key with proper error handling
		keyManager := GetSSES3KeyManager()
		sseS3Key, err := keyManager.GetOrCreateKey("")
		if err != nil {
			return nil, fmt.Errorf("failed to generate SSE-S3 key for multipart upload: %v", err)
		}

		keyData, serErr := SerializeSSES3Metadata(sseS3Key)
		if serErr != nil {
			return nil, fmt.Errorf("failed to serialize SSE-S3 metadata for multipart upload: %v", serErr)
		}

		config.S3KeyDataEncoded = base64.StdEncoding.EncodeToString(keyData)

		// Store key in manager for later retrieval
		keyManager.StoreKey(sseS3Key)
		glog.V(4).Infof("Stored SSE-S3 key %s for explicit multipart upload %s", sseS3Key.KeyID, uploadIdString)
	}

	// If no explicit encryption headers, check bucket-default encryption
	// This matches AWS S3 behavior and putToFiler() implementation
	if !hasExplicitSSEKMS && !hasExplicitSSES3 {
		encryptionConfig, err := s3a.GetBucketEncryptionConfig(bucket)
		if err != nil {
			// Check if this is just "no encryption configured" vs a real error
			if !errors.Is(err, ErrNoEncryptionConfig) {
				// Real error - propagate to prevent silent encryption bypass
				return nil, fmt.Errorf("failed to read bucket encryption config for multipart upload: %v", err)
			}
			// No default encryption configured, continue without encryption
		} else if encryptionConfig != nil && encryptionConfig.SseAlgorithm != "" {
			glog.V(3).Infof("prepareMultipartEncryptionConfig: applying bucket-default encryption %s for bucket %s, upload %s",
				encryptionConfig.SseAlgorithm, bucket, uploadIdString)

			switch encryptionConfig.SseAlgorithm {
			case EncryptionTypeKMS:
				// Apply SSE-KMS as bucket default
				config.IsSSEKMS = true
				config.KMSKeyID = encryptionConfig.KmsKeyId
				config.BucketKeyEnabled = encryptionConfig.BucketKeyEnabled
				// No encryption context for bucket defaults

				// Generate and encode base IV
				baseIV := make([]byte, s3_constants.AESBlockSize)
				n, readErr := rand.Read(baseIV)
				if readErr != nil || n != len(baseIV) {
					return nil, fmt.Errorf("failed to generate secure IV for bucket-default SSE-KMS multipart upload: %v (read %d/%d bytes)", readErr, n, len(baseIV))
				}
				config.KMSBaseIVEncoded = base64.StdEncoding.EncodeToString(baseIV)
				glog.V(4).Infof("Generated base IV %x for bucket-default SSE-KMS multipart upload %s", baseIV[:8], uploadIdString)

			case EncryptionTypeAES256:
				// Apply SSE-S3 (AES256) as bucket default
				config.IsSSES3 = true

				// Generate and encode base IV
				baseIV := make([]byte, s3_constants.AESBlockSize)
				n, readErr := rand.Read(baseIV)
				if readErr != nil || n != len(baseIV) {
					return nil, fmt.Errorf("failed to generate secure IV for bucket-default SSE-S3 multipart upload: %v (read %d/%d bytes)", readErr, n, len(baseIV))
				}
				config.S3BaseIVEncoded = base64.StdEncoding.EncodeToString(baseIV)
				glog.V(4).Infof("Generated base IV %x for bucket-default SSE-S3 multipart upload %s", baseIV[:8], uploadIdString)

				// Generate and serialize SSE-S3 key
				keyManager := GetSSES3KeyManager()
				sseS3Key, keyErr := keyManager.GetOrCreateKey("")
				if keyErr != nil {
					return nil, fmt.Errorf("failed to generate SSE-S3 key for bucket-default multipart upload: %v", keyErr)
				}

				keyData, serErr := SerializeSSES3Metadata(sseS3Key)
				if serErr != nil {
					return nil, fmt.Errorf("failed to serialize SSE-S3 metadata for bucket-default multipart upload: %v", serErr)
				}

				config.S3KeyDataEncoded = base64.StdEncoding.EncodeToString(keyData)

				// Store key in manager for later retrieval
				keyManager.StoreKey(sseS3Key)
				glog.V(4).Infof("Stored SSE-S3 key %s for bucket-default multipart upload %s", sseS3Key.KeyID, uploadIdString)

			default:
				glog.V(3).Infof("prepareMultipartEncryptionConfig: unsupported bucket-default encryption algorithm %s for bucket %s",
					encryptionConfig.SseAlgorithm, bucket)
			}
		}
	}

	return config, nil
}

// applyMultipartEncryptionConfig applies pre-prepared encryption configuration to filer entry
// This function is guaranteed not to fail since all error-prone operations were done during preparation
func (s3a *S3ApiServer) applyMultipartEncryptionConfig(entry *filer_pb.Entry, config *MultipartEncryptionConfig) {
	// Apply SSE-KMS configuration
	if config.IsSSEKMS {
		entry.Extended[s3_constants.SeaweedFSSSEKMSKeyID] = []byte(config.KMSKeyID)
		if config.BucketKeyEnabled {
			entry.Extended[s3_constants.SeaweedFSSSEKMSBucketKeyEnabled] = []byte("true")
		}
		if config.EncryptionContext != "" {
			entry.Extended[s3_constants.SeaweedFSSSEKMSEncryptionContext] = []byte(config.EncryptionContext)
		}
		entry.Extended[s3_constants.SeaweedFSSSEKMSBaseIV] = []byte(config.KMSBaseIVEncoded)
		glog.V(3).Infof("applyMultipartEncryptionConfig: applied SSE-KMS settings with keyID %s", config.KMSKeyID)
	}

	// Apply SSE-S3 configuration
	if config.IsSSES3 {
		entry.Extended[s3_constants.SeaweedFSSSES3Encryption] = []byte(s3_constants.SSEAlgorithmAES256)
		entry.Extended[s3_constants.SeaweedFSSSES3BaseIV] = []byte(config.S3BaseIVEncoded)
		entry.Extended[s3_constants.SeaweedFSSSES3KeyData] = []byte(config.S3KeyDataEncoded)
		glog.V(3).Infof("applyMultipartEncryptionConfig: applied SSE-S3 settings")
	}
}

func decodePartChecksum(partNumber int, entries []*filer_pb.Entry, checksumHeaderName string) ([]byte, *filer_pb.Entry, error) {
	if len(entries) == 0 {
		return nil, nil, fmt.Errorf("part %d not found", partNumber)
	}
	entry := entries[0]
	if entry.Extended == nil {
		return nil, nil, fmt.Errorf("part %d missing checksum: upload initiated with %s but part was uploaded without a checksum", partNumber, checksumHeaderName)
	}
	partAlgo, ok := entry.Extended[s3_constants.ExtChecksumAlgorithm]
	if !ok || len(partAlgo) == 0 {
		return nil, nil, fmt.Errorf("part %d missing checksum: upload initiated with %s but part was uploaded without a checksum", partNumber, checksumHeaderName)
	}
	if !strings.EqualFold(string(partAlgo), checksumHeaderName) {
		return nil, nil, fmt.Errorf("part %d checksum algorithm mismatch: upload expects %s but part has %s", partNumber, checksumHeaderName, string(partAlgo))
	}
	partChecksumB64, ok := entry.Extended[s3_constants.ExtChecksumValue]
	if !ok || len(partChecksumB64) == 0 {
		return nil, nil, fmt.Errorf("part %d missing checksum value: upload initiated with %s but part has no checksum value", partNumber, checksumHeaderName)
	}
	raw, err := base64.StdEncoding.DecodeString(string(partChecksumB64))
	if err != nil {
		return nil, nil, fmt.Errorf("part %d has invalid checksum encoding: %w", partNumber, err)
	}
	return raw, entry, nil
}

func computeCompositeChecksum(checksumHeaderName string, partEntries map[int][]*filer_pb.Entry, completedPartNumbers []int) (string, error) {
	algo := checksumAlgorithmFromHeaderName(checksumHeaderName)
	if algo == ChecksumAlgorithmNone {
		return "", fmt.Errorf("unknown checksum algorithm for header %q", checksumHeaderName)
	}

	var combined []byte
	for _, partNumber := range completedPartNumbers {
		raw, _, err := decodePartChecksum(partNumber, partEntries[partNumber], checksumHeaderName)
		if err != nil {
			return "", err
		}
		combined = append(combined, raw...)
	}

	h := getCheckSumWriter(algo)
	if h == nil {
		return "", fmt.Errorf("failed to create hash writer for %s", checksumHeaderName)
	}
	h.Write(combined)
	compositeRaw := h.Sum(nil)
	return fmt.Sprintf("%s-%d", base64.StdEncoding.EncodeToString(compositeRaw), len(completedPartNumbers)), nil
}

func computeFullObjectChecksum(checksumHeaderName string, partEntries map[int][]*filer_pb.Entry, completedPartNumbers []int) (string, error) {
	algo := checksumAlgorithmFromHeaderName(checksumHeaderName)
	params, ok := crcCombineParams[algo]
	if !ok {
		return "", fmt.Errorf("full object checksum not supported for %s", checksumHeaderName)
	}

	checksumBytes := int(params.width / 8)

	var combined uint64
	for i, partNumber := range completedPartNumbers {
		raw, entry, err := decodePartChecksum(partNumber, partEntries[partNumber], checksumHeaderName)
		if err != nil {
			return "", err
		}
		if len(raw) != checksumBytes {
			return "", fmt.Errorf("part %d checksum has unexpected length %d for %s", partNumber, len(raw), checksumHeaderName)
		}

		crc := util.BytesToUint64(raw)

		if i == 0 {
			combined = crc
		} else {
			partLen := filer.FileSize(entry)
			combined = combineCRC(combined, crc, partLen, params)
		}
	}

	out := make([]byte, checksumBytes)
	for i := 0; i < checksumBytes; i++ {
		out[checksumBytes-1-i] = byte(combined >> (i * 8))
	}

	return base64.StdEncoding.EncodeToString(out), nil
}

func parseChecksumAlgorithmName(headerOrAlgo string) string {
	cleaned := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(headerOrAlgo)), "x-amz-checksum-")
	switch strings.ToUpper(cleaned) {
	case "CRC32":
		return "CRC32"
	case "CRC32C":
		return "CRC32C"
	case "CRC64NVME":
		return "CRC64NVME"
	case "SHA1":
		return "SHA1"
	case "SHA256":
		return "SHA256"
	}
	return ""
}

func checksumAlgorithmFromHeaderName(headerName string) ChecksumAlgorithm {
	for _, entry := range checksumHeaders {
		if strings.EqualFold(entry.name, headerName) {
			return entry.alg
		}
	}
	return ChecksumAlgorithmNone
}

type checksumTypeSupport struct {
	composite  bool
	fullObject bool
}

var checksumTypeSupportByAlgo = map[ChecksumAlgorithm]checksumTypeSupport{
	ChecksumAlgorithmCRC32:     {composite: true, fullObject: true},
	ChecksumAlgorithmCRC32C:    {composite: true, fullObject: true},
	ChecksumAlgorithmCRC64NVMe: {fullObject: true},
	ChecksumAlgorithmSHA1:      {composite: true},
	ChecksumAlgorithmSHA256:    {composite: true},
}

func resolveMultipartChecksumType(algo ChecksumAlgorithm, requested string) (string, error) {
	support, ok := checksumTypeSupportByAlgo[algo]
	if !ok {
		return "", fmt.Errorf("unsupported checksum algorithm %v", algo)
	}

	switch strings.ToUpper(strings.TrimSpace(requested)) {
	case "":
		if support.composite {
			return s3_constants.ChecksumTypeComposite, nil
		}
		return s3_constants.ChecksumTypeFullObject, nil
	case s3_constants.ChecksumTypeComposite:
		if !support.composite {
			return "", fmt.Errorf("checksum algorithm %v does not support %s checksums", algo, s3_constants.ChecksumTypeComposite)
		}
		return s3_constants.ChecksumTypeComposite, nil
	case s3_constants.ChecksumTypeFullObject:
		if !support.fullObject {
			return "", fmt.Errorf("checksum algorithm %v does not support %s checksums", algo, s3_constants.ChecksumTypeFullObject)
		}
		return s3_constants.ChecksumTypeFullObject, nil
	default:
		return "", fmt.Errorf("invalid checksum type %q", requested)
	}
}
