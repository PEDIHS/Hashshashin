# Changelog

## v0.2.0-alpha — Multi-Transport & Smart Return

- Added pluggable carrier abstraction below HSH1.
- Added framed TCP carrier with reconnect, TCP_NODELAY and keepalive.
- Added KCP carrier with optional FEC and installer presets: 10/3, no FEC and 10/5.
- Added KCP integration through `github.com/xtaci/kcp-go/v5` (MIT).
- Added transport-aware installer, firewall rules, management dashboard and health checks.
- Added Smart Return for Direct Return deployments.
- Added authenticated `HDP1` direct-path probes using timestamp, random nonce and HMAC-SHA256.
- Added encrypted HSH1 probe acknowledgements.
- Added failure/recovery hysteresis for automatic Direct → Tunnel → Direct return switching.
- Added service-scoped Smart Return policy routing using mark `0x68` and table `168`.
- Kept user upload routing on `0x66/166` and carrier/probe bypass on `0x77/167`.
- Avoided broad main-table Iran routes during Smart Return fallback so unrelated management traffic is not redirected.
- Fixed TCP carrier host-firewall behavior for Iran dialer vs Kharej listener roles.
- Added Smart Return HMAC, hysteresis, validation and encrypted ACK tests.
- Preserved UDP/TCP/KCP-FEC integration tests on Go 1.18 and Go 1.22.

> v0.2.0 remains an alpha release until Direct Return and Smart Return have been validated across real Iran/Kharej providers and external security review has been completed.

## v0.1.0 — Initial Official Release

- Independent Linux L3/TUN core.
- Full Tunnel mode.
- Upload Tunnel / Direct Return mode.
- Authenticated encrypted UDP carrier.
- Automatic reconnect, keepalive and periodic rekey.
- 64-packet replay window with out-of-order tolerance.
- Directional AES-256-GCM keys.
- Dedicated data/carrier policy-routing tables to prevent recursive carrier loops.
- Direction-aware TCP MSS clamping.
- Dedicated iptables chains with cleanup/rollback.
- Interactive one-line installer and systemd service.
- Transport handshake/data integration test and CI on Go 1.18 and Go 1.22.
