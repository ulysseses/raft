package raft

import (
	"fmt"
	"sort"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/ulysseses/raft/raftpb"
)

var (
	// ErrDroppedProposal is the error emitted when a proposal got dropped.
	ErrDroppedProposal = fmt.Errorf("dropped proposal")
	// ErrDroppedRead is emitted if the node serving the read has stepped down
	// from leader to follower.
	ErrDroppedRead = fmt.Errorf("dropped read request")
)

type raftStateMachine struct {
	// ticker
	heartbeatTicker Ticker
	electionTicker  Ticker
	heartbeatC      <-chan struct{}

	// network io
	recvChan <-chan raftpb.Message
	sendChan chan<- raftpb.Message

	// proposals
	pendingProposal pendingProposal
	propReqChan     chan proposalRequest
	propRespChan    chan proposalResponse

	// reads
	pendingRead  pendingRead
	readReqChan  chan readRequest
	readRespChan chan readResponse

	// applies
	commitChan chan uint64

	// state requests
	stateReqChan  chan stateReq
	stateRespChan chan raftpb.State

	// peer requests
	peerReqChan  chan peerRequest
	peerRespChan chan map[uint64]Peer

	// raft state
	id             uint64
	consistency    Consistency
	state          raftpb.State
	quorumSize     int
	lastEntryIndex uint64
	lastEntryTerm  uint64
	peers          map[uint64]*Peer

	log                    *raftLog
	quorumMatchIndexBuffer []uint64
	stopChan               chan struct{}
	stopErrChan            chan error

	logger        *zap.Logger
	sugaredLogger *zap.SugaredLogger
}

func (r *raftStateMachine) run() {
	electionTickerC := r.electionTicker.C()
	for {
		select {
		case <-r.stopChan:
			r.stopErrChan <- nil
			return
		case <-r.heartbeatC: // heartbeatC is null when not leader
			r.broadcastApp()
		case <-electionTickerC:
			if r.state.Role == raftpb.RoleLeader {
				// Step down to follower role if could not establish quorum
				if !r.hasQuorumAcks() {
					r.becomeFollower()
				}
				// Reset acks
				for _, p := range r.peers {
					p.Ack = false
				}
			} else {
				r.becomeCandidate()
			}
		case msg := <-r.recvChan:
			if err := r.processMessage(msg); err != nil {
				r.logger.Error("raft run loop errored out", zap.Error(err))
				fmt.Printf("raft run loop errored out: %v", err)
				r.stopErrChan <- err
				return
			}
		case propReq := <-r.propReqChan:
			r.propose(propReq)
		case readReq := <-r.readReqChan:
			r.read(readReq)
		case <-r.stateReqChan:
			r.stateRespChan <- r.state
		case <-r.peerReqChan:
			peers := map[uint64]Peer{}
			for _, p := range r.peers {
				peers[p.ID] = *p
			}
			r.peerRespChan <- peers
		}
	}
}

func (r *raftStateMachine) hasQuorumAcks() bool {
	acks := 0
	for _, p := range r.peers {
		if p.ID == r.id {
			acks++
			continue
		}
		if p.Ack {
			acks++
		}
	}
	return acks >= r.quorumSize
}

func (r *raftStateMachine) processMessage(msg raftpb.Message) error {
	if msg.Term < r.state.Term {
		r.logger.Info(
			"ignoring stale msg",
			zap.Uint64("from", msg.From), zap.String("type", msg.Type.String()))
		return nil
	}
	if msg.Term > r.state.Term {
		r.logger.Info("received msg with higher term", zap.Uint64("msgTerm", msg.Term))
		r.state.Term = msg.Term
		if r.state.Role != raftpb.RoleFollower {
			r.becomeFollower()
		}

		// proposal may already have been committed, but just in case it wasn't...
		if r.pendingProposal.isPending() {
			r.endPendingProposal(ErrDroppedProposal)
		}
		// cancel any pending read requests
		if r.pendingProposal.isPending() {
			r.endPendingRead(ErrDroppedRead)
		}
	}

	switch msg.Type {
	case raftpb.MsgApp:
		msgApp := getApp(msg)
		r.processApp(msgApp)
	case raftpb.MsgAppResp:
		msgAppResp := getAppResp(msg)
		r.processAppResp(msgAppResp)
	case raftpb.MsgPing:
		msgPing := getPing(msg)
		r.processPing(msgPing)
	case raftpb.MsgPong:
		msgPong := getPong(msg)
		r.processPong(msgPong)
	case raftpb.MsgProp:
		msgProp := getProp(msg)
		r.processProp(msgProp)
	case raftpb.MsgPropResp:
		msgPropResp := getPropResp(msg)
		r.processPropResp(msgPropResp)
	case raftpb.MsgVote:
		msgVote := getVote(msg)
		r.processVote(msgVote)
	case raftpb.MsgVoteResp:
		msgVoteResp := getVoteResp(msg)
		r.processVoteResp(msgVoteResp)
	default:
		return fmt.Errorf("unrecognized msg type: %s", msg.Type.String())
	}

	return nil
}

