package raft

import (
	"time"

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
	// MsgBufferSize is the number of Raft protocol messages allowed to be
	// buffered before the Raft node can process/send them out.
	MsgBufferSize int
	// GRPCOptions is an optional list of GRPCOptions to apply to configure
	// gRPC.
	GRPCOptions []GRPCOption
	// TickerOptions is an optional list of TickerOptions to apply to configure
	// the ticker.
	TickerOptions []TickerOption
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
	opt grpc.ServerOption
}

func (WithGRPCServerOption) isGRPCOption() {}

// WithGRPCDialOption implements GRPCOption and adds a `grpc.DialOption`.
type WithGRPCDialOption struct {
	opt grpc.DialOption
}

func (WithGRPCDialOption) isGRPCOption() {}

// WithGRPCCallOption implements GRPCOption and adds a `grpc.CallOption`.
type WithGRPCCallOption struct {
	opt grpc.CallOption
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
