package raft

import (
	"time"
)

type timer struct {
	*time.Timer
}

func (t *timer) stop() bool {
	ret := t.Timer.Stop()
	select {
	case <-t.Timer.C:
	default:
	}
	// t.scr = false
	return ret
}

func (t *timer) Reset(d time.Duration) bool {
	ret := t.stop()
	t.Timer.Reset(d)
	return ret
}

func (t *timer) Stop() bool {
	return t.stop()
}

func newTimer(d time.Duration) *timer {
	return &timer{
		Timer: time.NewTimer(d),
	}
}
