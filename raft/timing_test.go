// +build perf

package raft

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func BenchmarkOneNodePutGetLockStep(b *testing.B) {
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	config := &Configuration{
		RecvChanSize: 100,
		SendChanSize: 100,
		ID:           111,
		Peers: map[uint64]string{
			111: fmt.Sprintf("unix://%s", filepath.Join(tmpDir, "BenchmarkOneNodePutGetLockStep.sock")),
		},
		MinElectionTimeoutTicks: 10,
		MaxElectionTimeoutTicks: 20,
		HeartbeatTicks:          1,
		TickMs:                  2,
	}

	const key = "someKey"
	for i := 0; i < b.N; i++ {
		//////////////////////////////////////////////////////////////////////
		// Prologue
		raftNode, err := NewNode(config)
		if err != nil {
			log.Fatal(err)
		}
		kvStore := raftNode.KVStore
		b.StartTimer()
		//////////////////////////////////////////////////////////////////////

		j := 1
		for j < 21 {
			want := fmt.Sprintf("%d", j)
			kvStore.Propose(key, want)
			k := 0
			for k < 10 {
				time.Sleep(5 * time.Millisecond)
				v, ok := kvStore.Get(key)
				if ok && v == want {
					break
				}
				k++
			}
			if k < 10 {
				j++
			}
		}

		//////////////////////////////////////////////////////////////////////
		// Epilogue
		b.StopTimer()
		raftNode.Stop()
		//////////////////////////////////////////////////////////////////////
	}
}
