// Package raft implements a proof-of-concept Raft consensus algorithm
// (See https://raft.github.io/).
//
// End-users of the raft package should configure the Raft cluster and then
// run the Raft cluster as follows:
//
//  func main() {
//    // Start a 1-node cluster.
//    id := uint64(1)
//    addresses := map[uint64]string{1: "tcp://localhost:8001"}
//    tr, err := raft.NewTransportConfig(id, addresses).Build()
//    if err != nil {
//      log.Fatal(err)
//    }
//    psm, err := raft.NewProtocolConfig(id).Build(tr)
//    if err != nil {
//      log.Fatal(err)
//    }
//    app := newApplication()
//    node, err := raft.NewNodeConfig(id).Build(psm, tr, app)
//    if err != nil {
//      log.Fatal(err)
//    }
//
//    node.Start()
//    defer node.Stop()
//
//    // Here, app can interact with node by calling node.Propose() and node.Read()
//    // ...
//  }
//
// See examples/kvstore/server for an example demo of a key-value store backed by a
// 3-node Raft cluster. Run start3cluster in the provided utils.sh
package raft
