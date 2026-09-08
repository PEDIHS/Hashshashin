package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

var buildRef = "source"

const (
	version     = "0.1.0"
	managerPath = "/usr/local/libexec/hashshashin-manager"
)

func main() {
	// `hashshashin` with no arguments is the operator-facing management panel.
	// The systemd unit always passes -c, so daemon startup remains unambiguous.
	if len(os.Args) == 1 {
		if err := launchManager(); err != nil {
			log.Fatal(err)
		}
		return
	}

	configPath := flag.String("c", "/etc/hashshashin/config.json", "config path")
	check := flag.Bool("check", false, "validate configuration and exit")
	cleanup := flag.Bool("cleanup", false, "remove Hashshashin routes/firewall rules and exit")
	keygen := flag.Bool("keygen", false, "generate a 256-bit shared key and exit")
	showVersion := flag.Bool("version", false, "print version and exit")
	showSummary := flag.Bool("summary", false, "print a safe machine-readable config summary")
	menu := flag.Bool("menu", false, "open the interactive management panel")
	flag.Parse()

	if *menu {
		if err := launchManager(); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *showVersion {
		fmt.Printf("Hashshashin %s (%s)\n", version, buildRef)
		return
	}
	if *keygen {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			log.Fatal(err)
		}
		fmt.Println(base64.StdEncoding.EncodeToString(b))
		return
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if *showSummary {
		printConfigSummary(cfg)
		return
	}
	if *check {
		fmt.Printf("Hashshashin %s: config ok\n", version)
		return
	}
	if os.Geteuid() != 0 {
		log.Fatal("Hashshashin must run as root")
	}
	if *cleanup {
		cleanupRouting(cfg)
		return
	}
	if err := run(cfg); err != nil {
		log.Fatal(err)
	}
}

func launchManager() error {
	if _, err := os.Stat(managerPath); err != nil {
		return fmt.Errorf("management panel is not installed at %s; run the official installer again", managerPath)
	}
	cmd := exec.Command(managerPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func printConfigSummary(c *Config) {
	ports := make([]string, 0, len(c.Ports))
	for _, p := range c.Ports {
		ports = append(ports, strconv.Itoa(p.Port)+"/"+p.Protocol)
	}
	peer := c.Transport.Peer
	if peer == "" {
		peer = "-"
	}
	fmt.Printf("role=%s\n", c.Role)
	fmt.Printf("mode=%s\n", c.Mode)
	fmt.Printf("tun_name=%s\n", c.Tun.Name)
	fmt.Printf("tun_cidr=%s\n", c.Tun.LocalCIDR)
	fmt.Printf("tun_peer=%s\n", c.Tun.PeerIP)
	fmt.Printf("mtu=%d\n", c.Tun.MTU)
	fmt.Printf("public_interface=%s\n", c.Network.PublicInterface)
	fmt.Printf("public_ip=%s\n", c.Network.PublicIP)
	fmt.Printf("foreign_public_ip=%s\n", c.Network.ForeignPublicIP)
	fmt.Printf("iran_public_ip=%s\n", c.Network.IranPublicIP)
	fmt.Printf("carrier_listen=%s\n", c.Transport.Listen)
	fmt.Printf("carrier_peer=%s\n", peer)
	fmt.Printf("service_ports=%s\n", strings.Join(ports, ","))
}

func run(c *Config) error {
	log.Printf("Hashshashin %s starting role=%s mode=%s", version, c.Role, c.Mode)
	tun, err := openTun(c.Tun.Name)
	if err != nil {
		return err
	}
	defer tun.Close()

	cleanupRouting(c)
	if err := setupTun(c); err != nil {
		return err
	}
	if err := setupRouting(c); err != nil {
		cleanupRouting(c)
		return err
	}
	defer cleanupRouting(c)

	key, _ := base64.StdEncoding.DecodeString(c.Transport.Key)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	udp, err := listenMarkedUDP(ctx, c.Transport.Listen)
	if err != nil {
		return err
	}
	defer udp.Close()

	st := &tunnelState{hellos: make(map[[32]byte]helloRecord)}
	go recvLoop(ctx, udp, tun, key, st, c.Role)

	if c.Role == "iran" {
		peer, err := net.ResolveUDPAddr("udp4", c.Transport.Peer)
		if err != nil {
			return err
		}
		go clientSupervisor(ctx, udp, peer, key, st, c)
	} else {
		go serverSupervisor(ctx, udp, st, c)
	}
	go statsLoop(ctx, st)

	go func() {
		<-ctx.Done()
		_ = udp.Close()
		_ = tun.Close()
	}()

	buf := make([]byte, 65535)
	for {
		n, err := tun.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("tun read: %w", err)
		}
		ss := st.loadSession()
		if ss == nil {
			continue
		}
		if err := sendEncrypted(udp, ss, msgData, buf[:n]); err != nil {
			log.Printf("transport send: %v", err)
		}
	}
}
