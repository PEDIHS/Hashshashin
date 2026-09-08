<div align="center">

# حشاشین | Hashshashin

### تونل L3 مستقل برای Linux با Full Tunnel و Direct Return

**آپلود از تونل، دانلود مستقیم از خارج — بدون تغییر کانفیگ کاربر**

[![CI](https://github.com/PEDIHS/Hashshashin/actions/workflows/ci.yml/badge.svg)](https://github.com/PEDIHS/Hashshashin/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.18%2B-00ADD8?logo=go&logoColor=white)
![Linux](https://img.shields.io/badge/Platform-Linux-FCC624?logo=linux&logoColor=black)
![License](https://img.shields.io/badge/License-MIT-green.svg)
![Release](https://img.shields.io/badge/Release-v0.1.0%20Beta-blue)

[راهنمای کامل فارسی](README_FA.md) · [English](README_EN.md) · [معماری](docs/ARCHITECTURE.md) · [امنیت](SECURITY.md) · [تغییرات نسخه‌ها](CHANGELOG.md)

</div>

---

## حشاشین چیست؟

**Hashshashin** یک تونل مستقل **Layer 3** برای Linux است که packetهای کامل IPv4 را از طریق interface نوع TUN با نام `hsh0` منتقل می‌کند.

این پروژه به‌جای اینکه مثل یک port forwarder معمولی اتصال کاربر را terminate کند و در سمت خارج اتصال جدید بسازد، در سطح packet کار می‌کند. همین معماری امکان انتخاب مستقل مسیر رفت و برگشت را فراهم می‌کند.

### دو حالت اصلی

| حالت | آپلود | دانلود | Endpoint کاربر |
|---|---|---|---|
| **Full Tunnel** | از تونل | از تونل | سرور ایران |
| **Direct Return** | از تونل | مستقیم خارج → ایران | سرور ایران |

در حالت **Direct Return** کاربر همچنان به همان IP و Port ایران متصل می‌شود؛ اما مسیر دانلود می‌تواند از سرور خارج مستقیماً از اینترنت عادی به ایران برگردد و داخل carrier تونل عبور نکند.

> **وضعیت پروژه:** نسخه `v0.1.0` یک **Official Beta** است. Core، رمزنگاری، reconnect/rekey، installer، build/vet و integration test داخل CI بررسی می‌شوند. رفتار TUN، policy routing و Direct Return همچنان به شبکه و Provider واقعی وابسته است و باید روی دو VPS واقعی verify شود.

---

## چرا Hashshashin؟

هدف پروژه ساخت یک تونل صرفاً «دیگر» نیست. طراحی Hashshashin حول جداسازی **Data Plane** از **Carrier** انجام شده تا مسیر هر جهت بتواند مستقل مدیریت شود.

مهم‌ترین تفاوت:

```text
روش معمول
User -> Iran -> Tunnel -> Kharej
User <- Iran <- Tunnel <- Kharej

Hashshashin Direct Return
User -> Iran -> Tunnel -> Kharej
User <- Iran <---------- Kharej
                 Direct
```

این یعنی در شبکه‌ای که مسیر مستقیم برگشت کیفیت بهتری دارد، دانلود مجبور نیست همان مسیر تونلی Upload را برگردد.

---

## ویژگی‌های اصلی

- **L3/TUN واقعی** — حمل packet کامل IPv4 به‌جای proxy کردن sessionهای کاربر
- **Full Tunnel** — مسیر رفت و برگشت داخل تونل
- **Direct Return** — Upload داخل تونل و Download مستقیم از خارج
- **بدون تغییر کانفیگ کاربر** — endpoint کاربر همان سرور ایران می‌ماند
- **UDP Carrier احراز هویت‌شده و رمزنگاری‌شده**
- **Shared Key با طول 256 بیت**
- **HMAC-SHA256 handshake**
- **AES-256-GCM payload encryption**
- **کلیدهای مستقل TX و RX**
- **nonce تصادفی Client/Server برای هر Session**
- **Replay Protection با window 64 packet**
- **تحمل UDP packet reordering**
- **Keepalive و Reconnect خودکار**
- **Rekey دوره‌ای**
- **Policy Routing مجزا برای Data و Carrier**
- **جلوگیری از Recursive Tunnel Loop**
- **MTU قابل تنظیم**
- **TCP MSS Clamp جهت‌دار**
- **Chainهای اختصاصی iptables**
- **عدم تغییر Global Firewall Policy به ACCEPT**
- **Installer یک‌خطی**
- **systemd service**
- **Cleanup و Uninstall امن**
- **CI روی Go 1.18 و Go 1.22**

---

## معماری

### Full Tunnel

```text
                         Hashshashin encrypted carrier
Client -> Iran -> hsh0  ==============================>  hsh0 -> Kharej Service
Client <- Iran <- hsh0  <==============================  hsh0 <- Kharej Service
```

در این حالت هر دو جهت از تونل عبور می‌کنند.

### Direct Return

```text
UPLOAD
Client -> Iran -> hsh0  ==============================>  hsh0 -> Kharej Service

DOWNLOAD
Client <- Iran  <--------------- Internet ---------------- Kharej Service
                                Direct
```

روی ایران، `conntrack` و NAT وضعیت اتصال را نگه می‌دارند و packet برگشتی مستقیم را به همان connection کاربر map می‌کنند؛ بنابراین کاربر همچنان IP ایران را به‌عنوان endpoint می‌بیند.

جزئیات دقیق markها، tableها، TUN و NAT در این فایل آمده است:

**[مشاهده معماری کامل](docs/ARCHITECTURE.md)**

---

# نصب سریع

روی **هر دو سرور ایران و خارج** همین دستور را اجرا کنید:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh)
```

## ترتیب نصب پیشنهادی

### 1. ابتدا سرور ایران

در Wizard انتخاب کنید:

```text
1) Iran (entry server)
```

سپس Mode:

```text
1) Full tunnel
2) Direct return
```

Installer به‌صورت مرحله‌ای موارد زیر را دریافت می‌کند:

- Public Interface
- Public IPv4
- Gateway
- UDP Carrier Port
- Service Ports
- Tunnel MTU
- IP سرور خارج

بعد یک **Shared Key** ایجاد می‌کند.

```text
Shared key — copy this exact value to the Kharej installer:
xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

این کلید را ذخیره کنید.

### 2. سپس سرور خارج

دوباره همان دستور نصب را اجرا کنید:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh)
```

این بار انتخاب کنید:

```text
2) Kharej (service server)
```

موارد زیر باید با ایران یکسان باشند:

- Mode
- Carrier Port
- Service Ports
- Shared Key

### 3. باز کردن Carrier در Firewall Provider

اگر Cloud Firewall یا Security Group دارید، UDP Carrier را **فقط بین IP ایران و IP خارج** باز کنید.

پیش‌فرض:

```text
Protocol: UDP
Port: 9000
Iran Public IP <-> Kharej Public IP
```

### 4. تنظیم Service روی خارج

مثلاً اگر Xray / 3x-ui روی پورت `443` است، listener باید روی یکی از این‌ها باشد:

```text
0.0.0.0:443
```

یا:

```text
KHAREJ_PUBLIC_IP:443
```

این حالت مناسب نیست:

```text
127.0.0.1:443
```

چون packet ورودی از `hsh0` برای IP عمومی خارج destination می‌شود.

برای آموزش کامل نصب:

**[نصب مرحله‌به‌مرحله فارسی](docs/INSTALL_FA.md)**

**[Step-by-step English Installation](docs/INSTALL_EN.md)**

---

## شرط مهم Direct Return

در حالت Direct Return، IP عمومی ایران باید **واقعاً روی interface سرور ایران assign شده باشد**.

بررسی:

```bash
ip -4 addr show
```

اگر سرور فقط IP خصوصی دارد و Public IP توسط NAT/CGNAT بالادستی ارائه می‌شود، Direct Return در نسخه فعلی پشتیبانی نمی‌شود.

در این شرایط از **Full Tunnel** استفاده کنید.

---

## Data Plane و جلوگیری از Loop

Hashshashin برای جلوگیری از loop بین carrier و tunnel دو routing domain جدا دارد.

### User/Data Traffic

```text
fwmark: 0x66
routing table: 166
```

### Outer Carrier

```text
fwmark: 0x77
routing table: 167
```

در Full Tunnel اگر route برگشت user traffic به `hsh0` منتقل شود، carrier با mark جدا همچنان از interface اصلی سرور خارج می‌شود و داخل تونل خودش loop نمی‌زند.

---

## MTU و MSS

مقدار پیش‌فرض MTU:

```text
1320
```

Installer بازه زیر را می‌پذیرد:

```text
900 - 1400
```

برای شروع `1320` پیشنهاد می‌شود.

در صورت مشاهده fragmentation، stall یا رفتار نامناسب مسیر می‌توانید این مقادیر را تست کنید:

```text
1280
1240
```

Hashshashin برای TCP از MSS Clamp استفاده می‌کند و در Direct Return این clamp به‌صورت جهت‌دار طراحی شده تا مسیر Download مستقیم بی‌دلیل به MTU مسیر Upload محدود نشود.

---

## امنیت Transport

Transport نسخه فعلی شامل موارد زیر است:

```text
256-bit PSK
HMAC-SHA256 authenticated handshake
Fresh client/server nonces
AES-256-GCM
Separate TX/RX keys
Packet counters
Replay window
Periodic rekey
Keepalive
Reconnect
```

> پروتکل هنوز Audit امنیتی مستقل خارجی نشده است. برای استفاده حساس یا deployment بزرگ، مطالعه `SECURITY.md` و review مستقل توصیه می‌شود.

**[Security Policy](SECURITY.md)**

---

## Firewall

Hashshashin از chainهای اختصاصی استفاده می‌کند و policy اصلی فایروال سیستم را به `ACCEPT` تغییر نمی‌دهد.

نمونه chainها:

```text
HSH_MPRE
HSH_NPRE
HSH_NPOST
HSH_FWD
HSH_INPUT
HSH_OUTPUT
HSH_MFWD
HSH_MOUT
```

در سمت خارج Installer می‌تواند دسترسی مستقیم Public به Service Portها را محدود کند.

---

## دستورات مدیریت

### وضعیت سرویس

```bash
systemctl status hashshashin
```

### لاگ زنده

```bash
journalctl -u hashshashin -f
```

### Restart

```bash
systemctl restart hashshashin
```

### بررسی Config

```bash
hashshashin -check -c /etc/hashshashin/config.json
```

### نمایش نسخه

```bash
hashshashin -version
```

### پاک‌سازی Ruleهای شبکه

```bash
hashshashin -cleanup -c /etc/hashshashin/config.json
```

### Uninstall

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh) --uninstall
```

