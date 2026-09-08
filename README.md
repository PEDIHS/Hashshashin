# حشاشین | Hashshashin

**Hashshashin** یک تانل L3 مستقل برای Linux است که IP packet را از طریق TUN حمل می‌کند و برای سناریوی ایران ↔ خارج طراحی شده است.

نسخه فعلی: **v0.1.0 — Initial Official Release (Beta)**

دو حالت اصلی دارد:

- **Full Tunnel** — آپلود و دانلود هر دو از تونل عبور می‌کنند.
- **Direct Return** — آپلود از ایران به خارج داخل تونل است، اما دانلود از سرور خارج مستقیماً از مسیر عادی اینترنت به ایران برمی‌گردد؛ کاربر همچنان به همان IP ایران متصل می‌شود و نیازی به تغییر کانفیگ ندارد.

Hashshashin کپی سورس Paqet، Backhaul یا BackPack نیست. ایده‌های معماری عمومی آن‌ها — packet transport، L3/TUN و جداسازی data plane از carrier — به‌عنوان مرجع بررسی شده‌اند، اما هسته و wire protocol این پروژه مستقل پیاده‌سازی شده است.

## نصب سریع

روی **هر دو سرور** اجرا کنید:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh)
```

ترتیب پیشنهادی:

1. ابتدا روی **Iran** نصب کنید؛ installer یک Shared Key تولید می‌کند.
2. همان کلید را ذخیره کنید.
3. روی **Kharej** installer را اجرا و همان Mode، Carrier Port، Service Ports و Shared Key را وارد کنید.
4. سرویس ایران تا زمان بالا آمدن Kharej به‌صورت خودکار handshake را retry می‌کند.

### پیش‌نیاز مهم Direct Return

در حالت `direct-return`، **IP عمومی ایران باید واقعاً روی interface سرور ایران assign شده باشد**. سروری که فقط پشت CGNAT/NAT بالادستی است در v0.1 برای این حالت پشتیبانی نمی‌شود.

## معماری

### Full Tunnel

```text
Client
  |
  v
Iran Public IP
  |
  | DNAT + SNAT + policy routing
  v
hsh0 (Iran)
  |
  | Hashshashin authenticated UDP carrier
  v
hsh0 (Kharej)
  |
  v
Service / Xray
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

### Direct Return

```text
UPLOAD
Client -> Iran -> hsh0 ===== encrypted carrier =====> hsh0 -> Kharej Service

DOWNLOAD
Client <- Iran <----------- normal Internet ----------- Kharej Service
```

در Direct Return، conntrack روی ایران state اتصال را نگه می‌دارد و پاسخ مستقیم Kharej را reverse-NAT می‌کند؛ به همین دلیل endpoint کاربر همان IP/Port ایران باقی می‌ماند.

## Carrier و امنیت Transport

v0.1 از UDP به‌عنوان carrier اولیه استفاده می‌کند و روی آن این موارد را دارد:

- Shared Key با طول 256 bit
- HMAC-SHA256 برای handshake
- client/server nonce تصادفی در هر session
- کلید جداگانه برای TX و RX
- AES-256-GCM برای payload
- counter مستقل برای هر جهت
- replay window با ظرفیت 64 packet برای تحمل UDP reordering
- keepalive
- reconnect خودکار
- rekey دوره‌ای

> پروتکل هنوز audit امنیتی مستقل نشده است. برای محیط‌های حساس، قبل از استفاده گسترده security review توصیه می‌شود.

## Data-plane و جلوگیری از loop

Hashshashin دو mark مجزا استفاده می‌کند:

- `0x66` برای user/data traffic
- `0x77` برای outer carrier

در Full Mode روی Kharej، carrier با table جداگانه `167` از interface عمومی خارج می‌شود تا route برگشت user traffic به `hsh0` باعث recursive tunnel نشود.

## MTU و MSS

