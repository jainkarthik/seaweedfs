package s3api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchVolumeServerWithRetrySuccessAfterRetry(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&hits, 1)
		if call == 1 {
			http.Error(w, "temporary failure", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	operation := "retry-success-" + time.Now().Format("150405.000000000")
	retryBefore := metricCounterValue(t, "SeaweedFS_s3_reliability_event_total", map[string]string{
		"operation": operation,
		"event":     "retry_attempt",
	})
	successBefore := metricCounterValue(t, "SeaweedFS_s3_reliability_event_total", map[string]string{
		"operation": operation,
		"event":     "retry_success",
	})

	resp, err := fetchVolumeServerWithRetry(context.Background(), operation, func() (*http.Request, error) {
		return http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	}, func(statusCode int) bool {
		return statusCode == http.StatusPartialContent
	})
	if err != nil {
		t.Fatalf("fetch with retry failed: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	retryAfter := metricCounterValue(t, "SeaweedFS_s3_reliability_event_total", map[string]string{
		"operation": operation,
		"event":     "retry_attempt",
	})
	successAfter := metricCounterValue(t, "SeaweedFS_s3_reliability_event_total", map[string]string{
		"operation": operation,
		"event":     "retry_success",
	})

	if got := retryAfter - retryBefore; got != 1 {
		t.Fatalf("retry_attempt delta: got %v, want 1", got)
	}
	if got := successAfter - successBefore; got != 1 {
		t.Fatalf("retry_success delta: got %v, want 1", got)
	}
}

func TestFetchVolumeServerWithRetryExhausted(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "still failing", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	operation := "retry-exhausted-" + time.Now().Format("150405.000000000")
	exhaustedBefore := metricCounterValue(t, "SeaweedFS_s3_reliability_event_total", map[string]string{
		"operation": operation,
		"event":     "retry_exhausted",
	})

	_, err := fetchVolumeServerWithRetry(context.Background(), operation, func() (*http.Request, error) {
		return http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	}, func(statusCode int) bool {
		return statusCode == http.StatusOK
	})
	if err == nil {
		t.Fatalf("expected retry exhaustion error")
	}

	exhaustedAfter := metricCounterValue(t, "SeaweedFS_s3_reliability_event_total", map[string]string{
		"operation": operation,
		"event":     "retry_exhausted",
	})
	if got := exhaustedAfter - exhaustedBefore; got != 1 {
		t.Fatalf("retry_exhausted delta: got %v, want 1", got)
	}
	if got := atomic.LoadInt32(&hits); got != volumeReadMaxAttempts {
		t.Fatalf("attempt count: got %d, want %d", got, volumeReadMaxAttempts)
	}
}

