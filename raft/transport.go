package raft

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/ulysseses/raft/raftpb"
	"google.golang.org/grpc"
)

var (
	// ErrNoRecipient is emitted when the raft message has no recipient.
	ErrNoRecipient = fmt.Errorf("no recipient")
	// ErrNoSender is emitted when the raft message has no sender.
	ErrNoSender = fmt.Errorf("no sender")
)

// transport interacts with the network via the raft protocol.
type transport struct {
	raftpb.UnimplementedRaftProtocolServer
	lis         net.Listener
	grpcServer  *grpc.Server
	peerClients map[uint64]*peerClient
	recvChan    chan raftpb.Message
	sendChan    chan raftpb.Message
	stopChan    chan struct{}
}

// Communicate loops receiving incoming raft protocol messages from the network.
func (t *transport) Communicate(stream raftpb.RaftProtocol_CommunicateServer) error {
	var (
		msg      raftpb.Message
		err      error
		recvChan chan<- raftpb.Message = t.recvChan
	)
	for err = stream.RecvMsg(msg); err == nil; {
		recvChan <- msg
	}
	return err
}

// sendLoop loops sending outgoing raft protocol messages.
func (t *transport) sendLoop() (err error) {
	var (
		msg      raftpb.Message
		sendChan <-chan raftpb.Message = t.sendChan
	)

	defer func() {
		if r := recover(); r != nil {
			log.Printf("recovered from %v", r)
			err = fmt.Errorf("responsible message: %s", msg.String())
		}
	}()

	for {
		select {
		case <-t.stopChan:
			return err
		case msg = <-sendChan:
			client := t.peerClients[msg.To].communicateClient
			if err = client.Send(&msg); err != nil {
				return err
			}
		}
	}
}

func (t *transport) start() {
	// start Communicate RPC
	go func() {
		if err := t.grpcServer.Serve(t.lis); err != nil {
			log.Print(err)
		}
	}()
	// start sendLoop
	go func() {
		if err := t.sendLoop(); err != nil {
			log.Print(err)
		}
	}()
}

// Stop stops the send and receive loop.
func (t *transport) stop() {
	t.stopChan <- struct{}{}
	t.grpcServer.Stop()
	if err := t.lis.Close(); err != nil {
		log.Print(err)
	}
	for _, peerClient := range t.peerClients {
		if err := peerClient.connCloser(); err != nil {
			log.Print(err)
		}
	}
}

// newTransport constructs a new `transport` from `Configuration`.
// Remember to connect this to a `raftStateMachine` via `bind`.
func newTransport(c Configuration) (*transport, error) {
	if c.MsgBufferSize < len(c.PeerAddresses)-1 {
		return nil, fmt.Errorf(
			"MsgBufferSize (%d) is too small; it must be at least %d",
			c.MsgBufferSize, len(c.PeerAddresses)-1)
	}
	t := transport{
		peerClients: map[uint64]*peerClient{},
		recvChan:    make(chan raftpb.Message, c.MsgBufferSize),
		sendChan:    make(chan raftpb.Message, c.MsgBufferSize),
		stopChan:    make(chan struct{}),
	}
	var (
		serverOptions = []grpc.ServerOption{}
		dialOptions   = []grpc.DialOption{}
		callOptions   = []grpc.CallOption{}
	)
	for _, opt := range c.GRPCOptions {
		switch opt.(type) {
		case WithGRPCServerOption:
			serverOptions = append(serverOptions, opt.(WithGRPCServerOption).opt)
		case WithGRPCDialOption:
			dialOptions = append(dialOptions, opt.(WithGRPCDialOption).opt)
		case WithGRPCCallOption:
			callOptions = append(callOptions, opt.(WithGRPCCallOption).opt)
		}
	}
	for id, addr := range c.PeerAddresses {
		if id == c.ID {
			tokens := strings.Split(addr, "://")
			if len(tokens) == 2 {
				lis, err := net.Listen(tokens[0], tokens[1])
				if err != nil {
					return nil, err
				}
				t.lis = lis
				t.grpcServer = grpc.NewServer(serverOptions...)
			} else {
				err := fmt.Errorf(
					"address for peer %d needs to be in {network}://{address} format. Got: %s",
					id, addr)
				return nil, err
			}
		} else {
			conn, err := grpc.Dial(addr, dialOptions...)
			if err != nil {
				return nil, err
			}
			client := raftpb.NewRaftProtocolClient(conn)
			stream, err := client.Communicate(context.Background(), callOptions...)
			if err != nil {
				return nil, err
			}
			peerClient := peerClient{
				communicateClient: stream,
				connCloser:        conn.Close,
			}
			t.peerClients[id] = &peerClient
		}
	}
	return &t, nil
}

type peerClient struct {
	communicateClient raftpb.RaftProtocol_CommunicateClient
	connCloser        func() error
}
