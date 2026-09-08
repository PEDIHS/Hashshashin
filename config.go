package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
)

type Port struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type Config struct {
	Role      string `json:"role"`
	Mode      string `json:"mode"`
	Transport struct {
		Listen                string `json:"listen"`
		Peer                  string `json:"peer"`
		Key                   string `json:"key"`
		KeepaliveSeconds      int    `json:"keepalive_seconds"`
		SessionTimeoutSeconds int    `json:"session_timeout_seconds"`
		RekeyMinutes          int    `json:"rekey_minutes"`
	} `json:"transport"`
	Tun struct {
		Name      string `json:"name"`
		LocalCIDR string `json:"local_cidr"`
		PeerIP    string `json:"peer_ip"`
		MTU       int    `json:"mtu"`
	} `json:"tun"`
	Network struct {
		PublicInterface  string `json:"public_interface"`
		PublicIP         string `json:"public_ip"`
		PublicGateway    string `json:"public_gateway"`
		ForeignPublicIP  string `json:"foreign_public_ip"`
		IranPublicIP     string `json:"iran_public_ip"`
		LockServicePorts bool   `json:"lock_service_ports"`
	} `json:"network"`
	Ports []Port `json:"ports"`
}

func loadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var c Config
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if c.Role != "iran" && c.Role != "kharej" {
		return nil, errors.New("role must be iran or kharej")
	}
	if c.Mode != "full" && c.Mode != "direct-return" {
		return nil, errors.New("mode must be full or direct-return")
	}
	if c.Tun.Name == "" {
		c.Tun.Name = "hsh0"
	}
	if len(c.Tun.Name) > 15 {
		return nil, errors.New("tun.name must be at most 15 characters")
	}
	if c.Tun.MTU == 0 {
		c.Tun.MTU = 1320
	}
	if c.Tun.MTU < 900 || c.Tun.MTU > 1400 {
		return nil, errors.New("tun.mtu must be between 900 and 1400")
	}
	if c.Transport.KeepaliveSeconds == 0 {
		c.Transport.KeepaliveSeconds = 5
	}
	if c.Transport.SessionTimeoutSeconds == 0 {
		c.Transport.SessionTimeoutSeconds = 25
	}
	if c.Transport.RekeyMinutes == 0 {
		c.Transport.RekeyMinutes = 30
	}
	if c.Transport.KeepaliveSeconds < 2 || c.Transport.KeepaliveSeconds > 60 {
		return nil, errors.New("keepalive_seconds must be between 2 and 60")
	}
	if c.Transport.SessionTimeoutSeconds < c.Transport.KeepaliveSeconds*2 {
		return nil, errors.New("session_timeout_seconds must be at least 2x keepalive_seconds")
	}
	if c.Transport.RekeyMinutes < 5 || c.Transport.RekeyMinutes > 1440 {
		return nil, errors.New("rekey_minutes must be between 5 and 1440")
	}

	key, err := base64.StdEncoding.DecodeString(c.Transport.Key)
	if err != nil || len(key) != 32 {
		return nil, errors.New("transport.key must be base64 of exactly 32 bytes")
	}
	if _, err := net.ResolveUDPAddr("udp4", c.Transport.Listen); err != nil {
		return nil, fmt.Errorf("invalid transport.listen: %w", err)
	}
	if c.Role == "iran" {
		if c.Transport.Peer == "" {
			return nil, errors.New("transport.peer is required on iran")
		}
		if _, err := net.ResolveUDPAddr("udp4", c.Transport.Peer); err != nil {
			return nil, fmt.Errorf("invalid transport.peer: %w", err)
		}
	}
	if _, _, err := net.ParseCIDR(c.Tun.LocalCIDR); err != nil {
		return nil, fmt.Errorf("invalid tun.local_cidr: %w", err)
	}
	if ip := net.ParseIP(c.Tun.PeerIP); ip == nil || ip.To4() == nil {
		return nil, errors.New("invalid tun.peer_ip")
	}

	if len(c.Ports) == 0 {
		return &c, nil
	}
	if c.Network.PublicInterface == "" || c.Network.PublicIP == "" || c.Network.ForeignPublicIP == "" || c.Network.IranPublicIP == "" {
		return nil, errors.New("network public_interface/public_ip/foreign_public_ip/iran_public_ip are required when ports are configured")
	}
	for _, ip := range []string{c.Network.PublicIP, c.Network.ForeignPublicIP, c.Network.IranPublicIP} {
		p := net.ParseIP(ip)
		if p == nil || p.To4() == nil {
			return nil, fmt.Errorf("invalid IPv4 address: %s", ip)
		}
	}

	seen := map[string]bool{}
	for _, p := range c.Ports {
		if p.Port < 1 || p.Port > 65535 {
			return nil, fmt.Errorf("invalid service port %d", p.Port)
		}
		if p.Protocol != "tcp" && p.Protocol != "udp" && p.Protocol != "both" {
			return nil, fmt.Errorf("invalid protocol %q for port %d", p.Protocol, p.Port)
		}
		k := fmt.Sprintf("%d/%s", p.Port, p.Protocol)
		if seen[k] {
			return nil, fmt.Errorf("duplicate port entry %s", k)
		}
		seen[k] = true
	}
	carrierPort, err := listenPort(c.Transport.Listen)
	if err != nil {
		return nil, err
	}
	for _, p := range c.Ports {
		if p.Port == carrierPort && (p.Protocol == "udp" || p.Protocol == "both") {
			return nil, fmt.Errorf("UDP carrier port %d conflicts with service port %d/%s", carrierPort, p.Port, p.Protocol)
		}
	}
	return &c, nil
}

func listenPort(addr string) (int, error) {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, fmt.Errorf("invalid listen address %q: %w", addr, err)
	}
	n, err := strconv.Atoi(p)
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("invalid listen port in %q", addr)
	}
	return n, nil
}
