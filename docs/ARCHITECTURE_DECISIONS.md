# OpenE2EE Projesi - Mimari Kararlar ve Teknoloji Yığını (Tech Stack)

Tarih: 1 Temmuz 2026

Fizibilite raporu ve MVP (Minimum Viable Product) hedefleri doğrultusunda alınan nihai mimari kararlar aşağıda listelenmiştir.

## 1. Frontend (Mobil ve Web)
Tüm kullanıcı arayüzleri (hem veri toplayan mobil uygulama hem de B2B SaaS platformu/dashboard) için tek bir kod tabanından ilerlenecektir:
* **Framework:** **Flutter**
* **Neden:** Flutter, hem iOS hem Android için güçlü Native performans sunarken, aynı zamanda Web derlemesi (Flutter Web) ile dashboard ihtiyacını da aynı ekip ve aynı kod tabanı ile çözmeyi sağlar.
* **Grafik ve Görselleştirme:** Şeffaflık matrisi ve entropi/güvenlik skorlarını görselleştirmek için Flutter ekosisteminin en popüler kütüphanesi olan **`fl_chart`** kullanılacaktır.

## 2. Backend (Analiz ve Skorlama Motoru)
Cihazlardan gelen telemetriyi ve paket verilerini işleyecek yüksek performanslı backend motoru:
* **Ana Dil:** **Go (Golang)**
* **Paket Analizi:** **`gopacket`** (Google tarafından geliştirilen paket işleme kütüphanesi).
* **Gopacket Analizi:** `gopacket` kütüphanesi incelendiğinde; `tls.go`, `tls_handshake.go` gibi modülleri ile TLS Client Hello şifreleme süitlerini doğrudan okuyabildiği, IP/TCP/UDP metadata katmanlarını kusursuz ayrıştırdığı teyit edilmiştir. Ayrıca paketlerin ham yüküne (Payload) erişim vererek üzerinden "entropi hesaplaması" yapılmasına tam olanak tanımaktadır. İhtiyaçları tamamen karşılar.

## 3. Veritabanı ve Önbellek (Veri Depolama)
Güvenlik skorları ve zaman serisi (time-series) analizlerini tutacak devasa veri altyapısı:
* **Ana Veritabanı:** **PostgreSQL**
* **Zaman Serisi Uzantısı:** Mevcut production postgre kurulumu içerisinde **TimescaleDB** eklentisi veritabanı bazında aktif edilecektir (Anlık saniyelik trafik loglarını performanslı sorgulamak için).
* **Geliştirme Ortamı (Dev):** `bildirops/postgredev` docker/altyapı kurulumu geliştirme süreci için kullanılacaktır.
* **Önbellek (Cache):** **Redis** (Sık sorgulanan IP adresi imzaları, anlık oturum verileri ve geçici analiz sonuçlarını hızlı sunmak için).

## 4. MVP (Minimum Viable Product) Kapsamı
1. Flutter ile yazılmış, arka planda VPN Service / NetworkExtension ile trafiği kopyalayıp Go sunucusuna telemetri atan bir mobil uygulama.
2. Go ve `gopacket` ile yazılmış, cihazlardan gelen anonim telemetri (örneklenmiş entropi skorları ve maskelenmiş IP/TLS verileri) üzerinden "Küresel Şeffaflık Matrisini" hesaplayan, kuralları işleyip TimescaleDB'ye yazan bir backend.
4. **Hibrit Yük Dağılımı (Pil ve Gizlilik Dengesi):** Uygulama mağazası kuralları gereği ham veriler cihaz dışına çıkamayacağı için entropi cihazda (Native katmanda) hesaplanacaktır. Ancak mobil cihazın pilini tüketmemek adına entropi işlemi sürekli değil, sadece **"Örnekleme (Sampling)"** yöntemiyle (örneğin yeni bir oturumun sadece ilk birkaç paketinde) hesaplanıp backend'e raporlanacaktır. Backend ise bu verileri birleştirip büyük resmi (global şeffaflık matrisi) çizecektir.
3. Aynı Flutter kod tabanı ile derlenmiş, veritabanındaki skorları `fl_chart` kullanarak çizen basit bir Web Dashboard.

