package raft

import (
	"fmt"
	"log"
	"sort"

	"github.com/ulysseses/raft/raftpb"
)

var (
	// ErrDroppedProposal is the error emitted when a proposal got dropped.
	ErrDroppedProposal = fmt.Errorf("dropped proposal")
	// ErrNotLeader is the error emitted when trying to propose to a non-leader.
	ErrNotLeader = fmt.Errorf("not leader")
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

	// raft state
	id             uint64
	consistency    Consistency
	state          raftpb.State
	quorumSize     int
	lastEntryIndex uint64
	lastEntryTerm  uint64
	peers          map[uint64]*peer

	log                    *raftLog
	quorumMatchIndexBuffer []uint64
	stopChan               chan struct{}
}

func (r *raftStateMachine) run() error {
	electionTickerC := r.electionTicker.C()
	for {
		select {
		case <-r.stopChan:
			return nil
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
					p.ack = false
				}
			} else {
				r.becomeCandidate()
			}
		case msg := <-r.recvChan:
			if err := r.processMessage(msg); err != nil {
				return err
			}
		case propReq := <-r.propReqChan:
			r.propose(propReq)
		case readReq := <-r.readReqChan:
			r.read(readReq)
		case _ = <-r.stateReqChan:
			r.stateRespChan <- r.state
		}
	}
}

func (r *raftStateMachine) hasQuorumAcks() bool {
	acks := 0
	for _, p := range r.peers {
		if p.id == r.id {
			acks++
			continue
		}
		if p.ack {
			acks++
		}
	}
	return acks >= r.quorumSize
}

