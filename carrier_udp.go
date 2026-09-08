package main

import (
	"context"
	"fmt"
	"net"
)

type udpCarrier struct {
	conn        *net.UDPConn
	defaultPeer string
}

func newUDPCarrier(ctx context.Context, c *Config) (packetCarrier, error) {
	pc, err := listenMarkedPacket(ctx, "udp4", c.Transport.Listen)
	if err != nil {
		return nil, fmt.Errorf("listen UDP %s: %w", c.Transport.Listen, err)
	}
	u, ok := pc.(*net.UDPConn)
	if !ok {
		_ = pc.Close()
		return nil, fmt.Errorf("unexpected UDP packet connection type")
	}
	uc := &udpCarrier{conn: u}
	if c.Role == "iran" {
		peer, err := net.ResolveUDPAddr("udp4", c.Transport.Peer)
		if err != nil {
			_ = u.Close()
			return nil, fmt.Errorf("resolve UDP peer: %w", err)
		}
		uc.defaultPeer = peer.String()
	}
	return uc, nil
}

func (u *udpCarrier) Name() string        { return "udp" }
func (u *udpCarrier) DefaultPeer() string { return u.defaultPeer }

func (u *udpCarrier) ReadPacket(buf []byte) (int, string, error) {
	n, peer, err := u.conn.ReadFromUDP(buf)
	if err != nil {
		return 0, "", err
	}
	return n, peer.String(), nil
}

func (u *udpCarrier) WritePacket(p []byte, peer string) error {
	if peer == "" {
		peer = u.defaultPeer
	}
	if peer == "" {
		return fmt.Errorf("UDP peer is not known yet")
	}
	addr, err := net.ResolveUDPAddr("udp4", peer)
	if err != nil {
		return err
	}
	_, err = u.conn.WriteToUDP(p, addr)
	return err
}

func (u *udpCarrier) Close() error { return u.conn.Close() }
