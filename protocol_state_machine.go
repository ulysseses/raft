package raft

import (
	"fmt"
	"sort"

	"go.uber.org/zap"

	"github.com/ulysseses/raft/pb"
)

// ProtocolStateMachine represents the Raft Protocol state machine of a Raft node.
// It has a central event loop that interacts with a heartbeat ticker, election ticker,
// and Raft protocol messages sent/received over the transport network.
type ProtocolStateMachine struct {
	// ticker
	heartbeatTicker Ticker
	electionTicker  Ticker
	heartbeatC      <-chan struct{}

	// network io
	recvChan <-chan pb.Message
	sendChan chan<- pb.Message

	// proposals
	propReqChan  chan proposalRequest
	propRespChan chan proposalResponse

	// reads
	readReqChan  chan readRequest
	readRespChan chan readResponse

	// applies
	commitChan chan uint64

	// state requests
	stateReqChan  chan stateReq
	stateRespChan chan State

	// members requests
	membersReqChan  chan membersRequest
	membersRespChan chan map[uint64]MemberState

	// raft state
	state   State
	members map[uint64]*MemberState

	log                    *raftLog
	quorumMatchIndexBuffer []uint64
	nowUnixNanoFunc        func() int64
	stopChan               chan struct{}

	logger *zap.Logger
	debug  bool
}

func (psm *ProtocolStateMachine) run() {
	electionTickerC := psm.electionTicker.C()
	for {
		select {
		case <-psm.stopChan:
			return
		case <-psm.heartbeatC: // heartbeatC is null when not leader
			psm.heartbeat()
		case <-electionTickerC:
			if psm.state.Role == RoleLeader {
				// Step down to follower role if could not establish quorum
				if !psm.hasQuorumAcks() {
					if psm.l() {
						psm.logger.Info("no heartbeats received within election timeout")
					}
					psm.becomeFollower()
				}
				// Reset acks
				for _, m := range psm.members {
					m.Ack = false
				}
			} else {
				psm.becomeCandidate()
			}
		case msg := <-psm.recvChan:
			psm.processMessage(msg)
		case propReq := <-psm.propReqChan:
			psm.propose(propReq)
		case readReq := <-psm.readReqChan:
			psm.read(readReq)
		case <-psm.stateReqChan:
			psm.stateRespChan <- psm.state
		case <-psm.membersReqChan:
			members := map[uint64]MemberState{}
			for _, m := range psm.members {
				members[m.ID] = *m
			}
			psm.membersRespChan <- members
		}
	}
}

func (psm *ProtocolStateMachine) hasQuorumAcks() bool {
	acks := 0
	for _, m := range psm.members {
		if m.ID == psm.state.ID {
			acks++
			continue
		}
		if m.Ack {
			acks++
		}
	}
	return acks >= psm.state.QuorumSize
}

func (psm *ProtocolStateMachine) processMessage(msg pb.Message) {
	if msg.Term < psm.state.Term {
		if psm.debug && psm.l() {
			psm.logger.Debug(
				"ignoring stale msg",
				zap.Uint64("from", msg.From), zap.String("type", msg.Type.String()))
		}
		return
	}
	if msg.Term > psm.state.Term {
		if psm.l() {
			psm.logger.Info("received msg with higher term", zap.Uint64("msgTerm", msg.Term))
		}
		psm.state.Term = msg.Term
		if psm.state.Role != RoleFollower {
			psm.becomeFollower()
		}
	}

	switch msg.Type {
	case pb.MsgApp:
		msgApp := getApp(msg)
		psm.processApp(msgApp)
	case pb.MsgAppResp:
		msgAppResp := getAppResp(msg)
		psm.processAppResp(msgAppResp)
	case pb.MsgRead:
		msgRead := getRead(msg)
		psm.processRead(msgRead)
	case pb.MsgReadResp:
		msgReadResp := getReadResp(msg)
		psm.processReadResp(msgReadResp)
	case pb.MsgProp:
		msgProp := getProp(msg)
		psm.processProp(msgProp)
	case pb.MsgPropResp:
		msgPropResp := getPropResp(msg)
		psm.processPropResp(msgPropResp)
	case pb.MsgVote:
		msgVote := getVote(msg)
		psm.processVote(msgVote)
	case pb.MsgVoteResp:
		msgVoteResp := getVoteResp(msg)
		psm.processVoteResp(msgVoteResp)
	default:
		panic(fmt.Errorf("unrecognized msg type: %s", msg.Type.String()))
	}
}

