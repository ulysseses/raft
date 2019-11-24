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
)

var port = flag.Int("port", 8080, "port")

type httpKVAPI struct {
	kvStore *raft.KVStore
}

func (h *httpKVAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.RequestURI
	defer r.Body.Close()

	switch r.Method {
	case "PUT":
		v, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Printf("Failed to read on PUT (%v)\n", err)
			http.Error(w, "Failed on PUT", http.StatusBadRequest)
			return
		}
		// Warning: Propose does not commit synchronously
		h.kvStore.Propose(key, string(v))
	case "GET":
		if v, ok := h.kvStore.Get(key); ok {
			w.Write([]byte(v))
		} else {
			http.Error(w, "Failed to GET", http.StatusNotFound)
		}
	default:
		w.Header().Set("Allow", "PUT")
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
			111: fmt.Sprintf("unix://%s", filepath.Join(tmpDir, "TestThreeNodeLinear111.sock")),
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

	kvStore := raftNode.KVStore

	server := http.Server{
		Addr: ":" + strconv.Itoa(*port),
		Handler: &httpKVAPI{
			kvStore: kvStore,
		},
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "hello world!")
}
