package leveldb

import (
	"fmt"
	"os"

	"github.com/seaweedfs/seaweedfs/weed/filer"
)

var _ filer.BucketAware = (*LevelDB3Store)(nil)

func (store *LevelDB3Store) OnBucketCreation(bucket string) {
	fmt.Printf("KJ_TRACE: weed::filer::leveldb::leveldb3_store_bucket::OnBucketCreation()\n")
	store.createDB(bucket)
}

func (store *LevelDB3Store) OnBucketDeletion(bucket string) {
	fmt.Printf("KJ_TRACE: weed::filer::leveldb::leveldb3_store_bucket::OnBucketDeletion()\n")
	store.closeDB(bucket)
	if bucket != "" { // just to make sure
		os.RemoveAll(store.dir + "/" + bucket)
	}
}

func (store *LevelDB3Store) CanDropWholeBucket() bool {
	fmt.Printf("KJ_TRACE: weed::filer::leveldb::leveldb3_store_bucket::CanDropWholeBucket()\n")
	return true
}
