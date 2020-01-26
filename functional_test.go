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

	"github.com/ulysseses/raft/pb"
)

type noOpApp struct{}

// Apply implements Application for noOpApp.
func (app *noOpApp) Apply(entries []pb.Entry) error {
	return nil
}

func newNoOpApp(id uint64) Application {
	return &noOpApp{}
}

// register is a single-valued store
type register struct {
	sync.RWMutex
	node *Node
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

// Due to there being 3 Raft nodes in a single Go process,
// there is a lot of inter-goroutine traffic (e.g. between transports, nodes, PSMs, apps, etc.).
// As such, please give generous timeout values to prevent test flakiness.
func spinUpNodes(
	tb testing.TB,
	ids []uint64,
	pOpts []ProtocolConfigOption,
	nOpts []NodeConfigOption,
	newApp func(uint64) Application,
) (map[uint64]*Node, map[uint64]Application) {
	if !*testDisableLoggingFlag {
		pOpts = append(pOpts, AddProtocolLogger())
		nOpts = append(nOpts, AddNodeLogger())
	}

	nodes := map[uint64]*Node{}
	apps := map[uint64]Application{}
	for _, id := range ids {
		tr, err := NewTransportConfig(id, WithChannelMedium(ids...)).Build()
		if err != nil {
			tb.Fatal(err)
		}

		pConfig := NewProtocolConfig(id, pOpts...)
		psm, err := pConfig.Build(tr)
		if err != nil {
			tb.Fatal(err)
		}

		nConfig := NewNodeConfig(id, nOpts...)
		app := newApp(id)
		node, err := nConfig.Build(psm, tr, app)
		if err != nil {
			tb.Fatal(err)
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

func stopAllNodes(tb testing.TB, nodes map[uint64]*Node) {
	for _, node := range nodes {
		if err := node.Stop(); err != nil {
			tb.Fatal(err)
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
		tConfig := NewTransportConfig(id, WithAddresses(addresses))
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
	msg := pb.Message{From: 1, To: 2}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sendC1 <- msg
		<-recvC2
	}
}

func Test_3Node_ConsistencyStrict_RoundRobin(t *testing.T) {
	// Spin 3 nodes
	nodes, apps := spinUpNodes(
		t,
		[]uint64{1, 2, 3},
		[]ProtocolConfigOption{WithConsistency(ConsistencyStrict)},
		nil,
		func(uint64) Application { return &register{} })
	defer stopAllNodes(t, nodes)

	// connect the commitLog to its corresponding node
	for id := range apps {
		r := apps[id].(*register)
		r.node = nodes[id]
	}

	// wait for leader election
	time.Sleep(3 * time.Second)

	testRoundRobin(t, apps, 3, true, time.Second, 60*time.Second)
}

func Test_3Node_ConsistencyLease_RoundRobin(t *testing.T) {
	// Spin 3 nodes
	nodes, apps := spinUpNodes(
		t,
		[]uint64{1, 2, 3},
		nil,
		nil,
		func(id uint64) Application { return &register{} })
	defer stopAllNodes(t, nodes)

	// connect the register to its corresponding node
	for id := range apps {
		cl := apps[id].(*register)
		cl.node = nodes[id]
	}

	// wait for leader election
	time.Sleep(3 * time.Second)

	testRoundRobin(t, apps, 3, false, time.Second, 30*time.Second)
}

func Benchmark_3Node_ConsistencyStrict_RoundRobin(b *testing.B) {
	// Spin 3 nodes
	nodes, apps := spinUpNodes(
		b,
		[]uint64{1, 2, 3},
		[]ProtocolConfigOption{WithConsistency(ConsistencyStrict)},
		nil,
		func(id uint64) Application { return &register{} })
	defer stopAllNodes(b, nodes)

	// connect the register to its corresponding node
	for id := range apps {
		cl := apps[id].(*register)
		cl.node = nodes[id]
	}

	// wait for leader election
	time.Sleep(3 * time.Second)

	testRoundRobin(b, apps, b.N, false, time.Second, 60*time.Second)
}

func Benchmark_3Node_ConsistencyLease_RoundRobin(b *testing.B) {
	// Spin 3 nodes
	nodes, apps := spinUpNodes(
		b,
		[]uint64{1, 2, 3},
		nil,
		nil,
		func(id uint64) Application { return &register{} })
	defer stopAllNodes(b, nodes)

	// connect the register to its corresponding node
	for id := range apps {
		cl := apps[id].(*register)
		cl.node = nodes[id]
	}

	// wait for leader election
	time.Sleep(3 * time.Second)

	testRoundRobin(b, apps, b.N, false, time.Second, 60*time.Second)
}

func Benchmark_1Node_ConsistencyLease_RoundRobin(b *testing.B) {
	// Spin 1 node
	nodes, apps := spinUpNodes(
		b,
		[]uint64{1},
		nil,
		nil,
		func(id uint64) Application { return &register{} })
	defer stopAllNodes(b, nodes)

	// connect the register to its corresponding node
	for id := range apps {
		cl := apps[id].(*register)
		cl.node = nodes[id]
	}

	// wait for leader election
	time.Sleep(3 * time.Second)

	testRoundRobin(b, apps, b.N, false, time.Second, 60*time.Second)
}

// Due to there being multiple (e.g. n=3) Raft nodes in a single Go process,
// there is a lot of inter-goroutine traffic (e.g. between transports, nodes, PSMs, apps, etc.).
// The raft package isn't designed to be performant for this 3-node cluster in-process
// test environment. As such, please give generous timeout values to prevent test flakiness.
func testRoundRobin(
	tb testing.TB,
	registers map[uint64]Application,
	rounds int,
	linearizable bool,
	reqTimeout time.Duration,
	testTimeout time.Duration,
) {
	errC := make(chan error)
	go func() {
		ids := []uint64{}
		for id := range registers {
			ids = append(ids, id)
		}
		if b, ok := tb.(*testing.B); ok {
			b.ResetTimer()
			b.ReportAllocs()
		}
		writerInd := 0
		readerInd := 1 % len(ids)
		for round := 1; round <= rounds; round++ {
			writer := registers[ids[writerInd]].(*register)
			reader := registers[ids[readerInd]].(*register)
			writerInd = (writerInd + 1) % len(ids)
			readerInd = (readerInd + 1) % len(ids)

			// write with writer
			writeAttempt := 1
			for {
				ctx, cancel := context.WithTimeout(context.Background(), reqTimeout)
				err := writer.set(ctx, round)
				cancel()
				if err != nil {
					tb.Logf("failed attempt #%d of writing value %d", writeAttempt, round)
					writeAttempt++
					continue
				}
				break
			}
			// then read with reader
			readAttempt := 1
			for {
				ctx, cancel := context.WithTimeout(context.Background(), reqTimeout)
				got, err := reader.get(ctx)
				cancel()
				if err != nil {
					tb.Logf("failed attempt #%d of reading value %d", readAttempt, round)
					readAttempt++
					continue
				}
				if got != round {
					err = fmt.Errorf("read %d, but expected %d", got, round)
					if linearizable {
						errC <- err
						return
					}
					tb.Log(err)
				}
				break
			}
		}

		if b, ok := tb.(*testing.B); ok {
			b.StopTimer()
		}
		errC <- nil
	}()

	select {
	case err := <-errC:
		if err != nil {
			tb.Fatal(err)
		}
	case <-time.After(testTimeout):
		tb.Fatalf("test timed out after %v", testTimeout)
	}
}

// TODO(ulysseses): test durability and consistency when under network or node failure
