<div align="center">

# Hashshashin

### A lightweight Linux L3 tunnel with Full Tunnel and Direct Return modes

**حشاشین — یک تونل L3 مستقل برای Linux با حالت تونل کامل و دانلود مستقیم از خارج**

[![CI](https://github.com/PEDIHS/Hashshashin/actions/workflows/ci.yml/badge.svg)](https://github.com/PEDIHS/Hashshashin/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.18%2B-00ADD8?logo=go&logoColor=white)
![Linux](https://img.shields.io/badge/Platform-Linux-FCC624?logo=linux&logoColor=black)
![License](https://img.shields.io/badge/License-MIT-green.svg)
![Release](https://img.shields.io/badge/Release-v0.1.0%20Beta-blue)

[English](README_EN.md) · [فارسی](README_FA.md) · [Architecture](docs/ARCHITECTURE.md) · [Security](SECURITY.md) · [Changelog](CHANGELOG.md)

</div>

---

## What is Hashshashin?

Hashshashin is an independently implemented **Layer-3 tunnel for Linux**. It carries complete IPv4 packets through a TUN interface (`hsh0`) and is designed around two operating modes:

| Mode | Upload | Download | Client endpoint |
|---|---|---|---|
| **Full Tunnel** | Encrypted tunnel | Encrypted tunnel | Iran server |
| **Direct Return** | Encrypted tunnel | Direct Kharej → Iran path | Iran server |

The Direct Return design is the main differentiator: users keep connecting to the same Iran IP/port, while Linux conntrack, NAT and policy routing allow return traffic from Kharej to use the normal Internet path when the network supports it.

> **Status:** v0.1.0 is an **official beta**. Transport, crypto, reconnect/rekey logic, installer syntax, vet/build and loopback integration tests are covered by CI. Real TUN/policy-routing behavior still depends on the VPS provider and must be validated on the target two-server network.

## Highlights

- **True L3/TUN data plane** — carries IPv4 packets instead of terminating every user TCP session as a port proxy.
- **Two modes** — Full Tunnel or Upload Tunnel + Direct Download.
- **No client configuration change** — users continue to connect to the Iran endpoint.
- **Authenticated encrypted UDP carrier** — 256-bit PSK, HMAC-SHA256 handshake and AES-256-GCM payload protection.
- **Directional session keys** — independent TX/RX keys derived from fresh client/server nonces.
- **Replay protection** — 64-packet replay window with out-of-order tolerance.
- **Reliability controls** — keepalive, reconnect and periodic rekey.
- **Routing isolation** — separate marks/tables for data traffic and the outer carrier to prevent recursive routing loops.
- **MTU/MSS handling** — configurable MTU and direction-aware TCP MSS clamping.
- **Safer firewall integration** — dedicated Hashshashin iptables chains; global INPUT/FORWARD policies are not changed.
- **One-line installer** — interactive setup for Iran and Kharej with systemd integration.
- **Clean uninstall** — removes Hashshashin routing/firewall state while preserving the configuration file by default.

## Architecture

### Full Tunnel

```text
                         encrypted UDP carrier
Client -> Iran -> hsh0  ========================>  hsh0 -> Kharej service
Client <- Iran <- hsh0  <========================  hsh0 <- Kharej service
```

### Direct Return

```text
UPLOAD
Client -> Iran -> hsh0  ========================>  hsh0 -> Kharej service

DOWNLOAD
Client <- Iran  <----------- normal Internet ----------- Kharej service
```

In Direct Return mode, the Iran host keeps conntrack/NAT state so the user's visible endpoint remains the Iran server even though the Kharej → Iran return path bypasses the tunnel carrier.

For the detailed data plane, marks, tables and routing model, see [Architecture](docs/ARCHITECTURE.md).

## Quick install

Run the installer on **both servers**:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh)
```

Recommended order:

1. Install **Iran** first.
2. Select `Full Tunnel` or `Direct Return`.
3. Save the generated shared key.
4. Run the same installer on **Kharej**.
5. Use the same mode, carrier port, service ports and shared key.
6. Allow the UDP carrier port between the two VPS IPs in any cloud firewall/security group.
7. Verify `hsh0`, logs and packet direction before putting production users on the path.

### Direct Return requirement

Direct Return requires the **Iran public IPv4 to be actually assigned to the Iran host interface**. A server that only receives an upstream NAT/CGNAT address is not supported for Direct Return in v0.1.

## Typical setup values

| Setting | Iran | Kharej |
|---|---|---|
| Role | `iran` | `kharej` |
| TUN | `10.77.0.1/30` | `10.77.0.2/30` |
| Interface | `hsh0` | `hsh0` |
| Carrier | UDP | UDP |
| Carrier port | `9000` by default | same value |
| Default MTU | `1320` | same value |
| Service ports | e.g. `443` | same value |

## Kharej service binding

The destination service (for example Xray/3x-ui) must listen on either:

```text
0.0.0.0:<service-port>
```

or:

```text
Kharej_Public_IP:<service-port>
```

A listener bound only to `127.0.0.1` will not receive packets addressed to the Kharej public IP through `hsh0`.

## Management commands

```bash
systemctl status hashshashin
systemctl restart hashshashin
journalctl -u hashshashin -f
hashshashin -version
hashshashin -check -c /etc/hashshashin/config.json
```

Cleanup Hashshashin network rules without uninstalling:

```bash
hashshashin -cleanup -c /etc/hashshashin/config.json
```

Uninstall:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh) --uninstall
```

## Security model

The v0.1 transport currently provides:

- 256-bit pre-shared key
- HMAC-SHA256 authenticated hello/ack handshake
- fresh random client/server nonces per session
- separate AES-256-GCM TX/RX session keys
- per-direction packet counters
- replay-window validation
- periodic rekey

The protocol has **not yet undergone an independent external cryptographic/security audit**. For sensitive or large-scale deployments, review [SECURITY.md](SECURITY.md) before production rollout.

## Verification

Useful first checks:

```bash
ip addr show hsh0
ip rule show
ip route show table 166
systemctl status hashshashin
journalctl -u hashshashin -n 100 --no-pager
```

For Direct Return, packet capture should show upload application traffic on `hsh0`, while download application traffic should appear on the public interfaces rather than returning through `hsh0`.

See the complete Persian verification guide: [docs/VERIFY_FA.md](docs/VERIFY_FA.md).

## Documentation

| Document | Purpose |
|---|---|
| [README_EN.md](README_EN.md) | Complete English guide |
| [README_FA.md](README_FA.md) | راهنمای کامل فارسی |
| [docs/INSTALL_EN.md](docs/INSTALL_EN.md) | Step-by-step English installation |
| [docs/INSTALL_FA.md](docs/INSTALL_FA.md) | نصب مرحله‌به‌مرحله فارسی |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | L3, TUN, routing and Direct Return architecture |
| [docs/VERIFY_FA.md](docs/VERIFY_FA.md) | تست واقعی دو سرور و بررسی مسیر |
| [docs/TROUBLESHOOTING_FA.md](docs/TROUBLESHOOTING_FA.md) | عیب‌یابی فارسی |
| [SECURITY.md](SECURITY.md) | Security policy and deployment considerations |
| [CHANGELOG.md](CHANGELOG.md) | Release history |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Contribution workflow |

## Roadmap

- [x] Linux TUN / L3 core
- [x] Full Tunnel mode
- [x] Direct Return mode
- [x] Authenticated encrypted UDP carrier
- [x] Reconnect / keepalive / periodic rekey
- [x] Routing-loop isolation
- [x] One-line interactive installer
- [ ] Automatic Direct → Tunnel quality failover
- [ ] Multipath carrier
- [ ] Optional Raw/KCP-style carrier
- [ ] IPv6
- [ ] Release binaries and package repositories
- [ ] External security audit
- [ ] Multi-provider live benchmark matrix

## Project philosophy

Hashshashin is **not a source-code copy** of Paqet, Backhaul or BackPack. Their public architectural ideas were studied as references—packet transport, L3/TUN routing and data-plane/carrier separation—while Hashshashin's core and wire protocol are independently implemented.

## Contributing

Issues and pull requests are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) before proposing protocol, routing or installer changes.

## License

Hashshashin is released under the [MIT License](LICENSE).

---

<div align="center">

**Hashshashin — route the data plane, keep control of the path.**

[English documentation](README_EN.md) · [مستندات فارسی](README_FA.md)

</div>
