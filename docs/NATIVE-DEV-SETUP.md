# OpenE2EE — Native Dev Tooling Setup

**Sürüm:** Sprint 3 / §9 — 6 Temmuz 2026
**Kapsam:** Android Studio + Xcode dev tooling (release build hariç)
**Hedef Kitle:** Mobile geliştiriciler (Android + iOS)
**İlgili ADR'ler:** `docs/ADR-0008-multiplatform-tooling.md`, `docs/MULTIPLATFORM.md`

Bu rehber, OpenE2EE monoreposunda Android Studio ve Xcode ile native mobile development için gerekli dev tooling kurulumunu anlatır. Release build (signed APK/IPA) **Sprint 4+** kapsamındadır — bu rehber yalnızca dev tooling setup'ını kapsar.

---

## 1. Hızlı başlangıç (TL;DR)

```bash
# Repo'yu zaten klonladıysanız (MULTIPLATFORM.md §2):
cd mobile
flutter create --platforms=android,ios --org com.opene2ee --project-name opene2ee .
# Bu komut Sprint 3 PR-22a (Android VPN) ve PR-22b (iOS VPN) için zaten
# çalıştırıldı; yeni dev'ler yine de re-run edebilir (idempotent).

# Android Studio ile aç:
#   File > Open > C:\repos\e2ee-app\mobile\android
#   (Android Studio Flutter + Dart plugin'i otomatik algılar)

# Xcode ile aç:
#   File > Open > C:\repos\e2ee-app\mobile/ios/Runner.xcworkspace
#   (Podfile'daki Pods/ klasörü `pod install` ile kurulur)
```

---

## 2. Önkoşullar (platform başına)

### 2.1 Android tarafı

| Bileşen | Minimum | Not |
|---|---|---|
| **JDK** | 17 (LTS) | Android Studio Hedgehog+ JDK 17 ile gelir; ayrıca kurulum gerekmez |
| **Android Studio** | Hedgehog (2023.1.1) veya üstü | <https://developer.android.com/studio> |
| **Android SDK** | API 34 (target) + API 21 (minSdk) | Flutter Android gereksinimi |
| **Flutter SDK** | ≥ 3.24 (Dart ≥ 3.5) | `mobile/pubspec.yaml` ile uyumlu |
| **Android Studio Plugin: Flutter** | Son sürüm | Settings → Plugins → "Flutter" → Install |
| **Android Studio Plugin: Dart** | Son sürüm | Flutter plugin'i ile birlikte gelir |

### 2.2 iOS tarafı (yalnızca macOS)

| Bileşen | Minimum | Not |
|---|---|---|
| **macOS** | 13.0 (Ventura) veya üstü | Xcode 15+ gerektirir |
| **Xcode** | 15.0 veya üstü | <https://developer.apple.com/xcode/> |
| **CocoaPods** | 1.13 veya üstü | `sudo gem install cocoapods` (Ruby sistemi ile) veya `brew install cocoapods` |
| **Command Line Tools** | 15.0 | `xcode-select --install` |
| **Flutter SDK** | ≥ 3.24 | Android ile aynı kurulum |
| **xcodeproj gem** | 1.23 | CocoaPods ile birlikte gelir |

> iOS tooling yalnızca macOS üzerinde çalışır. Linux/Windows geliştirici yalnızca Android + web hedeflerine katkı sağlayabilir.

---

## 3. Android Studio kurulumu

### 3.1 İlk açılış