func (r *raftStateMachine) processApp(msg msgApp) {
	r.state.Leader = msg.from
	switch r.state.Role {
	case raftpb.RoleLeader:
		r.becomeFollower()
	case raftpb.RoleCandidate:
		r.becomeFollower()
	}
	r.electionTicker.Reset()

	var largestMatchIndex uint64 = 0
	success := msg.term >= r.state.Term &&
		msg.index <= r.lastEntryIndex &&
		msg.logTerm == r.log.entry(msg.index).Term
	if success {
		r.lastEntryIndex, r.lastEntryTerm = r.log.append(msg.index, msg.entries...)
		largestMatchIndex = r.lastEntryIndex

		newCommit := msg.commit
		if newCommit >= r.lastEntryIndex {
			newCommit = r.lastEntryIndex
		}
		if newCommit > r.state.Commit {
			r.updateCommit(newCommit)
		}
	}

	resp := buildAppResp(r.state.Term, r.id, msg.from, largestMatchIndex)
	select {
	case r.sendChan <- resp:
	default:
	}
}

func (r *raftStateMachine) processAppResp(msg msgAppResp) {
	switch r.state.Role {
	case raftpb.RoleFollower:
		return
	case raftpb.RoleCandidate:
		return
	}

	p := r.peers[msg.from]

	p.Ack = true

	// Update match/next index
	success := msg.index != 0
	if success {
		if msg.index > p.Match {
			p.Match = msg.index
		}
		p.Next = r.lastEntryIndex + 1
	} else {
		p.Next--
		r.logger.Info("decreased next", zap.Uint64("follower", p.ID))
	}

	quorumMatchIndex := r.quorumMatchIndex()
	if r.log.entry(quorumMatchIndex).Term == r.state.Term && quorumMatchIndex > r.state.Commit {
		r.updateCommit(quorumMatchIndex)
	}

	if r.canAckLinearizableProposal() {
		r.endPendingProposal(nil)
	}

	if r.canAckLinearizableRead() {
		r.endPendingRead(nil)
	}
}

func (r *raftStateMachine) processPing(msg msgPing) {
	if msg.from == r.state.Leader {
		switch r.state.Role {
		case raftpb.RoleFollower:
			r.electionTicker.Reset()
		case raftpb.RoleCandidate:
			r.becomeFollower()
		default:
		}
	}

	pong := buildPong(r.state.Term, r.id, msg.from, msg.unixNano, msg.index)
	select {
	case r.sendChan <- pong:
	default:
	}
}

func (r *raftStateMachine) processPong(msg msgPong) {
	if r.state.Role != raftpb.RoleLeader {
		r.logger.Info(
			"ignored MsgPing",
			zap.Uint64("from", msg.from), zap.String("role", r.state.Role.String()))
		return
	}

	r.peers[msg.from].Ack = true

	if r.pendingRead.isPending() && msg.unixNano == r.pendingRead.unixNano && msg.index == r.pendingRead.index {
		r.pendingRead.acks++
	}

	if r.canAckLinearizableRead() {
		r.endPendingRead(nil)
	}
}

func (r *raftStateMachine) processProp(msg msgProp) {
	// if not leader, let proposer know we're not leader
	if r.state.Role != raftpb.RoleLeader {
		resp := buildPropResp(
			r.state.Term, r.id, msg.from,
			msg.unixNano, 0, 0)
		select {
		case r.sendChan <- resp:
		default:
		}
		return
	}

	// If leader, append proposed entry to log. At this point, if ConsistencySerializable, then
	// proposal is successful. If ConsistencyLinearizable, proposal isn't successful until committed.
	entry := raftpb.Entry{
		Index: r.lastEntryIndex + 1,
		Term:  r.state.Term,
		Data:  msg.data,
	}
	r.lastEntryIndex, r.lastEntryTerm = r.log.append(r.lastEntryIndex, entry)

	resp := buildPropResp(
		r.state.Term, r.id, msg.from,
		msg.unixNano, entry.Index, entry.Term)
	select {
	case r.sendChan <- resp:
	default:
	}
}

