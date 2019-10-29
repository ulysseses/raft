package raft

import (
	"io"

	"github.com/ulysseses/raft/raftpb"
)

type transport struct {
	raftpb.UnimplementedRaftServiceServer
	raftTransportFacade raftTransportFacade
	clientsToPeers      map[uint64]raftpb.RaftService_CommunicateWithPeerClient
	stop                chan struct{}
}

func (t *transport) CommunicateWithPeer(stream raftpb.RaftService_CommunicateWithPeerServer) error {
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		t.raftTransportFacade.recv() <- msg
	}
}

func (t *transport) sendLoop() {
	for {
		select {
		case <-t.stop:
			return
		case msg := <-t.raftTransportFacade.send():
			if err := t.clientsToPeers[msg.Recipient].Send(msg); err != nil {
				panic(err)
			}
		}
	}
}
