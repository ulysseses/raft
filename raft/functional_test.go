package raft

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ulysseses/raft/raftpb"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// fake Application
type fakeApplication struct{}

// Apply implements Application for fakeApplication.
func (app *fakeApplication) Apply(entries []raftpb.Entry) error {
	return nil
}

// remember to os.RemoveAll(tmpDir)
func createTmpDir() (string, error) {
	return ioutil.TempDir("", "")
}

func createSocket(dir, testName string, id uint64) string {
	return fmt.Sprintf("unix://%s%d.sock", filepath.Join(dir, testName), id)
}

func createConfigs(
	tmpl Configuration,
	dir, testName string,
	ids ...uint64,
) (map[uint64]Configuration, error) {
	logger := tmpl.Logger

	configs := map[uint64]Configuration{}
	tmpl.PeerAddresses = map[uint64]string{}
	for _, id := range ids {
		addr := createSocket(dir, testName, id)
		tmpl.PeerAddresses[id] = addr
	}
	for _, id := range ids {
		specificLogger := logger.With(zap.Uint64("id", id))
		tmpl.ID = id
		tmpl.Logger = specificLogger
		if err := tmpl.Verify(); err != nil {
			return nil, err
		}
		configs[id] = tmpl
	}
	return configs, nil
}

func BenchmarkTransport(b *testing.B) {
	unusedConsistency := ConsistencySerializable

	// Setup logger
	loggerCfg := zap.NewProductionConfig()
	// loggerCfg.Level.SetLevel(zapcore.DebugLevel)
	logger, err := loggerCfg.Build()
	if err != nil {
		b.Fatal(err)
	}

	// Spin up three nodes
	tmpDir, err := createTmpDir()
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	tmplConfig, err := BuildSensibleConfiguration(1, map[uint64]string{1: ""}, unusedConsistency, logger)
	if err != nil {
		b.Fatal(err)
	}
	tmplConfig.MsgBufferSize = 100
	configs, err := createConfigs(tmplConfig, tmpDir, b.Name(), 1, 2)
	if err != nil {
		b.Fatal(err)
	}
	transports := map[uint64]*transport{}
	for _, config := range configs {
		tr, err := newTransport(config)
		if err != nil {
			b.Fatal(err)
		}
		transports[config.ID] = tr
	}
	done := make(chan error)
	for _, tr := range transports {
		go func(tr *transport) {
			done <- tr.start()
		}(tr)
	}
	for i := uint64(1); i <= 2; i++ {
		<-done
	}
	close(done)

	// Start the benchmark
	tr1 := transports[1]
	tr2 := transports[2]
	msgIn := raftpb.Message{
		From: 1,
		To:   2,
	}
	var msgOut raftpb.Message
	b.SetBytes(int64(2 * msgIn.Size()))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// latency: ~21us per send-then-receive op
		// throughput: 95000 messages
		tr1.sendChan <- msgIn
		msgOut = <-tr2.recvChan
	}
	b.StopTimer()
	b.Log(msgIn.Size())
	b.Log(msgOut)

	// cleanup
	for _, tr := range transports {
		_ = tr.stop()
	}
}

func Test_3NodeStartup(t *testing.T) {
	tmpDir, err := createTmpDir()
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	logCfg := zap.NewProductionConfig()
	logger, err := logCfg.Build()
	if err != nil {
		t.Fatal(err)
	}

	tmplConfig := Configuration{
		TickPeriod:             time.Millisecond,
		MinElectionTicks:       10,
		MaxElectionTicks:       20,
		HeartbeatTicks:         1,
		Consistency:            ConsistencySerializable,
		MsgBufferSize:          8,
		DialTimeout:            time.Millisecond,
		ConnectionAttemptDelay: time.Millisecond,
		SendTimeout:            time.Millisecond,
		GRPCOptions: []GRPCOption{
			WithGRPCDialOption{Opt: grpc.WithInsecure()},
		},
		Logger: logger,
	}
	configs, err := createConfigs(tmplConfig, tmpDir, t.Name(), 1, 2, 3)
	if err != nil {
		t.Fatal(err)
	}

	// hack: use 1 fake application for all three nodes
	app := &fakeApplication{}

	nodes := map[uint64]*Node{}
	for _, config := range configs {
		node, err := NewNode(config, app)
		if err != nil {
			t.Fatal(err)
		}
		nodes[config.ID] = node
	}

	done := make(chan error)
	for _, node := range nodes {
		go func(node *Node) {
			err := node.Start()
			done <- err
		}(node)
	}

	for i := 0; i < 3; i++ {
		err := <-done
		if err != nil {
			t.Fatal(err)
		}
	}

	// simulate passive cluster
	time.Sleep(100 * time.Millisecond)

	for _, node := range nodes {
		err := node.Stop()
		if err != nil {
			fmt.Print(err)
		}
	}
}

// func TestOneNodeLinear(t *testing.T) {
// 	tmpDir, err := ioutil.TempDir("", "")
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	defer os.RemoveAll(tmpDir)

