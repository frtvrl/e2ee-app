# OpenE2EE — iOS Distribution Setup (Team Identifier + Entitlements)

**Sürüm:** Sprint 7 — 7 Temmuz 2026
**Kapsam:** Apple Developer Team ID injection for signed iOS distribution builds
**Hedef Kitle:** Mobile release engineer, security reviewer, on-call operator
**İlgili Item:** Sprint 7 Item 13 (MOB-6) — hand-off from cyber-security

> **Bu doküman, `mobile/ios/Runner.xcodeproj/project.pbxproj` içindeki
> `DEVELOPMENT_TEAM` alanını Apple Developer Program Team ID ile doldurma
> ve `.entitlements` dosyalarını `com.apple.developer.team-identifier`
> içerecek şekilde konfigüre etme prosedürünü anlatır.**

---

## 1. Neden bu doküman var (MOB-6 kök neden)

Sprint 7 cyber-security review'ünde Apple Developer Program'a bağlı signed
distribution build'lerin **tümünde** aşağıdaki iki boşluk tespit edildi:

1. **`mobile/ios/Runner.xcodeproj/project.pbxproj`** — altı adet
   `XCBuildConfiguration` (H3-H8: Runner Debug + Release, Tunnel Debug +
   Release, RunnerTests Debug + Release) `DEVELOPMENT_TEAM = "";` ile
   boş bırakılmıştı. Xcode bu durumda "Sign to Run Locally" moduna
   düşüyor; App Store / TestFlight / Ad Hoc dağıtım için build
   imzalanamıyor.
2. **`mobile/ios/Runner/Runner.entitlements`** +
   **`mobile/ios/NetworkExtension/OpenE2eeTunnelProvider.entitlements`**
   — her iki dosya da `com.apple.developer.team-identifier` anahtarını
   içermiyordu. Apple developer dokümanlarına göre App Groups
   entitlement'ı kullanan her uygulama aynı zamanda team identifier
   entitlement'ını da taşımalıdır; aksi takdirde tunnel target shared
   Keychain erişimi sessizce kesilir (Sprint 4 PR-25 keychain hand-off
   akışı bozulur).

Bu PR her iki boşluğu da kapatır: **Team ID artık xcconfig üzerinden
environment bazlı inject ediliyor**, Local config boş (dev/sim) bırakılır
ve Production config operator tarafından doldurulur.

---

## 2. Mimari (post-PR durumu)

```
+------------------------------+      +-------------------------------+
| mobile/ios/Config/           |      | Runner.xcodeproj/project.pbxproj
|   Local.xcconfig             |      |   H3 (Runner Debug)         --+
|     TEAMS_IDENTIFIER =       | <----|   H4 (Runner Release)         |
|                              |      |   H5 (Tunnel Debug)           |  baseConfigurationReference
|   Production.xcconfig        | <----|   H6 (Tunnel Release)        |
|     TEAMS_IDENTIFIER = XXX   |      |   H7 (RunnerTests Debug)      |
+------------------------------+      |   H8 (RunnerTests Release)  --+
                                      +-------------------------------+
                                                     |
                                                     | DEVELOPMENT_TEAM = $(TEAMS_IDENTIFIER)
                                                     v
+--------------------------------+    +---------------------------------+
| Runner/Runner.entitlements     |    | NetworkExtension/               |
|   com.apple.developer.team-    |    |   OpenE2eeTunnelProvider.       |
|     identifier = $(TEAMS_      |    |     entitlements               |
|     IDENTIFIER)                |    |   com.apple.developer.team-    |
+--------------------------------+    |     identifier = $(TEAMS_      |
                                      |     IDENTIFIER)                |
                                      +---------------------------------+
```

### 2.1 xcconfig seçimi build configuration'a göre otomatik

`baseConfigurationReference` alanı `project.pbxproj` içinde
XCBuildConfiguration düzeyinde çözümlenir; Xcode, ilgili build
configuration aktif olduğunda doğru xcconfig'i otomatik yükler:

| Build configuration | Hangi xcconfig | DEVELOPMENT_TEAM değeri |
|---|---|---|
| Runner Debug (H3) | `Local.xcconfig` | boş → "Sign to Run Locally" |
| Runner Release (H4) | `Production.xcconfig` | operatörün doldurduğu değer |
| Tunnel Debug (H5) | `Local.xcconfig` | boş → "Sign to Run Locally" |
| Tunnel Release (H6) | `Production.xcconfig` | operatörün doldurduğu değer |
| RunnerTests Debug (H7) | `Local.xcconfig` | boş → sim üzerinde test |
| RunnerTests Release (H8) | `Local.xcconfig` | boş → sim üzerinde test |

