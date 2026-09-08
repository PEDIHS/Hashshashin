package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"net"
	"testing"
	"time"
)

func TestDirectionalSessionKeys(t *testing.T) {
	psk := make([]byte, 32)
	cn := make([]byte, 32)
	sn := make([]byte, 32)
	_, _ = rand.Read(psk)
	_, _ = rand.Read(cn)
	_, _ = rand.Read(sn)
	client, err := deriveSession(psk, cn, sn, "peer", true)
	if err != nil {
		t.Fatal(err)
	}
	server, err := deriveSession(psk, cn, sn, "peer", false)
	if err != nil {
		t.Fatal(err)
	}
	aad := []byte("header")
	nonce := makeNonce(1)
	msg := []byte("hashshashin")
	ct := client.txAEAD.Seal(nil, nonce[:], msg, aad)
	got, err := server.rxAEAD.Open(nil, nonce[:], ct, aad)
	if err != nil || !bytes.Equal(got, msg) {
		t.Fatalf("c2s decrypt failed: %v", err)
	}
	ct = server.txAEAD.Seal(nil, nonce[:], msg, aad)
	got, err = client.rxAEAD.Open(nil, nonce[:], ct, aad)
	if err != nil || !bytes.Equal(got, msg) {
		t.Fatalf("s2c decrypt failed: %v", err)
	}
}

func TestReplayWindow(t *testing.T) {
	var r replayWindow
	if !r.accept(10) {
		t.Fatal("first packet rejected")
	}
	if !r.accept(12) {
		t.Fatal("new packet rejected")
	}
	if !r.accept(11) {
		t.Fatal("valid out-of-order packet rejected")
	}
	if r.accept(11) {
		t.Fatal("duplicate accepted")
	}
	if !r.accept(100) {
		t.Fatal("far-ahead packet rejected")
	}
	if r.accept(1) {
		t.Fatal("too-old packet accepted")
	}
}

func TestHelloHMAC(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	var cn [32]byte
	_, _ = rand.Read(cn[:])
	p := makeHello(key, cn)
	if len(p) != 77 {
		t.Fatalf("hello len=%d", len(p))
	}
	h := hmac.New(sha256.New, key)
	h.Write(p[:45])
	if !hmac.Equal(h.Sum(nil), p[45:]) {
		t.Fatal("valid hello MAC rejected")
	}
	p[20] ^= 1
	h = hmac.New(sha256.New, key)
	h.Write(p[:45])
	if hmac.Equal(h.Sum(nil), p[45:]) {
		t.Fatal("tampered hello MAC accepted")
	}
}

type chanWriter struct{ ch chan []byte }

func (w chanWriter) Write(p []byte) (int, error) {
	cp := append([]byte(nil), p...)
	w.ch <- cp
	return len(p), nil
}

func TestUDPHandshakeAndData(t *testing.T) {
	serverConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	clientConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	serverCarrier := &udpCarrier{conn: serverConn}
	clientCarrier := &udpCarrier{conn: clientConn, defaultPeer: serverConn.LocalAddr().String()}
	defer serverCarrier.Close()
	defer clientCarrier.Close()

	runCarrierHandshakeDataTest(t, serverCarrier, clientCarrier)
}

func TestTCPHandshakeAndData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serverCarrier := &streamCarrier{name: "tcp-test", ctx: ctx, listener: ln, defaultID: "tcp-peer"}
	clientCarrier := &streamCarrier{name: "tcp-test", ctx: ctx, defaultID: "tcp-peer"}
	clientCarrier.dial = func(dctx context.Context) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(dctx, "tcp4", ln.Addr().String())
	}
	defer serverCarrier.Close()
	defer clientCarrier.Close()

	runCarrierHandshakeDataTest(t, serverCarrier, clientCarrier)
}

func runCarrierHandshakeDataTest(t *testing.T, serverCarrier, clientCarrier packetCarrier) {
	t.Helper()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	serverState := &tunnelState{hellos: make(map[[32]byte]helloRecord)}
	clientState := &tunnelState{hellos: make(map[[32]byte]helloRecord)}
	serverOut := make(chan []byte, 1)
	clientOut := make(chan []byte, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go recvLoop(ctx, serverCarrier, chanWriter{serverOut}, key, serverState, "kharej")
	go recvLoop(ctx, clientCarrier, chanWriter{clientOut}, key, clientState, "iran")
	hctx, hcancel := context.WithTimeout(ctx, 3*time.Second)
	defer hcancel()
	if err := handshakeClient(hctx, clientCarrier, clientCarrier.DefaultPeer(), key, clientState); err != nil {
		t.Fatal(err)
	}
	if clientState.loadSession() == nil || serverState.loadSession() == nil {
		t.Fatal("session not established")
	}
	payload := []byte{0x45, 0x00, 0x00, 0x14, 0xde, 0xad, 0xbe, 0xef}
	if err := sendEncrypted(clientCarrier, clientState.loadSession(), msgData, payload); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-serverOut:
		if !bytes.Equal(got, payload) {
			t.Fatalf("payload mismatch: %x != %x", got, payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for decrypted payload")
	}
}

func TestKCPDefaultsAndValidation(t *testing.T) {
	var k KCPConfig
	applyKCPDefaults(&k)
	if k.MTU == 0 || k.SendWindow == 0 || k.ReceiveWindow == 0 {
		t.Fatal("KCP defaults were not applied")
	}
	if err := validateKCP(k); err != nil {
		t.Fatal(err)
	}
}
