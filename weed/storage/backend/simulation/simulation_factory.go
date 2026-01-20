package simulation

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/seaweedfs/seaweedfs/weed/pb/volume_server_pb"
	"github.com/seaweedfs/seaweedfs/weed/storage/backend"
)

type SimulatedBackendStorage struct {
	config SimulationConfig
}

func (s *SimulatedBackendStorage) ToProperties() map[string]string {
	return map[string]string{
		"latency":        s.config.Latency.String(),
		"error_rate":     fmt.Sprintf("%.6f", s.config.ErrorRate),
		"bandwidth":      fmt.Sprintf("%d", s.config.Bandwidth),
		"disk_space":     fmt.Sprintf("%d", s.config.DiskSpace),
		"enable_failure": fmt.Sprintf("%t", s.config.EnableFailure),
		"failure_point":  fmt.Sprintf("%d", s.config.FailurePoint),
	}
}

func (s *SimulatedBackendStorage) NewStorageFile(key string, tierInfo *volume_server_pb.VolumeInfo) backend.BackendStorageFile {
	return NewSimulatedBackendStorageFile(key, s.config)
}

func (s *SimulatedBackendStorage) CopyFile(f *os.File, fn func(progressed int64, percentage float32) error) (key string, size int64, err error) {
	// Simulate file copy operation
	fileInfo, err := f.Stat()
	if err != nil {
		return "", 0, err
	}

	size = fileInfo.Size()
	key = f.Name()

	// Simulate copy progress
	if fn != nil {
		chunkSize := int64(1024 * 1024) // 1MB chunks
		for progressed := int64(0); progressed < size; progressed += chunkSize {
			if progressed+chunkSize > size {
				progressed = size
			}
			percentage := float32(progressed) / float32(size) * 100
			if err := fn(progressed, percentage); err != nil {
				return "", 0, err
			}

			// Simulate bandwidth limitation during copy
			if s.config.Bandwidth > 0 {
				chunkTime := time.Duration(chunkSize * 1e9 / s.config.Bandwidth)
				time.Sleep(chunkTime)
			}
		}
	}

	return key, size, nil
}

func (s *SimulatedBackendStorage) DownloadFile(fileName string, key string, fn func(progressed int64, percentage float32) error) (size int64, err error) {
	// Simulate download operation
	size = int64(100 * 1024 * 1024) // Default 100MB simulated file size

	if fn != nil {
		chunkSize := int64(1024 * 1024) // 1MB chunks
		for progressed := int64(0); progressed < size; progressed += chunkSize {
			if progressed+chunkSize > size {
				progressed = size
			}
			percentage := float32(progressed) / float32(size) * 100
			if err := fn(progressed, percentage); err != nil {
				return 0, err
			}

			// Simulate bandwidth limitation during download
			if s.config.Bandwidth > 0 {
				chunkTime := time.Duration(chunkSize * 1e9 / s.config.Bandwidth)
				time.Sleep(chunkTime)
			}
		}
	}

	return size, nil
}

func (s *SimulatedBackendStorage) DeleteFile(key string) error {
	// Simulate delete operation - just return success
	return nil
}

type SimulatedBackendStorageFactory struct{}

func (f *SimulatedBackendStorageFactory) StorageType() backend.StorageType {
	return "simulation"
}

func (f *SimulatedBackendStorageFactory) BuildStorage(configuration backend.StringProperties, configPrefix string, id string) (backend.BackendStorage, error) {
	config := SimulationConfig{
		Latency:       5 * time.Millisecond, // Default 5ms latency
		ErrorRate:     0.0,                  // Default no errors
		Bandwidth:     0,                    // Default unlimited bandwidth
		DiskSpace:     0,                    // Default unlimited disk space
		EnableFailure: false,                // Default no failure injection
		FailurePoint:  0,                    // Default no failure point
	}

	// Parse latency
	if latencyStr := configuration.GetString("latency"); latencyStr != "" {
		if latency, err := time.ParseDuration(latencyStr); err == nil {
			config.Latency = latency
		}
	}

	// Parse error rate
	if errorRateStr := configuration.GetString("error_rate"); errorRateStr != "" {
		if errorRate, err := strconv.ParseFloat(errorRateStr, 64); err == nil {
			config.ErrorRate = errorRate
		}
	}

	// Parse bandwidth (supports units like "100MB", "1GB")
	if bandwidthStr := configuration.GetString("bandwidth"); bandwidthStr != "" {
		if bandwidth, err := parseBandwidth(bandwidthStr); err == nil {
			config.Bandwidth = bandwidth
		}
	}

	// Parse disk space (supports units like "100GB", "1TB")
	if diskSpaceStr := configuration.GetString("disk_space"); diskSpaceStr != "" {
		if diskSpace, err := parseSize(diskSpaceStr); err == nil {
			config.DiskSpace = diskSpace
		}
	}

	// Parse enable failure
	if enableFailureStr := configuration.GetString("enable_failure"); enableFailureStr != "" {
		if enableFailure, err := strconv.ParseBool(enableFailureStr); err == nil {
			config.EnableFailure = enableFailure
		}
	}

	// Parse failure point
	if failurePointStr := configuration.GetString("failure_point"); failurePointStr != "" {
		if failurePoint, err := strconv.ParseInt(failurePointStr, 10, 64); err == nil {
			config.FailurePoint = failurePoint
		}
	}

	return &SimulatedBackendStorage{config: config}, nil
}

func parseBandwidth(s string) (int64, error) {
	s = strings.ToUpper(strings.TrimSpace(s))

	if strings.HasSuffix(s, "B/S") {
		// Remove suffix and parse
		valueStr := s[:len(s)-3]
		if value, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
			return value, nil
		}
	} else if strings.HasSuffix(s, "MB/S") {
		valueStr := s[:len(s)-4]
		if value, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
			return value * 1024 * 1024, nil
		}
	} else if strings.HasSuffix(s, "GB/S") {
		valueStr := s[:len(s)-4]
		if value, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
			return value * 1024 * 1024 * 1024, nil
		}
	} else if strings.HasSuffix(s, "KB/S") {
		valueStr := s[:len(s)-4]
		if value, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
			return value * 1024, nil
		}
	} else {
		// Assume bytes per second if no unit
		return strconv.ParseInt(s, 10, 64)
	}

	return 0, fmt.Errorf("invalid bandwidth format: %s", s)
}

func parseSize(s string) (int64, error) {
	s = strings.ToUpper(strings.TrimSpace(s))

	if strings.HasSuffix(s, "B") {
		valueStr := s[:len(s)-1]
		if value, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
			return value, nil
		}
	} else if strings.HasSuffix(s, "KB") {
		valueStr := s[:len(s)-2]
		if value, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
			return value * 1024, nil
		}
	} else if strings.HasSuffix(s, "MB") {
		valueStr := s[:len(s)-2]
		if value, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
			return value * 1024 * 1024, nil
		}
	} else if strings.HasSuffix(s, "GB") {
		valueStr := s[:len(s)-2]
		if value, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
			return value * 1024 * 1024 * 1024, nil
		}
	} else if strings.HasSuffix(s, "TB") {
		valueStr := s[:len(s)-2]
		if value, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
			return value * 1024 * 1024 * 1024 * 1024, nil
		}
	} else {
		// Assume bytes if no unit
		return strconv.ParseInt(s, 10, 64)
	}

	return 0, fmt.Errorf("invalid size format: %s", s)
}

// Register the simulation backend factory
func init() {
	backend.BackendStorageFactories["simulation"] = &SimulatedBackendStorageFactory{}
}
