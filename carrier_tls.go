package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
)

func newTLSCarrier(ctx context.Context, c *Config) (packetCarrier, error) {
	s := &streamCarrier{name: "tls", ctx: ctx, defaultID: "tls-peer"}
	if c.Role == "iran" {
		peer := c.Transport.Peer
		s.dial = func(dctx context.Context) (net.Conn, error) {
			raw, err := dialMarkedStream(dctx, "tcp4", peer)
			if err != nil {
				return nil, fmt.Errorf("dial TLS carrier %s: %w", peer, err)
			}
			_ = markStreamConn(raw)
			return outerTLSClient(dctx, raw)
		}
		return s, nil
	}

	cert, err := ephemeralTLSCertificate()
	if err != nil {
		return nil, fmt.Errorf("create TLS certificate: %w", err)
	}
	rawListener, err := listenMarkedStream(ctx, "tcp4", c.Transport.Listen)
	if err != nil {
		return nil, fmt.Errorf("listen TLS carrier %s: %w", c.Transport.Listen, err)
	}
	s.listener = &tlsAcceptListener{
		ctx:      ctx,
		Listener: rawListener,
		config: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			Certificates: []tls.Certificate{cert},
		},
	}
	return s, nil
}