func (psm *ProtocolStateMachine) processApp(msg msgApp) {
	psm.state.Leader = msg.from
	switch psm.state.Role {
	case RoleFollower:
		psm.electionTicker.Reset()
	case RoleCandidate:
		psm.becomeFollower()
	}

	if msg.proxy == 0 {
		// append request from leader
		var largestMatchIndex uint64 = 0
		success := msg.term >= psm.state.Term &&
			msg.index <= psm.state.LastIndex &&
			msg.logTerm == psm.log.entry(msg.index).Term
		if success {
			psm.state.LastIndex, psm.state.LogTerm = psm.log.append(msg.index, msg.entries...)
			largestMatchIndex = psm.state.LastIndex

			newCommit := msg.commit
			if newCommit >= psm.state.LastIndex {
				newCommit = psm.state.LastIndex
			}
			if newCommit > psm.state.Commit {
				psm.updateCommit(newCommit)
			}
		}
		psm.sendChan <- buildAppResp(
			psm.state.Term, psm.state.ID, msg.from,
			largestMatchIndex, msg.tid, success)
	} else {
		// heartbeat (containing read request) from leader
		psm.sendChan <- buildAppRespStrictRead(
			psm.state.Term, psm.state.ID, msg.from,
			msg.tid, msg.proxy)
	}
}

func (psm *ProtocolStateMachine) processAppResp(msg msgAppResp) {
	if psm.state.Role != RoleLeader {
		return
	}

	m := psm.members[msg.from]
	m.Ack = true

	if msg.proxy == 0 {
		// append response to leader
		// Update match/next index
		if msg.success {
			if msg.index > m.Match {
				m.Match = msg.index
			}
			m.Next = m.Match + 1
		} else {
			if m.Next > m.Match {
				m.Next--
				if psm.l() {
					psm.logger.Info(
						"decreased next",
						zap.Uint64("follower", m.ID), zap.Uint64("newNext", m.Next))
				}
			}
		}

		// update commit if necessary
		quorumMatchIndex := psm.quorumMatchIndex()
		if psm.log.entry(quorumMatchIndex).Term == psm.state.Term && quorumMatchIndex > psm.state.Commit {
			psm.updateCommit(quorumMatchIndex)
		}

		// see if we can extend lease
		if psm.state.Consistency == ConsistencyLease && psm.state.Lease.Start == msg.tid {
			psm.state.Lease.Acks++
			if psm.state.Lease.Acks == psm.state.QuorumSize-1 {
				psm.state.Lease.Timeout = psm.state.Lease.Start + psm.state.Lease.Extension
			}
		}
	} else {
		// read response to leader
		var r *Read
		if msg.proxy == psm.state.ID {
			r = &psm.state.Read
		} else {
			r = &(psm.members[msg.proxy].Read)
		}
		r.Acks++
		if r.Acks == psm.state.QuorumSize-1 {
			if msg.proxy == psm.state.ID {
				// respond to own (leader's) original read request
				psm.endPendingRead(msg.tid)
			} else {
				// respond to original read request
				psm.sendChan <- buildReadResp(
					psm.state.Term, psm.state.ID, msg.proxy,
					r.TID, r.Index)
				r.Acks = 0
				r.Index = 0
			}
		}
	}
}

func (psm *ProtocolStateMachine) processRead(msg msgRead) {
	if psm.state.Role == RoleLeader {
		proxyMember := psm.members[msg.from]
		proxyMember.Read.TID = msg.tid
		proxyMember.Read.Acks = 1
		proxyMember.Read.Index = psm.state.Commit
		psm.heartbeatStrictRead(msg.tid, msg.from)
	}
}

func (psm *ProtocolStateMachine) processReadResp(msg msgReadResp) {
	psm.endPendingRead(msg.tid)
}

