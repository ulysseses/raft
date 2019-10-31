package raft

import (
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/ulysseses/raft/raftpb"
)

type role int

const (
	roleFollower role = iota
	roleCandidate
	roleLeader
)

type raft struct {
	// buffered and owned by transport
	recvChan chan *raftpb.Message
	// buffered and owned by transport
	sendChan chan *raftpb.Message

	// unbuffered
	applyChan chan []byte
	// unbuffered
	applyAckChan chan struct{}

	leader uint64
	// unbuffered
	proposeChan chan []byte

	peers                   []uint64
	minElectionTimeoutTicks int
	maxElectionTimeoutTicks int
	heartbeatTicks          int
	tickMs                  int
	electionTimeoutTimer    *timer
	heartbeatTimer          *timer

	// initialized to roleFollower
	role role
	// initialized to lastIndex + 1
	nextIndex map[uint64]uint64
	// initialized to 0
	matchIndex map[uint64]uint64

	id uint64
	// initially contains a dummy entry of index 1 and term 1
	log []*raftpb.Entry
	// initially at 1 (for the dummy entry)
	startIndex uint64
	// starts off at 1
	term    uint64
	applied uint64
	// starts off at 1
	committed uint64

	votes      map[uint64]bool
	votedFor   uint64
	quorumSize int

	stop chan struct{}
	// send-once channel indicating initial leader elected
	initialLeaderElectedSignalChan chan struct{}
	initialLeaderElected           bool
}

