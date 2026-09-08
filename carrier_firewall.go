package main

import (
	"fmt"
	"os/exec"
	"strconv"
)

// setupCarrierFirewall aligns INPUT rules with the selected carrier and role.
// UDP/KCP use the configured local UDP port on both nodes. TCP-underlay
// carriers (TCP/TLS/WS/WSS) are asymmetric at the socket layer: Iran dials and
// Kharej listens. Iran therefore accepts only ESTABLISHED replies instead of
// exposing the carrier destination port locally.
func setupCarrierFirewall(c *Config) error {
	if c.Network.PublicInterface == "" || c.Network.PublicIP == "" {
		return nil
	}
	sourceIP := c.Network.IranPublicIP
	if c.Role == "iran" {
		sourceIP = c.Network.ForeignPublicIP
	}
	if sourceIP == "" {
		return nil
	}
	port, err := listenPort(c.Transport.Listen)
	if err != nil {
		return err
	}

	runQuiet("iptables", "-t", "filter", "-N", "HSH_INPUT")
	if exec.Command("iptables", "-t", "filter", "-C", "INPUT", "-j", "HSH_INPUT").Run() != nil {
		if err := ipt("filter", "-I", "INPUT", "1", "-j", "HSH_INPUT"); err != nil {
			return err
		}
	}

	selected := transportNetwork(c.Transport.Type)
	var args []string
	if selected == "tcp" && c.Role == "iran" {
		args = []string{
			"-i", c.Network.PublicInterface,
			"-d", c.Network.PublicIP,
			"-s", sourceIP,
			"-p", "tcp",
			"--sport", strconv.Itoa(port),
			"-m", "conntrack", "--ctstate", "ESTABLISHED",
			"-j", "ACCEPT",
		}
	} else {
		args = []string{
			"-i", c.Network.PublicInterface,
			"-d", c.Network.PublicIP,
			"-s", sourceIP,
			"-p", selected,
			"--dport", strconv.Itoa(port),
			"-j", "ACCEPT",
		}
	}

	if exec.Command("iptables", append([]string{"-t", "filter", "-C", "HSH_INPUT"}, args...)...).Run() == nil {
		return nil
	}
	if err := ipt("filter", append([]string{"-I", "HSH_INPUT", "1"}, args...)...); err != nil {
		return fmt.Errorf("install %s carrier firewall rule: %w", c.Transport.Type, err)
	}
	return nil
}
