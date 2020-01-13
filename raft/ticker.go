package raft

import (
	"math/rand"
	"time"
)

// Ticker sends ticks.
type Ticker interface {
	// Start starts the ticker.
	Start()
	// Stop stops the ticker.
	Stop()
	// C returns the channel to read ticks from.
	C() <-chan struct{}

	// Reset resets the ticker.
	Reset()
}

type heartbeatTicker struct {
	t         time.Ticker
	cChan     chan struct{}
	stopChan  chan struct{}
	resetChan chan struct{}

	tick           uint
	heartbeatTicks uint
}

func (t *heartbeatTicker) loop() {
	select {
	case <-t.t.C:
	default:
	}

	for range t.t.C {
		select {
		case <-t.stopChan:
			return
		case <-t.resetChan:
			t.tick = 0
			continue
		default:
		}
		t.tick++
		if t.tick == t.heartbeatTicks {
			select {
			case t.cChan <- struct{}{}:
			default:
			}
			t.tick = 0
		}
	}
}

// Start: heartbeatTicker implements Ticker
func (t *heartbeatTicker) Start() {
	go t.loop()
}

// Stop: heartbeatTicker implements Ticker
func (t *heartbeatTicker) Stop() {
	t.stopChan <- struct{}{}
	t.t.Stop()
	select {
	case <-t.t.C:
	default:
	}
}

// C: heartbeatTicker implements Ticker
func (t *heartbeatTicker) C() <-chan struct{} {
	return t.cChan
}

// Reset: heartbeatTicker implements Ticker
func (t *heartbeatTicker) Reset() {
	t.resetChan <- struct{}{}
}

func newHeartbeatTicker(tickPeriod time.Duration, heartbeatTicks uint) Ticker {
	t := heartbeatTicker{
		t:              *time.NewTicker(tickPeriod),
		cChan:          make(chan struct{}, 1),
		stopChan:       make(chan struct{}),
		resetChan:      make(chan struct{}),
		heartbeatTicks: heartbeatTicks,
	}
	return &t
}

type electionTicker struct {
	t                                  time.Ticker
	cChan                              chan struct{}
	stopChan                           chan struct{}
	resetChan                          chan struct{}
	tick                               uint
	electionTicks                      uint
	minElectionTicks, maxElectionTicks uint
}

func (t *electionTicker) loop() {
	select {
	case <-t.t.C:
	default:
	}

	for range t.t.C {
		select {
		case <-t.stopChan:
			return
		case <-t.resetChan:
			t.tick = 0
			t.electionTicks = t.randomElectionTimeout()
			continue
		default:
		}
		t.tick++
		if t.tick == t.electionTicks {
			select {
			case t.cChan <- struct{}{}:
			default:
			}
			t.tick = 0
			t.electionTicks = t.randomElectionTimeout()
		}
	}
}

func (t *electionTicker) randomElectionTimeout() uint {
	return uint(rand.Uint32())%(t.maxElectionTicks-t.minElectionTicks+1) + t.minElectionTicks
}

// Start: electionTicker implements Ticker
func (t *electionTicker) Start() {
	go t.loop()
}

// Stop: electionTicker implements Ticker
func (t *electionTicker) Stop() {
	t.stopChan <- struct{}{}
	t.t.Stop()
	select {
	case <-t.t.C:
	default:
	}
}

// C: electionTicker implements Ticker
func (t *electionTicker) C() <-chan struct{} {
	return t.cChan
}

// Reset: electionTicker implements Ticker
func (t *electionTicker) Reset() {
	t.resetChan <- struct{}{}
}

func newElectionTicker(tickPeriod time.Duration, minElectionTicks, maxElectionTicks uint) Ticker {
	t := electionTicker{
		t:                *time.NewTicker(tickPeriod),
		cChan:            make(chan struct{}, 1),
		stopChan:         make(chan struct{}),
		resetChan:        make(chan struct{}),
		minElectionTicks: minElectionTicks,
		maxElectionTicks: maxElectionTicks,
	}
	t.electionTicks = t.randomElectionTimeout()
	return &t
}
