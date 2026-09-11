package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	magic                   = "HSH1"
	msgHello           byte = 1
	msgHelloAck        byte = 2
	msgData            byte = 3
	msgKeepalive       byte = 4
	msgDirectProbeAck  byte = 5
	transportMark           = 0x77
)

type helloRecord struct {
	expires time.Time
	ack     []byte
}

type runtimeMetrics struct {
	dataTxBytes    uint64
	dataRxBytes    uint64
	dataTxPackets  uint64
	dataRxPackets  uint64
	controlTxBytes uint64
	controlRxBytes uint64
	controlTxPkts  uint64
	controlRxPkts  uint64
	sendErrors     uint64
	decryptErrors  uint64
	replayDrops    uint64
	noSessionDrops uint64
	tunReadErrors  uint64
}

type tunnelState struct {
	sessMu     sync.RWMutex
	session    *session
	pendingMu  sync.Mutex
	pending    [32]byte
	pendingSet bool
	helloMu    sync.Mutex
	hellos     map[[32]byte]helloRecord
	probeMu    sync.Mutex
	probeAck   chan [16]byte
	metrics    runtimeMetrics
}

var encryptedPacketPool = sync.Pool{New: func() interface{} {
	b := make([]byte, 0, 65536+128)
	return &b
}}

func (s *tunnelState) loadSession() *session {
	s.sessMu.RLock()
	defer s.sessMu.RUnlock()
	return s.session
}

func (s *tunnelState) storeSession(v *session) {
	if v != nil {
		v.metrics = &s.metrics
	}
	s.sessMu.Lock()
	s.session = v
	s.sessMu.Unlock()
}

func (s *tunnelState) clearIf(v *session) bool {
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	if s.session != v {
		return false
	}
	s.session = nil
	return true
}

func clientSupervisor(ctx context.Context, carrier packetCarrier, key []byte, st *tunnelState, c *Config) {
	keep := time.NewTicker(time.Duration(c.Transport.KeepaliveSeconds) * time.Second)
	defer keep.Stop()
	for {
		ss := st.loadSession()
		needHandshake := ss == nil || ss.idle() > time.Duration(c.Transport.SessionTimeoutSeconds)*time.Second || ss.age() > time.Duration(c.Transport.RekeyMinutes)*time.Minute
		if needHandshake {
			hctx, cancel := context.WithTimeout(ctx, 6*time.Second)
			err := handshakeClient(hctx, carrier, carrier.DefaultPeer(), key, st)
			cancel()
			if err != nil && ctx.Err() == nil {
				log.Printf("%s handshake: %v; retrying", carrier.Name(), err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-keep.C:
			if ss = st.loadSession(); ss != nil {
				_ = sendEncrypted(carrier, ss, msgKeepalive, nil)
			}
		}
	}
}

func serverSupervisor(ctx context.Context, carrier packetCarrier, st *tunnelState, c *Config) {
	ticker := time.NewTicker(time.Duration(c.Transport.KeepaliveSeconds) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ss := st.loadSession()
			if ss == nil {
				continue
			}
			if ss.idle() > time.Duration(c.Transport.SessionTimeoutSeconds)*time.Second {
				st.clearIf(ss)
				log.Printf("session expired; waiting for Iran re-handshake")
				continue
			}
			_ = sendEncrypted(carrier, ss, msgKeepalive, nil)
		}
	}
}

func handshakeClient(ctx context.Context, carrier packetCarrier, peer string, key []byte, st *tunnelState) error {
	if peer == "" {
		return fmt.Errorf("carrier peer is not configured")
	}
	var cn [32]byte
	if _, err := rand.Read(cn[:]); err != nil {
		return err
	}
	hello := makeHello(key, cn)
	st.pendingMu.Lock()
	st.pending = cn
	st.pendingSet = true
	st.pendingMu.Unlock()
	defer func() {
		st.pendingMu.Lock()
		if st.pending == cn {
			st.pendingSet = false
		}
		st.pendingMu.Unlock()
	}()

	retry := time.NewTicker(500 * time.Millisecond)
	poll := time.NewTicker(50 * time.Millisecond)
	defer retry.Stop()
	defer poll.Stop()
	if err := carrier.WritePacket(hello, peer); err != nil {
		return err
	}
	started := time.Now()
	for {
		if ss := st.loadSession(); ss != nil && ss.created.After(started.Add(-time.Second)) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-retry.C:
			if err := carrier.WritePacket(hello, peer); err != nil {
				return err
			}
		case <-poll.C:
		}
	}
}

