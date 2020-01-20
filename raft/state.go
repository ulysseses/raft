package raft

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap/zapcore"
)

// Consistency is the consistency mode that Raft operations should support.
type Consistency uint8

const (
	// ConsistencySerializable follows the serializable consistency model.
	ConsistencySerializable Consistency = iota
	// ConsistencyLinearizable follows the linearizable consistency model.
	ConsistencyLinearizable
)

func (c Consistency) String() string {
	switch c {
	case ConsistencySerializable:
		return "serializable"
	case ConsistencyLinearizable:
		return "linearizable"
	default:
		panic("")
	}
}

// MarshalJSON implements json.Marshaler for Consistency
func (c Consistency) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, c.String())), nil
}

// UnmarshalJSON implements json.Unmarshaler for Consistency
func (c *Consistency) UnmarshalJSON(b []byte) error {
	var j string
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	switch strings.ToLower(j) {
	case "serializable":
		*c = ConsistencySerializable
	case "linearizable":
		*c = ConsistencyLinearizable
	default:
		return fmt.Errorf("unrecognized consistency: %s", j)
	}
	return nil
}

// Role can be follower, candidate, or leader.
type Role uint8

const (
	// RoleFollower is the follower role.
	RoleFollower Role = iota
	// RoleCandidate is the candidate role.
	RoleCandidate
	// RoleLeader is the leader role.
	RoleLeader
)

func (r Role) String() string {
	switch r {
	case RoleFollower:
		return "follower"
	case RoleCandidate:
		return "candidate"
	case RoleLeader:
		return "leader"
	default:
		panic("")
	}
}

// MarshalJSON implements json.Marshaler for Role
func (r Role) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, r.String())), nil
}

// UnmarshalJSON implements json.Unmarshaler for Role
func (r *Role) UnmarshalJSON(b []byte) error {
	var j string
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	switch strings.ToLower(j) {
	case "follower":
		*r = RoleFollower
	case "candidate":
		*r = RoleCandidate
	case "leader":
		*r = RoleLeader
	default:
		return fmt.Errorf("unrecognized role: %s", j)
	}
	return nil
}

// State contains all state of a Node.
type State struct {
	// id
	ID uint64

	// Consistency mode
	Consistency Consistency

	// quorum size
	QuorumSize int

	// cluster size
	ClusterSize int

	// role
	Role Role

	// current term
	Term uint64

	// who this node thinks currently is the leader.
	Leader uint64

	// committed index
	Commit uint64

	// who this node last voted for
	VotedFor uint64

	// last index of this node's log
	LastIndex uint64

	// largest term of this node's log
	LogTerm uint64

	// context of a read request originating from this node, if any.
	ReadContext ReadContext

	// context of a proposal, if any.
	ProposalContext ProposalContext
}

// MarshalLogObject implements zap.Marshaler for State.
func (s State) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint64("id", s.ID)
	enc.AddString("consistency", s.Consistency.String())
	enc.AddInt("quorumSize", s.QuorumSize)
	enc.AddInt("clusterSize", s.ClusterSize)
	enc.AddString("role", s.Role.String())
	enc.AddUint64("term", s.Term)
	enc.AddUint64("leader", s.Leader)
	enc.AddUint64("commit", s.Commit)
	enc.AddUint64("votedFor", s.VotedFor)
	enc.AddUint64("lastIndex", s.LastIndex)
	enc.AddUint64("logTerm", s.LogTerm)
	enc.AddObject("readContext", s.ReadContext)
	enc.AddObject("proposalContext", s.ProposalContext)
	return nil
}

// MemberState contains all info about a member node from the perspective of
// this node.
type MemberState struct {
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

	// ReadContext of this member.
	ReadContext ReadContext
}

// MarshalLogObject implements zap.Marshaler for MemberState.
func (m MemberState) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint64("id", m.ID)
	enc.AddUint64("match", m.Match)
	enc.AddUint64("next", m.Next)
	enc.AddBool("ack", m.Ack)
	enc.AddBool("voteGranted", m.VoteGranted)
	enc.AddObject("readContext", m.ReadContext)
	return nil
}

// ReadContext contains all fields relevant to read requests.
type ReadContext struct {
	// TID is the "transaction ID". It increases monotonically.
	TID int64

	// Index is the read index of the read request.
	Index uint64

	// Acks is the number of acks for the latest read request.
	Acks int
}

// MarshalLogObject implements zap.Marshaler for ReadContext.
func (rc ReadContext) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddInt64("tid", rc.TID)
	enc.AddUint64("index", rc.Index)
	enc.AddInt("acks", rc.Acks)
	return nil
}

// ProposalContext is the context associated with a proposal.
type ProposalContext struct {
	TID int64

	// Index is the proposed index.
	// Term is the proposed term.
	Index, Term uint64
}

// MarshalLogObject implements zap.Marshaler for ProposalContext.
func (pc ProposalContext) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddInt64("tid", pc.TID)
	enc.AddUint64("index", pc.Index)
	enc.AddUint64("term", pc.Term)
	return nil
}
