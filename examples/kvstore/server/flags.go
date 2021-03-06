package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ulysseses/raft"
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
	return v.c.String()
}

func (v *consistencyValue) Set(arg string) error {
	switch strings.ToLower(arg) {
	case "lease":
		*v.c = raft.ConsistencyLease
	case "strict":
		*v.c = raft.ConsistencyStrict
	case "stale":
		*v.c = raft.ConsistencyStale
	default:
		return fmt.Errorf("unknown consistency %s", arg)
	}
	return nil
}
