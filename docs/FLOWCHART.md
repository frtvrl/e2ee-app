# OpenE2EE - Görev Odaklı Test Akış Şeması (Flowchart)

Aşağıdaki şema, bir kullanıcının uygulamayı açıp testi başlatmasından sonucun gösterilmesine kadar geçen teknik ve kullanıcı akışını göstermektedir.

```mermaid
sequenceDiagram
    autonumber
    participant User as Kullanıcı (Gönderen)
    participant App as OpenE2EE Mobil App
    participant VPN as Local VPN (Rust/Native)
    participant Backend as Go Backend & DB
    participant Receiver as Gönüllü Alıcı (P2P)

    note over Receiver,Backend: Aktif Nöbet (Active Pool) Aşaması
    Receiver->>Backend: "Gönüllü Alıcı Ol (15 Dk)" Talebini İletir
    Backend-->>Receiver: Hazır Havuzuna (Active Pool) Eklendin
    Receiver->>Receiver: Kendi Cihazında VPN'i Dinlemeye Açar
    
    note over User,Backend: Testin Başlatılması
    User->>App: Test Türünü Seçer (WhatsApp/RCS)
    User->>App: Alıcı Yöntemini Seçer (P2P / Echo-Bot)
    App->>Backend: Yeni Test Oturumu Talebi (Session Request)
    Backend-->>App: Hedef Numara ve Benzersiz Test Metni İlet
    App->>VPN: Local VPN Profilini Aktif Et (Kayıt Başlar)
    App-->>User: Yönlendirme: "Hedef numaraya mesajı atın"
    
    User->>Receiver: Mesajlaşma Uygulamasından Mesajı Gönderir
    
    note over VPN: Veri Minimizasyonu (Cihaz İçi)
    VPN->>VPN: İlk 10 paketi kopyala (Sampling)
    VPN->>VPN: TLS Handshake Oku & Entropi Hesapla
    VPN->>App: Ham Veriyi Sil, Sadece Skorları İlet
    App->>Backend: Anonim Telemetriyi Gönder (JSON)
    App->>VPN: Local VPN Profilini Kapat (Kayıt Biter)
    
    note over Receiver: E2EE Bütünlük Kontrolü
    Receiver->>Backend: Gelen Paketin Entropi ve Hash Değerini Gönder
    
    Backend->>Backend: Gönderen ve Alıcı Skorlarını Karşılaştır
    Backend->>Backend: TimescaleDB'ye Logla (Global Matris)
    Backend-->>App: Doğrulama Sonucunu Dön (Confidence Score)
    
    App-->>User: Sonucu Ekrana Bas (Örn: "Turkcell RCS Şifrelemesi: %99 Başarılı")
```
