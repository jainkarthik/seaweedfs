package s3api

import (
	"fmt"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/seaweedfs/seaweedfs/weed/s3api/s3err"
	stats_collect "github.com/seaweedfs/seaweedfs/weed/stats"
)

func TestNextAssignedFid(t *testing.T) {
	base := "3,01637037d6"

	if got := nextAssignedFid(base, 0); got != base {
		t.Fatalf("index 0: got %q, want %q", got, base)
	}
	if got := nextAssignedFid(base, 1); got != base+"_1" {
		t.Fatalf("index 1: got %q, want %q", got, base+"_1")
	}
	if got := nextAssignedFid(base, 17); got != base+"_17" {
		t.Fatalf("index 17: got %q, want %q", got, base+"_17")
	}
}

func TestRecordPutToFilerResultMetrics(t *testing.T) {
	bucket := fmt.Sprintf("put-to-filer-metrics-%d", time.Now().UnixNano())

	successBefore := metricCounterValue(t, "SeaweedFS_s3_put_to_filer_result_total", map[string]string{
		"bucket": bucket,
		"result": "success",
	})
	errorBefore := metricCounterValue(t, "SeaweedFS_s3_put_to_filer_result_total", map[string]string{
		"bucket": bucket,
		"result": "error",
	})
	totalCountBefore := metricHistogramSampleCount(t, "SeaweedFS_s3_put_to_filer_stage_seconds", map[string]string{
		"bucket": bucket,
		"stage":  "total",
	})

	recordPutToFilerResultMetrics(bucket, time.Now().Add(-20*time.Millisecond), s3err.ErrNone)
	recordPutToFilerResultMetrics(bucket, time.Now().Add(-15*time.Millisecond), s3err.ErrInternalError)

	successAfter := metricCounterValue(t, "SeaweedFS_s3_put_to_filer_result_total", map[string]string{
		"bucket": bucket,
		"result": "success",
	})
	errorAfter := metricCounterValue(t, "SeaweedFS_s3_put_to_filer_result_total", map[string]string{
		"bucket": bucket,
		"result": "error",
	})
	totalCountAfter := metricHistogramSampleCount(t, "SeaweedFS_s3_put_to_filer_stage_seconds", map[string]string{
		"bucket": bucket,
		"stage":  "total",
	})

	if got := successAfter - successBefore; got != 1 {
		t.Fatalf("success counter delta: got %v, want 1", got)
	}
	if got := errorAfter - errorBefore; got != 1 {
		t.Fatalf("error counter delta: got %v, want 1", got)
	}
	if got := totalCountAfter - totalCountBefore; got != 2 {
		t.Fatalf("total stage histogram sample count delta: got %d, want 2", got)
	}
}

func metricCounterValue(t *testing.T, metricName string, labels map[string]string) float64 {
	t.Helper()
	for _, mf := range gatherMetrics(t) {
		if mf.GetName() != metricName {
			continue
		}
		for _, m := range mf.GetMetric() {
			if !hasLabels(m.GetLabel(), labels) {
				continue
			}
			if m.GetCounter() == nil {
				t.Fatalf("metric %s with labels %v is not a counter", metricName, labels)
			}
			return m.GetCounter().GetValue()
		}
	}
	return 0
}

func metricHistogramSampleCount(t *testing.T, metricName string, labels map[string]string) uint64 {
	t.Helper()
	for _, mf := range gatherMetrics(t) {
		if mf.GetName() != metricName {
			continue
		}
		for _, m := range mf.GetMetric() {
			if !hasLabels(m.GetLabel(), labels) {
				continue
			}
			if m.GetHistogram() == nil {
				t.Fatalf("metric %s with labels %v is not a histogram", metricName, labels)
			}
			return m.GetHistogram().GetSampleCount()
		}
	}
	return 0
}

func gatherMetrics(t *testing.T) []*dto.MetricFamily {
	t.Helper()
	mfs, err := stats_collect.Gather.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	return mfs
}

func hasLabels(current []*dto.LabelPair, expected map[string]string) bool {
	for key, want := range expected {
		matched := false
		for _, label := range current {
			if label.GetName() == key && label.GetValue() == want {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

