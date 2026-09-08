# راهنمای پنل مدیریت Hashshashin

بعد از نصب، برای مدیریت تونل نیازی به حفظ‌کردن دستورات systemd یا iptables ندارید. فقط اجرا کنید:

```bash
hashshashin
```

Hashshashin پنل مدیریتی interactive را باز می‌کند و در بالای صفحه وضعیت لحظه‌ای سرویس را نمایش می‌دهد:

- وضعیت `ACTIVE / STOPPED / FAILED`
- نسخه
- نقش سرور `IRAN / KHAREJ`
- Mode فعال `Full Tunnel / Direct Return`
- Public IP
- TUN interface
- MTU
- Service Ports
- RX / TX اینترفیس `hsh0`

## منوی اصلی

| گزینه | عملکرد |
|---|---|
| `1` Overview | نمایش Role، Mode، Public IPها، TUN CIDR، Carrier و Service Ports |
| `2` Health Check | بررسی systemd، config، TUN، IPv4 forwarding، UDP carrier و policy routing متناسب با Role/Mode |
| `3` Restart Tunnel | `systemctl restart hashshashin` |
| `4` Start Tunnel | اجرای سرویس |
| `5` Stop Tunnel | توقف سرویس |
| `6` Live Logs | نمایش `journalctl -f`؛ بازگشت با `Ctrl+C` |
| `7` Safe Config View | نمایش config با مخفی‌کردن Shared Key |
| `8` Reconfigure / Repair | اجرای دوباره Wizard نصب و ساخت config جدید با backup از config قبلی |
| `9` Update Hashshashin | دریافت نسخه main، تست، build و آپدیت Core/Manager بدون حذف config |
| `10` Network Diagnostics | نمایش TUN، `ip rule`، table 166/167، socket carrier، chainهای firewall و logs |
| `11` Reset Network State | Stop → cleanup ruleهای Hashshashin → Start و ساخت مجدد state |
| `12` Uninstall | حذف runtime با نگه‌داشتن config برای بازیابی |

## دستورات مستقیم

پنل مدیریتی لایه‌ای روی فرمان‌های واقعی است؛ دستورات مستقیم همچنان موجودند:

```bash
hashshashin -version
hashshashin -check -c /etc/hashshashin/config.json
hashshashin -summary -c /etc/hashshashin/config.json
hashshashin -cleanup -c /etc/hashshashin/config.json
hashshashin -menu
```

`-summary` هیچ Shared Key یا secretی چاپ نمی‌کند و برای ابزارهای monitoring/automation طراحی شده است.

## Health Check چه چیزی را بررسی می‌کند؟

Health Check بر اساس Role و Mode انتظار متفاوتی از routing دارد:

- **Iran**: وجود policy routing مربوط به data plane در table `166`.
- **Kharej + Full Tunnel**: وجود carrier isolation در table `167`.
- **Kharej + Direct Return**: نبود route برگشت اجباری از TUN یک خطا محسوب نمی‌شود؛ return path باید مستقیم باشد.

همچنین موارد زیر بررسی می‌شوند:

```text
systemd service
config validation
hsh0 interface
net.ipv4.ip_forward
UDP carrier socket
role/mode-specific routing
```

> Health Check یک تست local است. برای اثبات اینکه Direct Return واقعاً روی مسیر provider شما مستقیم برمی‌گردد، از `docs/VERIFY_FA.md` و packet capture روی هر دو VPS استفاده کنید.

## Update بدون حذف Config

از منو گزینه `9` یا مستقیم:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh) --update
```

Installer ابتدا source جدید را دریافت و test/build می‌کند، سپس config فعلی را validate کرده و سرویس را restart می‌کند. در این مسیر Shared Key و تنظیمات فعلی بازنویسی نمی‌شوند.

## Reconfigure

برای تغییر Role، Mode، IP، Carrier Port، Service Ports یا MTU:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/PEDIHS/Hashshashin/main/install.sh) --reconfigure
```

قبل از جایگزینی config، یک backup timestamped ساخته می‌شود.

## امنیت

- Shared Key در Dashboard نمایش داده نمی‌شود.
- Safe Config View مقدار `key` را با `***REDACTED***` جایگزین می‌کند.
- Manager در صورت نیاز با `sudo` اجرا می‌شود.
- Reset Network State فقط chainها و routeهای Hashshashin را هدف می‌گیرد.
- Uninstall config را به‌طور پیش‌فرض نگه می‌دارد تا secret ناخواسته از بین نرود یا کاربر lock-out نشود.
