package raft

import (
	"fmt"
	"testing"
	"time"

	"github.com/ulysseses/raft/kvpb"
)

// fakeRaftApplicationFacade implements raftApplicationFacade interface.
type fakeRaftApplicationFacade struct {
	proposeChan  chan []byte
	applyChan    chan []byte
	applyAckChan chan struct{}
	stopChan     chan struct{}
}

func (r *fakeRaftApplicationFacade) apply() <-chan []byte {
	return r.applyChan
}

func (r *fakeRaftApplicationFacade) applyAck() chan<- struct{} {
	return r.applyAckChan
}

func (r *fakeRaftApplicationFacade) propose() chan<- []byte {
	return r.proposeChan
}

func newFakeRaftApplicationFacade() *fakeRaftApplicationFacade {
	return &fakeRaftApplicationFacade{
		proposeChan:  make(chan []byte),
		applyChan:    make(chan []byte),
		applyAckChan: make(chan struct{}),
		stopChan:     make(chan struct{}),
	}
}

func TestPropose(t *testing.T) {
	fakeRaft := newFakeRaftApplicationFacade()
	go func() {
		for {
			select {
			case <-fakeRaft.stopChan:
				return
			case <-fakeRaft.proposeChan:
			}
		}
	}()
	defer func() { fakeRaft.stopChan <- struct{}{} }()
	kvStore := &KVStore{
		raftApplicationFacade: fakeRaft,
		store:                 map[string]string{},
		proposeChan:           make(chan kvpb.KV),
		stopChan:              make(chan struct{}),
	}
	go kvStore.loop()
	defer func() { kvStore.stopChan <- struct{}{} }()
	for i := 0; i < 3; i++ {
		done := make(chan struct{})
		go func(done chan<- struct{}) {
			kvStore.Propose("hi", "there")
			done <- struct{}{}
		}(done)
		select {
		case <-done:
		case <-time.After(10 * time.Millisecond):
			t.FailNow()
		}
	}
}

// potentially flaky
func TestGet(t *testing.T) {
	fakeRaft := newFakeRaftApplicationFacade()
	go func() {
		for {
			select {
			case data := <-fakeRaft.proposeChan:
				fakeRaft.applyChan <- data
			case <-fakeRaft.applyAckChan:
			case <-fakeRaft.stopChan:
				return
			}
		}
	}()
	defer func() { close(fakeRaft.stopChan) }()

	kvStore := &KVStore{
		raftApplicationFacade: fakeRaft,
		store:                 map[string]string{},
		proposeChan:           make(chan kvpb.KV),
		stopChan:              make(chan struct{}),
	}
	go kvStore.loop()
	defer func() { close(kvStore.stopChan) }()

	const key = "someKey"
	for i := 1; i < 6; i++ {
		want := fmt.Sprintf("%d", i)
		kvStore.Propose(key, want)
		time.Sleep(time.Millisecond)
		got, ok := kvStore.Get(key)
		if !ok {
			t.Fatalf("did not get proposed value of %s after 1 ms", want)
		}
		if got != want {
			t.Fatalf("expected %s, but got %s instead", want, got)
		}
	}
}