// 	config := &Configuration{
// 		RecvChanSize: 100,
// 		SendChanSize: 100,
// 		ID:           111,
// 		Peers: map[uint64]string{
// 			111: fmt.Sprintf("unix://%s", filepath.Join(tmpDir, "TestThreeNodeLinear111.sock")),
// 		},
// 		MinElectionTimeoutTicks: 10,
// 		MaxElectionTimeoutTicks: 20,
// 		HeartbeatTicks:          1,
// 		TickMs:                  2,
// 	}
// 	raftNode, err := NewNode(config)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	defer raftNode.Stop()

// 	kvStore := raftNode.KVStore
// 	const k = "someKey"
// 	i := 1
// 	for i < 21 {
// 		want := fmt.Sprintf("%d", i)
// 		kvStore.Propose(k, want)
// 		j := 0
// 		for j < 10 {
// 			time.Sleep(5 * time.Millisecond)
// 			v, ok := kvStore.Get(k)
// 			if !ok || v != want {
// 				t.Log("Haven't seen new proposed value yet...")
// 			} else {
// 				t.Logf("Got %s.\n", v)
// 				break
// 			}
// 			j++
// 		}
// 		if j == 10 {
// 			t.Logf("Proposal %d dropped. Proposing it again.\n", i)
// 		} else {
// 			i++
// 		}
// 	}
// }

// func TestThreeNodeLinear(t *testing.T) {
// 	tmpDir, err := ioutil.TempDir("", "")
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	defer os.RemoveAll(tmpDir)

// 	myConfiguration := func(id uint64) (*Configuration, error) {
// 		if id != 111 && id != 222 && id != 333 {
// 			return nil, fmt.Errorf("id must be 111, 222, or 333; got %d", id)
// 		}
// 		return &Configuration{
// 			RecvChanSize: 100,
// 			SendChanSize: 100,
// 			ID:           id,
// 			Peers: map[uint64]string{
// 				111: fmt.Sprintf("unix://%s", filepath.Join(tmpDir, "TestThreeNodeLinear111.sock")),
// 				222: fmt.Sprintf("unix://%s", filepath.Join(tmpDir, "TestThreeNodeLinear222.sock")),
// 				333: fmt.Sprintf("unix://%s", filepath.Join(tmpDir, "TestThreeNodeLinear333.sock")),
// 			},
// 			MinElectionTimeoutTicks: 10,
// 			MaxElectionTimeoutTicks: 20,
// 			HeartbeatTicks:          1,
// 			TickMs:                  2,
// 		}, nil
// 	}

// 	setup3Nodes := func(t *testing.T) ([]*Node, []func() error) {
// 		raftNodes := []*Node{}
// 		stoppers := []func() error{}
// 		type result struct {
// 			n   *Node
// 			err error
// 		}
// 		var wg sync.WaitGroup
// 		wg.Add(3)
// 		results := make(chan result, 3)
// 		for i := 1; i <= 3; i++ {
// 			go func(i uint64) {
// 				defer wg.Done()
// 				config, err := myConfiguration(uint64(i))
// 				if err != nil {
// 					results <- result{nil, err}
// 					return
// 				}
// 				raftNode, err := NewNode(config)
// 				results <- result{raftNode, err}
// 			}(uint64(i * 111))
// 		}
// 		wg.Wait()
// 		close(results)
// 		for res := range results {
// 			if res.err != nil {
// 				t.Fatal(res.err)
// 			}
// 			stoppers = append(stoppers, res.n.Stop)
// 			raftNodes = append(raftNodes, res.n)
// 		}
// 		return raftNodes, stoppers
// 	}

// 	test := func(kvStoreW, kvStoreR *KVStore, done chan struct{}) {
// 		const k = "someKey"
// 		i := 1
// 		for i < 21 {
// 			want := fmt.Sprintf("%d", i)
// 			kvStoreW.Propose(k, want)
// 			j := 0
// 			for j < 10 {
// 				time.Sleep(5 * time.Millisecond)
// 				v, ok := kvStoreR.Get(k)
// 				if !ok || v != want {
// 					t.Log("Haven't seen new proposed value yet...")
// 				} else {
// 					t.Logf("Got %s.\n", v)
// 					break
// 				}
// 				j++
// 			}
// 			if j == 10 {
// 				t.Logf("Proposal %d dropped. Proposing it again.\n", i)
// 			} else {
// 				i++
// 			}
// 		}
// 		close(done)
// 	}

// 	tester := func(
// 		t *testing.T,
// 		kvStoreW, kvStoreR *KVStore,
// 		timeout time.Duration,
// 	) {
// 		done := make(chan struct{})
// 		go test(kvStoreW, kvStoreR, done)
// 		select {
// 		case <-time.After(timeout):
// 			t.Error("timed out")
// 		case <-done:
// 		}
// 	}

// 	// DO NOT RUN IN PARALLEL
// 	t.Run("writer is leader, reader is follower", func(t *testing.T) {
// 		raftNodes, closers := setup3Nodes(t)
// 		for _, closer := range closers {
// 			defer closer()
// 		}

