package main

import (
	"fmt"
	"sync"
	"time"
)

type returnPath string

const (
	returnPathDirect returnPath = "direct"
	returnPathTunnel returnPath = "tunnel"
)

type returnPathController struct {
	mu               sync.Mutex
	path             returnPath
	failures         int
	successes        int
	failThreshold    int
	recoverThreshold int
}

func newReturnPathController(failThreshold, recoverThreshold int) *returnPathController {
	return &returnPathController{
		path:             returnPathDirect,
		failThreshold:    failThreshold,
		recoverThreshold: recoverThreshold,
	}
}

// observe implements hysteresis. Direct path needs N consecutive failures to
// fall back to the tunnel, while tunnel fallback needs M consecutive successful
// direct probes before it returns to direct. This prevents route flapping.
func (r *returnPathController) observe(ok bool) (returnPath, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch r.path {
	case returnPathDirect:
		if ok {
			r.failures = 0
			return r.path, false
		}
		r.failures++
		if r.failures >= r.failThreshold {
			r.path = returnPathTunnel
			r.failures = 0
			r.successes = 0
			return r.path, true
		}
	case returnPathTunnel:
		if !ok {
			r.successes = 0
			return r.path, false
		}
		r.successes++
		if r.successes >= r.recoverThreshold {
			r.path = returnPathDirect
			r.successes = 0
			r.failures = 0
			return r.path, true
		}
	}
	return r.path, false
}

func setKharejReturnPath(c *Config, path returnPath) error {
	if c.Role != "kharej" || c.Mode != "direct-return" {
		return nil
	}
	switch path {
	case returnPathDirect:
		runQuiet("ip", "route", "del", c.Network.IranPublicIP+"/32", "dev", c.Tun.Name)
		return nil
	case returnPathTunnel:
		if err := runCmd("ip", "route", "replace", c.Network.IranPublicIP+"/32", "dev", c.Tun.Name); err != nil {
			return fmt.Errorf("install tunnel return route: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unknown return path %q", path)
	}
}

func (s *tunnelState) initProbeAck() {
	s.probeMu.Lock()
	if s.probeAck == nil {
		s.probeAck = make(chan [16]byte, 32)
	}
	s.probeMu.Unlock()
}

func (s *tunnelState) notifyProbeAck(nonce [16]byte) {
	s.probeMu.Lock()
	ch := s.probeAck
	s.probeMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- nonce:
	default:
	}
}

func (s *tunnelState) waitProbeAck(ctxDone interface{ Done() <-chan struct{} }, nonce [16]byte, timeout time.Duration) bool {
	s.initProbeAck()
	s.probeMu.Lock()
	ch := s.probeAck
	s.probeMu.Unlock()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-ctxDone.Done():
			return false
		case <-timer.C:
			return false
		case got := <-ch:
			if got == nonce {
				return true
			}
		}
	}
}
