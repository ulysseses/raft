package raft

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Consistency is the consistency mode that Raft operations should support.
type Consistency uint8

const (
	// ConsistencyLease follows the serializable consistency model. If there is a leadership
	// change, read requests are potentially stale for a maximum of the lease duration amount.
	ConsistencyLease Consistency = iota
	// ConsistencyStrict follows the linearizable consistency model. Every read request will
	// require a quorum's worth of heartbeat acks.
	ConsistencyStrict
	// ConsistencyStale follows the serializable consistency model.
	// Reads can be stale for an arbitrary amount of time. Writes never diverge.
	ConsistencyStale
)

func (c Consistency) String() string {
	switch c {
	case ConsistencyLease:
		return "lease"
	case ConsistencyStrict:
		return "strict"
	case ConsistencyStale:
		return "stale"
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
	case "lease":
		*c = ConsistencyLease
	case "strict":
		*c = ConsistencyStrict
	case "stale":
		*c = ConsistencyStale
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

	// context of a proposal, if any.
	ProposalContext ProposalContext

	// context of a read request originating from this node, if any.
	// This is used only by ConsistencyStrict.
	ReadContext ReadContext

	// lease, if using ConsistencyBoundedStale mode.
	Lease Lease
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

	// ReadContext of this member. This is only used by ConsistencyStrict.
	ReadContext ReadContext
}

// ProposalContext is the context associated with a proposal.
type ProposalContext struct {
	TID int64

	// Index is the proposed index.
	// Term is the proposed term.
	Index, Term uint64
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

// Lease contains fields related to lease for read requests. This is used only by leaders in
// ConsistencyBoundedStale mode.
type Lease struct {
	// Timeout is the point in time that a lease should timeout.
	Timeout int64

	// Start marks the beginning of the attempt to extend the lease, i.e. sending out new heartbeats
	Start int64

	// If a majority of heartbeats were acked, the Timeout should be extended to Start + Extension
	Extension int64

	// Acks is the number of acks for the specified Start.
	Acks int
}
