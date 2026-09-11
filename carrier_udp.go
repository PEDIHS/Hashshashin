package main

import (
	"context"
	"fmt"
	"net"
	"sync"
)

type udpCarrier struct {
	conn        *net.UDPConn
	defaultPeer string
	defaultAddr *net.UDPAddr
	peers       sync.Map // map[string]*net.UDPAddr
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
	_ = u.SetReadBuffer(c.Performance.SocketBuffer)
	_ = u.SetWriteBuffer(c.Performance.SocketBuffer)

	uc := &udpCarrier{conn: u}
	if c.Role == "iran" {
		peer, err := net.ResolveUDPAddr("udp4", c.Transport.Peer)
		if err != nil {
			_ = u.Close()
			return nil, fmt.Errorf("resolve UDP peer: %w", err)
		}
		uc.defaultPeer = peer.String()
		uc.defaultAddr = peer
		uc.peers.Store(uc.defaultPeer, peer)
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
	peerID := peer.String()
	u.peers.Store(peerID, peer)
	return n, peerID, nil
}

func (u *udpCarrier) WritePacket(p []byte, peer string) error {
	if peer == "" {
		peer = u.defaultPeer
	}
	if peer == "" {
		return fmt.Errorf("UDP peer is not known yet")
	}
	var addr *net.UDPAddr
	if peer == u.defaultPeer && u.defaultAddr != nil {
		addr = u.defaultAddr
	} else if cached, ok := u.peers.Load(peer); ok {
		addr = cached.(*net.UDPAddr)
	} else {
		var err error
		addr, err = net.ResolveUDPAddr("udp4", peer)
		if err != nil {
			return err
		}
		u.peers.Store(peer, addr)
	}
	_, err := u.conn.WriteToUDP(p, addr)
	return err
}

func (u *udpCarrier) Close() error { return u.conn.Close() }
