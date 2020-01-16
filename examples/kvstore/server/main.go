package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"

	"github.com/ulysseses/raft/raft"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

var (
	addr       = ":3001"
	cpuProfile = ""
	memProfile = ""

	id            = uint64(1)
	peerAddresses = map[uint64]string{1: "tcp://localhost:8081"}
	consistency   = raft.ConsistencyLinearizable
)

func init() {
	flag.StringVar(&addr, "addr", addr, "address for http kv store")
	flag.StringVar(&cpuProfile, "cpuProfile", cpuProfile, "write cpu profile to a file")
	flag.StringVar(&memProfile, "memProfile", memProfile, "write memory profile to a file")

	flag.Uint64Var(&id, "id", id, "Raft node ID")
	flag.Var(
		&mapValue{m: &peerAddresses},
		"peerAddresses",
		"peer addresses specified as a |-separated string of key-value pairs, "+
			"which themselves are separated by commas. E.g. "+
			"\"1,tcp://localhost:8081|2,tcp://localhost:8082|3,tcp://localhost:8083\"")
	flag.Var(
		&consistencyValue{c: &consistency}, "consistency",
		"consistency level: serializable or linearizable")
}

func main() {
	flag.Parse()

	config, err := raft.BuildSensibleConfiguration(id, peerAddresses, consistency, nil)
	if err != nil {
		fmt.Print(err)
		return
	}
	logger := config.Logger

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

	kvStore := newKVStore()
	node, err := raft.NewNode(config, kvStore)
	if err != nil {
		logger.Fatal("could not create Raft node", zap.Error(err))
	}
	if err := node.Start(); err != nil {
		logger.Fatal("did not start node successfully", zap.Error(err))
	}
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
