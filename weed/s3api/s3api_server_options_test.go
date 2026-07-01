package s3api

import "testing"

func TestNormalizeThroughputOptionsDefaults(t *testing.T) {
	option := &S3ApiServerOption{
		UploadChunkParallelism: 0,
		UploadChunkSizeMB:      0,
		DownloadChunkPrefetch:  0,
		DownloadCopyBufferKB:   0,
	}

	normalizeThroughputOptions(option)

	if option.UploadChunkParallelism != defaultUploadChunkParallelism {
		t.Fatalf("UploadChunkParallelism: got %d, want %d", option.UploadChunkParallelism, defaultUploadChunkParallelism)
	}
	if option.UploadChunkSizeMB != defaultUploadChunkSizeMB {
		t.Fatalf("UploadChunkSizeMB: got %d, want %d", option.UploadChunkSizeMB, defaultUploadChunkSizeMB)
	}
	if option.DownloadChunkPrefetch != defaultDownloadChunkPrefetch {
		t.Fatalf("DownloadChunkPrefetch: got %d, want %d", option.DownloadChunkPrefetch, defaultDownloadChunkPrefetch)
	}
	if option.DownloadCopyBufferKB != defaultDownloadCopyBufferKB {
		t.Fatalf("DownloadCopyBufferKB: got %d, want %d", option.DownloadCopyBufferKB, defaultDownloadCopyBufferKB)
	}
}

func TestNormalizeThroughputOptionsClampMax(t *testing.T) {
	option := &S3ApiServerOption{
		UploadChunkParallelism: maxUploadChunkParallelism + 10,
		UploadChunkSizeMB:      maxUploadChunkSizeMB + 100,
		DownloadChunkPrefetch:  maxDownloadChunkPrefetch + 5,
		DownloadCopyBufferKB:   maxDownloadCopyBufferKB + 128,
	}

	normalizeThroughputOptions(option)

	if option.UploadChunkParallelism != maxUploadChunkParallelism {
		t.Fatalf("UploadChunkParallelism: got %d, want %d", option.UploadChunkParallelism, maxUploadChunkParallelism)
	}
	if option.UploadChunkSizeMB != maxUploadChunkSizeMB {
		t.Fatalf("UploadChunkSizeMB: got %d, want %d", option.UploadChunkSizeMB, maxUploadChunkSizeMB)
	}
	if option.DownloadChunkPrefetch != maxDownloadChunkPrefetch {
		t.Fatalf("DownloadChunkPrefetch: got %d, want %d", option.DownloadChunkPrefetch, maxDownloadChunkPrefetch)
	}
	if option.DownloadCopyBufferKB != maxDownloadCopyBufferKB {
		t.Fatalf("DownloadCopyBufferKB: got %d, want %d", option.DownloadCopyBufferKB, maxDownloadCopyBufferKB)
	}
}

func TestNormalizeThroughputOptionsKeepValid(t *testing.T) {
	option := &S3ApiServerOption{
		UploadChunkParallelism: 32,
		UploadChunkSizeMB:      16,
		DownloadChunkPrefetch:  12,
		DownloadCopyBufferKB:   512,
	}

	normalizeThroughputOptions(option)

	if option.UploadChunkParallelism != 32 {
		t.Fatalf("UploadChunkParallelism: got %d, want 32", option.UploadChunkParallelism)
	}
	if option.UploadChunkSizeMB != 16 {
		t.Fatalf("UploadChunkSizeMB: got %d, want 16", option.UploadChunkSizeMB)
	}
	if option.DownloadChunkPrefetch != 12 {
		t.Fatalf("DownloadChunkPrefetch: got %d, want 12", option.DownloadChunkPrefetch)
	}
	if option.DownloadCopyBufferKB != 512 {
		t.Fatalf("DownloadCopyBufferKB: got %d, want 512", option.DownloadCopyBufferKB)
	}
}
