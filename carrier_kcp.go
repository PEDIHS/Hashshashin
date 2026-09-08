package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"

	kcp "github.com/xtaci/kcp-go/v5"
)

type kcpCarrier struct {
	*streamCarrier
	packetConn net.PacketConn
}

func newKCPCarrier(ctx context.Context, c *Config) (packetCarrier, error) {
	pc, err := listenMarkedPacket(ctx, "udp4", c.Transport.Listen)
	if err != nil {
		return nil, fmt.Errorf("listen KCP UDP socket %s: %w", c.Transport.Listen, err)
	}

	s := &streamCarrier{name: "kcp", ctx: ctx, defaultID: "kcp-peer"}
	s.tune = func(conn net.Conn) { tuneKCPSession(conn, c) }

	if c.Role == "iran" {
		peer, err := net.ResolveUDPAddr("udp4", c.Transport.Peer)
		if err != nil {
			_ = pc.Close()
			return nil, fmt.Errorf("resolve KCP peer: %w", err)
		}
		s.dial = func(context.Context) (net.Conn, error) {
			conv, err := randomConversationID()
			if err != nil {
				return nil, err
			}
			sess, err := kcp.NewConn3(conv, peer, nil, c.Transport.KCP.DataShards, c.Transport.KCP.ParityShards, pc)
			if err != nil {
				return nil, fmt.Errorf("create KCP session: %w", err)
			}
			return sess, nil
		}
		return &kcpCarrier{streamCarrier: s, packetConn: pc}, nil
	}

	listener, err := kcp.ServeConn(nil, c.Transport.KCP.DataShards, c.Transport.KCP.ParityShards, pc)
	if err != nil {
		_ = pc.Close()
		return nil, fmt.Errorf("serve KCP: %w", err)
	}
	s.listener = listener
	return &kcpCarrier{streamCarrier: s, packetConn: pc}, nil
}

func tuneKCPSession(conn net.Conn, c *Config) {
	s, ok := conn.(*kcp.UDPSession)
	if !ok {
		return
	}
	s.SetStreamMode(true)
	s.SetNoDelay(c.Transport.KCP.NoDelay, c.Transport.KCP.Interval, c.Transport.KCP.Resend, c.Transport.KCP.NoCongestion)
	s.SetWindowSize(c.Transport.KCP.SendWindow, c.Transport.KCP.ReceiveWindow)
	s.SetMtu(c.Transport.KCP.MTU)
	s.SetACKNoDelay(true)
	_ = s.SetReadBuffer(c.Transport.KCP.SocketBuffer)
	_ = s.SetWriteBuffer(c.Transport.KCP.SocketBuffer)
}

func randomConversationID() (uint32, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint32(b[:])
	if v == 0 {
		v = 1
	}
	return v, nil
}

func (k *kcpCarrier) Close() error {
	err1 := k.streamCarrier.Close()
	err2 := k.packetConn.Close()
	if err1 != nil {
		return err1
	}
	return err2
}