func makeHello(key []byte, cn [32]byte) []byte {
	p := make([]byte, 45)
	copy(p[:4], magic)
	p[4] = msgHello
	binary.BigEndian.PutUint64(p[5:13], uint64(time.Now().Unix()))
	copy(p[13:45], cn[:])
	h := hmac.New(sha256.New, key)
	h.Write(p)
	return append(p, h.Sum(nil)...)
}

func makeHelloAck(key []byte, cn, sn [32]byte) []byte {
	p := make([]byte, 69)
	copy(p[:4], magic)
	p[4] = msgHelloAck
	copy(p[5:37], cn[:])
	copy(p[37:69], sn[:])
	h := hmac.New(sha256.New, key)
	h.Write(p)
	return append(p, h.Sum(nil)...)
}

func recvLoop(ctx context.Context, carrier packetCarrier, tun io.Writer, key []byte, st *tunnelState, role string) {
	b := make([]byte, maxCarrierFrame)
	for {
		n, peer, err := carrier.ReadPacket(b)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("%s receive: %v", carrier.Name(), err)
			}
			if ctx.Err() != nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if n < 5 || string(b[:4]) != magic {
			continue
		}
		switch b[4] {
		case msgHello:
			if role == "kharej" {
				handleHello(carrier, peer, b[:n], key, st)
			}
		case msgHelloAck:
			if role == "iran" {
				handleHelloAck(peer, b[:n], key, st)
			}
		case msgData, msgKeepalive, msgDirectProbeAck:
			handleEncrypted(tun, peer, b[:n], st)
		}
	}
}

func handleHello(carrier packetCarrier, peer string, p, key []byte, st *tunnelState) {
	if len(p) != 77 {
		return
	}
	h := hmac.New(sha256.New, key)
	h.Write(p[:45])
	if !hmac.Equal(h.Sum(nil), p[45:77]) {
		return
	}
	ts := int64(binary.BigEndian.Uint64(p[5:13]))
	if skew := time.Since(time.Unix(ts, 0)); skew > 90*time.Second || skew < -90*time.Second {
		return
	}

	var cn [32]byte
	copy(cn[:], p[13:45])
	st.helloMu.Lock()
	now := time.Now()
	for k, v := range st.hellos {
		if now.After(v.expires) {
			delete(st.hellos, k)
		}
	}
	if old, ok := st.hellos[cn]; ok {
		ack := append([]byte(nil), old.ack...)
		st.helloMu.Unlock()
		_ = carrier.WritePacket(ack, peer)
		return
	}
	var sn [32]byte
	if _, err := rand.Read(sn[:]); err != nil {
		st.helloMu.Unlock()
		return
	}
	ss, err := deriveSession(key, cn[:], sn[:], peer, false)
	if err != nil {
		st.helloMu.Unlock()
		return
	}
	ack := makeHelloAck(key, cn, sn)
	st.hellos[cn] = helloRecord{expires: now.Add(2 * time.Minute), ack: append([]byte(nil), ack...)}
	st.helloMu.Unlock()

	st.storeSession(ss)
	_ = carrier.WritePacket(ack, peer)
	log.Printf("session established from %s over %s", peer, carrier.Name())
}

func handleHelloAck(peer string, p, key []byte, st *tunnelState) {
	if len(p) != 101 {
		return
	}
	h := hmac.New(sha256.New, key)
	h.Write(p[:69])
	if !hmac.Equal(h.Sum(nil), p[69:101]) {
		return
	}
	var cn [32]byte
	copy(cn[:], p[5:37])

	st.pendingMu.Lock()
	ok := st.pendingSet && st.pending == cn
	if ok {
		st.pendingSet = false
	}
	st.pendingMu.Unlock()
	if !ok {
		return
	}

	ss, err := deriveSession(key, p[5:37], p[37:69], peer, true)
	if err != nil {
		return
	}
	st.storeSession(ss)
	log.Printf("session established to %s", peer)
}

