# Changelog

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