func (r *raftStateMachine) processPropResp(msg msgPropResp) {
	if !r.pendingProposal.isPending() {
		return
	}

	// check if the proposal response is the one we're looking for (equal unixNano)
	// and if the proposal succeeded (index != 0)
	if msg.unixNano == r.pendingProposal.unixNano && msg.index != 0 {
		// serializable shortcut
		if r.consistency == ConsistencySerializable {
			r.endPendingProposal(nil)
		}
	} else if r.pendingProposal.isPending() {
		r.endPendingProposal(ErrDroppedProposal)
	}
}

func (r *raftStateMachine) processVote(msg msgVote) {
	switch r.state.Role {
	case raftpb.RoleLeader:
		return
	case raftpb.RoleCandidate:
		return
	}
	grantVote := r.state.VotedFor == 0 &&
		(msg.logTerm > r.state.Term || msg.index >= r.lastEntryIndex)
	if grantVote {
		resp := buildVoteResp(r.state.Term, r.id, msg.from)
		select {
		case r.sendChan <- resp:
			r.state.VotedFor = msg.from
			r.logger.Info(
				"voted for candidate",
				zap.Uint64("votedFor", r.state.VotedFor), zap.Uint64("term", r.state.Term))
		default:
		}
	}
}

func (r *raftStateMachine) processVoteResp(msg msgVoteResp) {
	r.logger.Info("got vote", zap.Uint64("from", msg.from), zap.Uint64("term", msg.term))
	r.peers[msg.from].VoteGranted = true
	voteCount := 0
	for _, p := range r.peers {
		if p.VoteGranted {
			voteCount++
		}
	}
	if voteCount >= r.quorumSize {
		r.becomeLeader()
	}
}

func (r *raftStateMachine) propose(req proposalRequest) {
	// if not leader, then proxy proposal request to leader
	if r.id != r.state.Leader {
		r.pendingProposal = pendingProposal{unixNano: req.unixNano}
		prop := buildProp(
			r.state.Term, r.id, r.state.Leader,
			req.unixNano, req.data)
		select {
		case r.sendChan <- prop:
		default:
			r.pendingProposal = pendingProposal{}
			r.endPendingProposal(ErrDroppedProposal)
		}
		return
	}

	entry := raftpb.Entry{
		Index: r.lastEntryIndex + 1,
		Term:  r.state.Term,
		Data:  req.data,
	}
	r.pendingProposal = pendingProposal{
		index:    entry.Index,
		term:     entry.Term,
		unixNano: req.unixNano,
	}
	r.lastEntryIndex, r.lastEntryTerm = r.log.append(r.lastEntryIndex, entry)
	// serializable shortcut
	if r.consistency == ConsistencySerializable {
		r.endPendingProposal(nil)
	}
}

func (r *raftStateMachine) read(req readRequest) {
	r.pendingRead = pendingRead{
		index:    r.state.Commit,
		unixNano: req.unixNano,
		acks:     1,
	}

	// serializable shortcut
	if r.consistency == ConsistencySerializable {
		r.endPendingRead(nil)
		return
	}

	r.broadcastPing()
}

func (r *raftStateMachine) becomeFollower() {
	r.logger.Info("becoming follower", zap.Uint64("term", r.state.Term))
	r.heartbeatC = nil
	r.state.Role = raftpb.RoleFollower
	r.state.VotedFor = 0
	for _, p := range r.peers {
		p.VoteGranted = false
	}

	if r.pendingProposal.isPending() {
		r.endPendingProposal(ErrDroppedProposal)
	}
	if r.pendingRead.isPending() {
		r.endPendingRead(ErrDroppedRead)
	}
}

func (r *raftStateMachine) becomeCandidate() {
	r.logger.Info("becoming candidate", zap.Uint64("newTerm", r.state.Term+1))
	r.heartbeatC = nil
	r.state.Role = raftpb.RoleCandidate
	r.state.Leader = 0
	r.state.Term++
	r.state.VotedFor = r.id

	// Send vote requests to other peers
	lastEntry := r.log.entry(r.lastEntryIndex)
	for _, p := range r.peers {
		if r.id == p.ID {
			p.VoteGranted = true
			continue
		}
		p.VoteGranted = false
		req := buildVote(r.state.Term, r.id, p.ID, lastEntry.Index, lastEntry.Term)
		select {
		case r.sendChan <- req:
		default:
			r.logger.Info("did not send MsgVote", zap.Uint64("to", req.To))
		}
	}

	if r.pendingProposal.isPending() {
		r.endPendingProposal(ErrDroppedProposal)
	}
	if r.pendingRead.isPending() {
		r.endPendingRead(ErrDroppedRead)
	}

	// shortcut: 1-node cluster
	if r.quorumSize == 1 {
		r.logger.Info("1-node cluster shortcut: become leader instantly")
		r.becomeLeader()
	}
}

