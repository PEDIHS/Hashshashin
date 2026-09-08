<div align="center">

# حشاشین | Hashshashin

### تونل مستقل L3 برای Linux با Multi-Transport، Direct Return و Smart Failover

**آپلود از تونل؛ دانلود مستقیم از خارج؛ و در خرابی مسیر Direct، بازگشت خودکار دانلود به Tunnel**

[![CI](https://github.com/PEDIHS/Hashshashin/actions/workflows/ci.yml/badge.svg)](https://github.com/PEDIHS/Hashshashin/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.18%2B-00ADD8?logo=go&logoColor=white)
![Linux](https://img.shields.io/badge/Platform-Linux-FCC624?logo=linux&logoColor=black)
![Transport](https://img.shields.io/badge/Carrier-UDP%20%7C%20TCP%20%7C%20KCP-blue)
![License](https://img.shields.io/badge/License-MIT-green.svg)
![Status](https://img.shields.io/badge/Status-v0.2%20Alpha-orange)

[راهنمای فارسی](README_FA.md) · [English](README_EN.md) · [Transportها](docs/TRANSPORTS_FA.md) · [Smart Return](docs/SMART_RETURN_FA.md) · [معماری](docs/ARCHITECTURE.md) · [امنیت](SECURITY.md)

</div>

---

## حشاشین چیست؟

**Hashshashin** یک Data Plane مستقل **Layer 3 / TUN** برای Linux است. به‌جای terminate کردن session کاربر و ساختن یک اتصال جدید در خارج، packet کامل IPv4 را از interface مجازی `hsh0` حمل می‌کند.

این معماری اجازه می‌دهد مسیر Upload و Download مستقل کنترل شوند و Carrier زیرین نیز بدون تغییر هسته L3 قابل تعویض باشد.

```text
User Protocol
VLESS / VMess / Trojan / Shadowsocks / WireGuard / ...
                         |
                         v
                    Linux L3/TUN
                         |
                       HSH1
     HMAC handshake + AES-256-GCM + Replay Protection
                         |
                Transport Manager
                  /      |      \
                UDP     TCP     KCP/FEC
                         |
                   Iran <-> Kharej
```

> **وضعیت:** نسخه فعلی `v0.2.0-alpha` است. Core، Crypto و Carrierها در CI تست می‌شوند، اما Direct Return و Smart Return باید قبل از Production روی دو VPS و Provider واقعی verify شوند.

---

## حالت‌های مسیر

| Mode | Upload | Download | Auto Failover |
|---|---|---|---|
| **Full Tunnel** | Tunnel | Tunnel | — |
| **Direct Return** | Tunnel | Direct Kharej → Iran | خاموش |
| **Direct Return + Smart Return** | Tunnel | Direct، با fallback به Tunnel | ✅ |

### Full Tunnel

```text
Client -> Iran -> hsh0 ==================> hsh0 -> Kharej
Client <- Iran <- hsh0 <================== hsh0 <- Kharej
```

### Direct Return

```text
UPLOAD
Client -> Iran -> hsh0 ==================> Kharej

DOWNLOAD
Client <- Iran <--------- Internet -------- Kharej
                         Direct
```

### Smart Return

```text
Normal       : Download = DIRECT
Direct fails : Download = TUNNEL
Direct heals : Download = DIRECT
```

Smart Return برای جلوگیری از flapping از failure/recovery threshold استفاده می‌کند و فقط traffic مربوط به Service Portها را تغییر مسیر می‌دهد؛ SSH یا traffic مدیریتی دیگر به‌صورت عمومی وارد fallback نمی‌شوند.

**[جزئیات Smart Return](docs/SMART_RETURN_FA.md)**

---

## Carrierهای واقعی

| Carrier | Underlay | وضعیت | کاربرد |
|---|---|---:|---|
| **UDP** | UDP | ✅ | کمترین سربار و latency |
| **TCP** | framed TCP stream | ✅ | شبکه‌های محدودکننده UDP |
| **KCP** | KCP over UDP | ✅ | loss/jitter و ARQ سریع |
| **KCP + FEC 10/3** | KCP/FEC | ✅ | preset متعادل |
| **KCP + FEC 10/5** | KCP/FEC | ✅ | loss بالاتر، سربار بیشتر |
| Raw/Pcap | — | ⏳ Roadmap | هنوز قابلیت فعال نیست |
| QUIC/WSS | — | ⏳ Roadmap | هنوز قابلیت فعال نیست |
| Multipath | — | ⏳ Roadmap | هنوز قابلیت فعال نیست |

Carrier فقط outer transport است. امنیت Session Layer در همه Carrierها یکسان می‌ماند:

```text
HSH1
├── 256-bit PSK
├── HMAC-SHA256 authenticated handshake
├── fresh client/server nonces
├── AES-256-GCM
├── independent TX/RX keys
├── replay window
├── keepalive
├── reconnect
└── periodic rekey
```

**[راهنمای کامل Transportها](docs/TRANSPORTS_FA.md)**

---

# نصب سریع

روی **هر دو سرور ایران و خارج**:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh)
```

ترتیب پیشنهادی: ابتدا ایران، سپس خارج.

## Wizard نصب

Installer مراحل زیر را انجام می‌دهد:

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

### 1) ایران

Role:

```text
1) IRAN / Entry
```

Mode:

```text
1) Full Tunnel
2) Direct Return
```

اگر Direct Return انتخاب شود، Installer می‌پرسد:

```text
Enable automatic Direct → Tunnel → Direct failover? [Y]
```

سپس Carrier:

```text
1) UDP
2) TCP
3) KCP/FEC
```

برای KCP:

```text
1) Balanced FEC 10/3
2) No FEC
3) Strong FEC 10/5
```

در پایان ایران یک Shared Key تولید می‌کند. آن را برای سرور خارج ذخیره کنید.

### 2) خارج

همان دستور نصب را اجرا کنید و انتخاب کنید:

```text
2) KHAREJ / Exit
```

این مقادیر باید با ایران هماهنگ باشند:

- Mode
- Carrier
- Carrier Port
- Service Ports
- Smart Return settings
- Shared Key

---

## Firewall Provider

Carrier را فقط بین IP دو سرور باز کنید:

```text
UDP Carrier / KCP : UDP/<carrier-port>
TCP Carrier       : TCP/<carrier-port>
```

اگر Smart Return فعال است:

```text
Protocol : UDP
Source   : Kharej Public IP
Target   : Iran Public IP
Port     : 9001 (default probe port)
```

Probe Port را برای کل اینترنت باز نکنید.

---

## Xray / 3x-ui / سرویس خارج

اگر Service Port مثلاً `443` است، listener خارج باید packet واردشده از `hsh0` را بپذیرد. معمولاً:

```text
0.0.0.0:443
```

یا IP عمومی خود سرور خارج مناسب است.

Binding فقط به:

```text
127.0.0.1:443
```

برای این topology مناسب نیست.

---

## شرط Direct Return

IP عمومی ایران باید واقعاً روی interface سرور ایران assign شده باشد:

```bash
ip -4 addr show
```

اگر فقط private IP دارید و Public IP توسط CGNAT/NAT بالادستی ارائه می‌شود، Direct Return در نسخه فعلی پشتیبانی نمی‌شود. در آن شرایط از Full Tunnel استفاده کنید.

---

## Routing Isolation

Hashshashin سه domain مستقل دارد:

```text
0x66 / table 166  -> User Upload Data
0x68 / table 168  -> Smart Return Service Responses
0x77 / table 167  -> Carrier + Direct Health Probe
```

این جداسازی از recursive tunnel loop جلوگیری می‌کند و Smart Return را فقط به flowهای سرویس محدود نگه می‌دارد.

---

# مدیریت

بعد از نصب فقط اجرا کنید:

```bash
hashshashin
```

Dashboard شامل موارد زیر است:

- Service status
- Role / Mode
- Carrier
- TUN / MTU
- RX/TX
- Smart Return ON/OFF
- مسیر فعلی `DIRECT` یا `TUNNEL`
- Health Check
- Live Logs
- Safe Config View
- Reconfigure
- Update
- Network Diagnostics
- Reset Network State
- Uninstall

دستورات مستقیم:

```bash
systemctl status hashshashin
journalctl -u hashshashin -f
hashshashin -check -c /etc/hashshashin/config.json
hashshashin -summary -c /etc/hashshashin/config.json
```

Update بدون حذف Config:

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

## CI و تست‌ها

GitHub Actions روی Go `1.18` و `1.22` اجرا می‌شود و شامل این موارد است:

- Unit Tests
- Race Detector
- HSH1 crypto/session tests
- UDP handshake + encrypted payload integration
- TCP framed carrier integration
- KCP/FEC integration
- Smart Return probe authentication
- Smart Return hysteresis
- encrypted probe ACK dispatch
- `go vet`
- `go build`
- Installer/Manager shell syntax

CI جای تست infrastructure واقعی را نمی‌گیرد. برای Production باید TUN، conntrack/NAT، MTU، firewall و مسیر Direct روی VPS واقعی بررسی شوند.

---

## امنیت

Hashshashin policy اصلی Firewall سیستم را به `ACCEPT` تغییر نمی‌دهد و chainهای اختصاصی `HSH_*` ایجاد می‌کند. Shared Key را منتشر نکنید و SSH / پنل مدیریت / دیتابیس را جداگانه محدود کنید.

پروتکل HSH1 هنوز audit امنیتی مستقل خارجی نشده است.

**[Security Policy](SECURITY.md)**

---

## Roadmap

- [x] L3 / TUN Core
- [x] Full Tunnel
- [x] Direct Return
- [x] UDP Carrier
- [x] TCP Carrier
- [x] KCP/FEC Carrier
- [x] Keepalive / Reconnect / Rekey
- [x] Replay Protection
- [x] Smart Return Health Protocol
- [x] Direct → Tunnel → Direct automatic failover
- [x] Service-scoped return policy routing
- [ ] Raw TCP / pcap carrier
- [ ] QUIC / WSS carrier
- [ ] Multipath / multi-carrier bonding
- [ ] Automatic carrier failover
- [ ] IPv6
- [ ] Binary Releases
- [ ] Debian/RPM Packages
- [ ] External Security Audit
- [ ] Multi-provider Benchmark

---

## استقلال پروژه

Hashshashin کپی سورس Paqet، Backhaul یا BackPack نیست. ایده‌های معماری عمومی آن‌ها مطالعه شده‌اند، اما Core، HSH1، Routing Control، Installer و Smart Return مستقل پیاده‌سازی شده‌اند. Dependency خارجی KCP نیز به‌صورت شفاف از `xtaci/kcp-go` تحت مجوز MIT استفاده می‌شود.

---

<div align="center">

### Hashshashin

**L3 Data Plane · Multi-Transport · Direct Return · Smart Failover**

[راهنمای فارسی](README_FA.md) · [English](README_EN.md) · [Smart Return](docs/SMART_RETURN_FA.md)

</div>
