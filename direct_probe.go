package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"
)

const (
	directProbeMagic = "HDP1"
	directProbeSize  = 4 + 8 + 16 + 32
)

func makeDirectProbe(key []byte) ([]byte, [16]byte, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, nonce, err
	}
	p := make([]byte, 4+8+16)
	copy(p[:4], directProbeMagic)
	binary.BigEndian.PutUint64(p[4:12], uint64(time.Now().Unix()))
	copy(p[12:28], nonce[:])
	h := hmac.New(sha256.New, key)
	h.Write(p)
	return append(p, h.Sum(nil)...), nonce, nil
}

func verifyDirectProbe(key, p []byte) ([16]byte, bool) {
	var nonce [16]byte
	if len(p) != directProbeSize || string(p[:4]) != directProbeMagic {
		return nonce, false
	}
	h := hmac.New(sha256.New, key)
	h.Write(p[:28])
	if !hmac.Equal(h.Sum(nil), p[28:]) {
		return nonce, false
	}
	ts := int64(binary.BigEndian.Uint64(p[4:12]))
	skew := time.Since(time.Unix(ts, 0))
	if skew > 90*time.Second || skew < -90*time.Second {
		return nonce, false
	}
	copy(nonce[:], p[12:28])
	return nonce, true
}

// runDirectProbeResponder listens on Iran's public interface. A valid packet has
// travelled from Kharej to Iran over the ordinary Internet path. The ACK is sent
// back inside the authenticated HSH1 session, so the health decision specifically
// tests the Kharej -> Iran direct direction instead of requiring a direct reverse
// path as well.
func runDirectProbeResponder(ctx context.Context, c *Config, key []byte, carrier packetCarrier, st *tunnelState) {
	if !c.SmartReturn.Enabled || c.Mode != "direct-return" || c.Role != "iran" {
		return
	}
	addr := fmt.Sprintf("0.0.0.0:%d", c.SmartReturn.ProbePort)
	pc, err := listenMarkedPacket(ctx, "udp4", addr)
	if err != nil {
		log.Printf("smart-return probe responder: %v", err)
		return
	}
	defer pc.Close()
	log.Printf("smart-return probe responder listening udp/%d", c.SmartReturn.ProbePort)

	buf := make([]byte, 256)
	for {
		n, peer, err := pc.ReadFrom(buf)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("smart-return probe read: %v", err)
			}
			return
		}
		udpPeer, ok := peer.(*net.UDPAddr)
		if !ok || !udpPeer.IP.Equal(net.ParseIP(c.Network.ForeignPublicIP)) {
			continue
		}
		nonce, ok := verifyDirectProbe(key, buf[:n])
		if !ok {
			continue
		}
		ss := st.loadSession()
		if ss == nil {
			continue
		}
		if err := sendEncrypted(carrier, ss, msgDirectProbeAck, nonce[:]); err != nil && ctx.Err() == nil {
			log.Printf("smart-return probe ack: %v", err)
		}
	}
}

func runDirectProbeMonitor(ctx context.Context, c *Config, key []byte, st *tunnelState) {
	if !c.SmartReturn.Enabled || c.Mode != "direct-return" || c.Role != "kharej" {
		return
	}

	bind := net.JoinHostPort(c.Network.PublicIP, "0")
	pc, err := listenMarkedPacket(ctx, "udp4", bind)
	if err != nil {
		log.Printf("smart-return probe socket: %v", err)
		return
	}
	defer pc.Close()
	target := &net.UDPAddr{IP: net.ParseIP(c.Network.IranPublicIP), Port: c.SmartReturn.ProbePort}
	controller := newReturnPathController(c.SmartReturn.FailThreshold, c.SmartReturn.RecoverThreshold)

	if err := setKharejReturnPath(c, returnPathDirect); err != nil {
		log.Printf("smart-return initialize direct path: %v", err)
	}
	log.Printf("smart-return monitor active probe=%s interval=%ds fail=%d recover=%d", target, c.SmartReturn.IntervalSeconds, c.SmartReturn.FailThreshold, c.SmartReturn.RecoverThreshold)

	ticker := time.NewTicker(time.Duration(c.SmartReturn.IntervalSeconds) * time.Second)
	defer ticker.Stop()

	probe := func() {
		if st.loadSession() == nil {
			return
		}
		packet, nonce, err := makeDirectProbe(key)
		if err != nil {
			return
		}
		_, err = pc.WriteTo(packet, target)
		ok := false
		if err == nil {
			ok = st.waitProbeAck(ctx, nonce, time.Duration(c.SmartReturn.TimeoutSeconds)*time.Second)
		}
		path, changed := controller.observe(ok)
		if !changed {
			return
		}
		if err := setKharejReturnPath(c, path); err != nil {
			log.Printf("smart-return switch to %s failed: %v", path, err)
			return
		}
		if path == returnPathTunnel {
			log.Printf("smart-return: direct path unhealthy; DOWNLOAD fallback -> TUNNEL")
		} else {
			log.Printf("smart-return: direct path recovered; DOWNLOAD -> DIRECT")
		}
	}

	probe()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			probe()
		}
	}
}