func (r *raftStateMachine) becomeLeader() {
	r.logger.Info("becoming leader")
	r.state.Role = raftpb.RoleLeader
	r.state.Leader = r.id
	r.state.VotedFor = 0
	for _, p := range r.peers {
		p.VoteGranted = false
		p.Next = r.lastEntryIndex + 1
		p.Ack = false
	}

	// Try to commit an (empty) entry from the newly elected term
	r.lastEntryIndex, r.lastEntryTerm = r.log.append(r.lastEntryIndex, raftpb.Entry{
		Index: r.lastEntryIndex + 1,
		Term:  r.state.Term,
	})
	r.broadcastApp()
	r.heartbeatTicker.Reset()
	r.heartbeatC = r.heartbeatTicker.C()
}

func (r *raftStateMachine) broadcastApp() {
	for _, p := range r.peers {
		if r.id == p.ID {
			continue
		}
		entries := []raftpb.Entry{}
		if p.Next <= r.lastEntryIndex {
			r.log.RLock()
			entries = append(entries, r.log.entries(p.Next, r.lastEntryIndex)...)
			r.log.RUnlock()
		}
		prevEntry := r.log.entry(p.Next - 1)
		req := buildApp(r.state.Term, r.id, p.ID, r.state.Commit, entries, prevEntry.Index, prevEntry.Term)
		select {
		case r.sendChan <- req:
		default:
			r.logger.Info("did not send MsgApp", zap.Uint64("to", req.To))
		}
	}
}

func (r *raftStateMachine) broadcastPing() {
	for id := range r.peers {
		if r.id == id {
			continue
		}
		ping := buildPing(r.state.Term, r.id, id, r.pendingRead.unixNano, r.pendingRead.index)
		select {
		case r.sendChan <- ping:
		default:
			r.logger.Info("did not send MsgPing", zap.Uint64("to", ping.To))
		}
	}
}

// Figure out the largest match index of a quorum so far.
func (r *raftStateMachine) quorumMatchIndex() uint64 {
	matches := r.quorumMatchIndexBuffer
	i := 0
	for _, p := range r.peers {
		if r.id == p.ID {
			matches[i] = r.lastEntryIndex
		}
		matches[i] = p.Match
		i++
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i] > matches[j]
	})
	return matches[r.quorumSize-1]
}

// If linearizable consistency, Propose call ends when proposal is committed.
func (r *raftStateMachine) canAckLinearizableProposal() bool {
	return r.consistency == ConsistencyLinearizable &&
		r.pendingProposal.isPending() &&
		r.pendingProposal.index <= r.state.Commit &&
		r.pendingProposal.term <= r.state.Term
}

// If linearizable consistency, Read call ends when read request is acknowledged by a quorum.
func (r *raftStateMachine) canAckLinearizableRead() bool {
	return r.consistency == ConsistencyLinearizable &&
		r.pendingRead.isPending() &&
		r.pendingRead.acks >= r.quorumSize && r.lastEntryTerm == r.state.Term
}

// proposal may already have been committed, but just in case it wasn't...
func (r *raftStateMachine) endPendingProposal(err error) {
	result := proposalResponse{
		err: err,
	}
	select {
	case r.propRespChan <- result:
	default:
		r.logger.Info("could not send proposalResponse")
	}
	// zero out pending proposal
	r.pendingProposal = pendingProposal{}
}

// ack (nil error) or cancel (non-nil error) any pending read requests
func (r *raftStateMachine) endPendingRead(err error) {
	if r.state.Role == raftpb.RoleLeader {
		result := readResponse{
			index: r.pendingRead.index,
			err:   err,
		}
		select {
		case r.readRespChan <- result:
		default:
			r.logger.Info("could not send readResponse")
		}
	}
	// zero out pending read
	r.pendingRead = pendingRead{}
}

// update commit and alert downstream application state machine
func (r *raftStateMachine) updateCommit(newCommit uint64) {
	r.state.Commit = newCommit
	r.commitChan <- newCommit
}

func (r *raftStateMachine) start() error {
	r.logger.Info("starting election timeout ticker")
	r.electionTicker.Start()
	r.logger.Info("starting heartbeat ticker")
	r.heartbeatTicker.Start()
	r.logger.Info("starting raft state machine run loop")
	go r.run()
	return nil
}

