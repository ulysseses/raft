package raft

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/ulysseses/raft/raftpb"
)

// Node is a Raft node that interafts with an Application state machine and network.
type Node struct {
	psm *protocolStateMachine
	tr  *transport

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
	return n.propose(ctx, data)
}

// Read should be called by the client/application.
func (n *Node) Read(ctx context.Context) error {
	readIndex, err := n.read(ctx)
	if err != nil {
		return err
	}
	return n.applyTo(ctx, readIndex)
}

// State returns the latest known state of the Raft node.
func (n *Node) State() State {
	n.psm.stateReqChan <- stateReq{}
	return <-n.psm.stateRespChan
}

// Members returns the latest member states.
func (n *Node) Members() map[uint64]MemberState {
	n.psm.membersReqChan <- membersRequest{}
	return <-n.psm.membersRespChan
}

// propose proposes data to the raft log.
func (n *Node) propose(ctx context.Context, data []byte) error {
	n.proposeMu.Lock()
	done := ctx.Done()

	// drain
	select {
	case <-n.psm.propRespChan:
	case <-done:
		n.proposeMu.Unlock()
		return ctx.Err()
	default:
	}

	// send proposal request
	select {
	case n.psm.propReqChan <- proposalRequest{data: data}:
	case <-done:
		n.proposeMu.Unlock()
		return ctx.Err()
	}

	// get proposal response
	select {
	case _ = <-n.psm.propRespChan:
		n.proposeMu.Unlock()
		return nil
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
func (n *Node) read(ctx context.Context) (uint64, error) {
	n.readMu.Lock()
	done := ctx.Done()

	// drain
	select {
	case <-n.psm.readRespChan:
	case <-done:
		n.readMu.Unlock()
		return 0, ctx.Err()
	default:
	}

	// send read request
	select {
	case n.psm.readReqChan <- readRequest{}:
	case <-done:
		n.readMu.Unlock()
		return 0, ctx.Err()
	}

	// wait for response
	select {
	case resp := <-n.psm.readRespChan:
		n.readMu.Unlock()
		return resp.index, nil
	case <-done:
		n.readMu.Unlock()
		return 0, ctx.Err()
	}
}

func (n *Node) applyTo(ctx context.Context, index uint64) error {
	n.psm.log.Lock()
	err := n.applyFunc(n.psm.log.entries(n.applied+1, index))
	n.psm.log.Unlock()
	return err
}

func (n *Node) runApplication() error {
	var commitChan <-chan uint64 = n.psm.commitChan
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
func (n *Node) Start() {
	n.tr.start()
	n.psm.start()
	go func() {
		err := n.runApplication()
		if err != nil {
			n.logger.Error("runApplication ended with error", zap.Error(err))
		}
		n.stopAppErrChan <- err
	}()
}

// Stop stops the Raft node.
func (n *Node) Stop() error {
	n.psm.stop()
	n.tr.stop()
	n.stopAppChan <- struct{}{}
	return <-n.stopAppErrChan
}

// Application applies the committed raft entries. Applications interfacing with
// the Raft node must implement this interface.
type Application interface {
	Apply(entries []raftpb.Entry) error
}
