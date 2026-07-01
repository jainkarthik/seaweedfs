package s3api

import (
	"fmt"
	"testing"
	"time"

	"github.com/seaweedfs/seaweedfs/weed/s3api/s3err"
)

func TestRecordMultipartResultMetrics(t *testing.T) {
	bucket := fmt.Sprintf("multipart-metadata-%d", time.Now().UnixNano())
	operation := "multipart_complete"

	successBefore := metricCounterValue(t, "SeaweedFS_s3_metadata_result_total", map[string]string{
		"operation": operation,
		"bucket":    bucket,
		"result":    "success",
	})
	errorBefore := metricCounterValue(t, "SeaweedFS_s3_metadata_result_total", map[string]string{
		"operation": operation,
		"bucket":    bucket,
		"result":    "error",
	})
	totalBefore := metricHistogramSampleCount(t, "SeaweedFS_s3_metadata_stage_seconds", map[string]string{
		"operation": operation,
		"bucket":    bucket,
		"stage":     "total",
	})

	recordMultipartResult(operation, bucket, time.Now().Add(-10*time.Millisecond), s3err.ErrNone)
	recordMultipartResult(operation, bucket, time.Now().Add(-10*time.Millisecond), s3err.ErrInternalError)

	successAfter := metricCounterValue(t, "SeaweedFS_s3_metadata_result_total", map[string]string{
		"operation": operation,
		"bucket":    bucket,
		"result":    "success",
	})
	errorAfter := metricCounterValue(t, "SeaweedFS_s3_metadata_result_total", map[string]string{
		"operation": operation,
		"bucket":    bucket,
		"result":    "error",
	})
	totalAfter := metricHistogramSampleCount(t, "SeaweedFS_s3_metadata_stage_seconds", map[string]string{
		"operation": operation,
		"bucket":    bucket,
		"stage":     "total",
	})

	if got := successAfter - successBefore; got != 1 {
		t.Fatalf("success delta: got %v, want 1", got)
	}
	if got := errorAfter - errorBefore; got != 1 {
		t.Fatalf("error delta: got %v, want 1", got)
	}
	if got := totalAfter - totalBefore; got != 2 {
		t.Fatalf("total stage delta: got %d, want 2", got)
	}
}