MTU پیش‌فرض `1320` است. installer اجازه مقدار `900..1400` را می‌دهد.

- در Full Mode، MSS در مسیر tunnel محدود می‌شود.
- در Direct Return، MSS سمت upload به‌صورت جهت‌دار clamp می‌شود تا download مستقیم بی‌جهت با MTU تونل محدود نشود.

برای شروع مقدار `1320` پیشنهاد می‌شود. اگر provider مسیر کم‌MTU دارد، `1280` یا `1240` را تست کنید.

## Firewall

Hashshashin از chainهای اختصاصی iptables استفاده می‌کند و policy عمومی firewall را روی `ACCEPT` نمی‌گذارد.

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

روی Kharej installer می‌تواند دسترسی مستقیم Public به Service Portها را block کند و فقط traffic ورودی از `hsh0` را بپذیرد.

**Cloud Firewall / Security Group** خارج از کنترل Hashshashin است. Carrier UDP (پیش‌فرض `9000`) را بین IP ایران و خارج allow کنید.

## سرویس مقصد روی Kharej

سرویس مقصد، مثلاً Xray/3x-ui، باید روی یکی از این‌ها listen کند:

```text
0.0.0.0:<service-port>
Kharej_Public_IP:<service-port>
```

اگر فقط روی `127.0.0.1` listen کند، packetهای ورودی از `hsh0` به IP عمومی Kharej به آن listener نمی‌رسند.

## دستورات مدیریت

```bash
systemctl status hashshashin
systemctl restart hashshashin
journalctl -u hashshashin -f
hashshashin -version
hashshashin -check -c /etc/hashshashin/config.json
```

پاک‌سازی ruleهای Hashshashin بدون حذف برنامه:

```bash
hashshashin -cleanup -c /etc/hashshashin/config.json
```

حذف برنامه:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh) --uninstall
```

installer فایل `/etc/hashshashin/config.json` را هنگام uninstall نگه می‌دارد تا Shared Key و تنظیمات ناخواسته از بین نرود.

## تست صحت مسیر

روی ایران:

```bash
ip addr show hsh0
ip rule show
ip route show table 166
iptables -t nat -S | grep HSH
journalctl -u hashshashin -n 50 --no-pager
```

روی Kharej:

```bash
ip addr show hsh0
journalctl -u hashshashin -n 50 --no-pager
ss -lntup
```

برای Direct Return با `tcpdump` باید الگوی کلی این باشد:

```text
Iran hsh0:      upload application packets
Kharej hsh0:    upload application packets
Kharej public:  download application packets
Iran public:    download application packets
```

جزئیات تست در [`docs/VERIFY_FA.md`](docs/VERIFY_FA.md) آمده است.

## سیستم‌های هدف v0.1

- Linux IPv4
- Ubuntu / Debian با TUN فعال
- RHEL-compatible با `dnf` به‌صورت best-effort
- `iptables` / `iproute2`
- Go 1.18+ برای build از source

## محدودیت‌های v0.1

- IPv6 هنوز پیاده‌سازی نشده است.
- carrier فعلی UDP است؛ Raw/KCP و multipath در roadmap هستند.
- automatic Direct→Tunnel quality failover هنوز در v0.1 فعال نیست؛ mode به‌صورت explicit انتخاب می‌شود.
- Direct Return روی NAT/CGNAT ایران پشتیبانی نمی‌شود.
- رفتار providerها و anti-spoofing/security-groupها متفاوت است؛ تست واقعی دو VPS ضروری است.

## مستندات

- [Architecture](docs/ARCHITECTURE.md)
- [تست و Verify](docs/VERIFY_FA.md)
- [Troubleshooting فارسی](docs/TROUBLESHOOTING_FA.md)
- [Security Policy](SECURITY.md)
- [Changelog](CHANGELOG.md)

## License

MIT — فایل [`LICENSE`](LICENSE) را ببینید.
