# حشاشین — راهنمای کامل فارسی

[← صفحه اصلی پروژه](README.md) · [English](README_EN.md)

## معرفی

**Hashshashin / حشاشین** یک تونل مستقل **Layer 3 برای Linux** است که packetهای کامل IPv4 را از طریق یک interface از نوع TUN با نام `hsh0` حمل می‌کند.

دو حالت اصلی دارد:

- **Full Tunnel** — آپلود و دانلود هر دو از تونل رمزنگاری‌شده عبور می‌کنند.
- **Direct Return** — آپلود از ایران به خارج داخل تونل است، ولی دانلود از خارج از مسیر عادی اینترنت مستقیم به ایران برمی‌گردد. کاربر همچنان به همان IP و Port ایران وصل می‌شود و کانفیگش تغییر نمی‌کند.

نسخه فعلی: **v0.1.0 — Official Beta**

## چرا L3/TUN؟

در بسیاری از port forwarderها اتصال TCP کاربر روی سرور ایران terminate می‌شود و یک اتصال جدید در خارج ساخته می‌شود. Hashshashin در سطح packet کار می‌کند و packetهای IP را روی TUN حمل می‌کند.

این مدل باعث می‌شود Linux routing بتواند مسیر رفت و برگشت را مستقل از هم انتخاب کند و همین پایه‌ی اصلی Direct Return است.

## ویژگی‌ها

### Data Plane

- TUN واقعی Linux با نام `hsh0`
- حمل packet کامل IPv4
- Full Tunnel
- Direct Return
- NAT و conntrack
- Policy Routing اختصاصی
- forward کردن سرویس‌های مشخص از ایران به خارج

### امنیت Transport

- UDP carrier احراز هویت‌شده
- Shared Key با طول 256 بیت
- HMAC-SHA256 برای handshake
- nonce تصادفی Client و Server در هر session
- کلید جداگانه TX و RX
- AES-256-GCM برای payload
- packet counter در هر جهت
- replay window با ظرفیت 64 packet
- تحمل packetهای out-of-order
- keepalive
- reconnect خودکار
- rekey دوره‌ای

### جداسازی مسیر Data و Carrier

Hashshashin برای جلوگیری از loop دو مسیر routing جدا دارد:

- mark دیتای کاربر: `0x66`
- routing table دیتای کاربر: `166`
- mark خود carrier: `0x77`
- routing table carrier: `167`

در نتیجه وقتی در Full Tunnel مسیر برگشت user traffic داخل `hsh0` قرار می‌گیرد، UDP carrier خودش وارد تونل خودش نمی‌شود.

### Firewall

Hashshashin chainهای اختصاصی iptables می‌سازد و policy کلی `INPUT` یا `FORWARD` را به `ACCEPT` تغییر نمی‌دهد.

روی سرور خارج installer می‌تواند دسترسی مستقیم Public به service portهای انتخابی را ببندد و فقط مسیر موردنیاز تونل را نگه دارد.

## پیش‌نیازها

- Linux
- IPv4
- دسترسی root یا `CAP_NET_ADMIN`
- `/dev/net/tun`
- `iproute2`
- `iptables`
- systemd برای installer رسمی
- توزیع مبتنی بر `apt` یا `dnf`
- Go 1.18+ برای build از source

Ubuntu و Debian هدف اصلی installer هستند. سیستم‌های RHEL-compatible به‌صورت best-effort پشتیبانی می‌شوند.

## نصب با یک دستور

روی هر دو سرور اجرا کنید:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh)
```

بهتر است ابتدا ایران و بعد خارج نصب شود.

## مرحله ۱ — نصب روی ایران

در Wizard انتخاب کنید:

```text
1) Iran (entry server)
```

بعد Mode را انتخاب کنید:

```text
1) Full tunnel
2) Direct return
```

Installer موارد زیر را می‌پرسد:

- Public Interface
- IPv4 عمومی این سرور
- Gateway عمومی
- UDP Carrier Port
- Service Ports
- Tunnel MTU
- IP عمومی خارج

سپس یک Shared Key تولید می‌کند.

**این کلید را دقیقاً ذخیره کنید.**

## مرحله ۲ — نصب روی خارج

همان دستور را روی سرور خارج اجرا کنید و انتخاب کنید:

```text
2) Kharej (service server)
```

موارد زیر باید با ایران یکسان باشند:

- Mode
- UDP Carrier Port
- Service Ports
- Shared Key

Installer روی خارج می‌تواند از شما بپرسد که آیا دسترسی مستقیم Public به service portها بسته شود یا خیر.

## مرحله ۳ — Cloud Firewall / Security Group

اگر دیتاسنتر یا Cloud Provider فایروال جداگانه دارد، UDP carrier را بین IP ایران و IP خارج باز کنید.

نمونه با تنظیم پیش‌فرض:

```text
Protocol: UDP
Port: 9000
Source: Iran Public IP
Destination: Kharej Public IP
```

بهتر است پورت carrier فقط بین IP دو سرور باز باشد، نه برای کل اینترنت.

## مرحله ۴ — تنظیم سرویس مقصد در خارج

مثلاً اگر Xray / 3x-ui روی پورت `443` است، listener باید روی یکی از این‌ها باشد:

```text
0.0.0.0:443
```

یا:

```text
Kharej_Public_IP:443
```

اگر فقط روی این مقدار باشد:

```text
127.0.0.1:443
```

packetهایی که از `hsh0` به IP عمومی خارج می‌رسند به آن listener تحویل داده نمی‌شوند.

## شرط مهم Direct Return

برای Direct Return، IP عمومی ایران باید **واقعاً روی interface خود سرور ایران assign شده باشد**.

بررسی:

```bash
ip -4 addr show
```

اگر فقط یک IP خصوصی می‌بینید و Public IP توسط NAT/CGNAT بالادستی ارائه شده، Direct Return در v0.1 پشتیبانی نمی‌شود. در این حالت Full Tunnel را انتخاب کنید.

## معماری Full Tunnel

```text
Client
  |
  v