// 		var (
// 			kvStoreW, kvStoreR *KVStore
// 		)
// 		// hack: potential data race
// 		// TODO(ulysseses): remove data race potential
// 		count := 0
// 		for _, node := range raftNodes {
// 			if count == 2 {
// 				break
// 			}
// 			if node.raft.getRole() == roleLeader {
// 				kvStoreW = node.KVStore
// 				count++
// 			} else if node.raft.getRole() == roleFollower {
// 				kvStoreR = node.KVStore
// 				count++
// 			}
// 		}
// 		tester(t, kvStoreW, kvStoreR, 400*time.Millisecond)
// 	})
// 	t.Run("writer is leader, reader is leader", func(t *testing.T) {
// 		raftNodes, closers := setup3Nodes(t)
// 		for _, closer := range closers {
// 			defer closer()
// 		}

// 		var (
// 			kvStoreW, kvStoreR *KVStore
// 		)
// 		// hack: potential data race
// 		// TODO(ulysseses): remove data race potential
// 		for _, node := range raftNodes {
// 			if node.raft.getRole() == roleLeader {
// 				kvStoreW = node.KVStore
// 				kvStoreR = node.KVStore
// 				break
// 			}
// 		}
// 		tester(t, kvStoreW, kvStoreR, 400*time.Millisecond)
// 	})
// 	t.Run("writer is follower, reader is follower", func(t *testing.T) {
// 		raftNodes, closers := setup3Nodes(t)
// 		for _, closer := range closers {
// 			defer closer()
// 		}

// 		var (
// 			kvStoreW, kvStoreR *KVStore
// 		)
// 		// hack: potential data race
// 		// TODO(ulysseses): remove data race potential
// 		for _, node := range raftNodes {
// 			if node.raft.getRole() == roleFollower {
// 				kvStoreW = node.KVStore
// 				kvStoreR = node.KVStore
// 				break
// 			}
// 		}
// 		tester(t, kvStoreW, kvStoreR, 400*time.Millisecond)
// 	})
// 	t.Run("writer is follower, reader is leader", func(t *testing.T) {
// 		raftNodes, closers := setup3Nodes(t)
// 		for _, closer := range closers {
// 			defer closer()
// 		}

// 		var (
// 			kvStoreW, kvStoreR *KVStore
// 		)
// 		// hack: potential data race
// 		// TODO(ulysseses): remove data race potential
// 		count := 0
// 		for _, node := range raftNodes {
// 			if count == 2 {
// 				break
// 			}
// 			if node.raft.getRole() == roleFollower {
// 				kvStoreW = node.KVStore
// 				count++
// 			} else if node.raft.getRole() == roleLeader {
// 				kvStoreR = node.KVStore
// 				count++
// 			}
// 		}
// 		tester(t, kvStoreW, kvStoreR, 400*time.Millisecond)
// 	})
// 	t.Run("leader is dropped and brought back online", func(t *testing.T) {
// 		raftNodes, stoppers := setup3Nodes(t)
// 		for _, stopper := range stoppers {
// 			defer stopper()
// 		}

// 		var (
// 			downNode               *Node
// 			downKVStore, upKVStore *KVStore
// 		)
// 		// TODO(ulysseses): potential data race if downNode no longer is leader
// 		// immediately after we assign downNode = node
// 		for _, node := range raftNodes {
// 			if node.raft.getRole() == roleLeader {
// 				downNode = node
// 				downKVStore = node.KVStore
// 			} else {
// 				upKVStore = node.KVStore
// 			}
// 		}

// 		go func() {
// 			time.Sleep(50 * time.Millisecond) // small initial delay
// 			downNode.pause()
// 			time.Sleep(50 * time.Millisecond) // > election timeout
// 			downNode.unpause()
// 		}()
// 		tester(t, downKVStore, upKVStore, time.Second)
// 	})
// 	t.Run("follower is dropped and brought back online", func(t *testing.T) {
// 		raftNodes, stoppers := setup3Nodes(t)
// 		for _, stopper := range stoppers {
// 			defer stopper()
// 		}

// 		var (
// 			downNode               *Node
// 			downKVStore, upKVStore *KVStore
// 		)

// 		// TODO(ulysseses): potential data race if downNode becomes leader
// 		// immediately after we assign downNode = node
// 		for _, node := range raftNodes {
// 			if node.raft.getRole() == roleFollower {
// 				downNode = node
// 				downKVStore = node.KVStore
// 			} else if node.raft.getRole() == roleLeader {
// 				upKVStore = node.KVStore
// 			}
// 		}

// 		go func() {
// 			time.Sleep(50 * time.Millisecond) // small initial delay
// 			downNode.pause()
// 			time.Sleep(50 * time.Millisecond) // > election timeout
// 			downNode.unpause()
// 		}()
// 		tester(t, downKVStore, upKVStore, time.Second)
// 	})
// }