فایل زیر هنگام uninstall نگه داشته می‌شود تا Shared Key و تنظیمات تصادفی از بین نرود:

```text
/etc/hashshashin/config.json
```

---

## تست صحت نصب

روی ایران:

```bash
ip addr show hsh0
ip rule show
ip route show table 166
iptables -t nat -S | grep HSH
journalctl -u hashshashin -n 100 --no-pager
```

روی خارج:

```bash
ip addr show hsh0
ss -lntup
journalctl -u hashshashin -n 100 --no-pager
```

در Direct Return انتظار کلی این است:

```text
Iran hsh0       -> Upload application packets
Kharej hsh0     -> Upload application packets
Kharej Public   -> Download application packets
Iran Public     -> Download application packets
```

**[راهنمای Verify روی دو VPS](docs/VERIFY_FA.md)**

---

## CI و تست‌های پروژه

GitHub Actions روی Go `1.18` و `1.22` این موارد را اجرا می‌کند:

- Unit Tests
- Race Detector
- UDP Transport Integration Test
- Authenticated Handshake Test
- Directional Key Validation
- Encrypted Payload Delivery Test
- `go vet`
- `go build`
- Shell Syntax Check برای Installer

سبز بودن CI تضمین‌کننده رفتار همه دیتاسنترها نیست؛ چون TUN، route، firewall و Direct Return به infrastructure واقعی وابسته‌اند.

