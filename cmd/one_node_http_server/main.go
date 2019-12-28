package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"time"

	"github.com/ulysseses/raft/raft"
	"github.com/valyala/fasthttp"
)

var (
	port               = flag.Int("port", 8080, "port")
	cpuProfile         = flag.String("cpu_profile", "", "write cpu profile to `file`")
	cpuProfileDuration = flag.Int("cpu_profile_seconds", 10, "Number of seconds to profile CPU")
	memProfile         = flag.String("mem_profile", "", "write memory profile to `file`")
)

type httpKVAPI struct {
	kvStore *raft.KVStore
}

func (h *httpKVAPI) HandleFastHTTP(ctx *fasthttp.RequestCtx) {
	key := ctx.RequestURI()
	if ctx.IsPut() {
		v := ctx.Request.Body()
		// Warning: Propose does not commit synchronously
		h.kvStore.Propose(string(key), string(v))
	} else if ctx.IsGet() {
		if v, ok := h.kvStore.Get(string(key)); ok {
			ctx.WriteString(v)
		} else {
			ctx.Error("Failed to GET", http.StatusNotFound)
		}
	} else {
		ctx.Response.Header.Set("Allow", "PUT, GET")
		ctx.Error("Method not allowed", http.StatusMethodNotAllowed)
	}
}

func main() {
	flag.Parse()

	if *cpuProfile != "" {
		log.Print("writing cpu_profile to ", *cpuProfile)
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		go func() {
			log.Printf("Stopping CPU profiling in %d seconds...", *cpuProfileDuration)
			time.Sleep(time.Duration(*cpuProfileDuration) * time.Second)
			pprof.StopCPUProfile()
			log.Print("Stopped CPU profiling.")
		}()
	}

	if *memProfile != "" {
		log.Print("writing mem_profile to ", *memProfile)
		f, err := os.Create(*memProfile)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		defer f.Close()
		runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
	}

	// configure a raft node
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	config := &raft.Configuration{
		RecvChanSize: 100,
		SendChanSize: 100,
		ID:           111,
		Peers: map[uint64]string{
			111: fmt.Sprintf("unix://%s", filepath.Join(tmpDir, "one_node_http_server.sock")),
		},
		MinElectionTimeoutTicks: 10,
		MaxElectionTimeoutTicks: 20,
		HeartbeatTicks:          1,
		TickMs:                  2,
	}
	raftNode, err := raft.NewNode(config)
	if err != nil {
		log.Fatal(err)
	}
	defer raftNode.Stop()

	// serve an HTTP KV store backed by a single node raft cluster.
	httpKVAPI := &httpKVAPI{kvStore: raftNode.KVStore}
	log.Printf("starting http kv store back by single raft node at port %d", *port)
	if err := fasthttp.ListenAndServe(":"+strconv.Itoa(*port), httpKVAPI.HandleFastHTTP); err != nil {
		log.Fatal(err)
	}
}
