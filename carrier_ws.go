package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type wsCarrier struct {
	name      string
	ctx       context.Context
	path      string
	secure    bool
	listener  net.Listener
	tlsConfig *tls.Config
	dialAddr  string
	dialRaw   func(context.Context) (net.Conn, error)
	connectMu sync.Mutex
	peerMu    sync.Mutex
	peer      *wsPeer
	readMu    sync.Mutex
	writeMu   sync.Mutex
	closed    bool
}

func newWebSocketCarrier(ctx context.Context, c *Config, secure bool) (packetCarrier, error) {
	name := "ws"
	if secure { name = "wss" }
	w := &wsCarrier{name: name, ctx: ctx, path: c.Transport.WebSocketPath, secure: secure}
	if c.Role == "iran" {
		w.dialAddr = c.Transport.Peer
		w.dialRaw = func(dctx context.Context) (net.Conn, error) {
			return dialMarkedStream(dctx, "tcp4", c.Transport.Peer)
		}
		return w, nil
	}

	ln, err := listenMarkedStream(ctx, "tcp4", c.Transport.Listen)
	if err != nil {
		return nil, fmt.Errorf("listen %s carrier %s: %w", name, c.Transport.Listen, err)
	}
	w.listener = ln
	if secure {
		cert, err := ephemeralTLSCertificate()
		if err != nil {
			_ = ln.Close()
			return nil, fmt.Errorf("create WSS certificate: %w", err)
		}
		w.tlsConfig = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}}
	}
	return w, nil
}

func (w *wsCarrier) Name() string { return w.name }
func (w *wsCarrier) DefaultPeer() string { return w.name + "-peer" }

func (w *wsCarrier) ensurePeer() (*wsPeer, error) {
	w.peerMu.Lock()
	if w.closed { w.peerMu.Unlock(); return nil, io.ErrClosedPipe }
	if w.peer != nil { p := w.peer; w.peerMu.Unlock(); return p, nil }
	w.peerMu.Unlock()

	w.connectMu.Lock()
	defer w.connectMu.Unlock()
	w.peerMu.Lock()
	if w.closed { w.peerMu.Unlock(); return nil, io.ErrClosedPipe }
	if w.peer != nil { p := w.peer; w.peerMu.Unlock(); return p, nil }
	w.peerMu.Unlock()

	var p *wsPeer
	var err error
	if w.listener != nil { p, err = w.acceptPeer() } else { p, err = w.dialPeer() }
	if err != nil { return nil, err }
	w.peerMu.Lock()
	defer w.peerMu.Unlock()
	if w.closed { _ = p.Close(); return nil, io.ErrClosedPipe }
	w.peer = p
	return p, nil
}

func (w *wsCarrier) acceptPeer() (*wsPeer, error) {
	raw, err := w.listener.Accept()
	if err != nil { return nil, err }
	_ = markStreamConn(raw)
	conn := raw
	if w.secure {
		conn, err = outerTLSServer(w.ctx, raw, w.tlsConfig)
		if err != nil { return nil, err }
	}
	p, err := wsServerHandshake(conn, w.path)
	if err != nil { _ = conn.Close(); return nil, err }
	return p, nil
}

func (w *wsCarrier) dialPeer() (*wsPeer, error) {
	if w.dialAddr == "" { return nil, fmt.Errorf("%s peer is not configured", w.name) }
	dctx, cancel := context.WithTimeout(w.ctx, 8*time.Second)
	defer cancel()
	var (
		raw net.Conn
		err error
	)
	if w.dialRaw != nil { raw, err = w.dialRaw(dctx) } else { raw, err = dialMarkedStream(dctx, "tcp4", w.dialAddr) }
	if err != nil { return nil, fmt.Errorf("dial %s carrier %s: %w", w.name, w.dialAddr, err) }
	_ = markStreamConn(raw)
	conn := raw
	if w.secure {
		conn, err = outerTLSClient(dctx, raw)
		if err != nil { return nil, err }
	}
	p, err := wsClientHandshake(dctx, conn, w.path, w.dialAddr)
	if err != nil { _ = conn.Close(); return nil, err }
	return p, nil
}

func (w *wsCarrier) resetPeer(p *wsPeer) {
	w.peerMu.Lock()
	defer w.peerMu.Unlock()
	if w.peer == p { _ = w.peer.Close(); w.peer = nil }
}

func (w *wsCarrier) ReadPacket(buf []byte) (int, string, error) {
	w.readMu.Lock()
	defer w.readMu.Unlock()
	for {
		p, err := w.ensurePeer()
		if err != nil {
			if w.ctx.Err() != nil { return 0, "", w.ctx.Err() }
			time.Sleep(300 * time.Millisecond)
			continue
		}
		n, err := p.readBinary(buf)
		if err != nil {
			w.resetPeer(p)
			if w.ctx.Err() != nil { return 0, "", w.ctx.Err() }
			continue
		}
		return n, w.DefaultPeer(), nil
	}
}

func (w *wsCarrier) WritePacket(packet []byte, peer string) error {
	if len(packet) == 0 || len(packet) > maxCarrierFrame { return fmt.Errorf("%s packet length %d is invalid", w.name, len(packet)) }
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	for attempts := 0; attempts < 2; attempts++ {
		p, err := w.ensurePeer()
		if err != nil { return err }
		if err := p.writeBinary(packet); err != nil { w.resetPeer(p); continue }
		return nil
	}
	return fmt.Errorf("%s carrier write failed after reconnect", w.name)
}

func (w *wsCarrier) Close() error {
	w.peerMu.Lock()
	w.closed = true
	var err error
	if w.peer != nil { err = w.peer.Close(); w.peer = nil }
	if w.listener != nil { if e := w.listener.Close(); err == nil { err = e } }
	w.peerMu.Unlock()
	return err
}
