package raft_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
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

func ExampleApplication_register() {
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	id := uint64(1)
	addresses := map[uint64]string{
		1: fmt.Sprintf("unix://%s", filepath.Join(tmpDir, "ExampleApplication_register.sock")),
	}

	// Start up the Raft node.
	r := &register{}
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
	r.node = node
	node.Start()
	defer node.Stop()

	// Wait for leader election
	time.Sleep(2 * time.Second)

	// Commit an entry via the register IDed 1
	if err := r.set(context.Background(), 42); err != nil {
		log.Fatalf("set failed: %v", err)
	}

	// Get that committed entry via register IDed 2
	if x, err := r.get(context.Background()); err == nil {
		fmt.Println(x)
		// Output: 42
	} else {
		log.Fatalf("get failed: %v", err)
	}
}
