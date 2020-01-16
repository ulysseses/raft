package raft

import (
	"fmt"
	"sort"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/ulysseses/raft/raftpb"
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
	id          uint64
	consistency Consistency
	quorumSize  int
	state       raftpb.State
	peers       map[uint64]*Peer

	log                    *raftLog
	quorumMatchIndexBuffer []uint64
	stopChan               chan struct{}
	stopErrChan            chan error

	logger        *zap.Logger
	sugaredLogger *zap.SugaredLogger
	debug         bool
}

func (r *raftStateMachine) run() {
	electionTickerC := r.electionTicker.C()
	for {
		select {
		case <-r.stopChan:
			r.stopErrChan <- nil
			return
		case <-r.heartbeatC: // heartbeatC is null when not leader
			r.heartbeatWithEntries()
		case <-electionTickerC:
			if r.state.Role == raftpb.RoleLeader {
				// Step down to follower role if could not establish quorum
				if !r.hasQuorumAcks() {
					r.logger.Info("no heartbeats received within election timeout")
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
		if r.debug {
			r.logger.Debug(
				"ignoring stale msg",
				zap.Uint64("from", msg.From), zap.String("type", msg.Type.String()))
		}
		return nil
	}
	if msg.Term > r.state.Term {
		r.logger.Info("received msg with higher term", zap.Uint64("msgTerm", msg.Term))
		r.state.Term = msg.Term
		if r.state.Role != raftpb.RoleFollower {
			r.becomeFollower()
		}
	}

	switch msg.Type {
	case raftpb.MsgApp:
		msgApp := getApp(msg)
		r.processApp(msgApp)
	case raftpb.MsgAppResp:
		msgAppResp := getAppResp(msg)
		r.processAppResp(msgAppResp)
	case raftpb.MsgRead:
		msgRead := getRead(msg)
		r.processRead(msgRead)
	case raftpb.MsgReadResp:
		msgReadResp := getReadResp(msg)
		r.processReadResp(msgReadResp)
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
	case raftpb.RoleFollower:
		r.electionTicker.Reset()
	case raftpb.RoleCandidate:
		r.becomeFollower()
	}

	if msg.unixNano == 0 {
		// append request from leader
		var largestMatchIndex uint64 = 0
		success := msg.term >= r.state.Term &&
			msg.index <= r.state.LastIndex &&
			msg.logTerm == r.log.entry(msg.index).Term
		if success {
			r.state.LastIndex, r.state.LogTerm = r.log.append(msg.index, msg.entries...)
			largestMatchIndex = r.state.LastIndex

			newCommit := msg.commit
			if newCommit >= r.state.LastIndex {
				newCommit = r.state.LastIndex
			}
			if newCommit > r.state.Commit {
				r.updateCommit(newCommit)
			}
		}
		resp := buildAppResp(r.state.Term, r.id, msg.from, largestMatchIndex, 0, 0)
		select {
		case r.sendChan <- resp:
		default:
			r.logInfoMsg("could not send", resp)
		}
	} else {
		// read request from leader
		resp := buildAppResp(r.state.Term, r.id, msg.from, 0, msg.unixNano, msg.proxy)
		select {
		case r.sendChan <- resp:
		default:
			r.logInfoMsg("could not send", resp)
		}
	}
}

func (r *raftStateMachine) processAppResp(msg msgAppResp) {
	if r.state.Role != raftpb.RoleLeader {
		return
	}

	p := r.peers[msg.from]
	p.Ack = true

	if msg.unixNano == 0 {
		// append response to leader
		// Update match/next index
		success := msg.index != 0
		if success {
			if msg.index > p.Match {
				p.Match = msg.index
			}
			p.Next = r.state.LastIndex + 1
		} else {
			p.Next--
			r.logger.Info("decreased next", zap.Uint64("follower", p.ID))
		}

		quorumMatchIndex := r.quorumMatchIndex()
		if r.log.entry(quorumMatchIndex).Term == r.state.Term && quorumMatchIndex > r.state.Commit {
			r.updateCommit(quorumMatchIndex)
		}
	} else {
		// read response to leader
		origin := r.peers[msg.proxy]
		origin.ReadAcks++
		if origin.ReadAcks+1 >= r.quorumSize { // +1 from leader itself
			if msg.proxy == r.id {
				// respond to own (leader's) original read request
				r.endPendingRead(origin)
			} else {
				// respond to original read request
				resp := buildReadResp(
					r.state.Term, r.id, origin.ID,
					origin.UnixNano, origin.ReadIndex)
				select {
				case r.sendChan <- resp:
				default:
					r.logInfoMsg("could not send", resp)
				}
				origin.UnixNano = 0
				origin.ReadAcks = 0
				origin.ReadIndex = 0
			}
		}
	}

	canAckProp := r.pendingProposal.unixNano != 0 &&
		r.pendingProposal.unixNano <= msg.unixNano &&
		r.pendingProposal.index <= r.state.Commit &&
		r.pendingProposal.term <= r.state.Term
	if canAckProp {
		r.endPendingProposal()
	}
}

func (r *raftStateMachine) processRead(msg msgRead) {
	if r.state.Role == raftpb.RoleLeader {
		proxyPeer := r.peers[msg.from]
		proxyPeer.UnixNano = msg.unixNano
		proxyPeer.ReadAcks = 0
		proxyPeer.ReadIndex = r.state.Commit
		r.heartbeatWithContext(msg.unixNano, msg.from)
	}
}

func (r *raftStateMachine) processReadResp(msg msgReadResp) {
	me := r.peers[r.id]
	if me.UnixNano == msg.unixNano {
		r.endPendingRead(me)
	}
}

func (r *raftStateMachine) processProp(msg msgProp) {
	// cannot accept proposal if not leader
	if r.state.Role != raftpb.RoleLeader {
		return
	}

	// Append proposed entry to log.
	entry := raftpb.Entry{
		Index: r.state.LastIndex + 1,
		Term:  r.state.Term,
		Data:  msg.data,
	}
	r.state.LastIndex, r.state.LogTerm = r.log.append(r.state.LastIndex, entry)

	resp := buildPropResp(
		r.state.Term, r.id, msg.from,
		msg.unixNano, entry.Index, entry.Term)
	select {
	case r.sendChan <- resp:
	default:
		r.logInfoMsg("could not send", resp)
	}
}

func (r *raftStateMachine) processPropResp(msg msgPropResp) {
	// check if the proposal response is the one we're looking for (equal unixNano)
	if msg.unixNano == r.pendingProposal.unixNano {
		r.pendingProposal.index = msg.index
	}
}

func (r *raftStateMachine) processVote(msg msgVote) {
	if r.state.Role != raftpb.RoleFollower {
		return
	}
	grantVote := r.state.VotedFor == 0 &&
		(msg.logTerm > r.state.Term || msg.index >= r.state.LastIndex)
	if grantVote {
		resp := buildVoteResp(r.state.Term, r.id, msg.from)
		select {
		case r.sendChan <- resp:
			r.state.VotedFor = msg.from
			r.logger.Info(
				"voted for candidate",
				zap.Uint64("votedFor", r.state.VotedFor), zap.Uint64("term", r.state.Term))
		default:
			r.logInfoMsg("could not send", resp)
		}
	}
}

func (r *raftStateMachine) processVoteResp(msg msgVoteResp) {
	r.logger.Info("got vote", zap.Uint64("from", msg.from), zap.Uint64("term", msg.term))
	r.peers[msg.from].VoteGranted = true
	voteCount := 0
	for _, p := range r.peers {
		if r.id == p.ID || p.VoteGranted {
			voteCount++
		}
	}
	if voteCount >= r.quorumSize && r.state.Role != raftpb.RoleLeader {
		r.becomeLeader()
	}
}

func (r *raftStateMachine) propose(req proposalRequest) {
	if r.state.Role == raftpb.RoleLeader {
		entry := raftpb.Entry{
			Index: r.state.LastIndex + 1,
			Term:  r.state.Term,
			Data:  req.data,
		}
		r.pendingProposal = pendingProposal{
			index:    entry.Index,
			term:     entry.Term,
			unixNano: req.unixNano,
		}
		r.state.LastIndex, r.state.LogTerm = r.log.append(r.state.LastIndex, entry)
	} else {
		if r.state.Leader == 0 {
			r.pendingProposal = pendingProposal{}
			r.logger.Info("no leader")
			return
		}
		r.pendingProposal = pendingProposal{unixNano: req.unixNano}
		prop := buildProp(
			r.state.Term, r.id, r.state.Leader,
			req.unixNano, req.data)
		select {
		case r.sendChan <- prop:
		default:
			r.pendingProposal = pendingProposal{}
			r.logInfoMsg("could not send", prop)
		}
	}
}

func (r *raftStateMachine) read(req readRequest) {
	me := r.peers[r.id]
	me.UnixNano = req.unixNano

	// serializable shortcut
	if r.consistency == ConsistencySerializable {
		r.endPendingRead(me)
		return
	}

	if r.state.Role == raftpb.RoleLeader {
		// ping everyone
		r.heartbeatWithContext(req.unixNano, r.id)
	} else {
		// send read request to leader
		if r.state.Leader == 0 {
			return
		}
		req := buildRead(r.state.Term, r.id, r.state.Leader, req.unixNano)
		select {
		case r.sendChan <- req:
		default:
			r.logInfoMsg("could not send", req)
		}
	}
}

func (r *raftStateMachine) becomeFollower() {
	r.logger.Info("becoming follower", zap.Uint64("term", r.state.Term))
	r.heartbeatC = nil
	r.electionTicker.Reset()
	r.state.Role = raftpb.RoleFollower
	r.state.VotedFor = 0
	for _, p := range r.peers {
		p.VoteGranted = false
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
	lastEntry := r.log.entry(r.state.LastIndex)
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
			r.logInfoMsg("could not send", req)
		}
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
		p.Next = r.state.LastIndex + 1
		p.Ack = false
	}

	// Try to commit an (empty) entry from the newly elected term
	r.state.LastIndex, r.state.LogTerm = r.log.append(r.state.LastIndex, raftpb.Entry{
		Index: r.state.LastIndex + 1,
		Term:  r.state.Term,
	})
	r.heartbeatWithEntries()
	r.heartbeatTicker.Reset()
	r.heartbeatC = r.heartbeatTicker.C()
}

func (r *raftStateMachine) heartbeatWithEntries() {
	for _, p := range r.peers {
		if r.id == p.ID {
			continue
		}
		entries := []raftpb.Entry{}
		if p.Next <= r.state.LastIndex {
			r.log.RLock()
			entries = append(entries, r.log.entries(p.Next, r.state.LastIndex)...)
			r.log.RUnlock()
		}
		prevEntry := r.log.entry(p.Next - 1)
		req := buildApp(
			r.state.Term, r.id, p.ID,
			r.state.Commit, entries, prevEntry.Index, prevEntry.Term, 0, 0)
		select {
		case r.sendChan <- req:
		default:
			r.logInfoMsg("could not send", req)
		}
	}
}

func (r *raftStateMachine) heartbeatWithContext(unixNano int64, proxy uint64) {
	for _, p := range r.peers {
		if r.id == p.ID {
			continue
		}
		req := buildApp(
			r.state.Term, r.id, p.ID,
			r.state.Commit, []raftpb.Entry{}, 0, 0, unixNano, proxy)
		select {
		case r.sendChan <- req:
		default:
			r.logInfoMsg("could not send", req)
		}
	}
}

// Figure out the largest match index of a quorum so far.
func (r *raftStateMachine) quorumMatchIndex() uint64 {
	matches := r.quorumMatchIndexBuffer
	i := 0
	for _, p := range r.peers {
		if r.id == p.ID {
			matches[i] = r.state.LastIndex
		}
		matches[i] = p.Match
		i++
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i] > matches[j]
	})
	return matches[r.quorumSize-1]
}

