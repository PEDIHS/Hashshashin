# Hashshashin — Step-by-Step Installation

[← Project home](../README.md)

## 1. Requirements

Both Iran and Kharej servers need:

- Linux
- root access
- TUN/TAP enabled
- IPv4
- systemd
- iproute2
- iptables
- Internet access for downloading and building the source

Check TUN:

```bash
ls -l /dev/net/tun
```

If it is missing, enable TUN/TAP in your VPS provider panel.

## 2. Install Iran

Run:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh)
```

Choose:

```text
1) Iran (entry server)
```

Then choose the mode:

```text
1) Full tunnel
2) Direct return
```

The wizard asks for:

- public interface
- public IPv4
- public gateway
- UDP carrier port
- service ports
- tunnel MTU
- Kharej public IPv4

Recommended starting values:

```text
Carrier Port: 9000
MTU: 1320
Service Ports: 443
```

For multiple service ports:

```text
443,8443,2053
```

At the end, the Iran installer generates a shared key. Save it exactly.

## 3. Install Kharej

Run the same command:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh)
```

Choose:

```text
2) Kharej (service server)
```

Use exactly the same:

- mode
- carrier port
- service-port list
- shared key
- recommended MTU

## 4. Provider firewall

If your cloud provider has an external firewall/security group, allow the UDP carrier between the two servers.

Example:

```text
Protocol: UDP
Port: 9000
Source: IRAN_PUBLIC_IP
Destination: KHAREJ_PUBLIC_IP
```

Prefer restricting the rule to the peer IP rather than exposing the carrier globally.

## 5. Kharej destination service

The destination service, such as Xray/3x-ui, should listen on:

```text
0.0.0.0:443
```

or:

```text
KHAREJ_PUBLIC_IP:443
```

Avoid binding only to:

```text
127.0.0.1:443
```

because packets arriving through `hsh0` are addressed to the Kharej public IP.

## 6. Check the service

On both servers:

```bash
systemctl status hashshashin
```

Logs:

```bash
journalctl -u hashshashin -f
```

TUN interface:

```bash
ip addr show hsh0
```

## 7. Check Iran routing

```bash
ip rule show
ip route show table 166
```

## 8. Check firewall rules

```bash
iptables -t nat -S | grep HSH
iptables -t mangle -S | grep HSH
iptables -S | grep HSH
```

## 9. Verify Direct Return

Iran:

```bash
tcpdump -ni hsh0 host KHAREJ_PUBLIC_IP
```

and:

```bash
tcpdump -ni PUBLIC_INTERFACE host KHAREJ_PUBLIC_IP
```

Kharej:

```bash
tcpdump -ni hsh0 host IRAN_PUBLIC_IP
```

and:

```bash
tcpdump -ni PUBLIC_INTERFACE host IRAN_PUBLIC_IP
```

In Direct Return mode, upload application packets should traverse `hsh0`, while download application packets should primarily appear on the public interfaces.

## 10. Direct Return requirement

On Iran:

```bash
ip -4 addr show
```

The Iran public IPv4 must actually be assigned to the host interface. If the server only has upstream NAT/CGNAT, use Full Tunnel.

## 11. MTU tuning

Configuration file:

```text
/etc/hashshashin/config.json
```

After changing MTU:

```bash
systemctl restart hashshashin
```

Useful values to test:

```text
1320
1280
1240
```

## 12. Validate configuration

```bash
hashshashin -check -c /etc/hashshashin/config.json
```

## 13. Restart

```bash
systemctl restart hashshashin
```

## 14. Cleanup networking state

```bash
hashshashin -cleanup -c /etc/hashshashin/config.json
```

## 15. Uninstall

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh) --uninstall
```

The configuration is preserved by default at:

```text
/etc/hashshashin/config.json
```

## More documentation

- [English guide](../README_EN.md)
- [Architecture](ARCHITECTURE.md)
- [Security](../SECURITY.md)
- [Persian verification guide](VERIFY_FA.md)