func (psm *ProtocolStateMachine) processProp(msg msgProp) {
	// cannot accept proposal if not leader
	if psm.state.Role != RoleLeader {
		return
	}

	// Append proposed entry to log.
	entry := pb.Entry{
		Index: psm.state.LastIndex + 1,
		Term:  psm.state.Term,
		Data:  msg.data,
	}
	psm.state.LastIndex, psm.state.LogTerm = psm.log.append(psm.state.LastIndex, entry)

	psm.sendChan <- buildPropResp(
		psm.state.Term, psm.state.ID, msg.from,
		msg.tid, entry.Index, entry.Term)
}

func (psm *ProtocolStateMachine) processPropResp(msg msgPropResp) {
	// check if the proposal response is the one we're looking for (equal unixNano)
	if msg.tid == psm.state.Proposal.TID {
		psm.state.Proposal.Index = msg.index
	}
}

func (psm *ProtocolStateMachine) processVote(msg msgVote) {
	if psm.state.Role != RoleFollower {
		return
	}
	grantVote := psm.state.VotedFor == 0 &&
		(msg.logTerm > psm.state.Term || msg.index >= psm.state.LastIndex)
	if grantVote {
		psm.sendChan <- buildVoteResp(psm.state.Term, psm.state.ID, msg.from)
		psm.state.VotedFor = msg.from
		if psm.l() {
			psm.logger.Info(
				"voted for candidate",
				zap.Uint64("votedFor", psm.state.VotedFor), zap.Uint64("term", psm.state.Term))
		}
	}
}

func (psm *ProtocolStateMachine) processVoteResp(msg msgVoteResp) {
	if psm.l() {
		psm.logger.Info("got vote", zap.Uint64("from", msg.from), zap.Uint64("term", msg.term))
	}
	psm.members[msg.from].VoteGranted = true
	voteCount := 0
	for _, m := range psm.members {
		if psm.state.ID == m.ID || m.VoteGranted {
			voteCount++
		}
	}
	if voteCount >= psm.state.QuorumSize && psm.state.Role != RoleLeader {
		psm.becomeLeader()
	}
}

func (psm *ProtocolStateMachine) propose(req proposalRequest) {
	if psm.state.Role == RoleLeader {
		psm.proposeAsLeader(req)
	} else {
		psm.proposeToLeader(req)
	}
}

func (psm *ProtocolStateMachine) proposeAsLeader(req proposalRequest) {
	entry := pb.Entry{
		Index: psm.state.LastIndex + 1,
		Term:  psm.state.Term,
		Data:  req.data,
	}
	psm.state.Proposal.TID++
	psm.state.Proposal.Index = entry.Index
	psm.state.Proposal.Term = entry.Term
	psm.state.LastIndex, psm.state.LogTerm = psm.log.append(psm.state.LastIndex, entry)
	// shortcut: 1-node cluster
	if psm.state.QuorumSize == 1 {
		psm.endPendingProposal()
	}
}

func (psm *ProtocolStateMachine) proposeToLeader(req proposalRequest) {
	if psm.state.Leader == 0 {
		psm.state.Proposal.Index = 0
		psm.state.Proposal.Term = 0
		if psm.l() {
			psm.logger.Info("no leader")
		}
		return
	}
	psm.state.Proposal.TID++
	psm.state.Proposal.Index = 0
	psm.state.Proposal.Term = 0
	psm.sendChan <- buildProp(
		psm.state.Term, psm.state.ID, psm.state.Leader,
		psm.state.Proposal.TID, req.data)
}

// read is used only in ConsistencyStrict and ConsistencyLease modes.
func (psm *ProtocolStateMachine) read(req readRequest) {
	switch psm.state.Consistency {
	case ConsistencyStale:
		// note: read shouldn't even be called
		select {
		case psm.readRespChan <- readResponse{index: psm.state.Commit}:
		default:
		}
	case ConsistencyStrict:
		psm.readStrict(req)
	case ConsistencyLease:
		psm.readLease(req)
	default:
		panic("")
	}
}

