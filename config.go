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

type KCPConfig struct {
	DataShards    int `json:"data_shards"`
	ParityShards  int `json:"parity_shards"`
	NoDelay       int `json:"nodelay"`
	Interval      int `json:"interval"`
	Resend        int `json:"resend"`
	NoCongestion  int `json:"nc"`
	SendWindow    int `json:"send_window"`
	ReceiveWindow int `json:"receive_window"`
	MTU           int `json:"mtu"`
	SocketBuffer  int `json:"socket_buffer"`
}

type SmartReturnConfig struct {
	Enabled          bool `json:"enabled"`
	ProbePort        int  `json:"probe_port"`
	IntervalSeconds  int  `json:"interval_seconds"`
	TimeoutSeconds   int  `json:"timeout_seconds"`
	FailThreshold    int  `json:"fail_threshold"`
	RecoverThreshold int  `json:"recover_threshold"`
}

type Config struct {
	Role      string `json:"role"`
	Mode      string `json:"mode"`
	Transport struct {
		Type                  string    `json:"type"`
		Listen                string    `json:"listen"`
		Peer                  string    `json:"peer"`
		Key                   string    `json:"key"`
		KeepaliveSeconds      int       `json:"keepalive_seconds"`
		SessionTimeoutSeconds int       `json:"session_timeout_seconds"`
		RekeyMinutes          int       `json:"rekey_minutes"`
		KCP                   KCPConfig `json:"kcp,omitempty"`
	} `json:"transport"`
	SmartReturn SmartReturnConfig `json:"smart_return,omitempty"`
	Performance PerformanceConfig `json:"performance,omitempty"`
	Tun         struct {
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
	if c.Transport.Type == "" {
		c.Transport.Type = "udp"
	}
	if c.Transport.Type != "udp" && c.Transport.Type != "tcp" && c.Transport.Type != "kcp" {
		return nil, errors.New("transport.type must be udp, tcp or kcp")
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

	applyPerformanceDefaults(&c.Performance)
	if err := validatePerformance(c.Performance); err != nil {
		return nil, err
	}

	if c.Transport.Type == "kcp" {
		applyKCPDefaults(&c.Transport.KCP)
		if err := validateKCP(c.Transport.KCP); err != nil {
			return nil, err
		}
	}

	key, err := base64.StdEncoding.DecodeString(c.Transport.Key)
	if err != nil || len(key) != 32 {
		return nil, errors.New("transport.key must be base64 of exactly 32 bytes")
	}
	if err := validateTransportAddress(c.Transport.Type, c.Transport.Listen); err != nil {
		return nil, fmt.Errorf("invalid transport.listen: %w", err)
	}
	if c.Role == "iran" {
		if c.Transport.Peer == "" {
			return nil, errors.New("transport.peer is required on iran")
		}
		if err := validateTransportAddress(c.Transport.Type, c.Transport.Peer); err != nil {
			return nil, fmt.Errorf("invalid transport.peer: %w", err)
		}
	}
	if _, _, err := net.ParseCIDR(c.Tun.LocalCIDR); err != nil {
		return nil, fmt.Errorf("invalid tun.local_cidr: %w", err)
	}
	if ip := net.ParseIP(c.Tun.PeerIP); ip == nil || ip.To4() == nil {
		return nil, errors.New("invalid tun.peer_ip")
	}

	if c.SmartReturn.Enabled {
		if c.Mode != "direct-return" {
			return nil, errors.New("smart_return can only be enabled in direct-return mode")
		}
		if len(c.Ports) == 0 {
			return nil, errors.New("smart_return requires at least one service port")
		}
		applySmartReturnDefaults(&c.SmartReturn)
		if err := validateSmartReturn(c.SmartReturn); err != nil {
			return nil, err
		}
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
	carrierProto := transportNetwork(c.Transport.Type)
	for _, p := range c.Ports {
		if p.Port == carrierPort && protocolIncludes(p.Protocol, carrierProto) {
			return nil, fmt.Errorf("%s carrier port %d conflicts with service port %d/%s", c.Transport.Type, carrierPort, p.Port, p.Protocol)
		}
		if c.SmartReturn.Enabled && p.Port == c.SmartReturn.ProbePort && (p.Protocol == "udp" || p.Protocol == "both") {
			return nil, fmt.Errorf("smart_return probe port %d conflicts with service port %d/%s", c.SmartReturn.ProbePort, p.Port, p.Protocol)
		}
	}
	if c.SmartReturn.Enabled && carrierProto == "udp" && carrierPort == c.SmartReturn.ProbePort {
		return nil, errors.New("smart_return probe_port must differ from UDP/KCP carrier port")
	}
	return &c, nil
}

func applySmartReturnDefaults(s *SmartReturnConfig) {
	if s.ProbePort == 0 {
		s.ProbePort = 9001
	}
	if s.IntervalSeconds == 0 {
		s.IntervalSeconds = 5
	}
	if s.TimeoutSeconds == 0 {
		s.TimeoutSeconds = 2
	}
	if s.FailThreshold == 0 {
		s.FailThreshold = 3
	}
	if s.RecoverThreshold == 0 {
		s.RecoverThreshold = 3
	}
}

func validateSmartReturn(s SmartReturnConfig) error {
	if s.ProbePort < 1 || s.ProbePort > 65535 {
		return errors.New("smart_return.probe_port must be 1..65535")
	}
	if s.IntervalSeconds < 2 || s.IntervalSeconds > 300 {
		return errors.New("smart_return.interval_seconds must be 2..300")
	}
	if s.TimeoutSeconds < 1 || s.TimeoutSeconds >= s.IntervalSeconds {
		return errors.New("smart_return.timeout_seconds must be >=1 and less than interval_seconds")
	}
	if s.FailThreshold < 1 || s.FailThreshold > 20 || s.RecoverThreshold < 1 || s.RecoverThreshold > 20 {
		return errors.New("smart_return thresholds must be 1..20")
	}
	return nil
}

func applyKCPDefaults(k *KCPConfig) {
	if k.NoDelay == 0 {
		k.NoDelay = 1
	}
	if k.Interval == 0 {
		k.Interval = 20
	}
	if k.Resend == 0 {
		k.Resend = 2
	}
	if k.NoCongestion == 0 {
		k.NoCongestion = 1
	}
	if k.SendWindow == 0 {
		k.SendWindow = 512
	}
	if k.ReceiveWindow == 0 {
		k.ReceiveWindow = 512
	}
	if k.MTU == 0 {
		k.MTU = 1200
	}
	if k.SocketBuffer == 0 {
		k.SocketBuffer = 4 * 1024 * 1024
	}
}

func validateKCP(k KCPConfig) error {
	if k.DataShards < 0 || k.DataShards > 255 || k.ParityShards < 0 || k.ParityShards > 255 {
		return errors.New("kcp data_shards/parity_shards must be between 0 and 255")
	}
	if (k.DataShards == 0) != (k.ParityShards == 0) {
		return errors.New("kcp FEC requires both data_shards and parity_shards, or both zero")
	}
	if k.NoDelay < 0 || k.NoDelay > 1 || k.NoCongestion < 0 || k.NoCongestion > 1 {
		return errors.New("kcp nodelay and nc must be 0 or 1")
	}
	if k.Interval < 10 || k.Interval > 100 || k.Resend < 0 || k.Resend > 2 {
		return errors.New("kcp interval must be 10..100 and resend 0..2")
	}
	if k.SendWindow < 32 || k.SendWindow > 8192 || k.ReceiveWindow < 32 || k.ReceiveWindow > 8192 {
		return errors.New("kcp windows must be between 32 and 8192")
	}
	if k.MTU < 576 || k.MTU > 1400 {
		return errors.New("kcp mtu must be between 576 and 1400")
	}
	if k.SocketBuffer < 256*1024 || k.SocketBuffer > 64*1024*1024 {
		return errors.New("kcp socket_buffer must be between 256KiB and 64MiB")
	}
	return nil
}

func validateTransportAddress(kind, addr string) error {
	switch kind {
	case "tcp":
		_, err := net.ResolveTCPAddr("tcp4", addr)
		return err
	case "udp", "kcp":
		_, err := net.ResolveUDPAddr("udp4", addr)
		return err
	default:
		return fmt.Errorf("unsupported transport %q", kind)
	}
}

func protocolIncludes(serviceProto, carrierProto string) bool {
	return serviceProto == "both" || serviceProto == carrierProto
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
