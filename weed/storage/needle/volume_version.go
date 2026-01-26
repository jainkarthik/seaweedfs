package needle

import (
	"fmt"
)

type Version uint8

const (
	Version1 = Version(1)
	Version2 = Version(2)
	Version3 = Version(3)
)

func GetCurrentVersion() Version {
	fmt.Printf("KJ_TRACE: weed::storage::needle::volume_version::GetCurrentVersion()\n")
	return Version3
}

func IsSupportedVersion(v Version) bool {
	fmt.Printf("KJ_TRACE: weed::storage::needle::volume_version::IsSupportedVersion()\n")
	return v >= Version1 && v <= Version3
}
