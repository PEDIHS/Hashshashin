# Hashshashin — English Guide

[← Project Home](README.md) · [فارسی](README_FA.md) · [Transports](docs/TRANSPORTS_FA.md) · [Smart Return](docs/SMART_RETURN_FA.md)

## Overview

**Hashshashin** is an independently implemented Linux **Layer-3/TUN** tunnel that carries complete IPv4 packets through a virtual interface named `hsh0`.

Current development version: **v0.2.0-alpha**.

It supports three practical routing modes:

1. **Full Tunnel** — upload and download both traverse Hashshashin.
2. **Direct Return** — upload traverses the Iran → Kharej tunnel while download returns directly from Kharej to Iran through the normal Internet path.
3. **Direct Return + Smart Return** — direct download is preferred; if that path becomes unhealthy, service responses automatically fall back to the tunnel and later recover to direct routing.

The end-user endpoint remains the Iran server. Client-side VLESS, VMess, Trojan, Shadowsocks, WireGuard, or other service configuration does not need to change.

---

## Architecture

```text
User protocol
VLESS / VMess / Trojan / Shadowsocks / WireGuard / ...
                         |
                         v
                    Linux L3/TUN
                         |
                       HSH1
     HMAC handshake + AES-256-GCM + replay protection
                         |
                Transport Manager
                  /      |      \
                UDP     TCP     KCP/FEC
                         |
                   Iran <-> Kharej
```

Hashshashin is not a classic port proxy. It transports IP packets and relies on Linux routing, conntrack and NAT for forwarding decisions.

---

## Path Modes

| Mode | Upload | Download | Automatic Return Failover |
|---|---|---|---|
| Full Tunnel | Tunnel | Tunnel | — |
| Direct Return | Tunnel | Direct Kharej → Iran | No |
| Direct Return + Smart Return | Tunnel | Direct preferred, tunnel fallback | Yes |

### Smart Return

Smart Return probes the direct Kharej → Iran direction. Defaults:

```text
probe_port         9001/UDP
interval           5s
timeout             2s
failure threshold  3
recovery threshold 3
```

A probe contains an independent `HDP1` header, timestamp, random nonce and HMAC-SHA256 using the tunnel PSK. Iran returns the nonce through an encrypted HSH1 control message.

The routing domains are isolated:

```text
0x66 / table 166 -> user upload/data
0x68 / table 168 -> Smart Return service responses
0x77 / table 167 -> carrier + direct health probe bypass
```

Smart Return changes only configured service-response flows. It does not install a broad main-table route that would redirect unrelated SSH or management traffic.

See: [Smart Return design and operations](docs/SMART_RETURN_FA.md).

---

## Implemented Carriers

| Carrier | Status | Typical Use |
|---|---:|---|
| UDP | ✅ | lowest overhead and latency |
| Framed TCP | ✅ | networks where UDP is restricted or degraded |
| KCP | ✅ | lossy/jittery paths requiring fast ARQ |
| KCP + FEC 10/3 | ✅ | balanced loss recovery preset |
| KCP + FEC 10/5 | ✅ | stronger FEC with higher bandwidth overhead |
| Raw/Pcap | Roadmap | not implemented yet |
| QUIC/WSS | Roadmap | not implemented yet |
| Multipath | Roadmap | not implemented yet |

All carriers share the same HSH1 session security:

- 256-bit pre-shared key
- HMAC-SHA256 authenticated handshake
- fresh client/server nonces
- independent TX/RX AES-256-GCM session keys
- packet counters and replay window
- keepalive
- reconnect
- periodic rekey

KCP support uses `github.com/xtaci/kcp-go/v5` under its MIT license.

---

# Installation

