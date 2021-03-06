package main

import (
	"context"
	"sync"

	"github.com/ulysseses/raft"
	"github.com/ulysseses/raft/examples/kvstore/kvpb"
	"github.com/ulysseses/raft/pb"
)

// kvStore is a key value store that interfaces with Raft.
type kvStore struct {
	sync.RWMutex
	store    map[string]string
	_padding [64]byte
	node     *raft.Node
}

// set sets a key value pair.
func (kvStore *kvStore) set(ctx context.Context, k, v string) error {
	kv := kvpb.KV{K: k, V: v}
	data, err := kv.Marshal()
	if err != nil {
		return err
	}
	_, _, err = kvStore.node.Propose(ctx, data)
	return err
}

// get gets the value associated to a key.
func (kvStore *kvStore) get(ctx context.Context, k string) (v string, ok bool, err error) {
	if err = kvStore.node.Read(ctx); err != nil {
		return
	}

	kvStore.RLock()
	v, ok = kvStore.store[k]
	kvStore.RUnlock()
	return
}

// Apply implements raft.Application for kvStore
func (kvStore *kvStore) Apply(entries []pb.Entry) error {
	var kv kvpb.KV
	kvStore.Lock()
	for _, entry := range entries {
		if len(entry.Data) == 0 {
			continue
		}
		if err := kv.Unmarshal(entry.Data); err != nil {
			kvStore.Unlock()
			return err
		}
		kvStore.store[kv.K] = kv.V
	}
	kvStore.Unlock()
	return nil
}

// newKVStore constructs a new kvStore.
func newKVStore() *kvStore {
	kvStore := kvStore{
		store: map[string]string{},
	}
	return &kvStore
}
