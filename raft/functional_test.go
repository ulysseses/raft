package raft

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ulysseses/raft/raftpb"
)

type noOpApp struct{}

// Apply implements Application for noOpApp.
func (app *noOpApp) Apply(entries []raftpb.Entry) error {
	return nil
}

func newNoOpApp(id uint64) Application {
	return &noOpApp{}
}

type commitLog struct {
	node     *Node
	_padding [64]byte
	sync.RWMutex
	log []raftpb.Entry
}

// Apply implements Application for commitLog
func (cl *commitLog) Apply(entries []raftpb.Entry) error {
	cl.Lock()
	cl.log = append(cl.log, entries...)
	cl.Unlock()
	return nil
}

func (cl *commitLog) append(ctx context.Context, x int) error {
	s := strconv.Itoa(x)
	return cl.node.Propose(ctx, []byte(s))
}

func (cl *commitLog) getLatest(ctx context.Context) (int, error) {
	if err := cl.node.Read(ctx); err != nil {
		return 0, err
	}
	cl.RLock()
	if len(cl.log) == 0 {
		cl.RUnlock()
		return 0, nil
	}
	latestEntry := cl.log[len(cl.log)-1]
	cl.RUnlock()
	return strconv.Atoi(string(latestEntry.Data))
}

func newCommitLog(id uint64) Application {
	return &commitLog{
		log: []raftpb.Entry{},
	}
}

func createUnixSockets(
	testName string,
	ids ...uint64,
) (addresses map[uint64]string, cleanup func(), err error) {
	var tmpDir string
	tmpDir, err = ioutil.TempDir("", "")
	if err != nil {
		return
	}
	cleanup = func() {
		os.RemoveAll(tmpDir)
	}

	addresses = map[uint64]string{}
	for _, id := range ids {
		addresses[id] = fmt.Sprintf("unix://%s%d.sock", filepath.Join(tmpDir, testName), id)
	}
	return
}

func spinUpNodes(
	t *testing.T,
	newApp func(uint64) Application,
	ids []uint64,
	pOpts []ProtocolConfigOption,
	tOpts []TransportConfigOption,
	nOpts []NodeConfigOption,
) (map[uint64]*Node, map[uint64]Application) {
	addresses, cleanup, err := createUnixSockets(t.Name(), ids...)
	defer cleanup()
	if err != nil {
		t.Fatal(err)
	}

	nodes := map[uint64]*Node{}
	apps := map[uint64]Application{}
	for _, id := range ids {
		tConfig := NewTransportConfig(id, addresses, tOpts...)
		pConfig := NewProtocolConfig(id, pOpts...)
		nConfig := NewNodeConfig(id, nOpts...)
		app := newApp(id)
		node, err := nConfig.Build(pConfig, tConfig, app)
		if err != nil {
			t.Fatal(err)
		}
		nodes[id] = node
		apps[id] = app
	}

	// Start the Raft nodes
	done := make(chan struct{})
	for _, node := range nodes {
		go func(n *Node) {
			n.Start()
			done <- struct{}{}
		}(node)
	}
	for range ids {
		<-done
	}

	return nodes, apps
}

func stopAllNodes(t *testing.T, nodes map[uint64]*Node) {
	for _, node := range nodes {
		if err := node.Stop(); err != nil {
			t.Fatal(err)
		}
	}
}

func Test_3Node_StartAndStop(t *testing.T) {
	// Spin 3 nodes
	nodes, _ := spinUpNodes(t, newNoOpApp, []uint64{1, 2, 3}, nil, nil, nil)
	defer stopAllNodes(t, nodes)

	// Simulate passive cluster
	time.Sleep(3 * time.Second)
}

// Benchmark round trip time
func Benchmark_Transport_RTT(b *testing.B) {
	addresses, cleanup, err := createUnixSockets(b.Name(), 1, 2)
	defer cleanup()
	if err != nil {
		b.Fatal(err)
	}

	transports := map[uint64]*transport{}
	for _, id := range []uint64{1, 2} {
		tConfig := NewTransportConfig(id, addresses)
		tr, err := tConfig.build()
		if err != nil {
			b.Fatal(err)
		}
		defer tr.stop()
		transports[id] = tr
	}

	done := make(chan struct{})
	for _, tr := range transports {
		go func(tr *transport) {
			tr.start()
			done <- struct{}{}
		}(tr)
	}
	for range []uint64{1, 2} {
		<-done
	}

	// Benchmark
	tr1 := transports[1]
	tr2 := transports[2]
	msg := raftpb.Message{From: 1, To: 2}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr1.sendChan <- msg
		<-tr2.recvChan
	}
}

func Test_3Node_Linearizable_RoundRobin(t *testing.T) {
	// Spin 3 nodes
	nodes, apps := spinUpNodes(
		t,
		newCommitLog,
		[]uint64{1, 2, 3},
		[]ProtocolConfigOption{
			WithConsistency(ConsistencyLinearizable),
			// AddProtocolLogger(),
		},
		[]TransportConfigOption{
			// AddTransportLogger(),
		},
		[]NodeConfigOption{
			// AddNodeLogger(),
		})
	defer stopAllNodes(t, nodes)

	// connect the commitLog to its corresponding node
	for id := range apps {
		cl := apps[id].(*commitLog)
		cl.node = nodes[id]
	}

	errChan := make(chan error)
	stopChan := make(chan struct{})
	for id, app := range apps {
		cl := app.(*commitLog)
		go func(id uint64, cl *commitLog) {
			x := 0
			for {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				y, err := cl.getLatest(ctx)
				cancel()
				if err == nil {
					if y < x {
						errChan <- fmt.Errorf("app %d read %d after reading %d", id, y, x)
						return
					}
					t.Logf("app %d: x = %d, y = %d", id, x, y)
					x = y
				} else {
					t.Logf("app %d errored: %v", id, err)
				}

				select {
				case <-stopChan:
					errChan <- nil
					return
				case <-time.After(time.Second):
					// rate limit
				}
			}
		}(id, cl)
	}

	// Choose in round-robin which Raft node to write monotonically increasing number
	go func() {
		id := uint64(1)
		i := 1
		for i <= 5 {
			cl := apps[id].(*commitLog)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if err := cl.append(ctx, i); err != nil {
				t.Logf("app %d errored on i = %d: %v", id, i, err)
			} else {
				i++
			}
			cancel()
			id++
			if id == 4 {
				id = 1
			}
		}
		for range []uint64{1, 2, 3} {
			stopChan <- struct{}{}
		}
	}()

	select {
	case err := <-errChan:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("timed out after 30 seconds")
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