1. **Android Studio'yu başlat.**
2. **Plugin'leri kur:** Settings → Plugins → Marketplace → şunları yükle:
   - **Flutter** (Dart'ı otomatik yükler)
   - **Kotlin** (Android Studio ile birlikte gelir, sürümü doğrula)
   - **EditorConfig** (repo `.editorconfig`'unu otomatik okur)
3. **SDK Manager:** SDK Manager → SDK Platforms → "Android 14 (API 34)" + "Android 12 (API 31)" kutularını işaretle → Apply.
4. **Repo'yu aç:** File → Open → `C:\repos\e2ee-app\mobile\android`. Android Studio:
   - `.idea/codeStyles/Project.xml`'deki Kotlin official code style'ı okur
   - `.idea/inspectionProfiles/project_default.xml`'deki inspection profile'ı uygular
   - `gradle.properties`'i sync eder

### 3.2 İlk Gradle sync

İlk açılışta Gradle sync birkaç dakika sürebilir (Flutter + pluginler indirilir). Tamamlandığında `Build` paneli şunu göstermeli:

```
> Configure project :app
> Task :app:preBuild UP-TO-DATE
> Task :app:mergeDebugResources
...
BUILD SUCCESSFUL in 4m 12s
```

**Sorun giderme:** Sync başarısız olursa:
- `File → Invalidate Caches...` → "Invalidate and Restart"
- `local.properties` eksikse (bkz. §3.4)
- Gradle cache'i temizle: `cd mobile/android && ./gradlew clean`

### 3.3 Debug build

- **Toolbar → Run ▶ → "app" seçili → emulator seç → Run**
- Veya CLI: `cd mobile && flutter run -d <device-id>`

İlk build ~3-5 dakika (Gradle + Flutter pluginler). Sonraki rebuild'lar ~30-60 saniye.

### 3.4 `local.properties` — dev override pattern

`local.properties` makine-spesifik yollar içerir; **asla commit edilmez**. Repo'da `local.properties.example` template'i vardır:

```bash
cd mobile/android
cp local.properties.example local.properties
# Edit: flutter.sdk ve sdk.dir satırlarını kendi makinene göre ayarla
```

**Windows tipik değerler:**
```properties
flutter.sdk=C:\\src\\flutter
sdk.dir=C:\\Users\\<you>\\AppData\\Local\\Android\\Sdk
```

**macOS tipik değerler:**
```properties
flutter.sdk=/Users/<you>/development/flutter
sdk.dir=/Users/<you>/Library/Android/sdk
```

> **Otomatik doldurma:** `flutter doctor -v` çalıştırırsanız Flutter SDK yolunu otomatik algılar ve `local.properties`'i doldurur. Çoğu durumda manuel edit gerekmez.

---

## 4. Xcode kurulumu (yalnızca macOS)

### 4.1 Xcode ve Command Line Tools

```bash
# Xcode'u App Store'dan kur (büyük indirme ~12 GB)
# Veya komut satırından:
xcode-select --install      # CLT (Command Line Tools)
sudo softwareupdate --install-automation   # Xcode önerisi için
```

### 4.2 CocoaPods kurulumu

```bash
# Sistem Ruby ile (varsayılan):
sudo gem install cocoapods

# Veya Homebrew ile (önerilen — Ruby sürümü izole):
brew install cocoapods
```

Doğrula: `pod --version` → 1.13+ çıkmalı.

### 4.3 İlk açılış

1. **Xcode'u başlat.**
2. **Repo'yu aç:** File → Open → `C:\repos\e2ee-app\mobile/ios/Runner.xcworkspace`
   - `.xcodeproj` değil `.xcworkspace` aç! CocoaPods entegrasyonu workspace'tedir.
3. **Bundle identifier:** Target "Runner" → Signing & Capabilities → Bundle Identifier'ı `com.opene2ee.opene2ee` (veya kendi reverse-DNS'inize) ayarla.
   > Sprint 3 §9'da release signing devre dışı; Personal Team ile auto-signing yeterli.
4. **Team:** Signing & Capabilities → Team → Apple ID'nizle giriş yapın (ücretsiz Personal Team).

### 4.4 CocoaPods senkronizasyonu

`Podfile` değiştiğinde veya ilk açılışta:

```bash
cd mobile/ios
pod install                  # Pods/ + .symlinks/ üretir (~30-60s)
```

`Pods/` ve `Podfile.lock` `.gitignore`'dadır — commit edilmez.

### 4.5 Debug build

- **Xcode → Run ▶ (⌘R)** → Simulator seç → Run
- Veya CLI: `cd mobile && flutter run -d <simulator-id>`

İlk build ~3-5 dakika (Flutter + CocoaPods). Sonraki rebuild'lar ~20-40 saniye.

---

## 5. Build cache yönetimi

### 5.1 Gradle cache (Android)

`mobile/android/gradle.properties` shared cache ayarlarını içerir:

```properties
org.gradle.caching=true                  # local build cache
org.gradle.parallel=true                 # parallel project execution
org.gradle.configureondemand=true        # configure only what's needed
```

Cache konumu: `~/.gradle/caches/` (kullanıcı başına). Sprint 4+'da remote cache (Build Cache Node) opsiyonu değerlendirilebilir.

**Cache temizleme** (son çare):
```bash
cd mobile/android
./gradlew clean                           # build/ temizle
rm -rf ~/.gradle/caches/                  # nükleer seçenek — yeniden indirir
```

### 5.2 CocoaPods cache (iOS)

`Pods/` klasörü her `pod install`'da yeniden kurulur. Cache konumları:
- `~/Library/Caches/CocoaPods/` (specs repo cache)
- `mobile/ios/Pods/` (proje-local)

**Cache temizleme** (CI'da faydalı):
```bash
cd mobile/ios
rm -rf Pods/ Podfile.lock
pod cache clean --all                     # global cache temizle
pod install                               # yeniden kur
```

### 5.3 Flutter build cache (cross-platform)

```bash
cd mobile
flutter clean                             # build/ + .dart_tool/ temizle
flutter pub get                           # bağımlılıkları yeniden çek
```

---

## 6. Test cihaz provisioning

OpenE2EE'nin hedeflediği test cihaz çeşitliliği:

| Platform | Cihazlar | BrowserStack sınıfı |
|---|---|---|
| Android | Pixel 7 (Android 14), Samsung Galaxy S22 (Android 13), Xiaomi Redmi Note 11 (Android 12) | "Modern Android" |
| iOS | iPhone 15 Pro (iOS 17), iPhone 13 (iOS 16), iPhone SE (iOS 15) | "Modern iOS" |
| Eski sürümler | Android 8.0, iOS 14 | "Legacy" |

### 6.1 Firebase Test Lab (Android)

**Setup (bir kere):**
1. <https://console.firebase.google.com/> → "OpenE2EE" projesi (henüz yoksa oluştur).
2. Test Lab → Get started → Service account JSON indir.
3. JSON'ı `secrets/firebase-test-lab-sa.json` olarak kaydet (gitignored).

**Çalıştırma:**
```bash
cd mobile/android
./gradlew assembleDebugAndroidTest
# Instrumented test APK'sı app/build/outputs/apk/androidTest/debug/ altında

gcloud firebase test android run \
  --type instrumentation \
  --app app/build/outputs/apk/debug/app-debug.apk \
  --test app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk \
  --device model=Pixel7,version=34,locale=en_US \
  --timeout 10m
```

### 6.2 BrowserStack (iOS + Android cross-platform)

**Setup (bir kce):**
1. <https://www.browserstack.com/> → OpenE2EE ekibine katıl (Architect'ten erişim iste).
2. Access Key'i `secrets/browserstack-key.env` olarak kaydet.

**Çalıştırma:**
```bash
cd mobile
# Integration tests için:
flutter drive --driver=test_driver/integration_test.dart \
  --target=integration_test/app_test.dart \
  --device-id="Apple iPhone 15 Pro"
# BrowserStack App Live için: bs://<hash> çıktısını BrowserStack dashboard'a yapıştır.
```

### 6.3 Lokal emulator/simulator

**Android emulator:**
```bash
# Mevcut AVD'leri listele
emulator -list-avds

# Yeni AVD oluştur
sdkmanager --install "system-images;android-34;google_apis;x86_64"
avdmanager create avd -n Pixel7_API34 -k "system-images;android-34;google_apis;x86_64" -d pixel_7
```

**iOS simulator (macOS):**
```bash
xcrun simctl list devices available
# Yeni simulator
xcrun simctl create "iPhone 15 Pro Test" "iPhone 15 Pro" iOS17-0
```

---

## 7. Editor ayarları (VSCode kullanıcıları için)

Repo `.vscode/settings.json` Flutter extension'ı ile çalışacak şekilde ayarlıdır. Ek olarak:

1. **VSCode Extensions** (`.vscode/extensions.json`'da listelendi, otomatik yüklenir):
   - Dart
   - Flutter
   - Android-specific: yok (Android Studio daha iyi)
   - iOS-specific: yok (Xcode daha iyi)
2. **iOS / Android için IDE önerisi:** Flutter kod edit + Dart debug için **VSCode**, native platform-specific kod için (Kotlin/Swift) **Android Studio** veya **Xcode** kullan. Multi-IDE workflow doğal.

---

## 8. Sık karşılaşılan tuzaklar

| Sorun | Çözüm |
|---|---|
| `flutter` PATH'te yok | `MULTIPLATFORM.md` §2/§3/§4'e göre kur |
| `local.properties` eksik | `cp local.properties.example local.properties` ve düzenle |
| Gradle sync "SDK location not found" | `local.properties`'te `sdk.dir=` ayarla |
| Xcode "No signing certificate" | Signing & Capabilities → Team → Apple ID ekle |
| `pod install` "CocoaPods could not find compatible versions" | `pod repo update` ve `flutter clean` |
| `flutter build apk` "Android license status unknown" | `flutter doctor --android-licenses` ve tümünü kabul et |
| Apple Silicon Mac'te simulator "arm64 excluded" | Podfile `post_install`'a `config.build_settings['EXCLUDED_ARCHS[sdk=iphonesimulator*]'] = 'arm64'` ekle |
| `gradlew` "Permission denied" (macOS/Linux) | `chmod +x mobile/android/gradlew` |
| `flutter pub get` "Version solving failed" | `flutter pub upgrade --major-versions` (kırılmaya yol açabilir — dikkatli ol) |
| `.idea/workspace.xml` conflict | `.gitignore`'da whitelist sayesinde commit edilmiyor — ignore |

---

## 9. Sprint 3 §9 kapsamı dışında (Sprint 4+)

Bu rehber yalnızca **dev tooling**'i kapsar. Aşağıdakiler **Sprint 4+**'a ertelenmiştir:

- **Release build:** Signed APK / AAB (Android) ve signed IPA (iOS)
- **Provisioning profiles:** Apple Developer Portal paylaşımlı profiller
- **CI/CD:** GitHub Actions Fastlane lane (signed artifact üretimi)
- **App Store metadata:** Google Play Console + App Store Connect
- **Firebase App Distribution:** Internal QA dağıtımı
- **Kotlin DSL (build.gradle.kts):** Groovy'den Kotlin DSL'e geçiş

Detaylar: `docs/SPRINT-4-PLAN.md` (Sprint 4 başlangıcında yazılacak).

---

## 10. Referanslar

- `docs/MULTIPLATFORM.md` — Windows / macOS / Linux genel kurulum
- `docs/ADR-0008-multiplatform-tooling.md` — Multiplatform tooling kararları
- `docs/SPRINT-3-PLAN-TEMPLATE.md` — Sprint 3 planlama disiplini
- <https://docs.flutter.dev/get-started/install> — Flutter resmi kurulum
- <https://developer.android.com/studio/install> — Android Studio resmi kurulum
- <https://developer.apple.com/xcode/> — Xcode
- <https://guides.cocoapods.org/> — CocoaPods
- <https://firebase.google.com/docs/test-lab> — Firebase Test Lab
- <https://www.browserstack.com/> — BrowserStack

---

**Bu doküman `chore/native-dev-tooling` PR'ı ile birlikte (Sprint 3 §9) eklendi. Sprint 4+ release tooling PR'larında genişletilecek.**