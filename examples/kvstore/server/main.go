package main

import (
	"flag"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/ulysseses/raft/raft"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

var (
	addr       = ":3001"
	cpuProfile = ""
	memProfile = ""

	id             = uint64(1)
	addresses      = map[uint64]string{1: "tcp://localhost:8081"}
	consistency    = raft.ConsistencyLinearizable
	debug          = false
	readTimeout    = time.Second
	proposeTimeout = time.Second
)

func init() {
	flag.StringVar(&addr, "addr", addr, "address for http kv store")
	flag.StringVar(&cpuProfile, "cpuProfile", cpuProfile, "write cpu profile to a file")
	flag.StringVar(&memProfile, "memProfile", memProfile, "write memory profile to a file")

	flag.Uint64Var(&id, "id", id, "Raft node ID")
	flag.Var(
		&mapValue{m: &addresses},
		"addresses",
		"addresses specified as a |-separated string of key-value pairs, "+
			"which themselves are separated by commas. E.g. "+
			"\"1,tcp://localhost:8081|2,tcp://localhost:8082|3,tcp://localhost:8083\"")
	flag.Var(
		&consistencyValue{c: &consistency}, "consistency",
		"consistency level: serializable or linearizable")
	flag.BoolVar(&debug, "debug", debug, "enable debug logs")
	flag.DurationVar(
		&readTimeout, "readTimeout", readTimeout,
		"timeout for read requests to raft cluster")
	flag.DurationVar(
		&proposeTimeout, "proposeTimeout", proposeTimeout,
		"timeout for proposals to raft cluster")
}

func main() {
	flag.Parse()

	if cpuProfile != "" {
		zap.L().Info("writing cpu profile", zap.String("cpuProfile", cpuProfile))
		f, err := os.Create(cpuProfile)
		if err != nil {
			zap.L().Fatal("could not create CPU profile", zap.Error(err))
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			zap.L().Fatal("could not start CPU profile", zap.Error(err))
		}
	}

	if memProfile != "" {
		zap.L().Info("writing mem profile", zap.String("memProfile", memProfile))
		f, err := os.Create(memProfile)
		if err != nil {
			zap.L().Fatal("could not create memory profile", zap.Error(err))
		}
		defer f.Close()
		defer func() {
			runtime.GC() // get up-to-date statistics
			if err := pprof.WriteHeapProfile(f); err != nil {
				zap.L().Fatal("could not write memory profile", zap.Error(err))
			}
		}()
	}

	pConfig := raft.NewProtocolConfig(
		id,
		raft.WithConsistency(consistency),
		raft.WithProtocolDebug(debug))
	tConfig := raft.NewTransportConfig(
		id, addresses,
		raft.WithTransportDebug(debug))
	nConfig := raft.NewNodeConfig(
		id,
		raft.WithNodeDebug(debug))

	kvStore := newKVStore()
	node, err := nConfig.Build(pConfig, tConfig, kvStore)
	if err != nil {
		zap.L().Fatal("could not start Raft node", zap.Error(err))
	}
	kvStore.node = node

	httpKVAPI := &httpKVAPI{
		kvStore:        kvStore,
		logger:         zap.L(),
		readTimeout:    readTimeout,
		proposeTimeout: proposeTimeout,
	}
	zap.L().Info("starting http kv store backed by raft", zap.String("addr", addr))
	if err := fasthttp.ListenAndServe(addr, httpKVAPI.Route); err != nil {
		zap.L().Error("failed to listen", zap.Error(err))
	}
}
