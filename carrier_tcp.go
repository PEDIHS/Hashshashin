package main

import (
	"context"
	"fmt"
	"net"
)

func newTCPCarrier(ctx context.Context, c *Config) (packetCarrier, error) {
	s := &streamCarrier{name: "tcp", ctx: ctx, defaultID: "tcp-peer"}
	if c.Role == "iran" {
		peer := c.Transport.Peer
		s.dial = func(dctx context.Context) (net.Conn, error) {
			conn, err := dialMarkedStream(dctx, "tcp4", peer)
			if err != nil {
				return nil, fmt.Errorf("dial TCP carrier %s: %w", peer, err)
			}
			_ = markStreamConn(conn)
			if tc, ok := conn.(*net.TCPConn); ok {
				_ = tc.SetNoDelay(true)
				_ = tc.SetKeepAlive(true)
			}
			return conn, nil
		}
		return s, nil
	}

	ln, err := listenMarkedStream(ctx, "tcp4", c.Transport.Listen)
	if err != nil {
		return nil, fmt.Errorf("listen TCP carrier %s: %w", c.Transport.Listen, err)
	}
	s.listener = ln
	s.tune = func(conn net.Conn) {
		_ = markStreamConn(conn)
		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.SetNoDelay(true)
			_ = tc.SetKeepAlive(true)
		}
	}
	return s, nil
}
