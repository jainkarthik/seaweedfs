package version

import (
	"fmt"
	"os"

	"github.com/seaweedfs/seaweedfs/weed/stats"
	"github.com/seaweedfs/seaweedfs/weed/util"
)

var (
	MAJOR_VERSION  = int32(4)
	MINOR_VERSION  = int32(12)
	VERSION_NUMBER = fmt.Sprintf("%d.%02d", MAJOR_VERSION, MINOR_VERSION)
	VERSION        string
	COMMIT         string
	BUILD_DATE     string
)

func init() {
	VERSION = util.SizeLimit + " " + VERSION_NUMBER
	if v := os.Getenv("VERSION"); v != "" {
		VERSION = v
	}
	if commit := os.Getenv("COMMIT"); commit != "" {
		COMMIT = commit
	}
	if date := os.Getenv("BUILD_DATE"); date != "" {
		BUILD_DATE = date
	}
	stats.SetVersionInfo(VERSION_NUMBER, COMMIT, util.SizeLimit)
}

func Version() string {
	return VERSION + " " + COMMIT
}
