package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"sync"
	"sync/atomic"
	"testing"
)

type counterCarrier struct {
	mu       sync.Mutex
	counters map[uint64]struct{}
	writes   int
}

func (c *counterCarrier) ReadPacket([]byte) (int, string, error) { return 0, "", context.Canceled }
func (c *counterCarrier) DefaultPeer() string                    { return "peer" }
func (c *counterCarrier) Name() string                           { return "counter" }
func (c *counterCarrier) Close() error                           { return nil }
func (c *counterCarrier) WritePacket(p []byte, peer string) error {
	if len(p) < 13 {
		return nil
	}
	counter := binary.BigEndian.Uint64(p[5:13])
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.counters == nil {
		c.counters = make(map[uint64]struct{})
	}
	c.counters[counter] = struct{}{}
	c.writes++
	return nil
}

func testSession(t *testing.T) *session {
	t.Helper()
	psk := make([]byte, 32)
	cn := make([]byte, 32)
	sn := make([]byte, 32)
	if _, err := rand.Read(psk); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(cn); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(sn); err != nil {
		t.Fatal(err)
	}
	s, err := deriveSession(psk, cn, sn, "peer", true)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPerformanceDefaults(t *testing.T) {
	var p PerformanceConfig
	applyPerformanceDefaults(&p)
	if p.Profile != "turbo" {
		t.Fatalf("default profile=%q want turbo", p.Profile)
	}
	if p.TunQueues < 1 || p.TunQueues > 4 {
		t.Fatalf("default tun queues=%d want 1..4", p.TunQueues)
	}
	if p.ReceiveWorkers != 1 {
		t.Fatalf("default receive workers=%d want 1 for ordered single-flow UDP receive", p.ReceiveWorkers)
	}
	if p.TxQueueLen != 4096 {
		t.Fatalf("default tx queue=%d want 4096", p.TxQueueLen)
	}
	if p.Qdisc != "fq_codel" {
		t.Fatalf("default qdisc=%q want fq_codel", p.Qdisc)
	}
	if p.SocketBuffer != 8*1024*1024 {
		t.Fatalf("default socket buffer=%d", p.SocketBuffer)
	}
	if err := validatePerformance(p); err != nil {
		t.Fatal(err)
	}
}

func TestOfficialProfilesNormalizeReceiveWorkers(t *testing.T) {
	for _, profile := range []string{"balance", "turbo", "throughput"} {
		p := PerformanceConfig{Profile: profile, ReceiveWorkers: 8}
		applyPerformanceDefaults(&p)
		if p.ReceiveWorkers != 1 {
			t.Fatalf("profile=%s receive_workers=%d want 1", profile, p.ReceiveWorkers)
		}
	}
}

func TestCustomProfileCanOptIntoParallelReceive(t *testing.T) {
	p := PerformanceConfig{Profile: "custom", ReceiveWorkers: 4}
	applyPerformanceDefaults(&p)
	if p.ReceiveWorkers != 4 {
		t.Fatalf("custom receive workers=%d want 4", p.ReceiveWorkers)
	}
	if err := validatePerformance(p); err != nil {
		t.Fatal(err)
	}
}

func TestPerformanceValidationRejectsBadQdisc(t *testing.T) {
	p := PerformanceConfig{Profile: "custom", TunQueues: 2, ReceiveWorkers: 2, TxQueueLen: 4096, Qdisc: "pfifo_fast", SocketBuffer: 8 * 1024 * 1024, StatsIntervalSeconds: 10}
	if err := validatePerformance(p); err == nil {
		t.Fatal("invalid qdisc accepted")
	}
}

func TestRuntimeMetricsSurviveRekey(t *testing.T) {
	st := &tunnelState{hellos: make(map[[32]byte]helloRecord)}
	carrier := &counterCarrier{}

	s1 := testSession(t)
	st.storeSession(s1)
	if err := sendEncrypted(carrier, s1, msgData, make([]byte, 100)); err != nil {
		t.Fatal(err)
	}

	s2 := testSession(t)
	st.storeSession(s2)
	if err := sendEncrypted(carrier, s2, msgData, make([]byte, 200)); err != nil {
		t.Fatal(err)
	}

	if got := atomic.LoadUint64(&st.metrics.dataTxBytes); got != 300 {
		t.Fatalf("cumulative tx=%d want 300", got)
	}
	if got := atomic.LoadUint64(&st.metrics.dataTxPackets); got != 2 {
		t.Fatalf("cumulative tx packets=%d want 2", got)
	}
}

func TestConcurrentSendUsesUniqueNonces(t *testing.T) {
	st := &tunnelState{hellos: make(map[[32]byte]helloRecord)}
	s := testSession(t)
	st.storeSession(s)
	carrier := &counterCarrier{}

	const workers = 8
	const perWorker = 200
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload := make([]byte, 1200)
			for i := 0; i < perWorker; i++ {
				if err := sendEncrypted(carrier, s, msgData, payload); err != nil {
					t.Errorf("sendEncrypted: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	want := workers * perWorker
	carrier.mu.Lock()
	writes := carrier.writes
	unique := len(carrier.counters)
	carrier.mu.Unlock()
	if writes != want {
		t.Fatalf("writes=%d want %d", writes, want)
	}
	if unique != want {
		t.Fatalf("unique counters=%d want %d", unique, want)
	}
	if got := atomic.LoadUint64(&st.metrics.dataTxPackets); got != uint64(want) {
		t.Fatalf("metrics packets=%d want %d", got, want)
	}
}
