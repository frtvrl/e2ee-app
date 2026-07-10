# OpenE2EE API Auth Akışı

**Tarih:** 10 Temmuz 2026
**Kaynak:** Owner tarafından Sprint 10.1D kapsamında paylaşıldı
**İlgili sprint:** Sprint 10.1D (10.1B service dosyaları için JWT auth ekleme)

---

## Auth Akışı

### 1. Token Almak — `POST /api/v1/auth`

```bash
curl -s -X POST \
  -H "Host: api-test.opene2ee.com" \
  -H "Content-Type: application/json" \
  -H "X-API-Version: v1" \
  http://localhost/api/v1/auth \
  -d '{"user_id": "a1b2c3d4e5f60718a1b2c3d4"}'
```

**Response (200):**
```json
{
  "token":      "eyJhbGciOiJIUzI1NiIs...",
  "token_type": "Bearer",
  "expires_in": 3600
}
```

- `user_id` → salted device hash (ör: `a1b2c3d4e5f60718a1b2c3d4` seed'deki Turkcell device)
- **ADV-3 stub'ı:** herhangi bir non-empty `user_id` kabul eder, gerçek user table yok henüz
- **Token TTL: 1 saat**

### 2. Protected Endpoint'e İstek — `Authorization: Bearer <token>`

```bash
TOKEN="eyJhbGciOiJIUzI1NiIs..."  # yukarıdan aldığın token

curl -s \
  -H "Host: api-test.opene2ee.com" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "X-API-Version: v1" \
  http://localhost/api/v1/sessions
```

---

## JWT Payload (Claims)

```json
{
  "sub": "a1b2c3d4e5f60718a1b2c3d4",   // user_id
  "iss": "opene2ee",                     // issuer
  "exp": 1751900000,                     // +1h TTL
  "iat": 1751896400,                     // issued at
  "jti": "uuid-v4"                       // unique token id
}
```

- **Algoritma:** HS256
- **Secret:** `.env.secrets`'teki `JWT_SECRET` (backend + Kong aynısını kullanıyor)

---

## Notlar

- **Kong'daki JWT plugin** protected route'larda aktif ama Kong'un JWT consumer/credential setup'ı henüz yapılmadı. Şu an **backend kendi `IsAuthorized` middleware'i ile doğruluyor** (defence-in-depth).
- **Mevcut endpoint'ler:**
  - `POST /api/v1/auth` — token alma
  - `GET /api/v1/sessions` — protected (Bearer gerekli)
  - `POST /api/v1/telemetry` — protected (Bearer gerekli)
  - `GET /api/v1/matches` — protected (Bearer gerekli)

---

## Mobil implementasyon (Sprint 10.1D)

`mobile/lib/services/auth_service.dart`:
- `getToken()` — token cache miss/expired'da yeni token al, memory'de sakla
- `authHeaders()` — `Authorization: Bearer <token>` + `X-API-Version: v1` döner
- `invalidate()` — 401 sonrası cache temizle, sonraki call fresh token alır
- Refresh: 5 dk önce yenile (token 55dk'da expire olunca yeni al)

`mobile/lib/services/telemetry_service.dart` + `p2p_matcher.dart`:
- 401/403 → `authService.invalidate()` → retry (1 kez)
- 401 retry sonrası 401 → polling durur, hata snackbar

`mobile/lib/config.dart`:
```dart
class AppConfig {
  static const String deviceId = String.fromEnvironment('DEVICE_ID', defaultValue: 'a1b2c3d4e5f60718a1b2c3d4');
  static const String apiKey = String.fromEnvironment('API_KEY', defaultValue: 'test_key_placeholder');
  static const String apiBase = String.fromEnvironment('API_BASE', defaultValue: 'https://api-test.opene2ee.com');
  static const String apiVersion = 'v1';
}
```

Build:
```powershell
flutter build apk --debug --dart-define DEVICE_ID=a1b2c3d4e5f60718a1b2c3d4 --dart-define API_KEY=test_key_placeholder
```

---

## Test device ID (mock, veritabanında kayıtlı)

- `a1b2c3d4e5f60718a1b2c3d4` — Turkcell device seed (Owner onaylı)
- Backend bu user_id'yi kabul eder (ADV-3 stub), 200 + JWT döner

---

**Sahip notu (10.07.2026 22:32):** Bu auth flow Sprint 10.1D'de uygulandı. Backend'deki gerçek JWT_SECRET production'da secret-store'da (KMS/Vault). Dev/test ortamı `.env.secrets`'te.

**Cross-reference:**
- Sprint 10.1D brief: `workspace\brief-sprint101d.md`
- ARCHITECTURE_DECISIONS.md §5.7 (api-test.opene2ee.com spec)
