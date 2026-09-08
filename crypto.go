package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type replayWindow struct {
	mu      sync.Mutex
	highest uint64
	bitmap  uint64
}

func (r *replayWindow) accept(counter uint64) bool {
	if counter == 0 {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.highest == 0 {
		r.highest = counter
		r.bitmap = 1
		return true
	}
	if counter > r.highest {
		shift := counter - r.highest
		if shift >= 64 {
			r.bitmap = 1
		} else {
			r.bitmap = (r.bitmap << shift) | 1
		}
		r.highest = counter
		return true
	}
	delta := r.highest - counter
	if delta >= 64 {
		return false
	}
	bit := uint64(1) << delta
	if r.bitmap&bit != 0 {
		return false
	}
	r.bitmap |= bit
	return true
}

type session struct {
	txAEAD   cipher.AEAD
	rxAEAD   cipher.AEAD
	peer     *net.UDPAddr
	created  time.Time
	tx       uint64
	replay   replayWindow
	lastSeen int64
	txBytes  uint64
	rxBytes  uint64
}

func (s *session) touch()             { atomic.StoreInt64(&s.lastSeen, time.Now().UnixNano()) }
func (s *session) age() time.Duration { return time.Since(s.created) }
func (s *session) idle() time.Duration {
	v := atomic.LoadInt64(&s.lastSeen)
	if v == 0 {
		return time.Hour
	}
	return time.Since(time.Unix(0, v))
}

func deriveSession(psk, cn, sn []byte, peer *net.UDPAddr, initiator bool) (*session, error) {
	derive := func(label string) []byte {
		h := hmac.New(sha256.New, psk)
		h.Write([]byte("hashshashin/v1/" + label))
		h.Write(cn)
		h.Write(sn)
		return h.Sum(nil)
	}
	makeAEAD := func(key []byte) (cipher.AEAD, error) {
		block, err := aes.NewCipher(key[:32])
		if err != nil {
			return nil, err
		}
		return cipher.NewGCM(block)
	}
	c2s, err := makeAEAD(derive("c2s"))
	if err != nil {
		return nil, err
	}
	s2c, err := makeAEAD(derive("s2c"))
	if err != nil {
		return nil, err
	}

	s := &session{
		peer:    &net.UDPAddr{IP: append(net.IP(nil), peer.IP...), Port: peer.Port, Zone: peer.Zone},
		created: time.Now(),
	}
	if initiator {
		s.txAEAD, s.rxAEAD = c2s, s2c
	} else {
		s.txAEAD, s.rxAEAD = s2c, c2s
	}
	s.touch()
	return s, nil
}

func makeNonce(counter uint64) [12]byte {
	var nonce [12]byte
	binary.BigEndian.PutUint64(nonce[4:], counter)
	return nonce
}
