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
}

func (s *tunnelState) loadSession() *session {
	s.sessMu.RLock()
	defer s.sessMu.RUnlock()
	return s.session
}

func (s *tunnelState) storeSession(v *session) {
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
	plain, err := ss.rxAEAD.Open(nil, nonce[:], p[13:], p[:13])
	if err != nil || !ss.replay.accept(counter) {
		return
	}
	ss.touch()
	atomic.AddUint64(&ss.rxBytes, uint64(len(plain)))
	switch p[4] {
	case msgData:
		if len(plain) > 0 {
			_, _ = tun.Write(plain)
		}
	case msgDirectProbeAck:
		if len(plain) == 16 {
			var probeNonce [16]byte
			copy(probeNonce[:], plain)
			st.notifyProbeAck(probeNonce)
		}
	}
}

func sendEncrypted(carrier packetCarrier, s *session, typ byte, payload []byte) error {
	counter := atomic.AddUint64(&s.tx, 1)
	header := make([]byte, 13)
	copy(header[:4], magic)
	header[4] = typ
	binary.BigEndian.PutUint64(header[5:13], counter)
	nonce := makeNonce(counter)
	ciphertext := s.txAEAD.Seal(nil, nonce[:], payload, header)
	packet := append(header, ciphertext...)
	err := carrier.WritePacket(packet, s.peer)
	if err == nil {
		atomic.AddUint64(&s.txBytes, uint64(len(payload)))
	}
	return err
}

func statsLoop(ctx context.Context, st *tunnelState) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s := st.loadSession(); s != nil {
				log.Printf("stats peer=%s tx=%dB rx=%dB idle=%s", s.peer, atomic.LoadUint64(&s.txBytes), atomic.LoadUint64(&s.rxBytes), s.idle().Round(time.Second))
			}
		}
	}
}
