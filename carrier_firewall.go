package main

import (
	"fmt"
	"os/exec"
	"strconv"
)

// setupCarrierFirewall aligns the host INPUT rule with the selected carrier.
// Older v0.1 configs always installed an UDP carrier rule; when a node is
// reconfigured to TCP this function removes the stale rule before inserting
// the correct one. The HSH_INPUT chain is owned by Hashshashin and is removed
// by cleanupRouting.
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
	for _, proto := range []string{"udp", "tcp"} {
		if proto == selected {
			continue
		}
		for exec.Command("iptables", "-t", "filter", "-C", "HSH_INPUT",
			"-i", c.Network.PublicInterface,
			"-d", c.Network.PublicIP,
			"-s", sourceIP,
			"-p", proto,
			"--dport", strconv.Itoa(port),
			"-j", "ACCEPT").Run() == nil {
			runQuiet("iptables", "-t", "filter", "-D", "HSH_INPUT",
				"-i", c.Network.PublicInterface,
				"-d", c.Network.PublicIP,
				"-s", sourceIP,
				"-p", proto,
				"--dport", strconv.Itoa(port),
				"-j", "ACCEPT")
		}
	}

	args := []string{
		"-i", c.Network.PublicInterface,
		"-d", c.Network.PublicIP,
		"-s", sourceIP,
		"-p", selected,
		"--dport", strconv.Itoa(port),
		"-j", "ACCEPT",
	}
	if exec.Command("iptables", append([]string{"-t", "filter", "-C", "HSH_INPUT"}, args...)...).Run() == nil {
		return nil
	}
	if err := ipt("filter", append([]string{"-I", "HSH_INPUT", "1"}, args...)...); err != nil {
		return fmt.Errorf("install %s carrier firewall rule: %w", selected, err)
	}
	return nil
}
