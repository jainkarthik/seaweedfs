package backend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFakeDiskFile(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "fake_disk_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a temporary .dat file
	volumeFile := filepath.Join(tempDir, "1.dat")
	file, err := os.Create(volumeFile)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer file.Close()

	// Enable fake writes for this test
	os.Setenv("SEAWEED_FAKE_WRITES", "true")
	os.Setenv("SEAWEED_FAKE_WRITE_VOLUMES", "1")
	defer func() {
		os.Unsetenv("SEAWEED_FAKE_WRITES")
		os.Unsetenv("SEAWEED_FAKE_WRITE_VOLUMES")
		ResetFakeWriteConfig()
	}()

	// Create DiskFile (should be wrapped as FakeDiskFile)
	backendFile := NewDiskFile(file)
	defer backendFile.Close()

	// Check if it's a FakeDiskFile
	fakeFile, ok := backendFile.(*FakeDiskFile)
	if !ok {
		t.Fatalf("Expected FakeDiskFile, got %T", backendFile)
	}

	// Test if fake writes are enabled
	if !fakeFile.IsFakeWrite() {
		t.Error("Expected fake writes to be enabled")
	}

	// Test fake write operation
	testData := []byte("test data for fake write")
	n, err := backendFile.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if n != len(testData) {
		t.Errorf("Expected to write %d bytes, got %d", len(testData), n)
	}

	// Check write stats
	writeCount, bytesWritten, _ := fakeFile.GetWriteStats()
	if writeCount != 1 {
		t.Errorf("Expected 1 write, got %d", writeCount)
	}
	if bytesWritten != uint64(len(testData)) {
		t.Errorf("Expected %d bytes written, got %d", len(testData), bytesWritten)
	}

	// Test sync
	err = backendFile.Sync()
	if err != nil {
		t.Errorf("Sync failed: %v", err)
	}

	// Verify file size is updated (faked)
	size, modTime, err := backendFile.GetStat()
	if err != nil {
		t.Fatalf("GetStat failed: %v", err)
	}

	expectedSize := int64(len(testData))
	if size != expectedSize {
		t.Errorf("Expected file size %d, got %d", expectedSize, size)
	}

	// ModTime should be updated
	if modTime.IsZero() {
		t.Error("Expected modTime to be updated")
	}
}

func TestRealDiskFileWithoutFakeWrites(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "real_disk_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a temporary .dat file
	volumeFile := filepath.Join(tempDir, "2.dat")
	file, err := os.Create(volumeFile)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer file.Close()

	// Ensure fake writes are disabled
	os.Setenv("SEAWEED_FAKE_WRITES", "false")
	defer func() {
		os.Unsetenv("SEAWEED_FAKE_WRITES")
		ResetFakeWriteConfig()
	}()

	// Create DiskFile (should be regular DiskFile)
	backendFile := NewDiskFile(file)
	defer backendFile.Close()

	// Check if it's not a FakeDiskFile
	if _, ok := backendFile.(*FakeDiskFile); ok {
		t.Error("Expected regular DiskFile, got FakeDiskFile")
	}

	// Test real write operation
	testData := []byte("test data for real write")
	n, err := backendFile.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if n != len(testData) {
		t.Errorf("Expected to write %d bytes, got %d", len(testData), n)
	}

	// Verify actual file content
	file.Seek(0, 0)
	content := make([]byte, len(testData))
	readCount, err := file.Read(content)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if readCount != len(testData) {
		t.Errorf("Expected to read %d bytes, got %d", len(testData), readCount)
	}

	if string(content) != string(testData) {
		t.Errorf("Expected content %s, got %s", testData, content)
	}
}

func TestFakeWriteConfig(t *testing.T) {
	// Test default configuration
	os.Setenv("SEAWEED_FAKE_WRITES", "true")
	os.Setenv("SEAWEED_FAKE_WRITE_LOG", "true")
	os.Setenv("SEAWEED_FAKE_WRITE_LOG_LEVEL", "3")
	os.Setenv("SEAWEED_FAKE_WRITE_VOLUMES", "1,2,3")
	os.Setenv("SEAWEED_FAKE_WRITE_MAX_SIZE", "1048576")
	defer func() {
		os.Unsetenv("SEAWEED_FAKE_WRITES")
		os.Unsetenv("SEAWEED_FAKE_WRITE_LOG")
		os.Unsetenv("SEAWEED_FAKE_WRITE_LOG_LEVEL")
		os.Unsetenv("SEAWEED_FAKE_WRITE_VOLUMES")
		os.Unsetenv("SEAWEED_FAKE_WRITE_MAX_SIZE")
		ResetFakeWriteConfig()
	}()

	config := GetFakeWriteConfig()

	if !config.Enabled {
		t.Error("Expected fake writes to be enabled")
	}

	if !config.LogWrites {
		t.Error("Expected log writes to be enabled")
	}

	if config.LogLevel != 3 {
		t.Errorf("Expected log level 3, got %d", config.LogLevel)
	}

	expectedVolumes := []string{"1", "2", "3"}
	if len(config.VolumePatterns) != len(expectedVolumes) {
		t.Errorf("Expected %d volume patterns, got %d", len(expectedVolumes), len(config.VolumePatterns))
	}

	if config.MaxFakeSize != 1048576 {
		t.Errorf("Expected max fake size 1048576, got %d", config.MaxFakeSize)
	}
}
