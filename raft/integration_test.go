package raft

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func fatalOnError(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}

func TestOneNodeLinear(t *testing.T) {
	config := &Configuration{
		RecvChanSize: 100,
		SendChanSize: 100,
		ID:           111,
		Peers: map[uint64]string{
			111: "localhost:5551",
		},
		MinElectionTimeoutTicks: 10,
		MaxElectionTimeoutTicks: 20,
		HeartbeatTicks:          1,
		TickMs:                  2,
	}
	raftNode, err := NewNode(config)
	if err != nil {
		t.Fatal(err)
	}
	defer raftNode.Stop()

	kvStore := raftNode.KVStore
	const k = "someKey"
	i := 1
	for i < 21 {
		want := fmt.Sprintf("%d", i)
		kvStore.Propose(k, want)
		j := 0
		for j < 10 {
			time.Sleep(5 * time.Millisecond)
			v, ok := kvStore.Get(k)
			if !ok || v != want {
				fmt.Println("Haven't seen new proposed value yet...")
			} else {
				fmt.Printf("Got %s.\n", v)
				break
			}
			j++
		}
		if j == 10 {
			fmt.Printf("Proposal %d dropped. Proposing it again.\n", i)
		} else {
			i++
		}
	}
}

func TestThreeNodeLinear(t *testing.T) {
	myConfiguration := func(id uint64) (*Configuration, error) {
		if id != 111 && id != 222 && id != 333 {
			return nil, fmt.Errorf("id must be 111, 222, or 333; got %d", id)
		}
		return &Configuration{
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
			TickMs:                  2,
		}, nil
	}

	// spin up 3 raft nodes
	raftNodes := []*Node{}
	type result struct {
		n   *Node
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
			raftNode, err := NewNode(config)
			results <- result{raftNode, err}
		}(uint64(i * 111))
	}
	wg.Wait()
	close(results)
	for res := range results {
		if res.err != nil {
			t.Fatal(res.err)
		}
		defer res.n.Stop()
		raftNodes = append(raftNodes, res.n)
	}

	test := func(kvStoreW, kvStoreR *KVStore) {
		const k = "someKey"
		i := 1
		for i < 21 {
			want := fmt.Sprintf("%d", i)
			kvStoreW.Propose(k, want)
			j := 0
			for j < 10 {
				time.Sleep(5 * time.Millisecond)
				v, ok := kvStoreR.Get(k)
				if !ok || v != want {
					fmt.Println("Haven't seen new proposed value yet...")
				} else {
					fmt.Printf("Got %s.\n", v)
					break
				}
				j++
			}
			if j == 10 {
				fmt.Printf("Proposal %d dropped. Proposing it again.\n", i)
			} else {
				i++
			}
		}
	}

	// DO NOT RUN IN PARALLEL
	t.Run("writer is leader, reader is follower", func(t *testing.T) {
		var (
			kvStoreW, kvStoreR *KVStore
		)
		// hack: potential data race
		// TODO(ulysseses): remove data race potential
		count := 0
		for _, node := range raftNodes {
			if count == 2 {
				break
			}
			if node.raft.role == roleLeader {
				kvStoreW = node.KVStore
				count++
			} else if node.raft.role == roleFollower {
				kvStoreR = node.KVStore
				count++
			}
		}
		test(kvStoreW, kvStoreR)
	})
	t.Run("writer is leader, reader is leader", func(t *testing.T) {
		var (
			kvStoreW, kvStoreR *KVStore
		)
		// hack: potential data race
		// TODO(ulysseses): remove data race potential
		for _, node := range raftNodes {
			if node.raft.role == roleLeader {
				kvStoreW = node.KVStore
				kvStoreR = node.KVStore
				break
			}
		}
		test(kvStoreW, kvStoreR)
	})
	t.Run("writer is follower, reader is follower", func(t *testing.T) {
		var (
			kvStoreW, kvStoreR *KVStore
		)
		// hack: potential data race
		// TODO(ulysseses): remove data race potential
		for _, node := range raftNodes {
			if node.raft.role == roleFollower {
				kvStoreW = node.KVStore
				kvStoreR = node.KVStore
				break
			}
		}
		test(kvStoreW, kvStoreR)
	})
	t.Run("writer is follower, reader is leader", func(t *testing.T) {
		var (
			kvStoreW, kvStoreR *KVStore
		)
		// hack: potential data race
		// TODO(ulysseses): remove data race potential
		count := 0
		for _, node := range raftNodes {
			if count == 2 {
				break
			}
			if node.raft.role == roleFollower {
				kvStoreW = node.KVStore
				count++
			} else if node.raft.role == roleLeader {
				kvStoreR = node.KVStore
				count++
			}
		}
		test(kvStoreW, kvStoreR)
	})
}
