package backend

import (
	"os"
	"strconv"
	"strings"

	"github.com/seaweedfs/seaweedfs/weed/glog"
)

type FakeWriteConfig struct {
	Enabled        bool
	VolumePatterns []string
	FilePatterns   []string
	MaxFakeSize    int64
	LogWrites      bool
	LogLevel       int
}

var globalFakeWriteConfig *FakeWriteConfig

func GetFakeWriteConfig() *FakeWriteConfig {
	if globalFakeWriteConfig == nil {
		globalFakeWriteConfig = loadFakeWriteConfig()
	}
	return globalFakeWriteConfig
}

func loadFakeWriteConfig() *FakeWriteConfig {
	config := &FakeWriteConfig{
		Enabled:     os.Getenv("SEAWEED_FAKE_WRITES") == "true",
		MaxFakeSize: parseEnvSize("SEAWEED_FAKE_WRITE_MAX_SIZE", -1),
		LogWrites:   os.Getenv("SEAWEED_FAKE_WRITE_LOG") == "true",
		LogLevel:    parseEnvInt("SEAWEED_FAKE_WRITE_LOG_LEVEL", 2),
	}

	config.VolumePatterns = parseEnvList("SEAWEED_FAKE_WRITE_VOLUMES")
	config.FilePatterns = parseEnvList("SEAWEED_FAKE_WRITE_PATTERNS")

	// Validate configuration
	if err := validateFakeWriteConfig(config); err != nil {
		glog.Fatalf("Invalid fake write configuration: %v", err)
	}

	return config
}

func parseEnvSize(key string, defaultValue int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	size, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		glog.V(0).Infof("Invalid size value for %s: %s, using default %d", key, value, defaultValue)
		return defaultValue
	}

	return size
}

func parseEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		glog.V(0).Infof("Invalid int value for %s: %s, using default %d", key, value, defaultValue)
		return defaultValue
	}

	return intValue
}

func parseEnvList(key string) []string {
	value := os.Getenv(key)
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	var result []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

func validateFakeWriteConfig(config *FakeWriteConfig) error {
	if config.Enabled && isProductionEnvironment() {
		return configError("FAKE WRITES CANNOT BE ENABLED IN PRODUCTION")
	}

	if config.MaxFakeSize > 0 && config.MaxFakeSize < 1024 {
		return configError("MAX_FAKE_SIZE too small, minimum 1KB")
	}

	return nil
}

func isProductionEnvironment() bool {
	return os.Getenv("ENVIRONMENT") == "production" ||
		os.Getenv("SEAWeedFS_ENV") == "production" ||
		os.Getenv("GO_ENV") == "production"
}

type configError string

func (e configError) Error() string {
	return string(e)
}

func ResetFakeWriteConfig() {
	globalFakeWriteConfig = nil
}
