package s3err

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

func TestWriteErrorResponseDrainsRequestBodyBeforeWriting(t *testing.T) {
	body := &trackingReadCloser{data: bytes.Repeat([]byte("a"), 1024)}
	req := httptest.NewRequest(http.MethodPut, "/bucket/object", nil)
	req.Body = body
	req.ContentLength = int64(body.remaining())
	req = mux.SetURLVars(req, map[string]string{
		"bucket": "bucket",
		"object": "object",
	})

	rr := &drainCheckingResponseWriter{
		header: make(http.Header),
		body:   body,
	}

	WriteErrorResponse(rr, req, ErrInternalError)

	assert.Empty(t, rr.writeHeaderErr)
	assert.Equal(t, 0, body.remaining())
	assert.Equal(t, http.StatusInternalServerError, rr.status)
	assert.Len(t, rr.readDeadlines, 2)
	assert.False(t, rr.readDeadlines[0].IsZero())
	assert.True(t, rr.readDeadlines[1].IsZero())
}

type trackingReadCloser struct {
	data []byte
}

func (t *trackingReadCloser) Read(p []byte) (int, error) {
	if len(t.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, t.data)
	t.data = t.data[n:]
	return n, nil
}

func (t *trackingReadCloser) Close() error {
	return nil
}

func (t *trackingReadCloser) remaining() int {
	return len(t.data)
}

type drainCheckingResponseWriter struct {
	header         http.Header
	body           *trackingReadCloser
	status         int
	writeHeaderErr string
	readDeadlines  []time.Time
}

func (d *drainCheckingResponseWriter) Header() http.Header {
	return d.header
}

func (d *drainCheckingResponseWriter) Write(p []byte) (int, error) {
	if d.status == 0 {
		d.WriteHeader(http.StatusOK)
	}
	return len(p), nil
}

func (d *drainCheckingResponseWriter) WriteHeader(status int) {
	d.status = status
	if d.body.remaining() != 0 {
		d.writeHeaderErr = "request body was not drained before WriteHeader"
	}
}

func (d *drainCheckingResponseWriter) Flush() {}

func (d *drainCheckingResponseWriter) SetReadDeadline(t time.Time) error {
	d.readDeadlines = append(d.readDeadlines, t)
	return nil
}
