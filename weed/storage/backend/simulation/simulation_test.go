package simulation

import (
	"testing"
	"time"

	"github.com/seaweedfs/seaweedfs/weed/pb/volume_server_pb"
)

type testProperties struct {
	m map[string]string
}

func (tp *testProperties) GetString(key string) string {
	if v, found := tp.m[key]; found {
		return v
	}
	return ""
}

func TestSimulatedBackendStorageFile(t *testing.T) {
	config := SimulationConfig{
		Latency:       1 * time.Millisecond,
		ErrorRate:     0.0,
		Bandwidth:     1024 * 1024,       // 1MB/s
		DiskSpace:     100 * 1024 * 1024, // 100MB
		EnableFailure: false,
		FailurePoint:  0,
	}

	file := NewSimulatedBackendStorageFile("test.dat", config)

	// Test WriteAt and ReadAt
	testData := []byte("Hello, World!")
	n, err := file.WriteAt(testData, 0)
	if err != nil {
		t.Fatalf("WriteAt failed: %v", err)
	}
	if n != len(testData) {
		t.Fatalf("WriteAt wrote %d bytes, expected %d", n, len(testData))
	}

	readData := make([]byte, len(testData))
	n, err = file.ReadAt(readData, 0)
	if err != nil {
		t.Fatalf("ReadAt failed: %v", err)
	}
	if n != len(testData) {
		t.Fatalf("ReadAt read %d bytes, expected %d", n, len(testData))
	}

	if string(readData) != string(testData) {
		t.Fatalf("Read data %s doesn't match written data %s", string(readData), string(testData))
	}

	// Test GetStat
	size, modTime, err := file.GetStat()
	if err != nil {
		t.Fatalf("GetStat failed: %v", err)
	}
	if size != int64(len(testData)) {
		t.Fatalf("GetStat size %d doesn't match expected %d", size, len(testData))
	}
	if modTime.IsZero() {
		t.Fatal("GetStat modTime is zero")
	}

	// Test Truncate
	err = file.Truncate(5)
	if err != nil {
		t.Fatalf("Truncate failed: %v", err)
	}

	size, _, err = file.GetStat()
	if err != nil {
		t.Fatalf("GetStat after truncate failed: %v", err)
	}
	if size != 5 {
		t.Fatalf("Size after truncate %d doesn't match expected %d", size, 5)
	}

	// Test Sync
	err = file.Sync()
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Test Close
	err = file.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Test operations after close should fail
	_, err = file.WriteAt([]byte("test"), 0)
	if err == nil {
		t.Fatal("WriteAt after close should fail")
	}
}

func TestSimulatedBackendStorageFileMetrics(t *testing.T) {
	config := SimulationConfig{
		Latency:       0,
		ErrorRate:     0.0,
		Bandwidth:     0,
		DiskSpace:     0,
		EnableFailure: false,
		FailurePoint:  0,
	}

	file := NewSimulatedBackendStorageFile("test.dat", config)
	defer file.Close()

	// Perform some operations
	testData := make([]byte, 1024)
	for i := 0; i < 10; i++ {
		file.WriteAt(testData, int64(i*len(testData)))
		file.ReadAt(testData, int64(i*len(testData)))
	}

	file.Sync()
	file.Truncate(10240)

	// Check metrics
	metrics := file.GetMetrics()
	if metrics.WriteOperations != 10 {
		t.Fatalf("WriteOperations %d doesn't match expected 10", metrics.WriteOperations)
	}
	if metrics.ReadOperations != 10 {
		t.Fatalf("ReadOperations %d doesn't match expected 10", metrics.ReadOperations)
	}
	if metrics.BytesWritten != 10240 {
		t.Fatalf("BytesWritten %d doesn't match expected 10240", metrics.BytesWritten)
	}
	if metrics.BytesRead != 10240 {
		t.Fatalf("BytesRead %d doesn't match expected 10240", metrics.BytesRead)
	}
	if metrics.SyncOperations != 1 {
		t.Fatalf("SyncOperations %d doesn't match expected 1", metrics.SyncOperations)
	}
	if metrics.TruncateOps != 1 {
		t.Fatalf("TruncateOps %d doesn't match expected 1", metrics.TruncateOps)
	}

	// Test ResetMetrics
	file.ResetMetrics()
	metrics = file.GetMetrics()
	if metrics.WriteOperations != 0 {
		t.Fatalf("WriteOperations after reset %d doesn't match expected 0", metrics.WriteOperations)
	}
}

