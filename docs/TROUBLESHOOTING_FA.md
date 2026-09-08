# عیب‌یابی Hashshashin

## سرویس بالا نمی‌آید

```bash
systemctl status hashshashin --no-pager -l
journalctl -u hashshashin -n 100 --no-pager
hashshashin -check -c /etc/hashshashin/config.json
```

## خطای `/dev/net/tun`

```bash
modprobe tun
ls -l /dev/net/tun
```

اگر device وجود ندارد، TUN/TAP را از پنل VPS فعال کنید.

## Handshake تشکیل نمی‌شود

روی Kharej:

```bash
ss -lunp | grep 9000
tcpdump -ni <PUBLIC_IFACE> udp port 9000
```

موارد زیر را بررسی کنید:

- Shared Key دقیقاً یکسان باشد.
- carrier port یکسان باشد.
- IP ایران/خارج درست باشد.
- ساعت دو سرور sync باشد.
- UDP carrier در Cloud Firewall allow شده باشد.
- service port با UDP carrier port تداخل نداشته باشد.

## Handshake هست ولی سرویس باز نمی‌شود

روی خارج:

```bash
ss -lntup | grep <SERVICE_PORT>
ip addr show hsh0
iptables -S HSH_INPUT
```

سرویس باید روی `0.0.0.0` یا Public IP خارج listen کند، نه فقط `127.0.0.1`.

## Direct Return وصل نمی‌شود

Direct Return نیاز دارد IP عمومی ایران واقعاً روی interface ایران وجود داشته باشد:

```bash
ip -4 addr show dev <IRAN_PUBLIC_IFACE>
```

اگر IP عمومی فقط توسط NAT provider ارائه شده باشد، v0.1 Direct Return مناسب آن سرور نیست. از Full Mode استفاده کنید.

همچنین:

```bash
sysctl net.ipv4.ip_forward
sysctl net.ipv4.conf.hsh0.rp_filter
```

مقادیر مورد انتظار:

```text
net.ipv4.ip_forward = 1
net.ipv4.conf.hsh0.rp_filter = 2
```

## Full Mode loop یا قطع carrier

در v0.1 carrier با mark `0x77` و table `167` از data route جدا می‌شود. بررسی:

```bash
ip rule show | grep 77
ip route show table 167
```

اگر default route جدول 167 اشتباه است، مقدار `network.public_gateway` و `network.public_interface` را در config اصلاح کنید.

## پاک‌سازی ruleها

```bash
hashshashin -cleanup -c /etc/hashshashin/config.json
systemctl restart hashshashin
```

Hashshashin فقط chainها و routing tableهای خودش را پاک می‌کند.
