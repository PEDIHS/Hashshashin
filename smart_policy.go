package main

import (
	"fmt"
	"strconv"
)

const (
	returnMark  = 0x68
	returnTable = 168
)

// setupSmartReturnPolicy marks only service response flows on Kharej. This is
// deliberately separate from the main routing table: switching the download
// path must never redirect unrelated Kharej -> Iran traffic such as SSH.
func setupSmartReturnPolicy(c *Config) error {
	if !c.SmartReturn.Enabled || c.Role != "kharej" || c.Mode != "direct-return" || len(c.Ports) == 0 {
		return nil
	}

	// Idempotence for restarts/reconfigure.
	cleanupSmartReturnPolicy(c)
	if err := runCmd("ip", "rule", "add", "priority", "68", "fwmark", fmt.Sprintf("0x%x", returnMark), "lookup", strconv.Itoa(returnTable)); err != nil {
		return err
	}

	for _, p := range c.Ports {
		for _, proto := range protocols(p.Protocol) {
			port := strconv.Itoa(p.Port)
			// Never overwrite a socket mark. Hashshashin carrier/probe sockets use
			// 0x77 and must remain on the normal Internet route even in fallback.
			if err := ipt("mangle", "-A", "HSH_MOUT",
				"-s", c.Network.ForeignPublicIP,
				"-d", c.Network.IranPublicIP,
				"-p", proto,
				"--sport", port,
				"-m", "mark", "--mark", "0x0",
				"-j", "MARK", "--set-mark", fmt.Sprintf("0x%x", returnMark)); err != nil {
				cleanupSmartReturnPolicy(c)
				return err
			}

			if proto == "tcp" {
				// In fallback the route is re-evaluated after OUTPUT marking, so clamp
				// by mark rather than the initially selected output interface.
				mss := strconv.Itoa(c.Tun.MTU - 40)
				if err := ipt("mangle", "-A", "HSH_MOUT",
					"-s", c.Network.ForeignPublicIP,
					"-d", c.Network.IranPublicIP,
					"-p", "tcp", "--sport", port,
					"-m", "mark", "--mark", fmt.Sprintf("0x%x", returnMark),
					"--tcp-flags", "SYN,RST", "SYN",
					"-j", "TCPMSS", "--set-mss", mss); err != nil {
					cleanupSmartReturnPolicy(c)
					return err
				}
			}
		}
	}
	return setKharejReturnPath(c, returnPathDirect)
}

func cleanupSmartReturnPolicy(c *Config) {
	if c == nil {
		return
	}
	runQuiet("ip", "rule", "del", "priority", "68", "fwmark", fmt.Sprintf("0x%x", returnMark), "lookup", strconv.Itoa(returnTable))
	runQuiet("ip", "route", "flush", "table", strconv.Itoa(returnTable))
}

func setKharejReturnPath(c *Config, path returnPath) error {
	if c.Role != "kharej" || c.Mode != "direct-return" || !c.SmartReturn.Enabled {
		return nil
	}
	switch path {
	case returnPathDirect:
		// No route in table 168 means RPDB falls through to the normal main
		// table, preserving the direct Kharej -> Iran path.
		runQuiet("ip", "route", "flush", "table", strconv.Itoa(returnTable))
		return nil
	case returnPathTunnel:
		return runCmd("ip", "route", "replace", "table", strconv.Itoa(returnTable),
			c.Network.IranPublicIP+"/32", "dev", c.Tun.Name, "mtu", strconv.Itoa(c.Tun.MTU))
	default:
		return fmt.Errorf("unknown return path %q", path)
	}
}
