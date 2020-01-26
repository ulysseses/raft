package raft

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ulysseses/raft/pb"
)

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

func Test_1Node_ConsistencyLease_RoundRobin(t *testing.T) {
	// Spin 1 node
	nodes, apps := spinUpNodes(
		t,
		[]uint64{1},
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

	testRoundRobin(t, apps, 10, false, time.Second, 60*time.Second)
}

func Test_1Node_ConsistencyStrict_RoundRobin(t *testing.T) {
	// Spin 1 node
	nodes, apps := spinUpNodes(
		t,
		[]uint64{1},
		[]ProtocolConfigOption{WithConsistency(ConsistencyStrict)},
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

	testRoundRobin(t, apps, 10, true, time.Second, 60*time.Second)
}

func Test_1Node_ConsistencyStale_RoundRobin(t *testing.T) {
	// Spin 1 node
	nodes, apps := spinUpNodes(
		t,
		[]uint64{1},
		[]ProtocolConfigOption{WithConsistency(ConsistencyStale)},
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

	testRoundRobin(t, apps, 10, false, time.Second, 60*time.Second)
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

	testRoundRobin(t, apps, 10, false, time.Second, 60*time.Second)
}

func Test_3Node_ConsistencyStrict_RoundRobin(t *testing.T) {
	// Spin 3 nodes
	nodes, apps := spinUpNodes(
		t,
		[]uint64{1, 2, 3},
		[]ProtocolConfigOption{WithConsistency(ConsistencyStrict)},
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

	testRoundRobin(t, apps, 10, true, time.Second, 60*time.Second)
}

func Test_3Node_ConsistencyStale_RoundRobin(t *testing.T) {
	// Spin 3 nodes
	nodes, apps := spinUpNodes(
		t,
		[]uint64{1, 2, 3},
		[]ProtocolConfigOption{WithConsistency(ConsistencyStale)},
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

	testRoundRobin(t, apps, 10, false, time.Second, 60*time.Second)
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

func Benchmark_1Node_ConsistencyStrict_RoundRobin(b *testing.B) {
	// Spin 1 node
	nodes, apps := spinUpNodes(
		b,
		[]uint64{1},
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

	testRoundRobin(b, apps, b.N, true, time.Second, 60*time.Second)
}

func Benchmark_1Node_ConsistencyStale_RoundRobin(b *testing.B) {
	// Spin 1 node
	nodes, apps := spinUpNodes(
		b,
		[]uint64{1},
		[]ProtocolConfigOption{WithConsistency(ConsistencyStale)},
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

	testRoundRobin(b, apps, b.N, true, time.Second, 60*time.Second)
}

func Benchmark_3Node_ConsistencyStale_RoundRobin(b *testing.B) {
	// Spin 3 nodes
	nodes, apps := spinUpNodes(
		b,
		[]uint64{1, 2, 3},
		[]ProtocolConfigOption{WithConsistency(ConsistencyStale)},
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

func Benchmark_3Node_ProposalAsLeader(b *testing.B) {
	// Spin 1 node
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

	testSet(b, nodes, apps, RoleLeader, b.N, time.Second, 60*time.Second)
}

func Benchmark_3Node_ProposalAsFollower(b *testing.B) {
	// Spin 1 node
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

	testSet(b, nodes, apps, RoleFollower, b.N, time.Second, 60*time.Second)
}

func Benchmark_3Node_ConsistencyLease_ReadAsLeader(b *testing.B) {
	// Spin 1 node
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

	testGet(b, nodes, apps, RoleLeader, b.N, false, time.Second, 60*time.Second)
}

func Benchmark_3Node_ConsistencyLease_ReadAsFollower(b *testing.B) {
	// Spin 1 node
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

	testGet(b, nodes, apps, RoleFollower, b.N, false, time.Second, 60*time.Second)
}

func Benchmark_3Node_ConsistencyStrict_ReadAsLeader(b *testing.B) {
	// Spin 1 node
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

	testGet(b, nodes, apps, RoleLeader, b.N, true, time.Second, 60*time.Second)
}

func Benchmark_3Node_ConsistencyStrict_ReadAsFollower(b *testing.B) {
	// Spin 1 node
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

	testGet(b, nodes, apps, RoleFollower, b.N, true, time.Second, 60*time.Second)
}

func Benchmark_3Node_ConsistencyStale_ReadAsLeader(b *testing.B) {
	// Spin 1 node
	nodes, apps := spinUpNodes(
		b,
		[]uint64{1, 2, 3},
		[]ProtocolConfigOption{WithConsistency(ConsistencyStale)},
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

	testGet(b, nodes, apps, RoleLeader, b.N, false, time.Second, 60*time.Second)
}

func Benchmark_3Node_ConsistencyStale_ReadAsFollower(b *testing.B) {
	// Spin 1 node
	nodes, apps := spinUpNodes(
		b,
		[]uint64{1, 2, 3},
		[]ProtocolConfigOption{WithConsistency(ConsistencyStale)},
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

	testGet(b, nodes, apps, RoleFollower, b.N, false, time.Second, 60*time.Second)
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

	resetFakeTransportRegistry()

	// Manually trigger GC so there are minimal STW pauses within future benchmarks
	if _, ok := tb.(*testing.B); ok {
		runtime.GC()
	}
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
	ids := []uint64{}
	for id := range registers {
		ids = append(ids, id)
	}
	errC := make(chan error)
	go func() {
		if b, ok := tb.(*testing.B); ok {
			b.ResetTimer()
			b.ReportAllocs()
		}
		setterInd := 0
		getterInd := 1 % len(ids)
		for round := 1; round <= rounds; round++ {
			setter := registers[ids[setterInd]].(*register)
			getter := registers[ids[getterInd]].(*register)
			setterInd = (setterInd + 1) % len(ids)
			getterInd = (getterInd + 1) % len(ids)

			// set with setter
			retrySet(tb, setter, round, reqTimeout)
			// then get with getter
			err := retryGet(tb, getter, round, linearizable, reqTimeout)
			if err != nil {
				tb.Log(err)
				errC <- err
				return
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

// Due to there being multiple (e.g. n=3) Raft nodes in a single Go process,
// there is a lot of inter-goroutine traffic (e.g. between transports, nodes, PSMs, apps, etc.).
// The raft package isn't designed to be performant for this 3-node cluster in-process
// test environment. As such, please give generous timeout values to prevent test flakiness.
func testSet(
	tb testing.TB,
	nodes map[uint64]*Node,
	registers map[uint64]Application,
	role Role,
	rounds int,
	reqTimeout time.Duration,
	testTimeout time.Duration,
) {
	// This is assuming no failed nodes or election is taking place.
	var setter *register
	var raftNode *Node
	var beforeTerm uint64
	for _, node := range nodes {
		state := node.State()
		if state.Role == role {
			setter = registers[state.ID].(*register)
			raftNode = node
			beforeTerm = state.Term
			break
		}
	}
	if setter == nil {
		tb.Fatalf("no node found with role %s", role.String())
	}
	done := make(chan struct{})
	go func() {
		if b, ok := tb.(*testing.B); ok {
			b.ResetTimer()
			b.ReportAllocs()
		}
		for round := 1; round <= rounds; round++ {
			retrySet(tb, setter, round, reqTimeout)
		}
		if b, ok := tb.(*testing.B); ok {
			b.StopTimer()
		}
		done <- struct{}{}
	}()

	select {
	case <-done:
	case <-time.After(testTimeout):
		tb.Fatalf("test timed out after %v", testTimeout)
	}

	// double check that the term hasn't changed
	afterTerm := raftNode.State().Term
	if beforeTerm != afterTerm {
		tb.Fatalf("beforeTerm: %d, afterTerm: %d", beforeTerm, afterTerm)
	}
}

// Due to there being multiple (e.g. n=3) Raft nodes in a single Go process,
// there is a lot of inter-goroutine traffic (e.g. between transports, nodes, PSMs, apps, etc.).
// The raft package isn't designed to be performant for this 3-node cluster in-process
// test environment. As such, please give generous timeout values to prevent test flakiness.
func testGet(
	tb testing.TB,
	nodes map[uint64]*Node,
	registers map[uint64]Application,
	role Role,
	rounds int,
	linearizable bool,
	reqTimeout time.Duration,
	testTimeout time.Duration,
) {
	// This is assuming no failed nodes or election is taking place.
	var getter *register
	var raftNode *Node
	var beforeTerm uint64
	for _, node := range nodes {
		state := node.State()
		if state.Role == role {
			getter = registers[state.ID].(*register)
			raftNode = node
			beforeTerm = state.Term
			break
		}
	}
	if getter == nil {
		tb.Fatalf("no node found with role %s", role.String())
	}

	// pre-write a value of 42
	retrySet(tb, getter, 42, reqTimeout)

	errC := make(chan error)
	go func() {
		if b, ok := tb.(*testing.B); ok {
			b.ResetTimer()
			b.ReportAllocs()
		}
		for round := 1; round <= rounds; round++ {
			err := retryGet(tb, getter, 42, linearizable, reqTimeout)
			if err != nil {
				errC <- err
				return
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

	// double check that the term hasn't changed
	afterTerm := raftNode.State().Term
	if beforeTerm != afterTerm {
		tb.Fatalf("beforeTerm: %d, afterTerm: %d", beforeTerm, afterTerm)
	}
}

func retrySet(tb testing.TB, r *register, x int, reqTimeout time.Duration) {
	setAttempt := 1
	for {
		ctx, cancel := context.WithTimeout(context.Background(), reqTimeout)
		err := r.set(ctx, x)
		cancel()
		if err == nil {
			break
		}
		tb.Logf("failed attempt #%d of setting value %d", setAttempt, x)
		setAttempt++
	}
}

func retryGet(tb testing.TB, r *register, want int, linearizable bool, reqTimeout time.Duration) error {
	getAttempt := 1
	var got int
	for {
		ctx, cancel := context.WithTimeout(context.Background(), reqTimeout)
		var err error
		got, err = r.get(ctx)
		cancel()
		if err == nil {
			break
		}
		tb.Logf("failed attempt #%d of getting value %d", getAttempt, want)
		getAttempt++
	}
	if linearizable && got != want {
		return fmt.Errorf("got %d, but wanted %d", got, want)
	}
	return nil
}

// TODO(ulysseses): test durability and consistency when under network or node failure
