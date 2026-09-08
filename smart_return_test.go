package main

import (
	"crypto/rand"
	"testing"
)

func TestDirectProbeAuthentication(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	p, nonce, err := makeDirectProbe(key)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := verifyDirectProbe(key, p)
	if !ok || got != nonce {
		t.Fatal("valid direct probe was rejected")
	}
	p[14] ^= 0x40
	if _, ok := verifyDirectProbe(key, p); ok {
		t.Fatal("tampered direct probe was accepted")
	}
}

func TestReturnPathControllerHysteresis(t *testing.T) {
	c := newReturnPathController(3, 2)
	if p, changed := c.observe(false); p != returnPathDirect || changed {
		t.Fatal("switched after only one failure")
	}
	if p, changed := c.observe(false); p != returnPathDirect || changed {
		t.Fatal("switched after only two failures")
	}
	if p, changed := c.observe(true); p != returnPathDirect || changed {
		t.Fatal("success should reset direct failure streak")
	}
	c.observe(false)
	c.observe(false)
	if p, changed := c.observe(false); p != returnPathTunnel || !changed {
		t.Fatal("did not switch to tunnel after failure threshold")
	}
	if p, changed := c.observe(true); p != returnPathTunnel || changed {
		t.Fatal("recovered too early")
	}
	if p, changed := c.observe(false); p != returnPathTunnel || changed {
		t.Fatal("failure should reset recovery streak")
	}
	c.observe(true)
	if p, changed := c.observe(true); p != returnPathDirect || !changed {
		t.Fatal("did not return to direct after recovery threshold")
	}
}

func TestSmartReturnValidation(t *testing.T) {
	s := SmartReturnConfig{Enabled: true}
	applySmartReturnDefaults(&s)
	if err := validateSmartReturn(s); err != nil {
		t.Fatal(err)
	}
	if s.ProbePort == 0 || s.FailThreshold == 0 || s.RecoverThreshold == 0 {
		t.Fatal("smart return defaults were not applied")
	}
}
