package raft

import (
	"fmt"
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

	// HeartbeatTicks is the number of tick periods before a heartbeat
	// should fire.
	// MinElectionTicks is the minimum number of tick periods before an
	// election timeout should fire.
	// MaxElectionTicks is the maximum number of tick periods before an
	// election timeout should fire.
	HeartbeatTicks, MinElectionTicks, MaxElectionTicks uint

	// Consistency is the consistency level to use for the Raft cluster.
	Consistency Consistency

	// MsgBufferSize is max the number of Raft protocol messages allowed to be
	// buffered before the Raft node can process/send them out.
	MsgBufferSize int

	// DialTimeout is the timeout for dialing to peers.
	// ConnectionAttemptDelay is the duration to wait per connection attempt.
	// SendTimeout is the timeout for sending to peers.
	DialTimeout, ConnectionAttemptDelay, SendTimeout time.Duration

	// GRPCOptions is an optional list of GRPCOption to apply to configure gRPC.
	GRPCOptions []GRPCOption

	// TickerOptions is an optional list of TickerOption to apply to configure
	// the ticker.
	TickerOptions []TickerOption

	// Logger.
	Logger *zap.Logger

	// Debug instructs the logger to emit Debug-level logs.
	Debug bool
}

// Verify verifies the configuration.
func (c Configuration) Verify() error {
	if c.ID == 0 {
		return fmt.Errorf("ID cannot be 0")
	}
	if _, ok := c.PeerAddresses[c.ID]; !ok {
		return fmt.Errorf("no address correponding to peerID: %d", c.ID)
	}
	if c.MinElectionTicks == 0 {
		return fmt.Errorf("MinElectionTicks cannot be 0")
	}
	if c.MaxElectionTicks < c.MinElectionTicks {
		return fmt.Errorf("MaxElectionTicks cannot be less than MinElectionTicks")
	}
	if c.HeartbeatTicks == 0 {
		return fmt.Errorf("HeartbeatTicks cannot be 0")
	}
	if c.HeartbeatTicks >= c.MinElectionTicks {
		return fmt.Errorf("HeartbeatTicks cannot be greater than or equal to MinElectionTicks")
	}
	if c.DialTimeout == 0 {
		return fmt.Errorf("DialTimeout cannot be 0")
	}
	if c.ConnectionAttemptDelay == 0 {
		return fmt.Errorf("ConnectionAttemptDelay cannot be 0")
	}
	if c.SendTimeout == 0 {
		return fmt.Errorf("SendTimeout cannot be 0")
	}
	if c.Logger == nil {
		return fmt.Errorf("must provide a Logger")
	}
	return nil
}

// BuildSensibleConfiguration builds a sensible Configuration.
func BuildSensibleConfiguration(
	id uint64,
	peerAddresses map[uint64]string,
	consistency Consistency,
	logger *zap.Logger,
) (Configuration, error) {
	if logger == nil {
		var err error
		logger, err = zap.NewProduction()
		if err != nil {
			return Configuration{}, err
		}
	}
	config := Configuration{
		ID:                     id,
		PeerAddresses:          peerAddresses,
		TickPeriod:             100 * time.Millisecond,
		HeartbeatTicks:         1,
		MinElectionTicks:       10,
		MaxElectionTicks:       20,
		Consistency:            consistency,
		MsgBufferSize:          (len(peerAddresses) - 1) * 3,
		DialTimeout:            100 * time.Millisecond,
		ConnectionAttemptDelay: 100 * time.Millisecond,
		SendTimeout:            100 * time.Millisecond,
		GRPCOptions:            []GRPCOption{WithGRPCDialOption{Opt: grpc.WithInsecure()}},
		Logger:                 logger.With(zap.Uint64("id", id)),
	}
	err := config.Verify()
	return config, err
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
