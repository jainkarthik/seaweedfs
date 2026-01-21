package backend

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/seaweedfs/seaweedfs/weed/glog"
)

func shouldFakeWriteForFile(filePath string, config *FakeWriteConfig) bool {
	if !config.Enabled {
		return false
	}

	// Only fake writes for .dat files (data files)
	if !strings.HasSuffix(filePath, ".dat") {
		return false
	}

	// Extract volume ID from file path
	volumeId := extractVolumeIdFromPath(filePath)
	if volumeId == 0 {
		return false
	}

	// Check volume patterns
	if len(config.VolumePatterns) > 0 {
		if !matchesAnyPattern(strconv.Itoa(int(volumeId)), config.VolumePatterns) {
			return false
		}
	}

	return true
}

func extractVolumeIdFromPath(filePath string) uint32 {
	// Extract volume ID from path like "/path/to/volume.dat"
	// or "/path/to/1.dat" -> volume ID 1
	parts := strings.Split(filePath, "/")
	if len(parts) == 0 {
		return 0
	}

	filename := parts[len(parts)-1]
	// Remove extension
	base := strings.TrimSuffix(filename, ".dat")
	base = strings.TrimSuffix(base, ".idx")

	volumeId, err := strconv.Atoi(base)
	if err != nil {
		glog.V(4).Infof("Could not extract volume ID from path: %s", filePath)
		return 0
	}

	return uint32(volumeId)
}

func matchesAnyPattern(str string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, str); matched {
			return true
		}
	}
	return false
}
