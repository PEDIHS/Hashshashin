# Transportهای Hashshashin

Hashshashin در سطح L3 کار می‌کند؛ بنابراین **Carrier** فقط روش انتقال packetهای HSH1 بین ایران و خارج است. پروتکل کاربر (VLESS، VMess، Trojan، Shadowsocks، WireGuard و غیره) از Carrier مستقل است و درون packetهای IP حمل می‌شود.

## Carrierهای پیاده‌سازی‌شده

| Carrier | لایه زیرین | وضعیت | کاربرد پیشنهادی |
|---|---|---|---|
| UDP | UDP | ✅ | کمترین سربار، مسیرهای سالم و پایدار |
| TCP | TCP stream + framing | ✅ | شبکه‌هایی که UDP را محدود یا degrade می‌کنند |
| KCP | KCP روی UDP | ✅ | loss/jitter، نیاز به ARQ سریع و FEC |

تمام Carrierها از Session Layer یکسان Hashshashin استفاده می‌کنند:

```text
HSH1
├── HMAC-SHA256 authenticated handshake
├── AES-256-GCM payload encryption
├── independent TX/RX keys
├── replay window
├── keepalive
├── reconnect
└── periodic rekey
```

بنابراین KCP یا TCP جای رمزنگاری HSH1 را نمی‌گیرند؛ فقط outer transport را تغییر می‌دهند.

---

## UDP

مسیر:

```text
TUN -> HSH1/AES-GCM -> UDP -> Internet -> UDP -> HSH1 -> TUN
```

مزایا:
- سربار پایین
- latency پایین
- بدون head-of-line blocking در Carrier
- انتخاب مناسب برای مسیرهای خوب

Provider Firewall:

```text
UDP/<carrier-port>
```

---

## TCP

Hashshashin packetهای HSH1 را با یک length-prefix چهار بایتی روی TCP حمل می‌کند.

```text
TUN -> HSH1 -> frame -> TCP -> frame -> HSH1 -> TUN
```

تنظیمات runtime:
- `TCP_NODELAY`
- TCP keepalive
- reconnect outer stream
- `SO_MARK=0x77` برای جلوگیری از recursive routing در Full Tunnel

TCP برای شبکه‌ای مناسب است که UDP carrier محدود یا ناپایدار است. به دلیل TCP-over-TCP، در loss بالا ممکن است رفتار آن از UDP/KCP بدتر باشد؛ بنابراین برای همه مسیرها انتخاب پیش‌فرض نیست.

Provider Firewall:

```text
TCP/<carrier-port>
```

---

## KCP + FEC

KCP یک ARQ سریع روی UDP است. Hashshashin از `github.com/xtaci/kcp-go/v5` استفاده می‌کند و HSH1 را به‌صورت stream روی KCP منتقل می‌کند.

Presetهای Installer:

### Balanced

```text
FEC data/parity: 10/3
NoDelay: 1
Interval: 20 ms
Resend: 2
NC: 1
Window: 512/512
KCP MTU: 1200
```

پیشنهاد عمومی برای مسیرهایی با loss متوسط.

### Low Overhead

```text
FEC: disabled (0/0)
```

برای مسیرهای کم-loss که retransmission KCP کافی است.

### Strong FEC

```text
FEC: 10/5
```

سربار بیشتر، ولی تحمل بهتر در برابر packet loss.

Provider Firewall:

```text
UDP/<carrier-port>
```

> KCP/FEC مصرف bandwidth و CPU بیشتری از UDP ساده دارد. Strong FEC را فقط وقتی loss واقعی مسیر توجیه می‌کند فعال کنید.

---

## انتخاب Carrier

قاعده عملی اولیه:

```text
مسیر سالم / کم loss          -> UDP
UDP محدود یا فیلترشده        -> TCP
loss/jitter محسوس            -> KCP Balanced
loss شدید                    -> KCP Strong FEC (پس از اندازه‌گیری)
```

بهترین Carrier باید روی همان مسیر ایران/خارج benchmark شود؛ انتخاب ثابت برای همه دیتاسنترها وجود ندارد.

---

## Transportهایی که هنوز پیاده نشده‌اند

موارد زیر تا زمانی که در Core و CI قرار نگرفته‌اند، **قابلیت فعال Hashshashin محسوب نمی‌شوند**:

- Raw TCP / pcap carrier
- QUIC
- WebSocket / WSS
- ICMP
- Multipath / multi-carrier bonding
- automatic carrier failover

این موارد در Roadmap هستند و باید مستقل از HSH1 Session Layer اضافه شوند.

---

## تست‌های CI

برای Multi-Transport، تست‌های integration شامل موارد زیر است:

- UDP: handshake + encrypted payload delivery
- TCP: framed stream + handshake + encrypted payload delivery
- KCP: KCP session با FEC `10/3` + HSH1 handshake + encrypted payload delivery
- race detector
- `go vet`
- build روی Go 1.18 و 1.22

تست CI جای تست دو VPS واقعی را نمی‌گیرد؛ firewall، MTU، routing و کیفیت carrier باید روی Provider واقعی نیز بررسی شوند.
