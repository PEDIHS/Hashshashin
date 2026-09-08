package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

var buildRef = "source"

const (
	version     = "0.2.0-alpha"
	managerPath = "/usr/local/libexec/hashshashin-manager"
)

func main() {
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
		fmt.Printf("Hashshashin %s: config ok (transport=%s)\n", version, cfg.Transport.Type)
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
	fmt.Printf("transport=%s\n", c.Transport.Type)
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
	if c.Transport.Type == "kcp" {
		fmt.Printf("kcp_fec=%d/%d\n", c.Transport.KCP.DataShards, c.Transport.KCP.ParityShards)
		fmt.Printf("kcp_window=%d/%d\n", c.Transport.KCP.SendWindow, c.Transport.KCP.ReceiveWindow)
	}
}

func run(c *Config) error {
	log.Printf("Hashshashin %s starting role=%s mode=%s transport=%s", version, c.Role, c.Mode, c.Transport.Type)
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
	if err := setupCarrierFirewall(c); err != nil {
		cleanupRouting(c)
		return err
	}
	defer cleanupRouting(c)

	key, _ := base64.StdEncoding.DecodeString(c.Transport.Key)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	carrier, err := newCarrier(ctx, c)
	if err != nil {
		return err
	}
	defer carrier.Close()
	log.Printf("carrier=%s listen=%s peer=%s", carrier.Name(), c.Transport.Listen, c.Transport.Peer)

	st := &tunnelState{hellos: make(map[[32]byte]helloRecord)}
	go recvLoop(ctx, carrier, tun, key, st, c.Role)

	if c.Role == "iran" {
		go clientSupervisor(ctx, carrier, key, st, c)
	} else {
		go serverSupervisor(ctx, carrier, st, c)
	}
	go statsLoop(ctx, st)

	go func() {
		<-ctx.Done()
		_ = carrier.Close()
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
		if err := sendEncrypted(carrier, ss, msgData, buf[:n]); err != nil {
			log.Printf("%s send: %v", carrier.Name(), err)
		}
	}
}
