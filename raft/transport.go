package raft

import (
	"fmt"
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
	defer func() {
		// TODO(ulysseses): clean up recover debug info
		r := recover()
		if r == nil {
			return
		}
		fmt.Println("Recovered:", r)
		for peerID, peerClient := range t.peerClients {
			fmt.Printf("peerID: %d, peerClient: %v\n", peerID, peerClient)
		}
		panic("")
	}()
	for {
		select {
		case <-t.stopChan:
			return
		case msg := <-t.raftTransportFacade.send():
			client, ok := t.peerClients[msg.Recipient]
			if !ok {
				panic(fmt.Sprintf("msg: %s", msg.String()))
			}
			if err := client.Send(msg); err != nil && err != io.EOF {
				panic(fmt.Sprintf("msg: %s, err: %v", msg.String(), err))
			}
		}
	}
}

func (t *transport) stop() {
	t.stopChan <- struct{}{}
}
