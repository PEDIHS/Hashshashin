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
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	magic               = "HSH1"
	msgHello       byte  = 1
	msgHelloAck    byte  = 2
	msgData        byte  = 3
	msgKeepalive   byte  = 4
	transportMark        = 0x77
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

func listenMarkedUDP(ctx context.Context, listen string) (*net.UDPConn, error) {
	lc := net.ListenConfig{Control: func(network, address string, rc syscall.RawConn) error {
		var setErr error
		if err := rc.Control(func(fd uintptr) {
			if e := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, 36, transportMark); e != nil {
				setErr = e
			}
		}); err != nil {
			return err
		}
		return setErr
	}}
	pc, err := lc.ListenPacket(ctx, "udp4", listen)
	if err != nil {
		return nil, fmt.Errorf("listen UDP %s: %w", listen, err)
	}
	u, ok := pc.(*net.UDPConn)
	if !ok {
		_ = pc.Close()
		return nil, fmt.Errorf("unexpected packet connection type")
	}
	return u, nil
}

func clientSupervisor(ctx context.Context, u *net.UDPConn, peer *net.UDPAddr, key []byte, st *tunnelState, c *Config) {
	keep := time.NewTicker(time.Duration(c.Transport.KeepaliveSeconds) * time.Second)
	defer keep.Stop()
	for {
		ss := st.loadSession()
		needHandshake := ss == nil || ss.idle() > time.Duration(c.Transport.SessionTimeoutSeconds)*time.Second || ss.age() > time.Duration(c.Transport.RekeyMinutes)*time.Minute
		if needHandshake {
			hctx, cancel := context.WithTimeout(ctx, 6*time.Second)
			err := handshakeClient(hctx, u, peer, key, st)
			cancel()
			if err != nil && ctx.Err() == nil {
				log.Printf("handshake: %v; retrying", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-keep.C:
			if ss = st.loadSession(); ss != nil {
				_ = sendEncrypted(u, ss, msgKeepalive, nil)
			}
		}
	}
}

func serverSupervisor(ctx context.Context, u *net.UDPConn, st *tunnelState, c *Config) {
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
			_ = sendEncrypted(u, ss, msgKeepalive, nil)
		}
	}
}

func handshakeClient(ctx context.Context, u *net.UDPConn, peer *net.UDPAddr, key []byte, st *tunnelState) error {
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
	if _, err := u.WriteToUDP(hello, peer); err != nil {
		return err
	}
	for {
		if ss := st.loadSession(); ss != nil && ss.created.After(time.Now().Add(-3*time.Second)) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-retry.C:
			if _, err := u.WriteToUDP(hello, peer); err != nil {
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

func recvLoop(ctx context.Context, u *net.UDPConn, tun io.Writer, key []byte, st *tunnelState, role string) {
	b := make([]byte, 65535)
	for {
		n, peer, err := u.ReadFromUDP(b)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("transport receive: %v", err)
			}
			return
		}
		if n < 5 || string(b[:4]) != magic {
			continue
		}
		switch b[4] {
		case msgHello:
			if role == "kharej" {
				handleHello(u, peer, b[:n], key, st)
			}
		case msgHelloAck:
			if role == "iran" {
				handleHelloAck(peer, b[:n], key, st)
			}
		case msgData, msgKeepalive:
			handleEncrypted(tun, peer, b[:n], st)
		}
	}
}

func handleHello(u *net.UDPConn, peer *net.UDPAddr, p, key []byte, st *tunnelState) {
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
		_, _ = u.WriteToUDP(ack, peer)
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
	_, _ = u.WriteToUDP(ack, peer)
	log.Printf("session established from %s", peer)
}

func handleHelloAck(peer *net.UDPAddr, p, key []byte, st *tunnelState) {
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

func handleEncrypted(tun io.Writer, peer *net.UDPAddr, p []byte, st *tunnelState) {
	if len(p) < 29 {
		return
	}
	ss := st.loadSession()
	if ss == nil || !udpAddrEqual(peer, ss.peer) {
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
	if p[4] == msgData && len(plain) > 0 {
		_, _ = tun.Write(plain)
	}
}

func sendEncrypted(u *net.UDPConn, s *session, typ byte, payload []byte) error {
	counter := atomic.AddUint64(&s.tx, 1)
	header := make([]byte, 13)
	copy(header[:4], magic)
	header[4] = typ
	binary.BigEndian.PutUint64(header[5:13], counter)
	nonce := makeNonce(counter)
	ciphertext := s.txAEAD.Seal(nil, nonce[:], payload, header)
	packet := append(header, ciphertext...)
	_, err := u.WriteToUDP(packet, s.peer)
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

func udpAddrEqual(a, b *net.UDPAddr) bool {
	return a != nil && b != nil && a.Port == b.Port && a.IP.Equal(b.IP)
}
