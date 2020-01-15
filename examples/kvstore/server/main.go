package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/ulysseses/raft/raft"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

var (
	addr       = ":3001"
	cpuProfile = ""
	memProfile = ""
	config     = raft.Configuration{
		ID:                     1,
		PeerAddresses:          map[uint64]string{1: "tcp://localhost:8081"},
		TickPeriod:             time.Second,
		MinElectionTicks:       5,
		MaxElectionTicks:       15,
		HeartbeatTicks:         2,
		Consistency:            raft.ConsistencySerializable,
		MsgBufferSize:          8,
		DialTimeout:            time.Second,
		ConnectionAttemptDelay: time.Second,
		GRPCOptions: []raft.GRPCOption{
			raft.WithGRPCDialOption{Opt: grpc.WithInsecure()},
		},
	}
)

func init() {
	flag.StringVar(&addr, "addr", addr, "address for http kv store")
	flag.StringVar(&cpuProfile, "cpuProfile", cpuProfile, "write cpu profile to a file")
	flag.StringVar(&memProfile, "memProfile", memProfile, "write memory profile to a file")
	flag.Uint64Var(&config.ID, "id", config.ID, "Raft node ID")
	flag.Var(
		&mapValue{m: &config.PeerAddresses},
		"peerAddresses",
		"peer addresses specified as a |-separated string of key-value pairs, "+
			"which themselves are separated by commas. E.g. "+
			"\"1,tcp://localhost:8081|2,tcp://localhost:8082|3,tcp://localhost:8083\"")
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

	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Print(err)
		return
	}
	sugaredLogger := logger.Sugar()

	if cpuProfile != "" {
		logger.Info("writing cpu profile", zap.String("cpuProfile", cpuProfile))
		f, err := os.Create(cpuProfile)
		if err != nil {
			logger.Fatal("could not create CPU profile", zap.Error(err))
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			logger.Fatal("could not start CPU profile", zap.Error(err))
		}
	}

	if memProfile != "" {
		logger.Info("writing mem profile", zap.String("memProfile", memProfile))
		f, err := os.Create(memProfile)
		if err != nil {
			logger.Fatal("could not create memory profile", zap.Error(err))
		}
		defer f.Close()
		defer func() {
			runtime.GC() // get up-to-date statistics
			if err := pprof.WriteHeapProfile(f); err != nil {
				logger.Fatal("could not write memory profile", zap.Error(err))
			}
		}()
	}

	config.Logger = logger

	kvStore := newKVStore()
	node, err := raft.NewNode(config, kvStore)
	if err != nil {
		logger.Fatal("could not create Raft node", zap.Error(err))
	}
	if err := node.Start(); err != nil {
		sugaredLogger.Error(err)
	}
	defer func() {
		if err := node.Stop(); err != nil {
			sugaredLogger.Error(err)
		}
	}()
	kvStore.node = node
	httpKVAPI := &httpKVAPI{
		kvStore:       kvStore,
		logger:        logger,
		sugaredLogger: logger.Sugar(),
	}
	logger.Info("starting http kv store backed by raft", zap.String("addr", addr))
	if err := fasthttp.ListenAndServe(addr, httpKVAPI.Route); err != nil {
		logger.Error("failed to listen", zap.Error(err))
	}
}
