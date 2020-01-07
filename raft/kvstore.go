package raft

import (
	"sync"

	"github.com/ulysseses/raft/kvpb"
)

// KVStore is an in-memory key-value store that is attached to a raft node.
type KVStore struct {
	sync.RWMutex

	raftApplicationFacade raftApplicationFacade
	store                 map[string]string

	proposeChan chan kvpb.KV
	stopChan    chan struct{}
}

// Get gets the value for a key
func (s *KVStore) Get(k string) (string, bool) {
	s.RLock()
	v, ok := s.store[k]
	s.RUnlock()
	return v, ok
}

// Propose acts like an asynchronous set operation
func (s *KVStore) Propose(k, v string) {
	s.proposeChan <- kvpb.KV{K: k, V: v}
}

func (s *KVStore) loop() {
	for {
		select {
		case data := <-s.raftApplicationFacade.apply():
			var kv kvpb.KV
			if err := kv.Unmarshal(data); err != nil {
				panic(err)
			}
			s.Lock()
			s.store[string(kv.K)] = string(kv.V)
			s.Unlock()
			s.raftApplicationFacade.applyAck() <- struct{}{}
		case kv := <-s.proposeChan:
			data, err := kv.Marshal()
			if err != nil {
				panic(err)
			}
			s.raftApplicationFacade.propose() <- data
		case <-s.stopChan:
			return
		}
	}
}
