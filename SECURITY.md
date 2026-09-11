# Security Policy

Hashshashin transports network traffic and runs with `CAP_NET_ADMIN`; configuration, release integrity, routing rules and the HSH1 session layer are security-sensitive.

## Current security properties

- 256-bit pre-shared key
- HMAC-SHA256 authenticated handshake
- fresh random client/server nonces per session
- separate AES-256-GCM keys for each direction
- atomic per-direction packet counters
- 64-packet replay window with limited out-of-order tolerance
- periodic rekey and authenticated keepalive
- dedicated `HSH_*` firewall chains instead of changing the host policy to ACCEPT
- isolated policy-routing marks/tables for user data, carrier bypass and Smart Return
- systemd capability bounding to `CAP_NET_ADMIN`
- config file mode `0600`

## v0.3 data-plane changes

The v0.3 performance work adds concurrent Linux TUN queues and concurrent UDP receive workers. This increases throughput potential but also increases the importance of concurrency testing. CI therefore runs the Go race detector and explicit tests that verify concurrent encryption uses unique packet counters/nonces and that cumulative metrics survive session replacement/rekey.

Multi-queue support is a performance feature, not a security boundary. If the kernel rejects `IFF_MULTI_QUEUE`, Hashshashin falls back to one queue and reports the downgrade.

The installer may raise socket-buffer ceilings and TUN queue length. It does **not** enable BBR, replace the host firewall policy, or grant `CAP_NET_RAW`.

## Audit status

**Hashshashin has not undergone an independent cryptographic, protocol, kernel-integration, or implementation security audit.** The project must remain alpha/beta until an external audit and real two-host soak testing are completed. CI, race tests and integration tests are useful controls but are not a substitute for an independent audit.

Do not describe the project as formally audited, proven secure, or production-hardened.

## Operational guidance

Protect `/etc/hashshashin/config.json` because it contains the shared key. Restrict carrier and Smart Return probe ports at the provider firewall/security group to the known peer IP whenever possible.

Run new versions on staging nodes before production rollout. During load tests, watch `hsh0` drop counters, journal data-plane error counters, CPU saturation, conntrack pressure and host firewall counters. A faster data plane can expose a different bottleneck elsewhere in the host.

## Reporting

Please use GitHub Security Advisories/private reporting when available. Do not publish working secrets, private server addresses, production configs, or exploit details in public issues.
