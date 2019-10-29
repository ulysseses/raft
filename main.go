package main

import (
	"fmt"
	"os"
	"time"

	"github.com/ulysseses/raft/raft"
)

var myConfiguration *raft.Configuration = &raft.Configuration{
	RecvChanSize: 100,
	SendChanSize: 100,
	ID:           1,
	Peers: map[uint64]string{
		1: "localhost:5551",
	},
	MinElectionTimeoutTicks: 10,
	MaxElectionTimeoutTicks: 20,
	HeartbeatTicks:          1,
	TickMs:                  2,
}

func exitOnError(err error) {
	if err != nil {
		fmt.Printf("%v", err)
		os.Exit(1)
	}
}

func main() {
	raftNode, err := raft.NewNode(myConfiguration)
	exitOnError(err)
	defer raftNode.Stop()

	kvStore := raftNode.KVStore

	// test run
	i := 0
	const k = "someKey"
	for i < 50 {
		want := fmt.Sprintf("%d", i)
		kvStore.Propose(k, want)
		fmt.Printf("Proposed %s\n", want)
		for {
			time.Sleep(1 * time.Millisecond)
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