## 5. Regülasyon Uyumu ve Mağaza Politikaları (App Store & Google Play)
"Local VPN" arayüzü kullanılarak ağ trafiğinin cihaz içinde izlenmesi, uygulama mağazalarında çok sıkı denetimlere tabidir. Uygulamanın mağazalardan onay alabilmesi için mimaride aşağıdaki prensipler uygulanacaktır:
* **Açık Kaynak Şeffaflığı:** Proje tamamen **Açık Kaynak Kodlu (Open Source)** olacak ve **MIT Lisansı** ile GitHub'da yayınlanacaktır. İnceleme (App Review) ekiplerine kaynak kodun açık olduğu gösterilerek uygulamanın gizli bir ajandası olmadığı (arka kapı veya casus yazılım olmadığı) kanıtlanacaktır.
* **Veri Minimizasyonu (Anonimleştirme):** Flutter uygulaması, ağ paketlerinin ham içeriğini (payload) veya hedeflenen IP adresini **kesinlikle backend sunucusuna göndermeyecektir**. Paket ayrıştırma ve entropi hesaplaması işlemleri cihazda yerel olarak yapılacak ve backend'e sadece *"Uygulama: WhatsApp, Şifreleme Skoru: %99, Operatör: X"* formatında tamamen anonim JSON telemetri verisi gönderilecektir.
* **Pazarlama Konumlandırması:** Uygulama mağazalara bir VPN aracı olarak değil, uçtan uca şifreleme (E2EE) durumunu ve ağ trafiğini test eden bir **"Ağ Güvenliği ve Şeffaflık Aracı (Network Security Tool)"** olarak sunulacaktır (Google Play VpnService istisna kategorisi).
* **Açık Onam (Consent UI):** Uygulamanın ilk açılışında, trafiğin sadece güvenlik skorlaması için cihaz içinde işlendiğini ve sunucuya şifresiz veri aktarılmadığını belirten tam sayfa şeffaf bir aydınlatma/onam ekranı yer alacaktır. iOS tarafında ise sadece resmi `NetworkExtension` API'si kullanılacaktır.

## 6. Operasyonel Model: Gönüllülük ve Görev Odaklı Test (Gamification)
Uygulama arka planda 7/24 çalışan bir izleme aracı olmak yerine, tamamen **kullanıcı tetiklemesiyle (on-demand)** çalışan bir test aracına dönüştürülecektir. Bu modelin mimariye katkıları şunlardır:
* **Görev Tabanlı (Task-Based) Yaklaşım:** Kullanıcı uygulamayı açtığında karşısına *"Turkcell üzerinden RCS testi yap (Görev)"* veya *"WhatsApp şifreleme bütünlüğünü doğrula"* gibi görevler çıkacaktır. VPN profili sadece bu görev süresince (örneğin 2 dakika) aktif edilecek, test bitince kapatılacaktır. Bu durum "pil tüketimi" sorununu kökten çözer.
* **Kontrollü Alıcı (Receiver) Opsiyonları ve MVP Kararı:** MVP aşamasında henüz kurumsal WhatsApp Business API veya RCS sunucu altyapısı (ve bütçesi) bulunmadığı için testler tamamen **P2P (Gönüllü Eşleşmesi)** üzerinden yürüyecektir:
  1. **P2P Gönüllü Eşleşmesi (MVP Ana Yöntemi):** Uygulamanın ilk sürümünde (MVP) testler bizzat kurucular ve erken aşama gönüllüler arasında yapılacaktır. Kullanıcılar (örneğin siz ve ortağınız) uygulamadan "Alıcı Ol (Nöbet)" moduna geçecek ve test mesajlarını birbirinize atarak E2EE doğrulaması yapacaksınız.
  2. **Merkezi Echo-Bot ve Bulut Sanal Numaralar (Faz 2):** Proje büyüdüğünde, son kullanıcılar için manuel süreci ortadan kaldırmak (frictionless UX) adına resmi OpenE2EE bot numaraları ve API entegrasyonları devreye alınacaktır.
  Bu çoklu opsiyon mimarisi sayesinde hem gönderen cihazda hem de alıcıda ağ paketleri eşzamanlı ölçülerek E2EE %100 ispatlanabilecektir.
* **Mağaza Onayında Avantaj:** Kullanıcının testi kendi rızasıyla başlatıp bitirmesi, Apple ve Google'ın "arka planda izinsiz izleme" şüphelerini tamamen ortadan kaldırır.
* **Gönüllü Alıcı Tetikleme (Active Pool Modeli):** iOS işletim sisteminin arkaplanda bildirim okuma (Notification Listener) ve izinsiz VPN başlatma kısıtlamaları nedeniyle P2P testlerde "Aktif Nöbet" modeli kullanılacaktır. Gönüllü kullanıcı uygulamada "Alıcı Ol (15 dk)" butonuna basarak kendi VPN'ini aktif edecek ve Backend'deki hazır alıcılar havuzuna girecektir. Test yapmak isteyen diğer kullanıcılar, havuzdaki bu aktif (nöbetteki) gönüllülerle eşleştirilecektir.
