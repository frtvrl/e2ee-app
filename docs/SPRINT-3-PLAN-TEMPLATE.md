# OpenE2EE — Sprint 3+ Plan Template & Tasarım Rehberi

**Tarih:** 6 Temmuz 2026 11:23
**Yazan:** Architect (mvs_25a7a987f73243899e35a1485c6ba224)
**Referans:** SPRINT-1-CONSTRAINTS §8, ADR-0008, Sprint 1+2 kapanış dersleri
**Durum:** Accepted (Owner standing by ack)

---

## 1. Background — Engine Auto-Accept Honesty Gap

Sprint 1+2 kapanışında tespit edilen mimari sorun:

- **Mavis team plan engine** "plan_complete" sinyali otomatik merge-ready kabul mekanizması tetikliyor
- **Bu mekanizma kanıtsız** — Owner "merge-ready" ilan etti ama Verifier §6 integration gate olmadan gerçek merge-ready olduğu garanti değil
- **Sprint 1'de 5 kez premature ilan** — Owner dosya yazma ihmali + engine auto-accept birleşince yanlış merge-ready ilanlarına yol açtı
- **Sprint 2'de PR-MP-3 v1→v2 fix zinciri** — Verifier §6 review entegrasyonunun kanıtı (görünmese de çalışıyor)
- **Sprint 1+2 entegrasyonu** — Verifier §6 integration gate (`reports/sprint12-integration-gate.md`, 182 satır, 7 check) gerçek kanıtı sağladı

**Mimari Ders (memory):** Engine auto-accept = güvenilmez. Her sprint kapanışında **iki katmanlı doğrulama** zorunlu.

---

## 2. Mimari Karar — İki Katmanlı Doğrulama

Sprint 3+ için yeni standart:

| Katman | Mekanizma | Sorumluluk | Kanıt |
|---|---|---|---|
| **Katman 1: Plan/Mekanik** | Owner plan kurar, Coder implement eder, engine plan_complete sinyali | Owner / Coder / Engine | Git state (commit + branch + dosya varlık) |
| **Katman 2: Independent Verification** | Verifier §6 integration gate verify-as-task | Verifier (bağımsız) | `reports/<sprint>-integration-gate.md` (7 check kanıt zinciri) |

**Kural:** Integration gate PASS olmadan sprint merge-ready değildir. Engine auto-accept tek başına yeterli değildir.

---

## 3. Sprint 3+ Plan Template

### 3.1 Plan Yapısı (5 adım)

```
Step 1: Owner plan kurar
  ↓ (PR-X task listesi + sprint hedefi + kabul kriterleri)
Step 2: Coder implement
  ↓ (her PR ayrı branch + cherry-pick veya merge)
Step 3: Verifier §6 review (PR başına, verify=on)
  ↓ (her PR'ın kendi §6 review'ı, KAT + adversarial probes)
Step 4: Integration gate (verify-as-task)
  ↓ (pr-<sprint>-gate, 7 check kanıt zinciri)
Step 5: Mimari kabul
  ✅ Integration gate PASS → sprint merge-ready
  ❌ Integration gate FAIL → fix-up plan veya mini plan
```

### 3.2 Task Tipleri

| Tip | verify | Sorumluluk | Çıktı |
|---|---|---|---|
| **PR-X (implement)** | verify=on (PR başına) | Coder | Branch + commit + diff |
| **§6 review (PR başına)** | verify=on | Verifier | `reports/pr<X>-review.md` (her PR) |
| **pr-<sprint>-gate** | auto-off verify-as-task | Verifier | `reports/<sprint>-integration-gate.md` (7 check) |

---

## 4. Coder Task Yapısı (PR-X)

Her PR için Coder'ın üretmesi gereken:

