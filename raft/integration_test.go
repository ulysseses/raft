// +build integration

package raft

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
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
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	config := &Configuration{
		RecvChanSize: 100,
		SendChanSize: 100,
		ID:           111,
		Peers: map[uint64]string{
			111: fmt.Sprintf("unix://%s", filepath.Join(tmpDir, "TestThreeNodeLinear111.sock")),
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
				t.Log("Haven't seen new proposed value yet...")
			} else {
				t.Logf("Got %s.\n", v)
				break
			}
			j++
		}
		if j == 10 {
			t.Logf("Proposal %d dropped. Proposing it again.\n", i)
		} else {
			i++
		}
	}
}

func TestThreeNodeLinear(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	myConfiguration := func(id uint64) (*Configuration, error) {
		if id != 111 && id != 222 && id != 333 {
			return nil, fmt.Errorf("id must be 111, 222, or 333; got %d", id)
		}
		return &Configuration{
			RecvChanSize: 100,
			SendChanSize: 100,
			ID:           id,
			Peers: map[uint64]string{
				111: fmt.Sprintf("unix://%s", filepath.Join(tmpDir, "TestThreeNodeLinear111.sock")),
				222: fmt.Sprintf("unix://%s", filepath.Join(tmpDir, "TestThreeNodeLinear222.sock")),
				333: fmt.Sprintf("unix://%s", filepath.Join(tmpDir, "TestThreeNodeLinear333.sock")),
			},
			MinElectionTimeoutTicks: 10,
			MaxElectionTimeoutTicks: 20,
			HeartbeatTicks:          1,
			TickMs:                  2,
		}, nil
	}

	setup3Nodes := func(t *testing.T) ([]*Node, []func() error) {
		raftNodes := []*Node{}
		stoppers := []func() error{}
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
			stoppers = append(stoppers, res.n.Stop)
			raftNodes = append(raftNodes, res.n)
		}
		return raftNodes, stoppers
	}

	test := func(kvStoreW, kvStoreR *KVStore, done chan struct{}) {
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
					t.Log("Haven't seen new proposed value yet...")
				} else {
					t.Logf("Got %s.\n", v)
					break
				}
				j++
			}
			if j == 10 {
				t.Logf("Proposal %d dropped. Proposing it again.\n", i)
			} else {
				i++
			}
		}
		close(done)
	}

	tester := func(
		t *testing.T,
		kvStoreW, kvStoreR *KVStore,
		timeout time.Duration,
	) {
		done := make(chan struct{})
		go test(kvStoreW, kvStoreR, done)
		select {
		case <-time.After(timeout):
			t.Error("timed out")
		case <-done:
		}
	}

	// DO NOT RUN IN PARALLEL
	t.Run("writer is leader, reader is follower", func(t *testing.T) {
		raftNodes, closers := setup3Nodes(t)
		for _, closer := range closers {
			defer closer()
		}

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
			if node.raft.getRole() == roleLeader {
				kvStoreW = node.KVStore
				count++
			} else if node.raft.getRole() == roleFollower {
				kvStoreR = node.KVStore
				count++
			}
		}
		tester(t, kvStoreW, kvStoreR, 400*time.Millisecond)
	})
	t.Run("writer is leader, reader is leader", func(t *testing.T) {
		raftNodes, closers := setup3Nodes(t)
		for _, closer := range closers {
			defer closer()
		}

		var (
			kvStoreW, kvStoreR *KVStore
		)
		// hack: potential data race
		// TODO(ulysseses): remove data race potential
		for _, node := range raftNodes {
			if node.raft.getRole() == roleLeader {
				kvStoreW = node.KVStore
				kvStoreR = node.KVStore
				break
			}
		}
		tester(t, kvStoreW, kvStoreR, 400*time.Millisecond)
	})
	t.Run("writer is follower, reader is follower", func(t *testing.T) {
		raftNodes, closers := setup3Nodes(t)
		for _, closer := range closers {
			defer closer()
		}

		var (
			kvStoreW, kvStoreR *KVStore
		)
		// hack: potential data race
		// TODO(ulysseses): remove data race potential
		for _, node := range raftNodes {
			if node.raft.getRole() == roleFollower {
				kvStoreW = node.KVStore
				kvStoreR = node.KVStore
				break
			}
		}
		tester(t, kvStoreW, kvStoreR, 400*time.Millisecond)
	})
	t.Run("writer is follower, reader is leader", func(t *testing.T) {
		raftNodes, closers := setup3Nodes(t)
		for _, closer := range closers {
			defer closer()
		}

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
			if node.raft.getRole() == roleFollower {
				kvStoreW = node.KVStore
				count++
			} else if node.raft.getRole() == roleLeader {
				kvStoreR = node.KVStore
				count++
			}
		}
		tester(t, kvStoreW, kvStoreR, 400*time.Millisecond)
	})
	t.Run("leader is dropped and brought back online", func(t *testing.T) {
		raftNodes, stoppers := setup3Nodes(t)
		for _, stopper := range stoppers {
			defer stopper()
		}

		var (
			downNode               *Node
			downKVStore, upKVStore *KVStore
		)
		// TODO(ulysseses): potential data race if downNode no longer is leader
		// immediately after we assign downNode = node
		for _, node := range raftNodes {
			if node.raft.getRole() == roleLeader {
				downNode = node
				downKVStore = node.KVStore
			} else {
				upKVStore = node.KVStore
			}
		}

		go func() {
			time.Sleep(50 * time.Millisecond) // small initial delay
			downNode.pause()
			time.Sleep(50 * time.Millisecond) // > election timeout
			downNode.unpause()
		}()
		tester(t, downKVStore, upKVStore, time.Second)
	})
	t.Run("follower is dropped and brought back online", func(t *testing.T) {
		raftNodes, stoppers := setup3Nodes(t)
		for _, stopper := range stoppers {
			defer stopper()
		}

		var (
			downNode               *Node
			downKVStore, upKVStore *KVStore
		)

		// TODO(ulysseses): potential data race if downNode becomes leader
		// immediately after we assign downNode = node
		for _, node := range raftNodes {
			if node.raft.getRole() == roleFollower {
				downNode = node
				downKVStore = node.KVStore
			} else if node.raft.getRole() == roleLeader {
				upKVStore = node.KVStore
			}
		}

		go func() {
			time.Sleep(50 * time.Millisecond) // small initial delay
			downNode.pause()
			time.Sleep(50 * time.Millisecond) // > election timeout
			downNode.unpause()
		}()
		tester(t, downKVStore, upKVStore, time.Second)
	})
}