Test target'lar için Production kullanılmıyor: XCTest runner yalnızca
simulator'da koşar ve Apple Developer Team ID'sine ihtiyaç duymaz.

### 2.2 Entitlements'ta `$(TEAMS_IDENTIFIER)` — önemli caveat

**Xcode, xcconfig değişkenlerini plist dosyalarında otomatik olarak
yer değiştirmez.** `Runner.entitlements` ve
`OpenE2eeTunnelProvider.entitlements` içindeki `$(TEAMS_IDENTIFIER)`
string'i build-time substitution'a tabi değildir. Üç geçerli yaklaşım
vardan:

1. **Manuel düzenleme** (küçük ekipler için yeterli): Production'a
   geçmeden hemen önce her iki `.entitlements` dosyasında
   `<string>$(TEAMS_IDENTIFIER)</string>` satırını gerçek 10-karakter
   Team ID ile değiştirin, build alın, geri alın. Repo'ya asla gerçek
   değeri işlemeyin.
2. **Pre-build phase script** (önerilen): `Production.xcconfig` yerine
   bir Run Script build phase ekleyin; script xcconfig'teki
   `TEAMS_IDENTIFIER` değerini okuyup `sed` ile
   `.entitlements` dosyalarına yazar. Bu PR bu yaklaşımı zorunlu
   kılmıyor — Sprint 8 follow-up backlog'unda duruyor.
3. **Sadece DEVELOPMENT_TEAM'ı kullan** (Apple'ın resmi yaklaşımı):
   `com.apple.developer.team-identifier` anahtarını entitlements'tan
   tamamen kaldırın; Xcode, signing identity'den team ID'yi otomatik
   olarak derive eder. **Ancak bu yaklaşım ADIM 1'de listelenen Apple
   kuralını ihlal eder** ("App Groups + Team ID zorunlu"). Bu sebeple
   tercih edilmez.

Sprint 7 Item 13 kapsamında **yaklaşım 1 (manuel düzenleme)** ile
birlikte, Production build alınmadan hemen önce operatör checklist'inin
parçası olarak §4'te detaylandırılmıştır.

---

## 3. Production build — operatör prosedürü

### 3.1 Team ID'yi bulma

1. <https://developer.apple.com/account/#!/membership> adresine gidin
   (App Store Connect → Membership).
2. "Team ID" sütunundaki 10-karakterlik alfanümerik değeri kopyalayın
   (örnek format: `ABCDE12345`).
3. Bu değer **kimseyle paylaşılmaz**, repoya commit edilmez, CI log'larına
   yazılmaz (GitHub Actions secret olarak inject edilir — bkz. §5).

### 3.2 `mobile/ios/Config/Production.xcconfig` doldurma

`mobile/ios/Config/Production.xcconfig` dosyasını açın ve şu satırı
bulun:

```xcconfig
TEAMS_IDENTIFIER = REPLACE_WITH_YOUR_APPLE_DEVELOPER_TEAM_ID
```

`REPLACE_WITH_YOUR_APPLE_DEVELOPER_TEAM_ID` yerine 10-karakterlik Team
ID'yi yazın (tırnak işareti olmadan, boşluksuz). Son hali:

```xcconfig
TEAMS_IDENTIFIER = ABCDE12345
```

> **Güvenlik notu:** Production.xcconfig repoya commit edilir ama
> placeholder (gerçek olmayan değer) kalır. Operatör gerçek değeri
> repoya commit etmemelidir. CI build'i sırasında değer GitHub Actions
> secret'tan inject edilir — bkz. §5.

### 3.3 Entitlements dosyalarını güncelleme (yaklaşım 1, manuel)

Production build almadan **hemen önce**, iki dosyada aynı sed
işlemini yapın:

```bash
cd mobile/ios

# Runner.entitlements
sed -i '' 's|<string>$(TEAMS_IDENTIFIER)</string>|<string>ABCDE12345</string>|' Runner/Runner.entitlements

# NetworkExtension
sed -i '' 's|<string>$(TEAMS_IDENTIFIER)</string>|<string>ABCDE12345</string>|' NetworkExtension/OpenE2eeTunnelProvider.entitlements

# Build'i al
flutter build ios --release

# Hemen ardından geri al (repo temiz kalsın)
git checkout -- Runner/Runner.entitlements NetworkExtension/OpenE2eeTunnelProvider.entitlements
```

