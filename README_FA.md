# حشاشین — راهنمای کامل فارسی

[← صفحه اصلی](README.md) · [English](README_EN.md) · [Transportها](docs/TRANSPORTS_FA.md) · [Smart Return](docs/SMART_RETURN_FA.md)

## معرفی

**Hashshashin / حشاشین** یک تونل مستقل **Layer 3 / TUN** برای Linux است که packetهای کامل IPv4 را از طریق interface مجازی `hsh0` حمل می‌کند.

نسخه فعلی: **v0.2.0-alpha**

سه حالت عملیاتی اصلی:

1. **Full Tunnel** — آپلود و دانلود هر دو داخل تونل.
2. **Direct Return** — آپلود داخل تونل، دانلود مستقیم Kharej → Iran.
3. **Direct Return + Smart Return** — دانلود Direct است، ولی اگر مسیر Direct خراب شود خودکار به Tunnel fallback می‌کند و بعد از recovery دوباره Direct می‌شود.

کانفیگ کاربر تغییر نمی‌کند؛ endpoint همچنان سرور ایران است.

---

## Data Plane

```text
User protocol
VLESS / VMess / Trojan / Shadowsocks / WireGuard / ...
                         |
                         v
                       hsh0
                         |
                       HSH1
                         |
              UDP / TCP / KCP-FEC
                         |
                  Iran <-> Kharej
```

Hashshashin port proxy نیست؛ packet IP را حمل می‌کند و Linux routing/NAT/conntrack مسیر را کنترل می‌کنند.

---

## Carrierها

| Carrier | وضعیت | توضیح |
|---|---:|---|
| UDP | ✅ | کمترین سربار |
| TCP | ✅ | مناسب مسیرهای محدودکننده UDP |
| KCP | ✅ | ARQ سریع روی UDP |
| KCP + FEC 10/3 | ✅ | preset متعادل |
| KCP + FEC 10/5 | ✅ | تحمل loss بیشتر با سربار بیشتر |
| Raw/Pcap | ⏳ | هنوز پیاده نشده |
| QUIC/WSS | ⏳ | هنوز پیاده نشده |
| Multipath | ⏳ | هنوز پیاده نشده |

تمام Carrierها از Session Layer یکسان استفاده می‌کنند:

- 256-bit PSK
- HMAC-SHA256 handshake
- nonce تصادفی Client/Server
- AES-256-GCM
- کلیدهای جدا TX/RX
- replay window
- keepalive
- reconnect
- periodic rekey

جزئیات: [docs/TRANSPORTS_FA.md](docs/TRANSPORTS_FA.md)

---

## Smart Return

Smart Return فقط در `direct-return` فعال می‌شود.

```text
Healthy direct path
Download = DIRECT

N probe failures
Download = TUNNEL

M probe successes
Download = DIRECT
```

Probe سلامت:

```text
Kharej -> Iran Public IP : UDP/HDP1 + timestamp + nonce + HMAC
Iran -> Kharej           : encrypted HSH1 ACK
```

پیش‌فرض:

```text
probe_port       9001/UDP
interval         5s
timeout          2s
fail_threshold   3
recover_threshold 3
```

Routing جدا:

```text
0x66 / table 166 -> Upload/Data
0x68 / table 168 -> Smart Return service responses
0x77 / table 167 -> Carrier + Probe bypass
```

Smart Return فقط پاسخ Service Portهای تعریف‌شده را تغییر مسیر می‌دهد و main route عمومی Iran IP را تغییر نمی‌دهد.

جزئیات: [docs/SMART_RETURN_FA.md](docs/SMART_RETURN_FA.md)

---

# نصب

روی هر دو سرور:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh)
```

## مراحل Installer

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

### ایران

```text
Role: IRAN / Entry
Mode: Full Tunnel یا Direct Return
Smart Return: اختیاری در Direct Return
Carrier: UDP / TCP / KCP
```

Installer یک Shared Key تولید می‌کند. آن را ذخیره کنید.

### خارج

همان Installer را اجرا کنید و همان موارد زیر را وارد کنید:

- Mode
- Carrier
- Carrier Port
- Service Ports
- Smart Return settings
- Shared Key

---

## Firewall Provider

Carrier:

```text
UDP/KCP -> UDP/<carrier-port>
TCP     -> TCP/<carrier-port>
```

فقط بین IP ایران و خارج باز شود.

Smart Return:

```text
UDP/<probe-port>
Source: Kharej IP
Destination: Iran IP
```

پیش‌فرض probe port برابر `9001` است.

---

## شرط Direct Return

روی ایران Public IP باید واقعاً روی interface سرور باشد:

```bash
ip -4 addr show
```

اگر فقط private IP دارید و Provider با CGNAT/NAT بالادستی Public IP می‌دهد، Direct Return فعلاً پشتیبانی نمی‌شود؛ از Full Tunnel استفاده کنید.

---

## سرویس خارج

مثلاً برای Xray روی 443:

```text
0.0.0.0:443
```

یا bind روی IP عمومی خارج مناسب است.

فقط `127.0.0.1:443` مناسب نیست، چون packet از `hsh0` برای IP عمومی خارج وارد می‌شود.

---

# مدیریت

```bash
hashshashin
```

Manager نمایش می‌دهد:

- Role / Mode
- Carrier
- TUN / MTU
- RX/TX
- Smart Return ON/OFF
- Return Path فعلی `DIRECT` یا `TUNNEL`
- Health Check
- Live Logs
- Diagnostics
- Safe Config
- Update / Reconfigure / Uninstall

دستورات دستی:

```bash
systemctl status hashshashin
journalctl -u hashshashin -f
hashshashin -check -c /etc/hashshashin/config.json
hashshashin -summary -c /etc/hashshashin/config.json
```

Update:

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

## CI

CI روی Go 1.18 و 1.22 شامل:

- `go test -race ./...`
- crypto / replay tests
- UDP integration
- TCP integration
- KCP/FEC integration
- Smart Return HMAC probe tests
- hysteresis tests
- encrypted probe ACK dispatch
- `go vet`
- `go build`
- bash syntax Installer / Manager

سبز بودن CI به معنی تضمین رفتار تمام Providerها نیست؛ Direct Return، MTU، conntrack، firewall و Smart Return باید روی دو VPS واقعی verify شوند.

---

## امنیت

Hashshashin policy کلی Firewall را به ACCEPT تغییر نمی‌دهد و chainهای `HSH_*` ایجاد می‌کند. Shared Key باید محرمانه بماند. HSH1 هنوز audit امنیتی مستقل خارجی نشده است.

[SECURITY.md](SECURITY.md)

---

## Roadmap

- [x] L3/TUN
- [x] Full Tunnel
- [x] Direct Return
- [x] UDP/TCP/KCP-FEC
- [x] Smart Return protocol
- [x] Direct → Tunnel → Direct failover
- [ ] Raw/Pcap
- [ ] QUIC/WSS
- [ ] Multipath
- [ ] Carrier auto-failover
- [ ] IPv6
- [ ] Binary packages
- [ ] External audit
- [ ] Multi-provider benchmarks

---

Hashshashin به‌صورت مستقل پیاده‌سازی شده است. ایده‌های عمومی پروژه‌های دیگر مطالعه شده‌اند، اما Core، HSH1، Routing Controller و Smart Return کپی سورس آن‌ها نیستند.
