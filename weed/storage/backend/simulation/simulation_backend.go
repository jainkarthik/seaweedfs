package simulation

import (
	"fmt"
	"io"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

type SimulationConfig struct {
	Latency       time.Duration // Simulated disk latency
	ErrorRate     float64       // 0.0 to 1.0, probability of write errors
	Bandwidth     int64         // bytes per second
	DiskSpace     int64         // total available disk space
	EnableFailure bool          // enable failure injection
	FailurePoint  int64         // fail after this many bytes written
}

type SimulationMetrics struct {
	BytesWritten     int64
	BytesRead        int64
	WriteOperations  int64
	ReadOperations   int64
	ErrorsInjected   int64
	SyncOperations   int64
	TruncateOps      int64
	AverageLatency   time.Duration
	TotalWriteTimeNs int64
	TotalReadTimeNs  int64
	mu               sync.RWMutex
}

type SimulatedBackendStorageFile struct {
	config      SimulationConfig
	metrics     *SimulationMetrics
	data        []byte
	fileSize    int64
	modTime     time.Time
	name        string
	closed      int32
	mu          sync.RWMutex
	bytesFailed int64
}

func NewSimulatedBackendStorageFile(name string, config SimulationConfig) *SimulatedBackendStorageFile {
	return &SimulatedBackendStorageFile{
		config:   config,
		metrics:  &SimulationMetrics{},
		data:     make([]byte, 0),
		fileSize: 0,
		modTime:  time.Now(),
		name:     name,
	}
}

// ReadAt implements io.ReaderAt
func (s *SimulatedBackendStorageFile) ReadAt(p []byte, off int64) (n int, err error) {
	if atomic.LoadInt32(&s.closed) != 0 {
		return 0, fmt.Errorf("file is closed")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	start := time.Now()
	defer func() {
		atomic.AddInt64(&s.metrics.ReadOperations, 1)
		atomic.AddInt64(&s.metrics.BytesRead, int64(n))
		atomic.AddInt64(&s.metrics.TotalReadTimeNs, time.Since(start).Nanoseconds())
		s.updateAverageLatency()
	}()

	// Simulate read latency
	if s.config.Latency > 0 {
		time.Sleep(s.simulateLatency())
	}

	// Check bounds
	if off >= s.fileSize {
		return 0, io.EOF
	}

	// Calculate how much we can read
	maxRead := s.fileSize - off
	if int64(len(p)) > maxRead {
		n = int(maxRead)
	} else {
		n = len(p)
	}

	// Copy data from our in-memory storage
	copy(p, s.data[off:off+int64(n)])

	return n, nil
}

// WriteAt implements io.WriterAt
func (s *SimulatedBackendStorageFile) WriteAt(p []byte, off int64) (n int, err error) {
	if atomic.LoadInt32(&s.closed) != 0 {
		return 0, fmt.Errorf("file is closed")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	start := time.Now()
	defer func() {
		atomic.AddInt64(&s.metrics.WriteOperations, 1)
		atomic.AddInt64(&s.metrics.BytesWritten, int64(n))
		atomic.AddInt64(&s.metrics.TotalWriteTimeNs, time.Since(start).Nanoseconds())
		s.updateAverageLatency()
	}()

	// Check disk space limit
	if s.config.DiskSpace > 0 && off+int64(len(p)) > s.config.DiskSpace {
		return 0, fmt.Errorf("disk space exceeded: available %d, required %d", s.config.DiskSpace, off+int64(len(p)))
	}

	// Simulate failure injection
	if s.config.EnableFailure {
		s.bytesFailed += int64(len(p))
		if s.config.FailurePoint > 0 && s.bytesFailed >= s.config.FailurePoint {
			atomic.AddInt64(&s.metrics.ErrorsInjected, 1)
			return 0, fmt.Errorf("simulated disk failure at byte %d", s.bytesFailed)
		}
	}

	// Simulate random errors based on error rate
	if s.config.ErrorRate > 0 && rand.Float64() < s.config.ErrorRate {
		atomic.AddInt64(&s.metrics.ErrorsInjected, 1)
		return 0, fmt.Errorf("simulated write error")
	}

	// Simulate bandwidth limitation
	if s.config.Bandwidth > 0 {
		writeTime := time.Duration(int64(len(p)) * 1e9 / s.config.Bandwidth)
		time.Sleep(writeTime)
	}

	// Simulate disk latency
	if s.config.Latency > 0 {
		time.Sleep(s.simulateLatency())
	}

	// Expand our data slice if necessary
	if off+int64(len(p)) > int64(len(s.data)) {
		newData := make([]byte, off+int64(len(p)))
		copy(newData, s.data)
		s.data = newData
	}

	// Write the data
	copy(s.data[off:], p)
	if off+int64(len(p)) > s.fileSize {
		s.fileSize = off + int64(len(p))
	}
	s.modTime = time.Now()

	return len(p), nil
}

// Truncate implements backend.BackendStorageFile
func (s *SimulatedBackendStorageFile) Truncate(off int64) error {
	if atomic.LoadInt32(&s.closed) != 0 {
		return fmt.Errorf("file is closed")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	atomic.AddInt64(&s.metrics.TruncateOps, 1)

	// Simulate latency
	if s.config.Latency > 0 {
		time.Sleep(s.simulateLatency())
	}

	if off > int64(len(s.data)) {
		// Expand with zeros
		newData := make([]byte, off)
		copy(newData, s.data)
		s.data = newData
	} else {
		// Truncate
		s.data = s.data[:off]
	}
	s.fileSize = off
	s.modTime = time.Now()

	return nil
}

// Close implements io.Closer
func (s *SimulatedBackendStorageFile) Close() error {
	if !atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		return fmt.Errorf("file already closed")
	}
	return nil
}

// GetStat implements backend.BackendStorageFile
func (s *SimulatedBackendStorageFile) GetStat() (datSize int64, modTime time.Time, err error) {
	if atomic.LoadInt32(&s.closed) != 0 {
		return 0, time.Time{}, fmt.Errorf("file is closed")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.fileSize, s.modTime, nil
}

// Name implements backend.BackendStorageFile
func (s *SimulatedBackendStorageFile) Name() string {
	return s.name
}

// Sync implements backend.BackendStorageFile
func (s *SimulatedBackendStorageFile) Sync() error {
	if atomic.LoadInt32(&s.closed) != 0 {
		return fmt.Errorf("file is closed")
	}

	atomic.AddInt64(&s.metrics.SyncOperations, 1)

	// Simulate sync latency (typically longer than regular operations)
	if s.config.Latency > 0 {
		syncLatency := s.config.Latency * 2 // Sync typically takes longer
		time.Sleep(syncLatency)
	}

	return nil
}

// Helper methods

func (s *SimulatedBackendStorageFile) simulateLatency() time.Duration {
	// Add some randomness to latency (±50%)
	baseLatency := int64(s.config.Latency)
	variation := baseLatency / 2
	actualLatency := baseLatency + rand.Int63n(variation*2) - variation
	if actualLatency < 0 {
		actualLatency = 0
	}
	return time.Duration(actualLatency)
}

func (s *SimulatedBackendStorageFile) updateAverageLatency() {
	totalOps := atomic.LoadInt64(&s.metrics.WriteOperations) + atomic.LoadInt64(&s.metrics.ReadOperations)
	if totalOps > 0 {
		totalTime := atomic.LoadInt64(&s.metrics.TotalWriteTimeNs) + atomic.LoadInt64(&s.metrics.TotalReadTimeNs)
		s.metrics.mu.Lock()
		s.metrics.AverageLatency = time.Duration(totalTime / totalOps)
		s.metrics.mu.Unlock()
	}
}

// GetMetrics returns current metrics
func (s *SimulatedBackendStorageFile) GetMetrics() SimulationMetrics {
	s.metrics.mu.RLock()
	defer s.metrics.mu.RUnlock()
	return *s.metrics
}

// UpdateConfig updates the simulation configuration at runtime
func (s *SimulatedBackendStorageFile) UpdateConfig(config SimulationConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = config
}

// ResetMetrics resets all counters
func (s *SimulatedBackendStorageFile) ResetMetrics() {
	atomic.StoreInt64(&s.metrics.BytesWritten, 0)
	atomic.StoreInt64(&s.metrics.BytesRead, 0)
	atomic.StoreInt64(&s.metrics.WriteOperations, 0)
	atomic.StoreInt64(&s.metrics.ReadOperations, 0)
	atomic.StoreInt64(&s.metrics.ErrorsInjected, 0)
	atomic.StoreInt64(&s.metrics.SyncOperations, 0)
	atomic.StoreInt64(&s.metrics.TruncateOps, 0)
	atomic.StoreInt64(&s.metrics.TotalWriteTimeNs, 0)
	atomic.StoreInt64(&s.metrics.TotalReadTimeNs, 0)
	s.metrics.mu.Lock()
	s.metrics.AverageLatency = 0
	s.metrics.mu.Unlock()
}