Iran Public Endpoint
  |
  v
hsh0 (Iran)
  |
  | authenticated + encrypted UDP carrier
  v
hsh0 (Kharej)
  |
  v
Kharej Service
  |
  v
hsh0 (Kharej)
  |
  v
hsh0 (Iran)
  |
  v
Client
```

## معماری Direct Return

```text
UPLOAD
Client -> Iran -> hsh0 ===== encrypted tunnel =====> hsh0 -> Kharej Service

DOWNLOAD
Client <- Iran <----------- normal Internet ----------- Kharej Service
```

روی ایران، conntrack و NAT وضعیت اتصال را نگه می‌دارند تا packet برگشتی مستقیم خارج به همان connection کاربر map شود.

## MTU و MSS

MTU پیش‌فرض:

```text
1320
```

Installer بازه‌ی `900..1400` را می‌پذیرد.

برای شروع `1320` مناسب است. اگر مسیر provider overhead زیاد، fragmentation یا stall دارد می‌توانید `1280` یا `1240` را تست کنید.

Hashshashin TCP MSS را برای مسیر تونل clamp می‌کند تا احتمال fragmentation کاهش پیدا کند. در Direct Return این کار جهت‌دار است تا download مستقیم بی‌دلیل با MTU مسیر upload محدود نشود.

## دستورات مدیریت

وضعیت سرویس:

```bash
systemctl status hashshashin
```

لاگ زنده:

```bash
journalctl -u hashshashin -f
```

Restart:

```bash
systemctl restart hashshashin
```

بررسی Config:

```bash
hashshashin -check -c /etc/hashshashin/config.json
```

نسخه:

```bash
hashshashin -version
```

پاک‌سازی فقط ruleهای شبکه Hashshashin:

```bash
hashshashin -cleanup -c /etc/hashshashin/config.json
```

حذف کامل برنامه:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh) --uninstall
```

برای جلوگیری از از بین رفتن اتفاقی Shared Key و تنظیمات، فایل زیر هنگام uninstall حفظ می‌شود:

```text
/etc/hashshashin/config.json
```

## تست سلامت

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
Iran hsh0       -> packetهای upload
Kharej hsh0     -> packetهای upload
Kharej Public   -> packetهای download
Iran Public     -> packetهای download
```

راهنمای دقیق‌تر:

[docs/VERIFY_FA.md](docs/VERIFY_FA.md)

## تست‌هایی که در CI انجام می‌شوند

CI پروژه روی Go 1.18 و Go 1.22 این موارد را بررسی می‌کند:

- Unit Test با race detector
- Integration Test واقعی بین دو UDP endpoint روی loopback
- handshake احراز هویت‌شده
- ساخت session keyهای جهت‌دار
- encrypt/decrypt واقعی payload
- `go vet`
- `go build`
- بررسی syntax اسکریپت نصب

سبز بودن CI به معنی تضمین رفتار تمام Providerها نیست؛ چون TUN، policy routing، cloud firewall و Direct Return به شبکه واقعی VPS وابسته‌اند.

## نکات امنیتی

نسخه v0.1 هنوز Security Audit مستقل خارجی نشده است.

برای محیط حساس:

- UDP carrier را فقط بین IP دو سرور باز کنید.
- SSH و پنل‌های مدیریتی را جداگانه محدود کنید.
- Shared Key را محرمانه نگه دارید.
- در صورت نیاز روی خارج Public access به service portها را ببندید.
- قبل از ورود کاربر واقعی، route و firewall را verify کنید.
- فایل [SECURITY.md](SECURITY.md) را بخوانید.

## محدودیت‌های فعلی

- IPv6 هنوز پیاده‌سازی نشده است.
- Carrier فعلی UDP است.
- Raw/KCP و Multipath هنوز در Roadmap هستند.
- failover خودکار Direct → Tunnel هنوز در v0.1 فعال نیست.
- Direct Return روی ایران پشت NAT/CGNAT پشتیبانی نمی‌شود.
- رفتار providerها، anti-spoofing و cloud firewall ممکن است متفاوت باشد.

## Roadmap

- Health Monitoring برای Direct Return
- Failover خودکار Direct → Tunnel
- Multipath
- Raw/KCP-style carrier اختیاری
- IPv6
- Binary Release آماده دانلود
- Package Repository
- Security Audit مستقل
- Benchmark روی چند Provider

## استقلال پروژه

Hashshashin کپی سورس Paqet، Backhaul یا BackPack نیست. ایده‌های عمومی معماری مثل packet transport، TUN/L3 و جداسازی carrier از data plane بررسی شده‌اند، اما هسته و wire protocol پروژه مستقل پیاده‌سازی شده است.

## مستندات بیشتر

- [نصب مرحله‌به‌مرحله فارسی](docs/INSTALL_FA.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Verify](docs/VERIFY_FA.md)
- [Troubleshooting فارسی](docs/TROUBLESHOOTING_FA.md)
- [Security](SECURITY.md)
- [Changelog](CHANGELOG.md)
- [Contributing](CONTRIBUTING.md)
- [English Guide](README_EN.md)

## License

MIT — فایل [LICENSE](LICENSE) را ببینید.
