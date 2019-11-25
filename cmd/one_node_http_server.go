package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ulysseses/raft/raft"
	"github.com/valyala/fasthttp"
)

var port = flag.Int("port", 8080, "port")

type httpKVAPI struct {
	kvStore *raft.KVStore
}

func (h *httpKVAPI) HandleFastHTTP(ctx *fasthttp.RequestCtx) {
	// fmt.Fprintf(ctx, "Hello, world! Requested path is %q. Foobar is %q",
	// 	ctx.Path(), "foobar")
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

	httpKVAPI := &httpKVAPI{kvStore: raftNode.KVStore}
	if err := fasthttp.ListenAndServe(":"+strconv.Itoa(*port), httpKVAPI.HandleFastHTTP); err != nil {
		log.Fatal(err)
	}
}
