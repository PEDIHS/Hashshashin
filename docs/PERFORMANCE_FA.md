# عملکرد Hashshashin v0.3

این سند برای رفع و اندازه‌گیری مشکلات واقعی Data Plane نوشته شده است؛ هدف آن ساختن عدد تبلیغاتی از یک اجرای `iperf3` نیست.

## مسئله‌ای که v0.3 حل می‌کند

در v0.2 مسیر اصلی ارسال packet عملاً به یک حلقه سری محدود بود:

```text
hsh0 -> tun.Read -> AES-GCM -> carrier.Write
```

حتی اگر Go روی چند CPU اجرا شود، یک goroutine سری می‌تواند یک هسته را اشباع کند. در بار بالا، TUN با `txqueuelen` کوچک نیز packet را قبل از رسیدن به userspace از دست می‌دهد.

v0.3 مسیر را به این شکل تغییر می‌دهد:

```text
              TUN multi-queue
        ┌────────┼────────┬────────┐
      queue0   queue1   queue2   queue3
        │        │        │        │
      pump     pump     pump     pump
        └────────┴──── HSH1 ───────┘
                       │
                 UDP/TCP/KCP
```

اگر kernel از `IFF_MULTI_QUEUE` پشتیبانی نکند، daemon به یک queue برمی‌گردد و downgrade را در journal گزارش می‌کند.

## Queue و qdisc

پروفایل Turbo به‌صورت پیش‌فرض:

```text
TUN queues       = min(CPU, 4)
UDP RX workers   = min(CPU, 4)
txqueuelen       = 4096
qdisc            = fq_codel
socket buffer    = 8 MiB
stats interval   = 10 s
```

`txqueuelen` بزرگ‌تر فقط برای جذب burst است. بزرگ کردن بی‌نهایت queue راه‌حل نیست و می‌تواند latency ایجاد کند؛ به همین دلیل `fq_codel` پیش‌فرض است.

پروفایل‌ها:

| Profile | TUN queues | txqueuelen | Socket buffer | هدف |
|---|---:|---:|---:|---|
| Balance | تا 2 | 2048 | 4 MiB | VPS کوچک/اشتراکی |
| Turbo | تا 4 | 4096 | 8 MiB | پیش‌فرض عمومی |
| Throughput | تا 8 | 8192 | 16 MiB | Bulk throughput |
| Custom | دستی | دستی | دستی | تست کنترل‌شده |

## کاهش هزینه هر packet

v0.3 چند هزینه hot-path را حذف می‌کند:

- encryption buffer از pool گرفته می‌شود؛ برای هر packet header/ciphertext جدید ساخته نمی‌شود.
- AES-GCM decrypt به‌صورت in-place انجام می‌شود.
- آدرس UDP peer برای هر packet دوباره resolve نمی‌شود.
- socket buffer برای UDP/TCP/KCP با Performance Profile هماهنگ می‌شود.
- UDP می‌تواند چند receive worker داشته باشد.

Concurrency تست می‌شود: چند worker همزمان باید counter/nonce یکتای GCM تولید کنند. CI این مسیر را با `go test -race` اجرا می‌کند.

## چرا Direct Return ممکن است قبلاً سریع‌تر نشده باشد؟

Direct Return فقط این بخش را حذف می‌کند:

```text
Kharej response -> HSH1 downlink -> Iran TUN
```

ولی موارد زیر همچنان می‌توانند bottleneck باشند:

- Iran CPU / userspace upload path
- TUN queue drop
- conntrack/NAT
- NIC/virtual NIC limits
- provider path Kharej -> Iran
- client TCP behavior / packet reordering / loss
- host load هم‌زمان با تست

بنابراین «Direct از نظر routing کار می‌کند» با «Direct حتماً bandwidth بیشتری می‌دهد» یکی نیست. v0.3 برای همین drop/rate/error telemetry و benchmark تکراری دارد.

## Journal جدید

هر 10 ثانیه نمونه‌ای مشابه زیر ثبت می‌شود:

```text
stats role=iran mode=full peer=... \
 hsh_data_tx=... hsh_data_rx=... \
 rate_tx=...Mbit/s rate_rx=...Mbit/s \
 tun_tx_drop=... tun_rx_drop=... \
 send_err=... decrypt_err=... replay_drop=... no_session_drop=...
```

این counters در سطح runtime نگه داشته می‌شوند و با rekey/session replacement صفر نمی‌شوند. در Direct Return طبیعی است که HSH1 downlink کم یا صفر باشد؛ جهت و mode در همان log مشخص است.

## روش benchmark استاندارد

روی سرور مقصد:

```bash
hsh-bench server 39001
```

server به‌صورت persistent اجرا می‌شود و قبل از اعلام Ready، listener را با `ss` چک می‌کند. این کار مشکل server یک‌بارمصرف/`connection refused` را کاهش می‌دهد.

روی سرور تست‌کننده:

```bash
hsh-bench client PEER_IP 39001 5 8
```

یعنی:

- preflight با retry
- warm-up forward و reverse که در نتیجه حساب نمی‌شود
- 5 اجرای اندازه‌گیری‌شده
- 8 TCP stream موازی (`iperf3 -P 8`)
- forward و reverse
- raw JSON برای هر run
- median/min/max/stdev/spread
- delta مربوط به `hsh0 tx_dropped/rx_dropped`

یک عدد منفرد معیار تصمیم نیست. اگر spread بیشتر از 25% باشد ابزار نتیجه را ناپایدار علامت می‌زند.

## مقایسه Full و Direct

برای مقایسه معتبر:

1. همان دو VPS و همان ساعت/بار تقریبی را استفاده کنید.
2. Carrier، MTU و Performance Profile را ثابت نگه دارید.
3. یک سری benchmark در Full بگیرید.
4. فقط Mode را به Direct Return تغییر دهید.
5. همان benchmark را تکرار کنید.
6. median و drop delta را مقایسه کنید، نه بهترین run را.

اگر Direct bandwidth را بهتر نکرد ولی HSH1 downlink حذف شده و TUN drop نیز صفر است، bottleneck احتمالاً خود تونل downlink نیست و باید provider path، conntrack، CPU/NIC یا TCP را بررسی کرد.

## Restart و SIGKILL

v0.3 روی SIGTERM:

- context را cancel می‌کند؛
- carrier و همه TUN FDها را می‌بندد تا readهای blocked بیدار شوند؛
- workerها را جمع می‌کند؛
- سپس cleanup routing/firewall انجام می‌شود.

`TimeoutStopSec` نیز از 10 به 30 ثانیه افزایش یافته است. این طراحی باید SIGKILLهای معمول هنگام switch/restart را رفع کند، ولی نتیجه نهایی باید روی VPS واقعی با restart stress test تأیید شود.

## چیزی که عمداً انجام نشده

- BBR به‌صورت global فعال نمی‌شود.
- firewall policy میزبان تغییر نمی‌کند.
- `CAP_NET_RAW` اضافه نشده است.
- queue به شکل نامحدود بزرگ نمی‌شود.
- عدد throughput تضمین‌شده اعلام نمی‌شود.

Hashshashin هنوز مستقل audit نشده و v0.3 تا پایان دو-host soak test و security review باید alpha باقی بماند.
