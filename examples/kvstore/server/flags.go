package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ulysseses/raft/raft"
)

type mapValue struct {
	m *map[uint64]string
}

func (v *mapValue) String() string {
	return "0,tcp://localhost:8080"
}

func (v *mapValue) Set(arg string) error {
	*v.m = map[uint64]string{}
	splits := strings.Split(arg, "|")
	for _, split := range splits {
		innerSplits := strings.Split(split, ",")
		if len(innerSplits) != 2 {
			return fmt.Errorf("key should be separated from value with a comma")
		}
		id, err := strconv.ParseUint(innerSplits[0], 10, 64)
		if err != nil {
			return err
		}
		addr := innerSplits[1]
		(*v.m)[id] = addr
	}
	return nil
}

type consistencyValue struct {
	c *raft.Consistency
}

func (v *consistencyValue) String() string {
	switch *v.c {
	case raft.ConsistencySerializable:
		return "serializable"
	case raft.ConsistencyLinearizable:
		return "linearizable"
	default:
		panic("")
	}
}

func (v *consistencyValue) Set(arg string) error {
	switch arg {
	case "serializable":
		*v.c = raft.ConsistencySerializable
	case "linearizable":
		*v.c = raft.ConsistencyLinearizable
	default:
		return fmt.Errorf(
			"unknown consistency %s; acceptable values: serializable or linearizable",
			arg)
	}
	return nil
}
