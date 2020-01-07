package raft

import (
	"fmt"
	"io"

	"github.com/ulysseses/raft/raftpb"
)

type transport struct {
	raftpb.UnimplementedRaftServer
	raftTransportFacade raftTransportFacade
	peerClients         map[uint64]raftpb.Raft_CommunicateClient
	stopChan            chan struct{}
}

func (t *transport) CommunicateWithPeer(stream raftpb.Raft_CommunicateServer) error {
	recvChan := t.raftTransportFacade.recv()
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case <-t.stopChan:
			return nil
		case recvChan <- *msg:
		}
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
	sendChan := t.raftTransportFacade.send()
	for {
		select {
		case <-t.stopChan:
			return
		case msg := <-sendChan:
			client, ok := t.peerClients[msg.To]
			if !ok {
				panic(fmt.Sprintf("msg: %s", msg.String()))
			}
			if err := client.Send(&msg); err != nil && err != io.EOF {
				panic(fmt.Sprintf("msg: %s, err: %v", msg.String(), err))
			}
		}
	}
}

func (t *transport) stop() {
	t.stopChan <- struct{}{}
}
