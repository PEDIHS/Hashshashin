## خلاصه تغییر

توضیح کوتاه و دقیق درباره این PR.

## نوع تغییر

- [ ] Core / L3
- [ ] Routing / Direct Return
- [ ] Transport / Carrier
- [ ] Security / Crypto
- [ ] Installer / Packaging
- [ ] Documentation
- [ ] Bug Fix

## تست انجام‌شده

```bash
go test -race ./...
go vet ./...
go build -trimpath ./...
bash -n install.sh
```

## اثر روی شبکه

- آیا mark/table جدید اضافه شده؟
- آیا Full Tunnel تغییر می‌کند؟
- آیا Direct Return تغییر می‌کند؟
- آیا MTU/MSS تغییر می‌کند؟
- آیا Firewall Rule جدید اضافه می‌شود؟

## Compatibility

- [ ] Config قبلی همچنان معتبر است.
- [ ] Wire protocol شکسته نشده است.
- [ ] Cleanup/Uninstall ruleهای جدید را پوشش می‌دهد.

## Security

- [ ] Secret/Shared Key داخل commit یا log قرار نگرفته است.
- [ ] Global firewall policy باز نشده است.
- [ ] تغییر Crypto/Transport در صورت وجود تست مناسب دارد.

## توضیحات تکمیلی

هر نکته‌ای که Reviewer باید بداند.
