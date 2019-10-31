package raft

import (
	"io"

	"github.com/ulysseses/raft/raftpb"
)

type transport struct {
	raftpb.UnimplementedRaftServiceServer
	raftTransportFacade raftTransportFacade
	peerClients         map[uint64]raftpb.RaftService_CommunicateWithPeerClient
	stopChan            chan struct{}
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
		case <-t.stopChan:
			return
		case msg := <-t.raftTransportFacade.send():
			if err := t.peerClients[msg.Recipient].Send(msg); err != nil && err != io.EOF {
				panic(err)
			}
		}
	}
}

func (t *transport) stop() {
	t.stopChan <- struct{}{}
}
