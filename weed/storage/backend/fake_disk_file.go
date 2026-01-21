package backend

import (
	"os"
	"time"

	"github.com/seaweedfs/seaweedfs/weed/glog"
)

var (
	_ BackendStorageFile = &FakeDiskFile{}
)

type FakeDiskFile struct {
	*DiskFile
	fakeWrites    bool
	writeCounter  uint64
	bytesWritten  uint64
	lastWriteTime time.Time
}

func NewFakeDiskFile(diskFile *DiskFile) *FakeDiskFile {
	config := GetFakeWriteConfig()
	fakeWrites := shouldFakeWriteForFile(diskFile.Name(), config)

	if fakeWrites && config.LogWrites {
		glog.V(2).Infof("Created FakeDiskFile for: %s", diskFile.Name())
	}

	return &FakeDiskFile{
		DiskFile:     diskFile,
		fakeWrites:   fakeWrites,
		writeCounter: 0,
		bytesWritten: 0,
	}
}

func (fdf *FakeDiskFile) WriteAt(p []byte, off int64) (n int, err error) {
	if fdf.fakeWrites {
		fdf.writeCounter++
		fdf.bytesWritten += uint64(len(p))
		fdf.lastWriteTime = time.Now()

		config := GetFakeWriteConfig()
		if config.LogWrites {
			glog.V(2).Infof("FAKE WRITE: %s offset=%d size=%d total_writes=%d total_bytes=%d",
				fdf.Name(), off, len(p), fdf.writeCounter, fdf.bytesWritten)
		}

		// Update file size to maintain consistency
		waterMark := off + int64(len(p))
		if waterMark > fdf.fileSize {
			fdf.fileSize = waterMark
			fdf.modTime = time.Now()
		}

		return len(p), nil
	}

	return fdf.DiskFile.WriteAt(p, off)
}

func (fdf *FakeDiskFile) Write(p []byte) (n int, err error) {
	if fdf.fakeWrites {
		return fdf.WriteAt(p, fdf.fileSize)
	}
	return fdf.DiskFile.WriteAt(p, fdf.fileSize)
}

func (fdf *FakeDiskFile) Sync() error {
	if fdf.fakeWrites {
		config := GetFakeWriteConfig()
		if config.LogWrites {
			glog.V(3).Infof("FAKE SYNC: %s", fdf.Name())
		}
		return nil
	}
	return fdf.DiskFile.Sync()
}

func (fdf *FakeDiskFile) GetWriteStats() (uint64, uint64, time.Time) {
	return fdf.writeCounter, fdf.bytesWritten, fdf.lastWriteTime
}

func (fdf *FakeDiskFile) IsFakeWrite() bool {
	return fdf.fakeWrites
}

// Add File field access for compatibility
func (fdf *FakeDiskFile) GetFile() *os.File {
	return fdf.DiskFile.File
}
