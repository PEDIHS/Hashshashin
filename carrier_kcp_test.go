package main

import (
	"context"
	"net"
	"testing"

	kcp "github.com/xtaci/kcp-go/v5"
)

func TestKCPHandshakeAndDataWithFEC(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverPC, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer serverPC.Close()
	clientPC, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer clientPC.Close()

	const dataShards, parityShards = 10, 3
	listener, err := kcp.ServeConn(nil, dataShards, parityShards, serverPC)
	if err != nil {
		t.Fatal(err)
	}

	tune := func(conn net.Conn) {
		if s, ok := conn.(*kcp.UDPSession); ok {
			s.SetStreamMode(true)
			s.SetNoDelay(1, 20, 2, 1)
			s.SetWindowSize(128, 128)
			s.SetMtu(1200)
			s.SetACKNoDelay(true)
		}
	}

	serverCarrier := &streamCarrier{
		name:      "kcp-test",
		ctx:       ctx,
		listener:  listener,
		defaultID: "kcp-peer",
		tune:      tune,
	}
	clientCarrier := &streamCarrier{
		name:      "kcp-test",
		ctx:       ctx,
		defaultID: "kcp-peer",
		tune:      tune,
	}
	peer := serverPC.LocalAddr()
	clientCarrier.dial = func(context.Context) (net.Conn, error) {
		conv, err := randomConversationID()
		if err != nil {
			return nil, err
		}
		return kcp.NewConn3(conv, peer, nil, dataShards, parityShards, clientPC)
	}
	defer serverCarrier.Close()
	defer clientCarrier.Close()

	runCarrierHandshakeDataTest(t, serverCarrier, clientCarrier)
}
