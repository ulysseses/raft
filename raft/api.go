package raft

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/ulysseses/raft/raftpb"
	"google.golang.org/grpc"
)

// Configuration configures the Raft system.
type Configuration struct {
	RecvChanSize            int
	SendChanSize            int
	ID                      uint64
	Peers                   map[uint64]string
	MinElectionTimeoutTicks int
	MaxElectionTimeoutTicks int
	HeartbeatTicks          int
	TickMs                  int
	ProposeChanSize         int
}

// Node is the public structure for raft
type Node struct {
	raft              raftFacade
	transport         *transport
	grpcServer        *grpc.Server
	lis               net.Listener
	clientConnClosers []func() error
	KVStore           *KVStore
}

// Stop stops the running loop goroutines of Node.
func (n *Node) Stop() error {
	n.pause()
	if err := n.lis.Close(); err != nil {
		return err
	}
	for _, connCloser := range n.clientConnClosers {
		if err := connCloser(); err != nil {
			return err
		}
	}
	return nil
}

func (n *Node) pause() {
	n.raft.pause()
}

func (n *Node) unpause() {
	n.raft.unpause()
}

// NewNode constructs a Node.
func NewNode(c *Configuration) (*Node, error) {
	// construct raft
	raft := newRaft(c)

	// construct transport
	transport := &transport{
		raftTransportFacade: raft,
		peerClients:         map[uint64]raftpb.RaftService_CommunicateWithPeerClient{},
		stopChan:            make(chan struct{}),
	}

	// TODO(ulysseses): have a better way to specify protocol...
	var lis net.Listener
	tokens := strings.Split(c.Peers[raft.getID()], "://")
	if len(tokens) == 1 {
		// assume tcp
		var err error
		lis, err = net.Listen("tcp", c.Peers[raft.getID()])
		if err != nil {
			return nil, err
		}
	} else if len(tokens) == 2 {
		var err error
		lis, err = net.Listen(tokens[0], tokens[1])
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("invalid server address: %s", c.Peers[raft.getID()])
	}

	grpcServer := grpc.NewServer()
	raftpb.RegisterRaftServiceServer(grpcServer, transport)
	go grpcServer.Serve(lis)

	clientConnClosers := []func() error{}
	for peerID, peerAddr := range c.Peers {
		conn, err := grpc.Dial(peerAddr, grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			lis.Close()
			return nil, err
		}
		client := raftpb.NewRaftServiceClient(conn)
		stream, err := client.CommunicateWithPeer(context.Background())
		if err != nil {
			lis.Close()
			return nil, err
		}
		transport.peerClients[peerID] = stream
		clientConnClosers = append(clientConnClosers, conn.Close)
	}

	// construct KVStore
	kvStore := &KVStore{
		raftApplicationFacade: raft,
		store:                 map[string]string{},
		proposeChan:           make(chan raftpb.KV),
		stopChan:              make(chan struct{}),
	}

	go raft.loop()
	go transport.sendLoop()
	go kvStore.loop()

	raft.waitUntilInitialLeaderElected()

	node := &Node{
		raft:              raft,
		transport:         transport,
		grpcServer:        grpcServer,
		lis:               lis,
		clientConnClosers: clientConnClosers,
		KVStore:           kvStore,
	}
	return node, nil
}
