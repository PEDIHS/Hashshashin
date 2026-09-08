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
	"os/signal"
	"syscall"
)

var buildRef = "source"

const version = "0.1.0"

func main() {
	configPath := flag.String("c", "/etc/hashshashin/config.json", "config path")
	check := flag.Bool("check", false, "validate configuration and exit")
	cleanup := flag.Bool("cleanup", false, "remove Hashshashin routes/firewall rules and exit")
	keygen := flag.Bool("keygen", false, "generate a 256-bit shared key and exit")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

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
