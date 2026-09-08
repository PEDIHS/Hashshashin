# Hashshashin Architecture

## Design goals

Hashshashin separates three concerns:

1. **L3 data plane** — Linux TUN interface (`hsh0`) carries complete IPv4 packets.
2. **Carrier** — v0.1 transports those packets over authenticated UDP.
3. **Routing policy** — Linux policy routing and conntrack decide whether the return direction uses `hsh0` or the normal public interface.

This separation allows additional carriers to be implemented later without changing the L3 routing model.

## Packet model

The Iran node receives a user connection on the Iran public service port. The packet is marked, DNATed to the Kharej public service address and SNATed to the Iran public address. A dedicated policy-routing table sends only the marked flow into `hsh0`.

The packet injected into the Kharej TUN therefore has approximately this tuple:

```text
src = IRAN_PUBLIC_IP:nat_port
dst = KHAREJ_PUBLIC_IP:service_port
```

Because `KHAREJ_PUBLIC_IP` is local to the Kharej host, the kernel delivers it to the local service.

## Direct return

The Kharej host does not install a route for `IRAN_PUBLIC_IP` through `hsh0`. The local service response therefore leaves through the normal public route. When the response reaches Iran, the existing conntrack entry reverse-translates it back to the original client connection.

## Full tunnel

Kharej installs a host route for `IRAN_PUBLIC_IP/32` through `hsh0`, so service responses return through the tunnel. The outer Hashshashin UDP socket is marked `0x77`; an earlier policy rule sends that marked carrier through routing table `167`, which contains the normal public default route. This prevents the outer carrier from recursively entering its own TUN.

## Transport protocol v1

Control messages use magic `HSH1` and authenticated hello/ack messages. Session keys are derived with HMAC-SHA256 from the PSK plus fresh client and server nonces. Separate labels derive client-to-server and server-to-client AES-256-GCM keys.

Data packets contain a message type and monotonically increasing 64-bit counter. The counter forms the per-direction nonce component. A 64-packet replay window accepts valid out-of-order packets while rejecting duplicates and packets older than the active window.

## Rekey and liveness

Both peers exchange encrypted keepalives. Iran initiates a new handshake when:

- no session exists,
- the session has been idle beyond the configured timeout,
- or the configured rekey interval expires.

Kharej expires stale sessions and waits for the next authenticated Iran hello.

## Security boundaries

Hashshashin modifies only dedicated iptables chains and dedicated policy routing tables. It does not set the host's general INPUT/FORWARD policies to ACCEPT. The installer can optionally drop direct public access to configured Kharej service ports.
