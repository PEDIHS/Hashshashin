# تست عملی Hashshashin

این راهنما برای تست دو VPS واقعی نوشته شده است.

## 1. وضعیت سرویس

روی هر دو سرور:

```bash
systemctl is-active hashshashin
hashshashin -check -c /etc/hashshashin/config.json
ip -br addr show hsh0
```

انتظار:

```text
Iran:   10.77.0.1/30
Kharej: 10.77.0.2/30
```

## 2. بررسی Handshake

```bash
journalctl -u hashshashin -n 100 --no-pager | grep -E 'session established|handshake|stats'
```

روی ایران باید `session established to ...` و روی خارج `session established from ...` دیده شود.

## 3. بررسی Carrier

پیش‌فرض carrier پورت UDP 9000 است:

```bash
tcpdump -ni <PUBLIC_IFACE> udp port 9000
```

روی هر دو سمت باید packet دیده شود.

اگر در host دیده نمی‌شود، ابتدا Cloud Firewall/Security Group و سپس firewall سیستم را بررسی کنید.

## 4. تست Full Mode

روی ایران:

```bash
ip rule show | grep 166
ip route show table 166
```

روی خارج:

```bash
ip rule show | grep 167
ip route show table 167
ip route get <IRAN_PUBLIC_IP>
```

برای traffic سرویس، payload باید در هر دو جهت روی `hsh0` دیده شود:

```bash
tcpdump -ni hsh0 port <SERVICE_PORT>
```

## 5. تست Direct Return

چهار capture هم‌زمان بسیار مفید است.

Iran:

```bash
tcpdump -ni hsh0 host <KHAREJ_PUBLIC_IP> and port <SERVICE_PORT>
tcpdump -ni <IRAN_PUBLIC_IFACE> host <KHAREJ_PUBLIC_IP> and port <SERVICE_PORT>
```

Kharej:

```bash
tcpdump -ni hsh0 host <IRAN_PUBLIC_IP> and port <SERVICE_PORT>
tcpdump -ni <KHAREJ_PUBLIC_IFACE> host <IRAN_PUBLIC_IP> and port <SERVICE_PORT>
```

انتظار:

- upload application data روی `Iran hsh0` و `Kharej hsh0` دیده شود.
- download application data روی public interface خارج و public interface ایران دیده شود.
- carrier UDP جداگانه روی پورت carrier دیده می‌شود و نباید با application flow اشتباه گرفته شود.

## 6. بررسی Conntrack ایران

در صورت نصب بودن conntrack:

```bash
conntrack -L | grep <SERVICE_PORT>
```

Direct Return برای reverse NAT به state conntrack ایران وابسته است.

## 7. تست MTU

اگر ping و handshake درست است ولی دانلود/آپلود stall می‌کند، MTU را از 1320 به 1280 کاهش دهید، سپس:

```bash
systemctl restart hashshashin
```

برای TCP، Hashshashin MSS را بر اساس MTU انتخاب‌شده clamp می‌کند.