Windows PowerShell'de `sed -i ''` çalışmaz; aşağıdaki gibi yapın:

```powershell
cd mobile/ios

(Get-Content Runner\Runner.entitlements) `
  -replace '<string>\$\(TEAMS_IDENTIFIER\)</string>', '<string>ABCDE12345</string>' |
  Set-Content -Encoding UTF8 Runner\Runner.entitlements

(Get-Content NetworkExtension\OpenE2eeTunnelProvider.entitlements) `
  -replace '<string>\$\(TEAMS_IDENTIFIER\)</string>', '<string>ABCDE12345</string>' |
  Set-Content -Encoding UTF8 NetworkExtension\OpenE2eeTunnelProvider.entitlements

flutter build ios --release

git checkout -- Runner/Runner.entitlements NetworkExtension/OpenE2eeTunnelProvider.entitlements
```

> **Yaklaşım 2 (build phase script) Sprint 8 backlog'unda.**
> Script bu PR'a eklenmedi; bu doküman §2.2'deki caveat aktif kalır.

### 3.4 Production build doğrulama

Build başarılı olduktan sonra, derlenmiş IPA'nın doğru Team ID'yi
taşıdığını doğrulayın:

```bash
# IPA'yı aç
unzip -p build/ios/iphoneos/Runner.app/Info.plist | plutil -p - | grep -i team

# Beklenen: çıktı yoktur (Info.plist'te team identifier bulunmaz,
# yalnızca entitlements'tadır)

# Entitlements doğrulama
codesign -d --entitlements :- build/ios/iphoneos/Runner.app 2>&1 | grep team-identifier
# Beklenen: "com.apple.developer.team-identifier" = ["ABCDE12345"]
```

---

## 4. Local / sim build (operatör checklist'i)

`mobile/ios/Config/Local.xcconfig` **boş** `TEAMS_IDENTIFIER` ile gelir;
bu kasıtlıdır ve güvenlik gereksinimidir:

