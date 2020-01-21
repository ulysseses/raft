package raft

import (
	"sync"

	"github.com/ulysseses/raft/raftpb"
)

// raftLog is a sequence of entries. The raftLog can be truncated and appended.
type raftLog struct {
	sync.RWMutex
	log []raftpb.Entry
}

// newLog returns a new empty raft log.
func newLog() *raftLog {
	return &raftLog{
		log: []raftpb.Entry{{}},
	}
}

// entry returns the entry at index `i`.
func (l *raftLog) entry(i uint64) raftpb.Entry {
	l.RLock()
	entry := l.log[i]
	l.RUnlock()
	return entry
}

// entries returns the slice of entries within the `[lo, hi]` index range.
// If `lo > hi`, the empty slice is returned
func (l *raftLog) entries(lo, hi uint64) []raftpb.Entry {
	if lo > hi {
		return nil
	}
	entries := l.log[lo : hi+1]
	return entries
}

func (l *raftLog) append(prev uint64, entries ...raftpb.Entry) (uint64, uint64) {
	l.Lock()
	l.log = append(l.log[:prev+1], entries...)
	lastEntry := l.log[len(l.log)-1]
	lastIndex := lastEntry.Index
	lastTerm := lastEntry.Term
	l.Unlock()
	return lastIndex, lastTerm
}
