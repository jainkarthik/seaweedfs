package leveldb

import (
	"context"
	"fmt"

	"github.com/seaweedfs/seaweedfs/weed/filer"
	"github.com/syndtr/goleveldb/leveldb"
)

func (store *LevelDBStore) KvPut(ctx context.Context, key []byte, value []byte) (err error) {
	fmt.Printf("KJ_TRACE: weed::filer::leveldb::leveldb_store_kv::KvPut()\n")

	err = store.db.Put(key, value, nil)

	if err != nil {
		return fmt.Errorf("kv put: %w", err)
	}

	return nil
}

func (store *LevelDBStore) KvGet(ctx context.Context, key []byte) (value []byte, err error) {
	fmt.Printf("KJ_TRACE: weed::filer::leveldb::leveldb_store_kv::KvGet()\n")
	value, err = store.db.Get(key, nil)

	if err == leveldb.ErrNotFound {
		return nil, filer.ErrKvNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("kv get: %w", err)
	}

	return
}

func (store *LevelDBStore) KvDelete(ctx context.Context, key []byte) (err error) {
	fmt.Printf("KJ_TRACE: weed::filer::leveldb::leveldb_store_kv::KvDelete()\n")
	err = store.db.Delete(key, nil)

	if err != nil {
		return fmt.Errorf("kv delete: %w", err)
	}

	return nil
}
