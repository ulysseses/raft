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
	ids []uint64,
	pOpts []ProtocolConfigOption,
	nOpts []NodeConfigOption,
	newApp func(uint64) Application,
) (map[uint64]*Node, map[uint64]Application) {
	nodes := map[uint64]*Node{}
	apps := map[uint64]Application{}
	trs := newFakeTransports(ids...)
	for _, id := range ids {
		pConfig := NewProtocolConfig(id, pOpts...)
		psm, err := pConfig.Build(trs[id])
		if err != nil {
			t.Fatal(err)
		}
		nConfig := NewNodeConfig(id, nOpts...)
		app := newApp(id)
		node, err := nConfig.Build(psm, trs[id], app)
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
	nodes, _ := spinUpNodes(t, []uint64{1, 2, 3}, nil, nil, newNoOpApp)
	defer stopAllNodes(t, nodes)

	// Simulate passive cluster
	time.Sleep(2 * time.Second)
}

// Benchmark round trip time
func Benchmark_gRPCTransport_RTT(b *testing.B) {
	addresses, cleanup, err := createUnixSockets(b.Name(), 1, 2)
	defer cleanup()
	if err != nil {
		b.Fatal(err)
	}

	trs := map[uint64]Transport{}
	for _, id := range []uint64{1, 2} {
		tConfig := NewTransportConfig(id, addresses)
		tr, err := tConfig.Build()
		if err != nil {
			b.Fatal(err)
		}
		defer tr.stop()
		trs[id] = tr
	}

	done := make(chan struct{})
	for _, tr := range trs {
		go func(tr Transport) {
			tr.start()
			done <- struct{}{}
		}(tr)
	}
	for range []uint64{1, 2} {
		<-done
	}

	// Benchmark
	sendC1 := trs[1].send()
	recvC2 := trs[2].recv()
	msg := raftpb.Message{From: 1, To: 2}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sendC1 <- msg
		<-recvC2
	}
}

func Test_3Node_Linearizable_RoundRobin(t *testing.T) {
	// Spin 3 nodes
	nodes, apps := spinUpNodes(
		t,
		[]uint64{1, 2, 3},
		[]ProtocolConfigOption{
			WithConsistency(ConsistencyLinearizable),
			// AddProtocolLogger(),
		},
		[]NodeConfigOption{
			// AddNodeLogger(),
		},
		newCommitLog)
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
	case <-time.After(10 * time.Second):
		t.Fatal("timed out after 10 seconds")
	}
}

// TODO(ulysseses): test durability and consistency when under network or node failure
