package needle

import (
	"fmt"
)

type AsyncRequest struct {
	N              *Needle
	IsWriteRequest bool
	ActualSize     int64
	offset         uint64
	size           uint64
	doneChan       chan interface{}
	isUnchanged    bool
	err            error
}

func NewAsyncRequest(n *Needle, isWriteRequest bool) *AsyncRequest {
	fmt.Printf("KJ_TRACE: weed::storage::needle::async_request::NewAsyncRequest()\n")
	return &AsyncRequest{
		offset:         0,
		size:           0,
		ActualSize:     0,
		doneChan:       make(chan interface{}),
		N:              n,
		isUnchanged:    false,
		IsWriteRequest: isWriteRequest,
		err:            nil,
	}
}

func (r *AsyncRequest) WaitComplete() (uint64, uint64, bool, error) {
	<-r.doneChan
	fmt.Printf("KJ_TRACE: weed::storage::needle::async_request::WaitComplete()\n")
	return r.offset, r.size, r.isUnchanged, r.err
}

func (r *AsyncRequest) Complete(offset uint64, size uint64, isUnchanged bool, err error) {
	fmt.Printf("KJ_TRACE: weed::storage::needle::async_request::Complete()\n")
	r.offset = offset
	r.size = size
	r.isUnchanged = isUnchanged
	r.err = err
	close(r.doneChan)
}

func (r *AsyncRequest) UpdateResult(offset uint64, size uint64, isUnchanged bool, err error) {
	fmt.Printf("KJ_TRACE: weed::storage::needle::async_request::UpdateResult()\n")
	r.offset = offset
	r.size = size
	r.isUnchanged = isUnchanged
	r.err = err
}

func (r *AsyncRequest) Submit() {
	fmt.Printf("KJ_TRACE: weed::storage::needle::async_request::Submit()\n")
	close(r.doneChan)
}

func (r *AsyncRequest) IsSucceed() bool {
	fmt.Printf("KJ_TRACE: weed::storage::needle::async_request::IsSucceed()\n")
	return r.err == nil
}
