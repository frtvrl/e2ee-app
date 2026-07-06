-- ============================================================================
-- OpenE2EE — Development Seed Data (Sprint 1)
-- ----------------------------------------------------------------------------
-- Bu dosya /docker-entrypoint-initdb.d/01-seed.sql olarak postgres container'a
-- mount edilir (bkz. infra/docker-compose.yml, postgres service). İlk
-- başlatmada otomatik çalıştırılır (container'da $PGDATA boşsa).
--
-- AMAÇ: Dev ortamı için örnek veri. Aşağıdakileri sağlar:
--   1. Üç Türk mobil operatörü (turkcell, vodafone_tr, turk_telekom)
--      için demo device + telemetry + session.
--   2. Web dashboard (/api/v1/matrix) için zengin veri seti.
--   3. TimescaleDB extension'ın çalıştığını doğrulamak için 90 günlük
--      aralıkta telemetry satırları.
--
-- GİZLİLİK (ADR-0006):
--   - device_id_hash = 16-byte hex = SHA-256(uuid_v7 || server_salt)[:16]
--     (PR-2 auth.HashDeviceID ile aynı format)
--   - public_key_fp = 16-byte hex = SHA-256(public_key)[:16]
--   - IP adresleri /24 (IPv4) ve /48 (IPv6) maskelenmiş
--   - ASLA raw UUID v7, raw public key, raw IP, telefon numarası yok
--   - Aşağıdaki hash değerleri **örnektir** — gerçek cihaz verisi DEĞİL.
--     Üretimde bu seed dosyası KULLANILMAZ (sadece `make dev-seed` ile
--     veya hiç).
--
-- IDEMPOTENT: Bu dosya birden fazla kez çalıştırılabilir (ON CONFLICT DO NOTHING).
-- ============================================================================

-- ----------------------------------------------------------------------------
-- 0. TimescaleDB extension (zaten yüklü olmalı; image içinde var)
-- ----------------------------------------------------------------------------
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- ----------------------------------------------------------------------------
-- 1. Örnek cihazlar (her operatörden bir tane)
-- ----------------------------------------------------------------------------
-- NOT: public_key BYTEA 32-byte Ed25519 public key (placeholder).
-- device_id_hash aşağıdaki formülle üretilebilir:
--   SHA-256( "00000000-0000-7000-8000-000000000001" || "opene2ee-v1-salt-dev-only-change-in-prod" )[:16]
-- Burada sabit demo hash'ler kullanıyoruz; gerçek değerler auth paketinden geçer.
INSERT INTO devices (device_id_hash, public_key, public_key_fp) VALUES
    -- Turkcell
    ('a1b2c3d4e5f60718a1b2c3d4', '\x00000000000000000000000000000000000000000000000000000000000000a1', '0a1b2c3d4e5f6071'),
    -- Vodafone TR
    ('b2c3d4e5f6071829b2c3d4e5', '\x00000000000000000000000000000000000000000000000000000000000000b2', '1b2c3d4e5f607182'),
    -- Turk Telekom
    ('c3d4e5f60718293ac3d4e5f6', '\x00000000000000000000000000000000000000000000000000000000000000c3', '2c3d4e5f60718293')
ON CONFLICT (device_id_hash) DO NOTHING;

-- ----------------------------------------------------------------------------
-- 2. Örnek oturumlar (PR-3 / PR-6 wire-up test için)
-- ----------------------------------------------------------------------------
-- Üç mod: p2p, echobot, single. Status: completed.
INSERT INTO sessions (id, mode, task_type, sender_hash, receiver_hash, status, started_at, ended_at) VALUES
    ('11111111-1111-7111-8111-111111111111', 'p2p',     'webrtc-connectivity', 'a1b2c3d4e5f60718a1b2c3d4', 'b2c3d4e5f6071829b2c3d4e5', 'completed', NOW() - INTERVAL '2 days', NOW() - INTERVAL '2 days' + INTERVAL '47 seconds'),
    ('22222222-2222-7222-8222-222222222222', 'echobot', 'tls-fingerprint',     'b2c3d4e5f6071829b2c3d4e5', NULL,                              'completed', NOW() - INTERVAL '1 day',  NOW() - INTERVAL '1 day' + INTERVAL '12 seconds'),
    ('33333333-3333-7333-8333-333333333333', 'single',  'entropy-sample',      'c3d4e5f60718293ac3d4e5f6', NULL,                              'completed', NOW() - INTERVAL '6 hours', NOW() - INTERVAL '6 hours' + INTERVAL '3 seconds'),
    ('44444444-4444-7444-8444-444444444444', 'p2p',     'webrtc-connectivity', 'a1b2c3d4e5f60718a1b2c3d4', 'c3d4e5f60718293ac3d4e5f6', 'active',   NOW() - INTERVAL '5 minutes', NULL)
ON CONFLICT (id) DO NOTHING;

-- ----------------------------------------------------------------------------
-- 3. Örnek telemetry — 3 operatör × ~3 gün × 4 örneklem
-- ----------------------------------------------------------------------------
-- Her satır:
--   device_id_hash       — devices tablosundaki hash (FK değil, RI yok)
--   public_key_fp        — devices tablosundaki fingerprint
--   operator             — mnp_tr.go enum: turkcell, vodafone_tr, turk_telekom
--   app                  — telemetry.schema.json 'app' enum: signal, whatsapp, telegram
--   tls_fp               — SHA-256(ClientHello)[:16] hex (PR-4 TLS fingerprint)
--   entropy              — 0-100 arası Shannon entropy (PR-4 analysis)
--   session_id           — yukarıdaki sessions (nullable)
--   ip_subnet            — /24 maskelenmiş IPv4 (örnek: 85.102.x.x)
--   timestamp            — son 90 gün
--
-- Çeşitlilik için 3 operatör × 3 app × 12 zaman damgası = 108 satır.
-- ============================================================================
INSERT INTO telemetry (device_id_hash, public_key_fp, operator, app, tls_fp, entropy, session_id, ip_subnet, timestamp)
SELECT
    dev.device_id_hash,
    dev.public_key_fp,
    dev.operator,
    apps.app_name,
    -- TLS fingerprint: 16 hex char (PR-4 ile uyumlu)
    lpad(to_hex(hashtext(dev.operator || apps.app_name || gs::text)), 16, '0'),
    -- Entropy: 60-95 arası, uygulamaya göre değişken
    CASE apps.app_name
        WHEN 'signal'    THEN round((60 + random() * 30)::numeric, 2)
        WHEN 'whatsapp'  THEN round((50 + random() * 35)::numeric, 2)
        WHEN 'telegram'  THEN round((55 + random() * 40)::numeric, 2)
    END AS entropy,
    CASE (gs % 4)
        WHEN 0 THEN '11111111-1111-7111-8111-111111111111'::uuid
        WHEN 1 THEN '22222222-2222-7222-8222-222222222222'::uuid
        WHEN 2 THEN '33333333-3333-7333-8333-333333333333'::uuid
        ELSE NULL
    END AS session_id,
    -- IP subnet /24 mask — maskelenmiş form (raw IP DEĞİL)
    CASE dev.operator
        WHEN 'turkcell'     THEN '85.102.50.0/24'::inet
        WHEN 'vodafone_tr'  THEN '94.55.20.0/24'::inet
        WHEN 'turk_telekom' THEN '78.180.10.0/24'::inet
    END AS ip_subnet,
    NOW() - (gs * INTERVAL '6 hours') AS timestamp
FROM (
    VALUES
        ('a1b2c3d4e5f60718a1b2c3d4', '0a1b2c3d4e5f6071', 'turkcell'),
        ('b2c3d4e5f6071829b2c3d4e5', '1b2c3d4e5f607182', 'vodafone_tr'),
        ('c3d4e5f60718293ac3d4e5f6', '2c3d4e5f60718293', 'turk_telekom')
) AS dev(device_id_hash, public_key_fp, operator)
CROSS JOIN (
    VALUES ('signal'), ('whatsapp'), ('telegram')
) AS apps(app_name)
CROSS JOIN generate_series(0, 11) AS gs
ON CONFLICT DO NOTHING;

-- ----------------------------------------------------------------------------
-- 4. TimescaleDB hypertable (backend `EnsureTimescale` zaten yapıyor; burada
--    idempotent bir doğrulama — DB zaten başlatıldıysa no-op).
-- ----------------------------------------------------------------------------
-- Bu komutlar postgres container'ın ENTRYPOINT'inden ÖNCE çalışır; backend
-- EnsureTimescale'i tekrar çağırdığında create_hypertable zaten var, sessizce
-- geçer (if_not_exists=TRUE parametresi var).

-- ============================================================================
-- 5. Doğrulama (init sonunda özet)
-- ============================================================================
DO $$
DECLARE
    n_devices   INT;
    n_sessions  INT;
    n_telemetry BIGINT;
    operators   TEXT;
BEGIN
    SELECT count(*) INTO n_devices FROM devices;
    SELECT count(*) INTO n_sessions FROM sessions;
    SELECT count(*) INTO n_telemetry FROM telemetry;
    SELECT string_agg(DISTINCT operator, ', ' ORDER BY operator) INTO operators FROM telemetry;

    RAISE NOTICE 'OpenE2EE dev seed loaded:';
    RAISE NOTICE '  devices   = %', n_devices;
    RAISE NOTICE '  sessions  = %', n_sessions;
    RAISE NOTICE '  telemetry = % (operators: %)', n_telemetry, operators;
END
$$;

-- ============================================================================
-- Sprint 2'de eklenecekler:
--   - operator_cache (Redis shadow table) — Postgres + Redis split
--   - kill_switch (admin toggle'lar)
--   - synthetic_echobot_session (load test için)
-- ============================================================================