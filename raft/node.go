package raft

import (
	"context"
	"sync"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	"go.uber.org/zap"

	"github.com/ulysseses/raft/raftpb"
)

// Node is a Raft node that interafts with an Application state machine and network.
type Node struct {
	r *raftStateMachine
	t *transport

	_padding0 [64]byte
	proposeMu sync.Mutex
	_padding1 [64]byte
	readMu    sync.Mutex
	_padding2 [64]byte

	applied        uint64
	applyFunc      func([]raftpb.Entry) error
	stopAppChan    chan struct{}
	stopAppErrChan chan error

	logger        *zap.Logger
	sugaredLogger *zap.SugaredLogger
}

// Propose should be called by the client/application.
func (n *Node) Propose(ctx context.Context, data []byte) error {
	return n.propose(ctx, time.Now().UnixNano(), data)
}

// Read should be called by the client/application.
func (n *Node) Read(ctx context.Context) error {
	readIndex, err := n.read(ctx, time.Now().UnixNano())
	if err != nil {
		return err
	}
	return n.applyTo(ctx, readIndex)
}

// State returns the latest known state of the Raft node.
func (n *Node) State() raftpb.State {
	n.r.stateReqChan <- stateReq{}
	return <-n.r.stateRespChan
}

// Peers returns the latest state of this node's peers.
func (n *Node) Peers() map[uint64]Peer {
	n.r.peerReqChan <- peerRequest{}
	return <-n.r.peerRespChan
}

// propose proposes data to the raft log.
// ConsistencySerializable: propose blocks until the leader added proposal to log.
//   * ErrDroppedProposal is returned when leader no longer is leader and thus dropped proposal.
// ConsistencyLinearizable: propose blocks until the quorum has committed the proposal.
//   * ErrDroppedProposal is returned when leader no longer is leader and thus dropped proposal.
func (n *Node) propose(ctx context.Context, unixNano int64, data []byte) error {
	n.proposeMu.Lock()
	done := ctx.Done()
	select {
	case n.r.propReqChan <- proposalRequest{unixNano: unixNano, data: data}:
	case <-done:
		n.proposeMu.Unlock()
		return ctx.Err()
	}

	select {
	case result := <-n.r.propRespChan:
		n.proposeMu.Unlock()
		return result.err
	case <-done:
		n.proposeMu.Unlock()
		return ctx.Err()
	}
}

// read sends a read-only request to the raft cluster. It returns the committed index that the
// application needs to catch up to.
// ConsistencySerializable: No read request is sent to the raft cluster. read is immediately
//   acknowledged.
// ConsistencyLinearizable: A read request is sent to each raft node. read is acknowledged once
//   a quorum has acknowledged the read request.
func (n *Node) read(ctx context.Context, unixNano int64) (uint64, error) {
	n.readMu.Lock()
	done := ctx.Done()
	select {
	case n.r.readReqChan <- readRequest{unixNano: unixNano}:
	case <-done:
		n.readMu.Unlock()
		return 0, ctx.Err()
	}

	select {
	case readResult := <-n.r.readRespChan:
		n.readMu.Unlock()
		return readResult.index, readResult.err
	case <-done:
		n.readMu.Unlock()
		return 0, ctx.Err()
	}
}

func (n *Node) applyTo(ctx context.Context, index uint64) error {
	n.r.log.Lock()
	err := n.applyFunc(n.r.log.entries(n.applied+1, index))
	n.r.log.Unlock()
	return err
}

func (n *Node) runApplication() error {
	var commitChan <-chan uint64 = n.r.commitChan
	for {
		select {
		case <-n.stopAppChan:
			return nil
		case commit := <-commitChan:
			if err := n.applyTo(context.Background(), commit); err != nil {
				return err
			}
		}
	}
}

// Start starts the Raft node.
func (n *Node) Start() error {
	if err := n.t.start(); err != nil {
		return err
	}
	if err := n.r.start(); err != nil {
		return err
	}
	go func() {
		err := n.runApplication()
		if err != nil {
			n.logger.Error("runApplication ended with error", zap.Error(err))
		}
		n.stopAppErrChan <- err
	}()
	return nil
}

// Stop stops the Raft node.
func (n *Node) Stop() error {
	var result *multierror.Error
	if err := n.r.stop(); err != nil {
		result = multierror.Append(result, err)
	}
	if err := n.t.stop(); err != nil {
		result = multierror.Append(result, err)
	}
	n.stopAppChan <- struct{}{}
	result = multierror.Append(result, <-n.stopAppErrChan)
	return result.ErrorOrNil()
}

// NewNode constructs a new Raft Node from Configuration.
func NewNode(c Configuration, a Application) (*Node, error) {
	// Initialize transport
	t, err := newTransport(c)
	if err != nil {
		return nil, err
	}

	// Initialize raftStateMachine
	r, err := newRaftStateMachine(c, t.recvChan, t.sendChan)
	if err != nil {
		return nil, err
	}

	n := Node{
		r:              r,
		t:              t,
		stopAppChan:    make(chan struct{}),
		stopAppErrChan: make(chan error),
		logger:         c.Logger,
		sugaredLogger:  c.Logger.Sugar(),
	}

	// Attach Application to Node
	n.applyFunc = a.Apply

	return &n, nil
}

// Application applies the committed raft entries. Applications interfacing with
// the Raft node must implement this interface.
type Application interface {
	Apply(entries []raftpb.Entry) error
}
