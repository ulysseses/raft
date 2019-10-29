package raft

import (
	"context"
	"math/rand"
	"net"
	"time"

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
	raft        *raft
	transport   *transport
	grpcServer  *grpc.Server
	connClosers []func() error
	KVStore     *KVStore
}

// Stop stops the running loop goroutines of Node.
func (n *Node) Stop() {
	// raft must be stopped after transport/KVStore
	n.transport.stop <- struct{}{}
	n.KVStore.stop <- struct{}{}
	n.raft.stop <- struct{}{}
	n.grpcServer.Stop()
}

// NewNode constructs a Node.
func NewNode(c *Configuration) (*Node, error) {
	// construct raft
	raft := &raft{}

	if c.RecvChanSize > 0 {
		raft.recvChan = make(chan *raftpb.Message, c.RecvChanSize)
	} else {
		raft.recvChan = make(chan *raftpb.Message)
	}

	if c.SendChanSize > 0 {
		raft.sendChan = make(chan *raftpb.Message, c.SendChanSize)
	} else {
		raft.sendChan = make(chan *raftpb.Message)
	}

	raft.applyChan = make(chan []byte)
	raft.applyAckChan = make(chan struct{})

	if c.ProposeChanSize > 0 {
		raft.proposeChan = make(chan []byte, c.ProposeChanSize)
	} else {
		raft.proposeChan = make(chan []byte)
	}

	for peer := range c.Peers {
		raft.peers = append(raft.peers, peer)
	}
	raft.minElectionTimeoutTicks = c.MinElectionTimeoutTicks
	raft.maxElectionTimeoutTicks = c.MaxElectionTimeoutTicks
	raft.heartbeatTicks = c.HeartbeatTicks
	raft.tickMs = c.TickMs
	electionTimeoutTicks := rand.Intn(raft.maxElectionTimeoutTicks-raft.minElectionTimeoutTicks) + raft.minElectionTimeoutTicks
	electionTimeoutMs := time.Duration(electionTimeoutTicks*raft.tickMs) * time.Millisecond
	raft.electionTimeoutTimer = newTimer(electionTimeoutMs)
	raft.heartbeatTimer = newTimer(time.Duration(raft.heartbeatTicks*raft.tickMs) * time.Millisecond)
	raft.heartbeatTimer.Stop()

	raft.role = roleFollower
	raft.nextIndex = map[uint64]uint64{}
	raft.matchIndex = map[uint64]uint64{}
	for _, peer := range raft.peers {
		raft.nextIndex[peer] = 2
		raft.matchIndex[peer] = 1
	}

	raft.id = c.ID
	raft.log = []*raftpb.Entry{{Term: 1, Index: 1, Data: []byte{}}}
	raft.startIndex = 1
	raft.term = 1
	raft.applied = 1
	raft.committed = 1
	raft.quorumSize = (len(raft.peers) / 2) + 1
	raft.stop = make(chan struct{})
	raft.initialLeaderElectedSignalChan = make(chan struct{}, 1) // unbuffered

	// construct transport
	transport := &transport{
		raftTransportFacade: raft,
		clientsToPeers:      map[uint64]raftpb.RaftService_CommunicateWithPeerClient{},
		stop:                make(chan struct{}),
	}
	lis, err := net.Listen("tcp", c.Peers[raft.id])
	if err != nil {
		return nil, err
	}
	grpcServer := grpc.NewServer()
	raftpb.RegisterRaftServiceServer(grpcServer, transport)
	go grpcServer.Serve(lis)

	connClosers := []func() error{}
	for peerID, peerAddr := range c.Peers {
		conn, err := grpc.Dial(peerAddr, grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			return nil, err
		}
		client := raftpb.NewRaftServiceClient(conn)
		stream, err := client.CommunicateWithPeer(context.Background())
		if err != nil {
			return nil, err
		}
		transport.clientsToPeers[peerID] = stream
		connClosers = append(connClosers, conn.Close)
	}

	// construct KVStore
	kvStore := &KVStore{
		raftApplicationFacade: raft,
		store:                 map[string]string{},
		proposeChan:           make(chan raftpb.KV),
		stop:                  make(chan struct{}),
	}

	go raft.loop()
	go transport.sendLoop()
	go kvStore.loop()

	<-raft.initialLeaderElectedSignalChan

	node := &Node{
		raft:        raft,
		transport:   transport,
		grpcServer:  grpcServer,
		connClosers: connClosers,
		KVStore:     kvStore,
	}
	return node, nil
}
