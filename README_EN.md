# Hashshashin — English Guide

[← Back to project home](README.md) · [فارسی](README_FA.md)

## Overview

Hashshashin is an independently implemented Linux L3 tunnel that carries complete IPv4 packets through a TUN interface named `hsh0`.

It supports two deployment modes:

- **Full Tunnel** — both upload and download traverse the encrypted Hashshashin carrier.
- **Direct Return** — upload traverses the Iran → Kharej tunnel, while download returns from Kharej to Iran through the normal Internet path. The end user continues to connect to the same Iran IP/port.

Current release: **v0.1.0 Official Beta**.

## Why an L3 tunnel?

A classic port forwarder usually terminates the user connection and creates another connection on the remote server. Hashshashin instead carries IP packets through TUN. This allows the Linux routing stack to make independent decisions about the forward and return paths.

That packet-level model is what makes Direct Return possible without changing the user's configured endpoint.

## Features

### Data plane

- Linux TUN interface (`hsh0`)
- complete IPv4 packet transport
- Full Tunnel mode
- Direct Return mode
- transparent destination routing toward configured Kharej service ports
- Linux conntrack/NAT integration
- dedicated policy-routing tables

### Transport and session security

- authenticated UDP carrier
- 256-bit pre-shared key
- HMAC-SHA256 handshake authentication
- fresh random client and server nonces
- independent directional AES-256-GCM session keys
- packet counters
- 64-packet replay window
- out-of-order packet tolerance
- keepalive
- reconnect
- periodic rekey

### Routing hardening

Hashshashin separates the application data plane from the outer carrier:

- data mark: `0x66`
- data routing table: `166`
- carrier mark: `0x77`
- carrier routing table: `167`

This separation prevents the outer UDP carrier from recursively entering the tunnel when Full Tunnel routes are active.

### Firewall behavior

Hashshashin creates dedicated iptables chains and does not globally change the host's INPUT or FORWARD policy to ACCEPT.

The installer can optionally block direct public access to configured service ports on the Kharej host while still allowing the tunnel path.

## Requirements

- Linux
- IPv4
- root access / `CAP_NET_ADMIN`
- `/dev/net/tun`
- `iproute2`
- `iptables`
- systemd for the official installer
- `apt` or `dnf` based distribution for the one-line installer
- Go 1.18+ when building from source

Ubuntu and Debian are the primary tested installer targets. RHEL-compatible systems are supported on a best-effort basis.

## One-line installation

Run on both servers:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh)
```

Install Iran first, then Kharej.

### Step 1 — Iran

Choose:

```text
1) Iran (entry server)
```

Then choose the operating mode:

```text
1) Full tunnel
2) Direct return
```

The wizard asks for:

- public interface
- local public IPv4
- public gateway
- UDP carrier port
- service ports
- tunnel MTU
- Kharej public IPv4

The installer then generates a shared key. Save the exact value.

### Step 2 — Kharej

Run the same command and choose:

```text
2) Kharej (service server)
```

Use the **same**:

- operating mode
- UDP carrier port
- service port list
- shared key

The installer can also ask whether direct public access to the configured service ports should be blocked on Kharej.

### Step 3 — Cloud firewall

If your provider has a separate firewall/security-group layer, allow the UDP carrier port between the Iran and Kharej public IP addresses.

Default example:

```text
Protocol: UDP
Port:     9000
Source:   Iran public IP
Target:   Kharej public IP
```

Do not expose the carrier to the entire Internet unless your environment requires it.

### Step 4 — Destination service

Your Kharej service, for example Xray/3x-ui, must listen on:

```text
0.0.0.0:<service-port>
```

or:

```text
Kharej_Public_IP:<service-port>
```

A listener bound only to `127.0.0.1` will not receive traffic addressed to the Kharej public IP through `hsh0`.

## Direct Return prerequisites

Direct Return requires the Iran public IPv4 to be actually assigned to the Iran server's interface.

Verify with:

```bash
ip -4 addr show
```

If the host only receives Internet connectivity through an upstream NAT/CGNAT address that is not locally assigned, Direct Return is not supported in v0.1. Use Full Tunnel instead.

## Architecture

### Full Tunnel

```text
Client
  |
  v
Iran public endpoint
  |
  v
