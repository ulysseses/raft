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
	recvChan chan *raftpb.Message
	sendChan chan *raftpb.Message
}

// recv implements raftTransportFacade interface.
func (r *fakeRaftTransportFacade) recv() chan<- *raftpb.Message {
	return r.recvChan
}

// send implements raftTransportFacade interface.
func (r *fakeRaftTransportFacade) send() <-chan *raftpb.Message {
	return r.sendChan
}

// recvLoop receives messages from transport but does nothing with them.
func (r *fakeRaftTransportFacade) recvLoop() {
	for range r.recvChan {
	}
}

// stopRecvLoop stops recvLoop goroutine.
func (r *fakeRaftTransportFacade) stopRecvLoop() {
	close(r.recvChan)
}

func newFakeRaftTransportFacade() *fakeRaftTransportFacade {
	r := &fakeRaftTransportFacade{
		recvChan: make(chan *raftpb.Message),
		sendChan: make(chan *raftpb.Message),
	}
	return r
}

func TestCommunicateWithPeer(t *testing.T) {
	fakeRaft := newFakeRaftTransportFacade()
	transport := &transport{
		raftTransportFacade: fakeRaft,
	}

	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tmpUnixSocketPath := filepath.Join(tmpDir, "TestCommunicateWithPeer.sock")
	lis, err := net.Listen("unix", tmpUnixSocketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	grpcServer := grpc.NewServer()
	raftpb.RegisterRaftServiceServer(grpcServer, transport)
	go grpcServer.Serve(lis)
	defer grpcServer.Stop()

	conn, err := grpc.Dial(
		fmt.Sprintf("unix://%s", tmpUnixSocketPath),
		grpc.WithInsecure(),
		grpc.WithBlock())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	client := raftpb.NewRaftServiceClient(conn)
	stream, err := client.CommunicateWithPeer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	msgs := []*raftpb.Message{
		&raftpb.Message{},
		&raftpb.Message{},
		&raftpb.Message{},
	}
	go func() {
		for _, msg := range msgs {
			if err := stream.Send(msg); err != nil {
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
				if !proto.Equal(wantMsgs[i], gotMsg) {
					return false
				}
			}
		}
		return true
	}

	complete := verifyReceivedMessages(fakeRaft, msgs, 500*time.Millisecond)
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
	transports := map[uint64]*transport{}

	// Start all servers
	for serverID, serverAddr := range peers {
		lis, err := net.Listen("unix", serverAddr)
		if err != nil {
			t.Fatal(err)
		}
		defer lis.Close()

		fakeRaft := newFakeRaftTransportFacade()
		defer fakeRaft.stopRecvLoop()
		go fakeRaft.recvLoop()

		transport := &transport{
			raftTransportFacade: fakeRaft,
			stopChan:            make(chan struct{}),
			peerClients:         map[uint64]raftpb.RaftService_CommunicateWithPeerClient{},
		}

		grpcServer := grpc.NewServer()
		raftpb.RegisterRaftServiceServer(grpcServer, transport)
		go grpcServer.Serve(lis)
		defer grpcServer.Stop()

		transports[serverID] = transport
	}

	// for each transport, connect to all servers
	for serverID, transport := range transports {
		for peerID, peerAddr := range peers {
			if serverID == peerID {
				continue
			}

			conn, err := grpc.Dial(
				fmt.Sprintf("unix://%s", peerAddr),
				grpc.WithInsecure(),
				grpc.WithBlock())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()

			client := raftpb.NewRaftServiceClient(conn)
			stream, err := client.CommunicateWithPeer(context.Background())
			if err != nil {
				t.Fatal(err)
			}

			transport.peerClients[peerID] = stream
		}
	}

	for _, transport := range transports {
		go transport.sendLoop()
		defer transport.stop()
	}

	senderTransport := transports[111].raftTransportFacade.(*fakeRaftTransportFacade)

	// verifySentMessages verifies that each message in `msgs` was sent within `timeout`.
	verifySentMessages := func(
		r *fakeRaftTransportFacade,
		msgs []*raftpb.Message,
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
		[]*raftpb.Message{
			&raftpb.Message{Recipient: 222},
			&raftpb.Message{Recipient: 333},
		},
		500*time.Millisecond,
	)
	if !complete {
		t.Error("did not send out all messages out in time")
	}
}
