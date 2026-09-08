package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const (
	dataMark       = 0x66
	dataTable      = 166
	transportTable = 167
)

func setupTun(c *Config) error {
	if err := runCmd("ip", "link", "set", "dev", c.Tun.Name, "up", "mtu", strconv.Itoa(c.Tun.MTU)); err != nil {
		return err
	}
	if err := runCmd("ip", "addr", "replace", c.Tun.LocalCIDR, "dev", c.Tun.Name); err != nil {
		return err
	}
	if err := runCmd("sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		return err
	}
	return runCmd("sysctl", "-w", "net.ipv4.conf."+c.Tun.Name+".rp_filter=2")
}

func setupRouting(c *Config) error {
	if len(c.Ports) == 0 {
		return nil
	}
	if c.Role == "iran" {
		return setupIranRouting(c)
	}
	return setupKharejRouting(c)
}

func setupIranRouting(c *Config) error {
	if err := runCmd("ip", "rule", "add", "priority", "66", "fwmark", fmt.Sprintf("0x%x", dataMark), "lookup", strconv.Itoa(dataTable)); err != nil {
		return err
	}
	if err := runCmd("ip", "route", "add", "table", strconv.Itoa(dataTable), c.Network.ForeignPublicIP+"/32", "dev", c.Tun.Name); err != nil {
		return err
	}

	hooks := []struct{ table, chain, custom string }{
		{"mangle", "PREROUTING", "HSH_MPRE"}, {"nat", "PREROUTING", "HSH_NPRE"}, {"nat", "POSTROUTING", "HSH_NPOST"},
		{"filter", "FORWARD", "HSH_FWD"}, {"mangle", "FORWARD", "HSH_MFWD"}, {"filter", "INPUT", "HSH_INPUT"},
	}
	for _, h := range hooks {
		if err := resetChain(h.table, h.chain, h.custom); err != nil {
			return err
		}
	}

	// Carrier access is installed later by setupCarrierFirewall(), because it
	// depends on whether the selected carrier is UDP/KCP or TCP.
	if c.SmartReturn.Enabled {
		if err := ipt("filter", "-A", "HSH_INPUT", "-i", c.Network.PublicInterface, "-d", c.Network.IranPublicIP, "-s", c.Network.ForeignPublicIP, "-p", "udp", "--dport", strconv.Itoa(c.SmartReturn.ProbePort), "-j", "ACCEPT"); err != nil {
			return err
		}
	}

	for _, p := range c.Ports {
		for _, proto := range protocols(p.Protocol) {
			port := strconv.Itoa(p.Port)
			if err := ipt("mangle", "-A", "HSH_MPRE", "-i", c.Network.PublicInterface, "-d", c.Network.PublicIP, "-p", proto, "--dport", port, "-j", "MARK", "--set-mark", fmt.Sprintf("0x%x", dataMark)); err != nil {
				return err
			}
			if err := ipt("nat", "-A", "HSH_NPRE", "-i", c.Network.PublicInterface, "-d", c.Network.PublicIP, "-p", proto, "--dport", port, "-j", "DNAT", "--to-destination", fmt.Sprintf("%s:%d", c.Network.ForeignPublicIP, p.Port)); err != nil {
				return err
			}
			if err := ipt("filter", "-A", "HSH_FWD", "-i", c.Network.PublicInterface, "-o", c.Tun.Name, "-d", c.Network.ForeignPublicIP, "-p", proto, "--dport", port, "-m", "conntrack", "--ctstate", "NEW,ESTABLISHED", "-j", "ACCEPT"); err != nil {
				return err
			}
		}
	}
	if err := ipt("nat", "-A", "HSH_NPOST", "-o", c.Tun.Name, "-d", c.Network.ForeignPublicIP, "-j", "SNAT", "--to-source", c.Network.IranPublicIP); err != nil {
		return err
	}
	if err := ipt("filter", "-A", "HSH_FWD", "-i", c.Tun.Name, "-o", c.Network.PublicInterface, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
		return err
	}
	if err := ipt("filter", "-A", "HSH_FWD", "-i", c.Network.PublicInterface, "-o", c.Network.PublicInterface, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
		return err
	}

	mss := strconv.Itoa(c.Tun.MTU - 40)
	if c.Mode == "full" {
		return ipt("mangle", "-A", "HSH_MFWD", "-o", c.Tun.Name, "-m", "mark", "--mark", fmt.Sprintf("0x%x", dataMark), "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", mss)
	}
	return ipt("mangle", "-A", "HSH_MFWD", "-i", c.Network.PublicInterface, "-o", c.Network.PublicInterface, "-s", c.Network.ForeignPublicIP, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-m", "conntrack", "--ctstate", "ESTABLISHED", "-j", "TCPMSS", "--set-mss", mss)
}

func setupTransportBypass(c *Config) error {
	if err := runCmd("ip", "rule", "add", "priority", "77", "fwmark", fmt.Sprintf("0x%x", transportMark), "lookup", strconv.Itoa(transportTable)); err != nil {
		return err
	}
	routeArgs := []string{"route", "add", "table", strconv.Itoa(transportTable), "default"}
	if c.Network.PublicGateway != "" {
		routeArgs = append(routeArgs, "via", c.Network.PublicGateway)
	}
	routeArgs = append(routeArgs, "dev", c.Network.PublicInterface, "src", c.Network.PublicIP)
	return runCmd("ip", routeArgs...)
}

func setupKharejRouting(c *Config) error {
	hooks := []struct{ table, chain, custom string }{{"filter", "INPUT", "HSH_INPUT"}, {"filter", "OUTPUT", "HSH_OUTPUT"}, {"mangle", "OUTPUT", "HSH_MOUT"}}
	for _, h := range hooks {
		if err := resetChain(h.table, h.chain, h.custom); err != nil {
			return err
		}
	}

	// The transport-specific INPUT rule is added by setupCarrierFirewall().
	for _, p := range c.Ports {
		for _, proto := range protocols(p.Protocol) {
			port := strconv.Itoa(p.Port)
			if err := ipt("filter", "-A", "HSH_INPUT", "-i", c.Tun.Name, "-d", c.Network.ForeignPublicIP, "-p", proto, "--dport", port, "-m", "conntrack", "--ctstate", "NEW,ESTABLISHED", "-j", "ACCEPT"); err != nil {
				return err
			}
			if c.Network.LockServicePorts {
				if err := ipt("filter", "-A", "HSH_INPUT", "-i", c.Network.PublicInterface, "-d", c.Network.ForeignPublicIP, "-p", proto, "--dport", port, "-j", "DROP"); err != nil {
					return err
				}
		}
	}

	if c.Mode == "direct-return" {
		if err := ipt("filter", "-A", "HSH_OUTPUT", "-o", c.Network.PublicInterface, "-s", c.Network.ForeignPublicIP, "-d", c.Network.IranPublicIP, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
			return err
		}
		if !c.SmartReturn.Enabled {
			return nil
		}

		// Smart Return may install an Iran /32 route through hsh0. Carrier and
		// direct health-probe sockets use SO_MARK=0x77, so table 167 must always
		// retain a normal-Internet default route while failover is enabled.
		if err := setupTransportBypass(c); err != nil {
			return err
		}
		if err := ipt("filter", "-A", "HSH_OUTPUT", "-o", c.Tun.Name, "-s", c.Network.ForeignPublicIP, "-d", c.Network.IranPublicIP, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
			return err
		}
		mss := strconv.Itoa(c.Tun.MTU - 40)
		for _, p := range c.Ports {
			if p.Protocol == "udp" {
				continue
			}
			if err := ipt("mangle", "-A", "HSH_MOUT", "-o", c.Tun.Name, "-s", c.Network.ForeignPublicIP, "-d", c.Network.IranPublicIP, "-p", "tcp", "--sport", strconv.Itoa(p.Port), "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", mss); err != nil {
				return err
			}
		}
		return setKharejReturnPath(c, returnPathDirect)
	}

	if err := setupTransportBypass(c); err != nil {
		return err
	}
	if err := runCmd("ip", "route", "replace", c.Network.IranPublicIP+"/32", "dev", c.Tun.Name); err != nil {
		return err
	}
	if err := ipt("filter", "-A", "HSH_OUTPUT", "-o", c.Tun.Name, "-s", c.Network.ForeignPublicIP, "-d", c.Network.IranPublicIP, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
		return err
	}

	mss := strconv.Itoa(c.Tun.MTU - 40)
	for _, p := range c.Ports {
		if p.Protocol == "udp" {
			continue
		}
		if err := ipt("mangle", "-A", "HSH_MOUT", "-o", c.Tun.Name, "-s", c.Network.ForeignPublicIP, "-d", c.Network.IranPublicIP, "-p", "tcp", "--sport", strconv.Itoa(p.Port), "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--set-mss", mss); err != nil {
			return err
		}
	}
	return nil
}

func cleanupRouting(c *Config) {
	chains := []struct{ table, hook, custom string }{
		{"mangle", "PREROUTING", "HSH_MPRE"}, {"nat", "PREROUTING", "HSH_NPRE"}, {"nat", "POSTROUTING", "HSH_NPOST"},
		{"filter", "FORWARD", "HSH_FWD"}, {"mangle", "FORWARD", "HSH_MFWD"}, {"filter", "INPUT", "HSH_INPUT"},
		{"filter", "OUTPUT", "HSH_OUTPUT"}, {"mangle", "OUTPUT", "HSH_MOUT"},
	}
	for _, x := range chains {
		deleteChain(x.table, x.hook, x.custom)
	}
	runQuiet("ip", "rule", "del", "priority", "66", "fwmark", fmt.Sprintf("0x%x", dataMark), "lookup", strconv.Itoa(dataTable))
	runQuiet("ip", "route", "flush", "table", strconv.Itoa(dataTable))
	runQuiet("ip", "rule", "del", "priority", "77", "fwmark", fmt.Sprintf("0x%x", transportMark), "lookup", strconv.Itoa(transportTable))
	runQuiet("ip", "route", "flush", "table", strconv.Itoa(transportTable))
	if c.Role == "kharej" && c.Network.IranPublicIP != "" {
		runQuiet("ip", "route", "del", c.Network.IranPublicIP+"/32", "dev", c.Tun.Name)
	}
}

func protocols(p string) []string {
	if p == "both" {
		return []string{"tcp", "udp"}
	}
	return []string{p}
}

func resetChain(table, hook, custom string) error {
	runQuiet("iptables", "-t", table, "-N", custom)
	if err := ipt(table, "-F", custom); err != nil {
		return err
	}
	if exec.Command("iptables", "-t", table, "-C", hook, "-j", custom).Run() != nil {
		return ipt(table, "-I", hook, "1", "-j", custom)
	}
	return nil
}

func deleteChain(table, hook, custom string) {
	for exec.Command("iptables", "-t", table, "-C", hook, "-j", custom).Run() == nil {
		runQuiet("iptables", "-t", table, "-D", hook, "-j", custom)
	}
	runQuiet("iptables", "-t", table, "-F", custom)
	runQuiet("iptables", "-t", table, "-X", custom)
}

func ipt(table string, args ...string) error {
	return runCmd("iptables", append([]string{"-t", table}, args...)...)
}
func runQuiet(name string, args ...string) { _ = exec.Command(name, args...).Run() }
func runCmd(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