hsh0 (Iran)
  |
  | authenticated + encrypted UDP carrier
  v
hsh0 (Kharej)
  |
  v
Kharej service
  |
  v
hsh0 (Kharej)
  |
  v
hsh0 (Iran)
  |
  v
Client
```

### Direct Return

```text
UPLOAD
Client -> Iran -> hsh0 ===== encrypted tunnel =====> hsh0 -> Kharej service

DOWNLOAD
Client <- Iran <----------- normal Internet ----------- Kharej service
```

Iran conntrack preserves the connection/NAT state and rewrites the returning packets so the user's endpoint remains unchanged.

## MTU and MSS

Default tunnel MTU is `1320`.

The installer accepts values from `900` to `1400`.

Start with `1320`. If the path shows fragmentation symptoms, stalls, or provider-specific encapsulation overhead, test values such as `1280` or `1240`.

TCP MSS clamping is applied to reduce the chance of tunnel-path fragmentation. Direct Return uses direction-aware handling so the direct download side is not unnecessarily constrained by the upload tunnel MTU.

## Management

Status:

```bash
systemctl status hashshashin
```

Live logs:

```bash
journalctl -u hashshashin -f
```

Restart:

```bash
systemctl restart hashshashin
```

Validate configuration:

```bash
hashshashin -check -c /etc/hashshashin/config.json
```

Show version:

```bash
hashshashin -version
```

Remove Hashshashin networking rules without uninstalling:

```bash
hashshashin -cleanup -c /etc/hashshashin/config.json
```

Uninstall:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh) --uninstall
```

The installer keeps `/etc/hashshashin/config.json` during uninstall by default so the shared key and configuration are not accidentally lost.

## Verification

On Iran:

```bash
ip addr show hsh0
ip rule show
ip route show table 166
iptables -t nat -S | grep HSH
journalctl -u hashshashin -n 100 --no-pager
```

On Kharej:

```bash
ip addr show hsh0
ss -lntup
journalctl -u hashshashin -n 100 --no-pager
```

For Direct Return, packet capture should generally show:

```text
Iran hsh0       -> upload application packets
Kharej hsh0     -> upload application packets
Kharej public   -> download application packets
Iran public     -> download application packets
```

The Persian live verification guide contains more detailed commands: [docs/VERIFY_FA.md](docs/VERIFY_FA.md).

## CI and testing

The repository CI currently validates the core on Go 1.18 and Go 1.22 and includes:

- race-enabled unit tests
- transport/session integration test over loopback UDP
- authenticated handshake validation
- directional session-key validation
- encrypted payload delivery validation
- `go vet`
- `go build`
- installer shell syntax validation

CI passing does not replace a two-VPS network test because TUN, policy routing, provider firewalls and Direct Return path behavior are infrastructure-dependent.

## Security considerations

The v0.1 protocol has not yet undergone an independent external security audit.

For sensitive deployments:

- restrict the carrier port to known peer IPs where possible
- keep SSH/admin panels separately protected
- use the installer's service-port lock option on Kharej when appropriate
- keep the shared key private
- review `SECURITY.md`
- validate routing/firewall state before enabling user traffic

See [SECURITY.md](SECURITY.md).

## Current limitations

- IPv6 is not yet implemented.
- UDP is the current carrier.
- Raw/KCP and multipath carriers are roadmap items.
- automatic Direct → Tunnel quality failover is not yet active in v0.1.
- Direct Return does not support an Iran server that only has upstream NAT/CGNAT addressing.
- provider-specific routing/security behavior can affect operation.

## Roadmap

- automatic Direct Return health monitoring and fallback
- multipath transport
- optional Raw/KCP-style carrier
- IPv6
- release binaries
- package repositories
- external security review
- multi-provider benchmarks

## Original implementation

Hashshashin does not copy source code from Paqet, Backhaul or BackPack. Public architectural ideas such as packet transport, TUN/L3 forwarding and carrier/data-plane isolation were studied, while this project's core and wire protocol are independently implemented.

## More documentation

- [Step-by-step installation](docs/INSTALL_EN.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Security](SECURITY.md)
- [Changelog](CHANGELOG.md)
- [Contributing](CONTRIBUTING.md)
- [Persian guide](README_FA.md)

## License

MIT — see [LICENSE](LICENSE).
