package raft

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	"github.com/ulysseses/raft/raftpb"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

var (
	// ErrNoRecipient is emitted when the raft message has no recipient.
	ErrNoRecipient = fmt.Errorf("no recipient")
	// ErrNoSender is emitted when the raft message has no sender.
	ErrNoSender = fmt.Errorf("no sender")
)

type transportConfiguration struct {
	id                     uint64
	peerAddresses          map[uint64]string
	msgBufferSize          int
	dialTimeout            time.Duration
	connectionAttemptDelay time.Duration
	serverOptions          []grpc.ServerOption
	dialOptions            []grpc.DialOption
	callOptions            []grpc.CallOption
	tickerOptions          []TickerOption
}

// transport interacts with the network via the raft protocol.
type transport struct {
	raftpb.UnimplementedRaftProtocolServer
	config      transportConfiguration
	lis         net.Listener
	grpcServer  *grpc.Server
	peerClients map[uint64]*peerClient
	recvChan    chan raftpb.Message
	sendChan    chan raftpb.Message
	sendErrChan chan error
	stopChan    chan struct{}
	stopErrChan chan error

	logger        *zap.Logger
	sugaredLogger *zap.SugaredLogger
}

func (t *transport) logMsg(txt string, msg raftpb.Message) {
	switch msg.Type {
	case raftpb.MsgApp:
		t.logger.Info(txt, msgAppZapFields(msg)...)
	case raftpb.MsgAppResp:
		t.logger.Info(txt, msgAppRespZapFields(msg)...)
	case raftpb.MsgPing:
		t.logger.Info(txt, msgPingZapFields(msg)...)
	case raftpb.MsgPong:
		t.logger.Info(txt, msgPongZapFields(msg)...)
	case raftpb.MsgProp:
		t.logger.Info(txt, msgPropZapFields(msg)...)
	case raftpb.MsgPropResp:
		t.logger.Info(txt, msgPropRespZapFields(msg)...)
	case raftpb.MsgVote:
		t.logger.Info(txt, msgVoteZapFields(msg)...)
	case raftpb.MsgVoteResp:
		t.logger.Info(txt, msgVoteRespZapFields(msg)...)
	}
}

func (t *transport) logRecvMsg(msg raftpb.Message) {
	t.logMsg("received msg", msg)
}

func (t *transport) logSendMsg(msg raftpb.Message) {
	t.logMsg("sent msg", msg)
}

func (t *transport) logDropRecvMsg(msg raftpb.Message) {
	t.logMsg("dropped received msg", msg)
}

func (t *transport) logDropSendMsg(msg raftpb.Message) {
	t.logMsg("dropped sent msg", msg)
}

// Communicate loops receiving incoming raft protocol messages from the network.
func (t *transport) Communicate(stream raftpb.RaftProtocol_CommunicateServer) error {
	var (
		recvChan chan<- raftpb.Message = t.recvChan
	)
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if msg.From == 0 || msg.To == 0 || msg.From == msg.To {
			return fmt.Errorf("receiving with bogus recipient/sender! %v", msg.String())
		}
		select {
		case <-t.stopChan:
			return nil
		case recvChan <- *msg:
			t.logRecvMsg(*msg)
		default:
			t.logDropRecvMsg(*msg)
		}
	}
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
			if msg.From == 0 || msg.To == 0 || msg.From == msg.To {
				return fmt.Errorf("sending with bogus recipient/sender! %v", msg.String())
			}

			pc := t.peerClients[msg.To]
			pc.Lock()
			if pc.closed {
				pc.Unlock()
				t.logDropSendMsg(msg)
				continue
			}
			client := t.peerClients[msg.To].communicateClient
			// TODO(ulysseses): cleanup hard-coded timeout
			select {
			case t.sendErrChan <- client.Send(&msg):
				t.logSendMsg(msg)
			case <-time.After(time.Second):
				t.logDropSendMsg(msg)
			}
			pc.Unlock()

			select {
			case err := <-t.sendErrChan:
				if err == io.EOF {
					pc.Lock()
					if !pc.closed {
						pc.connCloser()
					}
					pc.closed = true
					pc.Unlock()
					go t.attemptConnectionUntilSuccess(pc, t.config.peerAddresses[pc.id])
				} else if err != nil {
					return err
				}
			default:
			}
		}
	}
}

func (t *transport) initGRPCServer() error {
	tokens := strings.Split(t.config.peerAddresses[t.config.id], "://")
	if len(tokens) == 2 {
		lis, err := net.Listen(tokens[0], tokens[1])
		if err != nil {
			return err
		}
		t.lis = lis
		t.grpcServer = grpc.NewServer(t.config.serverOptions...)
		raftpb.RegisterRaftProtocolServer(t.grpcServer, t)

		// start Communicate RPC
		go func() {
			err := t.grpcServer.Serve(t.lis)
			if err != nil {
				t.logger.Error(
					"gRPC serve ended with error",
					zap.Uint64("id", t.config.id),
					zap.Error(err))
			}
			t.stopErrChan <- err
		}()
	} else {
		err := fmt.Errorf(
			"address for peer %d needs to be in {network}://{address} format. Got: %s",
			t.config.id, t.config.peerAddresses[t.config.id])
		return err
	}
	t.logger.Info("started gRPC server", zap.String("addr", t.config.peerAddresses[t.config.id]))
	return nil
}

