package datastore

import (
	"context"
)

type op struct {
	delete bool
	value  []byte
}

// basicBatch implements the transaction interface for datastores who do
// not have any sort of underlying transactional support
type basicBatch struct {
	ops map[Key]op

	target Datastore
}

var _ Batch = (*basicBatch)(nil)

func NewBasicBatch(ds Datastore) Batch {
	return &basicBatch{
		ops:    make(map[Key]op),
		target: ds,
	}
}

func (bt *basicBatch) Put(ctx context.Context, key Key, val []byte) error {
	bt.ops[key] = op{value: val}
	return nil
}

func (bt *basicBatch) Delete(ctx context.Context, key Key) error {
	bt.ops[key] = op{delete: true}
	return nil
}

func (bt *basicBatch) Commit(ctx context.Context) error {
	var err error
	for k, op := range bt.ops {
		if op.delete {
			err = bt.target.Delete(ctx, k)
		} else {
			err = bt.target.Put(ctx, k, op.value)
		}
		if err != nil {
			break
		}
	}

	return err
}
