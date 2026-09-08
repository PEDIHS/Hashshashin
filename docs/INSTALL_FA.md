# نصب مرحله‌به‌مرحله Hashshashin

[← صفحه اصلی](../README.md)

## 1. پیش‌نیازها

روی هر دو سرور ایران و خارج باید موارد زیر موجود باشد:

- Linux
- دسترسی root
- TUN/TAP فعال
- IPv4 عمومی
- systemd
- iproute2
- iptables
- دسترسی اینترنت برای دریافت سورس و build

بررسی TUN:

```bash
ls -l /dev/net/tun
```

اگر وجود نداشت، از پنل VPS قابلیت TUN/TAP را فعال کنید.

## 2. نصب روی ایران

دستور نصب:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh)
```

در بخش Role انتخاب کنید:

```text
1) Iran (entry server)
```

### انتخاب Mode

Full Tunnel:

```text
1) Full tunnel
```

Direct Return:

```text
2) Direct return
```

### ورودی‌های Wizard

Installer معمولاً این موارد را خودش تشخیص می‌دهد و مقدار پیش‌فرض پیشنهاد می‌دهد:

```text
Public interface
This server public IPv4
Public gateway
Hashshashin UDP carrier port
Service ports
Tunnel MTU
Kharej public IPv4
```

مقادیر پیشنهادی اولیه:

```text
Carrier Port: 9000
MTU: 1320
Service Ports: 443
```

اگر چند پورت دارید:

```text
443,8443,2053
```

در پایان ایران یک Shared Key تولید می‌کند. آن را دقیقاً کپی و ذخیره کنید.

## 3. نصب روی خارج

همان دستور را اجرا کنید:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh)
```

Role:

```text
2) Kharej (service server)
```

Mode باید دقیقاً با ایران یکسان باشد.

همین موارد نیز باید یکسان باشند:

- Carrier Port
- Service Ports
- Shared Key
- MTU پیشنهادی

## 4. تنظیم Cloud Firewall

اگر Provider فایروال جدا دارد، UDP carrier را بین دو سرور باز کنید.

مثال:

```text
UDP 9000
Source: IRAN_PUBLIC_IP
Destination: KHAREJ_PUBLIC_IP
```

در صورت امکان دسترسی را فقط به IP سرور مقابل محدود کنید.

## 5. تنظیم سرویس مقصد

اگر Xray / 3x-ui یا سرویس دیگری روی خارج اجرا می‌شود، listener باید روی IP قابل دسترس از `hsh0` باشد.

مناسب:

```text
0.0.0.0:443
```

یا:

```text
KHAREJ_PUBLIC_IP:443
```

نامناسب:

```text
127.0.0.1:443
```

## 6. بررسی سرویس

روی هر دو سرور:

```bash
systemctl status hashshashin
```

لاگ:

```bash
journalctl -u hashshashin -f
```

بررسی TUN:

```bash
ip addr show hsh0
```

## 7. بررسی Route ایران

```bash
ip rule show
ip route show table 166
```

## 8. بررسی Firewall Rules

```bash
iptables -t nat -S | grep HSH
iptables -t mangle -S | grep HSH
iptables -S | grep HSH
```

## 9. تست Direct Return

روی ایران:

```bash
tcpdump -ni hsh0 host KHAREJ_PUBLIC_IP
```

و هم‌زمان:

```bash
tcpdump -ni PUBLIC_INTERFACE host KHAREJ_PUBLIC_IP
```

روی خارج:

```bash
tcpdump -ni hsh0 host IRAN_PUBLIC_IP
```

و:

```bash
tcpdump -ni PUBLIC_INTERFACE host IRAN_PUBLIC_IP
```

در Direct Return انتظار داریم Upload روی `hsh0` دیده شود و Download عمدتاً روی public interface خارج و ایران ظاهر شود.

## 10. شرط Direct Return

روی ایران:

```bash
ip -4 addr show
```

Public IP ایران باید واقعاً روی interface وجود داشته باشد. اگر سرور فقط پشت NAT/CGNAT است، Full Tunnel را استفاده کنید.

## 11. تغییر MTU

Config:

```text
/etc/hashshashin/config.json
```

پس از تغییر MTU:

```bash
systemctl restart hashshashin
```

مقادیر مناسب برای تست:

```text
1320
1280
1240
```

## 12. بررسی Config

```bash
hashshashin -check -c /etc/hashshashin/config.json
```

## 13. Restart

```bash
systemctl restart hashshashin
```

## 14. Cleanup بدون Uninstall

```bash
hashshashin -cleanup -c /etc/hashshashin/config.json
```

## 15. Uninstall

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh) --uninstall
```

Config برای حفظ Shared Key باقی می‌ماند:

```text
/etc/hashshashin/config.json
```

## مشکل دارید؟

- [راهنمای Verify](VERIFY_FA.md)
- [Troubleshooting](TROUBLESHOOTING_FA.md)
- [Security](../SECURITY.md)