func handleEncrypted(tun io.Writer, peer string, p []byte, st *tunnelState) {
	if len(p) < 29 {
		return
	}
	ss := st.loadSession()
	if ss == nil || peer != ss.peer {
		return
	}
	counter := binary.BigEndian.Uint64(p[5:13])
	nonce := makeNonce(counter)

	// Decrypt in place. The old path used Open(nil, ...), allocating a fresh
	// slice for every packet. The ciphertext area is no longer needed once the
	// tag verifies, so reusing it removes a hot-path allocation and copy.
	plain, err := ss.rxAEAD.Open(p[13:13], nonce[:], p[13:], p[:13])
	if err != nil {
		atomic.AddUint64(&st.metrics.decryptErrors, 1)
		return
	}
	if !ss.replay.accept(counter) {
		atomic.AddUint64(&st.metrics.replayDrops, 1)
		return
	}
	ss.touch()
	atomic.AddUint64(&ss.rxBytes, uint64(len(plain)))
	switch p[4] {
	case msgData:
		atomic.AddUint64(&st.metrics.dataRxBytes, uint64(len(plain)))
		atomic.AddUint64(&st.metrics.dataRxPackets, 1)
		if len(plain) > 0 {
			_, _ = tun.Write(plain)
		}
	case msgDirectProbeAck:
		atomic.AddUint64(&st.metrics.controlRxBytes, uint64(len(plain)))
		atomic.AddUint64(&st.metrics.controlRxPkts, 1)
		if len(plain) == 16 {
			var probeNonce [16]byte
			copy(probeNonce[:], plain)
			st.notifyProbeAck(probeNonce)
		}
	default:
		atomic.AddUint64(&st.metrics.controlRxBytes, uint64(len(plain)))
		atomic.AddUint64(&st.metrics.controlRxPkts, 1)
	}
}

func sendEncrypted(carrier packetCarrier, s *session, typ byte, payload []byte) error {
	counter := atomic.AddUint64(&s.tx, 1)
	holder := encryptedPacketPool.Get().(*[]byte)
	buf := *holder
	need := 13 + len(payload) + s.txAEAD.Overhead()
	if cap(buf) < need {
		buf = make([]byte, 13, need)
	} else {
		buf = buf[:13]
	}
	copy(buf[:4], magic)
	buf[4] = typ
	binary.BigEndian.PutUint64(buf[5:13], counter)
	nonce := makeNonce(counter)
	packet := s.txAEAD.Seal(buf, nonce[:], payload, buf[:13])
	err := carrier.WritePacket(packet, s.peer)
	if err == nil {
		atomic.AddUint64(&s.txBytes, uint64(len(payload)))
		if s.metrics != nil {
			if typ == msgData {
				atomic.AddUint64(&s.metrics.dataTxBytes, uint64(len(payload)))
				atomic.AddUint64(&s.metrics.dataTxPackets, 1)
			} else {
				atomic.AddUint64(&s.metrics.controlTxBytes, uint64(len(payload)))
				atomic.AddUint64(&s.metrics.controlTxPkts, 1)
			}
		}
	} else if s.metrics != nil {
		atomic.AddUint64(&s.metrics.sendErrors, 1)
	}
	*holder = packet[:0]
	if cap(*holder) <= maxCarrierFrame+128 {
		encryptedPacketPool.Put(holder)
	}
	return err
}

func readInterfaceCounter(name, stat string) uint64 {
	b, err := os.ReadFile("/sys/class/net/" + name + "/statistics/" + stat)
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	return v
}

func statsLoop(ctx context.Context, st *tunnelState, c *Config) {
	interval := time.Duration(c.Performance.StatsIntervalSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var prevTx, prevRx uint64
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tx := atomic.LoadUint64(&st.metrics.dataTxBytes)
			rx := atomic.LoadUint64(&st.metrics.dataRxBytes)
			txRate := float64(tx-prevTx) * 8 / interval.Seconds() / 1e6
			rxRate := float64(rx-prevRx) * 8 / interval.Seconds() / 1e6
			prevTx, prevRx = tx, rx
			peer, idle := "-", "-"
			if s := st.loadSession(); s != nil {
				peer = s.peer
				idle = s.idle().Round(time.Millisecond).String()
			}
			log.Printf("stats role=%s mode=%s peer=%s hsh_data_tx=%dB/%dpkts hsh_data_rx=%dB/%dpkts rate_tx=%.1fMbit/s rate_rx=%.1fMbit/s ctrl_tx=%dpkts ctrl_rx=%dpkts tun_tx_drop=%d tun_rx_drop=%d send_err=%d decrypt_err=%d replay_drop=%d no_session_drop=%d idle=%s",
				c.Role, c.Mode, peer,
				tx, atomic.LoadUint64(&st.metrics.dataTxPackets),
				rx, atomic.LoadUint64(&st.metrics.dataRxPackets), txRate, rxRate,
				atomic.LoadUint64(&st.metrics.controlTxPkts), atomic.LoadUint64(&st.metrics.controlRxPkts),
				readInterfaceCounter(c.Tun.Name, "tx_dropped"), readInterfaceCounter(c.Tun.Name, "rx_dropped"),
				atomic.LoadUint64(&st.metrics.sendErrors), atomic.LoadUint64(&st.metrics.decryptErrors),
				atomic.LoadUint64(&st.metrics.replayDrops), atomic.LoadUint64(&st.metrics.noSessionDrops), idle)
		}
	}
}