1. **Ayrı branch:** `feat/pr-<N>-<scope>` veya `fix/pr-<N>-<bug>`
2. **Commit stratejisi:** her PR kendi içinde atomik (squash/merge commit)
3. **Commit mesajı:** `feat(<scope>): <description> (Sprint <N> PR-<X>)` veya `fix(<scope>): <fix description>`
4. **Test coverage:** yeni kod ≥60% coverage (kritik fonksiyonlar 100%)
5. **Dokümantasyon:** karmaşık PR'lar için `docs/PR-<X>-NOTES.md` (kısa)

**Coder comm dosyası (5-maddelik mapping):** `coder/workspace/comm/<date>-coder-<N>-pr<X>-review.md` (Sprint 1+2 pattern'i)

---

## 5. Verifier §6 Task Yapısı (PR başına)

Her PR için Verifier §6 review:

1. **Method:** Kod review + test execution (gerekirse) + coverage doğrulama
2. **Evidence:** literal kod satırları + test çıktısı + coverage raporu
3. **Verdict:** PASS veya FAIL (her madde için)
4. **Adversarial probes:** KAT input order assertion + runtime probe (PR-15/PR-3 dersleri)
5. **Privacy spot-check:** ADR-0006 kuralları enforce mi?

**Çıktı:** `reports/pr<N>-review.md` (her PR'ın kendi §6 review dosyası)

---

## 6. Integration Gate (pr-\<sprint\>-gate verify-as-task)

Sprint kapanışında Verifier'ın yazacağı 7 check kanıt zinciri:

### Check A — Branch / Diff Scope
- `git log <sprint-branch> --oneline -N`
- Real integration, premature merge-ready değil
- Sprint-N commit'leri + integration commit + fix-up commit'ler (varsa)

### Check B — Build / Vet / Test
- `go vet ./...` clean
- `go build ./...` clean
- `go test -count=1 ./...` all packages PASS

### Check C — Coverage
- Tüm paketler ≥60% hedef
- **Kritik fonksiyonlar 100%** (masking, validation, KAT, cache, vb.)
- Per-function breakdown (Sprint 1+2 pattern'i)

### Check D — Mobile / Frontend
- `flutter analyze` no issues
- `flutter test` PASS (varsa)
- `flutter build web` PASS (varsa)

### Check E — Privacy Spot-Check
- ADR-0006 §Veri Minimizasyonu enforce
- Masking helpers 100% coverage
- Raw PII sızıntısı yok (KAT input order assertion)
- KVKK DELETE çalışıyor (varsa)

### Check F — Sprint-Specific File Presence
- Sprint PR listesi dosyaları mevcut
- `.editorconfig`, `.gitattributes`, etc. (multiplatform tooling)
- Schema validation, JSON Schema load (varsa)

### Check G — Adversarial Probes
- Static kod analizi (coverage gap analysis)
- Runtime probe (PR-MP-3 v1→v2 fix zinciri kanıtı)
- KAT input split order assertion
- Edge cases (empty, nil, IPv4/IPv6 boundary, vb.)

**Çıktı:** `reports/<sprint>-integration-gate.md` (Sprint 1+2'de `sprint12-integration-gate.md`)

---

## 7. Mimari Kabul Kriterleri

Sprint merge-ready = TÜM koşullar:

- [ ] Tüm PR-X (implement) merge edildi
- [ ] Her PR §6 review (PR başına) PASS
- [ ] Integration gate (pr-<sport>-gate) PASS — 7/7 check
- [ ] Bilinen kısıtlar (Docker runtime yok, CI runner yok, vb.) Sprint-N plan'ında skip olarak işaretli
- [ ] Push kararı: SPRINT-1-CONSTRAINTS §8 uyarınca release anında toplu push

**Mimari kabul = integration gate PASS + push kararı (kullanıcı/Architect)**

---

## 8. Sprint 3 Backlog Önerileri (Mimari Görüş)

Sprint 3 planı kurulmadan önce Owner'a öneriler:

### Öncelik 1 — Teknik Borç Kapatma
| # | PR | Scope | Tahmini Süre |
|---|---|---|---|
| 1 | **PR-19** | PR-15 commit message amend (yanıltıcı "salt \|\| uuid" iddiası düzelt) | 5dk |
| 2 | **PR-MP-CI** | GitHub Actions multi-OS matrix (ubuntu + macos + windows runner) | 60dk |

### Öncelik 2 — Yeni Özellik
| # | PR | Scope | Tahmini Süre |
|---|---|---|---|
| 3 | **PR-21** | WebRTC data channel (Echo-Bot yerine P2P signaling) | 2-3 gün |
| 4 | **PR-22** | Gerçek VPN implementasyonu (Android VpnService + iOS NetworkExtension) | 3-4 gün |
| 5 | **PR-23** | Gerçek operatör API entegrasyonu (BTK MNP feed + IP reverse DNS) | 2-3 gün |

### Öncelik 3 — Release Hazırlık
| # | PR | Scope | Tahmini Süre |
|---|---|---|---|
| 6 | **PR-24** | Android/iOS native build (CI/CD pipeline) | 2-3 gün |
| 7 | **PR-25** | Push to remote trigger (release anında) | 1 gün |
| 8 | **PR-26** | Mağaza yayını (Google Play + App Store metadata) | 1-2 gün |

**Toplam Sprint 3:** ~12-17 gün (paralel PR'lar ile ~7-10 gün)

**Önerilen Sprint 3 scope:** Öncelik 1 (PR-19 + PR-MP-CI) + Öncelik 2'nin 1-2 PR'ı. Tam liste Sprint 3 plan kurulumunda netleşir.

---

## 9. Örnek Sprint 3 Plan Yapısı

```
plan_<id>:
  - PR-19: PR-15 commit message amend (fix)
  - PR-MP-CI: GitHub Actions multi-OS matrix (feat)
  - PR-21: WebRTC data channel (feat) [VEYA]
  - pr-sprint3-gate: integration gate (verify-as-task, 7 check)
```

**Max_concurrency:** 3-4 (Coder session'ları paralel)
**Tahmini süre:** ~3-5 gün (Sprint 3 scope'a bağlı)
**Cost:** ~$0.50-1.00 (büyük PR'lar — WebRTC + VPN)

---

## 10. References

- **ADR-0008-multiplatform-tooling.md** — Sprint 2 multiplatform kararları
- **SPRINT-1-CONSTRAINTS.md** — tüm kısıtlar + §8 push kararı
- **SPRINT-1-OWNER-FINAL.md** — Sprint 1 kapanış notları
- **SPRINT-2-MULTIPLATFORM-PLAN.md** — Sprint 2 plan detayları
- **reports/sprint12-integration-gate.md** — Verifier §6 integration gate örneği (Sprint 1+2 kapanışı, 182 satır)
- **reports/pr18-sprint-gate.md** — Sprint 1 PR-18 gate (referans)
- **reports/pr3-fixup-review.md** — Sprint 1 PR-3 fixup follow-up review (PR-15/3 dersleri)

---

## 11. Özet — Sprint 3+ İçin Altın Kurallar

1. **Engine auto-accept = güvenilmez** — Owner "merge-ready" dediğinde integration gate olmadan kabul etmeyin
2. **İki katmanlı doğrulama:** git state + Verifier §6 integration gate
3. **Integration gate 7 check (A-G):** Branch scope, build/vet/test, coverage, mobile analyze, privacy spot-check, file presence, adversarial probes
4. **PR başına §6 review:** her PR kendi dosyasında (`reports/pr<N>-review.md`)
5. **Push kararı:** §8 release anında toplu push, mimari karar kullanıcı/Architect'te
6. **Sprint 3+ plan template:** 5 adım (Owner → Coder → Verifier PR → Verifier gate → Mimari kabul)

---

**Bu doküman Owner'ın "Sprint 3+ tasarım rehberi body" talebi üzerine yazıldı. Sprint 3 plan kurulumunda bu template uygulanacak.**
