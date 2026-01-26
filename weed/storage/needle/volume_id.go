package needle

import (
	"fmt"
	"strconv"
)

type VolumeId uint32

func NewVolumeId(vid string) (VolumeId, error) {
	fmt.Printf("KJ_TRACE: weed::storage::needle::volume_id::NewVolumeId(volId,err)\n")
	volumeId, err := strconv.ParseUint(vid, 10, 64)
	return VolumeId(volumeId), err
}
func (vid VolumeId) String() string {
	fmt.Printf("KJ_TRACE: weed::storage::needle::volume_id::String(str)\n")
	return strconv.FormatUint(uint64(vid), 10)
}
func (vid VolumeId) Next() VolumeId {
	fmt.Printf("KJ_TRACE: weed::storage::needle::volume_id::Next(volId)\n")
	return VolumeId(uint32(vid) + 1)
}
