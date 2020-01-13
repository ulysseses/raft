package main

import (
	"flag"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"time"

	"github.com/ulysseses/raft/raft"
	"github.com/valyala/fasthttp"
)

var (
	port       = 8080
	cpuProfile = ""
	memProfile = ""
	config     = raft.Configuration{
		ID:               0,
		PeerAddresses:    map[uint64]string{0: "tcp://localhost:8080"},
		TickPeriod:       time.Millisecond,
		MinElectionTicks: 10,
		MaxElectionTicks: 20,
		HeartbeatTicks:   1,
		Consistency:      raft.ConsistencySerializable,
		MsgBufferSize:    1,
	}
)

func init() {
	flag.IntVar(&port, "port", port, "port for http kv store")
	flag.StringVar(&cpuProfile, "cpuProfile", cpuProfile, "write cpu profile to a file")
	flag.StringVar(&memProfile, "memProfile", memProfile, "write memory profile to a file")
	flag.Var(
		&mapValue{m: config.PeerAddresses},
		"peerAddresses",
		"peer addresses specified as a |-separated string of key-value pairs, "+
			"which themselves are separated by commas. E.g. "+
			"\"0,tcp://localhost:8080|1,tcp://localhost:8081|2,tcp://localhost:8082\"")
	flag.DurationVar(&config.TickPeriod, "tickPeriod", config.TickPeriod, "tick period")
	flag.UintVar(
		&config.MinElectionTicks, "minElectionTicks", config.MinElectionTicks,
		"minimum number of tick periods before an election timeout should fire")
	flag.UintVar(
		&config.MaxElectionTicks, "maxElectionTicks", config.MaxElectionTicks,
		"maximum number of tick periods before an election timeout should fire")
	flag.UintVar(
		&config.HeartbeatTicks, "heartbeatTicks", config.HeartbeatTicks,
		"number of tick periods before a heartbeat should fire")
	flag.Var(
		&consistencyValue{c: &config.Consistency}, "consistency",
		"consistency level: serializable or linearizable")
	flag.IntVar(
		&config.MsgBufferSize, "msgBufferSize", config.MsgBufferSize,
		"number of Raft protocol messages allowed to be buffered before the "+
			"Raft node can process/send them out.")
}

func main() {
	flag.Parse()

	if cpuProfile != "" {
		log.Print("writing cpu profile to ", cpuProfile)
		f, err := os.Create(cpuProfile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
	}

	if memProfile != "" {
		log.Print("writing mem profile to ", memProfile)
		f, err := os.Create(memProfile)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		defer f.Close()
		defer func() {
			runtime.GC() // get up-to-date statistics
			if err := pprof.WriteHeapProfile(f); err != nil {
				log.Fatal("could not write memory profile: ", err)
			}
		}()
	}

	kvStore := newKVStore()
	node, err := raft.NewNode(config, kvStore)
	if err != nil {
		log.Fatal(err)
	}
	node.Start()
	defer node.Stop()
	kvStore.node = node
	httpKVAPI := &httpKVAPI{kvStore: kvStore}
	log.Printf(
		"starting http kv store back by single raft node at port %d",
		port)
	if err := fasthttp.ListenAndServe(":"+strconv.Itoa(port), httpKVAPI.HandleFastHTTP); err != nil {
		log.Fatal(err)
	}
}
