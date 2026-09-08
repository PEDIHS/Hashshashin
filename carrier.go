package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

const maxCarrierFrame = 1 << 20

type packetCarrier interface {
	ReadPacket([]byte) (int, string, error)
	WritePacket([]byte, string) error
	DefaultPeer() string
	Name() string
	Close() error
}

func newCarrier(ctx context.Context, c *Config) (packetCarrier, error) {
	switch c.Transport.Type {
	case "udp":
		return newUDPCarrier(ctx, c)
	case "tcp":
		return newTCPCarrier(ctx, c)
	case "tls":
		return newTLSCarrier(ctx, c)
	case "kcp":
		return newKCPCarrier(ctx, c)
	case "ws":
		return newWebSocketCarrier(ctx, c, false)
	case "wss":
		return newWebSocketCarrier(ctx, c, true)
	default:
		return nil, fmt.Errorf("unsupported transport type %q", c.Transport.Type)
	}
}

func transportNetwork(t string) string {
	switch t {
	case "tcp", "tls", "ws", "wss":
		return "tcp"
	default:
		return "udp"
	}
}

func socketMarkControl(network, address string, rc syscall.RawConn) error {
	var setErr error
	if err := rc.Control(func(fd uintptr) {
		if e := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, 36, transportMark); e != nil {
			setErr = e
		}
	}); err != nil {
		return err
	}
	return setErr
}

func markStreamConn(conn net.Conn) error {
	sc, ok := conn.(syscall.Conn)
	if !ok {
		return fmt.Errorf("connection does not expose SyscallConn")
	}
	rc, err := sc.SyscallConn()
	if err != nil {
		return err
	}
	return socketMarkControl("", "", rc)
}

func listenMarkedPacket(ctx context.Context, network, address string) (net.PacketConn, error) {
	lc := net.ListenConfig{Control: socketMarkControl}
	return lc.ListenPacket(ctx, network, address)
}

func listenMarkedStream(ctx context.Context, network, address string) (net.Listener, error) {
	lc := net.ListenConfig{Control: socketMarkControl}
	return lc.Listen(ctx, network, address)
}

func dialMarkedStream(ctx context.Context, network, address string) (net.Conn, error) {
	d := net.Dialer{Timeout: 6 * time.Second, Control: socketMarkControl}
	return d.DialContext(ctx, network, address)
}

type streamCarrier struct {
	name      string
	ctx       context.Context
	listener  net.Listener
	dial      func(context.Context) (net.Conn, error)
	tune      func(net.Conn)
	connectMu sync.Mutex
	connMu    sync.Mutex
	conn      net.Conn
	readMu    sync.Mutex
	writeMu   sync.Mutex
	closed    bool
	defaultID string
}

func (s *streamCarrier) Name() string         { return s.name }
func (s *streamCarrier) DefaultPeer() string { return s.defaultID }

func (s *streamCarrier) ensureConn() (net.Conn, error) {
	s.connMu.Lock()
	if s.closed {
		s.connMu.Unlock()
		return nil, io.ErrClosedPipe
	}
	if s.conn != nil {
		conn := s.conn
		s.connMu.Unlock()
		return conn, nil
	}
	s.connMu.Unlock()

	s.connectMu.Lock()
	defer s.connectMu.Unlock()

	s.connMu.Lock()
	if s.closed {
		s.connMu.Unlock()
		return nil, io.ErrClosedPipe
	}
	if s.conn != nil {
		conn := s.conn
		s.connMu.Unlock()
		return conn, nil
	}
	s.connMu.Unlock()

	var (
		conn net.Conn
		err  error
	)
	if s.listener != nil {
		conn, err = s.listener.Accept()
	} else if s.dial != nil {
		conn, err = s.dial(s.ctx)
	} else {
		return nil, errors.New("stream carrier has neither listener nor dialer")
	}
	if err != nil {
		return nil, err
	}
	if s.tune != nil {
		s.tune(conn)
	}

	s.connMu.Lock()
	defer s.connMu.Unlock()
	if s.closed {
		_ = conn.Close()
		return nil, io.ErrClosedPipe
	}
	s.conn = conn
	return conn, nil
}

func (s *streamCarrier) resetConn(conn net.Conn) {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	if s.conn == conn {
		_ = s.conn.Close()
		s.conn = nil
	}
}

func (s *streamCarrier) ReadPacket(buf []byte) (int, string, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	for {
		conn, err := s.ensureConn()
		if err != nil {
			if s.ctx.Err() != nil {
				return 0, "", s.ctx.Err()
			}
			time.Sleep(300 * time.Millisecond)
			continue
		}
		var hdr [4]byte
		if _, err := io.ReadFull(conn, hdr[:]); err != nil {
			s.resetConn(conn)
			if s.ctx.Err() != nil {
				return 0, "", s.ctx.Err()
			}
			continue
		}
		n := int(binary.BigEndian.Uint32(hdr[:]))
		if n <= 0 || n > maxCarrierFrame || n > len(buf) {
			s.resetConn(conn)
			return 0, "", fmt.Errorf("%s frame length %d is invalid", s.name, n)
		}
		if _, err := io.ReadFull(conn, buf[:n]); err != nil {
			s.resetConn(conn)
			continue
		}
		return n, s.defaultID, nil
	}
}

func (s *streamCarrier) WritePacket(p []byte, peer string) error {
	if len(p) == 0 || len(p) > maxCarrierFrame {
		return fmt.Errorf("%s frame length %d is invalid", s.name, len(p))
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	for attempts := 0; attempts < 2; attempts++ {
		conn, err := s.ensureConn()
		if err != nil {
			return err
		}
		var hdr [4]byte
		binary.BigEndian.PutUint32(hdr[:], uint32(len(p)))
		if err := writeAll(conn, hdr[:]); err != nil {
			s.resetConn(conn)
			continue
		}
		if err := writeAll(conn, p); err != nil {
			s.resetConn(conn)
			continue
		}
		return nil
	}
	return fmt.Errorf("%s carrier write failed after reconnect", s.name)
}

func writeAll(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := w.Write(p)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrUnexpectedEOF
		}
		p = p[n:]
	}
	return nil
}

func (s *streamCarrier) Close() error {
	s.connMu.Lock()
	s.closed = true
	var err error
	if s.conn != nil {
		err = s.conn.Close()
		s.conn = nil
	}
	if s.listener != nil {
		if e := s.listener.Close(); err == nil {
			err = e
		}
	}
	s.connMu.Unlock()
	return err
}
