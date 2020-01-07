package raft

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/ulysseses/raft/raftpb"
	"google.golang.org/grpc"
)

// fakeRaftTransportFacade implements raftTransportFacade interface.
type fakeRaftTransportFacade struct {
	recvChan chan raftpb.Message
	sendChan chan raftpb.Message
	stopChan chan struct{}
}

// recv implements raftTransportFacade interface.
func (r *fakeRaftTransportFacade) recv() chan<- raftpb.Message {
	return r.recvChan
}

// send implements raftTransportFacade interface.
func (r *fakeRaftTransportFacade) send() <-chan raftpb.Message {
	return r.sendChan
}

// recvLoop receives messages from transport but does nothing with them.
func (r *fakeRaftTransportFacade) recvLoop() {
	for {
		select {
		case <-r.recvChan:
		case <-r.stopChan:
			return
		}
	}
}

// stopRecvLoop stops recvLoop goroutine.
func (r *fakeRaftTransportFacade) stopRecvLoop() {
	r.stopChan <- struct{}{}
}

func newFakeRaftTransportFacade() *fakeRaftTransportFacade {
	r := &fakeRaftTransportFacade{
		recvChan: make(chan raftpb.Message),
		sendChan: make(chan raftpb.Message),
		stopChan: make(chan struct{}),
	}
	return r
}

type serverState struct {
	lis        net.Listener
	grpcServer *grpc.Server
	transport  *transport
}

func prepareServerState(unixSocketPath string) (*serverState, error) {
	fakeRaft := newFakeRaftTransportFacade()
	transport := &transport{
		raftTransportFacade: fakeRaft,
		stopChan:            make(chan struct{}),
		peerClients:         map[uint64]raftpb.RaftService_CommunicateWithPeerClient{},
	}

	lis, err := net.Listen("unix", unixSocketPath)
	if err != nil {
		return nil, err
	}
	grpcServer := grpc.NewServer()
	raftpb.RegisterRaftServiceServer(grpcServer, transport)
	go grpcServer.Serve(lis)

	return &serverState{
		lis:        lis,
		grpcServer: grpcServer,
		transport:  transport,
	}, nil
}

func (ss *serverState) stop() {
	ss.grpcServer.Stop()
	ss.lis.Close()
}

type clientState struct {
	conn   *grpc.ClientConn
	stream raftpb.RaftService_CommunicateWithPeerClient
}

func prepareClientState(unixSocketPath string) (*clientState, error) {
	conn, err := grpc.Dial(
		fmt.Sprintf("unix://%s", unixSocketPath),
		grpc.WithInsecure(),
		grpc.WithBlock())
	if err != nil {
		return nil, err
	}
	client := raftpb.NewRaftServiceClient(conn)
	stream, err := client.CommunicateWithPeer(context.Background())
	if err != nil {
		return nil, err
	}
	return &clientState{
		conn:   conn,
		stream: stream,
	}, nil
}

func (cs *clientState) stop() {
	cs.conn.Close()
}

func TestCommunicateWithPeer(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tmpUnixSocketPath := filepath.Join(tmpDir, "TestCommunicateWithPeer.sock")
	ss, err := prepareServerState(tmpUnixSocketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.stop()

	cs, err := prepareClientState(tmpUnixSocketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.stop()

	msgs := []*raftpb.Message{
		&raftpb.Message{},
		&raftpb.Message{},
		&raftpb.Message{},
	}
	go func() {
		for _, msg := range msgs {
			if err := cs.stream.Send(msg); err != nil {
				t.Fatal(err)
			}
		}
	}()

	// verifyReceivedMessages verifies that each message in `wantMsgs` was received within `timeout`.
	verifyReceivedMessages := func(
		r *fakeRaftTransportFacade,
		wantMsgs []*raftpb.Message,
		timeout time.Duration,
	) bool {
		for i := 0; i < len(wantMsgs); i++ {
			select {
			case <-time.After(timeout):
				return false
			case gotMsg := <-r.recvChan:
				if !proto.Equal(wantMsgs[i], &gotMsg) {
					return false
				}
			}
		}
		return true
	}

	complete := verifyReceivedMessages(
		ss.transport.raftTransportFacade.(*fakeRaftTransportFacade),
		msgs,
		500*time.Millisecond)
	if !complete {
		t.Error("did not receive all messages in time")
	}
}

func TestSendLoop(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	peers := map[uint64]string{
		111: filepath.Join(tmpDir, "TestSendLoop111.sock"),
		222: filepath.Join(tmpDir, "TestSendLoop222.sock"),
		333: filepath.Join(tmpDir, "TestSendLoop333.sock"),
	}
	serverStates := map[uint64]*serverState{}
	clientStates := map[uint64]map[uint64]*clientState{}

	// Start all servers
	for serverID, serverAddr := range peers {
		ss, err := prepareServerState(serverAddr)
		if err != nil {
			t.Fatal(err)
		}

		serverStates[serverID] = ss
	}

	// for each transport, connect to all servers
	for serverID, ss := range serverStates {
		clientStates[serverID] = map[uint64]*clientState{}
		for peerID, peerAddr := range peers {
			if serverID == peerID {
				continue
			}

			cs, err := prepareClientState(peerAddr)
			if err != nil {
				t.Fatal(err)
			}
			defer cs.stop()

			clientStates[serverID][peerID] = cs
			ss.transport.peerClients[peerID] = cs.stream
		}
	}

	for serverID, ss := range serverStates {
		transport := ss.transport
		raft := ss.transport.raftTransportFacade.(*fakeRaftTransportFacade)

		go raft.recvLoop()
		go transport.sendLoop()

		defer func(serverID uint64, ss *serverState) {
			for _, cs := range clientStates[serverID] {
				cs.stop()
			}
			ss.stop()
			transport.stop()
			raft.stopRecvLoop()
		}(serverID, ss)
	}

	senderTransport := serverStates[111].transport.raftTransportFacade.(*fakeRaftTransportFacade)

	// verifySentMessages verifies that each message in `msgs` was sent within `timeout`.
	verifySentMessages := func(
		r *fakeRaftTransportFacade,
		msgs []raftpb.Message,
		timeout time.Duration,
	) bool {
		for i := 0; i < len(msgs); i++ {
			select {
			case <-time.After(timeout):
				return false
			case r.sendChan <- msgs[i]:
			}
		}
		return true
	}

	complete := verifySentMessages(
		senderTransport,
		[]raftpb.Message{
			raftpb.Message{Recipient: 222},
			raftpb.Message{Recipient: 333},
		},
		500*time.Millisecond,
	)
	if !complete {
		t.Error("did not send out all messages out in time")
	}
}