// Rules for Servers
//
// All Servers:
//   * If commitIndex > lastApplied: increment lastApplied, apply
//     log[lastApplied] to state machine (3.5)
//   * If RPC request or response contains term T > currentTerm:
//     set currentTerm = T, convert to follower (3.3)
// Followers:
//   * Respond to RPCs from candidates and leaders
//   * If election timeout elapses without receiving AppendEntries
//     RPC from current leader or granting vote to candidate:
//     convert to candidate
// Candidates:
//   * On conversion to candidate, start election:
//     - Increment currentTerm
//     - Vote for self
//     - Reset election timer
//     - Send RequestVote RPCs to all other servers
//   * If votes received from majority of servers: become leader
//   * If AppendEntries RPC received from new leader: convert to
//     follower
//   * If election timeout elapses, start new election
// Leaders:
//   * Upon election: send initial empty AppendEntries RPC
//     (heartbeat) to each server; repeat during idle periods to
//     prevent election timeouts (3.4)
//   * If command received from client: append entry to local log,
//     respond after entry applied to state machine (3.5)
//   * If last log index >= nextIndex for a follower, send
//     AppendEntries RPC with log entries starting at nextIndex
//     - If successful: update nextIndex and matchIndex for
//       follower (3.5)
//     - If AppendEntries fails because of log inconsistency:
//       decrement nextIndex and retry (3.5)
//   * If there exists an N such that N > commitIndex, a majority
//     of matchIndex[i] >= N, and log[N].term == currentTerm:
//     set commitIndex = N (3.5, 3.6)
// RequestVote RPC:
// Receiver Implementation:
//   1. Reply false if term < currentTerm (3.3)
//   2. If votedFor is null or candidateId, and candidate's log is at
//      least as up-to-date as receiver's log, grant vote (3.4, 3.6)
// AppendEntries RPC:
// Receiver Implementation:
//   1. Reply false if term < currentTerm (3.3)
//   2. Reply false if log doesn't contain an entry at prevLogIndex
//      whose term matches prevLogTerm (3.5)
//   3. If an existing entry conflicts with a new one (same index
//      but different terms), delete the existing entry and all that
//      follow it (3.5)
//   4. Append any new entries not already in the log
//   5. If leaderCommit > commitIndex, set commitIndex =
//      min(leaderCommit, index of last new entry)
func (r *raft) loop() {
	var (
		recvChan                       <-chan *raftpb.Message = r.recvChan
		sendChan                       chan<- *raftpb.Message = r.sendChan
		applyChan                      chan<- []byte          = r.applyChan
		applyAckChan                   <-chan struct{}        = r.applyAckChan
		stop                           <-chan struct{}        = r.stop
		initialLeaderElectedSignalChan chan<- struct{}        = r.initialLeaderElectedSignalChan

		dataToApply []byte = nil
	)
	for {
		// All Servers:
		//   * If commitIndex > lastApplied: increment lastApplied, apply
		//     log[lastApplied] to state machine (3.5)  [PART 1]
		if dataToApply == nil && r.committed > r.applied {
			dataToApply = append([]byte{}, r.log[r.applied-r.startIndex+1].Data...)
		}
		if dataToApply != nil {
			select {
			case applyChan <- dataToApply:
			default:
			}
		}

		select {
		case <-stop:
			return
		case proposedData := <-r.proposeChan:
			// Leaders:
			//   * If command received from client: append entry to local log,
			//     respond after entry applied to state machine (3.5)
			if r.role == roleLeader {
				entry := &raftpb.Entry{
					Term:  r.term,
					Index: r.startIndex + uint64(len(r.log)),
					Data:  proposedData,
				}
				r.log = append(r.log, entry)
				// shortcut
				if r.quorumSize == 1 {
					r.committed++
				}
			} else if r.role == roleFollower {
				innerReq := &raftpb.ProposeRequest{
					Data: proposedData,
				}
				req := buildProposeRequest(
					r.term, r.id, r.leader,
					innerReq)
				// send proposal to leader, may fail in transport
				// TODO: maybe implement req/resp to prevent dropped proposals?
				sendChan <- req
			} else {
				// drop the proposal since there is no leader
				// TODO: maybe buffer the proposal until a leader exists?
				fmt.Printf("dropped proposal on candidate id = %d\n", r.id)
			}
		case <-r.heartbeatTimer.C:
			// Leaders:
			//   * Upon election: send initial empty AppendEntries RPC
			//     (heartbeat) to each server; repeat during idle periods to
			//     prevent election timeouts (3.4)
			//   * If last log index >= nextIndex for a follower, send
			//     AppendEntries RPC with log entries starting at nextIndex
			if r.role != roleLeader {
				continue
			}
			r.resetHeartbeatTimeout()
			for _, peerID := range r.peers {
				if peerID == r.id {
					continue
				}
				nextIndex := r.nextIndex[peerID]
				entries := append([]*raftpb.Entry{}, r.log[nextIndex-r.startIndex:]...)
				innerReq := &raftpb.AppendRequest{
					PrevIndex: nextIndex - 1,
					PrevTerm:  r.log[nextIndex-1-r.startIndex].Term,
					Entries:   entries,
					Committed: r.committed,
				}
				req := buildAppendRequest(
					r.term, r.id, peerID,
					innerReq)
				sendChan <- req
			}
		case <-r.electionTimeoutTimer.C:
			// Followers:
			//   * If election timeout elapses without receiving AppendEntries
			//     RPC from current leader or granting vote to candidate:
			//     convert to candidate
			// Candidates:
			//   * If election timeout elapses, start new election
			// Candidates:
			//   * On conversion to candidate, start election:
			//     - Increment currentTerm
			//     - Vote for self
			//     - Reset election timer
			//     - Send RequestVote RPCs to all other servers
			r.resetElectionTimeout()
			r.becomeCandidate()
		case <-applyAckChan:
			// All Servers:
			//   * If commitIndex > lastApplied: increment lastApplied, apply
			//     log[lastApplied] to state machine (3.5)  [PART 2]
			if dataToApply == nil {
				panic("got apply ack but wasn't expecting one")
			}
			r.applied++
			dataToApply = nil
		case msg := <-recvChan:
			// All Servers:
			//   * If RPC request or response contains term T > currentTerm:
			//     set currentTerm = T, convert to follower (3.3)
			if msg.Term > r.term {
				r.term = msg.Term
				r.becomeFollower()
			} else if msg.Term < r.term {
				continue
			}

			if r.role == roleCandidate {
				// Candidates:
				//   * If votes received from majority of servers: become leader
				//   * If AppendEntries RPC received from new leader: convert to
				//     follower
				switch msg.Message.(type) {
				case *raftpb.Message_VoteResponse:
					resp := getVoteResponse(msg)
					if resp.Granted {
						r.votes[msg.Sender] = true
					}
					voteCount := 0
					for _, granted := range r.votes {
						if granted {
							voteCount++
						}
					}
					if voteCount >= r.quorumSize {
						// Leaders:
						//   * Upon election: send initial empty AppendEntries RPC
						//     (heartbeat) to each server; repeat during idle periods to
						//     prevent election timeouts (3.4)
						r.becomeLeader()
					}
				case *raftpb.Message_AppendRequest:
					r.becomeFollower()
				default:

				}
			}

			if r.role == roleFollower {
				// Followers:
				//   * Respond to RPCs from candidates and leaders
				switch msg.Message.(type) {
				case *raftpb.Message_VoteRequest:
					// RequestVote RPC:
					// Receiver Implementation:
					//   1. Reply false if term < currentTerm (3.3)
					//   2. If votedFor is null or candidateId, and candidate's log is at
					//      least as up-to-date as receiver's log, grant vote (3.4, 3.6)
					if _, ok := msg.Message.(*raftpb.Message_VoteRequest); ok {
						req := getVoteRequest(msg)
						if msg.Term >= r.term &&
							(r.votedFor == 0 || r.votedFor == msg.Sender) &&
							(msg.Term > r.term || req.Index >= r.log[len(r.log)-1].Index) {
							r.votedFor = msg.Sender
						}
						resp := buildVoteResponse(
							r.term, r.id, msg.Sender,
							&raftpb.VoteResponse{Granted: r.votedFor == msg.Sender})
						sendChan <- resp
					}
				case *raftpb.Message_AppendRequest:
					// AppendEntries RPC:
					// Receiver Implementation:
					r.registerLeader(msg.Sender)
					req := getAppendRequest(msg)

					//   1. Reply false if term < currentTerm (3.3)
					//   2. Reply false if log doesn't contain an entry at prevLogIndex
					//      whose term matches prevLogTerm (3.5)
					if msg.Term >= r.term && req.PrevIndex < r.startIndex {
						panic(fmt.Sprintf(
							"got prev_index = %d, but startIndex = %d... msg:\n%s",
							req.PrevIndex, r.startIndex, msg.String()))
					}

					success := msg.Term >= r.term &&
						req.PrevIndex <= r.startIndex+uint64(len(r.log)-1) &&
						req.PrevTerm == r.log[req.PrevIndex-r.startIndex].Term

					//   3. If an existing entry conflicts with a new one (same index
					//      but different terms), delete the existing entry and all that
					//      follow it (3.5)
					//   4. Append any new entries not already in the log
					if success {
						lastI := req.PrevIndex - r.startIndex
						r.log = append(r.log[:lastI+1], req.Entries...)
					}

					//   5. If leaderCommit > commitIndex, set commitIndex =
					//      min(leaderCommit, index of last new entry)
					if req.Committed > r.committed {
						lastIndex := r.log[len(r.log)-1].Index
						if req.Committed < lastIndex {
							r.committed = req.Committed
						} else {
							r.committed = lastIndex
						}
					}

					resp := buildAppendResponse(
						r.term, r.id, msg.Sender,
						&raftpb.AppendResponse{
							Success: success,
							Index:   r.log[len(r.log)-1].Index,
						},
					)
					sendChan <- resp
					r.leader = msg.Sender
					if !r.initialLeaderElected {
						r.initialLeaderElected = true
						initialLeaderElectedSignalChan <- struct{}{}
					}
				default:

				}
			}

			if r.role == roleLeader {
				// Leaders:
				//     - If successful: update nextIndex and matchIndex for
				//       follower (3.5)
				//     - If AppendEntries fails because of log inconsistency:
				//       decrement nextIndex and retry (3.5)
				switch msg.Message.(type) {
				case *raftpb.Message_AppendResponse:
					resp := getAppendResponse(msg)
					if resp.Success {
						r.nextIndex[msg.Sender] = resp.Index + 1
						r.matchIndex[msg.Sender] = resp.Index
					} else {
						r.nextIndex[msg.Sender]--
					}

					// Leaders:
					//   * If there exists an N such that N > commitIndex, a majority
					//     of matchIndex[i] >= N, and log[N].term == currentTerm:
					//     set commitIndex = N (3.5, 3.6)
					type kv struct{ k, v uint64 }
					kvs := []kv{}
					for k, v := range r.matchIndex {
						kvs = append(kvs, kv{k, v})
					}
					sort.Slice(kvs, func(i, j int) bool {
						return kvs[i].v < kvs[j].v
					})
					n := kvs[len(kvs)-r.quorumSize].v
					if r.log[n-r.startIndex].Term == r.term {
						r.committed = n
					}
				case *raftpb.Message_ProposeRequest:
					req := getProposeRequest(msg)
					// go func() { r.proposeChan <- req.Data }() // TODO: better off doing inlining of propose
					entry := &raftpb.Entry{
						Term:  r.term,
						Index: r.log[len(r.log)-1].Index + 1,
						Data:  req.Data,
					}
					r.log = append(r.log, entry)
					if r.quorumSize == 1 {
						// shortcut
						r.committed++
					}
				default:

				}
			}
		}
	}
}