func (t *transport) initGRPCClients() error {
	t.peerClients = map[uint64]*peerClient{}

	for id, addr := range t.config.peerAddresses {
		if id == t.config.id {
			continue
		}

		pc := peerClient{id: id}
		t.attemptConnectionUntilSuccess(&pc, addr)
		t.peerClients[id] = &pc
	}
	t.logger.Info("connected to all peers", zap.Uint64("id", t.config.id))
	return nil
}

func (t *transport) attemptConnectionUntilSuccess(pc *peerClient, addr string) {
	ticker := time.NewTicker(t.config.connectionAttemptDelay)
	for attempt := 1; ; attempt++ {
		<-ticker.C

		tokens := strings.Split(addr, "://")
		if len(tokens) == 2 {
			if tokens[0] == "tcp" {
				addr = tokens[1]
			}
		} else if len(tokens) != 1 {
			t.sugaredLogger.Fatalf("unrecognized address: %s", addr)
		}

		ctx, cancel := context.WithTimeout(context.Background(), t.config.dialTimeout)
		conn, err := grpc.DialContext(ctx, addr, t.config.dialOptions...)
		cancel()
		if err != nil {
			t.logger.Error(
				"failed to connect to peer",
				zap.Int("attempt", attempt),
				zap.Uint64("peerID", pc.id),
				zap.Error(err))
			continue
		}
		client := raftpb.NewRaftProtocolClient(conn)
		stream, err := client.Communicate(context.Background(), t.config.callOptions...)
		if err != nil {
			t.logger.Error(
				"failed to connect to peer",
				zap.Int("attempt", attempt),
				zap.Uint64("peerID", pc.id),
				zap.Error(err))
			continue
		}
		pc.Lock()
		pc.communicateClient = stream
		pc.connCloser = conn.Close
		pc.closed = false
		pc.Unlock()
		break
	}

	ticker.Stop()
	select {
	case <-ticker.C:
	default:
	}
	return
}

func (t *transport) start() error {
	// Initiate gRPC
	if err := t.initGRPCServer(); err != nil {
		return err
	}
	if err := t.initGRPCClients(); err != nil {
		return err
	}

	// start sendLoop
	go func() {
		err := t.sendLoop()
		if err != nil {
			t.logger.Error(
				"transport sendLoop ended with error",
				zap.Uint64("id", t.config.id),
				zap.Error(err))
		}
		t.stopErrChan <- err
	}()
	return nil
}

// Stop stops the send and receive loop.
func (t *transport) stop() error {
	t.stopChan <- struct{}{}
	t.stopChan <- struct{}{}
	t.grpcServer.Stop()
	var result *multierror.Error
	if err := t.lis.Close(); err != nil {
		result = multierror.Append(result, err)
	}
	for _, pc := range t.peerClients {
		if pc.closed {
			continue
		}
		if err := pc.connCloser(); err != nil {
			result = multierror.Append(result, err)
		}
	}
	for i := 0; i < 2; i++ {
		result = multierror.Append(result, <-t.stopErrChan)
	}
	return result.ErrorOrNil()
}

// newTransport constructs a new `transport` from `Configuration`.
// Remember to connect this to a `raftStateMachine` via `bind`.
func newTransport(c Configuration) (*transport, error) {
	if c.MsgBufferSize < len(c.PeerAddresses)-1 {
		return nil, fmt.Errorf(
			"MsgBufferSize (%d) is too small; it must be at least %d",
			c.MsgBufferSize, len(c.PeerAddresses)-1)
	}
	var (
		serverOptions = []grpc.ServerOption{}
		dialOptions   = []grpc.DialOption{}
		callOptions   = []grpc.CallOption{}
	)
	for _, opt := range c.GRPCOptions {
		switch opt.(type) {
		case WithGRPCServerOption:
			serverOptions = append(serverOptions, opt.(WithGRPCServerOption).Opt)
		case WithGRPCDialOption:
			dialOptions = append(dialOptions, opt.(WithGRPCDialOption).Opt)
		case WithGRPCCallOption:
			callOptions = append(callOptions, opt.(WithGRPCCallOption).Opt)
		default:
			return nil, fmt.Errorf("unknown GRPCOption: %v", opt)
		}
	}

	t := transport{
		config: transportConfiguration{
			id:                     c.ID,
			peerAddresses:          c.PeerAddresses,
			msgBufferSize:          c.MsgBufferSize,
			dialTimeout:            c.DialTimeout,
			connectionAttemptDelay: c.ConnectionAttemptDelay,
			serverOptions:          serverOptions,
			dialOptions:            dialOptions,
			callOptions:            callOptions,
		},
		recvChan:      make(chan raftpb.Message, c.MsgBufferSize),
		sendChan:      make(chan raftpb.Message, c.MsgBufferSize),
		sendErrChan:   make(chan error, 1),
		stopChan:      make(chan struct{}),
		stopErrChan:   make(chan error),
		logger:        c.Logger,
		sugaredLogger: c.Logger.Sugar(),
	}
	return &t, nil
}

type peerClient struct {
	_padding0 [64]byte
	sync.Mutex
	communicateClient raftpb.RaftProtocol_CommunicateClient
	connCloser        func() error
	closed            bool
	_padding1         [64]byte
	id                uint64
}
