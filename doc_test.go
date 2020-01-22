package raft_test

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/ulysseses/raft"
	"github.com/ulysseses/raft/pb"
)

// register is a single-valued store
type register struct {
	sync.RWMutex
	node *raft.Node
	x    int
}

// Apply implements Application for register
func (r *register) Apply(entries []pb.Entry) error {
	r.Lock()
	defer r.Unlock()
	x, err := strconv.Atoi(string(entries[len(entries)-1].Data))
	if err != nil {
		return err
	}
	r.x = x
	return nil
}

func (r *register) append(ctx context.Context, x int) error {
	s := strconv.Itoa(x)
	_, _, err := r.node.Propose(ctx, []byte(s))
	return err
}

func (r *register) getLatest(ctx context.Context) (int, error) {
	if err := r.node.Read(ctx); err != nil {
		return 0, err
	}
	r.RLock()
	defer r.RUnlock()
	return r.x, nil
}

func ExampleApplication_register() {
	addresses := map[uint64]string{
		1: "tcp://localhost:8001",
		2: "tcp://localhost:8002",
		3: "tcp://localhost:8003",
	}
	registers := map[uint64]*register{}

	// Start up the Raft cluster.
	for id := range addresses {
		r := &register{}
		registers[id] = r

		tr, err := raft.NewTransportConfig(id, addresses).Build()
		if err != nil {
			log.Fatal(err)
		}
		psm, err := raft.NewProtocolConfig(id).Build(tr)
		if err != nil {
			log.Fatal(err)
		}
		node, err := raft.NewNodeConfig(id).Build(psm, tr, r)
		if err != nil {
			log.Fatal(err)
		}
		node.Start()
		defer node.Stop()
		r.node = node
	}

	// wait a bit for there to be a leader
	time.Sleep(3 * time.Second)

	// Commit an entry via the register IDed 1
	_ = registers[1].append(context.Background(), 42)

	// Get that committed entry via register IDed 2
	fmt.Println(registers[2].getLatest(context.Background()))
	// Output: 42
}