func (r *raft) resetHeartbeatTimeout() {
	r.heartbeatTimer.Reset(time.Duration(r.heartbeatTicks*r.tickMs) * time.Millisecond)
}

func (r *raft) resetElectionTimeout() {
	electionTimeoutTicks := rand.Intn(r.maxElectionTimeoutTicks-r.minElectionTimeoutTicks) + r.minElectionTimeoutTicks
	electionTimeoutMs := time.Duration(electionTimeoutTicks*r.tickMs) * time.Millisecond
	r.electionTimeoutTimer.Reset(electionTimeoutMs)
}

func (r *raft) becomeLeader() {
	r.electionTimeoutTimer.Stop()
	r.resetHeartbeatTimeout()
	r.role = roleLeader
	r.leader = r.id
	r.votes = map[uint64]bool{}
	r.votedFor = 0
	r.matchIndex[r.id] = r.startIndex + uint64(len(r.log)) - 1
	r.nextIndex[r.id] = r.matchIndex[r.id] + 1
	if !r.initialLeaderElected {
		r.initialLeaderElected = true
		r.initialLeaderElectedSignalChan <- struct{}{}
	}

	for _, peerID := range r.peers {
		if peerID == r.id {
			continue
		}
		innerReq := &raftpb.AppendRequest{
			PrevIndex: r.nextIndex[peerID] - 1,
			PrevTerm:  r.log[r.nextIndex[peerID]-1-r.startIndex].Term,
			Committed: r.committed,
		}
		req := buildAppendRequest(
			r.term, r.id, peerID,
			innerReq)
		r.sendChan <- req
	}
}

