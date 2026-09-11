<div align="center">

# حشاشین | Hashshashin

### تونل مستقل L3 برای Linux با Multi-Queue Data Plane، Direct Return و Smart Failover

**آپلود از تونل؛ دانلود مستقیم از خارج؛ و در خرابی مسیر Direct، بازگشت خودکار دانلود به Tunnel**

[![CI](https://github.com/PEDIHS/Hashshashin/actions/workflows/ci.yml/badge.svg)](https://github.com/PEDIHS/Hashshashin/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.18%2B-00ADD8?logo=go&logoColor=white)
![Linux](https://img.shields.io/badge/Platform-Linux-FCC624?logo=linux&logoColor=black)
![Transport](https://img.shields.io/badge/Carrier-UDP%20%7C%20TCP%20%7C%20KCP-blue)
![License](https://img.shields.io/badge/License-MIT-green.svg)
![Status](https://img.shields.io/badge/Status-v0.3%20Alpha-orange)

[راهنمای فارسی](README_FA.md) · [English](README_EN.md) · [عملکرد v0.3](docs/PERFORMANCE_FA.md) · [Transportها](docs/TRANSPORTS_FA.md) · [Smart Return](docs/SMART_RETURN_FA.md) · [معماری](docs/ARCHITECTURE.md) · [امنیت](SECURITY.md)

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
                 Linux L3/TUN Multi-Queue
                  /      |      |      \
               queue0  queue1 queue2  queue3
                  \      |      |      /
                         HSH1
     HMAC handshake + AES-256-GCM + Replay Protection
                         |
                Transport Manager
                  /      |      \
                UDP     TCP     KCP/FEC
                         |
                   Iran <-> Kharej
```

> **وضعیت:** نسخه فعلی `v0.3.0-alpha` است. Core، Crypto، concurrency و Carrierها در CI تست می‌شوند، اما افزایش throughput و رفتار shutdown باید قبل از Production روی دو VPS و Provider واقعی با benchmark/soak test تأیید شوند. پروژه هنوز audit امنیتی مستقل نشده است.

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

## Data Plane v0.3

v0.2 یک حلقه اصلی سری داشت:

```text
hsh0 -> tun.Read -> AES-GCM -> carrier.Write
```

v0.3 این مسیر را برای Linux چندصفه می‌کند:

```text
              hsh0 / IFF_MULTI_QUEUE
        ┌────────┼────────┬────────┐
      queue0   queue1   queue2   queue3
        │        │        │        │
      pump     pump     pump     pump
        └────────┴──── HSH1 ───────┘
                       │
                     Carrier
```

بهینه‌سازی‌های اصلی:

- `IFF_MULTI_QUEUE` با fallback خودکار به single queue در kernel ناسازگار
- چند TUN pump مستقل برای استفاده از چند CPU
- چند UDP receive worker
- AES-GCM buffer pool و decrypt in-place برای کاهش allocation/copy هر packet
- cache شدن UDP peer address
- socket buffer tuning برای UDP/TCP/KCP
- `txqueuelen=4096` و `fq_codel` در پروفایل Turbo
- آمار تجمعی که با rekey صفر نمی‌شود
- نمایش `hsh0 tx_dropped/rx_dropped`
- shutdown مبتنی بر SIGTERM/context به‌جای اتکا به SIGKILL

Performance Profileها:

| Profile | TUN Queue | qlen | Socket Buffer | کاربرد |
|---|---:|---:|---:|---|
| Balance | تا 2 | 2048 | 4 MiB | VPS کوچک/اشتراکی |
| **Turbo** | تا 4 | 4096 | 8 MiB | پیش‌فرض عمومی |
| Throughput | تا 8 | 8192 | 16 MiB | Bulk throughput |
| Custom | دستی | دستی | دستی | تست کنترل‌شده |

> بزرگ‌تر کردن queue به‌تنهایی performance fix نیست. `fq_codel` برای کنترل bufferbloat کنار queue عمیق‌تر استفاده می‌شود.

**[راهنمای Performance و روش benchmark](docs/PERFORMANCE_FA.md)**

---

## Carrierهای واقعی

| Carrier | Underlay | وضعیت | کاربرد |
|---|---|---:|---|
| **UDP** | UDP | ✅ | کمترین سربار و latency؛ انتخاب اول روی لینک تمیز |
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
[5/8] Carrier & performance profile
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

و Performance Profile:

```text
1) Turbo
2) Balance
3) Throughput
4) Custom
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
- Performance Profile
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

برای benchmark نیز port انتخابی `iperf3` (پیش‌فرض `39001/TCP`) را فقط موقتاً بین IP دو سرور اجازه دهید.

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

Direct Return فقط downlink HSH1 را حذف می‌کند. اگر bottleneck روی CPU/NIC/conntrack ایران، provider path یا خود مسیر مستقیم باشد، الزاماً bandwidth را بیشتر نمی‌کند. برای مقایسه Full و Direct از benchmark تکراری استفاده کنید، نه یک run منفرد.

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
- Performance Profile
- TUN queue count / txqueuelen / qdisc
- TUN RX/TX و drop counters
- Smart Return ON/OFF
- مسیر فعلی `DIRECT` یا `TUNNEL`
- Health Check
- Live Logs
- Safe Config View
- Reconfigure
- Update
- Network Diagnostics
- Reproducible Benchmark
- Reset Network State
- Uninstall

Benchmark مستقیم:

```bash
# روی مقصد
hsh-bench server 39001

# روی سمت تست‌کننده: 5 run، هر run با 8 stream موازی
hsh-bench client PEER_IP 39001 5 8
```

خروجی شامل raw JSON، median/min/max/stdev/spread و delta مربوط به TUN dropهاست.

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
- concurrent encryption test با 8 worker و بررسی uniqueness counter/nonce
- cumulative metrics test across session replacement/rekey
- UDP handshake + encrypted payload integration
- TCP framed carrier integration
- KCP/FEC integration
- Smart Return probe authentication
- Smart Return hysteresis
- encrypted probe ACK dispatch
- `go vet`
- `go build`
- Installer/Manager/Benchmark shell syntax

CI جای تست infrastructure واقعی را نمی‌گیرد. برای Production باید TUN multi-queue، conntrack/NAT، MTU، firewall، graceful restart و مسیر Direct روی VPS واقعی benchmark/soak شوند.

---

## امنیت

Hashshashin policy اصلی Firewall سیستم را به `ACCEPT` تغییر نمی‌دهد و chainهای اختصاصی `HSH_*` ایجاد می‌کند. Shared Key را منتشر نکنید و SSH / پنل مدیریت / دیتابیس را جداگانه محدود کنید.

پروتکل HSH1 و پیاده‌سازی multi-queue هنوز audit امنیتی مستقل خارجی نشده‌اند. Issue امنیتی پروژه audit و two-host soak را gate قبل از Stable v1.0 قرار می‌دهد.

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
- [x] Linux TUN Multi-Queue Data Plane
- [x] Queue/qdisc Performance Profiles
- [x] Cumulative Data Plane Telemetry
- [x] Reproducible Multi-Stream Benchmark Harness
- [ ] Raw TCP / pcap carrier
- [ ] QUIC / WSS carrier
- [ ] Multipath / multi-carrier bonding
- [ ] Automatic carrier failover
- [ ] IPv6
- [ ] Binary Releases
- [ ] Debian/RPM Packages
- [ ] External Security Audit
- [ ] Multi-provider Benchmark / Soak Test

---

## استقلال پروژه

Hashshashin کپی سورس Paqet، Backhaul یا BackPack نیست. ایده‌های معماری عمومی آن‌ها مطالعه شده‌اند، اما Core، HSH1، Routing Control، Installer، Smart Return و Data Plane v0.3 مستقل پیاده‌سازی شده‌اند. Dependency خارجی KCP نیز به‌صورت شفاف از `xtaci/kcp-go` تحت مجوز MIT استفاده می‌شود.

---

<div align="center">

### Hashshashin

**L3 Data Plane · Multi-Queue · Direct Return · Smart Failover**

[راهنمای فارسی](README_FA.md) · [English](README_EN.md) · [Performance](docs/PERFORMANCE_FA.md) · [Smart Return](docs/SMART_RETURN_FA.md)

</div>
