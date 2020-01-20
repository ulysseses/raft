package raft

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/ulysseses/raft/raftpb"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// transport is resposible for network interaction of the Raft cluster.
type transport struct {
	raftpb.UnimplementedRaftProtocolServer
	lis        net.Listener
	grpcServer *grpc.Server
	peers      map[uint64]*peer

	recvChan chan raftpb.Message
	sendChan chan raftpb.Message
	stopChan chan struct{}

	logger *zap.Logger
	debug  bool
}

// start starts the node's gRPC server and clients to all other peer servers.
func (t *transport) start() {
	// start Communicate RPC
	if t.l() {
		t.logger.Info("starting gRPC server")
	}
	go func() {
		err := t.grpcServer.Serve(t.lis)
		if err != nil && t.l() {
			t.logger.Error("gRPC serve ended with error", zap.Error(err))
		}
	}()

	// connect to peers' RaftProtocolServers
	done := make(chan struct{})
	for _, p := range t.peers {
		go func(p *peer) {
			p.connectLoop()
			done <- struct{}{}
		}(p)
	}
	for range t.peers {
		<-done
	}
	for _, p := range t.peers {
		go p.loop()
	}
	if t.l() {
		t.logger.Info("connected to all peers")
	}

	// start sendLoop
	go t.sendLoop()
}

// stop stops the Raft node's gRPC server and clients to peer servers.
func (t *transport) stop() {
	// Stop Communicate RPC and sendLoop
	t.grpcServer.Stop()
	for i := 1; i <= 2; i++ {
		t.stopChan <- struct{}{}
	}
	// Close connections to peers.
	for _, p := range t.peers {
		p.stop()
	}
}

// Communicate implements RaftProtocolServer for Transport.
func (t *transport) Communicate(stream raftpb.RaftProtocol_CommunicateServer) error {
	var (
		recvChan chan<- raftpb.Message = t.recvChan
		stopChan <-chan struct{}       = t.stopChan
	)
	for {
		msgPtr, err := stream.Recv()
		if err != nil {
			return err
		}
		if t.debug && t.l() {
			t.logger.Debug(
				"received message",
				zap.String("type", msgPtr.Type.String()), zap.Uint64("from", msgPtr.From))
		}
		select {
		case recvChan <- *msgPtr:
		case <-stopChan:
			return nil
		}
	}
}

func (t *transport) sendLoop() {
	var (
		msg      raftpb.Message
		stopChan <-chan struct{} = t.stopChan
	)
	for {
		select {
		case <-stopChan:
			return
		default:
		}

		msg = <-t.sendChan
		if p, ok := t.peers[msg.To]; ok {
			select {
			case p.sendChan <- msg:
			default:
				if t.l() {
					t.logger.Info(
						"could not send",
						zap.Uint64("to", msg.To), zap.String("type", msg.Type.String()))
				}
			}
		} else {
			panic(fmt.Sprintf("unknown recipient: %d", msg.To))
		}
	}
}

func (t *transport) l() bool {
	return t.logger != nil
}

type peer struct {
	stopChan    chan struct{}
	stopErrChan chan error
	sendChan    chan raftpb.Message
	stream      raftpb.RaftProtocol_CommunicateClient
	connCloser  func() error

	id   uint64
	addr string

	reconnectDelay time.Duration
	dialTimeout    time.Duration
	dialOptions    []grpc.DialOption
	callOptions    []grpc.CallOption

	logger *zap.Logger
	debug  bool
}

func (p *peer) loop() {
	var (
		sendChan <-chan raftpb.Message = p.sendChan
		stopChan <-chan struct{}       = p.stopChan
		msg      raftpb.Message
	)
	for {
		// alive loop
		for {
			select {
			case <-stopChan:
				return
			case msg = <-sendChan:
			}
			if p.debug && p.l() {
				p.logger.Debug(
					"sending msg",
					zap.String("type", msg.Type.String()), zap.Uint64("to", msg.To))
			}
			if err := p.stream.Send(&msg); err != nil && p.l() {
				p.logger.Info("stream send failed", zap.Error(err))
				break
			}
		}

		// connect loop
		p.connectLoop()
	}
}

func (p *peer) connectLoop() {
	var stopChan <-chan struct{} = p.stopChan

	if p.connCloser != nil {
		if err := p.connCloser(); err != nil && p.l() {
			p.logger.Error("error from closing connection", zap.Error(err))
		}
		p.connCloser = nil
	}
	for {
		select {
		case <-stopChan:
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), p.dialTimeout)
		conn, err := grpc.DialContext(ctx, p.addr, p.dialOptions...)
		cancel()
		if err != nil {
			if p.debug && p.l() {
				p.logger.Debug("will retry failed connection", zap.Error(err))
			}
			time.Sleep(p.reconnectDelay)
			continue
		}
		p.connCloser = conn.Close
		p.stream, err = raftpb.NewRaftProtocolClient(conn).
			Communicate(context.Background(), p.callOptions...)
		if err != nil {
			panic(err)
		}

		break
	}
}

func (p *peer) stop() {
	p.stopChan <- struct{}{}
	if err := p.connCloser(); err != nil && p.l() {
		p.logger.Warn(
			"error when closing connection to peer",
			zap.String("addr", p.addr), zap.Error(err))
	}
}

func (p *peer) l() bool {
	return p.logger != nil
}

func listen(target string) (net.Listener, error) {
	tokens := strings.Split(target, "://")
	if len(tokens) == 2 {
		return net.Listen(tokens[0], tokens[1])
	}
	return nil, fmt.Errorf("target must be in {net}://{addr} format. Got: %s", target)
}
