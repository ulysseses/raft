package raft

import "flag"

var testDisableLoggingFlag *bool

func init() {
	testDisableLoggingFlag = flag.Bool("test.disableLogging", false, "disable logging for the Raft node in test")
}