func (psm *ProtocolStateMachine) readStrict(req readRequest) {
	psm.state.Read.TID++

	if psm.state.Role == RoleLeader {
		// shortcut: 1-node cluster
		if psm.state.QuorumSize == 1 {
			psm.endPendingRead(psm.state.Read.TID)
			return
		}
		// ping everyone
		psm.heartbeatStrictRead(psm.state.Read.TID, psm.state.ID)
	} else {
		// send read request to leader
		if psm.state.Leader == 0 {
			return
		}
		psm.sendChan <- buildRead(
			psm.state.Term, psm.state.ID, psm.state.Leader,
			psm.state.Read.TID)
	}
}

func (psm *ProtocolStateMachine) readLease(req readRequest) {
	resp := readResponse{index: psm.state.Commit}
	if psm.state.QuorumSize > 1 && req.unixNano > psm.state.Lease.Timeout {
		if psm.l() {
			psm.logger.Info(
				"lease timed out",
				zap.Int64("readRequestTime", req.unixNano),
				zap.Int64("leaseTimeout", psm.state.Lease.Timeout))
		}
		resp.err = fmt.Errorf("lease timed out")
	}
	select {
	case psm.readRespChan <- resp:
	default:
	}
}

func (psm *ProtocolStateMachine) becomeFollower() {
	if psm.l() {
		psm.logger.Info("becoming follower", zap.Uint64("term", psm.state.Term))
	}
	psm.heartbeatC = nil
	psm.electionTicker.Reset()
	psm.state.Role = RoleFollower
	psm.state.VotedFor = 0
	for _, m := range psm.members {
		m.VoteGranted = false
	}
}

func (psm *ProtocolStateMachine) becomeCandidate() {
	if psm.l() {
		psm.logger.Info("becoming candidate", zap.Uint64("newTerm", psm.state.Term+1))
	}
	psm.heartbeatC = nil
	psm.state.Role = RoleCandidate
	psm.state.Leader = 0
	psm.state.Term++
	psm.state.VotedFor = psm.state.ID

	// Send vote requests to other peers
	lastEntry := psm.log.entry(psm.state.LastIndex)
	for _, m := range psm.members {
		if psm.state.ID == m.ID {
			m.VoteGranted = true
			continue
		}
		m.VoteGranted = false
		psm.sendChan <- buildVote(psm.state.Term, psm.state.ID, m.ID, lastEntry.Index, lastEntry.Term)
	}

	// shortcut: 1-node cluster
	if psm.state.QuorumSize == 1 {
		if psm.l() {
			psm.logger.Info("1-node cluster shortcut: become leader instantly")
		}
		psm.becomeLeader()
	}
}

func (psm *ProtocolStateMachine) becomeLeader() {
	if psm.l() {
		psm.logger.Info("becoming leader")
	}
	psm.state.Role = RoleLeader
	psm.state.Leader = psm.state.ID
	psm.state.VotedFor = 0
	for _, m := range psm.members {
		m.VoteGranted = false
		m.Next = psm.state.LastIndex + 1
		m.Ack = false
	}

	// Try to commit an (empty) entry from the newly elected term
	psm.state.LastIndex, psm.state.LogTerm = psm.log.append(psm.state.LastIndex, pb.Entry{
		Index: psm.state.LastIndex + 1,
		Term:  psm.state.Term,
	})
	psm.heartbeat()
	psm.heartbeatTicker.Reset()
	psm.heartbeatC = psm.heartbeatTicker.C()
}

func (psm *ProtocolStateMachine) heartbeat() {
	if psm.state.Consistency == ConsistencyLease {
		psm.state.Lease.Start = psm.nowUnixNanoFunc()
		psm.state.Lease.Acks = 0
	}

	for _, m := range psm.members {
		if psm.state.ID == m.ID {
			continue
		}
		entries := []pb.Entry{}
		if m.Next <= psm.state.LastIndex {
			psm.log.RLock()
			entries = append(entries, psm.log.entries(m.Next, psm.state.LastIndex)...)
			psm.log.RUnlock()
		}
		prevEntry := psm.log.entry(m.Next - 1)
		psm.sendChan <- buildApp(
			psm.state.Term, psm.state.ID, m.ID,
			prevEntry.Index, prevEntry.Term, psm.state.Commit,
			entries,
			psm.state.Lease.Start)
	}
}

