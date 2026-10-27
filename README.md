# AgyRemote — Mobile Web & Android Control Plane

**AgyRemote** adalah aplikasi mobile dan web control plane modern untuk memantau, mengorkestrasi, dan berinteraksi secara real-time dengan seluruh worker Google Antigravity (**AGY01 – AGY15**).

- **Website / Web App**: [https://agyremote.agentai.my.id](https://agyremote.agentai.my.id)
- **Unduh Android APK**: [https://agyremote.agentai.my.id/agyremote.apk](https://agyremote.agentai.my.id/agyremote.apk)

---

## Fitur Utama

1. **Dashboard Monitoring 15 Worker**:
   - Status real-time 15 worker kluster Antigravity (`agy01` – `agy15`).
   - Penyortiran otomatis (worker aktif berada di posisi teratas).
   - Indikator CPU, RAM, Disk, dan Uptime sistem VPS.

2. **Login & Logout Google OAuth**:
   - Integrasi autentikasi Google OAuth resmi langsung dari antarmuka.
   - Dilengkapi tautan login langsung, fitur salin URL, dan verifikasi kode otorisasi.
   - Kemampuan logout dan pemutusan token kredensial mandiri per worker.

3. **Prompt Interaktif & Live Streaming**:
   - Eksekusi prompt dengan Server-Sent Events (SSE) streaming real-time.
   - Filter pembersih otomatis output CLI tool clutter.
   - Ringkasan eksekutif dan riwayat transkrip per sesi.
   - Pelacakan perubahan file (created, modified, deleted).

4. **Desain Card UI Presisi**:
   - Tema terang profesional dengan sudut sudut subtil 4px (`--card-radius: 4px`).
   - Bayangan berlapis luar dan dalam untuk keterbacaan optimal.
   - Label ringkas dan navigasi bottom bar mobile yang responsif.

5. **Aplikasi Android Standalone (.apk)**:
   - Paket Android native WebView (`com.agyremote.mobile`) teroptimasi tanpa cache kadaluwarsa.
   - Versi 1.0.4 (VersionCode 5).

---

## Struktur Direktori

```
agyremote/
├── src/                    # Frontend React 18 + TypeScript + Vite
│   ├── components/         # Komponen UI (Cards, Sheets, Tabs, Pages)
│   ├── services/           # API Client & SSE streaming service
│   └── types/              # Type definitions telemetri & worker
├── server/                 # Backend Node.js daemon (Port 8787)
│   ├── routes/             # REST endpoints (/workers, /telemetry, /processes)
│   └── services/           # Antigravity executor & process supervisor
├── android/                # Native Android Studio Project
│   └── app/                # Gradle module & AndroidManifest
├── public/                 # Static assets (logo, manifest, favicon)
├── dist/                   # Production bundle & agyremote.apk
└── tests/                  # E2E Test Suite (Tier 1-4, 123 tests)
```

---

## Lisensi
Hak cipta dilindungi. Dikelola oleh Tim Antigravity Cluster.
