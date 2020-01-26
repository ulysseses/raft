package raft

import (
	"fmt"

	"github.com/ulysseses/raft/pb"
)

var (
	fakeTransportRegistry = map[uint64]chan pb.Message{}
)

// channelTransport is an in-memory transport consisting of channels.
// No messages are dropped.
type channelTransport struct {
	recvChan chan pb.Message
	sendChan chan pb.Message
	id       uint64
	mIDs     []uint64
	stopChan chan struct{}

	outboxes map[uint64]chan<- pb.Message
}

// recv implements Transport for fakeTransport
func (t *channelTransport) recv() <-chan pb.Message {
	return t.recvChan
}

// send implements Transport for fakeTransport
func (t *channelTransport) send() chan<- pb.Message {
	return t.sendChan
}

// memberIDs implements Transport for fakeTransport
func (t *channelTransport) memberIDs() []uint64 {
	return t.mIDs
}

// start implements Transport for fakeTransport
func (t *channelTransport) start() {
	// connect to all peers via the registry
	if len(fakeTransportRegistry) != len(t.mIDs) {
		panic(fmt.Sprintf("not all members registered on fakeTransportRegistry"))
	}
	for id, ch := range fakeTransportRegistry {
		if t.id == id {
			continue
		}

		if ch == nil {
			panic(fmt.Sprintf("channelTransport ID %d doesn't exist", id))
		}

		t.outboxes[id] = ch
	}

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

// stop implements Transport for fakeTransport
func (t *channelTransport) stop() {
	t.stopChan <- struct{}{}
}

func newChannelTransport(cfg *TransportConfig) Transport {
	tr := channelTransport{
		recvChan: make(chan pb.Message, cfg.MsgBufferSize),
		sendChan: make(chan pb.Message, cfg.MsgBufferSize),
		id:       cfg.ID,
		mIDs:     cfg.Medium.(*ChannelMedium).MemberIDs,
		stopChan: make(chan struct{}),
		outboxes: map[uint64]chan<- pb.Message{},
	}

	// register
	fakeTransportRegistry[cfg.ID] = tr.recvChan

	return &tr
}

// function to be called to flush out the fakeTranportRegistry
func resetFakeTransportRegistry() {
	for id := range fakeTransportRegistry {
		delete(fakeTransportRegistry, id)
	}
}
