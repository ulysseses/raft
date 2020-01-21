package raft

import (
	"fmt"
	"sort"

	"go.uber.org/zap"

	"github.com/ulysseses/raft/raftpb"
)

// protocolStateMachine represents the Raft Protocol state machine of a Raft node.
// It has a central event loop that interacts with a heartbeat ticker, election ticker,
// and Raft protocol messages sent/received over the transport network.
type protocolStateMachine struct {
	// ticker
	heartbeatTicker Ticker
	electionTicker  Ticker
	heartbeatC      <-chan struct{}

	// network io
	recvChan <-chan raftpb.Message
	sendChan chan<- raftpb.Message

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
	stopChan               chan struct{}

	logger *zap.Logger
	debug  bool
}

func (psm *protocolStateMachine) run() {
	electionTickerC := psm.electionTicker.C()
	for {
		select {
		case <-psm.stopChan:
			return
		case <-psm.heartbeatC: // heartbeatC is null when not leader
			psm.heartbeatWithEntries()
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
				for _, p := range psm.members {
					p.Ack = false
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

func (psm *protocolStateMachine) hasQuorumAcks() bool {
	acks := 0
	for _, p := range psm.members {
		if p.ID == psm.state.ID {
			acks++
			continue
		}
		if p.Ack {
			acks++
		}
	}
	return acks >= psm.state.QuorumSize
}

func (psm *protocolStateMachine) processMessage(msg raftpb.Message) {
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
	case raftpb.MsgApp:
		msgApp := getApp(msg)
		psm.processApp(msgApp)
	case raftpb.MsgAppResp:
		msgAppResp := getAppResp(msg)
		psm.processAppResp(msgAppResp)
	case raftpb.MsgRead:
		msgRead := getRead(msg)
		psm.processRead(msgRead)
	case raftpb.MsgReadResp:
		msgReadResp := getReadResp(msg)
		psm.processReadResp(msgReadResp)
	case raftpb.MsgProp:
		msgProp := getProp(msg)
		psm.processProp(msgProp)
	case raftpb.MsgPropResp:
		msgPropResp := getPropResp(msg)
		psm.processPropResp(msgPropResp)
	case raftpb.MsgVote:
		msgVote := getVote(msg)
		psm.processVote(msgVote)
	case raftpb.MsgVoteResp:
		msgVoteResp := getVoteResp(msg)
		psm.processVoteResp(msgVoteResp)
	default:
		panic(fmt.Errorf("unrecognized msg type: %s", msg.Type.String()))
	}
}

func (psm *protocolStateMachine) processApp(msg msgApp) {
	psm.state.Leader = msg.from
	switch psm.state.Role {
	case RoleFollower:
		psm.electionTicker.Reset()
	case RoleCandidate:
		psm.becomeFollower()
	}

	if msg.tid == 0 {
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
			largestMatchIndex,
			0, 0,
			success)
	} else {
		// heartbeat (containing read request) from leader
		psm.sendChan <- buildAppResp(
			psm.state.Term, psm.state.ID, msg.from,
			0,
			msg.tid, msg.proxy,
			false)
	}
}

func (psm *protocolStateMachine) processAppResp(msg msgAppResp) {
	if psm.state.Role != RoleLeader {
		return
	}

	p := psm.members[msg.from]
	p.Ack = true

	if msg.tid == 0 {
		// append response to leader
		// Update match/next index
		if msg.success {
			if msg.index > p.Match {
				p.Match = msg.index
			}
			p.Next = p.Match + 1
		} else {
			if p.Next > p.Match {
				p.Next--
				if psm.l() {
					psm.logger.Info(
						"decreased next",
						zap.Uint64("follower", p.ID), zap.Uint64("newNext", p.Next))
				}
			}
		}

		quorumMatchIndex := psm.quorumMatchIndex()
		if psm.log.entry(quorumMatchIndex).Term == psm.state.Term && quorumMatchIndex > psm.state.Commit {
			psm.updateCommit(quorumMatchIndex)
		}
	} else {
		// read response to leader
		var readContext *ReadContext
		if msg.proxy == psm.state.ID {
			readContext = &psm.state.ReadContext
		} else {
			readContext = &(psm.members[msg.proxy].ReadContext)
		}
		readContext.Acks++
		if readContext.Acks >= psm.state.QuorumSize {
			if msg.proxy == psm.state.ID {
				// respond to own (leader's) original read request
				psm.endPendingRead(msg.tid)
			} else {
				// respond to original read request
				psm.sendChan <- buildReadResp(
					psm.state.Term, psm.state.ID, msg.proxy,
					readContext.TID, readContext.Index)
				readContext.Acks = 0
				readContext.Index = 0
			}
		}
	}

	canAckProp := psm.state.ProposalContext.TID == msg.tid &&
		psm.state.ProposalContext.Index <= psm.state.Commit &&
		psm.state.ProposalContext.Term <= psm.state.Term
	if canAckProp {
		psm.endPendingProposal()
	}
}

func (psm *protocolStateMachine) processRead(msg msgRead) {
	if psm.state.Role == RoleLeader {
		proxyPeer := psm.members[msg.from]
		proxyPeer.ReadContext.TID = msg.tid
		proxyPeer.ReadContext.Acks = 1
		proxyPeer.ReadContext.Index = psm.state.Commit

		psm.heartbeatWithContext(msg.tid, msg.from)
	}
}

func (psm *protocolStateMachine) processReadResp(msg msgReadResp) {
	psm.endPendingRead(msg.tid)
}

func (psm *protocolStateMachine) processProp(msg msgProp) {
	// cannot accept proposal if not leader
	if psm.state.Role != RoleLeader {
		return
	}

	// Append proposed entry to log.
	entry := raftpb.Entry{
		Index: psm.state.LastIndex + 1,
		Term:  psm.state.Term,
		Data:  msg.data,
	}
	psm.state.LastIndex, psm.state.LogTerm = psm.log.append(psm.state.LastIndex, entry)

	psm.sendChan <- buildPropResp(
		psm.state.Term, psm.state.ID, msg.from,
		msg.tid, entry.Index, entry.Term)
}

func (psm *protocolStateMachine) processPropResp(msg msgPropResp) {
	// check if the proposal response is the one we're looking for (equal unixNano)
	if msg.tid == psm.state.ProposalContext.TID {
		psm.state.ProposalContext.Index = msg.index
	}
}

func (psm *protocolStateMachine) processVote(msg msgVote) {
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

func (psm *protocolStateMachine) processVoteResp(msg msgVoteResp) {
	if psm.l() {
		psm.logger.Info("got vote", zap.Uint64("from", msg.from), zap.Uint64("term", msg.term))
	}
	psm.members[msg.from].VoteGranted = true
	voteCount := 0
	for _, p := range psm.members {
		if psm.state.ID == p.ID || p.VoteGranted {
			voteCount++
		}
	}
	if voteCount >= psm.state.QuorumSize && psm.state.Role != RoleLeader {
		psm.becomeLeader()
	}
}

func (psm *protocolStateMachine) propose(req proposalRequest) {
	if psm.state.Role == RoleLeader {
		entry := raftpb.Entry{
			Index: psm.state.LastIndex + 1,
			Term:  psm.state.Term,
			Data:  req.data,
		}
		psm.state.ProposalContext.TID++
		psm.state.ProposalContext.Index = entry.Index
		psm.state.ProposalContext.Term = entry.Term
		psm.state.LastIndex, psm.state.LogTerm = psm.log.append(psm.state.LastIndex, entry)
	} else {
		if psm.state.Leader == 0 {
			psm.state.ProposalContext.Index = 0
			psm.state.ProposalContext.Term = 0
			if psm.l() {
				psm.logger.Info("no leader")
			}
			return
		}
		psm.state.ProposalContext.TID++
		psm.state.ProposalContext.Index = 0
		psm.state.ProposalContext.Term = 0
		psm.sendChan <- buildProp(
			psm.state.Term, psm.state.ID, psm.state.Leader,
			psm.state.ProposalContext.TID, req.data)
	}
}

func (psm *protocolStateMachine) read(req readRequest) {
	psm.state.ReadContext.TID++

	if psm.state.Role == RoleLeader {
		// shortcut: 1-node cluster
		if psm.state.QuorumSize == 1 {
			psm.endPendingRead(psm.state.ReadContext.TID)
			return
		}
		// ping everyone
		psm.heartbeatWithContext(psm.state.ReadContext.TID, psm.state.ID)
	} else {
		// send read request to leader
		if psm.state.Leader == 0 {
			return
		}
		psm.sendChan <- buildRead(
			psm.state.Term, psm.state.ID, psm.state.Leader,
			psm.state.ReadContext.TID)
	}
}

func (psm *protocolStateMachine) becomeFollower() {
	if psm.l() {
		psm.logger.Info("becoming follower", zap.Uint64("term", psm.state.Term))
	}
	psm.heartbeatC = nil
	psm.electionTicker.Reset()
	psm.state.Role = RoleFollower
	psm.state.VotedFor = 0
	for _, p := range psm.members {
		p.VoteGranted = false
	}
}

func (psm *protocolStateMachine) becomeCandidate() {
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
	for _, p := range psm.members {
		if psm.state.ID == p.ID {
			p.VoteGranted = true
			continue
		}
		p.VoteGranted = false
		psm.sendChan <- buildVote(psm.state.Term, psm.state.ID, p.ID, lastEntry.Index, lastEntry.Term)
	}

	// shortcut: 1-node cluster
	if psm.state.QuorumSize == 1 {
		if psm.l() {
			psm.logger.Info("1-node cluster shortcut: become leader instantly")
		}
		psm.becomeLeader()
	}
}

func (psm *protocolStateMachine) becomeLeader() {
	if psm.l() {
		psm.logger.Info("becoming leader")
	}
	psm.state.Role = RoleLeader
	psm.state.Leader = psm.state.ID
	psm.state.VotedFor = 0
	for _, p := range psm.members {
		p.VoteGranted = false
		p.Next = psm.state.LastIndex + 1
		p.Ack = false
	}

	// Try to commit an (empty) entry from the newly elected term
	psm.state.LastIndex, psm.state.LogTerm = psm.log.append(psm.state.LastIndex, raftpb.Entry{
		Index: psm.state.LastIndex + 1,
		Term:  psm.state.Term,
	})
	psm.heartbeatWithEntries()
	psm.heartbeatTicker.Reset()
	psm.heartbeatC = psm.heartbeatTicker.C()
}

func (psm *protocolStateMachine) heartbeatWithEntries() {
	for _, p := range psm.members {
		if psm.state.ID == p.ID {
			continue
		}
		entries := []raftpb.Entry{}
		if p.Next <= psm.state.LastIndex {
			psm.log.RLock()
			entries = append(entries, psm.log.entries(p.Next, psm.state.LastIndex)...)
			psm.log.RUnlock()
		}
		prevEntry := psm.log.entry(p.Next - 1)
		psm.sendChan <- buildApp(
			psm.state.Term, psm.state.ID, p.ID,
			prevEntry.Index, prevEntry.Term, psm.state.Commit,
			entries,
			0, 0)
	}
}

func (psm *protocolStateMachine) heartbeatWithContext(unixNano int64, proxy uint64) {
	for _, p := range psm.members {
		if psm.state.ID == p.ID {
			continue
		}
		psm.sendChan <- buildApp(
			psm.state.Term, psm.state.ID, p.ID,
			0, 0, psm.state.Commit,
			[]raftpb.Entry{},
			unixNano, proxy)
	}
}

// Figure out the largest match index of a quorum so far.
func (psm *protocolStateMachine) quorumMatchIndex() uint64 {
	matches := psm.quorumMatchIndexBuffer
	i := 0
	for _, p := range psm.members {
		if psm.state.ID == p.ID {
			matches[i] = psm.state.LastIndex
		}
		matches[i] = p.Match
		i++
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i] > matches[j]
	})
	return matches[psm.state.QuorumSize-1]
}

func (psm *protocolStateMachine) endPendingProposal() {
	if psm.debug && psm.l() {
		psm.logger.Debug("ending potentially pending proposal")
	}
	resp := proposalResponse{
		index: psm.state.ProposalContext.Index,
		term:  psm.state.ProposalContext.Term,
	}
	select {
	case psm.propRespChan <- resp:
	default:
	}
	// zero out pending proposal
	psm.state.ProposalContext.Index = 0
	psm.state.ProposalContext.Term = 0
}

// ack (nil error) or cancel (non-nil error) any pending read requests
func (psm *protocolStateMachine) endPendingRead(tid int64) {
	if tid != psm.state.ReadContext.TID {
		if psm.debug && psm.l() {
			psm.logger.Debug("TID has moved on", zap.Int64("oldTID", tid), zap.Int64("newTID", psm.state.ReadContext.TID))
		}
		return
	}
	if psm.debug && psm.l() {
		psm.logger.Debug("ending pending read")
	}
	resp := readResponse{
		index: psm.state.ReadContext.Index,
	}
	select {
	case psm.readRespChan <- resp:
	default:
	}
	// zero out pending read
	psm.state.ReadContext.Acks = 0
	psm.state.ReadContext.Index = 0
}

// update commit and alert downstream application state machine
func (psm *protocolStateMachine) updateCommit(newCommit uint64) {
	if psm.debug && psm.l() {
		psm.logger.Debug(
			"updating commit",
			zap.Uint64("oldCommit", psm.state.Commit), zap.Uint64("newCommit", newCommit))
	}
	psm.state.Commit = newCommit
	psm.commitChan <- newCommit
	if psm.state.ProposalContext.Index <= psm.state.Commit {
		psm.endPendingProposal()
	}
}

func (psm *protocolStateMachine) start() {
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

func (psm *protocolStateMachine) stop() {
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

func (psm *protocolStateMachine) l() bool {
	return psm.logger != nil
}

type proposalRequest struct {
	data []byte
}
type proposalResponse struct {
	index, term uint64
}
type readRequest struct{}
type readResponse struct {
	index uint64
}
type membersRequest struct{}
type stateReq struct{}
