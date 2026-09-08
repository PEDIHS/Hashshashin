package main

import (
	"context"
	"crypto/rand"
	"io"
	"testing"
	"time"
)

type captureCarrier struct {
	packet []byte
	peer   string
}

func (c *captureCarrier) ReadPacket([]byte) (int, string, error) { return 0, "", io.EOF }
func (c *captureCarrier) WritePacket(p []byte, peer string) error {
	c.packet = append(c.packet[:0], p...)
	c.peer = peer
	return nil
}
func (c *captureCarrier) DefaultPeer() string { return "peer" }
func (c *captureCarrier) Name() string        { return "capture" }
func (c *captureCarrier) Close() error        { return nil }

func TestEncryptedDirectProbeAckDispatch(t *testing.T) {
	psk := make([]byte, 32)
	cn := make([]byte, 32)
	sn := make([]byte, 32)
	if _, err := rand.Read(psk); err != nil { t.Fatal(err) }
	if _, err := rand.Read(cn); err != nil { t.Fatal(err) }
	if _, err := rand.Read(sn); err != nil { t.Fatal(err) }

	iran, err := deriveSession(psk, cn, sn, "peer", true)
	if err != nil { t.Fatal(err) }
	kharej, err := deriveSession(psk, cn, sn, "peer", false)
	if err != nil { t.Fatal(err) }

	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil { t.Fatal(err) }
	carrier := &captureCarrier{}
	if err := sendEncrypted(carrier, kharej, msgDirectProbeAck, nonce[:]); err != nil { t.Fatal(err) }
	if len(carrier.packet) == 0 { t.Fatal("no encrypted probe ACK produced") }

	st := &tunnelState{hellos: make(map[[32]byte]helloRecord), session: iran}
	st.initProbeAck()
	handleEncrypted(io.Discard, "peer", carrier.packet, st)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !st.waitProbeAck(ctx, nonce, 500*time.Millisecond) {
		t.Fatal("encrypted direct-probe ACK was not dispatched to Smart Return monitor")
	}
}