func (r *raft) becomeCandidate() {
	r.role = roleCandidate
	r.leader = 0
	r.term++
	r.votedFor = r.id
	r.votes = map[uint64]bool{r.id: true}

	// shortcut
	if r.quorumSize == 1 {
		r.becomeLeader()
	}

	for _, peerID := range r.peers {
		if peerID == r.id {
			continue
		}
		innerReq := &raftpb.VoteRequest{
			Index: r.startIndex + uint64(len(r.log)) - 1,
		}
		req := buildVoteRequest(
			r.term, r.id, peerID,
			innerReq)
		r.sendChan <- req
	}
}

func (r *raft) becomeFollower() {
	r.role = roleFollower
	r.leader = 0
	r.votedFor = 0
	if len(r.votes) > 1 {
		r.votes = map[uint64]bool{}
	}
}

func (r *raft) registerLeader(leader uint64) {
	r.leader = leader
	r.resetElectionTimeout()
	r.heartbeatTimer.Stop()
	if !r.initialLeaderElected {
		r.initialLeaderElected = true
		r.initialLeaderElectedSignalChan <- struct{}{}
	}
}

type raftFacade interface {
	raftApplicationFacade
	raftTransportFacade
}

type raftApplicationFacade interface {
	apply() <-chan []byte
	applyAck() chan<- struct{}
	propose() chan<- []byte
}

