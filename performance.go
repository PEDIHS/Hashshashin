package main

import (
	"errors"
	"runtime"
	"strings"
)

type PerformanceConfig struct {
	Profile              string `json:"profile,omitempty"`
	TunQueues            int    `json:"tun_queues,omitempty"`
	ReceiveWorkers       int    `json:"receive_workers,omitempty"`
	TxQueueLen           int    `json:"tx_queue_len,omitempty"`
	Qdisc                string `json:"qdisc,omitempty"`
	SocketBuffer         int    `json:"socket_buffer,omitempty"`
	StatsIntervalSeconds int    `json:"stats_interval_seconds,omitempty"`
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func applyPerformanceDefaults(p *PerformanceConfig) {
	p.Profile = strings.ToLower(strings.TrimSpace(p.Profile))
	if p.Profile == "" {
		p.Profile = "turbo"
	}
	cpus := runtime.NumCPU()
	if cpus < 1 {
		cpus = 1
	}

	var queues, txq, sock int
	switch p.Profile {
	case "balance":
		queues = clampInt(cpus, 1, 2)
		txq = 2048
		sock = 4 * 1024 * 1024
	case "throughput":
		queues = clampInt(cpus, 1, 8)
		txq = 8192
		sock = 16 * 1024 * 1024
	case "turbo", "custom":
		queues = clampInt(cpus, 1, 4)
		txq = 4096
		sock = 8 * 1024 * 1024
	default:
		// Validation returns the useful error after defaults are applied.
		queues = clampInt(cpus, 1, 4)
		txq = 4096
		sock = 8 * 1024 * 1024
	}

	if p.TunQueues == 0 {
		p.TunQueues = queues
	}
	if p.ReceiveWorkers == 0 {
		p.ReceiveWorkers = 1
	}
	// A single UDP carrier socket is one ordered outer flow. Multiple goroutines
	// may read that socket concurrently, but scheduling can complete/decrypt
	// adjacent datagrams out of order and create artificial TCP reordering in a
	// Full Tunnel. Official presets therefore keep one carrier receive worker;
	// the expensive Iran upload path is still parallelized by TUN multi-queue.
	// Custom remains available for experiments until true multi-flow/multipath
	// transport can give each receive worker an independent ordered outer flow.
	if p.Profile != "custom" {
		p.ReceiveWorkers = 1
	}
	if p.TxQueueLen == 0 {
		p.TxQueueLen = txq
	}
	if p.Qdisc == "" {
		p.Qdisc = "fq_codel"
	}
	if p.SocketBuffer == 0 {
		p.SocketBuffer = sock
	}
	if p.StatsIntervalSeconds == 0 {
		p.StatsIntervalSeconds = 10
	}
}

func validatePerformance(p PerformanceConfig) error {
	switch p.Profile {
	case "balance", "turbo", "throughput", "custom":
	default:
		return errors.New("performance.profile must be balance, turbo, throughput or custom")
	}
	if p.TunQueues < 1 || p.TunQueues > 32 {
		return errors.New("performance.tun_queues must be 1..32")
	}
	if p.ReceiveWorkers < 1 || p.ReceiveWorkers > 32 {
		return errors.New("performance.receive_workers must be 1..32")
	}
	if p.TxQueueLen < 256 || p.TxQueueLen > 65536 {
		return errors.New("performance.tx_queue_len must be 256..65536")
	}
	switch p.Qdisc {
	case "fq_codel", "fq", "none":
	default:
		return errors.New("performance.qdisc must be fq_codel, fq or none")
	}
	if p.SocketBuffer < 256*1024 || p.SocketBuffer > 64*1024*1024 {
		return errors.New("performance.socket_buffer must be between 256KiB and 64MiB")
	}
	if p.StatsIntervalSeconds < 5 || p.StatsIntervalSeconds > 60 {
		return errors.New("performance.stats_interval_seconds must be 5..60")
	}
	return nil
}
