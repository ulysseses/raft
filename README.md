<p align="center"></p>
<p align="center">
    <img src="./rafty.png" width="100"></img>
    <h1 align="center">Raft</h1>
    <p align="center">Proof of concept implementation of the <a href="https://raft.github.io">Raft consensus algorithm</a>.</p>
    <p align="center">
        <a href="https://godoc.org/github.com/ulysseses/wal"><img src="https://godoc.org/github.com/ulysseses/wal?status.svg"></a>
        <a href="https://github.com/ulysseses/raft/actions"><img src="https://github.com/ulysseses/raft/workflows/Build/badge.svg"></a>
        <a href="https://github.com/ulysseses/raft/actions"><img src="https://github.com/ulysseses/raft/workflows/Tests/badge.svg"></a>
        <img src="https://img.shields.io/badge/status-WIP-yellow">
    </p>
</p>

---

Raft is a consensus algorithm that is designed to be easy to understand. It's equivalent to Paxos in fault-tolerance and performance. Raft has already been implemented many times over and with multiple extensions and optimizations. Nevertheless, I decided to write one from the ground up to learn its internals.

## Tooling dependencies

```bash
cd $GOPATH/src
go get -u github.com/gogo/protobuf/protoc-gen-gofast
github.com/gogo/protobuf/install-protobuf.sh  # it you don't already have protoc
```

## Example KV Store

### Run a pre-configured 3-node cluster

```bash
cd examples/kvstore/server
source ./utils.sh
start3cluster
# eventually call stop3cluster
```

### Or run the cluster yourself manually

```bash
cd examples/kvstore/server
go build

# in one process
./server \
  -addr :3001 \
  -id 1 \
  -addresses "1,tcp://localhost:8081|2,tcp://localhost:8082|3,tcp://localhost:8083"

# in another process
./server \
  -addr :3002 \
  -id 2 \
  -addresses "1,tcp://localhost:8081|2,tcp://localhost:8082|3,tcp://localhost:8083"

# in yet another process
./server \
  -addr :3003 \
  -id 3 \
  -addresses "1,tcp://localhost:8081|2,tcp://localhost:8082|3,tcp://localhost:8083"
```

### Interact with the cluster!

You can perform set & get operations against `/store`, e.g.

```bash
curl -XPUT "http://localhost:3001/store?k=hi&v=bye"
curl -XGET "http://localhost:3002/store?k=hi"
```

You can also check out the current Raft node state as well as its point of views of all of the Raft cluster members:

```bash
curl -XGET "http://localhost:3003/state"
curl -XGET "http://localhost:3003/members"
```

Finally, there's always the trusty `/debug/pprof/` endpoint.

## Building Protobuf files

```bash
# make sure you're in repo base
protoc \
  -I pb/ \
  -I $GOPATH/src \
  pb/raft.proto \
  --gogofaster_out=plugins=grpc:pb

protoc \
  -I examples/kvstore/kvpb/ \
  -I $GOPATH/src \
  examples/kvstore/kvpb/kv.proto \
  --gogofaster_out=plugins=grpc:examples/kvstore/kvpb
```

## Tests

```bash
$ go test -v -timeout=5m -race . -args -test.disableLogging
=== RUN   Test_3Node_StartAndStop
--- PASS: Test_3Node_StartAndStop (2.50s)
=== RUN   Test_1Node_ConsistencyLease_RoundRobin
--- PASS: Test_1Node_ConsistencyLease_RoundRobin (3.21s)
=== RUN   Test_1Node_ConsistencyStrict_RoundRobin
--- PASS: Test_1Node_ConsistencyStrict_RoundRobin (3.20s)
=== RUN   Test_1Node_ConsistencyStale_RoundRobin
--- PASS: Test_1Node_ConsistencyStale_RoundRobin (3.20s)
=== RUN   Test_3Node_ConsistencyLease_RoundRobin
--- PASS: Test_3Node_ConsistencyLease_RoundRobin (5.40s)
=== RUN   Test_3Node_ConsistencyStrict_RoundRobin
--- PASS: Test_3Node_ConsistencyStrict_RoundRobin (5.20s)
=== RUN   Test_3Node_ConsistencyStale_RoundRobin
--- PASS: Test_3Node_ConsistencyStale_RoundRobin (5.10s)
=== RUN   Test_heartbeatTicker
--- PASS: Test_heartbeatTicker (0.01s)
=== RUN   Test_electionTicker
--- PASS: Test_electionTicker (0.02s)
=== RUN   ExampleApplication_register
--- PASS: ExampleApplication_register (2.20s)
PASS
ok  	github.com/ulysseses/raft	31.229s
```

