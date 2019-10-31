package raft

import (
	"sync"

	"github.com/gogo/protobuf/proto"
	"github.com/ulysseses/raft/raftpb"
)

// KVStore is an in-memory key-value store that is attached to a raft node.
type KVStore struct {
	raftApplicationFacade raftApplicationFacade
	store                 map[string]string

	mu sync.RWMutex

	proposeChan chan raftpb.KV
	stopChan    chan struct{}
}

// Get gets the value for a key
func (s *KVStore) Get(k string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.store[k]
	return v, ok
}

// Propose acts like an asynchronous set operation
func (s *KVStore) Propose(k, v string) {
	s.proposeChan <- raftpb.KV{K: k, V: v}
}

func (s *KVStore) loop() {
	for {
		select {
		case data := <-s.raftApplicationFacade.apply():
			var kv raftpb.KV
			if err := proto.Unmarshal(data, &kv); err != nil {
				panic(err)
			}
			s.mu.Lock()
			s.store[kv.K] = kv.V
			s.mu.Unlock()
			s.raftApplicationFacade.applyAck() <- struct{}{}
		case kv := <-s.proposeChan:
			data, err := proto.Marshal(&kv)
			if err != nil {
				panic(err)
			}
			s.raftApplicationFacade.propose() <- data
		case <-s.stopChan:
			return
		}
	}
}