func (r *raftStateMachine) stop() error {
	r.logger.Info("stopping raft state machine run loop...")
	r.stopChan <- struct{}{}
	err := <-r.stopErrChan
	r.logger.Info("stopped")
	r.logger.Info("stopping election timeout ticker...")
	r.electionTicker.Stop()
	r.logger.Info("stopped")
	r.logger.Info("stopping heartbeat ticker...")
	r.heartbeatTicker.Stop()
	r.logger.Info("stopped")
	return err
}

// newRaftStateMachine constructs a new `raftStateMachine` from `Configuration`.
// Remember to connect this to a `transport` via `bind`.
func newRaftStateMachine(
	c Configuration,
	recvChan <-chan raftpb.Message,
	sendChan chan<- raftpb.Message,
) (*raftStateMachine, error) {
	if c.MsgBufferSize < len(c.PeerAddresses)-1 {
		return nil, fmt.Errorf(
			"MsgBufferSize (%d) is too small; it must be at least %d",
			c.MsgBufferSize, len(c.PeerAddresses)-1)
	}

	heartbeatTicker := newHeartbeatTicker(c.TickPeriod, c.HeartbeatTicks)
	electionTicker := newElectionTicker(c.TickPeriod, c.MinElectionTicks, c.MaxElectionTicks)

	for _, tickerOption := range c.TickerOptions {
		switch x := tickerOption.(type) {
		case withElectionTickerTickerOption:
			heartbeatTicker = x.t
		case withHeartbeatTickerTickerOption:
			electionTicker = x.t
		}
	}

	r := raftStateMachine{
		// ticker
		heartbeatTicker: heartbeatTicker,
		electionTicker:  electionTicker,
		heartbeatC:      nil,

		// network io
		recvChan: recvChan,
		sendChan: sendChan,

		// proposals
		pendingProposal: pendingProposal{},
		propReqChan:     make(chan proposalRequest),
		propRespChan:    make(chan proposalResponse),

		// reads
		pendingRead:  pendingRead{},
		readReqChan:  make(chan readRequest),
		readRespChan: make(chan readResponse),

		// applies
		commitChan: make(chan uint64),

		// state requests
		stateReqChan:  make(chan stateReq),
		stateRespChan: make(chan raftpb.State),

		// peer requests
		peerReqChan:  make(chan peerRequest),
		peerRespChan: make(chan map[uint64]Peer),

		// raft state
		id:          c.ID,
		consistency: c.Consistency,
		state: raftpb.State{
			Role:     raftpb.RoleFollower,
			Term:     0,
			Leader:   0,
			Commit:   0,
			VotedFor: 0,
		},
		quorumSize:     len(c.PeerAddresses)/2 + 1,
		lastEntryIndex: 0,
		lastEntryTerm:  0,
		peers:          map[uint64]*Peer{},

		log:                    newLog(),
		quorumMatchIndexBuffer: make([]uint64, len(c.PeerAddresses)),
		stopChan:               make(chan struct{}, 1),
		stopErrChan:            make(chan error, 1),

		logger:        c.Logger,
		sugaredLogger: c.Logger.Sugar(),
	}
	for id := range c.PeerAddresses {
		r.peers[id] = &Peer{ID: id}
	}
	return &r, nil
}

// Peer contains all info about a Peer node from the perspective of
// this node.
type Peer struct {
	// peer node's ID
	ID uint64
	// last known largest index that this peer matches this node's log
	Match uint64
	// index of the prefix log of entries to send in the heartbeat to the peer
	Next uint64
	// whether or not the peer responded to the heartbeat within the election timeout
	Ack bool
	// vote was granted to elect us by this peer
	VoteGranted bool
}

type peerRequest struct{}

// MarshalLogObject implements zap.Marshaler for Peer.
func (p Peer) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint64("id", p.ID)
	enc.AddUint64("match", p.Match)
	enc.AddUint64("next", p.Next)
	enc.AddBool("ack", p.Ack)
	enc.AddBool("voteGranted", p.VoteGranted)
	return nil
}

// Proposals
type pendingProposal struct {
	index, term uint64
	unixNano    int64
}

type proposalRequest struct {
	unixNano int64
	data     []byte
}

func (p pendingProposal) isPending() bool {
	return p.unixNano != 0
}

type proposalResponse struct {
	err error
}

// Reads
type readRequest struct {
	unixNano int64
}

type pendingRead struct {
	index    uint64
	unixNano int64
	acks     int
}

func (p pendingRead) isPending() bool {
	return p.unixNano != 0
}

type readResponse struct {
	index uint64
	err   error
}

type stateReq struct{}