## Benchmarks

Benchmarks were performed on a relatively-noisy test environment (i.e. background apps were running). Tested on a Macbook Mac OS X 3.1 GHz Quad-Core i7 with 16 GB RAM.

> _**Note:** Go's builtin benchmark tooling may not accurately reflect the actual runtime. For example, reads performed on the leader of a 3-node cluster with `ConsistencyStrict` (i.e. reads require a quorum of heartbeat acks) take ~450ms on average, according to the benchmark shown below. However, the same read but on a follower node takes only ~12us. This is a huge discrepancy._
>
> _There may be a variety of factors of why this is the case, ranging from benchmark internals, cache contention, goroutine scheduler, GC, etc. I'm investigating the reasons why and may follow up with a solution or a blog post detailing my findings._
>
> _In the meantime, we can still rely on the `go-wrk` benchmarks, which paint a more accurate picture of the throughput and latency._

### `go-wrk` benchmarks

[`go-wrk`](https://github.com/adjust/go-wrk) is a heavy-duty but small small http benchmark utility. Here we show benchmarks for a Strict-Serializable (`ConsistencyStrict`) write and read to the leader. You can expect better performance if tuning to `ConsistencyLease` (lease-based consistency) or `ConsistencyStale` (eventual consistency). Sending reads/writes to followers also work, but expect extra latency due to RTT: these requests are proxied to the leader.

```bash
$ cd examples/kvstore/server
$ source ./utils.sh
$ start3cluster  # :3002 is the leader, ConsistencyStrict mode
[2] 26631
[3] 26632
[4] 26633
{"level":"info","ts":1580078483.289747,"caller":"server/main.go:207","msg":"starting http kv store backed by raft","addr":":3001"}
{"level":"info","ts":1580078486.270493,"caller":"server/main.go:207","msg":"starting http kv store backed by raft","addr":":3002"}
{"level":"info","ts":1580078486.280907,"caller":"server/main.go:207","msg":"starting http kv store backed by raft","addr":":3003"}
```
```bash
$ go-wrk -c 1 -n 1000 -m PUT "http://localhost:3002/store?k=hi&v=bye"
==========================BENCHMARK==========================
URL:				http://localhost:3002/store?k=hi&v=bye

Used Connections:		1
Used Threads:			1
Total number of calls:		1000

===========================TIMINGS===========================
Total time passed:		0.89s
Avg time per request:		0.89ms
Requests per second:		1117.82
Median time per request:	0.70ms
99th percentile time:		1.45ms
Slowest time for request:	101.00ms

=============================DATA=============================
Total response body sizes:		0
Avg response body per request:		0.00 Byte
Transfer rate per second:		0.00 Byte/s (0.00 MByte/s)
==========================RESPONSES==========================
20X Responses:		1000	(100.00%)
30X Responses:		0	(0.00%)
40X Responses:		0	(0.00%)
50X Responses:		0	(0.00%)
Errors:			0	(0.00%)
```
```bash
$ go-wrk -c 1 -n 1000 -m GET "http://localhost:3002/store?k=hi"
==========================BENCHMARK==========================
URL:				http://localhost:3002/store?k=hi

Used Connections:		1
Used Threads:			1
Total number of calls:		1000

===========================TIMINGS===========================
Total time passed:		1.16s
Avg time per request:		1.15ms
Requests per second:		862.38
Median time per request:	0.85ms
99th percentile time:		2.57ms
Slowest time for request:	106.00ms

=============================DATA=============================
Total response body sizes:		3000
Avg response body per request:		3.00 Byte
Transfer rate per second:		2587.15 Byte/s (0.00 MByte/s)
==========================RESPONSES==========================
20X Responses:		1000	(100.00%)
30X Responses:		0	(0.00%)
40X Responses:		0	(0.00%)
50X Responses:		0	(0.00%)
Errors:			0	(0.00%)
```
```bash
$ stop3cluster
[4]  + 26633 done       ./server -addr=:3003 -id=3 ${common_flags[@]}
[3]  + 26632 done       ./server -addr=:3002 -id=2 ${common_flags[@]}
[2]  + 26631 done       ./server -addr=:3001 -id=1 ${common_flags[@]}
```

### `go test -bench` benchmarks

```bash
$ go test -v -timeout=5m -run=^$ -bench=. -benchmem -args -test.disableLogging
goos: darwin
goarch: amd64
pkg: github.com/ulysseses/raft
Benchmark_gRPCTransport_RTT-8                        	   66874	     16175 ns/op	     597 B/op	      16 allocs/op
Benchmark_1Node_ConsistencyLease_RoundRobin-8        	  179720	      6337 ns/op	     864 B/op	      13 allocs/op
Benchmark_1Node_ConsistencyStrict_RoundRobin-8       	  185638	      6495 ns/op	     856 B/op	      13 allocs/op
Benchmark_1Node_ConsistencyStale_RoundRobin-8        	  236056	      4656 ns/op	     757 B/op	      12 allocs/op
Benchmark_3Node_ConsistencyLease_RoundRobin-8        	       7	 143167377 ns/op	    1442 B/op	      21 allocs/op
Benchmark_3Node_ConsistencyStrict_RoundRobin-8       	       7	 157589649 ns/op	    1449 B/op	      21 allocs/op
Benchmark_3Node_ConsistencyStale_RoundRobin-8        	      10	 169992499 ns/op	    1217 B/op	      21 allocs/op
Benchmark_3Node_ProposalAsLeader-8                   	   98838	     12310 ns/op	    1204 B/op	      16 allocs/op
Benchmark_3Node_ProposalAsFollower-8                 	       6	 199972993 ns/op	     976 B/op	      18 allocs/op
Benchmark_3Node_ConsistencyLease_ReadAsLeader-8   	  418468	      2945 ns/op	     304 B/op	       5 allocs/op
Benchmark_3Node_ConsistencyLease_ReadAsFollower-8    	  201946	      5605 ns/op	     304 B/op	       5 allocs/op
Benchmark_3Node_ConsistencyStrict_ReadAsLeader-8     	     100	 450769282 ns/op	    1560 B/op	      31 allocs/op
--- BENCH: Benchmark_3Node_ConsistencyStrict_ReadAsLeader-8
    functional_test.go:798: failed attempt #1 of getting value 42
    functional_test.go:798: failed attempt #1 of getting value 42
    functional_test.go:798: failed attempt #1 of getting value 42
    functional_test.go:798: failed attempt #1 of getting value 42
    functional_test.go:798: failed attempt #1 of getting value 42
    functional_test.go:798: failed attempt #1 of getting value 42
    functional_test.go:798: failed attempt #1 of getting value 42
    functional_test.go:798: failed attempt #1 of getting value 42
    functional_test.go:798: failed attempt #1 of getting value 42
    functional_test.go:798: failed attempt #1 of getting value 42
	... [output truncated]
Benchmark_3Node_ConsistencyStrict_ReadAsFollower-8   	   94327	     12136 ns/op	     304 B/op	       5 allocs/op
Benchmark_3Node_ConsistencyStale_ReadAsLeader-8      	 2327655	       512 ns/op	     208 B/op	       4 allocs/op
Benchmark_3Node_ConsistencyStale_ReadAsFollower-8    	 2327632	       509 ns/op	     208 B/op	       4 allocs/op
```

## License

This project is licensed under the MIT license.