type raftTransportFacade interface {
	recv() chan<- *raftpb.Message
	send() <-chan *raftpb.Message
}

func (r *raft) apply() <-chan []byte {
	return r.applyChan
}

func (r *raft) applyAck() chan<- struct{} {
	return r.applyAckChan
}

func (r *raft) propose() chan<- []byte {
	return r.proposeChan
}

func (r *raft) recv() chan<- *raftpb.Message {
	return r.recvChan
}

func (r *raft) send() <-chan *raftpb.Message {
	return r.sendChan
}

func getAppendRequest(msg *raftpb.Message) *raftpb.AppendRequest {
	return msg.Message.(*raftpb.Message_AppendRequest).AppendRequest
}

func getAppendResponse(msg *raftpb.Message) *raftpb.AppendResponse {
	return msg.Message.(*raftpb.Message_AppendResponse).AppendResponse
}

func getVoteRequest(msg *raftpb.Message) *raftpb.VoteRequest {
	return msg.Message.(*raftpb.Message_VoteRequest).VoteRequest
}

func getVoteResponse(msg *raftpb.Message) *raftpb.VoteResponse {
	return msg.Message.(*raftpb.Message_VoteResponse).VoteResponse
}

func getProposeRequest(msg *raftpb.Message) *raftpb.ProposeRequest {
	return msg.Message.(*raftpb.Message_ProposeRequest).ProposeRequest
}

func buildAppendRequest(term, sender, recipient uint64, req *raftpb.AppendRequest) *raftpb.Message {
	return &raftpb.Message{
		Term:      term,
		Sender:    sender,
		Recipient: recipient,
		Message: &raftpb.Message_AppendRequest{
			AppendRequest: req,
		},
	}
}

func buildAppendResponse(term, sender, recipient uint64, resp *raftpb.AppendResponse) *raftpb.Message {
	return &raftpb.Message{
		Term:      term,
		Sender:    sender,
		Recipient: recipient,
		Message: &raftpb.Message_AppendResponse{
			AppendResponse: resp,
		},
	}
}

func buildVoteRequest(term, sender, recipient uint64, req *raftpb.VoteRequest) *raftpb.Message {
	return &raftpb.Message{
		Term:      term,
		Sender:    sender,
		Recipient: recipient,
		Message: &raftpb.Message_VoteRequest{
			VoteRequest: req,
		},
	}
}

func buildVoteResponse(term, sender, recipient uint64, resp *raftpb.VoteResponse) *raftpb.Message {
	return &raftpb.Message{
		Term:      term,
		Sender:    sender,
		Recipient: recipient,
		Message: &raftpb.Message_VoteResponse{
			VoteResponse: resp,
		},
	}
}

func buildProposeRequest(term, sender, recipient uint64, req *raftpb.ProposeRequest) *raftpb.Message {
	return &raftpb.Message{
		Term:      term,
		Sender:    sender,
		Recipient: recipient,
		Message: &raftpb.Message_ProposeRequest{
			ProposeRequest: req,
		},
	}
}
