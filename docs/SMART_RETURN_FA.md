# Smart Return در Hashshashin

Smart Return لایه‌ی کنترل مسیر دانلود در حالت `direct-return` است.

هدف:

```text
Normal
Upload   : Iran -> Tunnel -> Kharej
Download : Kharej -> Direct Internet -> Iran

Fallback
Upload   : Iran -> Tunnel -> Kharej
Download : Kharej -> Tunnel -> Iran

Recovery
Download : Kharej -> Direct Internet -> Iran
```

بدون تغییر endpoint کاربر و بدون تغییر کانفیگ VLESS / VMess / Trojan / Shadowsocks / WireGuard یا سرویس دیگری که روی پورت‌های تعریف‌شده حمل می‌شود.

---

## نحوه تشخیص سلامت Direct Return

خارج به‌صورت دوره‌ای یک probe کوچک UDP را **مستقیم** به IP عمومی ایران می‌فرستد:

```text
Kharej
  |
  | HDP1 + timestamp + random nonce + HMAC-SHA256
  v
Iran Public IP
```

ایران فقط probeای را معتبر می‌داند که:

- از IP خارج مورد انتظار آمده باشد؛
- HMAC آن با Shared Key صحیح باشد؛
- timestamp آن خارج از پنجره زمانی مجاز نباشد.

بعد از دریافت probe معتبر، ایران nonce را به‌صورت یک پیام control رمزنگاری‌شده داخل Session Layer `HSH1` برمی‌گرداند.

بنابراین تصمیم Health بر اساس جهت مهم زیر است:

```text
Kharej -> Iran DIRECT
```

و برای ACK به سالم بودن Session خود Hashshashin متکی است.

---

## Hysteresis

برای جلوگیری از route flapping، یک خطا باعث تغییر مسیر نمی‌شود.

پیش‌فرض:

```text
Probe interval : 5s
Probe timeout  : 2s
Fail threshold : 3
Recover        : 3
```

یعنی:

```text
3 failure متوالی
    -> Download = TUNNEL

3 success متوالی
    -> Download = DIRECT
```

مقادیر از Installer قابل تغییر هستند.

---

## جداسازی Routing

Hashshashin برای جلوگیری از loop و اثرگذاری روی ترافیک نامرتبط سه domain جدا دارد:

```text
Upload/Data Plane
fwmark 0x66
routing table 166

Smart Return service responses
fwmark 0x68
routing table 168

Carrier + Health Probe
SO_MARK 0x77
routing table 167
```

Smart Return **main routing table را برای کل Iran IP تغییر نمی‌دهد**.

فقط packetهای پاسخ Service Portهای تعریف‌شده روی خارج mark `0x68` می‌گیرند. در حالت Direct، table 168 route خاصی ندارد و lookup به main table ادامه پیدا می‌کند. هنگام fallback، فقط در table 168 این route نصب می‌شود:

```text
Iran_Public_IP/32 dev hsh0 mtu <TUN_MTU>
```

نتیجه: SSH، مانیتورینگ یا traffic مدیریتی دیگر از خارج به ایران صرفاً به‌دلیل fallback وارد تونل نمی‌شود.

Carrier و probe نیز با `0x77` از table 167 استفاده می‌کنند و همچنان روی Internet عادی می‌مانند.

---

## Firewall

علاوه بر Carrier، وقتی Smart Return فعال است باید مسیر زیر در Cloud Firewall / Security Group مجاز باشد:

```text
Protocol : UDP
Source   : Kharej Public IP
Target   : Iran Public IP
Port     : smart_return.probe_port (default 9001)
```

این پورت لازم نیست برای کل اینترنت باز باشد.

Host firewall ایران نیز rule محدودشده به IP خارج ایجاد می‌کند.

---

## مشاهده مسیر فعلی

Manager:

```bash
hashshashin
```

روی Kharej در Dashboard یکی از این دو حالت دیده می‌شود:

```text
Return Path: DIRECT
```

یا:

```text
Return Path: TUNNEL
```

بررسی دستی:

```bash
ip rule show
ip route show table 168
```

در حالت Direct معمولاً table 168 برای Iran IP route ندارد.

در fallback انتظار می‌رود:

```text
Iran_IP/32 dev hsh0
```

---

## لاگ‌ها

```bash
journalctl -u hashshashin -f
```

پیام‌های مهم:

```text
smart-return: direct path unhealthy; DOWNLOAD fallback -> TUNNEL
smart-return: direct path recovered; DOWNLOAD -> DIRECT
```

---

## محدودیت مهم

Smart Return در CI از نظر موارد زیر تست می‌شود:

- HMAC probe authentication
- hysteresis state machine
- config validation
- encrypted HSH1 probe ACK dispatch
- regression روی UDP / TCP / KCP-FEC carriers

اما CI نمی‌تواند رفتار واقعی BGP/ISP/NAT/MTU یک Provider را بازسازی کند. قبل از Production، مسیر باید روی دو VPS واقعی با `tcpdump` و traffic واقعی verify شود.
