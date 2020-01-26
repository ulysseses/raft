package raft

import (
	"context"
	"strconv"
	"sync"

	"github.com/ulysseses/raft/pb"
)

// Application applies the committed raft entries. Applications interfacing with
// the Raft node must implement this interface.
type Application interface {
	// Apply applies the newly committed entries to the application.
	Apply(entries []pb.Entry) error
}

type noOpApp struct{}

// Apply implements Application for noOpApp.
func (app *noOpApp) Apply(entries []pb.Entry) error {
	return nil
}

func newNoOpApp(id uint64) Application {
	return &noOpApp{}
}

// register is a single-valued store
type register struct {
	sync.RWMutex
	node *Node
	x    int
}

// Apply implements Application for register
func (r *register) Apply(entries []pb.Entry) error {
	r.Lock()
	defer r.Unlock()
	for _, entry := range entries {
		if len(entry.Data) == 0 {
			continue
		}
		x, err := strconv.Atoi(string(entry.Data))
		if err != nil {
			return err
		}
		r.x = x
	}
	return nil
}

func (r *register) set(ctx context.Context, x int) error {
	s := strconv.Itoa(x)
	_, _, err := r.node.Propose(ctx, []byte(s))
	return err
}

func (r *register) get(ctx context.Context) (int, error) {
	if err := r.node.Read(ctx); err != nil {
		return 0, err
	}
	r.RLock()
	defer r.RUnlock()
	return r.x, nil
}
