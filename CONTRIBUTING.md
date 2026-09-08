# مشارکت در Hashshashin

از مشارکت در توسعه Hashshashin استقبال می‌شود. این پروژه روی بخش‌های حساس شبکه، routing و transport کار می‌کند؛ بنابراین تغییرات باید قابل بررسی، قابل تست و تا حد ممکن کوچک و مشخص باشند.

## قبل از شروع

1. Issue موجود را بررسی کنید.
2. برای تغییر بزرگ در Core، Wire Protocol، Routing یا Installer ابتدا یک Issue باز کنید.
3. تغییر را روی branch جدا انجام دهید.
4. قبل از Pull Request تست‌ها را اجرا کنید.

## تست محلی

```bash
go test -race ./...
go vet ./...
go build -trimpath ./...
bash -n install.sh
```

## استاندارد Pull Request

PR باید توضیح دهد:

- مشکل یا هدف چیست
- چه چیزی تغییر کرده است
- چه بخش‌هایی از routing/transport تحت تأثیر هستند
- چگونه تست شده است
- آیا config یا wire protocol تغییر کرده است
- آیا backward compatibility شکسته می‌شود

## تغییرات Routing

اگر PR مربوط به `routing.go` است، حتماً بررسی کنید:

- carrier وارد TUN خودش نشود
- table `166` و `167` تداخل نداشته باشند
- cleanup تمام ruleهای جدید را حذف کند
- global INPUT/FORWARD policy تغییر نکند
- Direct Return و Full Tunnel هر دو در نظر گرفته شوند

## تغییرات Transport/Crypto

تغییر در transport یا crypto باید همراه تست باشد.

موارد مهم:

- nonce reuse رخ ندهد
- TX/RX key separation حفظ شود
- replay protection دور زده نشود
- reconnect/rekey باعث session collision نشود
- authentication قبل از پذیرش data انجام شود

## Installer

Installer باید:

- idempotent تا حد ممکن باشد
- قبل از نصب config را validate کند
- در خطا سرویس شکسته باقی نگذارد
- firewall عمومی سیستم را باز نکند
- uninstall/cleanup داشته باشد

## Style

کد Go باید با `gofmt` فرمت شود.

```bash
gofmt -w *.go
```

## Security Issues

آسیب‌پذیری امنیتی را در Issue عمومی با جزئیات exploit منتشر نکنید. راهنمای `SECURITY.md` را دنبال کنید.

## License

با ارسال Pull Request موافقت می‌کنید که contribution شما تحت MIT License پروژه منتشر شود.
