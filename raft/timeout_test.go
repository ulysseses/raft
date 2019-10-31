package raft

import (
	"testing"
	"time"
)

// potentially flaky
func TestReset(t *testing.T) {
	timer := newTimer(time.Millisecond)
	got := timer.Reset(time.Millisecond)
	if !got {
		t.Fatal("Reset returned false, but expected true")
	}
	got = timer.Reset(time.Millisecond)
	if !got {
		t.Fatal("Reset returned false, but expected true")
	}
	time.Sleep(2 * time.Millisecond)
	got = timer.Reset(time.Millisecond)
	if got {
		t.Fatal("Reset return true, but expected false")
	}
}

func TestStop(t *testing.T) {
	t.Run("stop a running timer", func(t *testing.T) {
		timer := newTimer(time.Millisecond)
		got := timer.Stop()
		if !got {
			t.FailNow()
		}
	})
	t.Run("stop a stopped timer", func(t *testing.T) {
		timer := newTimer(time.Millisecond)
		time.Sleep(2 * time.Millisecond)
		got := timer.Stop()
		if got {
			t.FailNow()
		}
	})
}