func (r *raftStateMachine) endPendingProposal() {
	if r.debug {
		r.logger.Debug("ending pending proposal")
	}
	resp := proposalResponse{
		index: r.pendingProposal.index,
	}
	select {
	case r.propRespChan <- resp:
	default:
		r.logger.Info("could not send proposalResponse")
	}
	// zero out pending proposal
	r.pendingProposal = pendingProposal{}
}

// ack (nil error) or cancel (non-nil error) any pending read requests
func (r *raftStateMachine) endPendingRead(origin *Peer) {
	result := readResponse{
		index: origin.ReadIndex,
	}
	select {
	case r.readRespChan <- result:
	default:
		r.logger.Info("could not send readResponse")
	}
	origin.UnixNano = 0
	origin.ReadAcks = 0
	origin.ReadIndex = 0
}

// update commit and alert downstream application state machine
func (r *raftStateMachine) updateCommit(newCommit uint64) {
	if r.debug {
		r.logger.Debug(
			"updating commit",
			zap.Uint64("oldCommit", r.state.Commit), zap.Uint64("newCommit", newCommit))
	}
	r.state.Commit = newCommit
	r.commitChan <- newCommit
	if r.pendingProposal.unixNano != 0 && r.state.Commit >= r.pendingProposal.index {
		r.endPendingProposal()
	}
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

func (r *raftStateMachine) logInfoMsg(txt string, msg raftpb.Message) {
	// scrub entries
	msg.Entries = nil

	r.logger.Info(txt, zap.String("msg", msg.String()))
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
			Role:      raftpb.RoleFollower,
			Term:      0,
			Leader:    0,
			Commit:    0,
			VotedFor:  0,
			LastIndex: 0,
			LogTerm:   0,
		},
		quorumSize: len(c.PeerAddresses)/2 + 1,
		peers:      map[uint64]*Peer{},

		log:                    newLog(),
		quorumMatchIndexBuffer: make([]uint64, len(c.PeerAddresses)),
		stopChan:               make(chan struct{}, 1),
		stopErrChan:            make(chan error, 1),

		logger:        c.Logger,
		sugaredLogger: c.Logger.Sugar(),
		debug:         c.Debug,
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

	// latest read request context; if none, 0
	UnixNano int64

	// number of acks for the latest read request
	ReadAcks int

	// ReadIndex of the read request; if none, 0
	ReadIndex uint64
}

type peerRequest struct{}

// MarshalLogObject implements zap.Marshaler for Peer.
func (p Peer) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint64("id", p.ID)
	enc.AddUint64("match", p.Match)
	enc.AddUint64("next", p.Next)
	enc.AddBool("ack", p.Ack)
	enc.AddBool("voteGranted", p.VoteGranted)
	enc.AddInt64("unixNano", p.UnixNano)
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

type proposalResponse struct {
	index uint64
}

// Reads
type readRequest struct {
	unixNano int64
}

type readResponse struct {
	index uint64
}

// State
type stateReq struct{}