Run the same installer on both Iran and Kharej servers:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh)
```

Recommended order: **Iran first, Kharej second**.

The wizard performs:

```text
[1/8] System check & dependencies
[2/8] Download, test & build
[3/8] Select server role
[4/8] Tunnel mode & Smart Return
[5/8] Select carrier transport
[6/8] Network & service configuration
[7/8] Peer & security
[8/8] Start & verify
```

### Iran

Choose:

```text
IRAN / Entry
Full Tunnel or Direct Return
Smart Return (optional for Direct Return)
UDP / TCP / KCP carrier
```

The installer generates a 256-bit Shared Key. Save it for the Kharej installation.

### Kharej

Run the installer again and select `KHAREJ / Exit`. Use matching:

- mode
- carrier type
- carrier port
- service ports
- Smart Return settings
- Shared Key

---

## Provider Firewall

Allow the carrier only between the two server IPs:

```text
UDP or KCP : UDP/<carrier-port>
TCP        : TCP/<carrier-port>
```

When Smart Return is enabled, also allow:

```text
Protocol    UDP
Source      Kharej Public IP
Destination Iran Public IP
Port        smart_return.probe_port (default 9001)
```

There is no need to expose the probe port to the whole Internet.

---

## Direct Return Requirement

The Iran public IPv4 address must actually be assigned to the Iran server interface:

```bash
ip -4 addr show
```

An upstream-NAT/CGNAT-only Iran endpoint is not supported by Direct Return in the current release. Use Full Tunnel in that environment.

---

## Kharej Service Binding

For example, an Xray/3x-ui inbound on port `443` should normally listen on:

```text
0.0.0.0:443
```

or the Kharej public address. Binding only to `127.0.0.1:443` is not suitable for this topology because service packets arrive from `hsh0` addressed to the Kharej public IP.

---

# Management

After installation:

```bash
hashshashin
```

The interactive manager shows:

- service state
- Iran/Kharej role
- Full/Direct mode
- active carrier
- TUN/MTU
- RX/TX counters
- Smart Return enabled/disabled
- current return path (`DIRECT` or `TUNNEL` on Kharej)
- health checks
- live logs
- network diagnostics
- safe config view
- update/reconfigure/uninstall actions

Useful direct commands:

```bash
systemctl status hashshashin
journalctl -u hashshashin -f
hashshashin -check -c /etc/hashshashin/config.json
hashshashin -summary -c /etc/hashshashin/config.json
```

Update while preserving configuration:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh) --update
```

Reconfigure:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh) --reconfigure
```

Uninstall:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh) --uninstall
```

---

## CI and Verification

GitHub Actions runs on Go 1.18 and Go 1.22 and includes:

- `go test -race ./...`
- HSH1 crypto/session/replay tests
- UDP handshake + encrypted payload integration
- framed TCP carrier integration
- KCP/FEC carrier integration
- Smart Return HMAC probe authentication
- Smart Return hysteresis tests
- encrypted direct-probe ACK dispatch
- `go vet`
- `go build`
- installer and manager shell syntax checks

A green CI run does **not** prove every real-world ISP/provider route. TUN, conntrack/NAT, MTU, firewall behavior, Direct Return, and failover must still be verified on real Iran/Kharej VPS infrastructure before production deployment.

---

## Security Notes

Hashshashin does not change the host's global firewall policy to `ACCEPT`; it owns dedicated `HSH_*` chains. Keep the Shared Key private and separately restrict SSH, management panels and databases.

HSH1 has not yet undergone an independent external security audit.

See [SECURITY.md](SECURITY.md).

---

## Roadmap

- [x] L3/TUN core
- [x] Full Tunnel
- [x] Direct Return
- [x] UDP carrier
- [x] TCP carrier
- [x] KCP/FEC carrier
- [x] reconnect / keepalive / rekey / replay protection
- [x] Smart Return health protocol
- [x] Direct → Tunnel → Direct return-path failover
- [x] service-scoped return policy routing
- [ ] Raw TCP / pcap carrier
- [ ] QUIC / WSS carrier
- [ ] multipath / multi-carrier bonding
- [ ] automatic carrier failover
- [ ] IPv6
- [ ] binary/package releases
- [ ] independent external security audit
- [ ] multi-provider benchmark suite

---

Hashshashin is independently implemented. Public architectural concepts from other tunnel projects have been studied, but the L3 core, HSH1 protocol, routing controller, installer and Smart Return implementation are not source-code copies of Paqet, Backhaul or BackPack.
