package s3api

import (
	"hash/crc32"
	"testing"

	"github.com/seaweedfs/seaweedfs/weed/s3api/s3_constants"
	"github.com/stretchr/testify/assert"
)

func TestCRC32Combine(t *testing.T) {
	part1Data := []byte("Hello, ")
	part2Data := []byte("World! This is a test of multipart CRC combination.")
	fullData := append(append([]byte(nil), part1Data...), part2Data...)

	crc1 := crc32.ChecksumIEEE(part1Data)
	crc2 := crc32.ChecksumIEEE(part2Data)
	fullCrc := crc32.ChecksumIEEE(fullData)

	params := crcCombineParams[ChecksumAlgorithmCRC32]
	combined := combineCRC(uint64(crc1), uint64(crc2), uint64(len(part2Data)), params)
	assert.Equal(t, uint64(fullCrc), combined, "Combined CRC32 must match CRC32 of full object")
}

func TestCRC32CCombine(t *testing.T) {
	tab := crc32.MakeTable(crc32.Castagnoli)
	part1Data := []byte("Part 1: 0123456789")
	part2Data := []byte("Part 2: 9876543210")
	fullData := append(append([]byte(nil), part1Data...), part2Data...)

	crc1 := crc32.Checksum(part1Data, tab)
	crc2 := crc32.Checksum(part2Data, tab)
	fullCrc := crc32.Checksum(fullData, tab)

	params := crcCombineParams[ChecksumAlgorithmCRC32C]
	combined := combineCRC(uint64(crc1), uint64(crc2), uint64(len(part2Data)), params)
	assert.Equal(t, uint64(fullCrc), combined, "Combined CRC32C must match CRC32C of full object")
}

func TestChecksumResultStruct(t *testing.T) {
	res := ChecksumResult{}
	res.SetChecksum(s3_constants.AmzChecksumCRC32, "9CDPXw==")
	assert.Equal(t, "9CDPXw==", res.ChecksumCRC32)

	val := res.GetChecksum(s3_constants.AmzChecksumCRC32)
	assert.Equal(t, "9CDPXw==", val)

	res64 := ChecksumResult{}
	res64.SetChecksum(s3_constants.AmzChecksumCRC64NVME, "CXv7nsB9zGw=")
	assert.Equal(t, "CXv7nsB9zGw=", res64.ChecksumCRC64NVME)
}
