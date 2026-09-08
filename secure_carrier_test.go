package main

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
	"time"
)

func TestTLSCarrierHandshakeAndData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	cert, err := ephemeralTLSCertificate()
	if err != nil { t.Fatal(err) }
	server := &streamCarrier{
		name: "tls-test", ctx: ctx, defaultID: "tls-peer",
		listener: &tlsAcceptListener{ctx: ctx, Listener: ln, config: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}}},
	}
	client := &streamCarrier{name: "tls-test", ctx: ctx, defaultID: "tls-peer"}
	client.dial = func(dctx context.Context) (net.Conn, error) {
		d := net.Dialer{Timeout: 2 * time.Second}
		raw, err := d.DialContext(dctx, "tcp4", ln.Addr().String())
		if err != nil { return nil, err }
		return outerTLSClient(dctx, raw)
	}
	defer server.Close()
	defer client.Close()
	runCarrierHandshakeDataTest(t, server, client)
}

func TestWebSocketCarrierHandshakeAndData(t *testing.T) {
	server, client := testWebSocketCarrierPair(t, false)
	defer server.Close()
	defer client.Close()
	runCarrierHandshakeDataTest(t, server, client)
}

func TestWSSCarrierHandshakeAndData(t *testing.T) {
	server, client := testWebSocketCarrierPair(t, true)
	defer server.Close()
	defer client.Close()
	runCarrierHandshakeDataTest(t, server, client)
}

func testWebSocketCarrierPair(t *testing.T, secure bool) (*wsCarrier, *wsCarrier) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	name := "ws-test"
	if secure { name = "wss-test" }
	server := &wsCarrier{name: name, ctx: ctx, path: "/hsh-test", secure: secure, listener: ln}
	if secure {
		cert, err := ephemeralTLSCertificate()
		if err != nil { t.Fatal(err) }
		server.tlsConfig = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}}
	}
	client := &wsCarrier{name: name, ctx: ctx, path: "/hsh-test", secure: secure, dialAddr: ln.Addr().String()}
	client.dialRaw = func(dctx context.Context) (net.Conn, error) {
		d := net.Dialer{Timeout: 2 * time.Second}
		return d.DialContext(dctx, "tcp4", ln.Addr().String())
	}
	return server, client
}

func TestWebSocketAcceptVector(t *testing.T) {
	// RFC 6455 section 1.3 example vector.
	if got := websocketAccept("dGhlIHNhbXBsZSBub25jZQ=="); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("unexpected websocket accept: %s", got)
	}
}