func (r *raftStateMachine) processMessage(msg raftpb.Message) error {
	if msg.Term < r.state.Term {
		return nil
	}
	if msg.Term > r.state.Term {
		r.state.Term = msg.Term
		r.becomeFollower()

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
	localPrevTerm := r.log.entry(msg.index).Term
	success := msg.term >= r.state.Term &&
		msg.index <= r.lastEntryIndex &&
		msg.logTerm == localPrevTerm
	if success {
		r.lastEntryIndex = r.log.append(msg.index, msg.entries...)
		largestMatchIndex = r.lastEntryIndex
		r.lastEntryTerm = msg.entries[len(msg.entries)-1].Term

		newCommit := msg.commit
		if newCommit >= r.lastEntryIndex {
			newCommit = r.lastEntryIndex
		}
		r.updateCommit(newCommit)
	}

	resp := buildAppResp(r.state.Term, r.id, msg.to, largestMatchIndex)
	select {
	case r.sendChan <- resp:
	default:
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

func (r *raftStateMachine) processAppResp(msg msgAppResp) {
	switch r.state.Role {
	case raftpb.RoleFollower:
		return
	case raftpb.RoleCandidate:
		return
	}

	p := r.peers[msg.from]

	p.ack = true

	// Update match/next index
	success := msg.index != 0
	if success {
		if msg.index > p.match {
			p.match = msg.index
		}
		p.next = r.lastEntryIndex + 1
	} else {
		p.next--
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

func (r *raftStateMachine) processPong(msg msgPong) {
	if r.state.Role != raftpb.RoleLeader {
		return
	}

	r.peers[msg.from].ack = true

	if r.pendingRead.isPending() && msg.unixNano == r.pendingRead.unixNano && msg.index == r.pendingRead.index {
		r.pendingRead.acks++
	}

	if r.canAckLinearizableRead() {
		r.endPendingRead(nil)
	}
}

func (r *raftStateMachine) processProp(msg msgProp) {
	// fail the proposal if not leader
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

	entry := raftpb.Entry{
		Index: r.lastEntryIndex + 1,
		Term:  r.state.Term,
		Data:  msg.data,
	}
	r.lastEntryIndex = r.log.append(r.lastEntryIndex, entry)

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

	if msg.unixNano == r.pendingProposal.unixNano && msg.index != 0 {
		// serializable shortcut
		if r.consistency == ConsistencySerializable {
			r.endPendingProposal(nil)
		}
	} else {
		r.endPendingProposal(ErrNotLeader)
	}
}

func (r *raftStateMachine) processVote(msg msgVote) {
	switch r.state.Role {
	case raftpb.RoleLeader:
		return
	case raftpb.RoleCandidate:
		return
	}
	grantVote := msg.term >= r.state.Term &&
		(r.state.VotedFor == 0 || r.state.VotedFor == msg.from) &&
		(msg.term > r.state.Term || msg.index >= r.lastEntryIndex)
	if grantVote {
		r.state.VotedFor = msg.from
		resp := buildVoteResp(r.state.Term, r.id, msg.from)
		select {
		case r.sendChan <- resp:
		default:
		}
	}
}

func (r *raftStateMachine) processVoteResp(msg msgVoteResp) {
	r.peers[msg.from].voteGranted = true
	voteCount := 0
	for _, p := range r.peers {
		if p.voteGranted {
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
	r.lastEntryIndex = r.log.append(r.lastEntryIndex, entry)
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

func (r *raftStateMachine) getState() raftpb.State {
	r.stateReqChan <- stateReq{}
	return <-r.stateRespChan
}

func (r *raftStateMachine) becomeCandidate() {
	var sendChan chan<- raftpb.Message = r.sendChan
	r.state.Role = raftpb.RoleCandidate
	r.state.Leader = 0
	r.state.Term++
	r.endPendingProposal(ErrDroppedProposal)
	r.endPendingRead(ErrDroppedRead)
	r.state.VotedFor = r.id
	for _, p := range r.peers {
		p.voteGranted = false
	}

	lastEntry := r.log.entry(r.lastEntryIndex)
	for peerID := range r.peers {
		if r.id == peerID {
			continue
		}
		req := buildVote(r.state.Term, r.id, peerID, lastEntry.Index, lastEntry.Term)
		select {
		case sendChan <- req:
		default:
		}
	}
}

func (r *raftStateMachine) becomeFollower() {
	r.state.Role = raftpb.RoleFollower
	r.state.VotedFor = 0
	for _, p := range r.peers {
		p.voteGranted = false
	}
	r.endPendingProposal(ErrDroppedProposal)
	r.endPendingRead(ErrDroppedRead)
}

func (r *raftStateMachine) becomeLeader() {
	r.state.Role = raftpb.RoleLeader
	r.state.Leader = r.id
	r.state.VotedFor = 0
	for _, p := range r.peers {
		p.voteGranted = false
	}

	for _, p := range r.peers {
		p.next = r.lastEntryIndex + 1
		p.ack = false
	}

	// Try to commit an (empty) entry from the newly elected term
	r.lastEntryIndex = r.log.append(r.lastEntryIndex, raftpb.Entry{
		Index: r.lastEntryIndex + 1,
		Term:  r.state.Term,
	})
	r.lastEntryTerm = r.state.Term
	r.broadcastApp()
	r.heartbeatTicker.Reset()
	r.heartbeatC = r.heartbeatTicker.C()
}

func (r *raftStateMachine) broadcastApp() {
	for _, p := range r.peers {
		if r.id == p.id {
			continue
		}
		entries := []raftpb.Entry{}
		if p.next <= r.lastEntryIndex {
			r.log.RLock()
			entries = append(entries, r.log.entries(p.next, r.lastEntryIndex)...)
			r.log.RUnlock()
		}
		prevEntry := r.log.entry(p.next - 1)
		req := buildApp(r.state.Term, r.id, p.id, r.state.Commit, entries, prevEntry.Index, prevEntry.Term)
		select {
		case r.sendChan <- req:
		default:
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
		}
	}
}

// Figure out the largest match index of a quorum so far.
func (r *raftStateMachine) quorumMatchIndex() uint64 {
	matches := r.quorumMatchIndexBuffer
	i := 0
	for _, p := range r.peers {
		matches[i] = p.match
		i++
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i] > matches[j]
	})
	return matches[len(matches)-(r.quorumSize-1)]
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

func (r *raftStateMachine) start() {
	r.electionTicker.Start()
	r.heartbeatTicker.Start()
	go func() {
		if err := r.run(); err != nil {
			log.Print(err)
		}
	}()
}

func (r *raftStateMachine) stop() {
	r.stopChan <- struct{}{}
	r.electionTicker.Stop()
	r.heartbeatTicker.Stop()
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
		peers:          map[uint64]*peer{},

		log:                    newLog(),
		quorumMatchIndexBuffer: make([]uint64, len(c.PeerAddresses)/2),
		stopChan:               make(chan struct{}),
	}
	for id := range c.PeerAddresses {
		r.peers[id] = &peer{id: id}
	}
	return &r, nil
}

// peer contains all info about a peer node from the perspective of
// this node.
type peer struct {
	// peer node's ID
	id uint64
	// last known largest index that this peer matches this node's log
	match uint64
	// index of the prefix log of entries to send in the heartbeat to the peer
	next uint64
	// whether or not the peer responded to the heartbeat within the election timeout
	ack bool
	// vote was granted to elect us by this peer
	voteGranted bool
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