func (psm *ProtocolStateMachine) heartbeatStrictRead(unixNano int64, proxy uint64) {
	for _, m := range psm.members {
		if psm.state.ID == m.ID {
			continue
		}
		psm.sendChan <- buildAppStrictRead(
			psm.state.Term, psm.state.ID, m.ID,
			psm.state.Commit, unixNano, proxy)
	}
}

// Figure out the largest match index of a quorum so far.
func (psm *ProtocolStateMachine) quorumMatchIndex() uint64 {
	matches := psm.quorumMatchIndexBuffer
	i := 0
	for _, m := range psm.members {
		if psm.state.ID == m.ID {
			matches[i] = psm.state.LastIndex
		}
		matches[i] = m.Match
		i++
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i] > matches[j]
	})
	return matches[psm.state.QuorumSize-1]
}

func (psm *ProtocolStateMachine) endPendingProposal() {
	if psm.debug && psm.l() {
		psm.logger.Debug("ending potentially pending proposal")
	}
	resp := proposalResponse{
		index: psm.state.Proposal.Index,
		term:  psm.state.Proposal.Term,
	}
	select {
	case psm.propRespChan <- resp:
	default:
	}
	// zero out pending proposal
	psm.state.Proposal.Index = 0
	psm.state.Proposal.Term = 0
}

// ack (nil error) or cancel (non-nil error) any pending read requests
func (psm *ProtocolStateMachine) endPendingRead(tid int64) {
	if tid != psm.state.Read.TID {
		if psm.debug && psm.l() {
			psm.logger.Debug("TID has moved on", zap.Int64("oldTID", tid), zap.Int64("newTID", psm.state.Read.TID))
		}
		return
	}
	if psm.debug && psm.l() {
		psm.logger.Debug("ending pending read")
	}
	resp := readResponse{
		index: psm.state.Read.Index,
	}
	select {
	case psm.readRespChan <- resp:
	default:
	}
	// zero out pending read
	psm.state.Read.Acks = 0
	psm.state.Read.Index = 0
}

// update commit and alert downstream application state machine
func (psm *ProtocolStateMachine) updateCommit(newCommit uint64) {
	if psm.debug && psm.l() {
		psm.logger.Debug(
			"updating commit",
			zap.Uint64("oldCommit", psm.state.Commit), zap.Uint64("newCommit", newCommit))
	}
	psm.state.Commit = newCommit
	psm.commitChan <- newCommit

	canAckProp := psm.state.Proposal.Index <= psm.state.Commit &&
		psm.state.Proposal.Term <= psm.state.LogTerm
	if canAckProp {
		psm.endPendingProposal()
	}
}

func (psm *ProtocolStateMachine) start() {
	if psm.l() {
		psm.logger.Info("starting election timeout ticker")
	}
	psm.electionTicker.Start()
	if psm.l() {
		psm.logger.Info("starting heartbeat ticker")
	}
	psm.heartbeatTicker.Start()
	if psm.l() {
		psm.logger.Info("starting raft state machine run loop")
	}
	go psm.run()
}

func (psm *ProtocolStateMachine) stop() {
	if psm.l() {
		psm.logger.Info("stopping raft state machine run loop...")
	}
	psm.stopChan <- struct{}{}
	if psm.l() {
		psm.logger.Info("stopped")
		psm.logger.Info("stopping election timeout ticker...")
	}
	psm.electionTicker.Stop()
	if psm.l() {
		psm.logger.Info("stopped")
		psm.logger.Info("stopping heartbeat ticker...")
	}
	psm.heartbeatTicker.Stop()
	if psm.l() {
		psm.logger.Info("stopped")
	}
}

func (psm *ProtocolStateMachine) l() bool {
	return psm.logger != nil
}

type proposalRequest struct {
	data []byte
}
type proposalResponse struct {
	index, term uint64
}
type readRequest struct {
	unixNano int64 // used only by ConsistencyLease
}
type readResponse struct {
	index uint64
	err   error
}
type membersRequest struct{}
type stateReq struct{}
