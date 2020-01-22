package raft

import (
	"fmt"

	"github.com/ulysseses/raft/pb"
)

// fakeTransport is an in-memory transport consisting of channels.
// No messages are dropped.
type fakeTransport struct {
	recvChan chan pb.Message
	sendChan chan pb.Message
	id       uint64
	stopChan chan struct{}

	outboxes map[uint64]chan<- pb.Message
}

func (t *fakeTransport) recv() <-chan pb.Message {
	return t.recvChan
}

func (t *fakeTransport) send() chan<- pb.Message {
	return t.sendChan
}

func (t *fakeTransport) memberIDs() []uint64 {
	mIDs := []uint64{t.id}
	for pID := range t.outboxes {
		mIDs = append(mIDs, pID)
	}
	return mIDs
}

func (t *fakeTransport) start() {
	go func() {
		for {
			select {
			case <-t.stopChan:
				return
			case msg := <-t.sendChan:
				if outbox, ok := t.outboxes[msg.To]; ok {
					select {
					case <-t.stopChan:
						return
					case outbox <- msg:
					}
				} else {
					panic(fmt.Sprintf("unrecognized recipient: %d", msg.To))
				}
			}
		}
	}()
}

func (t *fakeTransport) stop() {
	t.stopChan <- struct{}{}
}

// bind is symmetric and idempotent
func (t *fakeTransport) bind(peer *fakeTransport) {
	t.outboxes[peer.id] = peer.recvChan
	peer.outboxes[t.id] = t.recvChan
}

func newFakeTransports(ids ...uint64) map[uint64]Transport {
	trs := map[uint64]Transport{}
	for _, id := range ids {
		trs[id] = &fakeTransport{
			recvChan: make(chan pb.Message, (len(ids)-1)*2+1),
			sendChan: make(chan pb.Message, (len(ids)-1)*2+1),
			id:       id,
			stopChan: make(chan struct{}),

			outboxes: map[uint64]chan<- pb.Message{},
		}
	}

	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			tri := trs[ids[i]].(*fakeTransport)
			trj := trs[ids[j]].(*fakeTransport)
			tri.bind(trj)
		}
	}

	return trs
}
