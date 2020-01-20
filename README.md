# raft
Repository trying to POC a raft implementation

## Tooling dependencies

```bash
cd $GOPATH/src
go get -u github.com/gogo/protobuf/protoc-gen-gofast
github.com/gogo/protobuf/install-protobuf.sh  # it you don't already have protoc
```

## Example KV Store

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

## Building Protobuf files

```bash
# make sure you're in repo base
protoc \
  -I raftpb/ \
  -I $GOPATH/src \
  raftpb/raft.proto \
  --gogofaster_out=plugins=grpc:raftpb

protoc \
  -I examples/kvstore/kvpb/ \
  -I $GOPATH/src \
  examples/kvstore/kvpb/kv.proto \
  --gogofaster_out=plugins=grpc:examples/kvstore/kvpb
```