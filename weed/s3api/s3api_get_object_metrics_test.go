package s3api

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/seaweedfs/seaweedfs/weed/filer"
)

func TestGetDownloadChunkPrefetch(t *testing.T) {
	server := &S3ApiServer{}
	if got, want := server.getDownloadChunkPrefetch(), filer.DefaultPrefetchCount; got != want {
		t.Fatalf("nil option: got %d, want %d", got, want)
	}

	server.option = &S3ApiServerOption{}
	if got, want := server.getDownloadChunkPrefetch(), filer.DefaultPrefetchCount; got != want {
		t.Fatalf("default option: got %d, want %d", got, want)
	}

	server.option.DownloadChunkPrefetch = 24
	if got, want := server.getDownloadChunkPrefetch(), 24; got != want {
		t.Fatalf("configured option: got %d, want %d", got, want)
	}
}

func TestGetDownloadCopyBufferBytes(t *testing.T) {
	server := &S3ApiServer{}
	if got, want := server.getDownloadCopyBufferBytes(), int64(defaultDownloadCopyBufferKB*1024); got != want {
		t.Fatalf("nil option: got %d, want %d", got, want)
	}

	server.option = &S3ApiServerOption{}
	if got, want := server.getDownloadCopyBufferBytes(), int64(defaultDownloadCopyBufferKB*1024); got != want {
		t.Fatalf("default option: got %d, want %d", got, want)
	}

	server.option.DownloadCopyBufferKB = 1024
	if got, want := server.getDownloadCopyBufferBytes(), int64(1024*1024); got != want {
		t.Fatalf("configured option: got %d, want %d", got, want)
	}
}

func TestRecordGetObjectResultMetrics(t *testing.T) {
	bucket := fmt.Sprintf("get-object-metrics-%d", time.Now().UnixNano())

	successBefore := metricCounterValue(t, "SeaweedFS_s3_get_object_result_total", map[string]string{
		"bucket": bucket,
		"result": "success",
	})
	errorBefore := metricCounterValue(t, "SeaweedFS_s3_get_object_result_total", map[string]string{
		"bucket": bucket,
		"result": "error",
	})
	canceledBefore := metricCounterValue(t, "SeaweedFS_s3_get_object_result_total", map[string]string{
		"bucket": bucket,
		"result": "canceled",
	})
	totalBefore := metricHistogramSampleCount(t, "SeaweedFS_s3_get_object_stage_seconds", map[string]string{
		"bucket": bucket,
		"stage":  "total",
	})
	downloadedBefore := metricHistogramSampleCount(t, "SeaweedFS_s3_get_object_downloaded_bytes", map[string]string{
		"bucket": bucket,
	})

	recordGetObjectResultMetrics(bucket, time.Now().Add(-25*time.Millisecond), nil, 64*1024)
	recordGetObjectResultMetrics(bucket, time.Now().Add(-25*time.Millisecond), context.Canceled, 0)
	recordGetObjectResultMetrics(bucket, time.Now().Add(-25*time.Millisecond), fmt.Errorf("stream failed"), 0)

	successAfter := metricCounterValue(t, "SeaweedFS_s3_get_object_result_total", map[string]string{
		"bucket": bucket,
		"result": "success",
	})
	errorAfter := metricCounterValue(t, "SeaweedFS_s3_get_object_result_total", map[string]string{
		"bucket": bucket,
		"result": "error",
	})
	canceledAfter := metricCounterValue(t, "SeaweedFS_s3_get_object_result_total", map[string]string{
		"bucket": bucket,
		"result": "canceled",
	})
	totalAfter := metricHistogramSampleCount(t, "SeaweedFS_s3_get_object_stage_seconds", map[string]string{
		"bucket": bucket,
		"stage":  "total",
	})
	downloadedAfter := metricHistogramSampleCount(t, "SeaweedFS_s3_get_object_downloaded_bytes", map[string]string{
		"bucket": bucket,
	})

	if got := successAfter - successBefore; got != 1 {
		t.Fatalf("success counter delta: got %v, want 1", got)
	}
	if got := errorAfter - errorBefore; got != 1 {
		t.Fatalf("error counter delta: got %v, want 1", got)
	}
	if got := canceledAfter - canceledBefore; got != 1 {
		t.Fatalf("canceled counter delta: got %v, want 1", got)
	}
	if got := totalAfter - totalBefore; got != 3 {
		t.Fatalf("total stage histogram delta: got %d, want 3", got)
	}
	if got := downloadedAfter - downloadedBefore; got != 1 {
		t.Fatalf("downloaded bytes histogram delta: got %d, want 1", got)
	}
}
