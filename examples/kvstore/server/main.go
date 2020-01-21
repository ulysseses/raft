package main

import (
	"flag"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/ulysseses/raft/raft"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	addr       = ":3001"
	cpuProfile = ""
	memProfile = ""

	id             = uint64(1)
	addresses      = map[uint64]string{1: "tcp://localhost:8081"}
	consistency    = raft.ConsistencyLinearizable
	enableLogging  = false
	debug          = false
	readTimeout    = 5 * time.Second
	proposeTimeout = 5 * time.Second
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
	flag.BoolVar(&enableLogging, "enableLogging", enableLogging, "enable logging")
	flag.BoolVar(&debug, "debug", debug, "enable debug logs")
	flag.DurationVar(
		&readTimeout, "readTimeout", readTimeout,
		"timeout for read requests to raft cluster")
	flag.DurationVar(
		&proposeTimeout, "proposeTimeout", proposeTimeout,
		"timeout for proposals to raft cluster")
}

func globalLogger(debug bool) *zap.Logger {
	loggerCfg := zap.NewProductionConfig()
	if debug {
		loggerCfg.Level.SetLevel(zapcore.DebugLevel)
	}
	logger, err := loggerCfg.Build()
	if err != nil {
		panic(err)
	}
	return logger
}

func main() {
	flag.Parse()

	gl := globalLogger(debug)

	signalChan := make(chan os.Signal, 2)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	var cpuProfChan, memProfChan chan struct{}
	var cpuProfAckChan, memProfAckChan chan struct{}

	go func() {
		<-signalChan
		select {
		case cpuProfChan <- struct{}{}:
			select {
			case <-cpuProfAckChan:
			case <-time.After(2 * time.Second):
				gl.Error("could not write cpu profile out (timed out after 2 seconds)")
			}
		default:
		}

		select {
		case memProfChan <- struct{}{}:
			select {
			case <-memProfAckChan:
			case <-time.After(2 * time.Second):
				gl.Error("could not write mem profile out (timed out after 2 seconds)")
			}
		default:
		}

		os.Exit(0)
	}()

	if cpuProfile != "" {
		cpuProfChan = make(chan struct{})
		cpuProfAckChan = make(chan struct{})
		gl.Info("writing cpu profile", zap.String("cpuProfile", cpuProfile))
		f, err := os.Create(cpuProfile)
		if err != nil {
			gl.Fatal("could not create CPU profile", zap.Error(err))
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			gl.Fatal("could not start CPU profile", zap.Error(err))
		}
		go func(f *os.File) {
			<-cpuProfChan
			gl.Sugar().Info("profile: caught interrupt, stopping cpu profile")
			pprof.StopCPUProfile()
			if err := f.Close(); err != nil {
				gl.Sugar().Error(err)
			}
			cpuProfAckChan <- struct{}{}
		}(f)
	}

	if memProfile != "" {
		memProfChan = make(chan struct{})
		memProfAckChan = make(chan struct{})
		gl.Info("writing mem profile", zap.String("memProfile", memProfile))
		f, err := os.Create(memProfile)
		if err != nil {
			gl.Fatal("could not create memory profile", zap.Error(err))
		}
		go func(f *os.File) {
			<-memProfChan
			gl.Sugar().Info("profile: caught interrupt, stopping mem profile")
			runtime.GC()
			if err := pprof.WriteHeapProfile(f); err != nil {
				gl.Error("could not write memory profile", zap.Error(err))
			}
			f.Close()
			memProfAckChan <- struct{}{}
		}(f)
	}

	pConfigOpts := []raft.ProtocolConfigOption{raft.WithConsistency(consistency)}
	tConfigOpts := []raft.TransportConfigOption{}
	nConfigOpts := []raft.NodeConfigOption{}
	pConfigOpts = append(pConfigOpts, raft.WithProtocolDebug(debug))
	tConfigOpts = append(tConfigOpts, raft.WithTransportDebug(debug))
	nConfigOpts = append(nConfigOpts, raft.WithNodeDebug(debug))
	if enableLogging {
		pConfigOpts = append(pConfigOpts, raft.AddProtocolLogger())
		tConfigOpts = append(tConfigOpts, raft.AddTransportLogger())
		nConfigOpts = append(nConfigOpts, raft.AddNodeLogger())
	}

	pConfig := raft.NewProtocolConfig(id, pConfigOpts...)
	tConfig := raft.NewTransportConfig(id, addresses, tConfigOpts...)
	nConfig := raft.NewNodeConfig(id, nConfigOpts...)

	kvStore := newKVStore()
	node, err := nConfig.Build(pConfig, tConfig, kvStore)
	if err != nil {
		gl.Fatal("could not start Raft node", zap.Error(err))
	}
	kvStore.node = node

	node.Start()
	defer func() {
		if err := node.Stop(); err != nil {
			gl.Error("node stopped with error", zap.Error(err))
		}
	}()

	httpKVAPILogger := gl
	if !enableLogging {
		httpKVAPILogger = nil
	}
	httpKVAPI := &httpKVAPI{
		kvStore:        kvStore,
		logger:         httpKVAPILogger,
		readTimeout:    readTimeout,
		proposeTimeout: proposeTimeout,
	}
	gl.Info("starting http kv store backed by raft", zap.String("addr", addr))
	if err := fasthttp.ListenAndServe(addr, httpKVAPI.Route); err != nil {
		gl.Error("failed to listen", zap.Error(err))
	}
}
