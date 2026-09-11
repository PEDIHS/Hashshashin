# Changelog

## v0.3.0-alpha — Data Plane Performance & Reproducibility

- Reworked Linux TUN from a single file descriptor to `IFF_MULTI_QUEUE` with automatic fallback to single-queue kernels.
- Added parallel TUN egress pumps so packet encryption/carrier writes are no longer forced through one `tun.Read` goroutine.
- Added configurable UDP receive workers for multi-core processing.
- Removed hot-path AES-GCM allocations by pooling encryption buffers and decrypting authenticated payloads in place.
- Removed per-packet UDP peer resolution by caching resolved peer addresses.
- Added carrier socket-buffer tuning for UDP, TCP and KCP.
- Raised the default TUN `txqueuelen` from the kernel-style shallow value to 4096 in the Turbo profile and added `fq_codel` by default.
- Added Balance, Turbo, Throughput and Custom performance profiles.
- Added conservative socket/backlog ceilings without silently enabling BBR or changing the host firewall policy.
- Replaced current-session-only journal counters with cumulative runtime metrics that survive rekey/session replacement.
- Added 10-second data-plane telemetry: data/control packets, Mbit/s, TUN drop counters, send/decrypt/replay/no-session errors and session idle time.
- Added graceful SIGTERM worker shutdown and increased systemd `TimeoutStopSec` to 30 seconds to avoid routine restart SIGKILLs.
- Added `hsh-bench`: persistent iperf3 server, connection-refused retries, warm-up, repeated forward/reverse tests, parallel streams, raw JSON capture and median/min/max/spread reporting.
- Added Manager visibility for requested/actual queue length, TUN queues, qdisc and drop counters, plus direct benchmark launching.
- Added race-tested concurrent encryption checks to prove unique GCM packet counters/nonces under parallel send workers.
- Added explicit security gate issue requiring independent audit and two-host soak testing before stable v1.0.

> v0.3.0 remains alpha. CI validates concurrency and software regressions, but real throughput improvement must be demonstrated with repeated two-VPS benchmarks. Direct Return only removes the downlink from the HSH1 tunnel; it cannot improve a path whose bottleneck is elsewhere (host CPU, conntrack, NIC, provider routing or the direct return link itself).

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
