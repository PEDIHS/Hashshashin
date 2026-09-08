package main

import (
	"fmt"
	"os/exec"
	"strconv"
)

// setupCarrierFirewall aligns INPUT rules with the selected carrier and role.
// UDP/KCP use the configured local port on both nodes. TCP is asymmetric at
// the socket layer: Iran dials, Kharej listens. Therefore Iran accepts only
// ESTABLISHED replies from the Kharej carrier source port instead of exposing
// the carrier destination port locally.
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

	var args []string
	if c.Transport.Type == "tcp" && c.Role == "iran" {
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
		selected := transportNetwork(c.Transport.Type)
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