---

## Roadmap

- [x] L3 / TUN Core
- [x] Full Tunnel
- [x] Direct Return
- [x] UDP Encrypted Carrier
- [x] Keepalive
- [x] Reconnect
- [x] Periodic Rekey
- [x] Replay Protection
- [x] Data/Carrier Routing Isolation
- [x] Installer یک‌خطی
- [x] systemd integration
- [x] CI چندنسخه Go
- [ ] Direct Return Health Monitor
- [ ] Automatic Direct → Tunnel Failover
- [ ] Multipath
- [ ] Raw/KCP-style Carrier
- [ ] IPv6
- [ ] Binary Releases
- [ ] Debian/RPM Packages
- [ ] External Security Audit
- [ ] Multi-provider Benchmark

---

## محدودیت‌های نسخه فعلی

- IPv6 هنوز پیاده‌سازی نشده است.
- Carrier فعلی UDP است.
- Raw/KCP هنوز اضافه نشده است.
- Multipath هنوز اضافه نشده است.
- Failover خودکار Direct → Tunnel هنوز فعال نیست.
- Direct Return روی ایران پشت CGNAT/NAT بالادستی پشتیبانی نمی‌شود.
- رفتار Providerها می‌تواند متفاوت باشد.

---

## مستندات پروژه

| مستند | توضیح |
|---|---|
| [README_FA.md](README_FA.md) | راهنمای کامل فارسی |
| [README_EN.md](README_EN.md) | راهنمای کامل انگلیسی |
| [docs/INSTALL_FA.md](docs/INSTALL_FA.md) | نصب مرحله‌به‌مرحله فارسی |
| [docs/INSTALL_EN.md](docs/INSTALL_EN.md) | نصب مرحله‌به‌مرحله انگلیسی |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | معماری L3، TUN، NAT و Routing |
| [docs/VERIFY_FA.md](docs/VERIFY_FA.md) | تست مسیر واقعی دو سرور |
| [docs/TROUBLESHOOTING_FA.md](docs/TROUBLESHOOTING_FA.md) | عیب‌یابی فارسی |
| [SECURITY.md](SECURITY.md) | سیاست امنیت و نکات Deployment |
| [CHANGELOG.md](CHANGELOG.md) | تاریخچه تغییرات |
| [CONTRIBUTING.md](CONTRIBUTING.md) | راهنمای مشارکت |

---

## استقلال پروژه

Hashshashin کپی سورس **Paqet، Backhaul یا BackPack** نیست.

ایده‌های عمومی معماری مانند:

- Packet Transport
- TUN / L3
- Data Plane / Carrier Separation
- Routing Isolation

به‌عنوان مرجع فنی بررسی شده‌اند، اما Core و Wire Protocol این پروژه به‌صورت مستقل پیاده‌سازی شده‌اند.

---

## مشارکت در توسعه

Issue و Pull Request برای بهبود Core، Routing، Transport، Installer و Documentation پذیرفته می‌شود.

قبل از تغییر بخش‌های حساس پروتکل یا Routing، این فایل را مطالعه کنید:

**[CONTRIBUTING.md](CONTRIBUTING.md)**

---

## License

Hashshashin تحت مجوز **MIT** منتشر شده است.

[مشاهده LICENSE](LICENSE)

---

<div align="center">

### حشاشین

**کنترل مسیر Data Plane، بدون وابستگی به مسیر برگشت تونل**

[راهنمای فارسی](README_FA.md) · [English Documentation](README_EN.md)

</div>
