// timeout_test.go tests are potentially flaky (due to timing). Flaky tests may fail
// when there is heavy CPU contention between this test and other processes.
package raft

import (
	"testing"
	"time"
)

func Test_heartbeatTicker(t *testing.T) {
	ticker := newHeartbeatTicker(time.Millisecond, 1)
	ticker.Start()
	ticker.Reset()
	defer ticker.Stop()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			<-ticker.C()
		}
		done <- struct{}{}
	}()
	select {
	case <-done:
	case <-time.After(11 * time.Millisecond):
		t.FailNow()
	}
}

func Test_electionTicker(t *testing.T) {
	ticker := newElectionTicker(time.Millisecond, 1, 3)
	ticker.Start()
	ticker.Reset()
	defer ticker.Stop()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			<-ticker.C()
		}
		done <- struct{}{}
	}()
	select {
	case <-done:
	case <-time.After(31 * time.Millisecond):
		t.FailNow()
	}
}
