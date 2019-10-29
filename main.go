package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ulysseses/raft/raft"
)

func myConfiguration(id uint64) (*raft.Configuration, error) {
	// if id < 1 || id > 3 {
	// 	return nil, fmt.Errorf("id must be in [1, 3] range. Got: %d", id)
	// }
	return &raft.Configuration{
		RecvChanSize: 100,
		SendChanSize: 100,
		ID:           id,
		Peers: map[uint64]string{
			111: "localhost:5551",
			222: "localhost:5552",
			333: "localhost:5553",
		},
		MinElectionTimeoutTicks: 10,
		MaxElectionTimeoutTicks: 20,
		HeartbeatTicks:          1,
		TickMs:                  10,
	}, nil
}

func exitOnError(err error) {
	if err != nil {
		fmt.Printf("%v", err)
		os.Exit(1)
	}
}

func main() {
	// spin up 3 raft nodes
	raftNodes := []*raft.Node{}
	type result struct {
		n   *raft.Node
		err error
	}
	var wg sync.WaitGroup
	wg.Add(3)
	results := make(chan result, 3)
	for i := 1; i <= 3; i++ {
		go func(i uint64) {
			defer wg.Done()
			config, err := myConfiguration(uint64(i))
			if err != nil {
				results <- result{nil, err}
				return
			}
			raftNode, err := raft.NewNode(config)
			results <- result{raftNode, err}
		}(uint64(i * 111))
	}
	wg.Wait()
	close(results)
	for r := range results {
		exitOnError(r.err)
		defer r.n.Stop()
		raftNodes = append(raftNodes, r.n)
	}

	// test run
	raftNode := raftNodes[0]
	kvStore := raftNode.KVStore
	i := 0
	const k = "someKey"
	for i < 50 {
		want := fmt.Sprintf("%d", i)
		kvStore.Propose(k, want)
		for {
			time.Sleep(10 * time.Millisecond)
			v, ok := kvStore.Get(k)
			if !ok || v != want {
				fmt.Println("Haven't seen new proposed value yet...")
			} else {
				fmt.Printf("Got %s.\n", v)
				break
			}
		}
		i++
	}
}