* Sim + dev build'leri gerçek bir Team ID'ye ihtiyaç duymaz (Xcode "Sign
  to Run Locally" moduna düşer).
* Dev build'e gerçek bir Team ID gömülmesi reverse-engineering'e karşı
  savunmasız bir attack-surface oluşturur (ADR-0006 anonymisation
  ilkesiyle çelişir).
* Production build yalnızca signed distribution pipeline'ında
  yapılmalıdır.

**Local dev loop:**

```bash
flutter run -d "iPhone 15 Pro"  # sim
# veya
flutter run -d <UDID>           # fiziksel cihaz (dev signing)
```

`Runner.entitlements`'taki literal `$(TEAMS_IDENTIFIER)` dev build'leri
etkilemez çünkü sim build App Store review'a girmez; App Groups
entitlement'ı test sırasında zaten çalışmaz (simulator App Group
sandbox'ı sınırlıdır — bkz. Sprint 4 PR-25 keychain test notu).

---

## 5. CI / GitHub Actions entegrasyonu

`.github/workflows/ios.yml`'in `macos-latest` leg'i şu sırayla
çalışmalıdır:

1. **Repository secret** tanımla: Settings → Secrets → New repository
   secret → `OPENE2EE_TEAMS_IDENTIFIER` = gerçek Team ID.
2. **Workflow step** ekle (mevcut `flutter build ios --release`
   adımından önce):

   ```yaml
   - name: Inject Production.xcconfig Team ID
     env:
       TEAMS_IDENTIFIER: ${{ secrets.OPENE2EE_TEAMS_IDENTIFIER }}
     run: |
       set -euo pipefail
       sed -i.bak "s/REPLACE_WITH_YOUR_APPLE_DEVELOPER_TEAM_ID/${TEAMS_IDENTIFIER}/" \
         mobile/ios/Config/Production.xcconfig
       echo "TEAMS_IDENTIFIER injected for CI build"
   ```

3. **Entitlements pre-build phase** (Sprint 8 backlog; bu PR'a
   dahil değildir): Build phase script ekleyerek
   `$(TEAMS_IDENTIFIER)` literal'ını CI secret'tan okunan değerle
   değiştirin. Alternatif olarak bu PR'ın §3.3 manual adımını
   workflow'un pre-build step'ine dönüştürün.

4. **Post-build cleanup**: build sonrası `git checkout --` ile
   `Production.xcconfig` üzerindeki değişikliği geri alın; CI
   workspace'i repoyu kirletmesin.

Mevcut `.github/workflows/ios.yml` (Sprint 7 Item 7 STRIDE-8-01 ile
eklenen) bu adımları henüz içermiyor; bu entegrasyon Sprint 8'de
planlanıyor.

---

## 6. Team ID rotation prosedürü

Bir Apple Developer hesabı başka bir ekibe/organizasyona transfer
edildiğinde (örneğin kurum içi devir, satın alma, program iptali)
Team ID değişebilir. Rotation prosedürü:

1. Eski Team ID'yi taşıyan tüm signed build'leri (App Store +
   TestFlight) yeni Team ID ile yeniden imzalayın.
2. `mobile/ios/Config/Production.xcconfig`'u yeni değerle güncelleyin
   (bkz. §3.2).
3. CI secret'ı (`OPENE2EE_TEAMS_IDENTIFIER`) güncelleyin.
4. Provisioning profillerini Apple Developer Portal'dan yeni Team ID
   altında yeniden oluşturun.
5. `mobile/ios/Runner/Runner.entitlements` +
   `OpenE2eeTunnelProvider.entitlements` içindeki literal Team ID'yi
   güncelleyin (Production build sırasında — bkz. §3.3).
6. App Store Connect'te bundle ID'yi yeniden register edin; yeni
   profil altında çalıştığını doğrulayın.

> **Sprint 8 backlog notu:** §3.3'teki manual adımı ve §5 adım 3'teki
> build-phase script'i tamamlandığında bu doküman sadeleşecek;
> operatörün Production build öncesi elle `.entitlements` düzenlemesi
> gerekmeyecek.

---

## 7. Doğrulama kontrol listesi (her release build öncesi)

- [ ] `mobile/ios/Config/Production.xcconfig` içinde
      `TEAMS_IDENTIFIER = ABCDE12345` (placeholder değil, 10-karakter
      gerçek değer) — CI tarafından inject edilecekse bu adım
      skip edilebilir.
- [ ] `Runner/Runner.entitlements` +
      `NetworkExtension/OpenE2eeTunnelProvider.entitlements` içinde
      `com.apple.developer.team-identifier` = `ABCDE12345`
      (production build öncesi manual substitution).
- [ ] `flutter build ios --release` çıkışında
      `Code Signing / Runner.app: signed ... with identity '<Team Name>'
      (Team ID: ABCDE12345)` mesajı görünüyor.
- [ ] `codesign -d --entitlements :- build/ios/iphoneof/Runner.app`
      çıktısında `com.apple.developer.team-identifier` doğru değer.
- [ ] Post-build: `git status` temiz, hiçbir secret
      (`Production.xcconfig` veya `.entitlements` içinde gerçek Team ID)
      workspace'te kalmamış.

---

## 8. Referanslar

* **Sprint 7 Item 13 (MOB-6) PR** — bu değişikliği yapan PR.
* **ADR-0003 (vpn-layer)** — App Groups + Keychain sharing'in neden
  team identifier gerektirdiğinin mimari gerekçesi.
* **ADR-0006 (anonimlik)** — local config'te Team ID boş bırakma
  kararının anonimlik gerekçesi.
* **`docs/NATIVE-DEV-SETUP.md`** §2.2 — Xcode + CocoaPods dev tooling
  kurulumu (CI entegrasyonu için önkoşul).
* **Apple developer docs — App Groups**: <https://developer.apple.com/documentation/bundleresources/entitlements/com_apple_security_application-groups>
  ("If your app uses the App Groups entitlement, you must also have
  the team identifier entitlement").
* **Sprint 7 Item 7 (STRIDE-8-01)** — `.github/workflows/ios.yml`
  macos-latest matrix leg'i (CI entegrasyonu için temel).
* **`memory/ios-native-bridging.md` (agent-local)** — hand-written
  pbxproj UUID collision gotchas; bu PR §1'deki UUID seçimleri
  (B8 + BD) o dokümanda listelenen free-slot tarama yöntemiyle
  doğrulanmıştır.