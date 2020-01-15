package raft

import (
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// Configuration configures the Raft system.
type Configuration struct {
	// ID of the Raft node to configure.
	ID uint64

	// PeerAddresses maps all Raft nodes from ID to address.
	PeerAddresses map[uint64]string

	// TickPeriod is the period of time at which the ticker should fire.
	TickPeriod time.Duration

	// MinElectionTicks is the minimum number of tick periods before an
	// election timeout should fire.
	// MaxElectionTIcks is the maximum number of tick periods before an
	// election timeout should fire.
	// HeartbeatTicks is the number of tick periods before a heartbeat
	// should fire.
	MinElectionTicks, MaxElectionTicks, HeartbeatTicks uint

	// Consistency is the consistency level to use for the Raft cluster.
	Consistency Consistency

	// MsgBufferSize is max the number of Raft protocol messages allowed to be
	// buffered before the Raft node can process/send them out.
	MsgBufferSize int

	// DialTimeout is the timeout for dialing to peers.
	DialTimeout time.Duration

	// ConnectionAttemptDelay is the duration to wait per connection attempt.
	ConnectionAttemptDelay time.Duration

	// SendTimeout is the timeout for sending to peers.
	SendTimeout time.Duration

	// GRPCOptions is an optional list of GRPCOptions to apply to configure gRPC.
	GRPCOptions []GRPCOption

	// TickerOptions is an optional list of TickerOptions to apply to configure
	// the ticker.
	TickerOptions []TickerOption

	// Logger.
	Logger *zap.Logger

	// Debug instructs the logger to emit Debug-level logs.
	Debug bool
}

// Consistency is the consistency mode that Raft operations should support.
type Consistency uint8

const (
	// ConsistencySerializable follows the serializable consistency model.
	ConsistencySerializable Consistency = iota
	// ConsistencyLinearizable follows the linearizable consistency model.
	ConsistencyLinearizable
)

// GRPCOption configures how we setup gRPC.
type GRPCOption interface{ isGRPCOption() }

// WithGRPCServerOption implements GRPCOption and adds a `grpc.ServerOption`.
type WithGRPCServerOption struct {
	Opt grpc.ServerOption
}

func (WithGRPCServerOption) isGRPCOption() {}

// WithGRPCDialOption implements GRPCOption and adds a `grpc.DialOption`.
type WithGRPCDialOption struct {
	Opt grpc.DialOption
}

func (WithGRPCDialOption) isGRPCOption() {}

// WithGRPCCallOption implements GRPCOption and adds a `grpc.CallOption`.
type WithGRPCCallOption struct {
	Opt grpc.CallOption
}

func (WithGRPCCallOption) isGRPCOption() {}

// TickerOption configures the ticker
type TickerOption interface{ isTickerOption() }

// WithHeartbeatTicker configures raft to utilize `c` to emit heartbeat ticks.
func WithHeartbeatTicker(t Ticker) TickerOption {
	return withHeartbeatTickerTickerOption{
		t: t,
	}
}

type withHeartbeatTickerTickerOption struct {
	t Ticker
}

func (withHeartbeatTickerTickerOption) isTickerOption() {}

// WithElectionTicker configures raft to utilize `c` to emit election timeout ticks.
func WithElectionTicker(t Ticker) TickerOption {
	return withElectionTickerTickerOption{
		t: t,
	}
}

type withElectionTickerTickerOption struct {
	t Ticker
}

func (withElectionTickerTickerOption) isTickerOption() {}

func coalesceLogger(logger *zap.Logger) (*zap.Logger, error) {
	if logger == nil {
		var err error
		logger, err = zap.NewProduction()
		if err != nil {
			return nil, err
		}
	}
	return logger, nil
}