func TestSimulatedBackendStorageFileErrorInjection(t *testing.T) {
	config := SimulationConfig{
		Latency:       0,
		ErrorRate:     1.0, // 100% error rate
		Bandwidth:     0,
		DiskSpace:     0,
		EnableFailure: false,
		FailurePoint:  0,
	}

	file := NewSimulatedBackendStorageFile("test.dat", config)
	defer file.Close()

	// Should fail due to 100% error rate
	_, err := file.WriteAt([]byte("test"), 0)
	if err == nil {
		t.Fatal("WriteAt should have failed with 100%% error rate")
	}

	// Test failure point
	config.ErrorRate = 0.0
	config.EnableFailure = true
	config.FailurePoint = 100 // Fail after 100 bytes
	file.UpdateConfig(config)

	// First write should succeed
	testData := make([]byte, 50)
	_, err = file.WriteAt(testData, 0)
	if err != nil {
		t.Fatalf("First write should have succeeded: %v", err)
	}

	// Second write should fail
	_, err = file.WriteAt(testData, 50)
	if err == nil {
		t.Fatal("Second write should have failed due to failure point")
	}
}

func TestSimulatedBackendFactory(t *testing.T) {
	factory := &SimulatedBackendStorageFactory{}

	if factory.StorageType() != "simulation" {
		t.Fatalf("StorageType %s doesn't match expected 'simulation'", factory.StorageType())
	}

	// Test build with default config
	testProps := map[string]string{}
	props := &testProperties{testProps}
	storage, err := factory.BuildStorage(props, "", "test")
	if err != nil {
		t.Fatalf("BuildStorage failed: %v", err)
	}

	// Test creating a storage file
	volumeInfo := &volume_server_pb.VolumeInfo{}
	file := storage.NewStorageFile("test.dat", volumeInfo)

	if file == nil {
		t.Fatal("NewStorageFile returned nil")
	}

	// Test basic operations
	_, err = file.WriteAt([]byte("test"), 0)
	if err != nil {
		t.Fatalf("WriteAt failed: %v", err)
	}

	err = file.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestBandwidthLimitation(t *testing.T) {
	// Test with low bandwidth to verify it takes longer
	config := SimulationConfig{
		Latency:       0,
		ErrorRate:     0.0,
		Bandwidth:     1024, // 1KB/s
		DiskSpace:     0,
		EnableFailure: false,
		FailurePoint:  0,
	}

	file := NewSimulatedBackendStorageFile("test.dat", config)
	defer file.Close()

	// Write 2KB, should take approximately 2 seconds
	testData := make([]byte, 2048)
	start := time.Now()
	_, err := file.WriteAt(testData, 0)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("WriteAt failed: %v", err)
	}

	// Should take at least 1.5 seconds (allowing for some variance)
	if elapsed < 1500*time.Millisecond {
		t.Fatalf("WriteAt took %v, expected at least 1.5s with 1KB/s bandwidth", elapsed)
	}
}

func TestConfigParsing(t *testing.T) {
	// Test bandwidth parsing
	bandwidth, err := parseBandwidth("100MB/s")
	if err != nil {
		t.Fatalf("parseBandwidth failed: %v", err)
	}
	if bandwidth != 100*1024*1024 {
		t.Fatalf("Bandwidth %d doesn't match expected %d", bandwidth, 100*1024*1024)
	}

	// Test size parsing
	size, err := parseSize("10GB")
	if err != nil {
		t.Fatalf("parseSize failed: %v", err)
	}
	if size != 10*1024*1024*1024 {
		t.Fatalf("Size %d doesn't match expected %d", size, 10*1024*1024*1024)
	}
}
