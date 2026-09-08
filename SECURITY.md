# Security Policy

Hashshashin transports network traffic and runs with `CAP_NET_ADMIN`; treat configuration and release integrity as security-sensitive.

## v0.1 security properties

- 256-bit pre-shared key
- HMAC-SHA256 authenticated handshake
- fresh random client/server nonces per session
- separate AES-256-GCM keys for each direction
- authenticated packet counters
- 64-packet replay window
- periodic rekey
- dedicated firewall chains and policy-routing tables
- systemd capability bounding to `CAP_NET_ADMIN`

## Important limitations

The Hashshashin wire protocol has not yet undergone an independent cryptographic or implementation audit. Do not describe it as formally audited or proven secure.

Protect `/etc/hashshashin/config.json` because it contains the shared key. The installer sets mode `0600`.

For the UDP carrier, restrict provider-level firewall/security-group access to the known peer IP whenever possible.

## Reporting

Please open a private security report through GitHub Security Advisories when available. Avoid publishing working secrets, private server addresses or production configuration files in public issues.
